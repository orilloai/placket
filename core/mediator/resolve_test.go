package mediator_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// salt16 is a salt of 16 bytes for an answer.
var salt16 = []byte("an answer's salt")

// ask gives a proposal from the proposer to the applier at seq and at, which the ask-and-log terms
// ask the decider about, and returns the question's id.
func ask(t testing.TB, m *mediator.Mediator, seq int64, id, at string) string {
	t.Helper()
	p, ap := key(t, "proposer"), key(t, "applier")
	msg := message(t, m.Accord(), id, ap.Reference(), "proposal", "Proposal "+id+".", nil)
	out, err := m.Give(p.Reference(), msg, envelope(t, seq, msg, p), at)
	if err != nil || out.Verdict != "escalate" {
		t.Fatalf("the ask: %+v, %v", out, err)
	}
	return out.ReceiptID
}

func accept(release bool) mediator.Resolution {
	return mediator.Resolution{Action: mediator.Accepted, Answer: "accepted", Salt: salt16, Release: release}
}

func change(release bool) mediator.Resolution {
	return mediator.Resolution{Action: mediator.Changed, Answer: "Rename total to amount_due.", Salt: salt16, Release: release}
}

func errorCode(err error, code mediator.ErrorCode) bool {
	var e *mediator.Error
	return errors.As(err, &e) && e.Code == code
}

// unchanged fails the test when the mediator's receipts or open questions differ from before.
func unchanged(t *testing.T, m *mediator.Mediator, receiptsBefore [][]byte, pendingBefore []string) {
	t.Helper()
	if !reflect.DeepEqual(m.Receipts(), receiptsBefore) || !reflect.DeepEqual(m.Pending(), pendingBefore) {
		t.Fatalf("a refused answer changed the mediator: %d receipts, pending %v", len(m.Receipts()), m.Pending())
	}
}

