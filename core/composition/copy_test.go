package composition_test

import (
	"reflect"
	"testing"

	"github.com/orilloai/placket/core/composition"
	ptesting "github.com/orilloai/placket/core/internal/testing"
)

// TestADecisionIsACopy changes the rule an escalate returned and decides the same crossing again:
// the second decision is the first, so a caller cannot rewrite the Accord's rule through a result
// (results-are-copies).
func TestADecisionIsACopy(t *testing.T) {
	t.Parallel()
	compose := accord(t, "compose")
	c := composition.Crossing{From: ptesting.Key(t, "proposer").Reference(), To: ptesting.Key(t, "applier").Reference(), Kind: "memo"}
	first := composition.Decide(compose, c)
	want := composition.Decision{Verdict: composition.Escalate, Reason: composition.ReasonMaybe}
	want.Rule.To, want.Rule.Timeout = []string{"decider", "lead"}, 60
	if !reflect.DeepEqual(first, want) {
		t.Fatalf("got %+v, want %+v", first, want)
	}
	first.Rule.To[0], first.Rule.Timeout = "changed", 1
	if again := composition.Decide(compose, c); !reflect.DeepEqual(again, want) {
		t.Fatalf("after the returned rule was changed: got %+v, want %+v", again, want)
	}
}

// TestNoAccordBlocks decides crossings under a nil Accord: it reads as an Accord with no members and
// no terms, so every crossing is blocked, no-match, by row 2 of section 5, and none passes or asks.
func TestNoAccordBlocks(t *testing.T) {
	t.Parallel()
	p, a := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference()
	for _, c := range []composition.Crossing{
		{From: p, To: a, Kind: "brief"},
		{From: p, To: a, Kind: "memo"},
		{From: p, To: p, Kind: "answer", Formed: true},
		{},
	} {
		got := composition.Decide(nil, c)
		if want := (composition.Decision{Verdict: composition.Block, Reason: composition.ReasonNoMatch}); !reflect.DeepEqual(got, want) {
			t.Fatalf("%+v: got %+v, want %+v", c, got, want)
		}
		if got.Verdict == composition.Pass {
			t.Fatalf("%+v passed with no Accord", c)
		}
	}
}
