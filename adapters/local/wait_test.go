package local_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
)

// timed is one call's reply, its error and when it came.
type timed struct {
	rep local.Reply
	err error
	at  time.Time
}

// callLater sends a call on a goroutine of the test's and returns the channel its reply comes on.
func (w *world) callLater(t *testing.T, who, op string, args map[string]any) <-chan timed {
	t.Helper()
	c := w.client(t, who)
	c.Timeout = 0
	out := make(chan timed, 1)
	go func() {
		rep, err := c.Call(op, args)
		out <- timed{rep, err, time.Now()}
	}()
	return out
}

// within waits at most d for a reply, and fails the test without one.
func within(t *testing.T, ch <-chan timed, d time.Duration) timed {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("the call failed: %v", r.err)
		}
		return r
	case <-time.After(d):
		t.Fatalf("no reply within %v", d)
	}
	return timed{}
}

// silent fails the test if a reply comes within d.
func silent(t *testing.T, ch <-chan timed, d time.Duration) {
	t.Helper()
	select {
	case r := <-ch:
		t.Fatalf("a reply came while nothing was to return: %+v, %v", r.rep, r.err)
	case <-time.After(d):
	}
}

// ids is the ids of the questions a reply carries, in order.
func ids(rep local.Reply) []string {
	var out []string
	for _, q := range rep.Questions {
		out = append(out, q.ID)
	}
	return out
}

// TestQuestionsWaitForAnAsk parks a decider's questions with wait 10 and no question open: a give
// asked 2 s later reaches it within a second of the ask, and that reply carries the question and a
// cursor. Another, with wait 3 and no ask, is answered empty once its wait ends, with a cursor too.
func TestQuestionsWaitForAnAsk(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	began := time.Now()
	parked := w.callLater(t, "decider", local.OpQuestions, map[string]any{"wait": 10})
	empty := w.callLater(t, "applier", local.OpQuestions, map[string]any{"wait": 3})
	time.Sleep(2 * time.Second)
	q := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.")
	asked := time.Now()
	r := within(t, parked, 2*time.Second)
	if d := r.at.Sub(asked); d > time.Second {
		t.Fatalf("the reply came %v after the ask", d)
	}
	if got := ids(r.rep); len(got) != 1 || got[0] != q.Outcome.ReceiptID || r.rep.Cursor == "" || r.rep.More {
		t.Fatalf("the reply carries %v, cursor %q", got, r.rep.Cursor)
	}
	e := within(t, empty, 5*time.Second)
	if d := e.at.Sub(began); d < 3*time.Second || d > 4500*time.Millisecond {
		t.Fatalf("the empty reply came after %v", d)
	}
	if !e.rep.IsQuestions || len(e.rep.Questions) != 0 || e.rep.Cursor == "" {
		t.Fatalf("the empty reply: %d questions, cursor %q", len(e.rep.Questions), e.rep.Cursor)
	}
}

// TestQuestionsAfterACursor shows a decider one open question, then parks a questions after that
// reply's cursor: it waits while no other question opens, and returns only the second once it does.
// Passed back, the cursor of an empty reply waits too, and does not return the older question still
// open. A cursor no window minted is taken for none.
func TestQuestionsAfterACursor(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	q1 := w.give(t, "proposer", "applier", "p-1", "proposal", "One.").Outcome.ReceiptID
	first := w.call(t, "decider", local.OpQuestions, map[string]any{})
	if got := ids(first); len(got) != 1 || got[0] != q1 {
		t.Fatalf("the first reply carries %v", got)
	}
	parked := w.callLater(t, "decider", local.OpQuestions, map[string]any{"after": first.Cursor, "wait": 10})
	silent(t, parked, 500*time.Millisecond)
	q2 := w.give(t, "proposer", "applier", "p-2", "proposal", "Two.").Outcome.ReceiptID
	second := within(t, parked, 2*time.Second).rep
	if got := ids(second); len(got) != 1 || got[0] != q2 {
		t.Fatalf("after the first cursor: %v, want only %s", got, q2)
	}
	// An empty reply's cursor, passed back, waits again: both questions are still open, and older.
	none := w.call(t, "decider", local.OpQuestions, map[string]any{"after": second.Cursor, "wait": 1})
	if len(none.Questions) != 0 || none.Cursor == "" {
		t.Fatalf("after the second cursor: %v", ids(none))
	}
	again := w.callLater(t, "decider", local.OpQuestions, map[string]any{"after": none.Cursor, "wait": 2})
	if r := within(t, again, 4*time.Second).rep; len(r.Questions) != 0 {
		t.Fatalf("after an empty reply's cursor: %v", ids(r))
	}
	for _, after := range []string{"nothing a window minted", strings.Repeat("0", 32) + "-1", first.Cursor + "0"} {
		if got := ids(w.call(t, "decider", local.OpQuestions, map[string]any{"after": after})); len(got) != 2 || got[0] != q1 || got[1] != q2 {
			t.Fatalf("after %q: %v", after, got)
		}
	}
}

