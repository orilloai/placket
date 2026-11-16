package canonical_test

import (
	"testing"

	"github.com/orilloai/placket/core/canonical"
)

// TestThrowawayLessOrdersByCodeUnit is a throwaway check for ORI-22: it already passes on main.
func TestThrowawayLessOrdersByCodeUnit(t *testing.T) {
	if !canonical.Less("a", "b") || canonical.Less("b", "a") {
		t.Fatal("Less does not order a before b")
	}
}
