package local_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
)

// TestTheWindowMintsA32ByteAnswerSalt accepts one question and changes another: the window mints
// each answer's salt, 32 bytes, and the reply carries it. A client refuses a reply whose salt is 16
// or 33 bytes, and reads one of 32 (crossing-3, maybe-8, maybe-9).
func TestTheWindowMintsA32ByteAnswerSalt(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	win := start(t, w.window(t))
	defer win.stop(t)
	for i, action := range []mediator.Action{mediator.Accepted, mediator.Changed} {
		id := []string{"p-1", "p-2"}[i]
		q := w.give(t, "proposer", "applier", id, "proposal", "Rename total.")
		if q.Outcome == nil || q.Outcome.Verdict != "escalate" {
			t.Fatalf("the proposal was not asked: %v", q.Outcome)
		}
		rep := w.call(t, "decider", local.OpResolve, local.ResolveArgs(q.Outcome.ReceiptID, action, "Rename it."))
		if rep.Outcome == nil || len(rep.Salt) != mediator.SaltBytes {
			t.Fatalf("%s: the reply's salt is %d bytes, want %d", action, len(rep.Salt), mediator.SaltBytes)
		}
		for n, ok := range map[int]bool{16: false, 33: false, mediator.SaltBytes: true} {
			b, _ := json.Marshal(map[string]any{"ok": true, "salt": hex.EncodeToString(make([]byte, n)), "outcome": map[string]any{
				"verdict": rep.Outcome.Verdict, "reason": rep.Outcome.Reason, "receiptId": rep.Outcome.ReceiptID,
				"receipt": json.RawMessage(rep.Outcome.Receipt)}})
			c, err := canonical.Form(b)
			if err != nil {
				t.Fatal(err)
			}
			if _, got := local.ParseReply(c); got != ok {
				t.Fatalf("%s: a reply with a salt of %d bytes is read: %v, want %v", action, n, got, ok)
			}
		}
	}
}
