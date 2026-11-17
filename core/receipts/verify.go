package receipts

import (
	"sort"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
)

var (
	pass     = string(composition.Pass)
	block    = string(composition.Block)
	escalate = string(composition.Escalate)
)

// Verify checks a list of receipts, in the order the mediator wrote them, with the mediator's
// reference alone, by the rows of section 8: the reference once, then each receipt in list order,
// every row of one before the next (receipt-6). It stops at the first failure and returns its code
// with the receipt's index. A list that verifies yields its statements as Verified, the type only
// verification makes (docs/adr/0016-verified-receipts.md); an empty list verifies.
func Verify(mediator string, list [][]byte) ([]Verified, error) {
	// Row 0.
	if !isReference(mediator) {
		return nil, &Error{Code: CodeReference, Index: -1}
	}
	statements, fail := chain(mediator, list)
	// Rows 1 to 6 hold up to fail; row 7 reads only the receipts up to each one, so a release failure
	// before fail is the first failure of the list.
	if _, err := releases(statements); err != nil {
		return nil, err
	}
	if fail != nil {
		return nil, fail
	}
	out := make([]Verified, len(statements))
	for i, s := range statements {
		out[i] = Verified{s: s}
	}
	return out, nil
}

// chain runs rows 1 to 6 over the list and returns the statements of the receipts before the first
// that fails, and that failure.
func chain(mediator string, list [][]byte) ([]Statement, *Error) {
	type link struct {
		seq int64
		id  string
	}
	last := map[[2]string]link{}
	var out []Statement
	for i, r := range list {
		st, code := one(r, mediator)
		if code != "" {
			return out, &Error{Code: code, Index: i}
		}
		// Row 6.
		pair := [2]string{st.Predicate.From, st.Predicate.To}
		prev, seen := last[pair]
		seq, ok := st.placed()
		switch {
		case !ok:
			return out, &Error{Code: CodeChain, Index: i}
		case !seen && (seq != 1 || st.hasPrev):
			return out, &Error{Code: CodeChain, Index: i}
		case seen && (seq != prev.seq+1 || st.prev != any(prev.id)):
			return out, &Error{Code: CodeChain, Index: i}
		}
		last[pair] = link{seq: seq, id: st.ID}
		out = append(out, st.Statement)
	}
	return out, nil
}

// one runs rows 1 to 5 of section 8 over one receipt.
func one(receipt []byte, mediator string) (reading, Code) {
	payload, code := open(receipt, mediator)
	if code != "" {
		return reading{}, code
	}
	st, ok := readStatement(payload)
	if !ok {
		return reading{}, CodeStatementType
	}
	if st.mediator != any(mediator) {
		return reading{}, CodeMediator
	}
	return st, ""
}

// Role is what a receipt is in the questions of its list (receipt-6).
type Role int

// The roles a receipt can have.
const (
	// RoleGive is a receipt that belongs to no question: a give's crossing receipt that passed or was
	// blocked.
	RoleGive Role = iota
	// RoleQuestion is a crossing receipt with verdict escalate: it opens a question of its own.
	RoleQuestion
	// RoleClosing is a question's closing receipt, the first that belongs to it.
	RoleClosing
	// RoleRelease is a question's release receipt, right after an answer that passed.
	RoleRelease
)

// A Place is a receipt's place among the questions of its list: its role and, for every role but
// RoleGive, the index of its question's receipt.
type Place struct {
	Role     Role
	Question int
}

// Releases reads the questions of a list of verified statements by receipt-6's rule, the one rule
// every reader of receipts applies, and returns each receipt's place. A receipt that belongs to a
// question but has no place in it, and a resolution receipt that belongs to no question, fail release
// at their index (section 8, row 7). It takes Verified alone, so no caller reads the questions of
// receipts nobody verified (docs/adr/0016-verified-receipts.md).
func Releases(list []Verified) ([]Place, error) {
	statements := make([]Statement, len(list))
	for i, v := range list {
		statements[i] = v.s
	}
	return releases(statements)
}

// releases is Releases over statements, which Verify runs as its row 7 before it has any Verified.
func releases(statements []Statement) ([]Place, error) {
	type question struct {
		closing  int
		answer   string // NameAnswer or NameChanged when the closing is an answer that passed
		by       string // the answer's from
		subject  Subject
		released bool
	}
	latest := map[Envelope]int{}
	questions := map[int]*question{}
	places := make([]Place, len(statements))
	fail := func(i int) ([]Place, error) { return nil, &Error{Code: CodeRelease, Index: i} }
	for i, s := range statements {
		p := s.Predicate
		if p.Kind == KindCrossing && p.Verdict == escalate {
			latest[p.Envelope] = i
			questions[i] = &question{closing: -1}
			places[i] = Place{Role: RoleQuestion, Question: i}
			continue
		}
		qi, belongs := latest[p.Envelope]
		if !belongs {
			if p.Kind == KindResolution {
				return fail(i)
			}
			places[i] = Place{Role: RoleGive, Question: -1}
			continue
		}
		q, ask := questions[qi], statements[qi]
		if q.closing < 0 {
			if p.Kind != KindResolution || p.To != ask.Predicate.From {
				return fail(i)
			}
			switch p.Reason {
			case ReasonDeclined, ReasonCancelled, ReasonTimeout, ReasonUndeliverable:
				if s.Subject != ask.Subject {
					return fail(i)
				}
			default:
				if s.Subject.Name != NameAnswer && s.Subject.Name != NameChanged {
					return fail(i)
				}
				if p.Verdict == pass {
					q.answer, q.by, q.subject = s.Subject.Name, p.From, s.Subject
				}
			}
			q.closing = i
			places[i] = Place{Role: RoleClosing, Question: qi}
			continue
		}
		if q.answer == "" || q.released || i != q.closing+1 {
			return fail(i)
		}
		a := ask.Predicate
		switch q.answer {
		case NameAnswer:
			if p.Kind != KindCrossing || p.From != a.From || p.To != a.To || s.Subject != ask.Subject ||
				(p.Verdict != pass && p.Verdict != block) {
				return fail(i)
			}
		case NameChanged:
			if p.Kind != KindResolution || p.From != q.by || p.To != a.To || s.Subject != q.subject ||
				a.To == a.From || a.To == q.by {
				return fail(i)
			}
		}
		q.released = true
		places[i] = Place{Role: RoleRelease, Question: qi}
	}
	return places, nil
}

