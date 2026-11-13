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
	decls, _ := filepath.Glob(filepath.Join(a.dir, "*.declaration"))
	for _, d := range decls {
		args = append(args, "--declaration", d, "--binding", strings.TrimSuffix(d, ".declaration")+".binding")
	}
	r := must(t, args...)
	founding = filepath.Join(a.dir, "founding2.json")
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
		done <- cli.Run(e, []string{"window", "--key", a.key("mediator"), "--founding", founding, "--socket", socket, "--journal", journal},
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
	return id, founding, socket, journal
}

// TestVerifyHoldsOneAccord joins the journals of two Accords under one mediator key, each of which
// verifies alone, into one receipts.jsonl: verify and log each refuse it with tool error verify,
// code accord, at the first receipt of the second Accord, and print nothing else. verify --founding
// with the second Accord's founding set, under the same mediator, over the first Accord's journal is
// tool error founding.
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
	for _, args := range [][]string{
		{"verify", "--mediator", a.refs["mediator"], "--receipts", both},
		{"log", "--mediator", a.refs["mediator"], "--receipts", both, "--format", "csv"},
	} {
		if r := run(t, env(), args...); r.code != 3 || r.stdout != "" || r.stderr != want {
			t.Fatalf("%s of two Accords: %v", args[0], r)
		}
	}
	r := run(t, env(), "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", founding)
	if r.code != 3 || r.stdout != "" || r.stderr != "error founding\n" {
		t.Fatalf("verify --founding with another Accord's set: %v", r)
	}
}

// TestReconcilePrintsAnActorSafely reconciles a state directory handed over with an actor file that
// holds terminal control sequences: the line on standard error shows the actor as a field, escaped,
// and standard output, a terminal, stays the same JSON with every rune a terminal could act on
// escaped.
func TestReconcilePrintsAnActorSafely(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "one")...)
	dir := copyDir(t, a.state("p1"))
	if err := os.WriteFile(filepath.Join(dir, "actor"), []byte("ed25519:\x1b]52;c;aGk=\x07\u009b2J\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, terminal(), "reconcile", "--mediator", a.refs["mediator"], "--accord", a.id, "--card", a.card, "--receipts", a.journal(), "--kept", dir)
	if r.code != 0 || r.stderr != `missing ed25519:\x1b]52;c;aGk=\x07\u009b2J seq 1`+"\n" ||
		r.stdout != `{"missing":[{"actor":"ed25519:\u001b]52;c;aGk=\u0007\u009b2J","seq":1}],"removed":[],"unrecorded":[]}`+"\n" {
		t.Fatalf("reconcile: %v", r)
	}
}
