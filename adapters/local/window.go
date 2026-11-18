package local

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/adapters/judge/jev"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// A Window is what Run needs to run the mediator of one Accord behind its door.
type Window struct {
	// Key is the mediator's key, and Founding the founding set's bytes.
	Key      signing.Key
	Founding []byte
	// Socket is the path the door listens on, and Journal the journal directory.
	Socket, Journal string
	// Close starts the window along mediator.Close, not Reload: it closes the journal's Accord,
	// writing the closings of its open questions and the close receipt, then serves the closed Accord
	// on Socket, or, with Socket empty, returns (close-1, docs/adr/0022-the-close.md).
	Close bool
	// Disk is what the journal writes through; nil is the operating system's.
	Disk journal.Disk
	// Now is the door's clock (door-owns-clock): read once per call, and at each idle tick.
	Now func() time.Time
	// Rand is read by nothing: the answerer mints its answer's salt, and the door mints none.
	//
	// Deprecated: setting it changes nothing (door-owns-clock,
	// docs/adr/0021-who-decided-and-the-answer-call.md). It is removed at the release.
	Rand io.Reader
	// Stderr takes the journal's repairs and clean-ups.
	Stderr io.Writer
	// Event, when set, is told of every event, from the window's one goroutine.
	Event func(Event)
	// Idle is how often the window ticks with no call; zero is a second.
	Idle time.Duration
	// Ready, when set, is closed once the door listens.
	Ready chan<- struct{}
	// Judges are the judges' configs, each with its hash: exactly one for each judge the card lists,
	// at its hash, or the window starts nothing (docs/adr/0023-judges.md). A window that cannot take a
	// give, a close or a closed journal, needs none.
	Judges []judge.Loaded
	// NewJudge makes the adapter of a judge's config; nil is one that has no key, and says none.
	NewJudge func(judge.Config) judge.Judge
	// Keyed says whether the judges have a key, for EventStarted's Reach only: the window never holds
	// the key.
	Keyed bool
	// Budget is how long a give's decide judges have together, and each advise run; zero is
	// judge.Budget.
	Budget time.Duration
	// CursorRand is read once at each start for the random value of the cursors the questions
	// replies carry, so a cursor from before a restart is never taken for one of this start's; nil is
	// crypto/rand.
	CursorRand io.Reader
	// Deadline is how long a connection may take to send its call and read its reply, when the call
	// does not wait; one that waits has its wait plus WaitSlack, when that is longer. Zero is 30
	// seconds.
	Deadline time.Duration
	// replyLimit is the most bytes a reply that carries the close may have; zero is MaxReply. Only
	// a test sets it, to reach the bound without a queue that long.
	replyLimit int
}

// WaitSlack is how much longer than its wait a connection whose call waits may stay open: the call
// is answered at the end of its wait at the latest.
const WaitSlack = 10 * time.Second

// What an Event is.
const (
	// EventStarted: the Accord is founded or reloaded, and the door listens.
	EventStarted = "started"
	// EventReloaded: a reload closed the questions a stop left open.
	EventReloaded = "reloaded"
	// EventClosed: a close wrote its closings and the close receipt, in one journal write; Outcomes
	// holds the closings and Close the close's id (close-1).
	EventClosed = "closed"
	// EventCall: a call the door let through, with the receipts it wrote.
	EventCall = "call"
	// EventTick: the passing of time closed questions.
	EventTick = "tick"
	// EventRefused: a call the door refused.
	EventRefused = "refused"
	// EventAdvised: a receipt's advise judges said what Advice holds, and their records are in its
	// judged file (docs/adr/0023-judges.md).
	EventAdvised = "advised"
)

// An Event is one thing the window did. It holds ids, codes and receipts, never a body, a salt, a
// question's part or a judge's output.
type Event struct {
	Kind string
	// For started: whether the Accord was founded now, and its id and card.
	Founded      bool
	Accord, Card string
	// For started on a closed Accord, and for closed: the close receipt's id, and its bytes in
	// CloseReceipt.
	Close        string
	CloseReceipt []byte
	// For a call: its op and caller.
	Op, Caller string
	// The receipts written, in order, and the index among them of a release receipt, or -1.
	Outcomes []mediator.Outcome
	Release  int
	// What refused the call: the door's code, the mediator's rejection or its error.
	Door      DoorCode
	Rejection mediator.RejectionCode
	Error     mediator.ErrorCode
	// For a get that handed out a delivery: the id of the receipt that queued it.
	Fetched string
	// For started, on a window that can take a give: one Reach per judge the card lists, in card
	// order.
	Reach []Reach
	// For advised: what each advise judge said of a receipt's part, in the rule's order.
	Advice []Advice
}

// An Advice is what an advise judge said of a receipt's part: its name, the outcome, pass, block or
// none, and the receipt's id. It holds nothing of the judge's output.
type Advice struct {
	Judge, Outcome, Receipt string
}

// A Reach is where a judge's requests may go, as the window prints it at start: the judge's name, the
// host and model of its config, the kinds of every term that lists it, a superset of what can reach
// it, and whether the window has a key for it (docs/adr/0023-judges.md).
type Reach struct {
	Judge, Host, Model string
	Kinds              []string
	Keyed              bool
}

// window is a running window's state, owned by its one goroutine (one-mediator-goroutine).
type window struct {
	cfg    Window
	m      *mediator.Mediator
	j      *journal.Journal
	nonces map[string]time.Time
	// The judges by name, their reach, and the callers whose give is with a judge.
	judges map[string]judge.Configured
	reach  []Reach
	busy   map[string]bool
	// The calls parked until they have something to return or their wait ends, oldest first; the
	// deliveries queued for each member, as the journal holds them, so a get that waits learns of
	// one without taking it; and, for the cursors, this start's random value, the number of asks
	// journaled since the start, and the position of each among them.
	parked []*parked
	queued map[string]int
	epoch  string
	asks   int
	askAt  map[string]int
	// What the judge runs need of serve: the context that stops them, the group that waits for
	// them, the channel their results come back on, and serve's end.
	runs    context.Context
	wg      *sync.WaitGroup
	results chan result
	done    <-chan struct{}
}

