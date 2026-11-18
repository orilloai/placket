package mediator_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/answer"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// answerSalt is a salt of SaltBytes for an answer.
var answerSalt = saltOf("an answer's salt")

// ask gives a proposal from the proposer to the applier at seq and at, which the ask-and-log terms
// ask the decider about, and returns the question's id.
func ask(t testing.TB, m *mediator.Mediator, seq int64, id, at string) string {
	t.Helper()
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	msg := message(t, m.Accord(), id, ap.Reference(), "proposal", "Proposal "+id+".", nil)
	out, err := m.Give(p.Reference(), msg, envelope(t, seq, msg, p), nil, at)
	if err != nil || out.Verdict != "escalate" {
		t.Fatalf("the ask: %+v, %v", out, err)
	}
	return out.ReceiptID
}

// signer returns the vectors' key whose reference is ref, of the names the tests answer as, or the
// zero Key, which signs nothing.
func signer(t testing.TB, ref string) signing.Key {
	t.Helper()
	for _, n := range []string{"proposer", "applier", "decider", "lead", "stranger", "mediator"} {
		if k := ptesting.Key(t, n); k.Reference() == ref {
			return k
		}
	}
	return signing.Key{}
}

// callBy signs, as the key of ref, an answer call of the action on the question q under m's Accord,
// with the digest and release of an accept or a change (maybe-11). A key the tests do not name, and
// a mediator no one founded, sign none: the call is nil.
func callBy(t testing.TB, m *mediator.Mediator, ref, q string, action answer.Action, digest string, release bool) []byte {
	t.Helper()
	b, _ := answer.Sign(answer.Call{Accord: m.Accord(), Question: q, Action: action, Digest: digest, Release: release}, signer(t, ref))
	return b
}

// accept is ref's accept of q with the text accepted under answerSalt, asking for release or not.
func accept(t testing.TB, m *mediator.Mediator, ref, q string, release bool) mediator.Resolution {
	t.Helper()
	text := "accepted"
	return mediator.Resolution{Call: callBy(t, m, ref, q, answer.Accepted, receipts.Digest(answerSalt, []byte(text)), release), Text: text, Salt: answerSalt}
}

// change is ref's change of q to a version under answerSalt, asking for release or not.
func change(t testing.TB, m *mediator.Mediator, ref, q string, release bool) mediator.Resolution {
	t.Helper()
	text := "Rename total to amount_due."
	return mediator.Resolution{Call: callBy(t, m, ref, q, answer.Changed, receipts.Digest(answerSalt, []byte(text)), release), Text: text, Salt: answerSalt}
}

// decline is ref's decline of q.
func decline(t testing.TB, m *mediator.Mediator, ref, q string) mediator.Resolution {
	t.Helper()
	return mediator.Resolution{Call: callBy(t, m, ref, q, answer.Declined, "", false)}
}

