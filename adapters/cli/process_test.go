package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestCommandsAtOnce runs two give processes for one actor at once, and eight actors calling at
// once, with the race detector: the two take consecutive seqs, no give is refused envelope-sequence,
// and every chain verifies (state-lock, one-mediator-goroutine). A give the mediator rejects leaves
// seq as it was and its envelope line removed; and a delivered.jsonl that ends in a line with no
// line feed loses that line, with one line on stderr, before the next fetched receipt is written on
// a line of its own.
func TestCommandsAtOnce(t *testing.T) {
	t.Parallel()
	peers := []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7", "p8"}
	a := newAccord(t, peers...)
	a.start(t)

	var procs []*exec.Cmd
	var outs []*bytes.Buffer
	for i := 0; i < 2; i++ {
		var b bytes.Buffer
		c := exec.Command(bin, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", fmt.Sprint("note ", i))...)
		c.Stdout, c.Stderr = &b, &b
		procs, outs = append(procs, c), append(outs, &b)
	}
	for _, c := range procs {
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for i, c := range procs {
		if err := c.Wait(); err != nil || !strings.HasPrefix(outs[i].String(), "pass ok ") {
			t.Fatalf("give process %d: %v\n%s", i, err, outs[i])
		}
	}
	if b, _ := os.ReadFile(filepath.Join(a.state("p1"), "seq")); string(b) != "2\n" {
		t.Fatalf("seq %q", b)
	}

	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for i, p := range peers {
		wg.Add(1)
		go func(p, to string) {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				if r := run(t, env(), a.as(p, "give", "--to", a.refs[to], "--kind", "note", "--text", "a note")...); r.code != 0 {
					errs <- p + ": " + r.String()
				}
				if r := run(t, env(), a.as(p, "get")...); r.code != 0 {
					errs <- p + ": " + r.String()
				}
			}
		}(p, peers[(i+1)%len(peers)])
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if r := must(t, "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal()); !strings.HasPrefix(r.stderr, "verified 26 receipts\n") {
		t.Fatalf("verify: %v", r)
	}

	seq, _ := os.ReadFile(filepath.Join(a.state("p1"), "seq"))
	envs, _ := os.ReadFile(filepath.Join(a.state("p1"), "envelopes.jsonl"))
	if r := run(t, env(), a.as("p1", "give", "--to", a.refs["stranger"], "--kind", "note", "--text", "x")...); r.code != 2 || r.stderr != "rejected receiver-unknown\n" {
		t.Fatalf("a rejected give: %v", r)
	}
	seq2, _ := os.ReadFile(filepath.Join(a.state("p1"), "seq"))
	envs2, _ := os.ReadFile(filepath.Join(a.state("p1"), "envelopes.jsonl"))
	if !bytes.Equal(seq, seq2) || !bytes.Equal(envs, envs2) {
		t.Fatalf("a rejected give changed seq %q to %q, or envelopes", seq, seq2)
	}

	must(t, a.as("p3", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "one more")...)
	p := filepath.Join(a.state("p2"), "delivered.jsonl")
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"payload":"torn`)
	_ = f.Close()
	before, _ := os.ReadFile(p)
	r := must(t, a.as("p2", "get")...)
	n := strings.Count(string(before), "\n")
	if r.lines()[0] != fmt.Sprintf("state: delivered.jsonl line %d had no line feed and was never written; removed", n+1) {
		t.Fatalf("get: %v", r)
	}
	after, _ := os.ReadFile(p)
	if !bytes.HasPrefix(after, before[:bytes.LastIndexByte(before, '\n')+1]) || strings.Count(string(after), "\n") != n+1 || !bytes.HasSuffix(after, []byte("}\n")) ||
		bytes.Contains(after, []byte("torn")) {
		t.Fatalf("delivered.jsonl: %q", after)
	}
}

// TestAWindowKilled kills a window process with SIGKILL while a question is open and a released
// proposal is not yet fetched, and starts it again: the question is closed by one receipt, an answer
// to it is no-open-question, the proposal is fetched once, and verify passes over the journal
// (restart-reproducible, LOG-13, LOG-18).
func TestAWindowKilled(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	var logs []*syncBuffer
	startProc := func() *exec.Cmd {
		b := &syncBuffer{}
		c := exec.Command(bin, "window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--socket", a.socket(), "--journal", a.journal())
		c.Stderr = b
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		logs = append(logs, b)
		for i := 0; i < 1000; i++ {
			if r := run(t, env(), a.as("decider", "questions")...); r.code == 0 {
				return c
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("the window did not start")
		return nil
	}
	c := startProc()
	released := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", released)...)
	open := a.openOne(t, "Drop notes.")
	before := strings.Count(string(a.receipts(t)), "\n")
	if err := c.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = c.Wait()
	c = startProc()
	defer func() { _ = c.Process.Signal(syscall.SIGTERM); _ = c.Wait() }()
	lines := strings.Split(strings.TrimSpace(string(a.receipts(t))), "\n")
	if len(lines) != before+1 {
		t.Fatalf("%d receipts after the restart, %d before", len(lines), before)
	}
	r := run(t, env(), a.as("proposer", "result", "--question", open)...)
	if r.code != 1 || !regexp.MustCompile(`^undeliverable `+receiptID+`\n$`).MatchString(r.stderr) {
		t.Fatalf("result: %v", r)
	}
	if r := run(t, env(), a.as("decider", "answer", "--question", open)...); r.code != 2 || r.stderr != "rejected no-open-question\n" {
		t.Fatalf("an answer after the restart: %v", r)
	}
	got := a.drain(t, "applier")
	if len(got) != 1 || got[0].stdout != "Rename total.\n" {
		t.Fatalf("the applier fetched %v", got)
	}
	if r := must(t, "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal()); r.lines()[0] != fmt.Sprintf("verified %d receipts", before+1) {
		t.Fatalf("verify: %v", r)
	}
	if !strings.Contains(logs[1].String(), "reloaded block undeliverable ") {
		t.Fatalf("the restarted window: %s", logs[1])
	}
}
