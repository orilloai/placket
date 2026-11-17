package mediator_test

import (
	"errors"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/mediator"
)

// FuzzGet fetches as any caller after a give that queued one delivery for the applier. It never
// panics; a caller who is not a member is rejected with not-a-member and changes nothing, and a
// member's fetch takes at most the one delivery, once.
func FuzzGet(f *testing.F) {
	p, ap := ptesting.Key(f, "proposer"), ptesting.Key(f, "applier")
	f.Add(ap.Reference())
	f.Add(p.Reference())
	f.Add(ptesting.Key(f, "mediator").Reference())
	f.Add("ed25519:")
	f.Add("")
	for _, s := range seeds.Values(f, "founding", "actor", "principal", "mediator") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, caller string) {
		m := found(t)
		note := message(t, m.Accord(), "n", ap.Reference(), "note", "A note.", nil)
		if _, err := m.Give(p.Reference(), note, envelopeFor(t, note, p), "2026-11-12T09:00:00Z"); err != nil {
			t.Fatal(err)
		}
		d, err := m.Get(caller)
		if err != nil {
			var r *mediator.Rejection
			if !errors.As(err, &r) || r.Code != mediator.NotAMember || d != nil {
				t.Fatalf("got %q, %v", d, err)
			}
			if again, _ := m.Get(ap.Reference()); again == nil {
				t.Fatal("a refused fetch took the applier's delivery")
			}
			return
		}
		if (d != nil) != (caller == ap.Reference()) {
			t.Fatalf("%q fetched %q", caller, d)
		}
		if again, err := m.Get(caller); again != nil || err != nil {
			t.Fatalf("a second fetch: %q, %v", again, err)
		}
	})
}
