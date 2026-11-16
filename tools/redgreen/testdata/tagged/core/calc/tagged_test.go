//go:build never

package calc

import "testing"

func TestTagged(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("Add(1, 2) != 3")
	}
}
