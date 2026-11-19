package conform

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/receipts"
)

// alone is a list of one receipt, the statement's subject and predicate with its by set to the call's
// typed hash, signed by the mediator's key as the first of its pair.
func alone(t *testing.T, st receipts.Statement, call []byte) [][]byte {
	t.Helper()
	p := st.Predicate
	p.By = receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(call)}
	s := newSigner(t)
	s.add(t, st.Subject, p)
	return s.list
}

// TestASubsetReadsAnOutsideCall gives the worked journal's accept and release as a subset, their
// question outside it: with the decider's call both conform, unwitnessed and signed; with no call,
// unwitnessed and not signed (verify-3).
func TestASubsetReadsAnOutsideCall(t *testing.T) {
	t.Parallel()
	w := journal(t)
	e := w.evidence()
	e.Receipts, e.Subset = w.list[2:], true
	r, err := Check(e)
	if err != nil || len(r.Decisions) != 2 || r.Unmatched != 0 {
		t.Fatalf("with the call: %+v, %v", r, err)
	}
	for _, d := range r.Decisions {
		if d.Checked || !d.Signed || d.Witness != Unwitnessed {
			t.Fatalf("with the call, decision %+v", d)
		}
	}
	e.Answers = nil
	r, err = Check(e)
	if err != nil {
		t.Fatalf("without the call: %v", err)
	}
	for _, d := range r.Decisions {
		if d.Signed || d.Witness != Unwitnessed {
			t.Fatalf("without the call, decision %+v", d)
		}
	}
	// In the whole list the same receipts are witnessed, not signed: Signed is for a receipt outside.
	r, err = Check(w.evidence())
	if err != nil || r.Decisions[3].Signed || r.Decisions[3].Witness != Witnessed {
		t.Fatalf("the whole list: %+v, %v", r, err)
	}
}

// TestAnOutsideCallThatDoesNotFit gives one receipt alone, as a subset, with the call its by names,
// and checks each call that does not fit as far as the receipt shows fails witness at index 0, and
// each that does is signed (verify-3).
func TestAnOutsideCallThatDoesNotFit(t *testing.T) {
	t.Parallel()
	w := journal(t)
	call, err := answer.Read(w.call)
	if err != nil {
		t.Fatal(err)
	}
	stAsk, _ := receipts.Decode(w.list[1])
	stAccept, _ := receipts.Decode(w.list[2])
	stRelease, _ := receipts.Decode(w.list[3])
	other := "sha256:" + strings.Repeat("ab", 32)
	declined := stAccept
	declined.Subject = stAsk.Subject
	declined.Predicate.Verdict, declined.Predicate.Reason = "block", receipts.ReasonDeclined
	cancelled := declined
	cancelled.Predicate.Reason = receipts.ReasonCancelled
	changed := stAccept
	changed.Subject = receipts.Subject{Name: receipts.NameChanged, Digest: call.Digest}
	changeRelease := changed
	changeRelease.Predicate.To = ptesting.Key(t, "applier").Reference()
	// A resolution pass ok of the asked part's kind, which no answer decides, with an accept of its digest.
	ofTheAskedKind := stAccept
	ofTheAskedKind.Subject = stAsk.Subject
	accept := func(accord string, release bool) answer.Call {
		return answer.Call{Accord: accord, Question: call.Question, Action: answer.Accepted, Digest: call.Digest, Release: release}
	}
	change := func(digest string, release bool) answer.Call {
		return answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Changed, Digest: digest, Release: release}
	}
	for name, c := range map[string]struct {
		st     receipts.Statement
		call   answer.Call
		key    string
		signed bool
	}{
		"a release, the decider's accept":      {stRelease, accept(w.accord, true), "decider", true},
		"a release, another member's accept":   {stRelease, accept(w.accord, true), "applier", true},
		"a release, a stranger's accept":       {stRelease, accept(w.accord, true), "stranger", false},
		"a release, of another Accord":         {stRelease, accept(other, true), "decider", false},
		"a release, an accept with no release": {stRelease, accept(w.accord, false), "decider", false},
		"a release, a change":                  {stRelease, change(call.Digest, true), "decider", false},
		"an accept, with no release":           {stAccept, accept(w.accord, false), "decider", true},
		"an accept, signed by another member":  {stAccept, accept(w.accord, true), "proposer", false},
		"an accept, of another digest":         {stAccept, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Accepted, Digest: strings.Repeat("0", 64)}, "decider", false},
		"a change to the asker, no release":    {changed, change(call.Digest, false), "decider", true},
		"a change to the receiver":             {changeRelease, change(call.Digest, true), "decider", true},
		"a change to the receiver, no release": {changeRelease, change(call.Digest, false), "decider", false},
		"a change, of another digest":          {changed, change(strings.Repeat("0", 64), true), "decider", false},
		"a resolution of the asked kind":       {ofTheAskedKind, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Accepted, Digest: stAsk.Subject.Digest}, "decider", false},
		"a decline, by a member":               {declined, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Declined}, "applier", true},
		"a decline, by a stranger":             {declined, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Declined}, "stranger", false},
		"a decline, an accept's call":          {declined, accept(w.accord, true), "decider", false},
		"a cancel, by the asker":               {cancelled, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Cancelled}, "proposer", true},
		"a cancel, by another member":          {cancelled, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Cancelled}, "decider", false},
		"a cancel, a decline's call":           {cancelled, answer.Call{Accord: w.accord, Question: call.Question, Action: answer.Declined}, "proposer", false},
	} {
		b, err := answer.Sign(c.call, ptesting.Key(t, c.key))
		if err != nil {
			t.Fatal(err)
		}
		e := w.evidence()
		e.Receipts, e.Answers, e.Subset = alone(t, c.st, b), [][]byte{b}, true
		r, err := Check(e)
		switch {
		case c.signed && (err != nil || !r.Decisions[0].Signed || r.Decisions[0].Witness != Unwitnessed):
			t.Errorf("%s: %+v, %v", name, r, err)
		case !c.signed && !isCode(err, CodeWitness, 0):
			t.Errorf("%s: got %v, want witness at 0", name, err)
		}
	}
}

