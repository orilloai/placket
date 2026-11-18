package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/signing"
)

// TestSealPrintsTheEntryAndWritesTheOpening seals a declaration (setup-contract-12,
// setup-contract-13): standard output is the seal entry, its actor the key's and its seal sha256: and
// the SHA-256 of the opening's salt and the declaration's canonical bytes; the opening is
// <hex>.opening.json in --out, mode 0600, holding exactly the declaration and a 32-byte salt; standard
// error names the file and never its bytes. A second seal into the same directory replaces nothing
// and is error file opening; a declaration another key signed is error seal.
func TestSealPrintsTheEntryAndWritesTheOpening(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	decl := filepath.Join(a.dir, "p1.declaration")
	out := t.TempDir()
	r := must(t, "seal", "--key", a.key("p1"), "--declaration", decl, "--out", out, "--json")
	var o struct {
		Seal    struct{ Actor, Seal string }
		Opening string
	}
	if err := json.Unmarshal([]byte(r.stdout), &o); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, strings.TrimPrefix(a.refs["p1"], "ed25519:")+".opening.json")
	if o.Opening != path || o.Seal.Actor != a.refs["p1"] || r.stderr != "opening "+path+"\n" {
		t.Fatalf("seal: %v", r)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the opening: %v, %v", fi, err)
	}
	b, _ := os.ReadFile(path)
	var op struct {
		Declaration json.RawMessage `json:"declaration"`
		Salt        string          `json:"salt"`
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(b, &op) != nil || json.Unmarshal(b, &members) != nil || len(members) != 2 || len(op.Salt) != 64 {
		t.Fatalf("the opening: %s", b)
	}
	d, _ := os.ReadFile(decl)
	want, _ := canonical.Form(bytes.TrimSuffix(d, []byte("\n")))
	got, _ := canonical.Form(op.Declaration)
	salt, _ := hex.DecodeString(op.Salt)
	sum := sha256.Sum256(append(salt, want...))
	if !bytes.Equal(got, want) || o.Seal.Seal != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("the seal %s does not seal the declaration under the opening's salt", o.Seal.Seal)
	}
	if strings.Contains(r.stderr, op.Salt) {
		t.Fatal("seal printed the salt")
	}
	if r := run(t, env(), "seal", "--key", a.key("p1"), "--declaration", decl, "--out", out); r.code != cli.ExitTool || r.stderr != "error file opening\n" {
		t.Fatalf("a second seal into the same directory: %v", r)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(after, b) {
		t.Fatal("a second seal replaced the opening")
	}
	if r := run(t, env(), "seal", "--key", a.key("p2"), "--declaration", decl, "--out", t.TempDir()); r.code != cli.ExitTool || r.stderr != "error seal\n" {
		t.Fatalf("another key's declaration: %v", r)
	}
}