// cancel is ref's cancel of q.
func cancel(t testing.TB, m *mediator.Mediator, ref, q string) mediator.Resolution {
	t.Helper()
	return mediator.Resolution{Call: callBy(t, m, ref, q, answer.Cancelled, "", false)}
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
	st, err := verifyAll(ptesting.Key(t, "mediator").Reference(), m.Receipts())
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
	_, err := m.Resolve(ptesting.Key(t, "stranger").Reference(), accept(t, m, ptesting.Key(t, "stranger").Reference(), q, true), "2026-11-12T09:01:00Z")
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
	if _, err := m.Resolve(ptesting.Key(t, "decider").Reference(), decline(t, m, ptesting.Key(t, "decider").Reference(), q), "2026-11-12T09:01:00Z"); err != nil {
		t.Fatal(err)
	}
	before, pending := m.Receipts(), m.Pending()
	_, err := m.Resolve(ptesting.Key(t, "proposer").Reference(), accept(t, m, ptesting.Key(t, "proposer").Reference(), q, false), "2026-11-12T09:02:00Z")
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
	_, err := m.Resolve(ptesting.Key(t, "proposer").Reference(), accept(t, m, ptesting.Key(t, "proposer").Reference(), q, true), "2026-11-12T09:01:00Z")
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
	p := ptesting.Key(t, "proposer")
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	if l, ok := m.Answerers(q); !ok || len(l) != 2 {
		t.Fatalf("answerers %v, %v", l, ok)
	}
	outs, err := m.Resolve(p.Reference(), accept(t, m, p.Reference(), q, false), "2026-11-12T09:01:00Z")
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

// misuse answers an open question as the decider with the Resolution make builds for the mediator
// and the question, and checks that it is rejected with code, a *Rejection, and that the question is
// untouched (section 6).
func misuse(t *testing.T, make func(m *mediator.Mediator, q string) mediator.Resolution, code mediator.RejectionCode) {
	t.Helper()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	before, pending := m.Receipts(), m.Pending()
	_, err := m.Resolve(ptesting.Key(t, "decider").Reference(), make(m, q), "2026-11-12T09:01:00Z")
	if !rejected(err, code) || err.Error() != "mediator: rejected: "+string(code) {
		t.Fatalf("got %v, want the rejection %s", err, code)
	}
	unchanged(t, m, before, pending)
}

// withSalt is the decider's accept or change of the question, its call's digest taken over the text
// under salt, given with the text and salt.
func withSalt(t *testing.T, action answer.Action, text string, salt []byte, release bool) func(*mediator.Mediator, string) mediator.Resolution {
	return func(m *mediator.Mediator, q string) mediator.Resolution {
		d := ptesting.Key(t, "decider").Reference()
		return mediator.Resolution{Call: callBy(t, m, d, q, action, receipts.Digest(salt, []byte(text)), release), Text: text, Salt: salt}
	}
}

// TestAShortSaltIsMisuse answers with a salt of 15 bytes, and with none: an accept and a change are
// rejected with answer-body, section 6's row 5.
func TestAShortSaltIsMisuse(t *testing.T) {
	t.Parallel()
	short := answerSalt[:15]
	misuse(t, withSalt(t, answer.Accepted, "accepted", short, false), mediator.AnswerBody)
	misuse(t, withSalt(t, answer.Changed, "A version.", short, true), mediator.AnswerBody)
	misuse(t, withSalt(t, answer.Accepted, "accepted", nil, false), mediator.AnswerBody)
}

// TestASaltOfAnotherLengthIsMisuse answers with salts of 16, 31 and 33 bytes: an accept and a change
// are each rejected with answer-body, write nothing and leave the question open. A salt of exactly
// SaltBytes closes it (section 6, maybe-8, maybe-9).
func TestASaltOfAnotherLengthIsMisuse(t *testing.T) {
	t.Parallel()
	for _, n := range []int{16, 31, 33} {
		salt := bytes.Repeat([]byte{7}, n)
		misuse(t, withSalt(t, answer.Accepted, "accepted", salt, false), mediator.AnswerBody)
		misuse(t, withSalt(t, answer.Changed, "A version.", salt, true), mediator.AnswerBody)
	}
	for _, action := range []answer.Action{answer.Accepted, answer.Changed} {
		m := found(t)
		q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
		r := withSalt(t, action, "A version.", answerSalt, false)(m, q)
		if len(r.Salt) != 32 {
			t.Fatalf("the answer's salt is %d bytes", len(r.Salt))
		}
		if outs, err := m.Resolve(ptesting.Key(t, "decider").Reference(), r, "2026-11-12T09:01:00Z"); err != nil || len(outs) != 1 {
			t.Fatalf("a %s with a salt of 32 bytes: %+v, %v", action, outs, err)
		}
		if len(m.Pending()) != 0 {
			t.Fatalf("a %s with a salt of 32 bytes left the question open", action)
		}
	}
}

// TestAnEmptyChange is maybe-9's scenario: a change with an empty version is rejected with
// answer-body; nothing is written and the question stays open.
func TestAnEmptyChange(t *testing.T) {
	t.Parallel()
	misuse(t, withSalt(t, answer.Changed, "", answerSalt, false), mediator.AnswerBody)
	misuse(t, withSalt(t, answer.Changed, "", answerSalt, true), mediator.AnswerBody)
}

// TestResolveRows holds section 6's five rows, each alone: a stranger is not-a-member; a call that
// does not read, is another's, or names another Accord is answer-call; a call for no open question
// is no-open-question; a member who may not take the action is not-an-answerer, an answerer's cancel
// among them; and a text or salt that does not fit the call is answer-body, a salt of 16 bytes among
// them (U5, U6). Each writes nothing and leaves the question open.
func TestResolveRows(t *testing.T) {
	t.Parallel()
	d, p := ptesting.Key(t, "decider").Reference(), ptesting.Key(t, "proposer").Reference()
	other := "sha256:" + strings.Repeat("ab", 32)
	for _, c := range []struct {
		name   string
		caller string
		make   func(m *mediator.Mediator, q string) mediator.Resolution
		code   mediator.RejectionCode
	}{
		{"a stranger", ptesting.Key(t, "stranger").Reference(), func(m *mediator.Mediator, q string) mediator.Resolution { return decline(t, m, d, q) }, mediator.NotAMember},
		{"bytes that are no call", d, func(*mediator.Mediator, string) mediator.Resolution { return mediator.Resolution{Call: []byte("{}")} }, mediator.AnswerCall},
		{"no call", d, func(*mediator.Mediator, string) mediator.Resolution { return mediator.Resolution{} }, mediator.AnswerCall},
		{"another's call", d, func(m *mediator.Mediator, q string) mediator.Resolution { return decline(t, m, p, q) }, mediator.AnswerCall},
		{"another Accord's call", d, func(m *mediator.Mediator, q string) mediator.Resolution {
			b, _ := answer.Sign(answer.Call{Accord: other, Question: q, Action: answer.Declined}, signer(t, d))
			return mediator.Resolution{Call: b}
		}, mediator.AnswerCall},
		{"a call edited after signing", d, func(m *mediator.Mediator, q string) mediator.Resolution {
			r := accept(t, m, d, q, true)
			r.Call = bytes.Replace(r.Call, []byte(`"release":true`), []byte(`"release":false`), 1)
			return r
		}, mediator.AnswerCall},
		{"no open question", d, func(m *mediator.Mediator, q string) mediator.Resolution { return decline(t, m, d, other) }, mediator.NoOpenQuestion},
		{"the proposer's accept", p, func(m *mediator.Mediator, q string) mediator.Resolution { return accept(t, m, p, q, true) }, mediator.NotAnAnswerer},
		{"the proposer's decline", p, func(m *mediator.Mediator, q string) mediator.Resolution { return decline(t, m, p, q) }, mediator.NotAnAnswerer},
		{"the answerer's cancel", d, func(m *mediator.Mediator, q string) mediator.Resolution { return cancel(t, m, d, q) }, mediator.NotAnAnswerer},
		{"a salt of 16 bytes", d, withSalt(t, answer.Accepted, "accepted", answerSalt[:16], true), mediator.AnswerBody},
		{"a text the digest does not name", d, func(m *mediator.Mediator, q string) mediator.Resolution {
			r := accept(t, m, d, q, true)
			r.Text = "accepted!"
			return r
		}, mediator.AnswerBody},
		{"a decline with a text", d, func(m *mediator.Mediator, q string) mediator.Resolution {
			r := decline(t, m, d, q)
			r.Text = "no"
			return r
		}, mediator.AnswerBody},
		{"a decline with a salt", d, func(m *mediator.Mediator, q string) mediator.Resolution {
			r := decline(t, m, d, q)
			r.Salt = answerSalt
			return r
		}, mediator.AnswerBody},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := found(t)
			q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
			before, pending := m.Receipts(), m.Pending()
			_, err := m.Resolve(c.caller, c.make(m, q), "2026-11-12T09:01:00Z")
			if !rejected(err, c.code) {
				t.Fatalf("got %v, want %s", err, c.code)
			}
			unchanged(t, m, before, pending)
		})
	}
}

