package local_test

import (
	"encoding/json"
	"testing"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// TestTheAnswererMintsTheAnswerSalt accepts one question and changes another, each under a salt of
// 32 bytes the answerer minted: the window's resolve uses that salt unchanged, so the asker's
// delivery carries it and the resolution's digest is the call's, and the reply carries none. An
// accept sent with no salt is answer-body, and nothing is written (crossing-3, maybe-8, maybe-9, U7).
func TestTheAnswererMintsTheAnswerSalt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	win := start(t, w.window(t))
	defer win.stop(t)
	for i, action := range []answer.Action{answer.Accepted, answer.Changed} {
		id := []string{"p-1", "p-2"}[i]
		q := w.give(t, "proposer", "applier", id, "proposal", "Rename total.")
		if q.Outcome == nil || q.Outcome.Verdict != "escalate" {
			t.Fatalf("the proposal was not asked: %v", q.Outcome)
		}
		args := w.answerArgs(t, "decider", q.Outcome.ReceiptID, action, "Rename it.")
		rep := w.call(t, "decider", local.OpResolve, args)
		if rep.Outcome == nil || rep.Salt != nil {
			t.Fatalf("%s: outcome %v, salt %x", action, rep.Outcome, rep.Salt)
		}
		st, err := receipts.Decode(rep.Outcome.Receipt)
		salt := saltOf("the answer's salt of " + q.Outcome.ReceiptID)
		if err != nil || st.Subject.Digest != receipts.Digest(salt, []byte("Rename it.")) || len(salt) != mediator.SaltBytes {
			t.Fatalf("%s: the resolution's digest is not the answerer's", action)
		}
		d := w.call(t, "proposer", local.OpGet, nil)
		var got struct{ Salt string }
		if err := json.Unmarshal(d.Delivery, &got); err != nil || got.Salt != args["salt"] {
			t.Fatalf("%s: the asker's delivery carries the salt %q, want %q", action, got.Salt, args["salt"])
		}
	}
	q := w.give(t, "proposer", "applier", "p-3", "proposal", "Rename total.")
	args := w.answerArgs(t, "decider", q.Outcome.ReceiptID, answer.Accepted, "accepted")
	delete(args, "salt")
	before := len(w.receipts(t))
	if rep := w.call(t, "decider", local.OpResolve, args); rep.Rejection != mediator.AnswerBody {
		t.Fatalf("an accept with no salt: rejection %q, door %q", rep.Rejection, rep.Door)
	}
	if len(w.receipts(t)) != before {
		t.Fatal("an accept with no salt wrote a receipt")
	}
}