// A result is what a judge run left. For a decide run: the call, the time it arrived, the reply
// channel its connection waits on, the judgment, the run's records, and the advise run to start once
// the receipt is written, or nil. For an advise run: the receipt it was about, in receipt, and its
// records.
type result struct {
	call     Call
	at       string
	reply    chan []byte
	judgment *composition.Judgment
	records  []judge.Record
	advise   *advising
	receipt  string
}

// An advising is the advise judges of a give's rule, and what they are asked.
type advising struct {
	stack judge.Stack
	req   judge.Request
}

type request struct {
	line     []byte
	tooLarge bool
	reply    chan []byte
	// extend sets the connection's deadline to d after it was accepted, for a call that waits.
	extend func(d time.Duration)
}

// A parked call waits for something to return, until its wait ends: the call, the reply channel
// its connection waits on, and the end of its wait, on the system's monotonic clock.
type parked struct {
	call  Call
	reply chan []byte
	until time.Time
}

// Run runs a window until ctx is done, then returns nil having written nothing more. A socket path
// too long is CodeSocketPath before anything else is done; so is, when it listens, a path within 3
// bytes of MaxSocketPath whose last element has fewer than 4 bytes (privateBound). Otherwise it opens the journal and takes
// its lock; founds the Accord at the current second on an empty journal or reloads the journal's, journaling the closings of what a stop left open; removes a stale socket
// file; and only then listens. Every call is handled by one goroutine, which owns the mediator. A
// journal write that fails ends Run with that error, and the call it was for gets no reply
// (journal-synced); so does a failure of the mediator outside a call's refusal, when it ticks, reads
// a question it just asked or closes one nobody can answer. An accept that fails for a reason that
// does not pass ends Run with CodeSocket. Run returns only once every goroutine it started has ended.
//
// With Close set it closes the Accord in place of the reload (close-1): an empty journal is
// CodeJournal, unfounded; a closed one is mediator's *Error closed; it emits EventClosed. With Socket
// empty it then lets the lock go and returns nil without listening; with Socket set it serves the
// closed Accord as any start does.
func Run(ctx context.Context, cfg Window) error {
	if !cfg.Close || cfg.Socket != "" {
		if err := CheckSocket(cfg.Socket); err != nil {
			return err
		}
	}
	if cfg.Idle == 0 {
		cfg.Idle = time.Second
	}
	if cfg.Stderr == nil {
		cfg.Stderr = io.Discard
	}
	if cfg.Event == nil {
		cfg.Event = func(Event) {}
	}
	disk := cfg.Disk
	if disk == nil {
		disk = journal.OS
	}
	j, err := journal.OpenWith(cfg.Journal, cfg.Stderr, disk)
	if err != nil {
		return err
	}
	defer j.Close()
	if cfg.NewJudge == nil {
		cfg.NewJudge = func(c judge.Config) judge.Judge { return jev.New(c, "", nil) }
	}
	if cfg.Budget == 0 {
		cfg.Budget = judge.Budget
	}
	if cfg.Deadline == 0 {
		cfg.Deadline = readDeadline
	}
	if cfg.CursorRand == nil {
		cfg.CursorRand = rand.Reader
	}
	w := &window{cfg: cfg, j: j, nonces: map[string]time.Time{}, busy: map[string]bool{}, queued: map[string]int{}, askAt: map[string]int{}}
	e := make([]byte, 16)
	if _, err := io.ReadFull(cfg.CursorRand, e); err != nil {
		return &Error{Code: CodeCursor}
	}
	w.epoch = hex.EncodeToString(e)
	founded, err := w.start()
	if err != nil {
		return err
	}
	if cfg.Close && cfg.Socket == "" {
		return nil
	}
	l, err := listen(cfg.Socket, nil)
	if err != nil {
		return err
	}
	defer l.Close()
	started := Event{Kind: EventStarted, Founded: founded, Accord: w.m.Accord(), Card: w.m.Card(), Reach: w.reach}
	if c, ok := w.m.Closed(); ok {
		started.Close, started.CloseReceipt = c.ID, c.Receipt
	}
	cfg.Event(started)
	if cfg.Ready != nil {
		close(cfg.Ready)
	}
	return w.serve(ctx, l)
}

// start founds or reloads the mediator, and reports whether it founded.
func (w *window) start() (bool, error) {
	rec, err := w.j.Read()
	if err != nil {
		return false, err
	}
	at := Stamp(w.cfg.Now())
	if rec.Empty && w.cfg.Close {
		return false, &journal.Error{Code: journal.CodeJournal, Core: string(mediator.CodeUnfounded)}
	}
	if rec.Empty {
		m, err := mediator.Found(w.cfg.Founding, w.cfg.Key, at)
		if err != nil {
			return false, err
		}
		if err := w.loadJudges(at); err != nil {
			return false, err
		}
		if err := w.j.Found(m.Accord(), m.Card(), w.cfg.Key.Reference(), at); err != nil {
			return false, err
		}
		w.m = m
		return true, nil
	}
	if id, err := canonical.TypedHash(w.cfg.Founding); err != nil || id != rec.Accord || rec.Mediator != w.cfg.Key.Reference() {
		return false, &journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1, Core: "accord"}
	}
	if w.cfg.Close {
		return false, w.close(rec, at)
	}
	m, outs, err := mediator.Reload(w.cfg.Founding, rec.Receipts, rec.Open, rec.Queued, rec.FoundedAt, w.cfg.Key, at)
	if err != nil {
		return false, reloadError(err, rec)
	}
	w.count(rec.Queued)
	if m.Card() != rec.Card {
		return false, &journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1, Core: "card"}
	}
	w.m = m
	// A closed Accord takes no give, so it needs no judge.
	if _, closed := m.Closed(); !closed {
		if err := w.loadJudges(rec.FoundedAt); err != nil {
			return false, err
		}
	}
	if err := w.j.RemoveStrays(rec); err != nil {
		return false, err
	}
	if err := w.j.AddReceipts(receiptsOf(outs)); err != nil {
		return false, err
	}
	if len(outs) > 0 {
		w.cfg.Event(Event{Kind: EventReloaded, Outcomes: outs, Release: -1})
	}
	return false, nil
}

