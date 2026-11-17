package setup_test

import (
	"testing"

	"github.com/orilloai/placket/core/setup"
)

// TestANilAccordReadsAsEmpty reads every method of a nil Accord: each returns its zero value, as an
// Accord with no members and no terms would, and none panics (core-no-panic).
func TestANilAccordReadsAsEmpty(t *testing.T) {
	t.Parallel()
	var a *setup.Accord
	const actor = "ed25519:0000000000000000000000000000000000000000000000000000000000000000"
	if a.ID() != "" || a.Card() != "" || a.Mediator() != "" || a.Principal(actor) != "" {
		t.Fatalf("id %q, card %q, mediator %q, principal %q", a.ID(), a.Card(), a.Mediator(), a.Principal(actor))
	}
	if a.Members() != nil || a.Member(actor) || a.Roles(actor) != nil || a.Terms(actor) != nil {
		t.Fatalf("members %v, member %v, roles %v, terms %v", a.Members(), a.Member(actor), a.Roles(actor), a.Terms(actor))
	}
}
