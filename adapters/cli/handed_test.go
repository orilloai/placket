package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAHandedFileThatIsNotRegularIsFile points verify, log and reconcile at a journal whose
// receipts.jsonl is a directory, and released and log at a state directory whose actor file is one:
// each is tool error file, naming the file, and prints nothing else.
func TestAHandedFileThatIsNotRegularIsFile(t *testing.T) {
	t.Parallel()
	s := six(t)
	journal := t.TempDir()
	if err := os.Mkdir(filepath.Join(journal, "receipts.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(stateDir, "actor"), 0o700); err != nil {
		t.Fatal(err)
	}
	crossed := writeFile(t, t.TempDir(), "crossed", []byte("crossed"))
	for _, c := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"verify", "--mediator", s.refs["mediator"], "--receipts", journal}, "error file receipts.jsonl\n"},
		{[]string{"log", "--mediator", s.refs["mediator"], "--receipts", journal, "--format", "csv"}, "error file receipts.jsonl\n"},
		{[]string{"reconcile", "--mediator", s.refs["mediator"], "--accord", s.id, "--card", s.card, "--receipts", journal, "--kept", s.state("proposer")}, "error file receipts.jsonl\n"},
		{[]string{"released", "--file", crossed, "--state", stateDir}, "error file actor\n"},
		{[]string{"log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--format", "csv", "--kept", stateDir}, "error file actor\n"},
	} {
		if r := run(t, env(), c.args...); r.code != 3 || r.stdout != "" || r.stderr != c.stderr {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
}

// TestAStateFileOverItsCapIsTooLarge gives a key a state directory whose actor file is 4 KiB and a
// byte, and runs get, then one whose seq file is, and runs give, which reads it: each is tool error
// state, naming the file, too-large, not error file, since the file is regular and only too long.
// Neither reaches the window.
func TestAStateFileOverItsCapIsTooLarge(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	over := strings.Repeat("1", 4<<10) + "\n"
	for _, c := range []struct {
		who, file, body string
		args            []string
		stderr          string
	}{
		{"p1", "actor", over, a.as("p1", "get"), "error state actor too-large\n"},
		{"p2", "seq", over, a.as("p2", "give", "--to", a.refs["p1"], "--kind", "note", "--text", "a note"), "error state seq too-large\n"},
	} {
		dir := a.state(c.who)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "actor"), []byte(a.refs[c.who]+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, c.file), []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := run(t, env(), c.args...); r.code != 3 || r.stdout != "" || r.stderr != c.stderr {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
}
