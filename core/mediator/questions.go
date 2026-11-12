package mediator

import (
	"encoding/hex"
	"fmt"

	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
)

// Action is what an answer does to a question (section 6), spelled as the vector format spells it
// (conformance-vectors-6).
type Action string

// The four actions of an answer: an accept (maybe-3), a change (maybe-9), and a decline and a cancel
// (maybe-4).
const (
	Accepted  Action = "accepted"
	Changed   Action = "changed"
	Declined  Action = "declined"
	Cancelled Action = "cancelled"
)

// A Resolution is an answer's own inputs (section 6): its action; for an accept the answer's text
// and for a change the version, in Answer; the salt the mediator's caller minted for it; and whether
// it asks for release (maybe-10). A decline and a cancel read only the action. A Resolution holds an
// answer's text and salt, so it prints as a fixed text under every format verb (secrets-never-print,
// record 0006).
type Resolution struct {
	Action  Action
	Answer  string
	Salt    []byte
	Release bool
}

// Format prints the Resolution as a fixed text, whatever the verb.
func (Resolution) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("mediator.Resolution")) }

// GoString prints the Resolution as a fixed text.
func (Resolution) GoString() string { return "mediator.Resolution" }

// String prints the Resolution as a fixed text.
func (Resolution) String() string { return "mediator.Resolution" }

// A batch is what one operation writes: its receipts, their chains and the deliveries they queue,
// all built before any state changes (release-in-resolve).
type batch struct {
	m          *Mediator
	chains     map[[2]string]link
	receipts   [][]byte
	statements []receipts.Statement
	queued     []queuedFor
	closed     map[string]bool
	outs       []Outcome
}

type queuedFor struct {
	to string
	q  queued
}

func (m *Mediator) batch() *batch {
	return &batch{m: m, chains: map[[2]string]link{}, closed: map[string]bool{}, outs: []Outcome{}}
}

// write builds the next receipt of the pair from p.From to p.To (receipt-5), of the subject, with
// the predicate's other members as given, and returns it and its id.
func (b *batch) write(subject receipts.Subject, p receipts.Predicate) ([]byte, string, error) {
	pair := [2]string{p.From, p.To}
	last, ok := b.chains[pair]
	if !ok {
		last = b.m.chains[pair]
	}
	p.Accord, p.Card, p.Mediator = b.m.accord.ID(), b.m.accord.Card(), b.m.key.Reference()
	p.Seq, p.Prev = last.seq+1, last.id
	r, id, err := receipts.Build(subject, p, b.m.key)
	if err != nil {
		return nil, "", failure(CodeInternal)
	}
	b.chains[pair] = link{seq: p.Seq, id: id}
	b.receipts = append(b.receipts, r)
	b.statements = append(b.statements, receipts.Statement{ID: id, Subject: subject, Predicate: p})
	b.outs = append(b.outs, Outcome{Verdict: p.Verdict, Reason: p.Reason, ReceiptID: id, Receipt: append([]byte(nil), r...)})
	return r, id, nil
}

// queue queues for to the part, as its delivery carries it with the receipt that passed it.
func (b *batch) queue(to string, part map[string]any, receipt []byte, id string) error {
	d := deliver(part, receipt, id)
	if d == nil {
		return failure(CodeInternal)
	}
	b.queued = append(b.queued, queuedFor{to: to, q: queued{id: id, delivery: d}})
	return nil
}

// close writes the resolution receipt that closes q with a reason of its own (maybe-4, maybe-5):
// from from to the asker, block, naming the asked part and echoing the question's envelope.
func (b *batch) close(q *question, from, reason, at string) error {
	b.closed[q.id] = true
	_, _, err := b.write(q.subject, receipts.Predicate{
		Kind: receipts.KindResolution, From: from, To: q.asker, At: at,
		Envelope: q.envelope, Verdict: string(composition.Block), Reason: reason,
	})
	return err
}

// commit changes the state: every receipt the batch built, the chains, the deliveries, and the
// questions it closed, which are no longer open.
func (m *Mediator) commit(b *batch) {
	m.receipts = append(m.receipts, b.receipts...)
	m.statements = append(m.statements, b.statements...)
	for pair, l := range b.chains {
		m.chains[pair] = l
	}
	for _, x := range b.queued {
		m.queues[x.to] = append(m.queues[x.to], x.q)
	}
	open := m.questions[:0]
	for _, q := range m.questions {
		if !b.closed[q.id] {
			open = append(open, q)
		}
	}
	for i := len(open); i < len(m.questions); i++ {
		m.questions[i] = nil
	}
	m.questions = open
}

