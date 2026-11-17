package local_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// A step of a script: a call at its own second, and whether a window may be dropped after it.
type step struct {
	do        func(t *testing.T, w *world) string
	restartOK bool
}

// script is twelve calls of the worked example: an ask accepted and released and both fetched; an
// ask nobody can answer; a note; an ask changed, released, fetched by both and read back by its
// asker. A window may be dropped after every call at which no question is open.
func script(q *[2]string) []step {
	reply := func(r local.Reply) string {
		switch {
		case r.Outcome != nil:
			s := r.Outcome.Verdict + " " + r.Outcome.Reason + " " + r.Outcome.ReceiptID
			if r.Release != nil {
				s += " / " + r.Release.Verdict + " " + r.Release.ReceiptID
			}
			return s
		case r.Delivery != nil:
			return string(r.Delivery) + string(r.AnswerCall)
		}
		return "ok " + string(r.Rejection) + string(r.Error) + string(r.Door)
	}
	return []step{
		{func(t *testing.T, w *world) string {
			r := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.")
			q[0] = r.Outcome.ReceiptID
			return reply(r)
		}, false},
		{func(t *testing.T, w *world) string {
			return reply(w.call(t, "decider", local.OpResolve, local.ResolveArgs(q[0], mediator.Accepted, "accepted")))
		}, true},
		{func(t *testing.T, w *world) string { return reply(w.call(t, "applier", local.OpGet, nil)) }, true},
		{func(t *testing.T, w *world) string { return reply(w.call(t, "proposer", local.OpGet, nil)) }, true},
		{func(t *testing.T, w *world) string {
			return reply(w.give(t, "proposer", "applier", "d-1", "draft", "A draft."))
		}, true},
		{func(t *testing.T, w *world) string {
			return reply(w.give(t, "proposer", "applier", "n-1", "note", "A note."))
		}, true},
		{func(t *testing.T, w *world) string {
			r := w.give(t, "proposer", "applier", "p-2", "proposal", "Drop notes.")
			q[1] = r.Outcome.ReceiptID
			return reply(r)
		}, false},
		{func(t *testing.T, w *world) string {
			return reply(w.call(t, "decider", local.OpResolve, local.ResolveArgs(q[1], mediator.Changed, "Hide notes.")))
		}, true},
		{func(t *testing.T, w *world) string { return reply(w.call(t, "applier", local.OpGet, nil)) }, true},
		{func(t *testing.T, w *world) string { return reply(w.call(t, "applier", local.OpGet, nil)) }, true},
		{func(t *testing.T, w *world) string {
			return reply(w.call(t, "proposer", local.OpResult, map[string]any{"question": q[1]}))
		}, true},
		{func(t *testing.T, w *world) string { return reply(w.call(t, "proposer", local.OpGet, nil)) }, true},
	}
}

// play runs the script against windows driven by one scripted clock and one scripted salt source,
// dropping the window in process after every call that allows it when restart is set, and returns
// the replies and the journal's receipts.
func play(t *testing.T, restart bool) ([]string, []byte, *world) {
	w := newWorld(t)
	rec := &recorder{}
	w.disk = rec
	win := start(t, w.window(t))
	var q [2]string
	var replies []string
	for i, s := range script(&q) {
		w.clock.Set(time.Date(2026, 11, 12, 9, 0, 10*(i+1), 0, time.UTC))
		replies = append(replies, s.do(t, w))
		if restart && s.restartOK {
			win.stop(t)
			if _, err := os.Lstat(w.socket); err != nil {
				t.Fatalf("a dropped window cleaned up its socket: %v", err)
			}
			win = start(t, w.window(t))
		}
	}
	win.stop(t)
	rec.inOrder(t)
	return replies, w.receipts(t), w
}

// A recorder is a Disk that writes through the system's and notes, in order, what each write named:
// the ids of the receipts a receipts.jsonl append holds, the question of a questions.jsonl line,
// and the receipt a held file is named for.
type recorder struct {
	mu     sync.Mutex
	writes []noted
}

type noted struct {
	file string
	ids  []string
}

