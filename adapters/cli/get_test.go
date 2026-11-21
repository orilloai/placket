package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// brokenWriter is a standard output that takes nothing, as a pipe whose reader has gone.
type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

// runBroken runs a command whose standard output fails every write.
func runBroken(t testing.TB, args ...string) result {
	t.Helper()
	var stderr bytes.Buffer
	code := cli.Run(env(), args, strings.NewReader(""), brokenWriter{}, &stderr)
	return result{"", stderr.String(), code}
}

// keptIDs is the receipt ids of the lines of kept.jsonl in a state directory, in order.
func keptIDs(t testing.TB, dir string) []string {
	t.Helper()
	var ids []string
	for _, k := range keptLines(t, dir) {
		ids = append(ids, str(k["receiptId"]))
	}
	return ids
}

// TestGetKeepsABodyItCannotWrite fetches a note with get when standard output fails every write,
// with and without --keep, --all and --json: each exits 3, its standard error ends in error file
// kept, and kept.jsonl holds the delivery once. With --all it stops at the first body it cannot
// write, so the second note is still queued.
func TestGetKeepsABodyItCannotWrite(t *testing.T) {
	t.Parallel()
	for _, flags := range [][]string{nil, {"--keep"}, {"--json"}, {"--json", "--keep"}, {"--json", "--all"}} {
		t.Run(strings.Join(append([]string{"get"}, flags...), " "), func(t *testing.T) {
			t.Parallel()
			a := newAccord(t, "p1", "p2")
			a.start(t)
			ids := give(t, a, []string{"one"})
			r := runBroken(t, a.as("p2", append([]string{"get"}, flags...)...)...)
			if l := r.lines(); r.code != 3 || len(l) != 2 || !strings.HasPrefix(l[0], "delivery "+ids[0]+" kind note ") || l[1] != "error file kept" {
				t.Fatalf("get: %v", r)
			}
			if got := keptIDs(t, a.state("p2")); len(got) != 1 || got[0] != ids[0] {
				t.Fatalf("kept.jsonl holds %v, want %v", got, ids)
			}
		})
	}
	t.Run("get --all", func(t *testing.T) {
		t.Parallel()
		a := newAccord(t, "p1", "p2")
		a.start(t)
		ids := give(t, a, []string{"one", "two"})
		r := runBroken(t, a.as("p2", "get", "--all")...)
		if l := r.lines(); r.code != 3 || len(l) != 2 || !strings.HasPrefix(l[0], "delivery "+ids[0]+" kind note ") || l[1] != "error file kept" {
			t.Fatalf("get --all: %v", r)
		}
		if got := keptIDs(t, a.state("p2")); len(got) != 1 || got[0] != ids[0] {
			t.Fatalf("kept.jsonl holds %v, want %v", got, ids[:1])
		}
		if r := must(t, a.as("p2", "get")...); r.stdout != "two\n" {
			t.Fatalf("the get after: %v", r)
		}
	})
}

// TestGetHandsOverABodyItCannotRecord fetches a note when the state directory cannot take
// delivered.jsonl, then salts.jsonl, then, with --keep, kept.jsonl: the directory is made read-only
// with the files before the one that fails already there. The body is printed on standard output,
// the delivery's line and then error state naming the file on standard error, exit 3; with --json
// the object holds the delivery and the error.
func TestGetHandsOverABodyItCannotRecord(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		file  string
		there []string
		flags []string
	}{
		{"delivered.jsonl", nil, nil},
		{"salts.jsonl", []string{"delivered.jsonl"}, nil},
		{"kept.jsonl", []string{"delivered.jsonl", "salts.jsonl"}, []string{"--keep"}},
	} {
		t.Run(c.file, func(t *testing.T) {
			t.Parallel()
			a := newAccord(t, "p1", "p2")
			a.start(t)
			if r := must(t, a.as("p2", "get")...); r.stderr != "no delivery\n" {
				t.Fatalf("the first get: %v", r)
			}
			ids := give(t, a, []string{"one", "two"})
			dir := a.state("p2")
			for _, f := range c.there {
				if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			r := run(t, env(), a.as("p2", append([]string{"get"}, c.flags...)...)...)
			if l := r.lines(); r.code != 3 || r.stdout != "one\n" || len(l) != 2 || !strings.HasPrefix(l[0], "delivery "+ids[0]+" kind note ") ||
				l[1] != "error state "+c.file {
				t.Fatalf("get: %v", r)
			}
			r = run(t, env(), a.as("p2", append([]string{"get", "--json"}, c.flags...)...)...)
			var o struct {
				Delivery struct {
					ReceiptID string `json:"receiptId"`
					Text      string `json:"text"`
				} `json:"delivery"`
				Error  string   `json:"error"`
				Detail []string `json:"detail"`
				Exit   int      `json:"exit"`
			}
			if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || r.code != 3 || o.Exit != 3 || o.Delivery.ReceiptID != ids[1] ||
				o.Delivery.Text != "two" || o.Error != "state" || len(o.Detail) != 1 || o.Detail[0] != c.file {
				t.Fatalf("get --json: %v", r)
			}
		})
	}
}
