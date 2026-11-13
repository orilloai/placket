package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetOutLosesNoDelivery fetches with --out into a directory that is not there, and into a path
// that is a file: each is tool error file, exit 3, before anything is fetched, so nothing is recorded
// and the delivery stays queued. Into a directory where the body's name is already taken, the
// delivery is fetched and recorded, the file there is left as it was, and the delivery is kept in
// kept.jsonl, which the error says: the next get finds nothing, and the kept part holds the body.
func TestGetOutLosesNoDelivery(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	gave := must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "a note")...)
	f := strings.Fields(gave.stderr)
	if len(f) != 3 || f[0] != "pass" {
		t.Fatalf("give: %v", gave)
	}
	id := f[2]
	plain := filepath.Join(a.dir, "plain")
	if err := os.WriteFile(plain, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{filepath.Join(a.dir, "none"), plain} {
		r := run(t, env(), a.as("p2", "get", "--out", out)...)
		if r.code != 3 || r.stdout != "" || r.stderr != "error file\n" {
			t.Fatalf("get --out %s: %v", filepath.Base(out), r)
		}
		if got := fileOf(t, filepath.Join(a.state("p2"), "delivered.jsonl")); got != "" {
			t.Fatalf("get --out %s recorded %q", filepath.Base(out), got)
		}
	}

	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	taken := filepath.Join(out, strings.TrimPrefix(id, "sha256:")+".note")
	if err := os.WriteFile(taken, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), a.as("p2", "get", "--out", out)...)
	if r.code != 3 || r.stdout != "" || len(r.lines()) != 2 || !strings.HasPrefix(r.lines()[0], "delivery "+id+" kind note ") || r.lines()[1] != "error file kept" {
		t.Fatalf("get --out onto a name taken: %v", r)
	}
	if fileOf(t, taken) != "mine" {
		t.Fatal("get --out wrote over a file")
	}
	kept := keptLines(t, a.state("p2"))
	if len(kept) != 1 || str(kept[0]["receiptId"]) != id || str(kept[0]["text"]) != "a note" {
		t.Fatalf("kept: %v", kept)
	}
	if d := strings.Split(strings.TrimSuffix(fileOf(t, filepath.Join(a.state("p2"), "delivered.jsonl")), "\n"), "\n"); len(d) != 1 || d[0] == "" {
		t.Fatalf("delivered.jsonl: %q", d)
	}
	if r := must(t, a.as("p2", "get")...); r.stderr != "no delivery\n" {
		t.Fatalf("the get after: %v", r)
	}
}