// TestACursorFromBeforeARestart restarts the window, which closes what was open, and asks again:
// questions after a cursor of the first start's returns the question the second start asked, at the
// same position among its asks as the first start's.
func TestACursorFromBeforeARestart(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	r := start(t, w.window(t))
	w.give(t, "proposer", "applier", "p-1", "proposal", "One.")
	old := w.call(t, "decider", local.OpQuestions, map[string]any{}).Cursor
	r.stop(t)
	start(t, w.window(t))
	q := w.give(t, "proposer", "applier", "p-2", "proposal", "Two.").Outcome.ReceiptID
	if got := ids(w.call(t, "decider", local.OpQuestions, map[string]any{"after": old})); len(got) != 1 || got[0] != q {
		t.Fatalf("after a cursor from before the restart: %v, want %s", got, q)
	}
}

// TestWaitingGetsDoNotHoldTheLoop parks twenty gets with wait 60: another member's give and answer
// each take their usual time, and once the answer queues a delivery for the waiting member, every
// parked get replies queued, and none takes it; an ordinary get then does.
func TestWaitingGetsDoNotHoldTheLoop(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	var parked []<-chan timed
	for range 20 {
		parked = append(parked, w.callLater(t, "applier", local.OpGet, map[string]any{"wait": 60}))
	}
	time.Sleep(200 * time.Millisecond)
	began := time.Now()
	q := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.")
	rep := w.call(t, "decider", local.OpResolve, w.answerArgs(t, "decider", q.Outcome.ReceiptID, answer.Accepted, "Yes."))
	if d := time.Since(began); rep.Release == nil || d > time.Second {
		t.Fatalf("the give and the answer took %v; release %v", d, rep.Release)
	}
	for i, ch := range parked {
		if r := within(t, ch, 2*time.Second).rep; !r.Queued || r.Delivery != nil {
			t.Fatalf("parked get %d: %+v", i, r)
		}
	}
	if d := w.call(t, "applier", local.OpGet, nil); d.Delivery == nil {
		t.Fatalf("the ordinary get: %+v", d)
	}
	if d := w.call(t, "applier", local.OpGet, map[string]any{"wait": 1}); d.Queued || d.Delivery != nil {
		t.Fatalf("a get that waits, with nothing left: %+v", d)
	}
}

// TestAWaitPastTheBaseDeadline runs a window whose base deadline is 2 s: a get with wait 4 and
// nothing queued replies empty after 4 s, through a client whose timeout is its default, and a
// connection that sends nothing is closed at the base deadline.
func TestAWaitPastTheBaseDeadline(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	cfg := w.window(t)
	cfg.Deadline = 2 * time.Second
	start(t, cfg)
	c, err := net.Dial("unix", w.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	closed := make(chan time.Duration, 1)
	idle := time.Now()
	go func() {
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		if _, err := bufio.NewReader(c).ReadByte(); err == nil {
			closed <- -1
			return
		}
		closed <- time.Since(idle)
	}()
	began := time.Now()
	r := within(t, w.callLater(t, "applier", local.OpGet, map[string]any{"wait": 4}), 8*time.Second)
	if d := r.at.Sub(began); d < 4*time.Second || d > 5500*time.Millisecond || r.rep.Queued || r.rep.Delivery != nil || r.rep.Door != "" {
		t.Fatalf("after %v: %+v", d, r.rep)
	}
	if d := <-closed; d < 1500*time.Millisecond || d > 4*time.Second {
		t.Fatalf("an idle connection was closed after %v", d)
	}
}

// TestAParkedGetWhoseClientHungUp parks a get with wait 30 on a connection that then closes: the
// delivery queued after stays queued, and the next get receives it.
func TestAParkedGetWhoseClientHungUp(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	line, err := local.SignCall(key(t, "applier"), local.OpGet, w.accord, w.clock.Now(), []byte("0123456789abcdef"), map[string]any{"wait": 30})
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("unix", w.socket)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_ = c.Close()
	if rep := w.give(t, "proposer", "applier", "n-1", "note", "A note."); rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("the note: %+v", rep)
	}
	d := w.call(t, "applier", local.OpGet, nil)
	if d.Delivery == nil {
		t.Fatalf("the next get: %+v", d)
	}
}

