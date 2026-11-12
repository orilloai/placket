package mediator

import (
	"bytes"
	"encoding/hex"
	"errors"
	"sort"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
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

// Reload makes a mediator again from the record a journal keeps (record 0008): the founding set and the time
// it was founded at, the mediator's key, every receipt in the order written, each open question's
// deadline and roles, and each delivery not yet fetched, in deliveries; and it closes every open question at the
// reload time at. It checks, in order, and returns no mediator and the first fault: at, by the
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
func Reload(founding []byte, foundedAt string, key signing.Key, list [][]byte, open []Open, deliveries []Queued, at string) (*Mediator, []Outcome, error) {
	if !shape.Timestamp(at) {
		return nil, nil, failure(CodeAt)
	}
	m, err := Found(founding, key, foundedAt)
	if err != nil {
		return nil, nil, err
	}
	st, err := receipts.Verify(key.Reference(), list)
	if err != nil {
		var re *receipts.Error
		if errors.As(err, &re) {
			return nil, nil, &Error{Code: CodeJournal, Index: re.Index, Err: re}
		}
		return nil, nil, failure(CodeJournal)
	}
	for i, s := range st {
		if s.Predicate.Accord != m.accord.ID() || s.Predicate.Card != m.accord.Card() {
			return nil, nil, &Error{Code: CodeJournal, Index: i}
		}
	}
	places, err := receipts.Releases(st)
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
		ri, ok := held(st, list, byID, q)
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
		m.statements = append(m.statements, s)
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
		reason, when := receipts.ReasonUndeliverable, at
		if o.Deadline <= at {
			reason, when = receipts.ReasonTimeout, o.Deadline
		}
		if err := b.close(q, m.firstAnswerer(q.roles), reason, when); err != nil {
			return nil, nil, err
		}
	}
	m.commit(b)
	return m, b.outs, nil
}

// held reads a delivery given back to Reload and returns the index of the receipt that queued it: a
// delivery in canonical form with exactly a delivery's members (crossing-3), whose receipt is one of
// the list, a pass addressed to the delivery's party, carried as that receipt's bytes, and whose kind,
// salt and body are the receipt's subject. It reads the receipts it is given; it decides nothing.
func held(st []receipts.Statement, list [][]byte, byID map[string]int, q Queued) (int, bool) {
	v, c, err := shape.Decode(q.Delivery)
	if err != nil || !bytes.Equal(c, q.Delivery) {
		return -1, false
	}
	o, ok := v.(shape.Object)
	if !ok {
		return -1, false
	}
	bodyName, bodies := "", 0
	for _, n := range []string{"text", "raw", "data"} {
		if _, has := o[n]; has {
			bodyName = n
			bodies++
		}
	}
	if bodies != 1 || !shape.Only(o, "receiptId", "receipt", "messageId", "kind", "salt", bodyName, "", "re") {
		return -1, false
	}
	id, _ := o["receiptId"].(string)
	ri, ok := byID[id]
	if !ok {
		return -1, false
	}
	s := st[ri]
	if s.Predicate.Verdict != string(composition.Pass) || s.Predicate.To != q.To {
		return -1, false
	}
	kept, err := canonical.Form(list[ri])
	if err != nil || !bytes.Equal(shape.Encode(o["receipt"]), kept) {
		return -1, false
	}
	messageID, ok := o["messageId"].(string)
	if !ok || messageID == "" || o["kind"] != any(s.Subject.Name) {
		return -1, false
	}
	if re, has := o["re"]; has {
		if r, ok := re.(string); !ok || !shape.Hash(r) {
			return -1, false
		}
	}
	saltHex, ok := o["salt"].(string)
	if !ok || len(saltHex) < 2*MinSaltBytes || !shape.LowerHex(saltHex) {
		return -1, false
	}
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return -1, false
	}
	var body []byte
	switch bodyName {
	case "text":
		t, ok := o["text"].(string)
		if !ok {
			return -1, false
		}
		body = []byte(t)
	case "raw":
		t, _ := o["raw"].(string)
		if body, ok = shape.Base64(t); !ok {
			return -1, false
		}
	case "data":
		if o["data"] == nil {
			return -1, false
		}
		body = shape.Encode(o["data"])
	}
	if receipts.Digest(salt, body) != s.Subject.Digest {
		return -1, false
	}
	return ri, true
}
