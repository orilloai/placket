package mediator_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

// TestDoorRowsOneCheckEach gives messages and envelopes that each break one check of the door's
// rows, signed by the caller so every row before it passes, and checks the exact code and that the
// give changed nothing; the unchanged give passes (section 4, crossing-1, crossing-2, crossing-6,
// crossing-7).
func TestDoorRowsOneCheckEach(t *testing.T) {
	t.Parallel()
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	ext := func(x map[string]any) map[string]any {
		return x["metadata"].(map[string]any)[extension].(map[string]any)
	}
	part := func(x map[string]any) map[string]any { return x["parts"].([]any)[0].(map[string]any) }
	partExt := func(x map[string]any) map[string]any {
		return part(x)["metadata"].(map[string]any)[extension].(map[string]any)
	}
	hash64 := strings.Repeat("ab", 32)
	cases := []struct {
		name   string
		change func(x map[string]any)
		extra  bool // the envelope carries a member it may not
		code   mediator.RejectionCode
	}{
		{"none", nil, false, ""},
		{"an envelope member not listed", nil, true, mediator.EnvelopeSignature},
		{"an empty messageId", func(x map[string]any) { x["messageId"] = "" }, false, mediator.ExtensionMissing},
		{"two extensions entries", func(x map[string]any) { x["extensions"] = []any{extension, extension} }, false, mediator.ExtensionMissing},
		{"another extension too", func(x map[string]any) { x["extensions"] = []any{extension, "https://example.com/x"} }, false, mediator.ExtensionMissing},
		{"another extension URI", func(x map[string]any) { x["extensions"] = []any{"https://placket.dev/crossing/0.2"} }, false, mediator.ExtensionMissing},
		{"no extensions entry", func(x map[string]any) { x["extensions"] = []any{} }, false, mediator.ExtensionMissing},
		{"a re of 63 hex digits", func(x map[string]any) { ext(x)["re"] = "sha256:" + hash64[:63] }, false, mediator.ExtensionMissing},
		{"a re in uppercase hex", func(x map[string]any) { ext(x)["re"] = "sha256:" + strings.ToUpper(hash64) }, false, mediator.ExtensionMissing},
		{"a re without its prefix", func(x map[string]any) { ext(x)["re"] = hash64 }, false, mediator.ExtensionMissing},
		{"a re that is a number", func(x map[string]any) { ext(x)["re"] = 1 }, false, mediator.ExtensionMissing},
		{"a typed hash as re", func(x map[string]any) { ext(x)["re"] = "sha256:" + hash64 }, false, ""},
		{"message metadata with another member", func(x map[string]any) {
			x["metadata"].(map[string]any)["https://example.com/x"] = map[string]any{}
		}, false, mediator.ExtensionMissing},
		{"a part with another member", func(x map[string]any) { part(x)["note"] = "x" }, false, mediator.OneBody},
		{"part metadata with another member", func(x map[string]any) {
			part(x)["metadata"].(map[string]any)["https://example.com/x"] = map[string]any{}
		}, false, mediator.PartMetadata},
		{"a part extension with another member", func(x map[string]any) { partExt(x)["topic"] = "x" }, false, mediator.PartMetadata},
	}
	for _, c := range cases {
		m := found(t)
		msg := message(t, m.Accord(), "m-1", ap.Reference(), "note", "A note.", c.change)
		env := envelope(t, 1, msg, p)
		if c.extra {
			env = envelopeWith(t, 1, msg, p, "note", "x")
		}
		out, err := m.Give(p.Reference(), msg, env, "2026-11-12T09:00:00Z")
		if c.code == "" {
			if err != nil || out.Verdict != "pass" {
				t.Fatalf("%s: got %+v, %v", c.name, out, err)
			}
			continue
		}
		if !rejected(err, c.code) {
			t.Fatalf("%s: got %v, want %s", c.name, err, c.code)
		}
		if d, _ := m.Get(ap.Reference()); len(m.Receipts()) != 0 || d != nil {
			t.Fatalf("%s: a rejected give wrote a receipt or queued a part", c.name)
		}
	}
}

// envelopeWith signs an envelope over message bytes at seq that also carries the member name.
func envelopeWith(t testing.TB, seq int64, msg []byte, k signing.Key, name, value string) []byte {
	t.Helper()
	h, _ := canonical.TypedHash(msg)
	o := map[string]any{"seq": seq, "hash": h, name: value}
	b, _ := json.Marshal(o)
	sig, err := signing.Sign(b, signing.TypeEnvelope, k)
	if err != nil {
		t.Fatal(err)
	}
	o["sig"] = sig
	out, _ := json.Marshal(o)
	if !signing.Verify(out, signing.TypeEnvelope, k.Reference()) {
		t.Fatal("the envelope does not verify")
	}
	return out
}

// TestAChainPerOrderedPair gives from one sender to two receivers and back again: each ordered pair
// has its own chain, so the first receipt to each receiver is seq 1 with no prev, and the next to the
// first receiver is seq 2 after that pair's last (receipt-5).
func TestAChainPerOrderedPair(t *testing.T) {
	t.Parallel()
	m := found(t)
	p, ap, d := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier"), ptesting.Key(t, "decider")
	for i, to := range []signing.Key{ap, d, ap} {
		msg := message(t, m.Accord(), "m-"+string(rune('a'+i)), to.Reference(), "note", "A note.", nil)
		if _, err := m.Give(p.Reference(), msg, envelope(t, int64(i+1), msg, p), "2026-11-12T09:00:00Z"); err != nil {
			t.Fatalf("give %d: %v", i, err)
		}
	}
	st, err := verifyAll(ptesting.Key(t, "mediator").Reference(), m.Receipts())
	if err != nil || len(st) != 3 {
		t.Fatalf("got %d statements, %v", len(st), err)
	}
	for i, want := range []struct {
		to   string
		seq  int64
		prev string
	}{{ap.Reference(), 1, ""}, {d.Reference(), 1, ""}, {ap.Reference(), 2, st[0].ID}} {
		got := st[i].Predicate
		if got.From != p.Reference() || got.To != want.to || got.Seq != want.seq || got.Prev != want.prev {
			t.Fatalf("receipt %d: to %s, seq %d, prev %q; want to %s, seq %d, prev %q", i, got.To, got.Seq, got.Prev, want.to, want.seq, want.prev)
		}
	}
}