// statement verifies the mediator's receipts and returns the statement of the receipt whose id is id.
func statement(t *testing.T, m *mediator.Mediator, id string) receipts.Statement {
	t.Helper()
	st, err := receipts.Verify(key(t, "mediator").Reference(), m.Receipts())
	if err != nil {
		t.Fatalf("the receipts do not verify: %v", err)
	}
	for _, s := range st {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no receipt %s", id)
	return receipts.Statement{}
}

// TestAStrangersAnswerIsNotAMember is row 1 of section 6: a key with no declaration answers an open
// question and is rejected with not-a-member, writing nothing (section 6).
func TestAStrangersAnswerIsNotAMember(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	before, pending := m.Receipts(), m.Pending()
	_, err := m.Resolve(key(t, "stranger").Reference(), q, accept(true), "2026-11-12T09:01:00Z")
	if !rejected(err, mediator.NotAMember) || err.Error() != "mediator: rejected: not-a-member" {
		t.Fatalf("got %v", err)
	}
	unchanged(t, m, before, pending)
}

// TestTheProposerOnAClosedQuestion is maybe-7 with row 2 before row 3: the proposer, who holds no
// answering role, answers a question the decider has already declined, and learns only that no
// question is open, not that it may not answer.
func TestTheProposerOnAClosedQuestion(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	if _, err := m.Resolve(key(t, "decider").Reference(), q, mediator.Resolution{Action: mediator.Declined}, "2026-11-12T09:01:00Z"); err != nil {
		t.Fatal(err)
	}
	before, pending := m.Receipts(), m.Pending()
	_, err := m.Resolve(key(t, "proposer").Reference(), q, accept(false), "2026-11-12T09:02:00Z")
	if !rejected(err, mediator.NoOpenQuestion) || err.Error() != "mediator: rejected: no-open-question" {
		t.Fatalf("got %v", err)
	}
	unchanged(t, m, before, pending)
}

// TestTheProposerIsNotAnAnswerer is maybe-6: in the ask-and-log Accord only the decider holds
// decider, so the proposer's accept of its own open question is not-an-answerer, writes nothing and
// leaves the question open.
func TestTheProposerIsNotAnAnswerer(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	before, pending := m.Receipts(), m.Pending()
	_, err := m.Resolve(key(t, "proposer").Reference(), q, accept(true), "2026-11-12T09:01:00Z")
	if !rejected(err, mediator.NotAnAnswerer) || err.Error() != "mediator: rejected: not-an-answerer" {
		t.Fatalf("got %v", err)
	}
	unchanged(t, m, before, pending)
	if len(pending) != 1 || pending[0] != q {
		t.Fatalf("pending %v", pending)
	}
}

// TestAnAskerWhoHoldsTheRoleAnswers is maybe-6's last sentence: when the proposer's binding also
// gives it decider, it answers its own question. Its accept is decided with the proposer as both
// sender and receiver, whose terms give and get an answer, and passes.
func TestAnAskerWhoHoldsTheRoleAnswers(t *testing.T) {
	t.Parallel()
	parties := askAndLogParties(3600)
	parties[0].roles = []string{"proposer", "decider"}
	parties[0].terms = append(parties[0].terms, term("want", "give", []string{"proposer"}, []string{"answer"}))
	m := foundSet(t, buildSet(t, askAndLogRoles, parties...))
	p := key(t, "proposer")
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	if l, ok := m.Answerers(q); !ok || len(l) != 2 {
		t.Fatalf("answerers %v, %v", l, ok)
	}
	outs, err := m.Resolve(p.Reference(), q, accept(false), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "pass" || outs[0].Reason != "ok" {
		t.Fatalf("got %+v, %v", outs, err)
	}
	st := statement(t, m, outs[0].ReceiptID)
	if st.Predicate.Kind != receipts.KindResolution || st.Predicate.From != p.Reference() || st.Predicate.To != p.Reference() ||
		st.Subject.Name != receipts.NameAnswer || st.Predicate.Envelope.Sender != p.Reference() {
		t.Fatalf("the resolution receipt: %+v", st)
	}
	if len(m.Pending()) != 0 {
		t.Fatalf("pending %v", m.Pending())
	}
	d, err := m.Get(p.Reference())
	if err != nil || d == nil {
		t.Fatalf("the answer is not queued for the proposer: %s, %v", d, err)
	}
}

// misuse answers an open question as the decider with r and checks that it is refused as the
// caller's misuse with code, an *Error and never a *Rejection, and that the question is untouched
// (section 6).
func misuse(t *testing.T, r mediator.Resolution, code mediator.ErrorCode) {
	t.Helper()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	before, pending := m.Receipts(), m.Pending()
	_, err := m.Resolve(key(t, "decider").Reference(), q, r, "2026-11-12T09:01:00Z")
	var rej *mediator.Rejection
	if errors.As(err, &rej) || !errorCode(err, code) || err.Error() != "mediator: "+string(code) {
		t.Fatalf("got %v, want the error %s", err, code)
	}
	unchanged(t, m, before, pending)
}

// TestAShortSaltIsMisuse answers with a salt of 15 bytes: an accept and a change are refused with the
// error salt.
func TestAShortSaltIsMisuse(t *testing.T) {
	t.Parallel()
	short := salt16[:15]
	misuse(t, mediator.Resolution{Action: mediator.Accepted, Answer: "accepted", Salt: short}, mediator.CodeSalt)
	misuse(t, mediator.Resolution{Action: mediator.Changed, Answer: "A version.", Salt: short, Release: true}, mediator.CodeSalt)
	misuse(t, mediator.Resolution{Action: mediator.Accepted, Answer: "accepted"}, mediator.CodeSalt)
}

// TestAnEmptyChange is maybe-9's scenario: a change with an empty version is the caller's misuse,
// the error answer; nothing is written and the question stays open.
func TestAnEmptyChange(t *testing.T) {
	t.Parallel()
	misuse(t, mediator.Resolution{Action: mediator.Changed, Salt: salt16}, mediator.CodeAnswer)
	misuse(t, mediator.Resolution{Action: mediator.Changed, Salt: salt16, Release: true}, mediator.CodeAnswer)
}

// TestAnUnknownActionIsMisuse answers with an action that is not one of the four, before its salt
// is read: the error action.
func TestAnUnknownActionIsMisuse(t *testing.T) {
	t.Parallel()
	misuse(t, mediator.Resolution{Action: "approved", Answer: "yes", Salt: salt16}, mediator.CodeAction)
	misuse(t, mediator.Resolution{Action: "undeliverable"}, mediator.CodeAction)
	misuse(t, mediator.Resolution{Action: "Accepted", Answer: "yes", Salt: salt16[:3]}, mediator.CodeAction)
	misuse(t, mediator.Resolution{}, mediator.CodeAction)
}

// TestTheRowsComeBeforeMisuse answers with a short salt as members the rows refuse: the rows decide
// (section 6), and an answer at a bad time is the error at before any row (check-order).
func TestTheRowsComeBeforeMisuse(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	bad := mediator.Resolution{Action: "approved", Salt: salt16[:1]}
	if _, err := m.Resolve(key(t, "stranger").Reference(), q, bad, "2026-11-12T09:01:00Z"); !rejected(err, mediator.NotAMember) {
		t.Fatalf("a stranger: %v", err)
	}
	if _, err := m.Resolve(key(t, "proposer").Reference(), q, bad, "2026-11-12T09:01:00Z"); !rejected(err, mediator.NotAnAnswerer) {
		t.Fatalf("the proposer: %v", err)
	}
	if _, err := m.Resolve(key(t, "decider").Reference(), "sha256:"+q[7:39]+q[7:39], bad, "2026-11-12T09:01:00Z"); !rejected(err, mediator.NoOpenQuestion) {
		t.Fatalf("another id: %v", err)
	}
	if _, err := m.Resolve(key(t, "stranger").Reference(), q, bad, "2026-11-12 09:01:00"); !errorCode(err, mediator.CodeAt) || err.Error() != "mediator: at" {
		t.Fatalf("a bad time: %v", err)
	}
	if _, err := m.Undeliverable(q, "2026-11-12T09:01:00"); !errorCode(err, mediator.CodeAt) {
		t.Fatalf("Undeliverable at a bad time: %v", err)
	}
	if _, err := m.Tick("2026-11-12T25:00:00Z"); !errorCode(err, mediator.CodeAt) {
		t.Fatalf("Tick at a bad time: %v", err)
	}
	if len(m.Receipts()) != 1 || len(m.Pending()) != 1 {
		t.Fatal("a refusal changed the mediator")
	}
}

// nobodyAnswers is an Accord whose maybe rule names auditor, a role no binding gives and the policy
// does not list, so founding accepts it (setup-contract-5).
func nobodyAnswers(t testing.TB) *mediator.Mediator {
	t.Helper()
	parties := askAndLogParties(3600)
	parties[0].terms[0] = maybe("give", []string{"applier"}, []string{"proposal"}, []string{"auditor"}, 3600)
	return foundSet(t, buildSet(t, askAndLogRoles, parties...))
}

// TestNobodyCanAnswer is maybe-4's scenario: a question whose rule names a role no member holds has
// an empty list of answerers, not nil, and the mediator's finding that it cannot be delivered is
// from the mediator's own reference to the asker, block, undeliverable.
func TestNobodyCanAnswer(t *testing.T) {
	t.Parallel()
	m := nobodyAnswers(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	l, ok := m.Answerers(q)
	if !ok || l == nil || len(l) != 0 {
		t.Fatalf("answerers %#v, %v", l, ok)
	}
	out, err := m.Undeliverable(q, "2026-11-12T09:00:00Z")
	if err != nil || out.Verdict != "block" || out.Reason != receipts.ReasonUndeliverable {
		t.Fatalf("got %+v, %v", out, err)
	}
	st := statement(t, m, out.ReceiptID)
	if st.Predicate.From != key(t, "mediator").Reference() || st.Predicate.To != key(t, "proposer").Reference() ||
		st.Predicate.Kind != receipts.KindResolution || st.Subject.Name != "proposal" {
		t.Fatalf("the receipt: %+v", st)
	}
	if _, ok := m.Answerers(q); ok {
		t.Fatal("a closed question has answerers")
	}
	if _, err := m.Undeliverable(q, "2026-11-12T09:00:01Z"); !rejected(err, mediator.NoOpenQuestion) {
		t.Fatalf("a second finding: %v", err)
	}
}

// TestADeadlineAtTheLastTime is maybe-1's last time with Tick: under the largest timeout the deadline
// is 9999-12-31T23:59:59Z, a tick a second before it writes nothing, and a tick at it closes the
// question with a timeout whose time is that last second, with no overflow.
func TestADeadlineAtTheLastTime(t *testing.T) {
	t.Parallel()
	m := foundSet(t, buildSet(t, askAndLogRoles, askAndLogParties(1<<53-1)...))
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	if outs, err := m.Tick("9999-12-31T23:59:58Z"); err != nil || len(outs) != 0 || len(m.Pending()) != 1 {
		t.Fatalf("a tick before the deadline: %+v, %v", outs, err)
	}
	outs, err := m.Tick("9999-12-31T23:59:59Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "block" || outs[0].Reason != receipts.ReasonTimeout {
		t.Fatalf("got %+v, %v", outs, err)
	}
	st := statement(t, m, outs[0].ReceiptID)
	if st.Predicate.At != "9999-12-31T23:59:59Z" || st.Predicate.From != key(t, "decider").Reference() || st.Predicate.Envelope.Seq != 1 {
		t.Fatalf("the timeout: %+v", st.Predicate)
	}
	if len(m.Pending()) != 0 {
		t.Fatalf("pending %v", m.Pending())
	}
	if _, err := m.Resolve(key(t, "decider").Reference(), q, accept(false), "9999-12-31T23:59:59Z"); !rejected(err, mediator.NoOpenQuestion) {
		t.Fatalf("an answer after the timeout: %v", err)
	}
}

// closedQueuesNothing checks that a closing queued nothing for the applier, the question's
// receiver, and that no receipt id in written has a queued delivery, but keep, whose delivery is for
// the proposer.
func closedQueuesNothing(t *testing.T, m *mediator.Mediator, written []mediator.Outcome, keep string) {
	t.Helper()
	if d, err := m.Get(key(t, "applier").Reference()); d != nil || err != nil {
		t.Fatalf("the receiver's fetch: %s, %v", d, err)
	}
	if len(written) == 0 {
		t.Fatal("the closing wrote nothing")
	}
	for _, o := range written {
		q, ok := m.Queued(o.ReceiptID)
		switch {
		case o.ReceiptID == keep && (!ok || q.To != key(t, "proposer").Reference()):
			t.Fatalf("the answer is not queued for the proposer: %v", ok)
		case o.ReceiptID != keep && ok:
			t.Fatalf("receipt %s queued a delivery for %s", o.ReceiptID, q.To)
		}
	}
	if len(m.Pending()) != 0 {
		t.Fatalf("pending %v", m.Pending())
	}
}

// TestATimeoutQueuesNothing closes a question by Tick: nothing for the receiver (release-in-resolve).
func TestATimeoutQueuesNothing(t *testing.T) {
	t.Parallel()
	m := found(t)
	ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	outs, err := m.Tick("2026-11-12T10:00:00Z")
	if err != nil || len(outs) != 1 {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, "")
}

// TestAnUndeliverableQueuesNothing closes a question by Undeliverable: nothing for the receiver.
func TestAnUndeliverableQueuesNothing(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	out, err := m.Undeliverable(q, "2026-11-12T09:01:00Z")
	if err != nil {
		t.Fatal(err)
	}
	st := statement(t, m, out.ReceiptID)
	if st.Predicate.From != key(t, "decider").Reference() {
		t.Fatalf("an undeliverable with an answerer is from %s", st.Predicate.From)
	}
	closedQueuesNothing(t, m, []mediator.Outcome{out}, "")
}

// TestADeclineQueuesNothing closes a question by a decline asking for release: nothing for the
// receiver.
func TestADeclineQueuesNothing(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	outs, err := m.Resolve(key(t, "decider").Reference(), q, mediator.Resolution{Action: mediator.Declined, Release: true}, "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonDeclined {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, "")
}

// TestACancelQueuesNothing closes a question by a cancel: nothing for the receiver.
func TestACancelQueuesNothing(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	outs, err := m.Resolve(key(t, "decider").Reference(), q, mediator.Resolution{Action: mediator.Cancelled}, "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonCancelled {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, "")
}

// TestAnAcceptWithoutReleaseQueuesOnlyTheAnswer closes a question by an accept that does not ask for
// release: the answer is queued for the proposer, and nothing for the receiver.
func TestAnAcceptWithoutReleaseQueuesOnlyTheAnswer(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	outs, err := m.Resolve(key(t, "decider").Reference(), q, accept(false), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "pass" {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, outs[0].ReceiptID)
}

// TestABlockedAcceptReleasesNothing is maybe-3's "an accept the asker's terms do not take", asking
// for release: the proposer has no term to get an answer, so the resolution is block, no-match, no
// release receipt follows, and nothing is queued for anyone.
func TestABlockedAcceptReleasesNothing(t *testing.T) {
	t.Parallel()
	parties := askAndLogParties(3600)
	parties[0].terms = parties[0].terms[:2]
	m := foundSet(t, buildSet(t, askAndLogRoles, parties...))
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	outs, err := m.Resolve(key(t, "decider").Reference(), q, accept(true), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "block" || outs[0].Reason != "no-match" {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, "")
	if d, _ := m.Get(key(t, "proposer").Reference()); d != nil {
		t.Fatalf("the proposer holds %s", d)
	}
	if len(m.Receipts()) != 2 {
		t.Fatalf("%d receipts", len(m.Receipts()))
	}
}

// changeReleasedWithin answers with a change asking for release in an Accord whose question goes
// to the party named, and checks that one outcome is returned, no release receipt is
// written, and the version is queued for the asker only (maybe-10).
func changeReleasedWithin(t *testing.T, receiver string) {
	t.Helper()
	parties := askAndLogParties(3600)
	// The proposer asks the decider about a proposal to the receiver; the receiver gets it.
	parties[0].terms[0] = maybe("give", []string{receiver}, []string{"proposal"}, []string{"decider"}, 3600)
	for i := range parties {
		if parties[i].name == receiver {
			parties[i].terms = append(parties[i].terms, term("want", "get", []string{"proposer"}, []string{"proposal"}))
		}
	}
	m := foundSet(t, buildSet(t, askAndLogRoles, parties...))
	p, to := key(t, "proposer"), key(t, receiver)
	msg := message(t, m.Accord(), "p-1", to.Reference(), "proposal", "A proposal.", nil)
	ask, err := m.Give(p.Reference(), msg, envelope(t, 1, msg, p), "2026-11-12T09:00:00Z")
	if err != nil || ask.Verdict != "escalate" {
		t.Fatalf("the ask: %+v, %v", ask, err)
	}
	outs, err := m.Resolve(key(t, "decider").Reference(), ask.ReceiptID, change(true), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "pass" {
		t.Fatalf("got %+v, %v", outs, err)
	}
	if len(m.Receipts()) != 2 {
		t.Fatalf("%d receipts: a release receipt was written", len(m.Receipts()))
	}
	got := map[string]int{}
	for _, n := range []string{"proposer", "applier", "decider"} {
		for {
			d, err := m.Get(key(t, n).Reference())
			if err != nil || d == nil {
				break
			}
			got[n]++
		}
	}
	if !reflect.DeepEqual(got, map[string]int{"proposer": 1}) {
		t.Fatalf("deliveries %v", got)
	}
}

// TestAChangeReleasedToItsAnswerer is maybe-10 with the decider as the one the proposal goes to and
// the one who answers: the change releases nothing, and only the asker holds the version.
func TestAChangeReleasedToItsAnswerer(t *testing.T) {
	t.Parallel()
	changeReleasedWithin(t, "decider")
}

// TestAChangeReleasedToItsAsker is maybe-10 with the proposal going to the proposer itself:
// the change releases nothing, and the asker holds the version once.
func TestAChangeReleasedToItsAsker(t *testing.T) {
	t.Parallel()
	changeReleasedWithin(t, "proposer")
}

// TestTheNewReadsAreCopies changes what Resolve, Tick, Answerers, Queued and Result return, and reads
// the mediator again (results-are-copies).
func TestTheNewReadsAreCopies(t *testing.T) {
	t.Parallel()
	m := found(t)
	d := key(t, "decider").Reference()
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	l, _ := m.Answerers(q)
	l[0] = "changed"
	if again, _ := m.Answerers(q); again[0] != d {
		t.Fatal("Answerers shares the mediator's state")
	}
	outs, err := m.Resolve(d, q, accept(true), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 2 {
		t.Fatalf("got %+v, %v", outs, err)
	}
	want := m.Receipts()
	outs[0].Receipt[0], outs[1].Receipt[0] = 'x', 'x'
	qd, ok := m.Queued(outs[1].ReceiptID)
	if !ok {
		t.Fatal("the release is not queued")
	}
	kept := append([]byte(nil), qd.Delivery...)
	qd.Delivery[0] = 'x'
	res, ok, err := m.Result(key(t, "proposer").Reference(), q)
	if err != nil || !ok || len(res) != 2 {
		t.Fatalf("Result: %+v, %v, %v", res, ok, err)
	}
	res[0].Receipt[0] = 'x'
	if !reflect.DeepEqual(m.Receipts(), want) {
		t.Fatal("an outcome shares the mediator's receipts")
	}
	if again, _ := m.Queued(outs[1].ReceiptID); !bytes.Equal(again.Delivery, kept) {
		t.Fatal("Queued shares the mediator's queue")
	}
	ap := key(t, "applier").Reference()
	if got, _ := m.Get(ap); !bytes.Equal(got, kept) {
		t.Fatal("the fetched delivery is not what Queued returned")
	}
	if _, ok := m.Queued(outs[1].ReceiptID); ok {
		t.Fatal("a fetched delivery is still queued")
	}
	ask(t, m, 2, "p-2", "2026-11-12T09:02:00Z")
	ticks, err := m.Tick("2026-11-12T10:02:00Z")
	if err != nil || len(ticks) != 1 {
		t.Fatalf("Tick: %+v, %v", ticks, err)
	}
	ticks[0].Receipt[0] = 'x'
	if r := m.Receipts(); r[len(r)-1][0] != '{' {
		t.Fatal("a tick's outcome shares the mediator's receipts")
	}
}
