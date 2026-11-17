package receipts_test

import (
	"reflect"
	"strings"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/receipts"
)

// TestByFitsTheReceipt holds receipt-11 through section 8 row 4: a by of the wrong form, or one that
// does not fit its receipt's kind, verdict and reason, fails statement-type at its index, and one
// that fits verifies (U3).
func TestByFitsTheReceipt(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	ask := entry{asked, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)}
	terms := receipts.By{Decider: receipts.ByTerms}
	judge := func(n string) receipts.By { return receipts.By{Decider: receipts.ByJudge, Judge: n} }
	mediator := func(e receipts.Event) receipts.By { return receipts.By{Decider: receipts.ByMediator, Event: e} }
	for _, c := range []struct {
		name, kind, verdict, reason string
		by                          receipts.By
		ok                          bool
	}{
		{"a give the terms passed", receipts.KindCrossing, "pass", "ok", terms, true},
		{"a give a judge passed", receipts.KindCrossing, "pass", "ok", judge("jev"), true},
		{"a judge's block", receipts.KindCrossing, "block", "judge:jev", judge("jev"), true},
		{"a judge's block naming another", receipts.KindCrossing, "block", "judge:jev", judge("other"), false},
		{"a judge's block by the terms", receipts.KindCrossing, "block", "judge:jev", terms, false},
		{"a judge on an ask", receipts.KindCrossing, "escalate", "maybe", judge("jev"), false},
		{"an answer on an ask", receipts.KindCrossing, "escalate", "maybe", answerBy, false},
		{"a judge on a no-match", receipts.KindCrossing, "block", "no-match", judge("jev"), false},
		{"the mediator on a crossing", receipts.KindCrossing, "block", "no-match", mediator(receipts.EventDeadline), false},
		{"a timeout by the terms", receipts.KindResolution, "block", receipts.ReasonTimeout, terms, false},
		{"a timeout by the wrong event", receipts.KindResolution, "block", receipts.ReasonTimeout, mediator(receipts.EventUndeliverable), false},
		{"a timeout at its deadline", receipts.KindResolution, "block", receipts.ReasonTimeout, mediator(receipts.EventDeadline), true},
		{"an undeliverable", receipts.KindResolution, "block", receipts.ReasonUndeliverable, mediator(receipts.EventUndeliverable), true},
		{"a decline by the mediator", receipts.KindResolution, "block", receipts.ReasonDeclined, mediator(receipts.EventDeadline), false},
		{"an accept by the terms", receipts.KindResolution, "pass", "ok", terms, false},
		{"an accept by a judge", receipts.KindResolution, "pass", "ok", judge("jev"), false},
		{"a decline by an answer", receipts.KindResolution, "block", receipts.ReasonDeclined, answerBy, true},
		{"an unset by", receipts.KindCrossing, "pass", "ok", receipts.By{}, false},
		{"an answer that is no hash", receipts.KindResolution, "block", receipts.ReasonCancelled, receipts.By{Decider: receipts.ByAnswer, Answer: "x"}, false},
		{"a judge that is no label", receipts.KindCrossing, "pass", "ok", judge("Jev"), false},
		{"an event not listed", receipts.KindResolution, "block", receipts.ReasonTimeout, mediator("close"), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var e entry
			if c.kind == receipts.KindResolution {
				sub := asked
				if c.reason == "ok" {
					sub = receipts.Subject{Name: receipts.NameAnswer, Digest: digestOf(2)}
				}
				e = entry{sub, predicateOf(c.kind, answerer, asker, c.verdict, c.reason, env)}
			} else {
				e = entry{receipts.Subject{Name: "note", Digest: digestOf(2)},
					predicateOf(c.kind, asker, receiver, c.verdict, c.reason, receipts.Envelope{Sender: asker, Seq: 2, Hash: mhash})}
			}
			e.pred.By = c.by
			_, err := receipts.Verify(med.Reference(), journal(t, med, []entry{ask, e}))
			if c.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.ok && !isCode(err, receipts.CodeStatementType, 1) {
				t.Fatalf("got %v, want statement-type at index 1", err)
			}
		})
	}
}

// TestByOfAReleaseIsItsClosings holds section 8 row 7 for receipt-11: a release repeats its closing's
// by, and a crossing receipt an answer decided that is not a release fails release (U3).
func TestByOfAReleaseIsItsClosings(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	ask := entry{asked, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)}
	accept := entry{receipts.Subject{Name: receipts.NameAnswer, Digest: digestOf(2)}, predicateOf(receipts.KindResolution, answerer, asker, "pass", "ok", env)}
	release := func(by receipts.By) entry {
		e := entry{asked, predicateOf(receipts.KindCrossing, asker, receiver, "pass", "ok", env)}
		e.pred.By = by
		return e
	}
	other := receipts.By{Decider: receipts.ByAnswer, Answer: "sha256:" + strings.Repeat("b2", 32)}
	if _, err := receipts.Verify(med.Reference(), journal(t, med, []entry{ask, accept, release(answerBy)})); err != nil {
		t.Fatalf("a release with its closing's by: %v", err)
	}
	for name, last := range map[string]entry{
		"another answer's by": release(other),
		"the terms' by":       release(receipts.By{Decider: receipts.ByTerms}),
	} {
		if _, err := receipts.Verify(med.Reference(), journal(t, med, []entry{ask, accept, last})); !isCode(err, receipts.CodeRelease, 2) {
			t.Fatalf("%s: got %v, want release at index 2", name, err)
		}
	}
	give := entry{receipts.Subject{Name: "note", Digest: digestOf(3)},
		predicateOf(receipts.KindCrossing, asker, receiver, "pass", "ok", receipts.Envelope{Sender: asker, Seq: 5, Hash: mhash})}
	give.pred.By = answerBy
	if _, err := receipts.Verify(med.Reference(), journal(t, med, []entry{give})); !isCode(err, receipts.CodeRelease, 0) {
		t.Fatalf("an answer's crossing receipt of no question: got %v, want release at index 0", err)
	}
}

