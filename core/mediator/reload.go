package mediator

import (
	"errors"
	"sort"

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
// (inputs-as-parameters, record 0019). It checks, in order, and returns no mediator and the first fault: at, by the
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
func Reload(founding []byte, list [][]byte, open []Open, deliveries []Queued, foundedAt string, key signing.Key, at string) (*Mediator, []Outcome, error) {
	if !shape.Timestamp(at) {
		return nil, nil, failure(CodeAt)
	}
	m, err := Found(founding, key, foundedAt)
	if err != nil {
		return nil, nil, err
	}
	verified, err := receipts.Verify(key.Reference(), list)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return nil, nil, &Error{Code: CodeJournal, Index: re.Index, Err: re}
		}
		return nil, nil, failure(CodeJournal)
	}
	st := make([]receipts.Statement, len(verified))
	for i, v := range verified {
		st[i] = v.Statement()
	}
	for i, s := range st {
		if s.Predicate.Accord != m.accord.ID() || s.Predicate.Card != m.accord.Card() {
			return nil, nil, &Error{Code: CodeJournal, Index: i}
		}
	}
	places, err := receipts.Releases(verified)
	if err != nil {
		return nil, nil, failure(CodeJournal)
	}

	// The questions the receipts hold open, in the order they were opened, and the entry of each.
	closed := map[int]bool{}
	for _, p := range places {
		if p.Role == receipts.RoleClosing {
			closed[p.Question] = true
		}
	}
	openAt := map[string]int{}
	var order []int
	for i, p := range places {
		if p.Role == receipts.RoleQuestion && !closed[i] {
			openAt[st[i].ID] = i
			order = append(order, i)
		}
	}
	given := map[int]Open{}
	for i, o := range open {
		if !shape.Timestamp(o.Deadline) {
			return nil, nil, &Error{Code: CodeAt, Index: i}
		}
		qi, ok := openAt[o.Question]
		if _, twice := given[qi]; !ok || twice {
			return nil, nil, &Error{Code: CodeQuestion, Index: i}
		}
		given[qi] = Open{Question: o.Question, Deadline: o.Deadline, Roles: append([]string(nil), o.Roles...)}
	}
	for _, qi := range order {
		if _, ok := given[qi]; !ok {
			return nil, nil, failure(CodeQuestion)
		}
	}

	// The deliveries not yet fetched, each the one its receipt queued.
	byID := map[string]int{}
	for i, s := range st {
		byID[s.ID] = i
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
			return nil, nil, &Error{Code: CodeHeld, Index: i}
		}
		seen[ri] = true
		back = append(back, restored{index: ri, to: q.To, q: queued{id: st[ri].ID, delivery: append([]byte(nil), q.Delivery...)}})
	}
	sort.Slice(back, func(i, j int) bool { return back[i].index < back[j].index })

	// The state the receipts leave.
	for i, r := range list {
		m.receipts = append(m.receipts, append([]byte(nil), r...))
		s := st[i]
		m.statements = append(m.statements, verified[i])
		m.chains[[2]string{s.Predicate.From, s.Predicate.To}] = link{seq: s.Predicate.Seq, id: s.ID}
		if e := s.Predicate.Envelope; e.Seq > m.highest[e.Sender] {
			m.highest[e.Sender] = e.Seq
		}
	}
	for _, x := range back {
		m.queues[x.to] = append(m.queues[x.to], x.q)
	}

	// Every open question closes, in the order opened; its receipts are built before the state takes
	// them.
	b := m.batch()
	for _, qi := range order {
		o, ask := given[qi], st[qi]
		q := &question{id: ask.ID, asker: ask.Predicate.From, receiver: ask.Predicate.To,
			envelope: ask.Predicate.Envelope, subject: ask.Subject, roles: o.Roles, deadline: o.Deadline}
		reason, when, by := receipts.ReasonUndeliverable, at, byUndeliverable
		if o.Deadline <= at {
			reason, when, by = receipts.ReasonTimeout, o.Deadline, byDeadline
		}
		if err := b.close(q, m.firstAnswerer(q.roles), reason, when, by); err != nil {
			return nil, nil, err
		}
	}
	m.commit(b)
	return m, b.outs, nil
}