// loadJudges compares the card's judges with the window's configs, before anything is written to
// the journal (docs/adr/0023-judges.md): each judge the card lists needs exactly one config of its
// name whose hash is the card's, at this adapter's version, and each config must be one the card
// lists; otherwise CodeJudge. It makes each judge's adapter, and the reach the window prints. The
// Accord is founded at foundedAt, as the mediator was.
func (w *window) loadJudges(foundedAt string) error {
	a, err := setup.Found(w.cfg.Founding, foundedAt)
	if err != nil {
		return err
	}
	loaded := map[string]judge.Loaded{}
	for _, l := range w.cfg.Judges {
		if _, twice := loaded[l.Config.Name]; twice {
			return &Error{Code: CodeJudge}
		}
		loaded[l.Config.Name] = l
	}
	cards := a.CardJudges()
	if len(cards) != len(loaded) {
		return &Error{Code: CodeJudge}
	}
	w.judges = map[string]judge.Configured{}
	w.reach = nil
	for _, c := range cards {
		l, ok := loaded[c.Name]
		if !ok || l.Hash != c.Hash || c.Version != jev.Version {
			return &Error{Code: CodeJudge}
		}
		w.judges[c.Name] = judge.Configured{Config: l.Config, Judge: w.cfg.NewJudge(l.Config)}
		w.reach = append(w.reach, Reach{Judge: c.Name, Host: l.Config.Host, Model: l.Config.Model,
			Kinds: a.JudgeKinds(c.Name), Keyed: w.cfg.Keyed})
	}
	return nil
}

// close closes the journal's Accord at the time at (close-1): the closings and the close receipt in
// one journal write, then EventClosed. A closed journal is mediator's *Error closed, returned as it
// is; any other refusal is named as reloadError names it. A close receipt whose closeReply is longer
// than MaxReply is CodeJournal, receipts.jsonl, CoreCloseSize, and nothing is written: every reply
// that carries the close, and every line of the journal and of a state directory, holds it whole.
func (w *window) close(rec *journal.Record, at string) error {
	m, outs, closed, err := mediator.Close(w.cfg.Founding, rec.Receipts, rec.Open, rec.Queued, rec.FoundedAt, w.cfg.Key, at)
	if err != nil {
		var me *mediator.Error
		if errors.As(err, &me) && me.Code == mediator.CodeClosed {
			return err
		}
		return reloadError(err, rec)
	}
	w.count(rec.Queued)
	if m.Card() != rec.Card {
		return &journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1, Core: "card"}
	}
	limit := w.cfg.replyLimit
	if limit == 0 {
		limit = MaxReply
	}
	if b := closeReply(closed.Receipt); len(b) > limit || string(b) == internalReply {
		return &journal.Error{Code: journal.CodeJournal, File: journal.FileReceipts, Core: CoreCloseSize}
	}
	w.m = m
	if err := w.j.RemoveStrays(rec); err != nil {
		return err
	}
	if err := w.j.AddReceipts(append(receiptsOf(outs), closed.Receipt)); err != nil {
		return err
	}
	w.cfg.Event(Event{Kind: EventClosed, Accord: m.Accord(), Card: m.Card(), Outcomes: outs, Release: -1,
		Close: closed.ID, CloseReceipt: closed.Receipt})
	return nil
}

// CoreCloseSize follows error journal receipts.jsonl when a close receipt would not fit in the
// largest reply that carries it: the members fetch what is queued for them, and the close runs again
// (docs/adr/0022-the-close.md).
const CoreCloseSize = "close-size"

// closeReply is the largest reply that carries the close receipt c: the questions reply of a closed
// Accord, the close beside an empty list and the longest cursor a window mints. A get's reply, the
// close alone, is shorter.
func closeReply(c []byte) []byte {
	empty := []wireAsked{}
	longest := (&window{epoch: strings.Repeat("0", 32)}).cursor(math.MaxInt64)
	return encodeReply(wireReply{Questions: &empty, Close: c, Cursor: &longest})
}

// reloadError names the file and line of what Reload refused.
func reloadError(err error, rec *journal.Record) error {
	var me *mediator.Error
	if !errors.As(err, &me) {
		return err
	}
	e := &journal.Error{Code: journal.CodeJournal, Core: string(me.Code)}
	switch me.Code {
	case mediator.CodeJournal:
		e.File = journal.FileReceipts
		if me.Index >= 0 {
			e.Line = me.Index + 1
		}
		var re *receipts.Error
		if errors.As(me, &re) {
			e.Core = string(re.Code)
		}
	case mediator.CodeClose:
		// A close cut short: its closed closings are in receipts.jsonl, with no close after them.
		e.File = journal.FileReceipts
	case mediator.CodeHeld:
		e.File = journal.DirHeld
		if me.Index >= 0 && me.Index < len(rec.HeldFiles) {
			e.File = rec.HeldFiles[me.Index]
		}
	case mediator.CodeQuestion, mediator.CodeAt:
		e.File = journal.FileQuestions
		if me.Index >= 0 && me.Index < len(rec.QuestionLines) {
			e.Line = rec.QuestionLines[me.Index]
		}
	}
	return e
}

