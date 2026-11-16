package calc

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2, 3) = %d, want 5", got)
	}
}

func TestKeep(t *testing.T) {
	if got := Add(1, 1); got != 2 {
		t.Fatalf("Add(1, 1) = %d, want 2", got)
	}
}

func TestMain(m *testing.M) { m.Run() }
