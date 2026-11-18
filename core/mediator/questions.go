package mediator

import (
	"encoding/hex"
	"fmt"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/crossing"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
)

// Action is what an answer does to a question.
//
// Deprecated: use answer.Action, its one definition (constants-once,
// docs/adr/0021-who-decided-and-the-answer-call.md). It is removed at the release.
type Action = answer.Action

// The four actions of an answer.
//
// Deprecated: use answer.Accepted, answer.Changed, answer.Declined and answer.Cancelled. They are
// removed at the release.
const (
	Accepted  = answer.Accepted
	Changed   = answer.Changed
	Declined  = answer.Declined
	Cancelled = answer.Cancelled
)

// A Resolution is an answer as its caller gives it (section 6, maybe-11): Call, the signed answer
// call's canonical bytes, which say the question, the action and whether it asks for release; and,
// for an accept or a change, Text, the answer's text or the version, and Salt, the salt the
// answerer minted for it, under which the call's digest was taken. A decline and a cancel have
// neither. A Resolution holds an answer's text and salt, so it prints as a fixed text under every
// format verb (secrets-never-print, record 0006).
type Resolution struct {
	Call []byte
	Text string
	Salt []byte
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
	statements []receipts.Verified
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
	r, verified, err := receipts.Build(subject, p, b.m.key)
	if err != nil {
		return nil, "", failure(CodeInternal)
	}
	id := verified.Statement().ID
	b.chains[pair] = link{seq: p.Seq, id: id}
	b.receipts = append(b.receipts, r)
	b.statements = append(b.statements, verified)
	b.outs = append(b.outs, Outcome{Verdict: p.Verdict, Reason: p.Reason, ReceiptID: id, Receipt: append([]byte(nil), r...)})
	return r, id, nil
}

// queue queues for to the part, as its delivery carries it with the receipt that passed it.
func (b *batch) queue(to string, part map[string]any, receipt []byte, id string) error {
	d := crossing.Deliver(part, receipt, id)
	if d == nil {
		return failure(CodeInternal)
	}
	b.queued = append(b.queued, queuedFor{to: to, q: queued{id: id, delivery: d}})
	return nil
}

// close writes the resolution receipt that closes q with a reason of its own (maybe-4, maybe-5):
// from from to the asker, block, naming the asked part, echoing the question's envelope and naming
// by as its decider (receipt-11).
func (b *batch) close(q *question, from, reason, at string, by receipts.By) error {
	b.closed[q.id] = true
	_, _, err := b.write(q.subject, receipts.Predicate{
		Kind: receipts.KindResolution, From: from, To: q.asker, At: at,
		Envelope: q.envelope, Verdict: string(composition.Block), Reason: reason, By: by,
	})
	return err
}

// The by of the mediator's own closings (receipt-11): a deadline, a question nobody can answer, and
// a close.
var (
	byDeadline      = receipts.By{Decider: receipts.ByMediator, Event: receipts.EventDeadline}
	byUndeliverable = receipts.By{Decider: receipts.ByMediator, Event: receipts.EventUndeliverable}
	byClose         = receipts.By{Decider: receipts.ByMediator, Event: receipts.EventClose}
)

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

