package cli_test

import (
	"os"
	"path/filepath"
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
