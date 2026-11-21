package conform

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

const body = "The planted body of the proposal."

// A worked journal of the ask-and-log Accord: a note that passes, a proposal asked about, and the
// decider's accept with release.
type worked struct {
	set        founding.Inputs
	call       []byte
	accord     string
	list       [][]byte
	deliveries [][]byte
}

func message(t testing.TB, accord, id, kind, text string) []byte {
	t.Helper()
	ext := "https://placket.tech/crossing/0.1"
	m := map[string]any{"messageId": id, "role": "ROLE_USER", "extensions": []any{ext},
		"metadata": map[string]any{ext: map[string]any{"accord": accord, "to": ptesting.Key(t, "applier").Reference()}},
		"parts": []any{map[string]any{"text": text, "metadata": map[string]any{ext: map[string]any{
			"kind": kind, "salt": hex.EncodeToString(bytes.Repeat([]byte(id[:1]), 32))}}}}}
	b, _ := json.Marshal(m)
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func envelope(t testing.TB, seq int64, msg []byte, k signing.Key) []byte {
	t.Helper()
	h, _ := canonical.TypedHash(msg)
	b, _ := json.Marshal(map[string]any{"seq": seq, "hash": h})
	sig, err := signing.Sign(b, signing.TypeEnvelope, k)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{"seq": seq, "hash": h, "sig": sig})
	return out
}