// TestVerifySubset holds receipt-6 for a subset: a chain may start at any seq, each late start is
// listed, and a receipt whose question is not in the subset is outside it, where Verify fails it
// (U24).
func TestVerifySubset(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	notes := []entry{}
	for i := int64(1); i <= 3; i++ {
		notes = append(notes, entry{receipts.Subject{Name: "note", Digest: digestOf(byte(10 + i))},
			predicateOf(receipts.KindCrossing, asker, receiver, "pass", "ok", receipts.Envelope{Sender: asker, Seq: 1 + i, Hash: mhash})})
	}
	all := append([]entry{
		{asked, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)},
		{receipts.Subject{Name: receipts.NameAnswer, Digest: digestOf(2)}, predicateOf(receipts.KindResolution, answerer, asker, "pass", "ok", env)},
	}, notes...)
	list := journal(t, med, all)
	// The answer and the last two notes: the answer's question and the first note are left out.
	sub := [][]byte{list[1], list[3], list[4]}
	s, err := receipts.VerifySubset(med.Reference(), sub)
	if err != nil {
		t.Fatalf("VerifySubset: %v", err)
	}
	if len(s.Verified) != 3 {
		t.Fatalf("%d verified, want 3", len(s.Verified))
	}
	wantStarts := []receipts.Start{{From: asker, To: receiver, Seq: 3}}
	if !reflect.DeepEqual(s.Starts, wantStarts) {
		t.Fatalf("starts %+v, want %+v", s.Starts, wantStarts)
	}
	wantPlaces := []receipts.Place{{Role: receipts.RoleOutside, Question: -1}, {Role: receipts.RoleGive, Question: -1}, {Role: receipts.RoleGive, Question: -1}}
	if !reflect.DeepEqual(s.Places, wantPlaces) {
		t.Fatalf("places %+v, want %+v", s.Places, wantPlaces)
	}
	if _, err := receipts.Verify(med.Reference(), sub); !isCode(err, receipts.CodeRelease, 0) {
		t.Fatalf("Verify of the subset: got %v, want release at index 0", err)
	}
	if _, err := receipts.Verify(med.Reference(), sub[1:]); !isCode(err, receipts.CodeChain, 0) {
		t.Fatalf("Verify of a late chain: got %v, want chain at index 0", err)
	}
	if _, err := receipts.VerifySubset(med.Reference(), [][]byte{list[3], list[3]}); !isCode(err, receipts.CodeChain, 1) {
		t.Fatalf("a repeated receipt in a subset: got %v, want chain at index 1", err)
	}
	if _, err := receipts.VerifySubset("mediator", sub); !isCode(err, receipts.CodeReference, -1) {
		t.Fatalf("a mediator that is no reference: got %v, want reference", err)
	}
	whole, err := receipts.VerifySubset(med.Reference(), list)
	if err != nil || len(whole.Starts) != 0 || whole.Places[1].Role != receipts.RoleClosing {
		t.Fatalf("the whole list as a subset: %+v, %v", whole, err)
	}
}

// TestShowNamesTheDecider shows a receipt's by on the line after its verdict (receipt-8, U32).
func TestShowNamesTheDecider(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, _ := chain(t, med, 1)
	lines, err := receipts.Show(list[0])
	if err != nil {
		t.Fatal(err)
	}
	if lines[3] != "verdict pass, reason ok" || lines[4] != "by terms" {
		t.Fatalf("lines %q", lines[3:5])
	}
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	for want, e := range map[string]entry{
		"by answer " + answerBy.Answer: {asked, predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonDeclined, env)},
		"by mediator deadline":         {asked, predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonTimeout, env)},
		"by judge jev":                 {asked, predicateOf(receipts.KindCrossing, asker, receiver, "block", "judge:jev", env)},
	} {
		r := journal(t, med, []entry{e})
		lines, err := receipts.Show(r[0])
		if err != nil || lines[4] != want {
			t.Fatalf("Show: %q, %v; want %q", lines, err, want)
		}
	}
}

// FuzzVerifySubset verifies any list of receipt bytes as a subset. It never panics, and a list that
// verifies yields a place for each receipt (fuzz-parsers, core-no-panic).
func FuzzVerifySubset(f *testing.F) {
	med := ptesting.Key(f, "mediator").Reference()
	for _, b := range seeds.Inputs(f, "receipts", "receipt") {
		f.Add(b, b)
	}
	for _, b := range receiptSeeds(f) {
		f.Add(b, b)
	}
	f.Fuzz(func(t *testing.T, a, b []byte) {
		s, err := receipts.VerifySubset(med, [][]byte{a, b})
		if err == nil && (len(s.Verified) != 2 || len(s.Places) != 2) {
			t.Fatalf("a subset that verifies: %+v", s)
		}
	})
}
