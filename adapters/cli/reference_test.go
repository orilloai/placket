package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	const words = "The object speaks the product words of that section: a verdict is `pass`, `block` or `ask`, " +
		"and a question still open has the closing word `open`. The MCP tools of `placket mcp` and the " +
		"[mediator API](mediator-api.md) keep the window's own words instead: they name the verdict `escalate` " +
		"where `--json` says `ask`, and the MCP tool `result` carries a question still open as `open: true`."
	if paragraphs := strings.Split(opening, "\n\n"); !slices.Contains(paragraphs, words) {
		t.Errorf("the page's opening has no paragraph naming both vocabularies, exactly %q:\n%s", words, opening)
	}
	lines := strings.Split(page, "\n")
	for _, row := range []string{
		"| `1` | A block on `give`, `answer` or `change`; a release that blocked; a `give --wait` or `result` whose " +
			"question closed any way but an accept that passed and was released; a comment that did not cross; a file, " +
			"or a value inside it, that no release matches; a log that fails verification in `verify`: one that is not " +
			"valid, holds more than one Accord or is not the one a successor supersedes, each with `error verify` and its " +
			"code on standard error, or one that does not conform; and a body that is not its receipt's part, in `verify`. |",
		"| `3` | A tool or door error: standard error ends `error`, or `door`, and its code. `error verify` from `verify` " +
			"is a log that fails, exit 1. |",
	} {
		if !slices.Contains(lines, row) {
			t.Errorf("the exit table has no row, exactly %q", row)
		}
	}
}