// count notes the deliveries the journal held at the start, by the member each is queued for.
func (w *window) count(list []mediator.Queued) {
	for _, q := range list {
		w.queued[q.To]++
	}
}

// putHeld journals a delivery for its member, and counts it.
func (w *window) putHeld(q mediator.Queued) error {
	if err := w.j.PutHeld(q.To, q.Delivery); err != nil {
		return err
	}
	w.queued[q.To]++
	return nil
}

func receiptsOf(outs []mediator.Outcome) [][]byte {
	list := make([][]byte, len(outs))
	for i, o := range outs {
		list[i] = o.Receipt
	}
	return list
}

// listen removes a stale socket file at path and listens there, mode 0600. A socket file is stale
// when connecting to it is refused: one that something answers on, or that cannot be tried, is left
// to its owner, and so is a path that holds anything but a socket. The listener never removes the
// file: a window stopped in any way leaves it, and the next start removes it. beforeLink, when not
// nil, is called with the bound socket's path before the socket is linked to path.
//
// The socket is bound at a path no longer than path itself whenever path's last element has at least
// 4 bytes (privateBound), so every such path CheckSocket takes can be bound. When the bound path is
// longer than MaxSocketPath, listen is CodeSocketPath.
func listen(path string, beforeLink func(bound string)) (*net.UnixListener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&fs.ModeSocket == 0 {
			return nil, &Error{Code: CodeSocket}
		}
		c, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			_ = c.Close()
		}
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, &Error{Code: CodeSocket}
		}
		if err := os.Remove(path); err != nil {
			return nil, &Error{Code: CodeSocket}
		}
	}
	// The socket is bound in a directory of mode 0700 that this window has just made beside the path,
	// set to mode 0600, and only then linked to the path. So no other user can connect to it before
	// its mode is set, on any system, and no process-wide umask is changed. A link, unlike a rename,
	// never replaces a socket another window bound at the path meanwhile. A window killed before it
	// removes the directory leaves it beside the path, holding a socket nothing answers on.
	dir, bound, err := privateBound(path)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: bound, Net: "unix"})
	if err != nil {
		return nil, &Error{Code: CodeSocket}
	}
	l.SetUnlinkOnClose(false)
	if err := os.Chmod(bound, 0o600); err != nil {
		_ = l.Close()
		return nil, &Error{Code: CodeSocket}
	}
	if beforeLink != nil {
		beforeLink(bound)
	}
	if err := os.Link(bound, path); err != nil {
		_ = l.Close()
		return nil, &Error{Code: CodeSocket}
	}
	return l, nil
}

// privateBound makes a directory of mode 0700 beside path and returns it with the path to bind the
// socket at inside it, <dir>/.<random>/s. The random part has 3 bytes fewer than path's last
// element, at least 1 and at most 26, so the bound path is exactly as long as path when that element
// has 4 to 29 bytes, shorter when it has more, and longer by at most 3 bytes when it has fewer. A
// bound path longer than MaxSocketPath is CodeSocketPath, before anything is made. A name already
// taken is tried again with another.
func privateBound(path string) (dir, bound string, err error) {
	n := min(max(len(filepath.Base(path))-3, 1), 26)
	for range 10000 {
		dir = filepath.Join(filepath.Dir(path), "."+rand.Text()[:n])
		if len(dir)+len("/s") > MaxSocketPath {
			return "", "", &Error{Code: CodeSocketPath}
		}
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return dir, filepath.Join(dir, "s"), nil
		}
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	return "", "", &Error{Code: CodeSocket}
}

// serve accepts connections, each read on a goroutine of its own, and handles every call on this
// one, which owns the mediator. It owns every goroutine it starts (goroutines-owned): when it returns,
// it has closed the listener, cut every open connection short and waited for each goroutine to end.
// An accept that fails for want of a file descriptor, or times out, is tried again after a pause that
// doubles from minAcceptPause to maxAcceptPause; any other failure ends serve with CodeSocket.
func (w *window) serve(ctx context.Context, l *net.UnixListener) error {
	reqs := make(chan request)
	done := make(chan struct{})
	acceptErr := make(chan error, 1)
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		conns  = map[net.Conn]struct{}{}
		closed bool
	)
	// The judge runs are this goroutine's too: they stop when it returns, and it waits for them.
	runs, stopRuns := context.WithCancel(ctx)
	w.runs, w.wg, w.results, w.done = runs, &wg, make(chan result), done
	defer func() {
		stopRuns()
		close(done)
		_ = l.Close()
		mu.Lock()
		closed = true
		for c := range conns {
			_ = c.SetDeadline(time.Now())
		}
		mu.Unlock()
		wg.Wait()
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		var pause time.Duration
		for {
			c, err := l.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
				}
				if !temporary(err) {
					acceptErr <- &Error{Code: CodeSocket}
					return
				}
				pause = min(max(2*pause, minAcceptPause), maxAcceptPause)
				t := time.NewTimer(pause)
				select {
				case <-t.C:
					continue
				case <-done:
					t.Stop()
					return
				}
			}
			pause = 0
			// The deadline is set before the connection is registered, so the shutdown's deadline of
			// now, set to every registered connection, is never overwritten by a later one.
			accepted := time.Now()
			_ = c.SetDeadline(accepted.Add(w.cfg.Deadline))
			mu.Lock()
			if closed {
				mu.Unlock()
				_ = c.Close()
				return
			}
			conns[c] = struct{}{}
			wg.Add(1)
			mu.Unlock()
			go func() {
				defer wg.Done()
				read(c, reqs, done, func(d time.Duration) { _ = c.SetDeadline(accepted.Add(d)) })
				mu.Lock()
				delete(conns, c)
				mu.Unlock()
			}()
		}
	}()
	tick := time.NewTicker(w.cfg.Idle)
	defer tick.Stop()
	// The end of the soonest wait of a parked call: a timer this goroutine owns, stopped on return.
	ends := time.NewTimer(time.Hour)
	ends.Stop()
	defer ends.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-acceptErr:
			return err
		case <-tick.C:
			if err := w.tick(Stamp(w.cfg.Now()), EventTick); err != nil {
				return err
			}
		case <-ends.C:
			// A wait ends: the time passes first, as for a call, so the reply is the one the call would
			// get now.
			if err := w.tick(Stamp(w.cfg.Now()), EventTick); err != nil {
				return err
			}
		case r := <-reqs:
			reply, err := w.handle(r)
			if err != nil {
				return err
			}
			// A give with a judge, or a call parked, has no reply yet: its result or the wake brings it.
			if reply != nil {
				r.reply <- reply
			}
		case res := <-w.results:
			// A run the stop cut short writes nothing: the window is stopping.
			if ctx.Err() != nil {
				return nil
			}
			if res.receipt != "" {
				if err := w.advised(res); err != nil {
					return err
				}
				continue
			}
			delete(w.busy, res.call.Caller)
			ev := Event{Kind: EventCall, Op: OpGive, Caller: res.call.Caller, Release: -1}
			reply, err := w.decide(res.call, res.at, &ev, res.judgment, res.records, res.advise)
			if err != nil {
				return err
			}
			w.cfg.Event(ev)
			res.reply <- reply
		}
		// Whatever changed, every parked call is looked at again, by this goroutine
		// (one-mediator-goroutine), and the timer is set for the soonest wait left.
		w.wake(time.Now())
		ends.Stop()
		if len(w.parked) > 0 {
			soonest := w.parked[0].until
			for _, p := range w.parked[1:] {
				if p.until.Before(soonest) {
					soonest = p.until
				}
			}
			ends.Reset(time.Until(soonest))
		}
	}
}

