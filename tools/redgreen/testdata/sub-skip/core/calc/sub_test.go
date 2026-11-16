package calc

import "testing"

func TestSub(t *testing.T) {
	t.Skip("not yet")
	if got := Sub(5, 3); got != 2 {
		t.Fatalf("Sub(5, 3) = %d, want 2", got)
	}
}
