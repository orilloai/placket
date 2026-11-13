package cli_test

import "testing"

// TestQuestionsAreForAnswerersOnly asks questions as a stranger and as the mediator's own key, with
// no question open and with one open: each is rejected not-a-member, exit 2, and prints nothing on
// standard output, so the part is shown to no key but its answerers'. The decider sees it.
func TestQuestionsAreForAnswerersOnly(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	check := func(when string) {
		t.Helper()
		for _, who := range []string{"stranger", "mediator"} {
			if r := run(t, env(), a.as(who, "questions")...); r.code != 2 || r.stdout != "" || r.stderr != "rejected not-a-member\n" {
				t.Fatalf("the %s's questions, %s: %v", who, when, r)
			}
		}
	}
	check("with none open")
	q := a.openOne(t, "Drop notes.")
	check("with one open")
	if got := a.question(t, "decider"); got != q {
		t.Fatalf("the decider's question: %s", got)
	}
}
