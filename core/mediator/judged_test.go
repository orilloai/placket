package mediator_test

import (
	"encoding/json"
	"testing"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/composition"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// judgedMediator founds the judge scenarios' set of vectors/founding/, whose report both sides list
// jev to decide.
func judgedMediator(t *testing.T) *mediator.Mediator {
	t.Helper()
	set, at := ptesting.Founding(t, "judged")
	m, err := mediator.Found(set, ptesting.Key(t, "mediator"), at)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// giveReport gives a report from the proposer to the applier at seq with the judgment.
func giveReport(t *testing.T, m *mediator.Mediator, seq int64, j *composition.Judgment) mediator.Outcome {
	t.Helper()
	p := ptesting.Key(t, "proposer")
	msg := message(t, m.Accord(), "report-"+string(rune('0'+seq)), ptesting.Key(t, "applier").Reference(), "report", "A report.", nil)
	out, err := m.Give(p.Reference(), msg, envelope(t, seq, msg, p), j, "2026-11-12T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestGiveTakesTheJudgment gives a report jev's pass, its block and no judgment: the first passes and
// the second blocks, each by the judge, and the last is asked, by the terms (composition-7,
// receipt-11).
func TestGiveTakesTheJudgment(t *testing.T) {
	t.Parallel()
	m := judgedMediator(t)
	judge := receipts.By{Decider: receipts.ByJudge, Judge: "jev"}
	for i, c := range []struct {
		j       *composition.Judgment
		verdict string
		reason  string
		by      receipts.By
	}{
		{&composition.Judgment{Name: "jev", Verdict: composition.Pass}, "pass", "ok", judge},
		{&composition.Judgment{Name: "jev", Verdict: composition.Block}, "block", "judge:jev", judge},
		{nil, "escalate", "maybe", receipts.By{Decider: receipts.ByTerms}},
	} {
		out := giveReport(t, m, int64(i+1), c.j)
		st := statement(t, m, out.ReceiptID)
		if out.Verdict != c.verdict || out.Reason != c.reason || st.Predicate.By != c.by {
			t.Fatalf("judgment %+v: %s %s by %+v", c.j, out.Verdict, out.Reason, st.Predicate.By)
		}
	}
}

// TestReleaseNotJudged accepts with release a report whose terms list jev to decide: the release is
// decided again with no judgment, so it is pass, ok, by the answer's call, never judge:jev; and the
// call is inside both deliveries the answer queues (maybe-10, maybe-12, U8, U17).
func TestReleaseNotJudged(t *testing.T) {
	t.Parallel()
	m := judgedMediator(t)
	q := giveReport(t, m, 1, nil).ReceiptID
	d := ptesting.Key(t, "decider").Reference()
	r := accept(t, m, d, q, true)
	outs, err := m.Resolve(d, r, "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 2 {
		t.Fatalf("got %+v, %v", outs, err)
	}
	by := receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(r.Call)}
	for _, o := range outs {
		if st := statement(t, m, o.ReceiptID); o.Verdict != "pass" || o.Reason != "ok" || st.Predicate.By != by {
			t.Fatalf("%s %s by %+v, want pass ok by the answer", o.Verdict, o.Reason, st.Predicate.By)
		}
	}
	for _, n := range []string{"proposer", "applier"} {
		dl, err := m.Get(ptesting.Key(t, n).Reference())
		if err != nil || dl == nil {
			t.Fatalf("%s's delivery: %v", n, err)
		}
		var got struct {
			Answer json.RawMessage `json:"answer"`
		}
		if err := json.Unmarshal(dl, &got); err != nil || string(got.Answer) != string(r.Call) {
			t.Fatalf("%s's delivery carries the call %s, want %s", n, got.Answer, r.Call)
		}
	}
}
