package local_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// closeRun runs a window with Close set and no socket, as `window close` does without --socket, and
// returns what Run returned and the events it emitted.
func closeRun(t *testing.T, w *world) (error, []local.Event) {
	t.Helper()
	cfg := w.window(t)
	cfg.Close, cfg.Socket = true, ""
	var events []local.Event
	cfg.Event = func(e local.Event) { events = append(events, e) }
	return local.Run(context.Background(), cfg), events
}

// TestWindowCloseLocked is fork 1's guard (U23): a close of a journal a running window holds is
// journal-locked and writes nothing; once the window stops, the close writes the closings and the
// close in one write, emits closed, and returns without making a socket.
func TestWindowCloseLocked(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	r := start(t, w.window(t))
	if rep := w.give(t, "proposer", "applier", "p-1", "proposal", "A proposal."); rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
		t.Fatalf("the ask: %+v", rep)
	}
	before := w.receipts(t)
	err, _ := closeRun(t, w)
	var je *journal.Error
	if !errors.As(err, &je) || je.Code != journal.CodeLocked {
		t.Fatalf("a close of a held journal: %v, want journal-locked", err)
	}
	if !bytes.Equal(w.receipts(t), before) {
		t.Fatal("a locked close wrote")
	}
	r.stop(t)
	if err := os.Remove(w.socket); err != nil {
		t.Fatal(err)
	}
	err, events := closeRun(t, w)
	if err != nil || len(events) != 1 || events[0].Kind != local.EventClosed || len(events[0].Outcomes) != 1 ||
		events[0].Outcomes[0].Reason != receipts.ReasonClosed || events[0].Accord != w.accord {
		t.Fatalf("the close: %v, %+v", err, events)
	}
	lines := bytes.Split(bytes.TrimSuffix(w.receipts(t), []byte("\n")), []byte("\n"))
	c, err := receipts.DecodeClose(lines[len(lines)-1])
	if err != nil || c.ID != events[0].Close || !bytes.Equal(lines[len(lines)-1], events[0].CloseReceipt) ||
		!bytes.Equal(lines[len(lines)-2], events[0].Outcomes[0].Receipt) {
		t.Fatalf("the journal's last lines: %v", err)
	}
	if _, err := os.Lstat(w.socket); !os.IsNotExist(err) {
		t.Fatalf("a close without a socket made one: %v", err)
	}
	err, _ = closeRun(t, w)
	var me *mediator.Error
	if !errors.As(err, &me) || me.Code != mediator.CodeClosed {
		t.Fatalf("a second close: %v, want closed", err)
	}
	empty := newWorld(t)
	err, _ = closeRun(t, empty)
	if !errors.As(err, &je) || je.Code != journal.CodeJournal || je.Core != string(mediator.CodeUnfounded) {
		t.Fatalf("the close of an empty journal: %v, want journal unfounded", err)
	}
}

// TestClosedWindowServesGet is U7, U8 and U22's --socket case at the door (close-3): a window
// started on a closed journal writes nothing and names the close at its start; a give is rejected
// closed; a get hands out what was queued, then the close each time; and a close run with a socket
// serves the closed Accord too.
func TestClosedWindowServesGet(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	r := start(t, w.window(t))
	if rep := w.give(t, "proposer", "applier", "n-1", "note", "A note."); rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("the note: %+v", rep)
	}
	r.stop(t)
	cfg := w.window(t)
	cfg.Close = true
	c := start(t, cfg)
	var closeID string
	c.mu.Lock()
	for _, e := range *c.events {
		if e.Kind == local.EventClosed {
			closeID = e.Close
		}
		if e.Kind == local.EventStarted && e.Close == "" {
			t.Errorf("the start of a closed window names no close: %+v", e)
		}
	}
	c.mu.Unlock()
	if closeID == "" {
		t.Fatal("the close emitted no closed event")
	}
	closed := w.receipts(t)
	if rep := w.give(t, "proposer", "applier", "n-2", "note", "After."); rep.Rejection != mediator.AccordClosed {
		t.Fatalf("a give after the close: %+v", rep)
	}
	first := w.call(t, "applier", local.OpGet, nil)
	if first.Delivery == nil || first.Close != nil {
		t.Fatalf("the first get: %+v", first)
	}
	for i := 0; i < 2; i++ {
		rep := w.call(t, "applier", local.OpGet, nil)
		cl, err := receipts.DecodeClose(rep.Close)
		if rep.Delivery != nil || err != nil || cl.ID != closeID {
			t.Fatalf("get %d: %+v, %v", i+2, rep, err)
		}
	}
	q := w.call(t, "decider", local.OpQuestions, nil)
	if !q.IsQuestions || len(q.Questions) != 0 || q.Close == nil {
		t.Fatalf("questions on a closed Accord: %+v", q)
	}
	c.stop(t)
	again := start(t, w.window(t))
	again.mu.Lock()
	started := (*again.events)[0]
	again.mu.Unlock()
	if started.Kind != local.EventStarted || started.Close != closeID || len(*again.events) != 1 {
		t.Fatalf("a restart on a closed journal: %+v", *again.events)
	}
	if !bytes.Equal(w.receipts(t), closed) {
		t.Fatal("the closed window wrote a receipt")
	}
}

// TestTornCloseRefusedAtStart is U14 at the window (close-1): a journal whose close was cut after
// its closed closings is refused at a start with journal, receipts.jsonl, close; a close then writes
// the close receipt alone, at the closings' time.
func TestTornCloseRefusedAtStart(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	r := start(t, w.window(t))
	w.give(t, "proposer", "applier", "p-1", "proposal", "A proposal.")
	r.stop(t)
	if err, _ := closeRun(t, w); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.journal, journal.FileReceipts)
	full := w.receipts(t)
	lines := bytes.Split(bytes.TrimSuffix(full, []byte("\n")), []byte("\n"))
	if err := os.WriteFile(path, append(bytes.Join(lines[:len(lines)-1], []byte("\n")), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := w.window(t)
	err := local.Run(context.Background(), cfg)
	var je *journal.Error
	if !errors.As(err, &je) || je.Code != journal.CodeJournal || je.File != journal.FileReceipts || je.Core != string(mediator.CodeClose) {
		t.Fatalf("a start on a torn close: %v, want journal receipts.jsonl close", err)
	}
	w.clock.Set(w.clock.Now().Add(time.Hour))
	err, events := closeRun(t, w)
	if err != nil || len(events) != 1 || len(events[0].Outcomes) != 0 {
		t.Fatalf("completing the close: %v, %+v", err, events)
	}
	if !bytes.Equal(w.receipts(t), full) {
		t.Fatal("the completed close is not the one the whole write would have made")
	}
}

// FuzzParseReplyClose reads any reply as a client does, seeded with the replies that carry a close:
// a get's, and the questions of a closed Accord (fuzz-parsers). It never panics, and a close it
// reads decodes as one and comes alone or beside an empty list of questions.
func FuzzParseReplyClose(f *testing.F) {
	f.Add([]byte(`{"close":{},"ok":true}`))
	f.Add([]byte(`{"close":{"payload":"e30=","payloadType":"application/vnd.in-toto+json","signatures":[]},"ok":true,"questions":[]}`))
	f.Add([]byte(`{"close":{},"delivery":{"a":1},"ok":true}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, ok := local.ParseReply(b)
		if !ok || r.Close == nil {
			return
		}
		if _, err := receipts.DecodeClose(r.Close); err != nil || r.Delivery != nil || r.Outcome != nil || len(r.Questions) != 0 {
			t.Fatalf("a reply with a close: %+v, %v", r, err)
		}
	})
}