// A Kept record is what one party kept: the envelopes it signed, by seq and hash, and the receipts
// of the deliveries it fetched, in the order kept (section 9).
type Kept struct {
	Actor     string
	Envelopes []Envelope
	Delivered [][]byte
}

// An Item names a receipt, by its id, in a party's list of a report.
type Item struct {
	Actor     string
	ReceiptID string
}

// A Missed names a kept envelope, by its seq, that no presented receipt echoes for its party.
type Missed struct {
	Actor string
	Seq   int64
}

// A Report is reconciliation's three lists (receipt-7), parties in actor-reference order.
type Report struct {
	Removed    []Item
	Unrecorded []Item
	Missing    []Missed
}

// Reconcile compares the presented receipts with what each party kept, by the rows of section 9, and
// reports what is in one record and not the other (receipt-7). It verifies first and reports only
// when every row passes. Before any row, it checks that each kept record names its party by an actor
// reference: one that does not fails kept at the record's index in kept, and its error names no
// party, so text a caller passed never reaches the error.
func Reconcile(mediator, accord, card string, presented [][]byte, kept []Kept) (Report, error) {
	for i, k := range kept {
		if !isReference(k.Actor) {
			return Report{}, &Error{Code: CodeKept, Index: i}
		}
	}
	// Row 1.
	verified, err := Verify(mediator, presented)
	if err != nil {
		return Report{}, err
	}
	statements := make([]Statement, len(verified))
	for i, v := range verified {
		statements[i] = v.s
	}
	places, _ := releases(statements)
	// Row 2.
	for i, s := range statements {
		if s.Predicate.Accord != accord || s.Predicate.Card != card {
			return Report{}, &Error{Code: CodePresented, Index: i}
		}
	}
	parties := append([]Kept(nil), kept...)
	sort.SliceStable(parties, func(i, j int) bool { return canonical.Less(parties[i].Actor, parties[j].Actor) })
	// Row 3.
	delivered := make([][]string, len(parties))
	for pi, k := range parties {
		for i, r := range k.Delivered {
			st, code := one(r, mediator)
			if code != "" || st.Predicate.Accord != accord || st.Predicate.Card != card ||
				st.Predicate.Verdict != pass || st.Predicate.To != k.Actor {
				return Report{}, &Error{Code: CodeKept, Index: i, Actor: k.Actor}
			}
			delivered[pi] = append(delivered[pi], st.ID)
		}
	}

	ids := map[string]bool{}
	type echo struct {
		actor string
		seq   int64
		hash  string
	}
	echoed := map[echo]bool{}
	for i, s := range statements {
		ids[s.ID] = true
		// A receipt counts as an echo for the envelope's signer only: a give's sender, or the asker
		// of the question it belongs to (crossing-5).
		actor := s.Predicate.From
		if places[i].Role != RoleGive {
			actor = statements[places[i].Question].Predicate.From
		}
		echoed[echo{actor, s.Predicate.Envelope.Seq, s.Predicate.Envelope.Hash}] = true
	}
	var rep Report
	for pi, k := range parties {
		for _, id := range delivered[pi] {
			if !ids[id] {
				rep.Removed = append(rep.Removed, Item{Actor: k.Actor, ReceiptID: id})
			}
		}
	}
	for pi, k := range parties {
		have := map[string]bool{}
		for _, id := range delivered[pi] {
			have[id] = true
		}
		for _, s := range statements {
			if s.Predicate.Verdict == pass && s.Predicate.To == k.Actor && !have[s.ID] {
				rep.Unrecorded = append(rep.Unrecorded, Item{Actor: k.Actor, ReceiptID: s.ID})
			}
		}
	}
	for _, k := range parties {
		var seqs []int64
		listed := map[int64]bool{}
		for _, e := range k.Envelopes {
			if !echoed[echo{k.Actor, e.Seq, e.Hash}] && !listed[e.Seq] {
				listed[e.Seq] = true
				seqs = append(seqs, e.Seq)
			}
		}
		sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
		for _, s := range seqs {
			rep.Missing = append(rep.Missing, Missed{Actor: k.Actor, Seq: s})
		}
	}
	return rep, nil
}
