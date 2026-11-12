package setup_test

import (
	"testing"

	"github.com/orilloai/placket/core/setup"
)

// FuzzAccordReads reads any actor's membership, roles and terms in the ask-and-log Accord. It never
// panics; an actor who is not a member has no role and no term, and a member has both.
func FuzzAccordReads(f *testing.F) {
	v := vectors(f)["ask-and-log"]
	a, err := setup.Found(v.set(), v.FoundedAt)
	if err != nil {
		f.Fatal(err)
	}
	for _, m := range a.Members() {
		f.Add(m)
	}
	f.Add(a.Mediator())
	f.Add("ed25519:")
	f.Add("")
	f.Add("role:proposer")
	f.Fuzz(func(t *testing.T, actor string) {
		member := false
		for _, m := range a.Members() {
			member = member || m == actor
		}
		roles, terms := a.Roles(actor), a.Terms(actor)
		if a.Member(actor) != member || member != (len(roles) > 0) || member != (len(terms) > 0) {
			t.Fatalf("%q: member %v, %d roles, %d terms", actor, a.Member(actor), len(roles), len(terms))
		}
	})
}