func journal(t testing.TB) worked {
	t.Helper()
	set, at := ptesting.Founding(t, "ask-and-log")
	m, err := mediator.Found(set, ptesting.Key(t, "mediator"), at)
	if err != nil {
		t.Fatal(err)
	}
	p, d := ptesting.Key(t, "proposer"), ptesting.Key(t, "decider")
	note := message(t, m.Accord(), "n-1", "note", "A note.")
	if _, err := m.Give(p.Reference(), note, envelope(t, 1, note, p), nil, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	prop := message(t, m.Accord(), "p-1", "proposal", body)
	ask, err := m.Give(p.Reference(), prop, envelope(t, 2, prop, p), nil, "2026-11-12T09:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	salt := bytes.Repeat([]byte{9}, 32)
	call, err := answer.Sign(answer.Call{Accord: m.Accord(), Question: ask.ReceiptID, Action: answer.Accepted,
		Digest: receipts.Digest(salt, []byte("accepted")), Release: true}, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Resolve(d.Reference(), mediator.Resolution{Call: call, Text: "accepted", Salt: salt}, "2026-11-12T09:02:00Z"); err != nil {
		t.Fatal(err)
	}
	w := worked{set: set, call: call, accord: m.Accord(), list: m.Receipts()}
	for _, n := range []string{"applier", "proposer"} {
		for {
			dl, _ := m.Get(ptesting.Key(t, n).Reference())
			if dl == nil {
				break
			}
			w.deliveries = append(w.deliveries, dl)
		}
	}
	return w
}

func (w worked) evidence() Evidence {
	return Evidence{Accord: w.accord, Founding: w.set.Set, Openings: w.set.Openings, Acceptances: w.set.Acceptances,
		Receipts: w.list, Answers: [][]byte{w.call}, Kernel: kernelOf(w.set.Set)}
}

func isCode(err error, code Code, index int) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code && e.Index == index
}

// TestAWorkedJournalConforms checks the worked journal: valid, every decision conforms, the accept
// and its release witnessed by the decider's call, and the module the trusted one (verify-2,
// verify-3).
func TestAWorkedJournalConforms(t *testing.T) {
	t.Parallel()
	w := journal(t)
	r, err := Check(w.evidence())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !r.Module || r.Mediator != ptesting.Key(t, "mediator").Reference() || r.Members != 3 || r.OperatorPrincipalOf != 0 || r.Unmatched != 0 {
		t.Fatalf("report %+v", r)
	}
	var got []Witness
	for _, d := range r.Decisions {
		got = append(got, d.Witness)
	}
	if want := []Witness{"", "", Witnessed, Witnessed}; strings.Join(ws(got), ",") != strings.Join(ws(want), ",") {
		t.Fatalf("witnesses %v, want %v", got, want)
	}
	e := w.evidence()
	e.Answers = nil
	if r, err := Check(e); err != nil || r.Decisions[2].Witness != Unwitnessed {
		t.Fatalf("without the call: %+v, %v", r, err)
	}
	e = w.evidence()
	e.Answers = append(e.Answers, []byte(`{"not":"a call"}`))
	if r, err := Check(e); err != nil || r.Unmatched != 1 {
		t.Fatalf("an extra call: %+v, %v", r, err)
	}
}

func ws(l []Witness) []string {
	var out []string
	for _, w := range l {
		out = append(out, string(w))
	}
	return out
}

// TestNeverNeedsABody holds invariant 4: the evidence holds no body and no salt, and Check decides
// conformance from it alone; the deliveries, which hold them, change nothing.
func TestNeverNeedsABody(t *testing.T) {
	t.Parallel()
	w := journal(t)
	e := w.evidence()
	for _, b := range append(append([][]byte{e.Founding}, e.Receipts...), e.Answers...) {
		if bytes.Contains(b, []byte(body)) || bytes.Contains(b, []byte(hex.EncodeToString(bytes.Repeat([]byte{9}, 32)))) {
			t.Fatal("the evidence holds a body or a salt")
		}
	}
	r, err := Check(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range w.deliveries {
		if err := Body(r, d); err != nil {
			t.Fatalf("a delivery the mediator queued: %v", err)
		}
	}
}

// TestBodyWithoutFounding checks one delivery with no founding set, under the mediator's reference
// alone: its own body passes, and a byte changed fails body at its receipt's index (U25).
func TestBodyWithoutFounding(t *testing.T) {
	t.Parallel()
	w := journal(t)
	r, err := Check(Evidence{Mediator: ptesting.Key(t, "mediator").Reference(), Receipts: w.list})
	if err != nil || r.Module {
		t.Fatalf("Check: %+v, %v", r, err)
	}
	d := w.deliveries[1]
	if err := Body(r, d); err != nil {
		t.Fatalf("its own body: %v", err)
	}
	changed := bytes.Replace(d, []byte(`"text":"The planted`), []byte(`"text":"the planted`), 1)
	if bytes.Equal(changed, d) {
		t.Fatal("no byte changed")
	}
	if err := Body(r, changed); !isCode(err, CodeBody, 3) {
		t.Fatalf("one byte changed: %v", err)
	}
	if err := Body(r, []byte("{}")); !isCode(err, CodeBody, -1) {
		t.Fatalf("a delivery of no receipt: %v", err)
	}
}

// signer builds receipts one after another as the mediator, each chained in its pair.
type signer struct {
	key    signing.Key
	chains map[[2]string]int64
	ids    map[[2]string]string
	list   [][]byte
}

func newSigner(t testing.TB) *signer {
	return &signer{key: ptesting.Key(t, "mediator"), chains: map[[2]string]int64{}, ids: map[[2]string]string{}}
}

func (s *signer) add(t testing.TB, sub receipts.Subject, p receipts.Predicate) string {
	t.Helper()
	pair := [2]string{p.From, p.To}
	p.Seq, p.Prev = s.chains[pair]+1, s.ids[pair]
	r, v, err := receipts.Build(sub, p, s.key)
	if err != nil {
		t.Fatal(err)
	}
	s.chains[pair], s.ids[pair] = p.Seq, v.Statement().ID
	s.list = append(s.list, r)
	return v.Statement().ID
}

// TestEachCode fails each row alone with a list the mediator's key signs, and one receipt that fails
// rows 1 and 2 at once, which is accord (verify-2, check-order).
func TestEachCode(t *testing.T) {
	t.Parallel()
	w := journal(t)
	st, err := receipts.Decode(w.list[0])
	if err != nil {
		t.Fatal(err)
	}
	p, ap, stranger := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference(), ptesting.Key(t, "stranger").Reference()
	terms := receipts.By{Decider: receipts.ByTerms}
	pred := func(from, accord, kind, verdict, reason string) receipts.Predicate {
		return receipts.Predicate{Accord: accord, Card: st.Predicate.Card, Kind: receipts.KindCrossing, From: from, To: ap,
			At: "2026-11-12T09:00:00Z", Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: st.Predicate.Envelope.Hash},
			Verdict: verdict, Reason: reason, By: terms}
	}
	other := "sha256:" + strings.Repeat("ab", 32)
	for name, c := range map[string]struct {
		pred receipts.Predicate
		kind string
		code Code
	}{
		"another Accord and a stranger": {pred(stranger, other, "note", "pass", "ok"), "note", CodeAccord},
		"a stranger":                    {pred(stranger, w.accord, "note", "pass", "ok"), "note", CodeParty},
		"a pass the terms block":        {pred(p, w.accord, "tests", "pass", "ok"), "tests", CodeDecision},
		"a judge no term grants": {func() receipts.Predicate {
			x := pred(p, w.accord, "note", "pass", "ok")
			x.By = receipts.By{Decider: receipts.ByJudge, Judge: "jev"}
			return x
		}(), "note", CodeDecision},
	} {
		s := newSigner(t)
		s.add(t, receipts.Subject{Name: c.kind, Digest: strings.Repeat("cd", 32)}, c.pred)
		e := w.evidence()
		e.Receipts = s.list
		if _, err := Check(e); !isCode(err, c.code, 0) {
			t.Errorf("%s: got %v, want %s at 0", name, err, c.code)
		}
	}
	e := w.evidence()
	e.Accord = other
	if _, err := Check(e); !isCode(err, CodeFounding, -1) {
		t.Fatalf("another Accord id: %v", err)
	}
	e = w.evidence()
	e.Mediator = p
	if _, err := Check(e); !isCode(err, CodeFounding, -1) {
		t.Fatalf("another mediator: %v", err)
	}
	e = w.evidence()
	e.Kernel = composition.KernelHash()
	if r, err := Check(e); err != nil || r.Module {
		t.Fatalf("another module: %+v, %v", r, err)
	}
	e = w.evidence()
	e.Receipts = e.Receipts[1:]
	var re *receipts.Error
	if _, err := Check(e); !errors.As(err, &re) || re.Code != receipts.CodeChain {
		t.Fatalf("a list cut at its start: %v", err)
	}
	e.Subset = true
	if r, err := Check(e); err != nil || len(r.Subset.Starts) != 1 {
		t.Fatalf("the same list as a subset: %+v, %v", r, err)
	}
}

// TestAWitnessThatDoesNotFit fails witness for a matched call another key signed, one of another
// Accord, and one that asks for no release though a release followed (verify-2, row 4).
func TestAWitnessThatDoesNotFit(t *testing.T) {
	t.Parallel()
	w := journal(t)
	call, err := answer.Read(w.call)
	if err != nil {
		t.Fatal(err)
	}
	stAsk, _ := receipts.Decode(w.list[1])
	stAccept, _ := receipts.Decode(w.list[2])
	stRelease, _ := receipts.Decode(w.list[3])
	for name, c := range map[string]struct {
		call  answer.Call
		key   string
		index int
	}{
		"another Accord":     {answer.Call{Accord: "sha256:" + strings.Repeat("ab", 32), Question: call.Question, Action: call.Action, Digest: call.Digest, Release: true}, "decider", 2},
		"no release":         {answer.Call{Accord: call.Accord, Question: call.Question, Action: call.Action, Digest: call.Digest}, "decider", 3},
		"signed by the lead": {answer.Call{Accord: call.Accord, Question: call.Question, Action: call.Action, Digest: call.Digest, Release: true}, "proposer", 2},
	} {
		b, err := answer.Sign(c.call, ptesting.Key(t, c.key))
		if err != nil {
			t.Fatal(err)
		}
		by := receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(b)}
		s := newSigner(t)
		s.list = append(s.list, w.list[0], w.list[1])
		s.chains[[2]string{stAsk.Predicate.From, stAsk.Predicate.To}], s.ids[[2]string{stAsk.Predicate.From, stAsk.Predicate.To}] = 2, stAsk.ID
		a := stAccept.Predicate
		a.By = by
		s.add(t, stAccept.Subject, a)
		rl := stRelease.Predicate
		rl.By = by
		s.add(t, stRelease.Subject, rl)
		e := w.evidence()
		e.Receipts, e.Answers = s.list, [][]byte{b}
		if _, err := Check(e); !isCode(err, CodeWitness, c.index) {
			t.Errorf("%s: got %v, want witness at %d", name, err, c.index)
		}
	}
}

// TestADeclineUnderAnotherModule declines the worked journal's question with a call, under a card
// whose module is not the one the verifier trusts, so that the question's rule is not known: a
// decline signed by a member is witnessed, and one signed by a key outside the Accord fails witness
// (verify-2, row 4, which still runs when row 3 is skipped).
func TestADeclineUnderAnotherModule(t *testing.T) {
	t.Parallel()
	w := journal(t)
	stAsk, _ := receipts.Decode(w.list[1])
	stAccept, _ := receipts.Decode(w.list[2])
	for name, c := range map[string]struct {
		key  string
		fail bool
	}{
		"signed by the decider":  {"decider", false},
		"signed by the applier":  {"applier", false},
		"signed by a stranger":   {"stranger", true},
		"signed by the mediator": {"mediator", true},
	} {
		k := ptesting.Key(t, c.key)
		b, err := answer.Sign(answer.Call{Accord: w.accord, Question: stAsk.ID, Action: answer.Declined, From: k.Reference()}, k)
		if err != nil {
			t.Fatal(err)
		}
		s := newSigner(t)
		s.list = append(s.list, w.list[0], w.list[1])
		s.chains[[2]string{stAsk.Predicate.From, stAsk.Predicate.To}], s.ids[[2]string{stAsk.Predicate.From, stAsk.Predicate.To}] = 2, stAsk.ID
		d := stAccept.Predicate
		d.Verdict, d.Reason, d.By = "block", receipts.ReasonDeclined, receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(b)}
		s.add(t, stAsk.Subject, d)
		e := w.evidence()
		e.Receipts, e.Answers, e.Kernel = s.list, [][]byte{b}, composition.KernelHash()
		r, err := Check(e)
		switch {
		case c.fail && !isCode(err, CodeWitness, 2):
			t.Errorf("%s: got %v, want witness at 2", name, err)
		case !c.fail && (err != nil || r.Module || r.Decisions[2].Witness != Witnessed):
			t.Errorf("%s: %+v, %v", name, r, err)
		}
	}
}

