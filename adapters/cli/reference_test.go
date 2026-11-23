package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestTheCLIReferenceSaysHowWatchPrints: watch, which prints one object per question before its own
// (commands-1), is the one command besides mcp the page's --json paragraph excepts from one object,
// and watch's own --json line says so.
func TestTheCLIReferenceSaysHowWatchPrints(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	var framing string
	for _, p := range strings.Split(page, "\n\n") {
		if strings.HasPrefix(p, "With `--json`, ") {
			framing = p
			break
		}
	}
	if !strings.Contains(framing, "every command but `mcp` and `watch` prints its result as one JSON object") ||
		!strings.Contains(framing, "`watch`, which runs until it is stopped, prints one object per question") {
		t.Errorf("the page's --json paragraph does not except watch from one object: %q", framing)
	}
	i := strings.Index(page, "\n## `watch`\n")
	if i < 0 {
		t.Fatal("the page has no section watch")
	}
	section := page[i:]
	if j := strings.Index(section[1:], "\n## "); j >= 0 {
		section = section[:j+1]
	}
	if !strings.Contains(section, "| `--json` | no |  | Print one JSON object per question on standard output") {
		t.Errorf("watch's --json line does not say it prints one object per question:\n%s", section)
	}
}

// TestTheCLIReferenceNamesBothVocabularies: the page's opening says that --json speaks the product
// words of section 11, while the MCP tools and the mediator API name a verdict escalate where --json
// says ask, and carry a question still open as open: true; and its exit code 1 is where verify's
// error verify ends, a log that fails (commands-8).
func TestTheCLIReferenceNamesBothVocabularies(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	opening, _, _ := strings.Cut(page, "\n## ")
	for _, want := range []string{"the product words", "`ask`", "`escalate`", "`open: true`", "MCP tools", "[mediator API](mediator-api.md)"} {
		if !strings.Contains(opening, want) {
			t.Errorf("the page's opening does not hold %q", want)
		}
	}
	for _, l := range strings.Split(page, "\n") {
		if strings.HasPrefix(l, "| `1` |") && !strings.Contains(l, "a log that fails verification in `verify`") {
			t.Errorf("exit code 1 does not name a log that fails verify: %q", l)
		}
		if strings.HasPrefix(l, "| `3` |") && !strings.Contains(l, "`error verify` from `verify` is a log that fails, exit 1") {
			t.Errorf("exit code 3 does not leave verify's failing log to exit 1: %q", l)
		}
	}
}