// TestAWaitIsWholeSecondsUpToSixty refuses as bad-call a wait above MaxWait, below zero, or not a
// number (the canonical form already refuses a number that is not whole), on each op that takes one, and a wait on an op that takes none.
func TestAWaitIsWholeSecondsUpToSixty(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	for _, op := range []string{local.OpGet, local.OpQuestions, local.OpResult} {
		for _, v := range []any{61, -1, "1", true} {
			args := map[string]any{"wait": v}
			if op == local.OpResult {
				args["question"] = "sha256:" + strings.Repeat("0", 64)
			}
			if rep := w.call(t, "applier", op, args); rep.Door != local.BadCall {
				t.Fatalf("%s with wait %v: %+v", op, v, rep)
			}
		}
	}
	msg := w.message(t, "n-1", key(t, "applier").Reference(), "note", "text", "A note.", "")
	args := local.GiveArgs(msg, envelope(t, 1, hashOf(msg), key(t, "proposer")))
	args["wait"] = 1
	if rep := w.call(t, "proposer", local.OpGive, args); rep.Door != local.BadCall {
		t.Fatalf("a give with a wait: %+v", rep)
	}
	if rep := w.call(t, "applier", local.OpGet, map[string]any{"wait": 60, "after": "x"}); rep.Door != local.BadCall {
		t.Fatalf("a get with after: %+v", rep)
	}
}

// TestAResultWaitsForTheClosing parks the asker's result with wait 10: the answer reaches it at once,
// with the closing and the release; another member's result on the question waits its whole wait and
// is empty.
func TestAResultWaitsForTheClosing(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	q := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.").Outcome.ReceiptID
	mine := w.callLater(t, "proposer", local.OpResult, map[string]any{"question": q, "wait": 10})
	other := w.callLater(t, "applier", local.OpResult, map[string]any{"question": q, "wait": 2})
	silent(t, mine, 300*time.Millisecond)
	w.call(t, "decider", local.OpResolve, w.answerArgs(t, "decider", q, answer.Accepted, "Yes."))
	if r := within(t, mine, 2*time.Second).rep; r.Outcome == nil || r.Release == nil {
		t.Fatalf("the asker's result: %+v", r)
	}
	if r := within(t, other, 4*time.Second).rep; r.Outcome != nil {
		t.Fatalf("another member's result: %+v", r)
	}
}

// TestCallContextEndsAParkedCall parks a get and cancels its context: the call ends at once, error
// socket, and the window goes on.
func TestCallContextEndsAParkedCall(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var err error
	var ended time.Time
	wg.Add(1)
	go func() {
		defer wg.Done()
		c := w.client(t, "applier")
		c.Timeout = 0
		_, err = c.CallContext(ctx, local.OpGet, map[string]any{"wait": 30})
		ended = time.Now()
	}()
	time.Sleep(300 * time.Millisecond)
	cancelled := time.Now()
	cancel()
	wg.Wait()
	var le *local.Error
	if !errors.As(err, &le) || le.Code != local.CodeSocket || ended.Sub(cancelled) > time.Second {
		t.Fatalf("a cancelled call: %v, after %v", err, ended.Sub(cancelled))
	}
	if rep := w.call(t, "applier", local.OpGet, nil); rep.Delivery != nil || rep.Door != "" {
		t.Fatalf("the window after: %+v", rep)
	}
}