// edit is a delivery with its members changed: each one set, or removed when its value is nil, in
// canonical form.
func edit(t *testing.T, d []byte, set map[string]json.RawMessage) []byte {
	t.Helper()
	var o map[string]json.RawMessage
	if err := json.Unmarshal(d, &o); err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		if v == nil {
			delete(o, k)
		} else {
			o[k] = v
		}
	}
	b, _ := json.Marshal(o)
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestBodyReadsTheDeliverysCall checks row 5 reads a delivery's answer: the released proposal's
// delivery without its call, or with another call, fails body at its receipt's index, and so does the
// note's, which no answer decided, with a call added (verify-2 row 5, maybe-12).
func TestBodyReadsTheDeliverysCall(t *testing.T) {
	t.Parallel()
	w := journal(t)
	r, err := Check(w.evidence())
	if err != nil {
		t.Fatal(err)
	}
	released, note := w.deliveries[1], w.deliveries[0]
	if err := Body(r, released); err != nil {
		t.Fatalf("the released proposal: %v", err)
	}
	if err := Body(r, edit(t, released, map[string]json.RawMessage{"answer": nil})); !isCode(err, CodeBody, 3) {
		t.Fatalf("without its call: %v", err)
	}
	call, _ := answer.Read(w.call)
	call.Release = false
	b, err := answer.Sign(call, ptesting.Key(t, "decider"))
	if err != nil {
		t.Fatal(err)
	}
	if err := Body(r, edit(t, released, map[string]json.RawMessage{"answer": b})); !isCode(err, CodeBody, 3) {
		t.Fatalf("with another call: %v", err)
	}
	if err := Body(r, note); err != nil {
		t.Fatalf("the note: %v", err)
	}
	if err := Body(r, edit(t, note, map[string]json.RawMessage{"answer": w.call})); !isCode(err, CodeBody, 0) {
		t.Fatalf("the note with a call: %v", err)
	}
}

// TestADeliveryCheckedAlone checks the applier's delivery of the released proposal as verify-5 does:
// its receipt alone, as a subset, its answer the one call. It conforms, its answer signed, and its
// body is the part; without its answer it fails body at index 0.
func TestADeliveryCheckedAlone(t *testing.T) {
	t.Parallel()
	w := journal(t)
	d := w.deliveries[1]
	var o map[string]json.RawMessage
	if err := json.Unmarshal(d, &o); err != nil {
		t.Fatal(err)
	}
	e := w.evidence()
	e.Receipts, e.Answers, e.Subset = [][]byte{o["receipt"]}, [][]byte{o["answer"]}, true
	r, err := Check(e)
	if err != nil || len(r.Decisions) != 1 || !r.Decisions[0].Signed || r.Unmatched != 0 {
		t.Fatalf("Check: %+v, %v", r, err)
	}
	if err := Body(r, d); err != nil {
		t.Fatalf("Body: %v", err)
	}
	e.Answers = nil
	if r, err = Check(e); err != nil || r.Decisions[0].Signed {
		t.Fatalf("Check without its answer: %+v, %v", r, err)
	}
	if err := Body(r, edit(t, d, map[string]json.RawMessage{"answer": nil})); !isCode(err, CodeBody, 0) {
		t.Fatalf("Body without its answer: %v", err)
	}
}