// Resolve answers a question as caller at the time at (section 6). Its rows run in order: the
// caller is a member (not-a-member); the answer call reads, names this Accord and is the caller's
// (answer-call, maybe-11); the question it names is open at a time before its deadline
// (no-open-question, maybe-7); the caller may take its action, an answerer for an accept, a change or
// a decline and the question's asker for a cancel of a question with an answerer (not-an-answerer,
// maybe-6); and the answer's text and salt fit the call (answer-body). Once the Accord is closed, a
// member's answer is rejected with closed, right after membership (close-3), and while the close has
// no room for what it may add, with accord-full right after closed (close-2). A rejected answer
// writes nothing and leaves the question as it was.
//
// An accept forms the answer and a change the version (maybe-8, maybe-9), under the answerer's salt,
// so its digest is the call's: a part decided by its kind through composition from the answerer to
// the asker as a part the mediator formed; its resolution receipt holds composition's verdict, and a
// pass queues the part for the asker. A decline and a cancel write a block from the first answerer
// (maybe-4). An accept or a change that passes and whose call asks for release then writes the
// release receipt (maybe-10). Every receipt names the call by its typed hash (receipt-11), and every
// delivery it queues carries the call (maybe-12). Every receipt is built before any state changes,
// the resolution first, and both are returned in that order.
func (m *Mediator) Resolve(caller string, r Resolution, at string) ([]Outcome, error) {
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
	// Right after membership: a closed Accord takes no answer (close-3).
	if m.closed != nil {
		return nil, &Rejection{Code: AccordClosed}
	}
	// Right after closed: room for the close (close-2).
	if m.full() {
		return nil, &Rejection{Code: AccordFull}
	}
	// Row 2: the answer call, the caller's, under this Accord (maybe-11).
	c, err := answer.Read(r.Call)
	if err != nil || c.Accord != m.accord.ID() || c.From != caller {
		return nil, &Rejection{Code: AnswerCall}
	}
	// Row 3: an open question, at a time before its deadline (maybe-7).
	q := m.open(c.Question, at)
	if q == nil {
		return nil, &Rejection{Code: NoOpenQuestion}
	}
	// Row 4: an answerer answers, and the asker cancels a question someone could answer (maybe-6).
	answerers := m.answerers(q.roles)
	if c.Action == answer.Cancelled {
		if caller != q.asker || len(answerers) == 0 {
			return nil, &Rejection{Code: NotAnAnswerer}
		}
	} else if !contains(answerers, caller) {
		return nil, &Rejection{Code: NotAnAnswerer}
	}
	// Row 5: the answer's text and salt (maybe-8, maybe-9).
	switch c.Action {
	case answer.Accepted, answer.Changed:
		if len(r.Salt) != SaltBytes || (c.Action == answer.Changed && r.Text == "") ||
			receipts.Digest(r.Salt, []byte(r.Text)) != c.Digest {
			return nil, &Rejection{Code: AnswerBody}
		}
	default:
		if r.Text != "" || len(r.Salt) != 0 {
			return nil, &Rejection{Code: AnswerBody}
		}
	}
	// The call is canonical bytes that read, so it decodes, and its typed hash names it.
	call, _, _ := shape.Decode(r.Call)
	by := receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(r.Call)}

	b := m.batch()
	if c.Action == answer.Declined || c.Action == answer.Cancelled {
		reason := receipts.ReasonDeclined
		if c.Action == answer.Cancelled {
			reason = receipts.ReasonCancelled
		}
		if err := b.close(q, m.firstAnswerer(q.roles), reason, at, by); err != nil {
			return nil, err
		}
		m.commit(b)
		return b.outs, nil
	}

	// The answer, or the version: one part the mediator forms (maybe-8, maybe-9), with the call
	// inside its delivery (maybe-12).
	kind, prefix := receipts.NameAnswer, "answer:"
	if c.Action == answer.Changed {
		kind, prefix = receipts.NameChanged, "change:"
	}
	subject := receipts.Subject{Name: kind, Digest: c.Digest}
	part := map[string]any{"messageId": prefix + q.id, "kind": kind, "salt": hex.EncodeToString(r.Salt), "text": r.Text, "re": q.id,
		"answer": call}
	d := composition.Decide(m.accord, composition.Crossing{From: caller, To: q.asker, Kind: kind, Formed: true})
	b.closed[q.id] = true
	receipt, id, err := b.write(subject, receipts.Predicate{
		Kind: receipts.KindResolution, From: caller, To: q.asker, At: at,
		Envelope: q.envelope, Verdict: string(d.Verdict), Reason: d.Reason, By: by,
	})
	if err != nil {
		return nil, err
	}
	if d.Verdict == composition.Pass {
		if err := b.queue(q.asker, part, receipt, id); err != nil {
			return nil, err
		}
		if c.Release {
			if err := m.release(b, q, c.Action, caller, subject, part, call, by, at); err != nil {
				return nil, err
			}
		}
	}
	m.commit(b)
	return b.outs, nil
}

// release writes the release receipt of an answer that passed and asked for release, into b
// (maybe-10), naming the answer's call as its decider, as its closing does (receipt-11). After an
// accept, the held part is decided again, as given and with no judgment, from the asker to the
// receiver: a block is written as composition returns it, and any other verdict is the answer's
// pass, which queues the held part for the receiver as it was given, with the call inside its
// delivery (maybe-12). After a change, the same version is decided from the answerer to the receiver
// as a part the mediator formed, and a pass queues it for the receiver; when the receiver is the
// answerer or the asker, a change releases nothing.
func (m *Mediator) release(b *batch, q *question, action answer.Action, answerer string, version receipts.Subject, part map[string]any, call any, by receipts.By, at string) error {
	if action == answer.Accepted {
		d := composition.Decide(m.accord, composition.Crossing{From: q.asker, To: q.receiver, Kind: q.subject.Name})
		verdict, reason := composition.Pass, composition.ReasonOK
		if d.Verdict == composition.Block {
			verdict, reason = d.Verdict, d.Reason
		}
		receipt, id, err := b.write(q.subject, receipts.Predicate{
			Kind: receipts.KindCrossing, From: q.asker, To: q.receiver, At: at,
			Envelope: q.envelope, Verdict: string(verdict), Reason: reason, By: by,
		})
		if err != nil || verdict != composition.Pass {
			return err
		}
		held := map[string]any{"answer": call}
		for k, v := range q.delivery {
			held[k] = v
		}
		return b.queue(q.receiver, held, receipt, id)
	}
	if q.receiver == answerer || q.receiver == q.asker {
		return nil
	}
	d := composition.Decide(m.accord, composition.Crossing{From: answerer, To: q.receiver, Kind: version.Name, Formed: true})
	receipt, id, err := b.write(version, receipts.Predicate{
		Kind: receipts.KindResolution, From: answerer, To: q.receiver, At: at,
		Envelope: q.envelope, Verdict: string(d.Verdict), Reason: d.Reason, By: by,
	})
	if err != nil || d.Verdict != composition.Pass {
		return err
	}
	return b.queue(q.receiver, part, receipt, id)
}

// Undeliverable closes the question questionID as one nobody can be reached to answer, at the time
// at (maybe-4): a resolution receipt from its first answerer, or from the mediator's own reference
// when it has none, to the asker, block, undeliverable. It reads only row 3 of section 6: an id that
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
	if err := b.close(q, m.firstAnswerer(q.roles), receipts.ReasonUndeliverable, at, byUndeliverable); err != nil {
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
			if err := b.close(q, m.firstAnswerer(q.roles), receipts.ReasonTimeout, q.deadline, byDeadline); err != nil {
				return nil, err
			}
		}
	}
	m.commit(b)
	return b.outs, nil
}
