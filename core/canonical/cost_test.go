package canonical_test

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
)

// TestCostGrowsWithSizeNotDepth puts a long string at the deepest nesting the form accepts, inside
// arrays, inside objects whose members are in order and inside objects whose members are not, and
// measures what Form allocates: a small multiple of the input's size, whatever its depth
// (canonical-1, canonical-8). It does not run in parallel, so the count is Form's alone.
func TestCostGrowsWithSizeNotDepth(t *testing.T) {
	const depth = canonical.MaxDepth - 1
	long := `"` + strings.Repeat("a", 1<<20) + `"`
	for _, c := range []struct {
		name       string
		open, shut string
	}{
		{"arrays", "[", "]"},
		{"objects in order", `{"a":`, "}"},
		{"objects out of order", `{"b":0,"a":`, "}"},
	} {
		in := []byte(strings.Repeat(c.open, depth) + long + strings.Repeat(c.shut, depth))
		var want []byte
		if c.name == "objects out of order" {
			want = []byte(strings.Repeat(`{"a":`, depth) + long + strings.Repeat(`,"b":0}`, depth))
		} else {
			want = in
		}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		out, err := canonical.Form(in)
		runtime.ReadMemStats(&after)
		if err != nil || !bytes.Equal(out, want) {
			t.Fatalf("%s: the form changed the value, or refused it: %v", c.name, err)
		}
		if got, limit := after.TotalAlloc-before.TotalAlloc, uint64(16*len(in)); got > limit {
			t.Fatalf("%s: Form allocated %d bytes for an input of %d, over %d", c.name, got, len(in), limit)
		}
	}
}
