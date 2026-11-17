package mediator_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/orilloai/placket/core/mediator"
)

// The key is not varied here: a founding set names its mediator, so Found refuses any other key, and
// receipts' TestBuildDependsOnEachInput shows that the key alters a receipt.

// TestGiveDependsOnEachInput gives one part from the proposer to the applier, then the same part at
// another time and under another salt: each writes another receipt, and the same inputs write the
// same bytes (inputs-as-parameters).
func TestGiveDependsOnEachInput(t *testing.T) {
	t.Parallel()
	p, ap := key(t, "proposer"), key(t, "applier")
	give := func(at, salt string) []byte {
		t.Helper()
		m := found(t)
		msg := message(t, m.Accord(), "m-1", ap.Reference(), "note", "A note.", func(v map[string]any) {
			part := v["parts"].([]any)[0].(map[string]any)
			part["metadata"].(map[string]any)[extension].(map[string]any)["salt"] = salt
		})
		out, err := m.Give(p.Reference(), msg, envelope(t, 1, msg, p), at)
		if err != nil || out.Verdict != "pass" {
			t.Fatalf("the give: %+v, %v", out, err)
		}
		return out.Receipt
	}
	salt := hex.EncodeToString([]byte("a salt of sixteen"))
	base := give("2026-11-12T09:00:00Z", salt)
	if !bytes.Equal(base, give("2026-11-12T09:00:00Z", salt)) {
		t.Fatal("the same inputs wrote two receipts")
	}
	for name, got := range map[string][]byte{
		"time": give("2026-11-12T09:00:01Z", salt),
		"salt": give("2026-11-12T09:00:00Z", hex.EncodeToString([]byte("another salt, too"))),
	} {
		if bytes.Equal(got, base) {
			t.Fatalf("a changed %s left the receipt as it was", name)
		}
	}
}

// TestResolveDependsOnEachInput accepts one question with release, then the same answer at another
// time and under another salt: each writes other receipts, and the same inputs write the same bytes
// (inputs-as-parameters).
func TestResolveDependsOnEachInput(t *testing.T) {
	t.Parallel()
	resolve := func(at string, salt []byte) [][]byte {
		t.Helper()
		m := found(t)
		q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
		outs, err := m.Resolve(key(t, "decider").Reference(), q,
			mediator.Resolution{Action: mediator.Accepted, Answer: "accepted", Salt: salt, Release: true}, at)
		if err != nil || len(outs) != 2 {
			t.Fatalf("the answer: %d outcomes, %v", len(outs), err)
		}
		return [][]byte{outs[0].Receipt, outs[1].Receipt}
	}
	base := resolve("2026-11-12T09:01:00Z", salt16)
	if again := resolve("2026-11-12T09:01:00Z", salt16); !bytes.Equal(base[0], again[0]) || !bytes.Equal(base[1], again[1]) {
		t.Fatal("the same inputs wrote other receipts")
	}
	for name, got := range map[string][][]byte{
		"time": resolve("2026-11-12T09:01:01Z", salt16),
		"salt": resolve("2026-11-12T09:01:00Z", []byte("another answer's salt")),
	} {
		if bytes.Equal(got[0], base[0]) {
			t.Fatalf("a changed %s left the resolution receipt as it was", name)
		}
	}
}
