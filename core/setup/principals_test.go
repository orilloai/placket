package setup_test

import (
	"slices"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
)

// TestEachPartyItsOwnPrincipal founds the set with a principal per side: the principal may grant
// proposer and applier and binds the proposer and the applier, the other principal may grant decider
// and binds the decider. Each member holds its binding's roles, and its principal is the one that
// signed its binding (setup-contract-2, setup-contract-9).
func TestEachPartyItsOwnPrincipal(t *testing.T) {
	t.Parallel()
	set, at := base(t, "two-principals")
	a, err := foundClear(t, encode(t, set), at)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		roles     []string
		principal string
	}{
		ptesting.Key(t, "proposer").Reference(): {[]string{"proposer"}, ptesting.Key(t, "principal").Reference()},
		ptesting.Key(t, "applier").Reference():  {[]string{"applier"}, ptesting.Key(t, "principal").Reference()},
		ptesting.Key(t, "decider").Reference():  {[]string{"decider"}, ptesting.Key(t, "other-principal").Reference()},
	}
	if len(a.Members()) != len(want) {
		t.Fatalf("%d members, want %d", len(a.Members()), len(want))
	}
	for _, m := range a.Members() {
		w, ok := want[m]
		if !ok {
			t.Fatalf("an unexpected member %s", m)
		}
		if got := a.Roles(m); !slices.Equal(got, w.roles) {
			t.Fatalf("%s holds %q, want %q", m, got, w.roles)
		}
		if got := a.Principal(m); got != w.principal {
			t.Fatalf("%s is bound by %s, want %s", m, got, w.principal)
		}
	}
	if got := a.Principal(ptesting.Key(t, "stranger").Reference()); got != "" {
		t.Fatalf("a stranger's principal is %q, want none", got)
	}
}

// TestOnePrincipalForEveryParty founds the ask-and-log set, whose one policy entry may grant every
// role: every member's principal is that one key.
func TestOnePrincipalForEveryParty(t *testing.T) {
	t.Parallel()
	set, at := base(t, "ask-and-log")
	a, err := foundClear(t, encode(t, set), at)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range a.Members() {
		if got, want := a.Principal(m), ptesting.Key(t, "principal").Reference(); got != want {
			t.Fatalf("%s is bound by %s, want %s", m, got, want)
		}
	}
}

// TestAPrincipalGrantsOnlyItsRoles founds each founding vector of a principal per party and holds
// its code: a role its principal's entry does not list is binding-role, a binding for another policy
// binding-policy, the role before the mediator and before the policy, an unlisted principal still
// binding-principal, a role held that the policy does not require founds, and every defect of the
// new shape is invalid-shape (setup-contract-3, setup-contract-5, setup-contract-8,
// setup-contract-9).
func TestAPrincipalGrantsOnlyItsRoles(t *testing.T) {
	t.Parallel()
	cases := map[string]setup.Code{
		"two-principals":                    founded,
		"two-entries-one-role":              founded,
		"a-role-held-not-required":          founded,
		"binding-role-not-granted":          setup.BindingRole,
		"binding-other-policy":              setup.BindingPolicy,
		"ungranted-role-and-other-mediator": setup.BindingRole,
		"ungranted-role-and-other-policy":   setup.BindingRole,
		"binding-unlisted-principal":        setup.BindingPrincipal,
		"principals-empty":                  setup.InvalidShape,
		"entry-roles-empty":                 setup.InvalidShape,
		"entry-roles-repeated":              setup.InvalidShape,
		"entry-role-not-a-label":            setup.InvalidShape,
		"entry-principal-not-a-reference":   setup.InvalidShape,
		"two-entries-one-principal":         setup.InvalidShape,
		"entry-unlisted-member":             setup.InvalidShape,
		"binding-without-policy":            setup.InvalidShape,
		"binding-policy-not-a-hash":         setup.InvalidShape,
	}
	vs := vectors(t)
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v, ok := vs[name]
			if !ok {
				t.Fatalf("no vector %s", name)
			}
			if c := foundCode(t, v.set(), v.FoundedAt); c != want {
				t.Fatalf("got %q, want %q", c, want)
			}
			if string(want) != v.Expect.Code {
				t.Fatalf("the vector expects %q, the test %q", v.Expect.Code, want)
			}
		})
	}
}
