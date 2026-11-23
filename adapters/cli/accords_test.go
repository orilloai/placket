package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
)

// another founds a second Accord of the same parties and keys under the same mediator, with the
// stranger as its operator, runs its window until the test ends, and returns its id, its founding
// set's path, its socket and its journal.
func (a *accord) another(t testing.TB) (id, founding, socket, journal string) {
	t.Helper()
	med := filepath.Join(a.dir, "mediator2.json")
	if err := os.WriteFile(med, []byte(`{"actor":"`+a.refs["mediator"]+`","operator":"`+a.refs["stranger"]+`","runs":"process"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"founding", "--policy", filepath.Join(a.dir, "policy.json"), "--mediator", med, "--card", filepath.Join(a.dir, "card.json")}
	seals, _ := filepath.Glob(filepath.Join(a.dir, "*.seal"))
	for _, d := range seals {
		args = append(args, "--seal", d, "--binding", strings.TrimSuffix(d, ".seal")+".binding")
	}
	r := must(t, args...)
	// Its own directory, so its acceptances.json is its own.
	if err := os.Mkdir(filepath.Join(a.dir, "2"), 0o700); err != nil {
		t.Fatal(err)
	}
	founding = filepath.Join(a.dir, "2", "founding.json")
	if err := os.WriteFile(founding, []byte(r.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, l := range r.lines() {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "accord" {
			id = f[1]
		}
	}
	if id == "" || id == a.id {
		t.Fatalf("the second founding: %v", r)
	}
	socket, journal = filepath.Join(a.dir, "s2"), filepath.Join(a.dir, "j2")
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan int, 1)
	e := env()
	e.Context, e.Ready = ctx, ready
	stderr := &syncBuffer{}
	go func() {
		done <- cli.Run(e, []string{"window", "--key", a.key("mediator"), "--founding", founding, "--openings", a.openings(), "--socket", socket, "--journal", journal},
			strings.NewReader(""), &bytes.Buffer{}, stderr)
	}()
	select {
	case <-ready:
	case code := <-done:
		t.Fatalf("the second window exited %d: %s", code, stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("the second window did not start")
	}
	t.Cleanup(func() { cancel(); <-done })
	a.acceptAllOn(t, socket, id)
	return id, founding, socket, journal
}

// TestVerifyHoldsOneAccord joins the journals of two Accords under one mediator key, each of which
// verifies alone, into one receipts.jsonl: verify and log each refuse it with error verify, code
// accord, at the first receipt of the second Accord, and print nothing else. verify exits 1, since
// the log fails, and log exits 3, the tool error of a log it cannot report on. verify --founding with
// the second Accord's founding set, under the same mediator, over the first Accord's journal is tool
// error founding.
func TestVerifyHoldsOneAccord(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "one")...)
	id, founding, socket, journal := a.another(t)
	must(t, "give", "--to", a.refs["p1"], "--kind", "note", "--text", "two", "--key", a.key("p2"), "--socket", socket, "--accord", id)
	second, err := os.ReadFile(filepath.Join(journal, "receipts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{a.journal(), journal} {
		must(t, "verify", "--mediator", a.refs["mediator"], "--receipts", dir)
	}
	first := a.receipts(t)
	both := t.TempDir()
	if err := os.WriteFile(filepath.Join(both, "receipts.jsonl"), append(append([]byte(nil), first...), second...), 0o600); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("error verify accord receipts.jsonl line %d\n", bytes.Count(first, []byte{'\n'})+1)
	for code, args := range map[int][]string{
		cli.ExitBlock: {"verify", "--mediator", a.refs["mediator"], "--receipts", both},
		cli.ExitTool:  {"log", "--mediator", a.refs["mediator"], "--receipts", both, "--format", "csv"},
	} {
		if r := run(t, env(), args...); r.code != code || r.stdout != "" || r.stderr != want {
			t.Fatalf("%s of two Accords: %v", args[0], r)
		}
	}
	r := run(t, env(), "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", founding)
	if r.code != 3 || r.stdout != "" || r.stderr != "error founding\n" {
		t.Fatalf("verify --founding with another Accord's set: %v", r)
	}
}

// TestReconcilePrintsAnActorSafely reconciles a state directory handed over with an actor file that
// holds terminal control sequences: it is not an actor reference, so reconciliation refuses it as
// kept, naming the --kept it came from and its actor file, and nothing of what the file holds
// reaches either stream.
func TestReconcilePrintsAnActorSafely(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "one")...)
	dir := copyDir(t, a.state("p1"))
	if err := os.WriteFile(filepath.Join(dir, "actor"), []byte("ed25519:\x1b]52;c;aGk=\x07\u009b2J\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, terminal(), "reconcile", "--mediator", a.refs["mediator"], "--accord", a.id, "--card", a.card, "--receipts", a.journal(),
		"--kept", copyDir(t, a.state("p1")), "--kept", dir)
	if r.code != 3 || r.stderr != "error verify kept --kept 2 actor\n" || r.stdout != "" {
		t.Fatalf("reconcile: %v", r)
	}
}
