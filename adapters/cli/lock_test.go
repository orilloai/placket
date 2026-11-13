package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestChangeKeepsUnderTheLock changes a proposal with --keep, as a decider whose kept.jsonl ends in
// a line with no line feed: the change takes the state lock first, so the line is removed with one
// line on standard error before anything else is printed, and the version is kept on a line of its
// own (state-lock).
func TestChangeKeepsUnderTheLock(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Drop notes.")
	dir := a.state("decider")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "kept.jsonl")
	if err := os.WriteFile(p, []byte(`{"torn":{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := must(t, a.as("decider", "change", "--question", q, "--text", "Hide notes.", "--keep")...)
	l := r.lines()
	if len(l) != 3 || l[0] != "state: kept.jsonl line 1 had no line feed and was never written; removed" ||
		!strings.HasPrefix(l[1], "pass ok ") || !strings.HasPrefix(l[2], "released ok ") {
		t.Fatalf("change --keep: %v", r)
	}
	b := fileOf(t, p)
	k := keptLines(t, dir)
	if strings.Count(b, "\n") != 1 || !strings.HasSuffix(b, "\n") || len(k) != 1 || str(k[0]["text"]) != "Hide notes." || str(k[0]["re"]) != q {
		t.Fatalf("kept.jsonl: %q", b)
	}
}
