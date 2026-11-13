package cli_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
)

// bin is the placket binary, built once for the tests that need processes of their own, with the
// race detector when the tests run under it.
var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "placket-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin = filepath.Join(dir, "placket")
	args := []string{"build", "-o", bin}
	if raceBuild {
		args = append(args, "-race")
	}
	cmd := exec.Command("go", append(args, "github.com/orilloai/placket/cmd/placket")...)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the binary: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// env is the world a command runs in: the system's clock and randomness, and a wait that polls
// quickly.
func env() cli.Env {
	return cli.Env{Now: time.Now, Rand: rand.Reader, Poll: 20 * time.Millisecond}
}

// A result is what one command printed and its exit code.
type result struct {
	stdout, stderr string
	code           int
}

func (r result) String() string {
	return fmt.Sprintf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
}

// lines is what a command printed on standard error, one line each.
func (r result) lines() []string { return strings.Split(strings.TrimSuffix(r.stderr, "\n"), "\n") }

func run(t testing.TB, e cli.Env, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(e, args, strings.NewReader(""), &stdout, &stderr)
	return result{stdout.String(), stderr.String(), code}
}

// must runs a command that must exit 0, and returns what it printed.
func must(t testing.TB, args ...string) result {
	t.Helper()
	r := run(t, env(), args...)
	if r.code != 0 {
		t.Fatalf("%v: %v", args, r)
	}
	return r
}

// An accord is one Accord's files, made by the commands: keys, terms, declarations, bindings and the
// founding set, in a directory short enough for a unix socket.
type accord struct {
	dir, id, card string
	refs          map[string]string
	window        *window
}

func (a *accord) key(name string) string { return filepath.Join(a.dir, name+".key") }
func (a *accord) socket() string         { return filepath.Join(a.dir, "s") }
func (a *accord) journal() string        { return filepath.Join(a.dir, "j") }
func (a *accord) state(name string) string {
	return filepath.Join(a.dir, name+".key.state", strings.TrimPrefix(a.id, "sha256:"))
}

// as is the socket flags of a command run by name.
func (a *accord) as(name string, args ...string) []string {
	return append(append([]string(nil), args...), "--key", a.key(name), "--socket", a.socket(), "--accord", a.id)
}

// The terms of the worked example's parties, and of a note any peer gives any other.
var sixTerms = map[string]string{
	"proposer": `{"terms":[
		{"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal"]},"resolve":{"to":["role:decider"],"timeout":3600}},
		{"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","changed-version","comment"]}}]}`,
	"applier": `{"terms":[
		{"stance":"want","dir":"get","with":["role:proposer"],"select":{"kind":["proposal"]}},
		{"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["changed-version"]}}]}`,
	"decider": `{"terms":[
		{"stance":"want","dir":"give","with":["role:proposer"],"select":{"kind":["answer","changed-version","comment"]}},
		{"stance":"want","dir":"give","with":["role:applier"],"select":{"kind":["changed-version"]}}]}`,
}

const peerTerms = `{"terms":[{"stance":"want","dir":"give","with":["role:peer"],"select":{"kind":["note"]}},
	{"stance":"want","dir":"get","with":["role:peer"],"select":{"kind":["note"]}}]}`