// TestASubsetMayLeaveTheReleaseOut cuts the worked journal before the release of its accept, whose
// call asks for one: the whole list fails witness at the accept, and the same receipts given as a
// subset conform, since a subset may leave the release out (verify-2, row 4).
func TestASubsetMayLeaveTheReleaseOut(t *testing.T) {
	t.Parallel()
	w := journal(t)
	e := w.evidence()
	e.Receipts = w.list[:3]
	if _, err := Check(e); !isCode(err, CodeWitness, 2) {
		t.Fatalf("the list cut before its release: %v", err)
	}
	e.Subset = true
	if r, err := Check(e); err != nil || r.Decisions[2].Witness != Witnessed {
		t.Fatalf("the same receipts as a subset: %+v, %v", r, err)
	}
}

// TestANonconformingLogKeepsItsReport puts a pass the terms block between the worked journal's
// question and its accept: Check fails decision at that receipt, and still returns the report of the
// whole list, every receipt's decision with the accept and its release witnessed after the failure,
// so that a verifier can show the log it refuses (verify-2, commands-8).
func TestANonconformingLogKeepsItsReport(t *testing.T) {
	t.Parallel()
	w := journal(t)
	ask, err := receipts.Decode(w.list[1])
	if err != nil {
		t.Fatal(err)
	}
	s := newSigner(t)
	s.list = append(s.list, w.list[0], w.list[1])
	pair := [2]string{ask.Predicate.From, ask.Predicate.To}
	s.chains[pair], s.ids[pair] = ask.Predicate.Seq, ask.ID
	forged := ask.Predicate
	forged.Verdict, forged.Reason, forged.At = "pass", "ok", "2026-11-12T09:01:30Z"
	forged.Envelope = receipts.Envelope{Sender: forged.From, Seq: 3, Hash: "sha256:" + strings.Repeat("ef", 32)}
	s.add(t, receipts.Subject{Name: "tests", Digest: strings.Repeat("cd", 32)}, forged)
	for _, b := range w.list[2:] {
		st, err := receipts.Decode(b)
		if err != nil {
			t.Fatal(err)
		}
		s.add(t, st.Subject, st.Predicate)
	}
	e := w.evidence()
	e.Receipts = s.list
	r, err := Check(e)
	if !isCode(err, CodeDecision, 2) {
		t.Fatalf("got %v, want decision at 2", err)
	}
	var got []Witness
	for _, d := range r.Decisions {
		got = append(got, d.Witness)
	}
	if want := []Witness{"", "", "", Witnessed, Witnessed}; strings.Join(ws(got), ",") != strings.Join(ws(want), ",") ||
		len(r.Subset.Verified) != 5 || r.Unmatched != 0 || !r.Module {
		t.Fatalf("witnesses %v, want %v; report %+v", got, want, r)
	}
}