// open returns the question that is open under id at the time at, one whose deadline is after at
// (maybe-7), or nil. Two times that keep the timestamp rule compare as strings as they do in time.
func (m *Mediator) open(id, at string) *question {
	for _, q := range m.questions {
		if q.id == id && at < q.deadline {
			return q
		}
	}
	return nil
}

// answerers returns the members whose bindings give them one of the roles, in actor-reference order
// (section 6): an empty list, not nil, when no member holds one.
func (m *Mediator) answerers(roles []string) []string {
	out := []string{}
	for _, member := range m.accord.Members() {
		for _, r := range m.accord.Roles(member) {
			if contains(roles, r) {
				out = append(out, member)
				break
			}
		}
	}
	return out
}

// firstAnswerer returns the first of the roles' answerers, or the mediator's own reference when no
// member holds one (maybe-4, maybe-5).
func (m *Mediator) firstAnswerer(roles []string) string {
	if l := m.answerers(roles); len(l) > 0 {
		return l[0]
	}
	return m.key.Reference()
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// Answerers returns the answerers of the question open under questionID: the members whose bindings
// give them a role its rule names, in actor-reference order, as a copy. The list is empty, not nil,
// when no member holds one (section 6, maybe-4, record 0006). An id that names no open question
// returns nil and false.
func (m *Mediator) Answerers(questionID string) ([]string, bool) {
	if !m.founded() {
		return nil, false
	}
	for _, q := range m.questions {
		if q.id == questionID {
			return m.answerers(q.roles), true
		}
	}
	return nil, false
}

// Resolve answers the question questionID as caller at the time at (section 6). Its rows run in
// order: the caller is a member (not-a-member), the question is open at a time before its deadline
// (no-open-question, maybe-7), and the caller is one of its answerers (not-an-answerer, maybe-6). A
// rejected answer writes nothing. The answer's own inputs are checked next, and their misuse is an
// *Error, writing nothing: an action that is not one of the four (action), an accept or a change with a salt
// shorter than MinSaltBytes (salt), a change with an empty version (answer).
//
// An accept forms the answer and a change the version (maybe-8, maybe-9), a part decided by its kind
// through composition from the answerer to the asker as a part the mediator formed; its resolution
// receipt holds composition's verdict, and a pass queues the part for the asker. A decline and a
// cancel write a block from the first answerer (maybe-4). An accept or a change that passes and asks
// for release then writes the release receipt (maybe-10). Every receipt is built before any state
// changes, the resolution first, and both are returned in that order.
func (m *Mediator) Resolve(caller, questionID string, r Resolution, at string) ([]Outcome, error) {
	if !m.founded() {
		return nil, failure(CodeUnfounded)
	}
	if !shape.Timestamp(at) {
		return nil, failure(CodeAt)
	}
	// Row 1: the caller is a member (setup-contract-6).
	if !m.accord.Member(caller) {
		return nil, &Rejection{Code: NotAMember}
	}
	// Row 2: an open question, at a time before its deadline (maybe-7).
	q := m.open(questionID, at)
	if q == nil {
		return nil, &Rejection{Code: NoOpenQuestion}
	}
	// Row 3: the caller is among its answerers (maybe-6).
	if !contains(m.answerers(q.roles), caller) {
		return nil, &Rejection{Code: NotAnAnswerer}
	}
	// The answer's own inputs, the caller's to get right.
	switch r.Action {
	case Accepted, Changed:
		if len(r.Salt) < MinSaltBytes {
			return nil, failure(CodeSalt)
		}
		if r.Action == Changed && r.Answer == "" {
			return nil, failure(CodeAnswer)
		}
	case Declined, Cancelled:
	default:
		return nil, failure(CodeAction)
	}

	b := m.batch()
	if r.Action == Declined || r.Action == Cancelled {
		reason := receipts.ReasonDeclined
		if r.Action == Cancelled {
			reason = receipts.ReasonCancelled
		}
		if err := b.close(q, m.firstAnswerer(q.roles), reason, at); err != nil {
			return nil, err
		}
		m.commit(b)
		return b.outs, nil
	}

	// The answer, or the version: one part the mediator forms (maybe-8, maybe-9).
	kind, prefix := receipts.NameAnswer, "answer:"
	if r.Action == Changed {
		kind, prefix = receipts.NameChanged, "change:"
	}
	digest := receipts.Digest(r.Salt, []byte(r.Answer))
	subject := receipts.Subject{Name: kind, Digest: digest}
	part := map[string]any{"messageId": prefix + q.id, "kind": kind, "salt": hex.EncodeToString(r.Salt), "text": r.Answer, "re": q.id}
	d := composition.Decide(m.accord, composition.Crossing{From: caller, To: q.asker, Kind: kind, Formed: true})
	b.closed[q.id] = true
	receipt, id, err := b.write(subject, receipts.Predicate{
		Kind: receipts.KindResolution, From: caller, To: q.asker, At: at,
		Envelope: q.envelope, Verdict: string(d.Verdict), Reason: d.Reason,
	})
	if err != nil {
		return nil, err
	}
	if d.Verdict == composition.Pass {
		if err := b.queue(q.asker, part, receipt, id); err != nil {
			return nil, err
		}
		if r.Release {
			if err := m.release(b, q, r.Action, caller, subject, part, at); err != nil {
				return nil, err
			}
		}
	}
	m.commit(b)
	return b.outs, nil
}

// release writes the release receipt of an answer that passed and asked for release, into b
// (maybe-10). After an accept, the held part is decided again, as given, from the asker to the
// receiver: a block is written as composition returns it, and any other verdict is the answer's
// pass, which queues the held part for the receiver as it was given. After a change, the same version
// is decided from the answerer to the receiver as a part the mediator formed, and a pass queues it
// for the receiver; when the receiver is the answerer or the asker, a change releases nothing.
func (m *Mediator) release(b *batch, q *question, action Action, answerer string, version receipts.Subject, part map[string]any, at string) error {
	if action == Accepted {
		d := composition.Decide(m.accord, composition.Crossing{From: q.asker, To: q.receiver, Kind: q.subject.Name})
		verdict, reason := composition.Pass, composition.ReasonOK
		if d.Verdict == composition.Block {
			verdict, reason = d.Verdict, d.Reason
		}
		receipt, id, err := b.write(q.subject, receipts.Predicate{
			Kind: receipts.KindCrossing, From: q.asker, To: q.receiver, At: at,
			Envelope: q.envelope, Verdict: string(verdict), Reason: reason,
		})
		if err != nil || verdict != composition.Pass {
			return err
		}
		return b.queue(q.receiver, q.delivery, receipt, id)
	}
	if q.receiver == answerer || q.receiver == q.asker {
		return nil
	}
	d := composition.Decide(m.accord, composition.Crossing{From: answerer, To: q.receiver, Kind: version.Name, Formed: true})
	receipt, id, err := b.write(version, receipts.Predicate{
		Kind: receipts.KindResolution, From: answerer, To: q.receiver, At: at,
		Envelope: q.envelope, Verdict: string(d.Verdict), Reason: d.Reason,
	})
	if err != nil || d.Verdict != composition.Pass {
		return err
	}
	return b.queue(q.receiver, part, receipt, id)
}

// Undeliverable closes the question questionID as one nobody can be reached to answer, at the time
// at (maybe-4): a resolution receipt from its first answerer, or from the mediator's own reference
// when it has none, to the asker, block, undeliverable. It reads only row 2 of section 6: an id that
// names no question open before its deadline is rejected with no-open-question (maybe-7).
func (m *Mediator) Undeliverable(questionID, at string) (Outcome, error) {
	if !m.founded() {
		return Outcome{}, failure(CodeUnfounded)
	}
	if !shape.Timestamp(at) {
		return Outcome{}, failure(CodeAt)
	}
	q := m.open(questionID, at)
	if q == nil {
		return Outcome{}, &Rejection{Code: NoOpenQuestion}
	}
	b := m.batch()
	if err := b.close(q, m.firstAnswerer(q.roles), receipts.ReasonUndeliverable, at); err != nil {
		return Outcome{}, err
	}
	m.commit(b)
	return b.outs[0], nil
}

// Tick passes time to at (maybe-5): every open question whose deadline is at or before at closes, in
// the order the questions were opened, with a resolution receipt from its first answerer, or the
// mediator when it has none, to the asker, block, timeout, whose time is the deadline. A tick that
// reaches no deadline writes nothing and returns an empty list. It refuses nothing.
func (m *Mediator) Tick(at string) ([]Outcome, error) {
	if !m.founded() {
		return nil, failure(CodeUnfounded)
	}
	if !shape.Timestamp(at) {
		return nil, failure(CodeAt)
	}
	b := m.batch()
	for _, q := range m.questions {
		if q.deadline <= at {
			if err := b.close(q, m.firstAnswerer(q.roles), receipts.ReasonTimeout, q.deadline); err != nil {
				return nil, err
			}
		}
	}
	m.commit(b)
	return b.outs, nil
}