func (r *recorder) note(path string, b []byte) {
	n := noted{file: filepath.Base(path)}
	switch {
	case n.file == journal.FileReceipts:
		for _, l := range splitLines(b) {
			st, err := receipts.Decode(l)
			if err != nil {
				n.ids = append(n.ids, "not a receipt")
				continue
			}
			n.ids = append(n.ids, st.ID)
		}
	case n.file == journal.FileQuestions:
		for _, l := range splitLines(b) {
			o, _ := journal.ParseQuestion(l)
			n.ids = append(n.ids, o.Question)
		}
	case filepath.Base(filepath.Dir(path)) == journal.DirHeld:
		n.file, n.ids = journal.DirHeld, []string{"sha256:" + filepath.Base(path)}
	}
	r.mu.Lock()
	r.writes = append(r.writes, n)
	r.mu.Unlock()
}

func (r *recorder) Append(path string, b []byte) error {
	err := journal.OS.Append(path, b)
	if err == nil {
		r.note(path, b)
	}
	return err
}

func (r *recorder) Create(path string, b []byte) error {
	err := journal.OS.Create(path, b)
	if err == nil {
		r.note(path, b)
	}
	return err
}

func (r *recorder) Remove(path string) error { return journal.OS.Remove(path) }

// inOrder checks that every held file and every questions.jsonl line was written before the
// receipt it names, in an earlier write, and that each names a receipt written later.
func (r *recorder) inOrder(t *testing.T) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt := map[string]int{}
	for i, n := range r.writes {
		if n.file != journal.FileReceipts {
			continue
		}
		for _, id := range n.ids {
			if _, seen := receipt[id]; !seen {
				receipt[id] = i
			}
		}
	}
	held, asked := 0, 0
	for i, n := range r.writes {
		if n.file != journal.DirHeld && n.file != journal.FileQuestions {
			continue
		}
		for _, id := range n.ids {
			at, ok := receipt[id]
			if !ok || at <= i {
				t.Fatalf("write %d, of %s for %s, is not before its receipt's (%d, %v)", i, n.file, id, at, ok)
			}
			if n.file == journal.DirHeld {
				held++
			} else {
				asked++
			}
		}
	}
	if held == 0 || asked == 0 {
		t.Fatalf("%d held files and %d questions lines written", held, asked)
	}
}

// TestARestartChangesNoReceipt runs one script of calls against a window never stopped and against
// windows dropped in process, files closed and nothing else cleaned up, and started again between
// every two calls at which no question is open: the replies and the journals' receipts are the same
// (restart-reproducible). The journal of the second reloaded after an answered question, after an
// ask nobody could answer and after every fetch.
func TestARestartChangesNoReceipt(t *testing.T) {
	t.Parallel()
	once, a, _ := play(t, false)
	again, b, w := play(t, true)
	if strings.Join(once, "\n") != strings.Join(again, "\n") {
		t.Fatalf("the replies differ:\n%s\n--\n%s", strings.Join(once, "\n"), strings.Join(again, "\n"))
	}
	if !bytes.Equal(a, b) || len(a) == 0 {
		t.Fatalf("the receipts differ:\n%s\n--\n%s", a, b)
	}
	st, err := verifyAll(key(t, "mediator").Reference(), splitLines(b))
	if err != nil || len(st) != 9 {
		t.Fatalf("%d receipts, %v", len(st), err)
	}
	if st[4].Predicate.Reason != receipts.ReasonUndeliverable || st[3].Predicate.Verdict != string(composition.Escalate) {
		t.Fatalf("the draft: %+v, %+v", st[3].Predicate, st[4].Predicate)
	}
	held, _ := os.ReadDir(filepath.Join(w.journal, journal.DirHeld))
	if len(held) != 0 {
		t.Fatalf("%d held files after every fetch", len(held))
	}
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	for _, l := range bytes.Split(bytes.TrimSuffix(b, []byte{'\n'}), []byte{'\n'}) {
		if len(l) > 0 {
			out = append(out, l)
		}
	}
	return out
}

// failing is a Disk that fails every append to receipts.jsonl, and does what the system does for
// the rest.
type failing struct{}

func (failing) Append(path string, b []byte) error {
	if filepath.Base(path) == journal.FileReceipts {
		return errors.New("injected")
	}
	return journal.OS.Append(path, b)
}
func (failing) Create(path string, b []byte) error { return journal.OS.Create(path, b) }
func (failing) Remove(path string) error           { return journal.OS.Remove(path) }

