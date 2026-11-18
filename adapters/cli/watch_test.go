package cli_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
)

// A proc is a command that runs until it ends or is stopped, in this process: its output as it
// comes, and its exit code once it ends.
type proc struct {
	cancel         context.CancelFunc
	done           chan int
	stdout, stderr *syncBuffer
	code           int
	ended          bool
}

// background runs a command with stdin until it ends or the test does.
func background(t testing.TB, e cli.Env, stdin io.Reader, args ...string) *proc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	e.Context = ctx
	p := &proc{cancel: cancel, done: make(chan int, 1), stdout: &syncBuffer{}, stderr: &syncBuffer{}}
	go func() { p.done <- cli.Run(e, args, stdin, p.stdout, p.stderr) }()
	t.Cleanup(func() { p.stop(t) })
	return p
}

// startWith runs a window command with e and waits until it listens.
func startWith(t testing.TB, e cli.Env, args ...string) *proc {
	t.Helper()
	ready := make(chan struct{})
	e.Ready = ready
	p := background(t, e, strings.NewReader(""), args...)
	select {
	case <-ready:
	case code := <-p.done:
		t.Fatalf("the window exited %d: %s", code, p.stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	return p
}

// wait waits for the command to end, at most d, and returns its exit code.
func (p *proc) wait(t testing.TB, d time.Duration) int {
	t.Helper()
	if p.ended {
		return p.code
	}
	select {
	case p.code = <-p.done:
		p.ended = true
		return p.code
	case <-time.After(d):
		t.Fatalf("the command did not end; stdout %s; stderr %s", p.stdout.String(), p.stderr.String())
	}
	return 0
}

func (p *proc) stop(t testing.TB) {
	p.cancel()
	if !p.ended {
		p.wait(t, 10*time.Second)
	}
}

// until waits, at most 10 seconds, for the command's standard output to hold n lines.
func (p *proc) until(t testing.TB, n int) []string {
	t.Helper()
	for i := 0; i < 1000; i++ {
		if s := p.stdout.String(); strings.Count(s, "\n") >= n {
			return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %d lines came: %q", n, p.stdout.String())
	return nil
}

// TestWatchPrintsEachQuestionOnce watches the decider's label while two proposals are asked: it
// prints two blocks, each with the label, kind, asker and deadline and the lines that answer it, and
// with --count 2 then exits 0. The printed answer line runs as printed and closes its question; a
// question printed is not printed again. With --json each question is one object.
func TestWatchPrintsEachQuestionOnce(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	e, _ := labelsEnv(t, a.entry("decider", "approve"), a.entry("proposer", "merge"))
	w := background(t, e, strings.NewReader(""), "watch", "--label", "approve", "--count", "2")
	j := background(t, e, strings.NewReader(""), "watch", "--label", "approve", "--count", "2", "--json")
	time.Sleep(300 * time.Millisecond)
	var asked []string
	for _, text := range []string{"One.", "Two."} {
		g := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", text)
		if g.code != 4 {
			t.Fatalf("give: %v", g)
		}
		asked = append(asked, last(g.stdout))
		w.until(t, 9*len(asked))
	}
	if code := w.wait(t, 10*time.Second); code != 0 {
		t.Fatalf("watch exited %d: %s", code, w.stderr.String())
	}
	blocks := strings.Split(strings.TrimSuffix(w.stdout.String(), "\n\n"), "\n\n")
	if len(blocks) != 2 {
		t.Fatalf("watch printed %d blocks: %q", len(blocks), w.stdout.String())
	}
	for i, b := range blocks {
		l := strings.Split(b, "\n")
		at := "--label approve --question " + asked[i]
		want := []string{"question " + asked[i], "label approve", "kind proposal", "asker " + a.refs["proposer"]}
		if len(l) != 8 || strings.Join(l[:4], "\n") != strings.Join(want, "\n") || !strings.HasPrefix(l[4], "deadline 20") ||
			l[5] != "placket answer "+at || l[6] != "placket change "+at+" --text TEXT" || l[7] != "placket decline "+at+" --comment TEXT" {
			t.Fatalf("block %d: %q", i, b)
		}
		if i == 0 {
			if r := run(t, e, strings.Fields(l[5])[1:]...); r.code != 0 || !strings.HasPrefix(r.stderr, "pass ") {
				t.Fatalf("the printed answer line: %v", r)
			}
		}
	}
	if code := j.wait(t, 10*time.Second); code != 0 {
		t.Fatalf("watch --json exited %d", code)
	}
	lines := strings.Split(strings.TrimSuffix(j.stdout.String(), "\n"), "\n")
	if len(lines) != 3 || lines[2] != `{"exit":0}` {
		t.Fatalf("watch --json printed %q", lines)
	}
	for i, l := range lines[:2] {
		var o struct {
			Label, ID, Kind, Message string
			Commands                 map[string]string
		}
		if err := json.Unmarshal([]byte(l), &o); err != nil || o.Label != "approve" || o.ID != asked[i] || o.Kind != "proposal" ||
			o.Commands["answer"] != "placket answer --label approve --question "+asked[i] {
			t.Fatalf("object %d: %s, %v", i, l, err)
		}
	}
	// A watch started now prints only the question still open: the first was answered.
	again := background(t, e, strings.NewReader(""), "watch", "--label", "approve", "--count", "1")
	if code := again.wait(t, 10*time.Second); code != 0 || !strings.HasPrefix(again.stdout.String(), "question "+asked[1]+"\n") {
		t.Fatalf("a second watch: %d, %q", code, again.stdout.String())
	}
}
