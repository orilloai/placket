package shape

import "testing"

// TestThrowawayTwice is a throwaway for ORI-22's fork check: it fails on main, where there is no
// throwawayTwice, and passes with it.
func TestThrowawayTwice(t *testing.T) {
	t.Parallel()
	if got := throwawayTwice(21); got != 42 {
		t.Fatalf("throwawayTwice(21) = %d, want 42", got)
	}
}
