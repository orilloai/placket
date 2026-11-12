package local

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
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
	// Disk is what the journal writes through; nil is the operating system's.
	Disk journal.Disk
	// Now is the door's clock (door-owns-clock): read once per call, and at each idle tick.
	Now func() time.Time
	// Rand is where the door mints an answer's salt.
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

// Run runs a window until ctx is done, then returns nil having written nothing more. It opens the
// journal and takes its lock; founds the Accord at the current second on an empty journal or
// reloads the journal's, journaling the closings of what a stop left open; removes a stale socket
// file; and only then listens. Every call is handled by one goroutine, which owns the mediator. A
// journal write that fails ends Run with that error, and the call it was for gets no reply
// (journal-synced).
func Run(ctx context.Context, cfg Window) error {
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
	l, err := listen(cfg.Socket)
	if err != nil {
		return err
	}
	defer l.Close()
	cfg.Event(Event{Kind: EventStarted, Founded: founded, Accord: w.m.Accord(), Card: w.m.Card()})
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
	m, outs, err := mediator.Reload(w.cfg.Founding, rec.FoundedAt, w.cfg.Key, rec.Receipts, rec.Open, rec.Queued, at)
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

// listen removes a stale socket file at path and listens there, mode 0600. The listener never
// removes the file: a window stopped in any way leaves it, and the next start removes it.
func listen(path string) (*net.UnixListener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&fs.ModeSocket == 0 {
			return nil, &Error{Code: CodeSocket}
		}
		if err := os.Remove(path); err != nil {
			return nil, &Error{Code: CodeSocket}
		}
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, &Error{Code: CodeSocket}
	}
	l.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, &Error{Code: CodeSocket}
	}
	return l, nil
}

// serve accepts connections, each read on a goroutine of its own, and handles every call on this
// one, which owns the mediator.
func (w *window) serve(ctx context.Context, l *net.UnixListener) error {
	reqs := make(chan request)
	done := make(chan struct{})
	defer func() {
		close(done)
		_ = l.Close()
	}()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go read(c, reqs, done)
		}
	}()
	tick := time.NewTicker(w.cfg.Idle)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
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

// read reads one call from c, hands it to the window, and writes the reply, if one comes.
func read(c net.Conn, reqs chan<- request, done <-chan struct{}) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(30 * time.Second))
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

// tick passes the time to at and journals what it closed.
func (w *window) tick(at, kind string) error {
	outs, err := w.m.Tick(at)
	if err != nil || len(outs) == 0 {
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
	ev.Error = mediator.CodeInternal
	return refusal(byError, string(mediator.CodeInternal))
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
	out, err := w.m.Give(c.Caller, c.Message, c.Envelope, at)
	if err != nil {
		return refused(err, ev), nil
	}
	switch out.Verdict {
	case string(composition.Pass):
		if q, ok := w.m.Queued(out.ReceiptID); ok {
			if err := w.j.PutHeld(q.To, q.Delivery, nil); err != nil {
				return nil, err
			}
		}
	case string(composition.Escalate):
		q, ok, _ := w.m.Question(w.cfg.Key.Reference(), out.ReceiptID)
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
			if err == nil {
				if err := w.j.AddReceipts([][]byte{u.Receipt}); err != nil {
					return nil, err
				}
				ev.Outcomes = append(ev.Outcomes, u)
			}
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
		return encodeReply(wireReply{}), nil
	}
	var head struct {
		ReceiptID string `json:"receiptId"`
	}
	if err := json.Unmarshal(d, &head); err != nil {
		return nil, &journal.Error{Code: journal.CodeJournal, File: journal.DirHeld}
	}
	answerCall, err := w.j.TakeHeld(head.ReceiptID)
	if err != nil {
		return nil, err
	}
	ev.Fetched = head.ReceiptID
	return encodeReply(wireReply{Delivery: d, AnswerCall: answerCall}), nil
}

func (w *window) questions(c Call, ev *Event) []byte {
	list := []wireAsked{}
	pending := w.m.Pending()
	if len(pending) == 0 {
		// No question is open: the core still says whether the caller is a member.
		if _, _, err := w.m.Question(c.Caller, ""); err != nil {
			return refused(err, ev)
		}
	}
	for _, id := range pending {
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
		a := wireAsked{ID: &q.ID, Asker: &q.Asker, Kind: &q.Kind, Deadline: &q.Deadline, Form: &q.Form}
		if q.Form == "raw" {
			n := len(q.Part)
			a.Size = &n
		} else {
			s := string(q.Part)
			a.Message = &s
		}
		list = append(list, a)
	}
	return encodeReply(wireReply{Questions: &list})
}

func (w *window) resolve(c Call, at string, ev *Event) ([]byte, error) {
	r := mediator.Resolution{Action: mediator.Action(c.Action), Answer: c.Answer}
	var salt []byte
	if r.Action == mediator.Accepted || r.Action == mediator.Changed {
		salt = make([]byte, mediator.MinSaltBytes)
		if _, err := io.ReadFull(w.cfg.Rand, salt); err != nil {
			ev.Error = mediator.CodeInternal
			return refusal(byError, string(mediator.CodeInternal)), nil
		}
		r.Salt, r.Release = salt, true
	}
	outs, err := w.m.Resolve(c.Caller, c.Question, r, at)
	if err != nil {
		return refused(err, ev), nil
	}
	for _, o := range outs {
		q, ok := w.m.Queued(o.ReceiptID)
		if !ok {
			continue
		}
		// The answer call travels beside a delivery whose body is its answer: the answer or the
		// version, each queued by a resolution receipt, never the released proposal.
		var answerCall []byte
		if s, err := receipts.Decode(o.Receipt); err == nil && s.Predicate.Kind == receipts.KindResolution {
			answerCall = AnswerCall(c)
		}
		if err := w.j.PutHeld(q.To, q.Delivery, answerCall); err != nil {
			return nil, err
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
	if salt != nil {
		s := hex.EncodeToString(salt)
		reply.Salt = &s
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