// TestARecodedCallIsNotTheCall gives, before the decider's call, a copy of it with one space added:
// answer.ID gives the copy the receipt's by.answer, but only the canonical bytes are the call, so
// the copy matches no receipt, the log still conforms, and the copy is counted unmatched (verify-3).
func TestARecodedCallIsNotTheCall(t *testing.T) {
	t.Parallel()
	w := journal(t)
	twin := append([]byte("{ "), w.call[1:]...)
	if answer.ID(twin) != answer.ID(w.call) {
		t.Fatal("the twin does not share the call's typed hash")
	}
	e := w.evidence()
	e.Answers = [][]byte{twin, w.call}
	r, err := Check(e)
	if err != nil || r.Unmatched != 1 || r.Decisions[2].Witness != Witnessed {
		t.Fatalf("the twin first: %+v, %v", r, err)
	}
	e.Answers = [][]byte{twin}
	if r, err := Check(e); err != nil || r.Unmatched != 1 || r.Decisions[2].Witness != Unwitnessed {
		t.Fatalf("the twin alone: %+v, %v", r, err)
	}
}

// TestAContradictedCallIsNeverWitnessed fails row 4 by each of its branches: a call of another
// Accord, a release whose call asks for none, a decline and a cancel signed by a key that may not
// take that action, an accept signed by another member, and a passing accept whose call asks for a
// release with none after it. Each receipt that fails row 4 is contradicted, never witnessed or
// unwitnessed; a release after a contradicted accept is contradicted too, and an accept whose call
// fits stays witnessed when only its release fails (verify-1 row 4, verify-3).
func TestAContradictedCallIsNeverWitnessed(t *testing.T) {
	t.Parallel()
	w := journal(t)
	call, err := answer.Read(w.call)
	if err != nil {
		t.Fatal(err)
	}
	stAsk, _ := receipts.Decode(w.list[1])
	stAccept, _ := receipts.Decode(w.list[2])
	stRelease, _ := receipts.Decode(w.list[3])
	askPair := [2]string{stAsk.Predicate.From, stAsk.Predicate.To}
	// answered is the note and the ask, then the closing and, when release is true, its release, each
	// decided by the call b.
	answered := func(b []byte, closing receipts.Predicate, release bool) [][]byte {
		s := newSigner(t)
		s.list = append(s.list, w.list[0], w.list[1])
		s.chains[askPair], s.ids[askPair] = 2, stAsk.ID
		closing.By = receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(b)}
		sub := stAccept.Subject
		if closing.Verdict == "block" {
			sub = stAsk.Subject
		}
		s.add(t, sub, closing)
		if release {
			rl := stRelease.Predicate
			rl.By = closing.By
			s.add(t, stRelease.Subject, rl)
		}
		return s.list
	}
	refusal := func(reason string) receipts.Predicate {
		p := stAccept.Predicate
		p.Verdict, p.Reason = "block", reason
		return p
	}
	for name, c := range map[string]struct {
		call    answer.Call
		key     string
		closing receipts.Predicate
		release bool
		other   bool // the card's module is not the trusted one, so row 3 does not fail first
		index   int
		want    []Witness
	}{
		"another Accord": {answer.Call{Accord: "sha256:" + strings.Repeat("ab", 32), Question: call.Question, Action: call.Action, Digest: call.Digest, Release: true},
			"decider", stAccept.Predicate, true, false, 2, []Witness{"", "", Contradicted, Contradicted}},
		"another member's accept": {answer.Call{Accord: call.Accord, Question: call.Question, Action: call.Action, Digest: call.Digest, Release: true},
			"proposer", stAccept.Predicate, true, false, 2, []Witness{"", "", Contradicted, Contradicted}},
		"a release the call does not ask for": {answer.Call{Accord: call.Accord, Question: call.Question, Action: call.Action, Digest: call.Digest},
			"decider", stAccept.Predicate, true, false, 3, []Witness{"", "", Witnessed, Contradicted}},
		"a release the call asks for, not there": {answer.Call{Accord: call.Accord, Question: call.Question, Action: call.Action, Digest: call.Digest, Release: true},
			"decider", stAccept.Predicate, false, false, 2, []Witness{"", "", Contradicted}},
		"a decline by a stranger": {answer.Call{Accord: call.Accord, Question: call.Question, Action: answer.Declined, From: ptesting.Key(t, "stranger").Reference()},
			"stranger", refusal(receipts.ReasonDeclined), false, true, 2, []Witness{"", "", Contradicted}},
		"a cancel by the decider": {answer.Call{Accord: call.Accord, Question: call.Question, Action: answer.Cancelled, From: ptesting.Key(t, "decider").Reference()},
			"decider", refusal(receipts.ReasonCancelled), false, true, 2, []Witness{"", "", Contradicted}},
	} {
		b, err := answer.Sign(c.call, ptesting.Key(t, c.key))
		if err != nil {
			t.Fatal(err)
		}
		e := w.evidence()
		e.Receipts, e.Answers = answered(b, c.closing, c.release), [][]byte{b}
		if c.other {
			e.Kernel = composition.KernelHash()
		}
		r, err := Check(e)
		var got []Witness
		for _, d := range r.Decisions {
			got = append(got, d.Witness)
		}
		if !isCode(err, CodeWitness, c.index) || strings.Join(ws(got), ",") != strings.Join(ws(c.want), ",") {
			t.Errorf("%s: got %v with witnesses %v, want witness at %d with %v", name, err, got, c.index, c.want)
		}
	}
}

// TestABodyOfNoReceiptAfterAClose gives a list that ends in its close a delivery whose receiptId is
// empty, and one with none: the close's statement is the zero one, but a delivery names a receipt of
// a decision, so each fails body at index -1 (verify-1 row 5).
func TestABodyOfNoReceiptAfterAClose(t *testing.T) {
	t.Parallel()
	list, set, accord, _ := closedJournal(t, "2026-11-12T09:30:00Z")
	r, err := Check(Evidence{Accord: accord, Founding: set.Set, Openings: set.Openings, Acceptances: set.Acceptances, Receipts: list, Kernel: kernelOf(set.Set)})
	if err != nil || r.Close == "" {
		t.Fatalf("a closed journal: %+v, %v", r, err)
	}
	for name, d := range map[string]string{
		"an empty receiptId": `{"receiptId":"","kind":"proposal","salt":"` + strings.Repeat("00", 32) + `","text":"x"}`,
		"no receiptId":       `{"kind":"proposal","salt":"` + strings.Repeat("00", 32) + `","text":"x"}`,
	} {
		if err := Body(r, []byte(d)); !isCode(err, CodeBody, -1) {
			t.Errorf("%s: got %v, want body at -1", name, err)
		}
	}
}