// wake answers every parked call that now has something to return, and every one whose wait has
// ended by now with the reply it gets at this moment; it keeps the others parked, in their order.
func (w *window) wake(now time.Time) {
	kept := w.parked[:0]
	for _, p := range w.parked {
		ev := Event{Kind: EventCall, Op: p.call.Op, Caller: p.call.Caller, Release: -1}
		reply, has := w.waiting(p.call, &ev)
		if !has && now.Before(p.until) {
			kept = append(kept, p)
			continue
		}
		w.cfg.Event(ev)
		p.reply <- reply
	}
	for i := len(kept); i < len(w.parked); i++ {
		w.parked[i] = nil
	}
	w.parked = kept
	w.prune()
}

// prune drops the positions of asks no longer open, once they outnumber the open ones.
func (w *window) prune() {
	open := w.m.Pending()
	if len(w.askAt) <= 2*len(open)+16 {
		return
	}
	at := make(map[string]int, len(open))
	for _, id := range open {
		if n, ok := w.askAt[id]; ok {
			at[id] = n
		}
	}
	w.askAt = at
}

// The pauses between tries of an accept that failed for a while.
const (
	minAcceptPause = 10 * time.Millisecond
	maxAcceptPause = time.Second
)

// temporary reports whether an accept's failure may pass: the process or the system is out of file
// descriptors, or the accept timed out.
func temporary(err error) bool {
	var ne net.Error
	return errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) || (errors.As(err, &ne) && ne.Timeout())
}

// readDeadline is how long a connection may take to send its call and read its reply, when the
// window's Deadline is zero and the call does not wait, and replyMargin how long it has to read the
// reply once the window writes it. judge.Budget and replyMargin together are below readDeadline, so
// a judged give's reply is written in time.
const (
	readDeadline = 30 * time.Second
	replyMargin  = 5 * time.Second
)

// read reads one call from c, hands it to the window, and writes the reply, if one comes. Its
// deadline is set before it starts, and the window lengthens it, with extend, for a call that waits;
// the reply's write has a deadline of its own, replyMargin, so a call that was slow to arrive does
// not shorten the reply's time.
func read(c net.Conn, reqs chan<- request, done <-chan struct{}, extend func(time.Duration)) {
	defer c.Close()
	br := bufio.NewReaderSize(c, 64<<10)
	var line []byte
	tooLarge := false
	for {
		chunk, err := br.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxCall+1 {
			tooLarge, line = true, nil
			break
		}
		if err == nil {
			line = line[:len(line)-1]
			break
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return
		}
	}
	r := request{line: line, tooLarge: tooLarge, reply: make(chan []byte, 1), extend: extend}
	select {
	case reqs <- r:
	case <-done:
		return
	}
	select {
	case b := <-r.reply:
		_ = c.SetWriteDeadline(time.Now().Add(replyMargin))
		_, _ = c.Write(append(b, '\n'))
	case <-done:
	}
}

// tick passes the time to at and journals what it closed. The mediator failing to is a failure of
// the core, which ends the window as a failed journal write does.
func (w *window) tick(at, kind string) error {
	outs, err := w.m.Tick(at)
	if err != nil {
		return err
	}
	if len(outs) == 0 {
		return nil
	}
	if err := w.j.AddReceipts(receiptsOf(outs)); err != nil {
		return err
	}
	w.cfg.Event(Event{Kind: kind, Outcomes: outs, Release: -1})
	return nil
}

