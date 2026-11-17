package cli_test

import (
	"strings"
	"testing"
)

// TestAWaitTakesOnlyItsOwnAnswer gives the proposer, before the answer, a part of kind answer with re
// the question, which the decider's terms let it give. The wait fetches it first but prints it as get
// prints another delivery, and ends with the answer that closed the question, the delivery of the
// closing receipt. A part of that kind kept by an earlier get is passed over by result in the same
// way.
func TestAWaitTakesOnlyItsOwnAnswer(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q, w := a.ask(t, "Rename total.", "billing")
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "answer", "--re", q, "--text", "Not the answer.")...)
	must(t, a.as("decider", "answer", "--question", q, "--text", "Yes.")...)
	r := w.end(t)
	l := r.lines()
	if r.code != 0 || len(l) != 3 || !strings.HasPrefix(l[1], "accepted sha256:") || !strings.HasPrefix(l[2], "delivery ") || !strings.HasSuffix(l[2], " re "+q) {
		t.Fatalf("the wait: %v", r)
	}
	// The receipts of the ask and the answer, then the other part as get prints it, then the answer.
	if out := strings.Split(r.stdout, "\n"); len(out) != 6 || out[3] != "Not the answer." || out[4] != "Yes." || out[5] != "" {
		t.Fatalf("the wait printed %q", r.stdout)
	}

	q2 := a.openOne(t, "Drop notes.")
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "answer", "--re", q2, "--text", "Not the answer either.")...)
	must(t, a.as("decider", "answer", "--question", q2, "--text", "Yes again.")...)
	if g := must(t, a.as("proposer", "get", "--keep")...); g.stdout != "Not the answer either.\n" {
		t.Fatalf("the get: %v", g)
	}
	r = run(t, env(), a.as("proposer", "result", "--question", q2)...)
	if r.code != 0 || last(r.stdout) != "Yes again." || !strings.HasPrefix(r.stderr, "accepted sha256:") {
		t.Fatalf("result: %v", r)
	}
}