// newAccord makes an Accord of the parties, each holding the role of its own name, or peer for a
// party with no terms of its own; mediator, operator and principal keys besides. A party's terms
// are the worked example's or those of the comment tests (commentTerms).
func newAccord(t testing.TB, parties ...string) *accord {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "plk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	a := &accord{dir: d, refs: map[string]string{}}
	for _, k := range append([]string{"mediator", "operator", "principal", "stranger"}, parties...) {
		a.refs[k] = strings.TrimSpace(must(t, "key", "new", "--out", a.key(k)).stdout)
	}
	write := func(name, s string) string {
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	var roles []string
	args := []string{"founding"}
	for _, p := range parties {
		terms, role := sixTerms[p], p
		if more, ok := commentTerms[p]; ok {
			terms = more
		}
		if terms == "" {
			terms, role = peerTerms, "peer"
		}
		if !contains(roles, role) {
			roles = append(roles, role)
		}
		decl := must(t, "declare", "--key", a.key(p), "--terms", write(p+".terms", terms)).stdout
		bind := must(t, "bind", "--key", a.key("principal"), "--actor", a.refs[p], "--role", role, "--mediator", a.refs["mediator"], "--until", "2099-01-01T00:00:00Z").stdout
		args = append(args, "--declaration", write(p+".declaration", decl), "--binding", write(p+".binding", bind))
	}
	rb, _ := json.Marshal(roles)
	args = append(args,
		"--policy", write("policy.json", `{"roles":`+string(rb)+`,"principals":["`+a.refs["principal"]+`"]}`),
		"--mediator", write("mediator.json", `{"actor":"`+a.refs["mediator"]+`","operator":"`+a.refs["operator"]+`","runs":"process"}`),
		"--card", write("card.json", `{"spec":"0.1","kernel":{"version":"0.1.0","hash":"sha256:`+strings.Repeat("0", 64)+`"},"judges":[],"checks":[]}`))
	r := must(t, args...)
	write("founding.json", r.stdout)
	for _, l := range r.lines() {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "accord" {
			a.id = f[1]
		} else if len(f) == 2 && f[0] == "card" {
			a.card = f[1]
		}
	}
	return a
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// A window run in this process by the window command.
type window struct {
	cancel context.CancelFunc
	done   chan int
	stderr *syncBuffer
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// start runs the Accord's window until the test ends or stop is called.
func (a *accord) start(t testing.TB) *window {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	e := env()
	e.Context, e.Ready = ctx, ready
	w := &window{cancel: cancel, done: make(chan int, 1), stderr: &syncBuffer{}}
	go func() {
		w.done <- cli.Run(e, []string{"window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"),
			"--socket", a.socket(), "--journal", a.journal()}, strings.NewReader(""), &bytes.Buffer{}, w.stderr)
	}()
	select {
	case <-ready:
	case code := <-w.done:
		t.Fatalf("the window exited %d: %s", code, w.stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	a.window = w
	t.Cleanup(func() { w.stop(t) })
	return w
}

func (w *window) stop(t testing.TB) {
	if w.cancel == nil {
		return
	}
	w.cancel()
	w.cancel = nil
	select {
	case code := <-w.done:
		if code != 0 {
			t.Errorf("the window exited %d: %s", code, w.stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Error("the window did not stop")
	}
}

// question waits for the first open question the key may answer and returns its id.
func (a *accord) question(t testing.TB, who string) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		r := run(t, env(), a.as(who, "questions")...)
		if r.code != 0 {
			t.Fatalf("questions: %v", r)
		}
		if l := strings.SplitN(r.stdout, "\n", 2)[0]; l != "" {
			var q struct{ ID string }
			if err := json.Unmarshal([]byte(l), &q); err != nil {
				t.Fatal(err)
			}
			return q.ID
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no question came")
	return ""
}

// waiting is a give --wait running in the background.
type waiting chan result

// ask gives a proposal with --keep and --wait, in the background, and returns its question.
func (a *accord) ask(t testing.TB, text, topic string, more ...string) (string, waiting) {
	t.Helper()
	w := make(waiting, 1)
	args := a.as("proposer", append([]string{"give", "--to", a.refs["applier"], "--kind", "proposal", "--text", text, "--topic", topic, "--keep", "--wait"}, more...)...)
	go func() { w <- run(t, env(), args...) }()
	return a.question(t, "decider"), w
}

func (w waiting) end(t testing.TB) result {
	t.Helper()
	select {
	case r := <-w:
		return r
	case <-time.After(20 * time.Second):
		t.Fatal("the wait did not end")
	}
	return result{}
}

// drain fetches, with --keep, every delivery queued for the key.
func (a *accord) drain(t testing.TB, who string) []result {
	t.Helper()
	var out []result
	for {
		r := must(t, a.as(who, "get", "--keep")...)
		if r.stderr == "no delivery\n" {
			return out
		}
		out = append(out, r)
	}
}

// A sixRun is the worked example played in process: what each step printed, and the questions.
type sixRun struct {
	*accord
	accepted, changed, declined, open string
	steps                             map[string]result
}

// six plays the worked example's steps 1 to 9: an accept, a change with a comment, a decline with a
// comment, a proposal left open, and a restart, after which every actor fetches what is queued.
func six(t testing.TB) *sixRun {
	t.Helper()
	s := &sixRun{accord: newAccord(t, "proposer", "applier", "decider"), steps: map[string]result{}}
	s.start(t)
	var w waiting
	s.accepted, w = s.ask(t, "Rename the column total to amount.", "billing")
	s.steps["answer"] = must(t, s.as("decider", "answer", "--question", s.accepted)...)
	s.steps["accepted"] = w.end(t)
	s.steps["applier get 1"] = must(t, s.as("applier", "get", "--keep")...)
	s.changed, w = s.ask(t, "Drop the column notes.", "billing")
	s.steps["change"] = must(t, s.as("decider", "change", "--question", s.changed, "--text", "Keep notes, hidden.", "--comment", "Someone reads it.", "--keep")...)
	s.steps["changed"] = w.end(t)
	s.declined, w = s.ask(t, "Delete the table invoices.", "billing")
	s.steps["decline"] = must(t, s.as("decider", "decline", "--question", s.declined, "--comment", "Kept seven years.")...)
	s.steps["declined"] = w.end(t)
	r := run(t, env(), s.as("proposer", "give", "--to", s.refs["applier"], "--kind", "proposal", "--text", "Add an index.", "--topic", "billing", "--keep")...)
	if r.code != 4 {
		t.Fatalf("the open proposal: %v", r)
	}
	s.open = strings.TrimSpace(r.stdout[strings.LastIndex(strings.TrimSuffix(r.stdout, "\n"), "\n")+1:])
	s.window.stop(t)
	s.start(t)
	for _, who := range []string{"applier", "proposer", "decider"} {
		s.drain(t, who)
	}
	return s
}
