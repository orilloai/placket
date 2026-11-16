package calc

import "testing"

func TestA(t *testing.T) {
	if Sub(5, 3) != 2 {
		panic("Sub(5, 3) != 2")
	}
}

func TestB(t *testing.T) {
	if Add(5, 3) != 8 {
		t.Fatal("Add(5, 3) != 8")
	}
}