// handle runs one call: the door's checks in order, the tick, then the op. It returns the reply, or
// a journal failure, for which no reply is sent.
func (w *window) handle(r request) ([]byte, error) {
	now := w.cfg.Now().UTC().Truncate(time.Second)
	at := Stamp(now)
	refuse := func(code DoorCode) ([]byte, error) {
		w.cfg.Event(Event{Kind: EventRefused, Door: code, Release: -1})
		return refusal(byDoor, string(code)), nil
	}
	if r.tooLarge {
		return refuse(TooLarge)
	}
	c, code := ParseCall(r.line)
	if code != "" {
		return refuse(code)
	}
	if !c.Verified() {
		return refuse(BadSignature)
	}
	if c.Accord != w.m.Accord() {
		return refuse(WrongAccord)
	}
	t, _ := time.Parse(time.RFC3339, c.At)
	if d := t.Sub(now); d > StaleSeconds*time.Second || d < -StaleSeconds*time.Second {
		return refuse(Stale)
	}
	for n, seen := range w.nonces {
		if now.Sub(seen) > ReplaySeconds*time.Second {
			delete(w.nonces, n)
		}
	}
	if _, seen := w.nonces[c.Nonce]; seen {
		return refuse(Replayed)
	}
	w.nonces[c.Nonce] = now
	// A caller's give while its last is with a judge is refused, so its gives reach the core in the
	// order of their seq (crossing-4, docs/adr/0023-judges.md); its other calls go on.
	if c.Op == OpGive && w.busy[c.Caller] {
		return refuse(JudgeBusy)
	}

	if err := w.tick(at, EventTick); err != nil {
		return nil, err
	}
	if c.Wait > 0 && r.extend != nil {
		r.extend(max(w.cfg.Deadline, time.Duration(c.Wait)*time.Second+WaitSlack))
	}
	ev := Event{Kind: EventCall, Op: c.Op, Caller: c.Caller, Release: -1}
	reply, err := w.op(c, at, &ev, r.reply)
	if err != nil || reply == nil {
		return nil, err
	}
	w.cfg.Event(ev)
	return reply, nil
}

// refused is the reply to an op the mediator refused, noted on the event.
func refused(err error, ev *Event) []byte {
	var rj *mediator.Rejection
	if errors.As(err, &rj) {
		ev.Rejection = rj.Code
		return refusal(byRejection, string(rj.Code))
	}
	var me *mediator.Error
	if errors.As(err, &me) {
		ev.Error = me.Code
		return refusal(byError, string(me.Code))
	}
	ev.Door = Internal
	return refusal(byDoor, string(Internal))
}

// op runs the call's operation. A give that a judge runs on returns no reply and no error: its
// result brings the reply.
func (w *window) op(c Call, at string, ev *Event, reply chan []byte) ([]byte, error) {
	switch c.Op {
	case OpGive:
		return w.give(c, at, ev, reply)
	case OpGet:
		if c.Wait == 0 {
			return w.get(c, ev)
		}
	case OpResolve:
		return w.resolve(c, at, ev)
	}
	// questions, result, and a get that waits: the reply now, unless the call waits and has nothing
	// to return yet, when it is parked, and the wake answers it.
	b, has := w.waiting(c, ev)
	if has || c.Wait == 0 {
		return b, nil
	}
	w.parked = append(w.parked, &parked{call: c, reply: reply, until: time.Now().Add(time.Duration(c.Wait) * time.Second)})
	return nil, nil
}

// waiting is the reply to a questions, a result or a get that waits, as it would be now, and
// whether it has something to return: a refusal, a question, a closing, a delivery queued or the
// close. A get that waits never takes a delivery: its reply says one is queued.
func (w *window) waiting(c Call, ev *Event) ([]byte, bool) {
	switch c.Op {
	case OpQuestions:
		return w.questions(c, ev)
	case OpResult:
		return w.result(c, ev)
	}
	if _, _, err := w.m.Result(c.Caller, ""); err != nil {
		// No question has the empty id, so the read answers with its membership row, as get's would.
		return refused(err, ev), true
	}
	if w.queued[c.Caller] > 0 {
		t := true
		return encodeReply(wireReply{Queued: &t}), true
	}
	if cl, ok := w.m.Closed(); ok {
		return encodeReply(wireReply{Close: cl.Receipt}), true
	}
	return encodeReply(wireReply{}), false
}

// give asks the core which judges the give would reach (composition-7). With no decide judge it
// decides the give at once with no judgment. A raw body is sent to no judge: each judge the rule
// lists says none, raw-body. Otherwise it marks the caller busy and runs the decide judges on a
// goroutine of the window's, and goes on serving; their judgment comes back on the results channel,
// and serve decides the give with it, at the time the give arrived (door-owns-clock).
func (w *window) give(c Call, at string, ev *Event, reply chan []byte) ([]byte, error) {
	jg, judged := w.m.Judges(c.Caller, c.Message, c.Envelope)
	if !judged {
		return w.decide(c, at, ev, nil, nil, nil)
	}
	if jg.Form == formRaw {
		var recs []judge.Record
		for _, j := range jg.Judges {
			recs = append(recs, judge.Record{Judge: j.Name, Use: string(j.Use), Outcome: outcomeNone, Error: string(judge.CodeRawBody)})
		}
		return w.decide(c, at, ev, nil, recs, nil)
	}
	s := w.stack(jg.Judges)
	req := judge.Request{Kind: jg.Kind, Body: jg.Body}
	var adv *advising
	if hasUse(s, setup.Advise) {
		adv = &advising{stack: s, req: req}
	}
	if !hasUse(s, setup.Decide) {
		return w.decide(c, at, ev, nil, nil, adv)
	}
	w.busy[c.Caller] = true
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		j, recs := judge.Run(w.runs, s, req, w.cfg.Budget)
		select {
		case w.results <- result{call: c, at: at, reply: reply, judgment: j, records: recs, advise: adv}:
		case <-w.done:
		}
	}()
	return nil, nil
}

// formRaw is the form of a raw body, which is never sent to a judge; outcomeNone is a run that
// decided nothing.
const (
	formRaw     = "raw"
	outcomeNone = "none"
)

// stack is the rule's judges, in its order, each with its config and adapter.
func (w *window) stack(js []setup.Judge) judge.Stack {
	var s judge.Stack
	for _, j := range js {
		c := w.judges[j.Name]
		c.Use = j.Use
		s = append(s, c)
	}
	return s
}

