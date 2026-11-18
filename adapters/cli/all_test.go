package cli_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// TestGetAllFetchesEveryDelivery fetches with get --all: on an empty queue it prints no delivery and
// exits 0; with three notes queued it fetches all three in one command, in the order they were
// given, each printed as get prints it, each recorded in delivered.jsonl and kept in kept.jsonl,
// and the get after it finds nothing.
func TestGetAllFetchesEveryDelivery(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	if r := must(t, a.as("p2", "get", "--all")...); r.stdout != "" || r.stderr != "no delivery\n" {
		t.Fatalf("get --all on an empty queue: %v", r)
	}
	texts := []string{"one", "two", "three"}
	ids := give(t, a, texts)
	r := must(t, a.as("p2", "get", "--all")...)
	if r.stdout != "one\ntwo\nthree\n" {
		t.Fatalf("get --all: %v", r)
	}
	l := r.lines()
	if len(l) != 3 {
		t.Fatalf("get --all: %v", r)
	}
	for i, id := range ids {
		if !strings.HasPrefix(l[i], "delivery "+id+" kind note messageId ") {
			t.Fatalf("get --all line %d: %v", i, r)
		}
	}
	if d := strings.Split(strings.TrimSuffix(fileOf(t, filepath.Join(a.state("p2"), "delivered.jsonl")), "\n"), "\n"); len(d) != 3 {
		t.Fatalf("delivered.jsonl: %q", d)
	}
	kept := keptLines(t, a.state("p2"))
	if len(kept) != 3 {
		t.Fatalf("kept: %v", kept)
	}
	for i, k := range kept {
		if str(k["receiptId"]) != ids[i] || str(k["text"]) != texts[i] {
			t.Fatalf("kept line %d: %v", i, kept)
		}
	}
	if r := must(t, a.as("p2", "get")...); r.stdout != "" || r.stderr != "no delivery\n" {
		t.Fatalf("the get after: %v", r)
	}
}

// TestGetAllOut fetches every delivery with --out: each body is written to a file of its own, named
// as get --out names it, and its path printed on standard output, one line each.
func TestGetAllOut(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	texts := []string{"one", "two"}
	ids := give(t, a, texts)
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	r := must(t, a.as("p2", "get", "--all", "--out", out)...)
	var want string
	for i, id := range ids {
		p := filepath.Join(out, strings.TrimPrefix(id, "sha256:")+".note")
		want += p + "\n"
		if got := fileOf(t, p); got != texts[i] {
			t.Fatalf("%s: %q", filepath.Base(p), got)
		}
	}
	if r.stdout != want || len(r.lines()) != 2 {
		t.Fatalf("get --all --out: %v", r)
	}
	if k := keptLines(t, a.state("p2")); len(k) != 2 {
		t.Fatalf("kept: %v", k)
	}
}

// TestGetAllStopsAtAFileItCannotWrite fetches two deliveries with --all --out into a directory
// where the first body's name is already taken: the first is fetched, kept and refused as error
// file kept, exit 3, and get --all stops there, so the second stays queued for the next get.
func TestGetAllStopsAtAFileItCannotWrite(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	ids := give(t, a, []string{"one", "two"})
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, strings.TrimPrefix(ids[0], "sha256:")+".note"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), a.as("p2", "get", "--all", "--out", out)...)
	if l := r.lines(); r.code != 3 || r.stdout != "" || len(l) != 2 || !strings.HasPrefix(l[0], "delivery "+ids[0]+" kind note ") || l[1] != "error file kept" {
		t.Fatalf("get --all --out onto a name taken: %v", r)
	}
	if k := keptLines(t, a.state("p2")); len(k) != 1 || str(k[0]["receiptId"]) != ids[0] {
		t.Fatalf("kept: %v", k)
	}
	r = must(t, a.as("p2", "get")...)
	if r.stdout != "two\n" || !strings.HasPrefix(r.stderr, "delivery "+ids[1]+" kind note ") {
		t.Fatalf("the get after: %v", r)
	}
}

// give gives each text as a note from p1 to p2 and returns the receipt ids.
func give(t *testing.T, a *accord, texts []string) []string {
	t.Helper()
	var ids []string
	for _, s := range texts {
		f := strings.Fields(must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", s)...).stderr)
		if len(f) != 3 || f[0] != "pass" {
			t.Fatalf("give %s: %v", s, f)
		}
		ids = append(ids, f[2])
	}
	return ids
}

// socketLimit is the longest socket path the system takes, its sun_path less the closing zero byte.
const socketLimit = len(syscall.RawSockaddrUnix{}.Path) - 1

// TestALongSocketPathIsRefused runs the window and every socket command with a socket path one byte
// longer than the system takes: each is tool error socket-path, naming the limit, exit 3, before
// anything is bound, connected or written, so the window makes no journal and the command no state
// directory. A path of exactly the limit is tried: with no window there, it is error socket.
func TestALongSocketPathIsRefused(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	// The parties accepted their views: p1's state directory goes, to show that nothing makes it.
	if err := os.RemoveAll(a.key("p1") + ".state"); err != nil {
		t.Fatal(err)
	}
	long := filepath.Join(a.dir, strings.Repeat("s", socketLimit-len(a.dir)))
	if len(long) != socketLimit+1 {
		t.Fatalf("the long path is %d bytes", len(long))
	}
	refused := "error socket-path limit " + strconv.Itoa(socketLimit) + "\n"
	journal := filepath.Join(a.dir, "j-long")
	r := run(t, env(), "window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--openings", a.openings(), "--socket", long, "--journal", journal)
	if r.code != 3 || r.stdout != "" || r.stderr != refused {
		t.Fatalf("window: %v", r)
	}
	if _, err := os.Lstat(journal); !os.IsNotExist(err) {
		t.Fatalf("the window made its journal: %v", err)
	}
	q := "sha256:" + strings.Repeat("0", 64)
	for _, args := range [][]string{
		{"get"}, {"get", "--all"}, {"questions"},
		{"give", "--to", a.refs["p2"], "--kind", "note", "--text", "a note"},
		{"result", "--question", q}, {"answer", "--question", q}, {"change", "--question", q, "--text", "x"},
		{"decline", "--question", q}, {"cancel", "--question", q},
	} {
		full := append(append([]string(nil), args...), "--key", a.key("p1"), "--socket", long, "--accord", a.id)
		r := run(t, env(), full...)
		if r.code != 3 || r.stdout != "" || r.stderr != refused {
			t.Fatalf("%v: %v", args, r)
		}
	}
	if _, err := os.Lstat(a.key("p1") + ".state"); !os.IsNotExist(err) {
		t.Fatalf("a refused command made its state directory: %v", err)
	}
	limit := long[:socketLimit]
	r = run(t, env(), "get", "--key", a.key("p1"), "--socket", limit, "--accord", a.id)
	if r.code != 3 || r.stdout != "" || r.stderr != "error socket\n" {
		t.Fatalf("get at the limit: %v", r)
	}
}