// TestTheRowsComeBeforeMisuse answers with inputs that fail two rows at once: the earlier row decides
// (check-order). A call signed by another key for a question that is not open is answer-call; a
// stranger's bad call is not-a-member; the proposer's call with a short salt is not-an-answerer;
// and an answer at a bad time is the error at before any row.
func TestTheRowsComeBeforeMisuse(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	d, p := ptesting.Key(t, "decider").Reference(), ptesting.Key(t, "proposer").Reference()
	other := "sha256:" + q[7:39] + q[7:39]
	short := withSalt(t, answer.Accepted, "accepted", answerSalt[:1], false)
	if _, err := m.Resolve(d, decline(t, m, p, other), "2026-11-12T09:01:00Z"); !rejected(err, mediator.AnswerCall) {
		t.Fatalf("another's call for no open question: %v", err)
	}
	if _, err := m.Resolve(ptesting.Key(t, "stranger").Reference(), mediator.Resolution{Call: []byte("x")}, "2026-11-12T09:01:00Z"); !rejected(err, mediator.NotAMember) {
		t.Fatalf("a stranger: %v", err)
	}
	r := short(m, q)
	r.Call = callBy(t, m, p, q, answer.Accepted, receipts.Digest(r.Salt, []byte(r.Text)), false)
	if _, err := m.Resolve(p, r, "2026-11-12T09:01:00Z"); !rejected(err, mediator.NotAnAnswerer) {
		t.Fatalf("the proposer: %v", err)
	}
	if _, err := m.Resolve(d, withSalt(t, answer.Accepted, "accepted", answerSalt[:1], false)(m, other), "2026-11-12T09:01:00Z"); !rejected(err, mediator.NoOpenQuestion) {
		t.Fatalf("another id: %v", err)
	}
	if _, err := m.Resolve(ptesting.Key(t, "stranger").Reference(), r, "2026-11-12 09:01:00"); !errorCode(err, mediator.CodeAt) || err.Error() != "mediator: at" {
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
	if st.Predicate.From != ptesting.Key(t, "mediator").Reference() || st.Predicate.To != ptesting.Key(t, "proposer").Reference() ||
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
	if st.Predicate.At != "9999-12-31T23:59:59Z" || st.Predicate.From != ptesting.Key(t, "decider").Reference() || st.Predicate.Envelope.Seq != 1 {
		t.Fatalf("the timeout: %+v", st.Predicate)
	}
	if len(m.Pending()) != 0 {
		t.Fatalf("pending %v", m.Pending())
	}
	if _, err := m.Resolve(ptesting.Key(t, "decider").Reference(), accept(t, m, ptesting.Key(t, "decider").Reference(), q, false), "9999-12-31T23:59:59Z"); !rejected(err, mediator.NoOpenQuestion) {
		t.Fatalf("an answer after the timeout: %v", err)
	}
}

// closedQueuesNothing checks that a closing queued nothing for the applier, the question's
// receiver, and that no receipt id in written has a queued delivery, but keep, whose delivery is for
// the proposer.
func closedQueuesNothing(t *testing.T, m *mediator.Mediator, written []mediator.Outcome, keep string) {
	t.Helper()
	if d, err := m.Get(ptesting.Key(t, "applier").Reference()); d != nil || err != nil {
		t.Fatalf("the receiver's fetch: %s, %v", d, err)
	}
	if len(written) == 0 {
		t.Fatal("the closing wrote nothing")
	}
	for _, o := range written {
		q, ok := m.Queued(o.ReceiptID)
		switch {
		case o.ReceiptID == keep && (!ok || q.To != ptesting.Key(t, "proposer").Reference()):
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
	if st.Predicate.From != ptesting.Key(t, "decider").Reference() {
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
	outs, err := m.Resolve(ptesting.Key(t, "decider").Reference(), decline(t, m, ptesting.Key(t, "decider").Reference(), q), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonDeclined {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, "")
}

// TestACancelQueuesNothing closes a question by its asker's cancel: nothing for the receiver. The
// receipt is from the first answerer, and names the asker's call as its decider (maybe-4, U6).
func TestACancelQueuesNothing(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	p := ptesting.Key(t, "proposer").Reference()
	r := cancel(t, m, p, q)
	outs, err := m.Resolve(p, r, "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonCancelled {
		t.Fatalf("got %+v, %v", outs, err)
	}
	st := statement(t, m, outs[0].ReceiptID)
	if st.Predicate.From != ptesting.Key(t, "decider").Reference() || st.Predicate.By != (receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(r.Call)}) {
		t.Fatalf("the cancel's receipt: %+v", st.Predicate)
	}
	closedQueuesNothing(t, m, outs, "")
}

// TestNobodyCancelsAQuestionNoOneCanAnswer is maybe-6's cancel of a question whose rule names a role
// no member holds: the asker's cancel is not-an-answerer, nothing is written and the question stays
// open (U6).
func TestNobodyCancelsAQuestionNoOneCanAnswer(t *testing.T) {
	t.Parallel()
	m := nobodyAnswers(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	p := ptesting.Key(t, "proposer").Reference()
	before, pending := m.Receipts(), m.Pending()
	if _, err := m.Resolve(p, cancel(t, m, p, q), "2026-11-12T09:01:00Z"); !rejected(err, mediator.NotAnAnswerer) {
		t.Fatalf("got %v, want not-an-answerer", err)
	}
	unchanged(t, m, before, pending)
}

// TestAnAcceptWithoutReleaseQueuesOnlyTheAnswer closes a question by an accept that does not ask for
// release: the answer is queued for the proposer, and nothing for the receiver.
func TestAnAcceptWithoutReleaseQueuesOnlyTheAnswer(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	outs, err := m.Resolve(ptesting.Key(t, "decider").Reference(), accept(t, m, ptesting.Key(t, "decider").Reference(), q, false), "2026-11-12T09:01:00Z")
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
	outs, err := m.Resolve(ptesting.Key(t, "decider").Reference(), accept(t, m, ptesting.Key(t, "decider").Reference(), q, true), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "block" || outs[0].Reason != "no-match" {
		t.Fatalf("got %+v, %v", outs, err)
	}
	closedQueuesNothing(t, m, outs, "")
	if d, _ := m.Get(ptesting.Key(t, "proposer").Reference()); d != nil {
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
	p, to := ptesting.Key(t, "proposer"), ptesting.Key(t, receiver)
	msg := message(t, m.Accord(), "p-1", to.Reference(), "proposal", "A proposal.", nil)
	ask, err := m.Give(p.Reference(), msg, envelope(t, 1, msg, p), nil, "2026-11-12T09:00:00Z")
	if err != nil || ask.Verdict != "escalate" {
		t.Fatalf("the ask: %+v, %v", ask, err)
	}
	outs, err := m.Resolve(ptesting.Key(t, "decider").Reference(), change(t, m, ptesting.Key(t, "decider").Reference(), ask.ReceiptID, true), "2026-11-12T09:01:00Z")
	if err != nil || len(outs) != 1 || outs[0].Verdict != "pass" {
		t.Fatalf("got %+v, %v", outs, err)
	}
	if len(m.Receipts()) != 2 {
		t.Fatalf("%d receipts: a release receipt was written", len(m.Receipts()))
	}
	got := map[string]int{}
	for _, n := range []string{"proposer", "applier", "decider"} {
		for {
			d, err := m.Get(ptesting.Key(t, n).Reference())
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
	d := ptesting.Key(t, "decider").Reference()
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	l, _ := m.Answerers(q)
	l[0] = "changed"
	if again, _ := m.Answerers(q); again[0] != d {
		t.Fatal("Answerers shares the mediator's state")
	}
	outs, err := m.Resolve(d, accept(t, m, d, q, true), "2026-11-12T09:01:00Z")
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
	res, ok, err := m.Result(ptesting.Key(t, "proposer").Reference(), q)
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
	ap := ptesting.Key(t, "applier").Reference()
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
