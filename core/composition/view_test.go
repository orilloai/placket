package composition_test

import (
	"testing"

	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
)

func sealedAccord(t *testing.T, parties []ptesting.Party) *setup.Accord {
	t.Helper()
	a, err := ptesting.Open(t, ptesting.ClearSet(t, parties), "2026-11-18T08:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestSidesAreTheMembersOwnTerms lists the crossings a member is a side of from its own terms only
// (setup-contract-14): each kind its terms select toward each member holding a role they name,
// itself included, decided by Decide; a kind only the other side names, and a member holding no role
// a term names, never appear; the rows are in counterpart, direction and kind order, each once.
func TestSidesAreTheMembersOwnTerms(t *testing.T) {
	t.Parallel()
	l := func(s ...string) []string { return s }
	a := sealedAccord(t, []ptesting.Party{
		{Name: "proposer", Roles: l("peer"), Terms: []shape.Object{
			ptesting.Term("want", "give", l("peer"), l("note", "brief")),
			ptesting.Term("want", "give", l("peer"), l("note")),
			ptesting.Term("want", "get", l("peer"), l("note")),
		}},
		{Name: "applier", Roles: l("peer"), Terms: []shape.Object{
			ptesting.Term("want", "get", l("peer"), l("note", "only-mine")),
		}},
		{Name: "lead", Roles: l("lead"), Terms: []shape.Object{ptesting.Term("want", "get", l("peer"), l("audit"))}},
	})
	p, ap := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference()
	got := composition.Sides(a, p)
	type row struct{ c, dir, kind string }
	want := map[row]composition.Verdict{
		{p, "get", "note"}:    composition.Pass,
		{p, "give", "brief"}:  composition.Block,
		{p, "give", "note"}:   composition.Pass,
		{ap, "get", "note"}:   composition.Block,
		{ap, "give", "brief"}: composition.Block,
		{ap, "give", "note"}:  composition.Pass,
	}
	if len(got) != len(want) {
		t.Fatalf("%d sides, want %d: %+v", len(got), len(want), got)
	}
	for i, s := range got {
		v, ok := want[row{s.Counterpart, s.Dir, s.Kind}]
		if !ok || s.Decision.Verdict != v {
			t.Fatalf("side %+v", s)
		}
		if i > 0 {
			x := got[i-1]
			if x.Counterpart == s.Counterpart && (x.Dir > s.Dir || (x.Dir == s.Dir && x.Kind >= s.Kind)) {
				t.Fatalf("sides out of order: %+v then %+v", x, s)
			}
		}
	}
	if s := composition.Sides(a, ptesting.Key(t, "stranger").Reference()); s != nil {
		t.Fatalf("a stranger's sides: %+v", s)
	}
	if got := composition.Answers(a, p); got != nil {
		t.Fatalf("answers with no maybe: %+v", got)
	}
}
