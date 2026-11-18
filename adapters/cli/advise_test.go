package cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
)

// TestTheWindowPrintsAdvice gives a patch whose rule lists review to advise, with no key: the patch
// is asked, and the window prints exactly the line advise review none <receipt id> once review has
// said none (docs/adr/0023-judges.md).
func TestTheWindowPrintsAdvice(t *testing.T) {
	t.Parallel()
	a := newJudgedAccordOf(t, "advise")
	w := a.startJudged(t, nil, "review", "spare")
	if r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "patch", "--text", "Drop the table.")...); r.code != cli.ExitOpen {
		t.Fatalf("the patch: %v", r)
	}
	id := judgedReceipt(t, a)
	want := "advise review none " + id
	for i := 0; i < 500; i++ {
		if strings.Contains(w.stderr.String(), want+"\n") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	n := 0
	for _, l := range strings.Split(w.stderr.String(), "\n") {
		if strings.HasPrefix(l, "advise ") {
			n++
			if l != want {
				t.Fatalf("the advice line %q, want %q", l, want)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d advice lines:\n%s", n, w.stderr.String())
	}
}
