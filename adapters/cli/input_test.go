package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/composition"
)

// notUTF8 is text with a byte that is not UTF-8 between two letters.
const notUTF8 = "a\xffb"

// TestTextThatIsNotUTF8IsRefused gives, answers and changes with text holding a byte that is not
// UTF-8: each is tool error text, exits 3 and sends nothing, so the question stays open; nothing is
// repaired into other text (parse-strict).
func TestTextThatIsNotUTF8IsRefused(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	before := string(a.receipts(t))
	r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "proposal", "--text", notUTF8)...)
	if r.code != 3 || r.stdout != "" || r.stderr != "error text\n" {
		t.Fatalf("give: %v", r)
	}
	if string(a.receipts(t)) != before {
		t.Fatal("give sent a call")
	}
	q := a.openOne(t, "Rename total.")
	before = string(a.receipts(t))
	for _, args := range [][]string{
		{"answer", "--question", q, "--text", notUTF8},
		{"change", "--question", q, "--text", notUTF8},
	} {
		r := run(t, env(), a.as("decider", args...)...)
		if r.code != 3 || r.stdout != "" || r.stderr != "error text\n" {
			t.Fatalf("%s: %v", args[0], r)
		}
	}
	if string(a.receipts(t)) != before {
		t.Fatal("an answer sent a call")
	}
	must(t, a.as("decider", "answer", "--question", q)...)
}

// TestBindRefusesFlagsThatAreNotUTF8 binds with an actor, a role or a mediator holding a byte that
// is not UTF-8: each is tool error usage and prints no binding.
func TestBindRefusesFlagsThatAreNotUTF8(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	policy := filepath.Join(a.dir, "policy.json")
	flags := map[string]string{"--actor": a.refs["p1"], "--role": "peer", "--mediator": a.refs["mediator"]}
	for _, bad := range []string{"--actor", "--role", "--mediator"} {
		args := []string{"bind", "--key", a.key("principal"), "--until", "2099-01-01T00:00:00Z", "--policy", policy}
		for f, v := range flags {
			if f == bad {
				v = notUTF8
			}
			args = append(args, f, v)
		}
		r := run(t, env(), args...)
		if r.code != 3 || r.stdout != "" || r.stderr != "error usage\n" {
			t.Fatalf("bind with %s not UTF-8: %v", bad, r)
		}
	}
}

// TestGiveRefusesAsWithText gives text on the command line with --as raw or data: tool error
// usage, since --as names how a file is read.
func TestGiveRefusesAsWithText(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	for _, as := range []string{"raw", "data"} {
		r := run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--as", as, "--text", "x")...)
		if r.code != 3 || r.stdout != "" || r.stderr != "error usage\n" {
			t.Fatalf("give --as %s --text: %v", as, r)
		}
	}
	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--as", "text", "--text", "x")...)
}

// TestAnAccordThatIsNotAnIDBuildsNoPath calls with --accord values that are not an Accord id, among
// them paths that would leave the key's state directory: each is tool error
// usage, and no directory is made, under the key's state directory or beside it.
func TestAnAccordThatIsNotAnIDBuildsNoPath(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	// The state directory of ../../<name> would be beside the Accord's directory.
	outside := filepath.Base(a.dir) + "-outside"
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(filepath.Dir(a.dir), outside)) })
	for _, accord := range []string{"../../" + outside, "..", "x", "sha256:" + strings.Repeat("A", 64), "sha256:" + strings.Repeat("g", 64), "sha256:" + strings.Repeat("a", 63) + "z", "sha256:" + strings.Repeat("a", 63), strings.Repeat("a", 64)} {
		r := run(t, env(), "get", "--key", a.key("ann"), "--socket", a.socket(), "--accord", accord)
		if r.code != 3 || r.stdout != "" || r.stderr != "usage: placket: --accord is not an Accord id\nerror usage\n" {
			t.Fatalf("--accord %q: %v", accord, r)
		}
	}
	for _, p := range []string{a.key("ann") + ".state", filepath.Join(filepath.Dir(a.dir), outside)} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("%s was made: %v", p, err)
		}
	}
}

// TestTheWindowEscapesAnUncheckedKernel starts a window on a founding set whose card names a
// kernel version holding an escape sequence: the line that names another module prints it escaped,
// so it never reaches the operator's terminal as a control.
func TestTheWindowEscapesAnUncheckedKernel(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	b, err := os.ReadFile(filepath.Join(a.dir, "founding.json"))
	if err != nil {
		t.Fatal(err)
	}
	set := strings.Replace(string(b), `"version":"0.1.0"`, `"version":"0.1.0\u001b[2J"`, 1)
	if set == string(b) {
		t.Fatal("the founding set names no kernel version")
	}
	path := filepath.Join(a.dir, "escaped.json")
	if err := os.WriteFile(path, []byte(set), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), "window", "--key", a.key("mediator"), "--founding", path, "--socket", a.socket(), "--journal", a.journal())
	want := otherLine(`0.1.0\x1b[2J`, composition.KernelHash()) + "\nerror founding kernel\n"
	if r.code != 3 || r.stdout != "" || r.stderr != want {
		t.Fatalf("window: %v\nwant stderr %q", r, want)
	}
}

// TestFoundingNamesTheFirstUnreadableFile founds with the policy and the card both unreadable, many
// times: it always names the policy, the first file it reads (check-order).
func TestFoundingNamesTheFirstUnreadableFile(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	none := filepath.Join(a.dir, "none.json")
	for i := 0; i < 30; i++ {
		r := run(t, env(), "founding", "--policy", none, "--mediator", filepath.Join(a.dir, "mediator.json"), "--card", none,
			"--declaration", none, "--binding", none)
		if r.code != 3 || r.stdout != "" || r.stderr != "error file policy\n" {
			t.Fatalf("run %d: %v", i, r)
		}
	}
	r := run(t, env(), "founding", "--policy", filepath.Join(a.dir, "policy.json"), "--mediator", filepath.Join(a.dir, "mediator.json"),
		"--card", filepath.Join(a.dir, "card.json"), "--declaration", none, "--binding", none)
	if r.code != 3 || r.stderr != "error file declarations\n" {
		t.Fatalf("declarations and bindings unreadable: %v", r)
	}
}

// TestVerifyAFoundingSetWithNoReceipts verifies a journal whose window founded the Accord and wrote
// no receipt, with its founding set: the set is checked alone, its lines are printed, and the
// command exits 0.
func TestVerifyAFoundingSetWithNoReceipts(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t).stop(t)
	r := run(t, env(), "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", filepath.Join(a.dir, "founding.json"))
	l := r.lines()
	if r.code != 0 || r.stdout != "" || len(l) < 3 || l[0] != "verified 0 receipts" || l[2] != "operator "+a.refs["operator"]+" runs process" {
		t.Fatalf("verify: %v", r)
	}
	r = run(t, env(), "verify", "--mediator", a.refs["p1"], "--receipts", a.journal(), "--founding", filepath.Join(a.dir, "founding.json"))
	if r.code != 3 || r.stderr != "error founding\n" {
		t.Fatalf("verify under another mediator: %v", r)
	}
}