// hasUse reports whether the stack holds a judge of use u.
func hasUse(s judge.Stack, u setup.Use) bool {
	for _, c := range s {
		if c.Use == u {
			return true
		}
	}
	return false
}

// decide gives the call's part with the judgment, journals its receipt and then the judges' records
// of it, starts the advise run, when there is one, and returns the reply. A give the core rejects
// drops the records and advises nothing.
func (w *window) decide(c Call, at string, ev *Event, judgment *composition.Judgment, recs []judge.Record, adv *advising) ([]byte, error) {
	out, err := w.m.Give(c.Caller, c.Message, c.Envelope, judgment, at)
	if err != nil {
		return refused(err, ev), nil
	}
	switch out.Verdict {
	case string(composition.Pass):
		if q, ok := w.m.Queued(out.ReceiptID); ok {
			if err := w.putHeld(q); err != nil {
				return nil, err
			}
		}
	case string(composition.Escalate):
		q, ok, err := w.m.Question(w.cfg.Key.Reference(), out.ReceiptID)
		if err != nil {
			return nil, err
		}
		if ok {
			if err := w.j.AddQuestion(mediator.Open{Question: q.ID, Deadline: q.Deadline, Roles: q.Roles}); err != nil {
				return nil, err
			}
			w.asks++
			w.askAt[q.ID] = w.asks
		}
	}
	if err := w.j.AddReceipts([][]byte{out.Receipt}); err != nil {
		return nil, err
	}
	ev.Outcomes = []mediator.Outcome{out}
	if out.Verdict == string(composition.Escalate) {
		if l, ok := w.m.Answerers(out.ReceiptID); ok && len(l) == 0 {
			u, err := w.m.Undeliverable(out.ReceiptID, at)
			if err != nil {
				return nil, err
			}
			if err := w.j.AddReceipts([][]byte{u.Receipt}); err != nil {
				return nil, err
			}
			ev.Outcomes = append(ev.Outcomes, u)
		}
	}
	if len(recs) > 0 {
		lines := make([][]byte, len(recs))
		for i, r := range recs {
			lines[i] = judge.Line(r)
		}
		if err := w.j.AddJudged(out.ReceiptID, lines); err != nil {
			return nil, err
		}
	}
	if adv != nil {
		w.advise(out.ReceiptID, adv)
	}
	return encodeReply(wireReply{Outcome: wire(out)}), nil
}

// advise runs the advise judges of a receipt's part on a goroutine of the window's, each within its
// own budget; their records come back on the results channel. They decide nothing, and the reply
// does not wait on them (docs/adr/0023-judges.md).
func (w *window) advise(receiptID string, adv *advising) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		recs := judge.Advise(w.runs, adv.stack, adv.req, w.cfg.Budget)
		select {
		case w.results <- result{receipt: receiptID, records: recs}:
		case <-w.done:
		}
	}()
}

// advised appends an advise run's records to its receipt's judged file, and emits EventAdvised. A
// failed append is a failed journal write (journal-synced).
func (w *window) advised(res result) error {
	if len(res.records) == 0 {
		return nil
	}
	lines := make([][]byte, len(res.records))
	ev := Event{Kind: EventAdvised, Release: -1}
	for i, r := range res.records {
		lines[i] = judge.Line(r)
		ev.Advice = append(ev.Advice, Advice{Judge: r.Judge, Outcome: r.Outcome, Receipt: res.receipt})
	}
	if err := w.j.AddJudged(res.receipt, lines); err != nil {
		return err
	}
	w.cfg.Event(ev)
	return nil
}

func (w *window) get(c Call, ev *Event) ([]byte, error) {
	d, err := w.m.Get(c.Caller)
	if err != nil {
		return refused(err, ev), nil
	}
	if d == nil {
		// Once the Accord is closed, a fetch that finds nothing queued returns the close (close-3).
		if cl, ok := w.m.Closed(); ok {
			return encodeReply(wireReply{Close: cl.Receipt}), nil
		}
		return encodeReply(wireReply{}), nil
	}
	var head struct {
		ReceiptID string `json:"receiptId"`
	}
	if err := json.Unmarshal(d, &head); err != nil {
		return nil, &journal.Error{Code: journal.CodeJournal, File: journal.DirHeld}
	}
	if err := w.j.TakeHeld(head.ReceiptID); err != nil {
		return nil, err
	}
	if w.queued[c.Caller]--; w.queued[c.Caller] <= 0 {
		delete(w.queued, c.Caller)
	}
	ev.Fetched = head.ReceiptID
	return encodeReply(wireReply{Delivery: d}), nil
}

// questions answers a questions call with the open questions the caller may answer, oldest first,
// and reports whether the reply has something to return: a refusal, the close or a question. The
// core says first whether the caller is a member: no question has the empty id, so the read answers
// with its membership row. The mediator's own reference reads every open question, for the journal;
// it is never handed to the read for a call, and a call it signs is shown none (maybe-1).
//
// A call whose after is a cursor of this start's is shown only the questions asked after the last
// one that cursor's reply carried; any other after is taken for none. Every reply carries a cursor:
// the position of the last question it carries, or, when it carries none, of the newest ask
// journaled, or after's when that is later.
func (w *window) questions(c Call, ev *Event) ([]byte, bool) {
	if _, _, err := w.m.Question(c.Caller, ""); err != nil {
		return refused(err, ev), true
	}
	after, _ := w.cursorAt(c.After)
	list := []wireAsked{}
	newest := max(w.asks, after)
	if c.Caller == w.cfg.Key.Reference() {
		cur := w.cursor(newest)
		return encodeReply(wireReply{Questions: &list, Cursor: &cur}), false
	}
	// A closed Accord has no open question; the reply says why, with the close (close-3), so an
	// answerer's command learns the Accord is closed before it signs anything.
	if cl, ok := w.m.Closed(); ok {
		cur := w.cursor(newest)
		return encodeReply(wireReply{Questions: &list, Close: cl.Receipt, Cursor: &cur}), true
	}
	var at []int
	for _, id := range w.m.Pending() {
		if w.askAt[id] <= after {
			continue
		}
		q, ok, err := w.m.Question(c.Caller, id)
		var rj *mediator.Rejection
		switch {
		case errors.As(err, &rj) && rj.Code == mediator.NotAnAnswerer:
			continue
		case err != nil:
			return refused(err, ev), true
		case !ok:
			continue
		}
		list = append(list, wireAskedOf(q))
		at = append(at, w.askAt[id])
	}
	return w.questionsReply(list, at, newest, ev)
}

