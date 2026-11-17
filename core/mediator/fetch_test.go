package mediator_test

import (
	"encoding/json"
	"runtime"
	"testing"
	"weak"

	ptesting "github.com/orilloai/placket/core/internal/testing"
)

// TestAFetchedDeliveryIsNotKept gives three notes to the applier and fetches them one by one: once
// the caller lets go of a delivery it fetched, nothing the mediator holds keeps it, its body and salt
// included, while the deliveries after it stay queued in order (crossing-3).
func TestAFetchedDeliveryIsNotKept(t *testing.T) {
	m := found(t)
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	ids := []string{"n-1", "n-2", "n-3"}
	for i, id := range ids {
		msg := message(t, m.Accord(), id, ap.Reference(), "note", "The note "+id+".", nil)
		if _, err := m.Give(p.Reference(), msg, envelope(t, int64(i+1), msg, p), nil, "2026-11-12T09:00:00Z"); err != nil {
			t.Fatalf("give %s: %v", id, err)
		}
	}
	// fetch takes the next delivery, checks its messageId and returns only a weak pointer to its
	// bytes, so the test itself holds nothing of it.
	fetch := func(want string) weak.Pointer[byte] {
		d, err := m.Get(ap.Reference())
		if err != nil || d == nil {
			t.Fatalf("fetching %s: got %q, %v", want, d, err)
		}
		var o struct {
			MessageID string `json:"messageId"`
		}
		if err := json.Unmarshal(d, &o); err != nil || o.MessageID != want {
			t.Fatalf("fetched %q, want %s (%v)", o.MessageID, want, err)
		}
		return weak.Make(&d[0])
	}
	for _, id := range ids {
		fetched := fetch(id)
		runtime.GC()
		if fetched.Value() != nil {
			t.Fatalf("the mediator still holds delivery %s after it was fetched", id)
		}
	}
	if d, err := m.Get(ap.Reference()); d != nil || err != nil {
		t.Fatalf("a fourth fetch got %q, %v", d, err)
	}
	runtime.KeepAlive(m)
}
