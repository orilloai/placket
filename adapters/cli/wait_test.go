package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// last is the last line of a command's standard output.
func last(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return s[strings.LastIndex(s, "\n")+1:]
}

// TestWaitAndResultEndAlike asks three proposals and waits for each: an accept prints accepted, exits
// 0 and prints its answer; a change prints changed, exits 1 and prints the version; a decline prints
// declined and exits 1. result, after a wait that died and after a restart, prints the line and
// exits with the code the wait would have, and fetches the same; on an open question, or one that is
// not the key's, it prints open and exits 4. A wait that is interrupted prints its question and
// exits 4 (wait-and-result-shared).
func TestWaitAndResultEndAlike(t *testing.T) {
	t.Parallel()
	s := six(t)
	cases := []struct {
		step, word, text string
		code             int
	}{
		{"accepted", "accepted", "accepted", 0},
		{"changed", "changed", "Keep notes, hidden.", 1},
		{"declined", "declined", "", 1},
	}
	for _, c := range cases {
		r := s.steps[c.step]
		l := r.lines()
		if r.code != c.code || len(l) != 2 || !strings.HasPrefix(l[0], "ask maybe sha256:") || !strings.HasPrefix(l[1], c.word+" sha256:") {
			t.Fatalf("%s: %v", c.step, r)
		}
		if c.text != "" && last(r.stdout) != c.text {
			t.Fatalf("%s: printed %q", c.step, last(r.stdout))
		}
		if c.text == "" && (strings.Count(r.stdout, "\n") != 2 || !strings.HasPrefix(last(r.stdout), `{"payload":`)) {
			t.Fatalf("%s: printed %q", c.step, r.stdout)
		}
	}
	// The open proposal was closed by the restart: result reads the closing from the receipts.
	r := run(t, env(), s.as("proposer", "result", "--question", s.open)...)
	if r.code != 1 || !strings.HasPrefix(r.stderr, "undeliverable sha256:") {
		t.Fatalf("result after the restart: %v", r)
	}
	// result, for a wait that died, and again after a restart: the same line, code and text.
	g := run(t, env(), s.as("proposer", "give", "--to", s.refs["applier"], "--kind", "proposal", "--text", "Add a view.")...)
	q := last(g.stdout)
	if g.code != 4 {
		t.Fatalf("give: %v", g)
	}
	if r := run(t, env(), s.as("proposer", "result", "--question", q)...); r.code != 4 || r.stderr != "open\n" || last(r.stdout) != q {
		t.Fatalf("result on an open question: %v", r)
	}
	must(t, s.as("decider", "answer", "--question", q, "--text", "Yes, add it.")...)
	first := run(t, env(), s.as("proposer", "result", "--question", q)...)
	s.window.stop(t)
	s.start(t)
	again := run(t, env(), s.as("proposer", "result", "--question", q)...)
	if first.code != 0 || again.code != 0 || first.lines()[0] != again.lines()[0] || !strings.HasPrefix(first.stderr, "accepted sha256:") ||
		last(first.stdout) != "Yes, add it." || last(again.stdout) != "Yes, add it." {
		t.Fatalf("result:\n%v\nafter a restart:\n%v", first, again)
	}
	if r := run(t, env(), s.as("applier", "result", "--question", q)...); r.code != 4 || r.stderr != "open\n" {
		t.Fatalf("result to another key: %v", r)
	}
	// An interrupted wait.
	ctx, cancel := context.WithCancel(context.Background())
	e := env()
	e.Context = ctx
	done := make(chan result, 1)
	go func() {
		done <- run(t, e, s.as("proposer", "give", "--to", s.refs["applier"], "--kind", "proposal", "--text", "Wait.", "--wait")...)
	}()
	q = s.question(t, "decider")
	cancel()
	if r := (waiting(done)).end(t); r.code != 4 || last(r.stdout) != q {
		t.Fatalf("an interrupted wait: %v", r)
	}
}

// TestTwoWaitsEachEndWithTheirOwn runs two waits by one proposer at once; the second is answered
// first, so the first wait's fetch meets the second's answer ahead of its own: each wait ends with
// its own answer, and a wait that fetched another's keeps it and prints it as get does (RCP-15).
func TestTwoWaitsEachEndWithTheirOwn(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q1, w1 := a.ask(t, "One.", "t")
	w2 := make(waiting, 1)
	go func() {
		w2 <- run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Two.", "--wait")...)
	}()
	q2 := ""
	for i := 0; i < 500 && q2 == ""; i++ {
		r := must(t, a.as("decider", "questions")...)
		for _, l := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
			var q struct{ ID string }
			if json.Unmarshal([]byte(l), &q) == nil && q.ID != q1 {
				q2 = q.ID
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	must(t, a.as("decider", "change", "--question", q2, "--text", "Two, changed.")...)
	must(t, a.as("decider", "answer", "--question", q1, "--text", "One, yes.")...)
	r1, r2 := w1.end(t), w2.end(t)
	// The first wait may fetch the second's changed version before its own answer, and then prints it
	// as get does.
	first := regexp.MustCompile(`^ask maybe ` + q1 + `\naccepted ` + receiptID + `\n(delivery ` + receiptID + ` kind changed-version messageId change:` + q2 +
		` salt [0-9a-f]{32} re ` + q2 + `\n)?$`)
	if r1.code != 0 || last(r1.stdout) != "One, yes." || !first.MatchString(r1.stderr) {
		t.Fatalf("the first wait: %v", r1)
	}
	second := regexp.MustCompile(`^ask maybe ` + q2 + `\nchanged ` + receiptID + `\n$`)
	if r2.code != 1 || last(r2.stdout) != "Two, changed." || !second.MatchString(r2.stderr) {
		t.Fatalf("the second wait: %v", r2)
	}
	kept, _ := os.ReadFile(filepath.Join(a.state("proposer"), "kept.jsonl"))
	if strings.Count(string(kept), `"re":"`+q1+`"`) != 1 || strings.Count(string(kept), `"re":"`+q2+`"`) != 1 {
		t.Fatalf("kept.jsonl: %s", kept)
	}
	if r := must(t, a.as("proposer", "get")...); r.stderr != "no delivery\n" {
		t.Fatalf("a delivery was left: %v", r)
	}
}

// receiptID matches a receipt id, a typed hash, in a pattern.
const receiptID = `sha256:[0-9a-f]{64}`