// cursor is the cursor of the position n among this start's asks: the start's random value and n.
func (w *window) cursor(n int) string { return w.epoch + "-" + strconv.Itoa(n) }

// cursorAt reads a cursor of this start's: its position, and true. Anything else, a cursor of
// another start's or one no window minted, is position 0, as no after at all.
func (w *window) cursorAt(s string) (int, bool) {
	e, n, ok := strings.Cut(s, "-")
	if !ok || e != w.epoch || n == "" || len(n) > 19 || (len(n) > 1 && n[0] == '0') {
		return 0, false
	}
	at := 0
	for i := 0; i < len(n); i++ {
		if n[i] < '0' || n[i] > '9' {
			return 0, false
		}
		at = at*10 + int(n[i]-'0')
	}
	if at > w.asks {
		return 0, false
	}
	return at, true
}

// wireAskedOf is an open question as the questions reply names it to an answerer (maybe-1): its
// id, asker, kind, deadline and form, the ask's receipt, the part's salt in hex, its size, and its
// body in the form it was given, a raw body as standard base64.
func wireAskedOf(q mediator.Question) wireAsked {
	n := len(q.Part)
	salt := hex.EncodeToString(q.Salt)
	a := wireAsked{ID: &q.ID, Asker: &q.Asker, Kind: &q.Kind, Deadline: &q.Deadline, Form: &q.Form, Size: &n,
		Receipt: q.Receipt, Salt: &salt}
	switch q.Form {
	case "text":
		t := string(q.Part)
		a.Text = &t
	case "raw":
		r := b64.EncodeToString(q.Part)
		a.Raw = &r
	default:
		a.Data = q.Part
	}
	return a
}

// questionsReply encodes the questions reply in at most MaxReply bytes, so a client reads every
// reply the window sends, however many questions are open and however large their parts. Each
// question goes whole, oldest first, while the reply fits; the first that does not fit is left out
// with every one after it, and the reply says more, so the client answers and asks again. Its cursor
// is the position, in at, of the last question kept, or newest when it keeps none; it reports
// whether it keeps one.
func (w *window) questionsReply(list []wireAsked, at []int, newest int, ev *Event) ([]byte, bool) {
	t := true
	empty := []wireAsked{}
	longest := w.cursor(math.MaxInt64)
	used := len(encodeReply(wireReply{Questions: &empty, More: &t, Cursor: &longest}))
	var more *bool
	for i := range list {
		n := encodedLen(list[i])
		if i > 0 {
			n++ // the comma before it
		}
		if used+n > MaxReply {
			list, more = list[:i], &t
			break
		}
		used += n
	}
	cur := w.cursor(newest)
	if len(list) > 0 {
		cur = w.cursor(at[len(list)-1])
	}
	b := encodeReply(wireReply{Questions: &list, More: more, Cursor: &cur})
	if len(b) > MaxReply {
		ev.Door = Internal
		return refusal(byDoor, string(Internal)), true
	}
	return b, len(list) > 0
}

// encodedLen is the length of a question's canonical bytes in a reply.
func encodedLen(a wireAsked) int {
	b, err := json.Marshal(a)
	if err != nil {
		return MaxReply + 1
	}
	c, err := canonical.Form(b)
	if err != nil {
		return MaxReply + 1
	}
	return len(c)
}

// resolve hands the mediator the answerer's signed call, text and salt as the call carried them
// (door-passes-bytes). When it is accepted, the journal takes the call before the receipts of its
// closing, and every delivery it queued, the call inside each (answer-call-attached).
func (w *window) resolve(c Call, at string, ev *Event) ([]byte, error) {
	outs, err := w.m.Resolve(c.Caller, mediator.Resolution{Call: c.AnswerCall, Text: c.Answer, Salt: c.Salt}, at)
	if err != nil {
		return refused(err, ev), nil
	}
	if err := w.j.AddAnswer(c.AnswerCall); err != nil {
		return nil, err
	}
	for _, o := range outs {
		if q, ok := w.m.Queued(o.ReceiptID); ok {
			if err := w.putHeld(q); err != nil {
				return nil, err
			}
		}
	}
	if err := w.j.AddReceipts(receiptsOf(outs)); err != nil {
		return nil, err
	}
	ev.Outcomes = outs
	reply := wireReply{Outcome: wire(outs[0])}
	if len(outs) > 1 {
		reply.Release = wire(outs[1])
		ev.Release = 1
	}
	return encodeReply(reply), nil
}

// result answers a result call with the question's closing, and its release, to its asker, and
// reports whether it has something to return: a refusal or a closing.
func (w *window) result(c Call, ev *Event) ([]byte, bool) {
	outs, ok, err := w.m.Result(c.Caller, c.Question)
	if err != nil {
		return refused(err, ev), true
	}
	if !ok {
		return encodeReply(wireReply{}), false
	}
	reply := wireReply{Outcome: wire(outs[0])}
	if len(outs) > 1 {
		reply.Release = wire(outs[1])
	}
	return encodeReply(reply), true
}
