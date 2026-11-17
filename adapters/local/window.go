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
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
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
}

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
)

// An Event is one thing the window did. It holds ids, codes and receipts, never a body, a salt or a
// question's part.
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
}

// window is a running window's state, owned by its one goroutine (one-mediator-goroutine).
type window struct {
	cfg    Window
	m      *mediator.Mediator
	j      *journal.Journal
	nonces map[string]time.Time
}

type request struct {
	line     []byte
	tooLarge bool
	reply    chan []byte
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
	w := &window{cfg: cfg, j: j, nonces: map[string]time.Time{}}
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
	started := Event{Kind: EventStarted, Founded: founded, Accord: w.m.Accord(), Card: w.m.Card()}
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
	if m.Card() != rec.Card {
		return false, &journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1, Core: "card"}
	}
	w.m = m
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

// close closes the journal's Accord at the time at (close-1): the closings and the close receipt in
// one journal write, then EventClosed. A closed journal is mediator's *Error closed, returned as it
// is; any other refusal is named as reloadError names it.
func (w *window) close(rec *journal.Record, at string) error {
	m, outs, closed, err := mediator.Close(w.cfg.Founding, rec.Receipts, rec.Open, rec.Queued, rec.FoundedAt, w.cfg.Key, at)
	if err != nil {
		var me *mediator.Error
		if errors.As(err, &me) && me.Code == mediator.CodeClosed {
			return err
		}
		return reloadError(err, rec)
	}
	if m.Card() != rec.Card {
		return &journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1, Core: "card"}
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
	defer func() {
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
			_ = c.SetDeadline(time.Now().Add(readTimeout))
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
				read(c, reqs, done)
				mu.Lock()
				delete(conns, c)
				mu.Unlock()
			}()
		}
	}()
	tick := time.NewTicker(w.cfg.Idle)
	defer tick.Stop()
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
		case r := <-reqs:
			reply, err := w.handle(r)
			if err != nil {
				return err
			}
			r.reply <- reply
		}
	}
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

// readTimeout is how long a connection may take to send its call and read its reply.
const readTimeout = 30 * time.Second

// read reads one call from c, hands it to the window, and writes the reply, if one comes. Its
// deadline is set before it starts.
func read(c net.Conn, reqs chan<- request, done <-chan struct{}) {
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
	r := request{line: line, tooLarge: tooLarge, reply: make(chan []byte, 1)}
	select {
	case reqs <- r:
	case <-done:
		return
	}
	select {
	case b := <-r.reply:
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

	if err := w.tick(at, EventTick); err != nil {
		return nil, err
	}
	ev := Event{Kind: EventCall, Op: c.Op, Caller: c.Caller, Release: -1}
	reply, err := w.op(c, at, &ev)
	if err != nil {
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

func (w *window) op(c Call, at string, ev *Event) ([]byte, error) {
	switch c.Op {
	case OpGive:
		return w.give(c, at, ev)
	case OpGet:
		return w.get(c, ev)
	case OpQuestions:
		return w.questions(c, ev), nil
	case OpResolve:
		return w.resolve(c, at, ev)
	default:
		return w.result(c, ev), nil
	}
}

func (w *window) give(c Call, at string, ev *Event) ([]byte, error) {
	// No judge runs at this door: every give is decided with no judgment (composition-7).
	out, err := w.m.Give(c.Caller, c.Message, c.Envelope, nil, at)
	if err != nil {
		return refused(err, ev), nil
	}
	switch out.Verdict {
	case string(composition.Pass):
		if q, ok := w.m.Queued(out.ReceiptID); ok {
			if err := w.j.PutHeld(q.To, q.Delivery); err != nil {
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
	return encodeReply(wireReply{Outcome: wire(out)}), nil
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
	ev.Fetched = head.ReceiptID
	return encodeReply(wireReply{Delivery: d}), nil
}

// questions answers a questions call with the open questions the caller may answer. The core says
// first whether the caller is a member: no question has the empty id, so the read answers with its
// membership row. The mediator's own reference reads every open question, for the journal; it is
// never handed to the read for a call, and a call it signs is shown none (maybe-1).
func (w *window) questions(c Call, ev *Event) []byte {
	if _, _, err := w.m.Question(c.Caller, ""); err != nil {
		return refused(err, ev)
	}
	list := []wireAsked{}
	if c.Caller == w.cfg.Key.Reference() {
		return encodeReply(wireReply{Questions: &list})
	}
	// A closed Accord has no open question; the reply says why, with the close (close-3), so an
	// answerer's command learns the Accord is closed before it signs anything.
	if cl, ok := w.m.Closed(); ok {
		return encodeReply(wireReply{Questions: &list, Close: cl.Receipt})
	}
	for _, id := range w.m.Pending() {
		q, ok, err := w.m.Question(c.Caller, id)
		var rj *mediator.Rejection
		switch {
		case errors.As(err, &rj) && rj.Code == mediator.NotAnAnswerer:
			continue
		case err != nil:
			return refused(err, ev)
		case !ok:
			continue
		}
		list = append(list, wireAskedOf(q))
	}
	return questionsReply(list, ev)
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
// with every one after it, and the reply says more, so the client answers and asks again.
func questionsReply(list []wireAsked, ev *Event) []byte {
	t := true
	empty := []wireAsked{}
	used := len(encodeReply(wireReply{Questions: &empty, More: &t}))
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
	b := encodeReply(wireReply{Questions: &list, More: more})
	if len(b) > MaxReply {
		ev.Door = Internal
		return refusal(byDoor, string(Internal))
	}
	return b
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
			if err := w.j.PutHeld(q.To, q.Delivery); err != nil {
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

func (w *window) result(c Call, ev *Event) []byte {
	outs, ok, err := w.m.Result(c.Caller, c.Question)
	if err != nil {
		return refused(err, ev)
	}
	if !ok {
		return encodeReply(wireReply{})
	}
	reply := wireReply{Outcome: wire(outs[0])}
	if len(outs) > 1 {
		reply.Release = wire(outs[1])
	}
	return encodeReply(reply)
}
