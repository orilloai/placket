package setup_test

import (
	"testing"

	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/setup"
)

// FuzzAccordReads reads any actor's membership, roles, principal and terms in the ask-and-log Accord.
// It never panics; an actor who is not a member has no role, no principal and no term, and a member
// has all three.
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
	for _, s := range seeds.Strings(f, "founding") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, actor string) {
		member := false
		for _, m := range a.Members() {
			member = member || m == actor
		}
		roles, terms, principal := a.Roles(actor), a.Terms(actor), a.Principal(actor)
		if a.Member(actor) != member || member != (len(roles) > 0) || member != (len(terms) > 0) || member != (principal != "") {
			t.Fatalf("%q: member %v, %d roles, %d terms, principal %q", actor, a.Member(actor), len(roles), len(terms), principal)
		}
	})
}
