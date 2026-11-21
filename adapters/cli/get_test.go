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

// TestAResultHandsOverWhatItCannotRecord asks a question that the decider accepts, with and without a
// part of kind answer given to the proposer first, and runs result when the proposer's state
// directory cannot take delivered.jsonl, made read-only before the file is. The first delivery result
// fetches is no longer queued, so it is printed on standard output, the answer as result prints it or
// the other part as get prints it, and then error state delivered.jsonl, exit 3; with --json the
// object holds it, as answer or under deliveries, and the error.
func TestAResultHandsOverWhatItCannotRecord(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		json bool
		body string
	}{{"the answer", false, "Yes."}, {"another part", false, "Not the answer."}, {"the answer --json", true, "Yes."}, {"another part --json", true, "Not the answer."}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := newAccord(t, "proposer", "applier", "decider")
			a.start(t)
			q := a.openOne(t, "Rename total.")
			other := c.body != "Yes."
			if other {
				must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "answer", "--re", q, "--text", c.body)...)
			}
			must(t, a.as("decider", "answer", "--question", q, "--text", "Yes.")...)
			dir := a.state("proposer")
			if _, err := os.Stat(filepath.Join(dir, "delivered.jsonl")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("delivered.jsonl before result: %v", err)
			}
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			if c.json {
				r := run(t, env(), a.as("proposer", "result", "--question", q, "--json")...)
				var o struct {
					Answer     *struct{ Text string }  `json:"answer"`
					Deliveries []struct{ Text string } `json:"deliveries"`
					Error      string                  `json:"error"`
					Detail     []string                `json:"detail"`
					Exit       int                     `json:"exit"`
				}
				if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || r.code != 3 || o.Exit != 3 || o.Error != "state" ||
					len(o.Detail) != 1 || o.Detail[0] != "delivered.jsonl" {
					t.Fatalf("result --json: %v", r)
				}
				if other && (o.Answer != nil || len(o.Deliveries) != 1 || o.Deliveries[0].Text != c.body) ||
					!other && (o.Answer == nil || o.Answer.Text != c.body || len(o.Deliveries) != 0) {
					t.Fatalf("result --json holds %+v and %+v", o.Answer, o.Deliveries)
				}
				return
			}
			r := run(t, env(), a.as("proposer", "result", "--question", q)...)
			l := r.lines()
			want := 2
			if other {
				want = 3
			}
			if r.code != 3 || last(r.stdout) != c.body || len(l) != want || !strings.HasPrefix(l[0], "accepted sha256:") || l[want-1] != "error state delivered.jsonl" ||
				other && !strings.HasPrefix(l[1], "delivery ") {
				t.Fatalf("result: %v", r)
			}
		})
	}
}

// TestAResultKeepsWhatItCanNeitherRecordNorWrite asks a question that the decider accepts, with and
// without a part of kind answer given to the proposer first, and runs result when the proposer's
// state directory cannot take delivered.jsonl, made read-only with kept.jsonl already there, and its
// standard output fails every write. The first delivery result fetches, no longer queued, is kept in
// kept.jsonl, and standard error ends in error file kept, exit 3.
func TestAResultKeepsWhatItCanNeitherRecordNorWrite(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"Yes.", "Not the answer."} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			a := newAccord(t, "proposer", "applier", "decider")
			a.start(t)
			q := a.openOne(t, "Rename total.")
			if body != "Yes." {
				must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "answer", "--re", q, "--text", body)...)
			}
			must(t, a.as("decider", "answer", "--question", q, "--text", "Yes.")...)
			dir := a.state("proposer")
			before := len(keptLines(t, dir))
			if before == 0 {
				t.Fatal("kept.jsonl holds nothing before result")
			}
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			r := runBroken(t, a.as("proposer", "result", "--question", q)...)
			if l := r.lines(); r.code != 3 || len(l) == 0 || l[len(l)-1] != "error file kept" {
				t.Fatalf("result: %v", r)
			}
			k := keptLines(t, dir)
			if len(k) != before+1 || str(k[before]["text"]) != body {
				t.Fatalf("kept.jsonl holds %d lines, want %d, the last %q", len(k), before+1, body)
			}
		})
	}
}

// TestGetWritesNoStateWithoutTheLock fetches a note with get --wait when the state directory cannot
// take delivered.jsonl, made read-only with kept.jsonl already there, and standard output fails
// every write. With the lock taken again after the wait, the body is kept and get ends in error
// file kept. When the lock cannot be taken again, its file no longer writable, nothing is written
// to kept.jsonl and get ends in error state lock, exit 3. Not parallel: it sets get's hook.
func TestGetWritesNoStateWithoutTheLock(t *testing.T) {
	for _, c := range []struct {
		name   string
		broken bool
		want   string
		kept   int
	}{{"the lock taken again", false, "error file kept", 1}, {"the lock not taken again", true, "error state lock", 0}} {
		t.Run(c.name, func(t *testing.T) {
			a := newAccord(t, "p1", "p2")
			a.start(t)
			if r := must(t, a.as("p2", "get")...); r.stderr != "no delivery\n" {
				t.Fatalf("the first get: %v", r)
			}
			ids := give(t, a, []string{"one"})
			dir := a.state("p2")
			if err := os.WriteFile(filepath.Join(dir, "kept.jsonl"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
			if c.broken {
				cli.SetRelocking(func() { _ = os.Chmod(filepath.Join(dir, "lock"), 0o400) })
				t.Cleanup(func() { cli.SetRelocking(nil) })
			}
			r := runBroken(t, a.as("p2", "get", "--wait", "1")...)
			if l := r.lines(); r.code != 3 || len(l) != 2 || !strings.HasPrefix(l[0], "delivery "+ids[0]+" kind note ") || l[1] != c.want {
				t.Fatalf("get: %v", r)
			}
			if got := keptIDs(t, dir); len(got) != c.kept {
				t.Fatalf("kept.jsonl holds %v", got)
			}
		})
	}
}
