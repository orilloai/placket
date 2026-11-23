package mediator

import (
	"errors"
	"sort"

	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/internal/crossing"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// Open is a question that was open when the mediator stopped, as its door recorded it when it was
// asked: the question's id, its deadline, and the role labels that may answer it (record 0008).
type Open struct {
	Question string
	Deadline string
	Roles    []string
}

// Reload makes a mediator again from the record a journal keeps (record 0008): the founding set,
// every receipt in the order written, each open question's deadline and roles, each delivery not
// yet fetched, in deliveries, the time the set was founded at and the mediator's key; and it closes
// every open question at the reload time at. Its data come first, then the times and the key
// (inputs-as-parameters, record 0008). It checks, in order, and returns no mediator and the first fault: at, by the
// timestamp rule (at); the set, as Found founds it (founding's *Error, or ErrOtherMediator); the
// receipts, by receipts.Verify (journal, at the failing receipt, the verifier's error inside); each
// receipt's Accord and card (journal, with its index); each open entry's deadline (at) and question,
// one the receipts hold open and no earlier entry names (question), with the entry's index, then
// that every question the receipts hold open has an entry (question); and each queued delivery
// (held, with its index).
//
// It rebuilds every chain, for each sender the largest envelope seq a receipt echoes for it
// (crossing-4), and the queues, in the order of the receipts that queued them. Then it
// closes the open questions, oldest first, before anything else: block, timeout, at
// the deadline when that is at or before at (maybe-5), and otherwise block, undeliverable, at at
// (maybe-4), from the first member holding one of the entry's roles, or the mediator. It decides
// nothing again: no crossing is put to composition (one-decision-module), and the closings' outcomes
// are returned in order.
//
// A record that ends in its close is a closed Accord (close-3): Reload returns a closed mediator and
// writes nothing. A record that holds closed closings with no close after them was cut inside a close
// (close-1), and only Close completes it: Reload refuses it with close.
func Reload(in founding.Inputs, list [][]byte, open []Open, deliveries []Queued, foundedAt string, key signing.Key, at string) (*Mediator, []Outcome, error) {
	r, err := rebuild(in, list, open, deliveries, foundedAt, key, at, true)
	if err != nil {
		return nil, nil, err
	}
	m := r.m
	// A record cut inside a close is completed by Close alone; one that ends in its close is a closed
	// Accord, and its reload writes nothing (close-1, close-3).
	if r.torn != "" {
		return nil, nil, failure(CodeClose)
	}
	if m.closed != nil {
		return m, []Outcome{}, nil
	}

	// Every open question closes, in the order opened; its receipts are built before the state takes
	// them.
	b := m.batch()
	for _, qi := range r.order {
		q := r.question(qi)
		reason, when, by := receipts.ReasonUndeliverable, at, byUndeliverable
		if q.deadline <= at {
			reason, when, by = receipts.ReasonTimeout, q.deadline, byDeadline
		}
		if err := b.close(q, m.firstAnswerer(q.roles), reason, when, by); err != nil {
			return nil, nil, err
		}
	}
	m.commit(b)
	return m, b.outs, nil
}

// A record is what rebuild made of a journal's record: the mediator with the state its receipts
// leave, its open questions still open; the verified statements; the indexes of the questions the
// receipts leave open, in the order opened, each with its entry; and, when the record ends in closed
// closings with no close after them, their time.
type record struct {
	m     *Mediator
	st    []receipts.Statement
	order []int
	given map[int]Open
	torn  string
}

// question is the open question at index qi of the record, as its entry recorded it.
func (r record) question(qi int) *question {
	o, ask := r.given[qi], r.st[qi]
	return &question{id: ask.ID, asker: ask.Predicate.From, receiver: ask.Predicate.To,
		envelope: ask.Predicate.Envelope, subject: ask.Subject, roles: o.Roles, deadline: o.Deadline}
}

// rebuild is what Reload and Close share (record 0008, record 0010): it checks the record in
// Reload's order and makes the mediator its receipts leave, with nothing written. A close in the
// list makes the mediator closed. With terms set it needs every member's opening, as Found does.
func rebuild(in founding.Inputs, list [][]byte, open []Open, deliveries []Queued, foundedAt string, key signing.Key, at string, terms bool) (record, error) {
	fail := func(err error) (record, error) { return record{}, err }

	if !shape.Timestamp(at) {
		return fail(failure(CodeAt))
	}
	m, err := found(in, key, foundedAt, terms)
	if err != nil {
		return fail(err)
	}
	verified, err := receipts.Verify(key.Reference(), list)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return fail(&Error{Code: CodeJournal, Index: re.Index, Err: re})
		}
		return fail(failure(CodeJournal))
	}
	st := make([]receipts.Statement, len(verified))
	var closed *Closed
	torn := ""
	for i, v := range verified {
		st[i] = v.Statement()
		accord, card := st[i].Predicate.Accord, st[i].Predicate.Card
		if c, ok := v.Close(); ok {
			// The verifier lets nothing follow a close, so it is the last receipt (close-4).
			accord, card = c.Accord, c.Card
			closed = &Closed{ID: c.ID, Receipt: append([]byte(nil), list[i]...)}
		} else if p := st[i].Predicate; p.Kind == receipts.KindResolution && p.Reason == receipts.ReasonClosed {
			// The verifier holds the closed closings together at the end, before their close.
			torn = p.At
		}
		if accord != m.accord.ID() || card != m.accord.Card() {
			return fail(&Error{Code: CodeJournal, Index: i})
		}
	}
	if closed != nil {
		torn = ""
	}
	places, err := receipts.Releases(verified)
	if err != nil {
		return fail(failure(CodeJournal))
	}

	// The questions the receipts hold open, in the order they were opened, and the entry of each.
	answered := map[int]bool{}
	for _, p := range places {
		if p.Role == receipts.RoleClosing {
			answered[p.Question] = true
		}
	}
	openAt := map[string]int{}
	var order []int
	for i, p := range places {
		if p.Role == receipts.RoleQuestion && !answered[i] {
			openAt[st[i].ID] = i
			order = append(order, i)
		}
	}
	given := map[int]Open{}
	for i, o := range open {
		if !shape.Timestamp(o.Deadline) {
			return fail(&Error{Code: CodeAt, Index: i})
		}
		qi, ok := openAt[o.Question]
		if _, twice := given[qi]; !ok || twice {
			return fail(&Error{Code: CodeQuestion, Index: i})
		}
		given[qi] = Open{Question: o.Question, Deadline: o.Deadline, Roles: append([]string(nil), o.Roles...)}
	}
	for _, qi := range order {
		if _, ok := given[qi]; !ok {
			return fail(failure(CodeQuestion))
		}
	}

	// The deliveries not yet fetched, each the one its receipt queued.
	byID := map[string]int{}
	for i, s := range st {
		if s.ID != "" {
			byID[s.ID] = i
		}
	}
	type restored struct {
		index int
		to    string
		q     queued
	}
	var back []restored
	seen := map[int]bool{}
	for i, q := range deliveries {
		ri, ok := crossing.Held(st, list, byID, q.To, q.Delivery)
		if !ok || seen[ri] {
			return fail(&Error{Code: CodeHeld, Index: i})
		}
		seen[ri] = true
		back = append(back, restored{index: ri, to: q.To, q: queued{id: st[ri].ID, delivery: append([]byte(nil), q.Delivery...)}})
	}
	sort.Slice(back, func(i, j int) bool { return back[i].index < back[j].index })

	// The state the receipts leave. A close is in no chain and echoes no envelope.
	for i, r := range list {
		m.receipts = append(m.receipts, append([]byte(nil), r...))
		s := st[i]
		m.statements = append(m.statements, verified[i])
		if s.ID == "" {
			continue
		}
		m.chains[[2]string{s.Predicate.From, s.Predicate.To}] = link{seq: s.Predicate.Seq, id: s.ID}
		if e := s.Predicate.Envelope; e.Seq > m.highest[e.Sender] {
			m.highest[e.Sender] = e.Seq
		}
	}
	for _, x := range back {
		m.queues[x.to] = append(m.queues[x.to], x.q)
	}
	m.closed = closed
	return record{m: m, st: st, order: order, given: given, torn: torn}, nil
}