// proposedAccord is an Accord none of whose parties has accepted yet: no acceptances.json beside the
// founding set and no view or acceptance in any party's state directory, so its window starts it
// proposed.
func proposedAccord(t *testing.T, parties ...string) *accord {
	t.Helper()
	a := newAccord(t, parties...)
	if err := os.Remove(a.acceptances()); err != nil {
		t.Fatal(err)
	}
	for _, p := range parties {
		if err := os.RemoveAll(a.key(p) + ".state"); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// TestAcceptSignsTheKeptView is U11 and U12 at the command line (setup-contract-14,
// setup-contract-15): on a proposed Accord, accept with no view kept is error view; view prints the
// view and keeps it as view.json; accept signs it, prints the acceptance, whose view is the kept
// view's typed hash and whose signature verifies under the acceptance's type URI, keeps it as
// acceptance.json and says the Accord is not founded yet; a give then is door not-founded; once the
// last party accepts, the Accord is founded, acceptances.json is beside the founding set, and a
// give passes.
func TestAcceptSignsTheKeptView(t *testing.T) {
	t.Parallel()
	a := proposedAccord(t, "p1", "p2")
	w := a.start(t)
	if r := run(t, env(), a.as("p1", "accept")...); r.code != cli.ExitTool || r.stderr != "error view\n" {
		t.Fatalf("accept with no view: %v", r)
	}
	v := must(t, a.as("p1", "view")...)
	kept, err := os.ReadFile(filepath.Join(a.state("p1"), "view.json"))
	if err != nil || string(kept) != v.stdout {
		t.Fatalf("view.json: %q, %v; view printed %q", kept, err, v.stdout)
	}
	h, _ := canonical.TypedHash(bytes.TrimSuffix(kept, []byte("\n")))
	if l := v.lines(); l[len(l)-1] != "view "+h {
		t.Fatalf("view: %v", v)
	}
	r := must(t, a.as("p1", "accept")...)
	acc := []byte(strings.TrimSuffix(r.stdout, "\n"))
	if memberOf(t, acc, "view") != h || memberOf(t, acc, "accord") != a.id || !signing.Verify(acc, signing.TypeAcceptance, a.refs["p1"]) ||
		r.stderr != "accepted view "+h+"\n"+cli.LineNotYet+"\n" {
		t.Fatalf("accept: %v", r)
	}
	if b, err := os.ReadFile(filepath.Join(a.state("p1"), "acceptance.json")); err != nil || string(b) != r.stdout {
		t.Fatalf("acceptance.json: %q, %v", b, err)
	}
	if g := run(t, env(), a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "early")...); g.code != cli.ExitTool ||
		g.stderr != "door not-founded\n" {
		t.Fatalf("a give before every party accepted: %v", g)
	}
	must(t, a.as("p2", "view")...)
	if r := must(t, a.as("p2", "accept", "--json")...); !strings.Contains(r.stderr, cli.LineFounded) || !strings.Contains(r.stdout, `"founded":true`) {
		t.Fatalf("the last acceptance: %v", r)
	}
	if _, err := os.Stat(a.acceptances()); err != nil {
		t.Fatalf("acceptances.json: %v", err)
	}
	if !strings.Contains(w.stderr.String(), "accord "+a.id+" proposed\n") || !strings.Contains(w.stderr.String(), "accord "+a.id+" founded\n") {
		t.Fatalf("the window's lines: %s", w.stderr.String())
	}
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "now")...)
}

// memberOf is a string member of a JSON object.
func memberOf(t *testing.T, b []byte, name string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	s, _ := m[name].(string)
	return s
}

// TestWindowReadsOpeningsOnlyInItsDirectory starts a window whose opening is a FIFO, and one whose
// set names an actor that is a path out of --openings, with a FIFO there: the first is error file
// opening and the second is refused by founding, each at once, with no read that waits.
func TestWindowReadsOpeningsOnlyInItsDirectory(t *testing.T) {
	t.Parallel()
	start := func(a *accord, set string) result {
		t.Helper()
		done := make(chan result, 1)
		go func() {
			done <- run(t, env(), "window", "--key", a.key("mediator"), "--founding", set, "--openings", a.openings(),
				"--socket", a.socket(), "--journal", a.journal())
		}()
		select {
		case r := <-done:
			return r
		case <-time.After(10 * time.Second):
			t.Fatal("the window waits on a read")
			return result{}
		}
	}

	a := newAccord(t, "proposer", "applier")
	fifo := filepath.Join(a.openings(), cli.OpeningName(a.refs["applier"]))
	if err := os.Remove(fifo); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := start(a, a.founding()); r.code != cli.ExitTool || r.stderr != "error file opening\n" {
		t.Fatalf("an opening that is a FIFO: %v", r)
	}

	b := newAccord(t, "proposer", "applier")
	set, err := os.ReadFile(b.founding())
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(b.dir, "x"+cli.OpeningSuffix), 0o600); err != nil {
		t.Fatal(err)
	}
	escaped := filepath.Join(b.dir, "escaped.json")
	if err := os.WriteFile(escaped, bytes.Replace(set, []byte(`"actor":"`+b.refs["applier"]+`","seal"`), []byte(`"actor":"../x","seal"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := start(b, escaped); r.code != cli.ExitTool || !strings.HasPrefix(r.stderr, "error founding ") {
		t.Fatalf("an actor that is a path out of --openings: %v", r)
	}
}
