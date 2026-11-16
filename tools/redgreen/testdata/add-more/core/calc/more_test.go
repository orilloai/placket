package calc

import "testing"

func TestAddMore(t *testing.T) {
	if got := Add(-1, 1); got != 0 {
		t.Fatalf("Add(-1, 1) = %d, want 0", got)
	}
}
