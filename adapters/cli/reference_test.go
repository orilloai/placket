package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTheCLIReferenceIsGenerated runs the CLI reference's generator afresh: docs/reference/cli.md is
// exactly what it writes, so a row of the command table, or a sentence of the error table, changed
// without the page fails here.
func TestTheCLIReferenceIsGenerated(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	out := filepath.Join(t.TempDir(), "cli.md")
	if b, err := exec.Command("go", "run", "./gen", "-o", out, "-spec", filepath.Join(root, "spec", "protocol.md")).CombinedOutput(); err != nil {
		t.Fatalf("the generator: %v\n%s", err, b)
	}
	fresh, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join(root, "docs", "reference", "cli.md"))
	if err != nil || !bytes.Equal(fresh, committed) {
		t.Fatalf("docs/reference/cli.md differs from a fresh go run ./adapters/cli/gen (%v)", err)
	}
}
