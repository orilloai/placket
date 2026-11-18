package conform

import (
	"strings"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

// TestConformanceWithTheOpeningsItHolds holds verify-1 and verify-2 over a sealed set
// (setup-contract-16): with every opening the worked journal conforms with its terms checked, and a
// pass the terms block fails decision; with none, the same journal conforms with its terms not
// checked and the forged pass is not seen, though its party is still checked; with some, the
// decision row runs only for a receipt whose sides' seals are open.
func TestConformanceWithTheOpeningsItHolds(t *testing.T) {
	t.Parallel()
	w := journal(t)
	every := w.evidence()
	if r, err := Check(every); err != nil || !r.Terms || !r.Module {
		t.Fatalf("with every opening: %+v, %v", r, err)
	}
	none := w.evidence()
	none.Openings = setup.Openings{}
	r, err := Check(none)
	if err != nil || r.Terms || !r.Module || len(r.Decisions) != len(w.list) {
		t.Fatalf("with no opening: %+v, %v", r, err)
	}
	st, err := receipts.Decode(w.list[0])
	if err != nil {
		t.Fatal(err)
	}
	p, ap, d := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference(), ptesting.Key(t, "decider").Reference()
	forged := func(from, to string) [][]byte {
		s := newSigner(t)
		s.add(t, receipts.Subject{Name: "tests", Digest: strings.Repeat("cd", 32)}, receipts.Predicate{Accord: w.accord,
			Card: st.Predicate.Card, Kind: receipts.KindCrossing, From: from, To: to, At: "2026-11-12T09:00:00Z",
			Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: st.Predicate.Envelope.Hash},
			Verdict: "pass", Reason: "ok", By: receipts.By{Decider: receipts.ByTerms}})
		return s.list
	}
	every.Receipts, none.Receipts = forged(p, ap), forged(p, ap)
	if _, err := Check(every); !isCode(err, CodeDecision, 0) {
		t.Fatalf("a forged pass with every opening: %v", err)
	}
	if r, err := Check(none); err != nil || r.Terms {
		t.Fatalf("a forged pass with no opening: %+v, %v", r, err)
	}
	none.Receipts = forged(ptesting.Key(t, "stranger").Reference(), ap)
	if _, err := Check(none); !isCode(err, CodeParty, 0) {
		t.Fatalf("a stranger with no opening: %v", err)
	}
	some := w.evidence()
	some.Openings = setup.Openings{ByActor: map[string][]byte{}, Some: true}
	for _, m := range []string{p, ap} {
		some.Openings.ByActor[m] = w.set.Openings.ByActor[m]
	}
	some.Receipts = forged(p, ap)
	if r, err := Check(some); !isCode(err, CodeDecision, 0) || r.Terms {
		t.Fatalf("a forged pass between two opened members: %+v, %v", r, err)
	}
	some.Receipts = forged(d, ap)
	if r, err := Check(some); err != nil || r.Terms {
		t.Fatalf("a forged pass from a member not opened: %+v, %v", r, err)
	}
	some.Receipts = w.list
	if _, err := Check(some); err != nil {
		t.Fatalf("the worked journal with some openings: %v", err)
	}
	// An opening that does not open its seal is founding, as any founding refusal is.
	bad := w.evidence()
	bad.Openings = setup.Openings{ByActor: map[string][]byte{p: w.set.Openings.ByActor[ap]}, Some: true}
	if _, err := Check(bad); !isCode(err, CodeFounding, -1) {
		t.Fatalf("another member's opening: %v", err)
	}
	// So is a missing acceptance.
	bad = w.evidence()
	bad.Acceptances = []byte(`[]`)
	if _, err := Check(bad); !isCode(err, CodeFounding, -1) {
		t.Fatalf("no acceptance: %v", err)
	}
}
