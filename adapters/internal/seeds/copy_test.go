package seeds

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestTheCoresCopyIsTheSame reads this file and the core's seeds helper: past their package
// comments they are the same bytes. Go's internal rule keeps each module from importing the
// other's, so a fix to one must be made to both.
func TestTheCoresCopyIsTheSame(t *testing.T) {
	t.Parallel()
	ours, err := os.ReadFile("seeds.go")
	if err != nil {
		t.Fatal(err)
	}
	core, err := os.ReadFile(filepath.Join("..", "..", "..", "core", "internal", "testing", "seeds", "seeds.go"))
	if err != nil {
		t.Fatal(err)
	}
	if a, b := code(ours), code(core); a == nil || !bytes.Equal(a, b) {
		t.Fatal("adapters/internal/seeds/seeds.go and core/internal/testing/seeds/seeds.go differ past their package comments")
	}
}

// code is a Go file's bytes from its package clause on, or nil when it has none.
func code(b []byte) []byte {
	i := bytes.Index(b, []byte("\npackage seeds\n"))
	if i < 0 {
		return nil
	}
	return b[i:]
}