// TestAJournalWriteThatFails is LOG-15: a journal write fails, through an injected writer and in a
// read-only journal. The call gets no reply, the window returns the failure, and the next start
// reloads what the disk holds: the held file whose receipt never reached the journal is removed, and
// the next give takes the same seq.
func TestAJournalWriteThatFails(t *testing.T) {
	t.Parallel()
	for _, how := range []string{"injected", "read-only"} {
		w := newWorld(t)
		win := start(t, w.window(t))
		win.stop(t)
		cfg := w.window(t)
		if how == "injected" {
			cfg.Disk = failing{}
		} else {
			// A read-only journal: its directory, and its receipts file when a start made one.
			if err := os.Chmod(w.journal, 0o500); err != nil {
				t.Fatal(err)
			}
			_ = os.Chmod(filepath.Join(w.journal, journal.FileReceipts), 0o400)
		}
		win = start(t, cfg)
		msg := w.message(t, "n-1", key(t, "applier").Reference(), "note", "text", "A note.", "")
		h := hashOf(msg)
		_, err := w.client(t, "proposer").Call(local.OpGive, local.GiveArgs(msg, envelope(t, 1, h, key(t, "proposer"))))
		var le *local.Error
		if !errors.As(err, &le) || le.Code != local.CodeSocket {
			t.Fatalf("%s: the call got %v, not no reply", how, err)
		}
		select {
		case err := <-win.done:
			var je *journal.Error
			if !errors.As(err, &je) || je.File != journal.FileReceipts {
				t.Fatalf("%s: the window ended with %v", how, err)
			}
			win.cancel = nil
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: the window did not end", how)
		}
		_ = os.Chmod(w.journal, 0o700)
		_ = os.Chmod(filepath.Join(w.journal, journal.FileReceipts), 0o600)
		held, err := os.ReadDir(filepath.Join(w.journal, journal.DirHeld))
		if err != nil || len(held) != 1 {
			t.Fatalf("%s: held/ holds %v, %v", how, held, err)
		}
		win = start(t, w.window(t))
		want := "journal: held/" + held[0].Name() + " holds a delivery whose receipt was never journaled; it never crossed, and is removed\n"
		if win.stderr.String() != want {
			t.Fatalf("%s: stderr %q", how, win.stderr.String())
		}
		if r := w.give(t, "proposer", "applier", "n-1", "note", "A note."); r.Outcome == nil || r.Outcome.Verdict != "pass" {
			t.Fatalf("%s: the give after the restart: %+v", how, r.Rejection)
		}
		win.stop(t)
	}

	// A journal write that fails on the idle tick, with no call to answer: the window ends with it
	// too, and the next start closes the question the tick could not.
	w := newWorld(t)
	arm := &armed{}
	w.disk = arm
	win := start(t, w.window(t))
	if r := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total."); r.Outcome == nil || r.Outcome.Verdict != string(composition.Escalate) {
		t.Fatal("the ask was not asked")
	}
	arm.on.Store(true)
	w.clock.Set(w.clock.Now().Add(3601 * time.Second))
	select {
	case err := <-win.done:
		var je *journal.Error
		if !errors.As(err, &je) || je.File != journal.FileReceipts {
			t.Fatalf("on the idle tick: the window ended with %v", err)
		}
		win.cancel = nil
	case <-time.After(10 * time.Second):
		t.Fatal("on the idle tick: the window did not end")
	}
	w.disk = nil
	win = start(t, w.window(t))
	win.stop(t)
	st, err := verifyAll(key(t, "mediator").Reference(), splitLines(w.receipts(t)))
	if err != nil || len(st) != 2 || st[1].Predicate.Reason != receipts.ReasonTimeout {
		t.Fatalf("after the idle tick failed: %d receipts, %v", len(st), err)
	}
}

// armed is a Disk that fails every append to receipts.jsonl once it is on.
type armed struct{ on atomic.Bool }

func (a *armed) Append(path string, b []byte) error {
	if a.on.Load() && filepath.Base(path) == journal.FileReceipts {
		return errors.New("injected")
	}
	return journal.OS.Append(path, b)
}
func (a *armed) Create(path string, b []byte) error { return journal.OS.Create(path, b) }
func (a *armed) Remove(path string) error           { return journal.OS.Remove(path) }

// verifyAll verifies list under mediator and returns the statements, for tests that read them.
func verifyAll(mediator string, list [][]byte) ([]receipts.Statement, error) {
	verified, err := receipts.Verify(mediator, list)
	if err != nil {
		return nil, err
	}
	out := make([]receipts.Statement, len(verified))
	for i, v := range verified {
		out[i] = v.Statement()
	}
	return out, nil
}
