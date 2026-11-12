package local_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	return replies, w.receipts(t), w
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
	st, err := receipts.Verify(key(t, "mediator").Reference(), splitLines(b))
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
		win = start(t, w.window(t))
		if !strings.Contains(win.stderr.String(), "a delivery whose receipt was never journaled") {
			t.Fatalf("%s: stderr %q", how, win.stderr.String())
		}
		if r := w.give(t, "proposer", "applier", "n-1", "note", "A note."); r.Outcome == nil || r.Outcome.Verdict != "pass" {
			t.Fatalf("%s: the give after the restart: %+v", how, r.Rejection)
		}
		win.stop(t)
	}
}
