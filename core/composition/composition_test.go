package composition_test

import (
	"testing"

	"github.com/orilloai/placket/core/composition"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
)

// accord founds a set of the founding vectors.
func accord(t testing.TB, name string) *setup.Accord {
	t.Helper()
	set, foundedAt := ptesting.Founding(t, name)
	a, err := setup.Found(set, foundedAt)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestDecide decides each kind of the composition Accord, and of the ask-and-log Accord, by the rows
// of section 5, and a part the mediator formed (composition-5).
func TestDecide(t *testing.T) {
	t.Parallel()
	compose, ask := accord(t, "compose"), accord(t, "ask-and-log")
	p, a, d := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference(), ptesting.Key(t, "decider").Reference()
	for _, c := range []struct {
		accord   *setup.Accord
		crossing composition.Crossing
		verdict  composition.Verdict
		reason   string
		to       []string
		timeout  int64
	}{
		{compose, composition.Crossing{From: p, To: a, Kind: "brief"}, composition.Pass, "ok", nil, 0},
		{compose, composition.Crossing{From: p, To: a, Kind: "draft"}, composition.Block, "no-match", nil, 0},
		{compose, composition.Crossing{From: p, To: a, Kind: "memo"}, composition.Escalate, "maybe", []string{"decider", "lead"}, 60},
		{compose, composition.Crossing{From: p, To: a, Kind: "memo", Formed: true}, composition.Block, "maybe", nil, 0},
		{compose, composition.Crossing{From: p, To: a, Kind: "plan"}, composition.Block, "maybe", nil, 0},
		{compose, composition.Crossing{From: p, To: a, Kind: "sketch"}, composition.Block, "no-match", nil, 0},
		{compose, composition.Crossing{From: p, To: a, Kind: "audit"}, composition.Block, "no-match", nil, 0},
		{compose, composition.Crossing{From: a, To: p, Kind: "brief"}, composition.Block, "no-match", nil, 0},
		{ask, composition.Crossing{From: p, To: a, Kind: "proposal"}, composition.Escalate, "maybe", []string{"decider"}, 3600},
		{ask, composition.Crossing{From: d, To: p, Kind: "answer", Formed: true}, composition.Pass, "ok", nil, 0},
		{ask, composition.Crossing{From: d, To: a, Kind: "changed-version", Formed: true}, composition.Pass, "ok", nil, 0},
		{ask, composition.Crossing{From: d, To: a, Kind: "answer", Formed: true}, composition.Block, "no-match", nil, 0},
		{ask, composition.Crossing{From: "ed25519:00", To: a, Kind: "note"}, composition.Block, "no-match", nil, 0},
	} {
		got := composition.Decide(c.accord, c.crossing)
		if got.Verdict != c.verdict || got.Reason != c.reason || got.Rule.Timeout != c.timeout || len(got.Rule.To) != len(c.to) {
			t.Fatalf("%+v: got %+v, want %s, %s, %v, %d", c.crossing, got, c.verdict, c.reason, c.to, c.timeout)
		}
		for i := range c.to {
			if got.Rule.To[i] != c.to[i] {
				t.Fatalf("%+v: rule %v, want %v", c.crossing, got.Rule.To, c.to)
			}
		}
	}
}

// FuzzDecide decides any crossing under the composition Accord. It never panics, and every decision
// pairs its verdict and reason as receipt-4 says.
func FuzzDecide(f *testing.F) {
	compose := accord(f, "compose")
	f.Add(ptesting.Key(f, "proposer").Reference(), ptesting.Key(f, "applier").Reference(), "memo", false)
	f.Add(ptesting.Key(f, "proposer").Reference(), ptesting.Key(f, "proposer").Reference(), "brief", true)
	f.Add("", "", "", false)
	f.Fuzz(func(t *testing.T, from, to, kind string, formed bool) {
		d := composition.Decide(compose, composition.Crossing{From: from, To: to, Kind: kind, Formed: formed})
		switch {
		case d.Verdict == composition.Pass && d.Reason == "ok":
		case d.Verdict == composition.Escalate && d.Reason == "maybe" && len(d.Rule.To) > 0 && !formed:
		case d.Verdict == composition.Block && (d.Reason == "no-match" || d.Reason == "maybe"):
		default:
			t.Fatalf("a decision %+v", d)
		}
	})
}
