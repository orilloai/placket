package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/receipts"
)

// TestAGiveWhoseReplyWasLost gives a note that the window journals, and then puts the state
// directory back as it is when the reply never came: the seq file still says nothing was taken, and
// the envelope line stays. The next give takes the seq after the envelope's, passes, and the seq
// file catches up; reconcile finds nothing missing (crossing-4, crossing-5).
func TestAGiveWhoseReplyWasLost(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	dir := a.state("p1")
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "one")...)
	if err := os.Remove(filepath.Join(dir, "seq")); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "two")...)
	if r.code != 0 || !strings.HasPrefix(r.stderr, "pass ok sha256:") {
		t.Fatalf("the give after a lost reply: %v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "seq")); string(b) != "2\n" {
		t.Fatalf("seq %q", b)
	}
	envs, _ := os.ReadFile(filepath.Join(dir, "envelopes.jsonl"))
	if l := strings.Split(strings.TrimSuffix(string(envs), "\n"), "\n"); len(l) != 2 || !strings.HasSuffix(l[0], `"seq":1}`) || !strings.HasSuffix(l[1], `"seq":2}`) {
		t.Fatalf("envelopes.jsonl: %q", envs)
	}
	// A third give, with the seq file two behind: the envelope lines still lead.
	if err := os.WriteFile(filepath.Join(dir, "seq"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "three")...)
	if b, _ := os.ReadFile(filepath.Join(dir, "seq")); string(b) != "3\n" {
		t.Fatalf("seq %q", b)
	}
	r = must(t, "reconcile", "--mediator", a.refs["mediator"], "--accord", a.id, "--card", a.card, "--receipts", a.journal(), "--kept", dir)
	if r.stdout != `{"missing":[],"removed":[],"unrecorded":[]}`+"\n" || r.stderr != "nothing to reconcile\n" {
		t.Fatalf("reconcile: %v", r)
	}
}

// TestAJournalThatIsNotThereOrCut points verify, log and reconcile at a directory with no
// receipts.jsonl: each is tool error journal, naming the file, and prints nothing else. A journal
// founded with no receipt yet verifies as zero receipts. A receipts.jsonl whose last line has no line
// feed is read up to that line, and each command says on standard error that the file was cut there.
func TestAJournalThatIsNotThereOrCut(t *testing.T) {
	t.Parallel()
	s := six(t)
	empty := t.TempDir()
	commands := func(dir string) [][]string {
		return [][]string{
			{"verify", "--mediator", s.refs["mediator"], "--receipts", dir},
			{"log", "--mediator", s.refs["mediator"], "--receipts", dir, "--format", "csv"},
			{"reconcile", "--mediator", s.refs["mediator"], "--accord", s.id, "--card", s.card, "--receipts", dir, "--kept", s.state("proposer")},
		}
	}
	for _, args := range commands(empty) {
		if r := run(t, env(), args...); r.code != 3 || r.stdout != "" || r.stderr != "error journal receipts.jsonl\n" {
			t.Fatalf("%s of a directory with no journal: %v", args[0], r)
		}
	}

	fresh := newAccord(t, "p1", "p2")
	fresh.start(t)
	fresh.window.stop(t)
	if r := must(t, "verify", "--mediator", fresh.refs["mediator"], "--receipts", fresh.journal()); !strings.HasPrefix(r.stderr, "verified 0 receipts\n") {
		t.Fatalf("verify of a journal with no receipt yet: %v", r)
	}

	cut := copyDir(t, s.journal())
	p := filepath.Join(cut, "receipts.jsonl")
	whole, _ := os.ReadFile(p)
	n := bytes.Count(whole, []byte{'\n'})
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"payload":"cut`)
	_ = f.Close()
	note := fmt.Sprintf("journal: receipts.jsonl line %d has no line feed; the file was cut there, and the line is left out", n+1)
	for i, args := range commands(cut) {
		want := must(t, commands(s.journal())[i]...)
		r := must(t, args...)
		if r.stdout != want.stdout || r.stderr != note+"\n"+want.stderr {
			t.Fatalf("%s of a cut journal:\n%v\nwanted:\n%v", args[0], r, want)
		}
	}
	if r := must(t, "verify", "--mediator", s.refs["mediator"], "--receipts", cut); r.lines()[1] != fmt.Sprintf("verified %d receipts", n) {
		t.Fatalf("verify of a cut journal: %v", r)
	}
}

// TestVerifyFoundingNeedsItsMediator signs the worked example's first receipt again with a key that
// is not the founding set's mediator, under the same Accord and card: the receipt verifies under
// that key alone, and verify --founding with the Accord's founding set is tool error founding, exit
// 3, and prints no count, no operator and no claim.
func TestVerifyFoundingNeedsItsMediator(t *testing.T) {
	t.Parallel()
	s := six(t)
	b, _ := os.ReadFile(filepath.Join(s.journal(), "receipts.jsonl"))
	first, err := receipts.Decode([]byte(strings.SplitN(string(b), "\n", 2)[0]))
	if err != nil {
		t.Fatal(err)
	}
	k, err := keyfile.Read(s.key("stranger"))
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := receipts.Build(first.Subject, first.Predicate, k)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), append(again, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := must(t, "verify", "--mediator", s.refs["stranger"], "--receipts", dir); r.lines()[1] != "mediator "+s.refs["stranger"]+" accord "+s.id+" card "+s.card {
		t.Fatalf("verify under the other key: %v", r)
	}
	r := run(t, env(), "verify", "--mediator", s.refs["stranger"], "--receipts", dir, "--founding", filepath.Join(s.dir, "founding.json"))
	if r.code != 3 || r.stdout != "" || r.stderr != "error founding\n" {
		t.Fatalf("verify --founding under a key that is not the set's mediator: %v", r)
	}
	if r := must(t, "verify", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--founding", filepath.Join(s.dir, "founding.json")); r.lines()[2] != "operator "+s.refs["operator"]+" runs process" {
		t.Fatalf("verify --founding under the set's mediator: %v", r)
	}
}
