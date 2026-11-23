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
// verification makes (record 0002); an empty list verifies.
func Verify(mediator string, list [][]byte) ([]Verified, error) {
	// Row 0.
	if !isReference(mediator) {
		return nil, &Error{Code: CodeReference, Index: -1}
	}
	entries, _, fail := chain(mediator, list, false)
	// Rows 1 to 6 hold up to fail; rows 7 to 9 read only the receipts up to each one, so a failure of
	// theirs before fail is the first failure of the list.
	_, err := releases(entries, false)
	if err := first(err, closeRows(entries, false), fail); err != nil {
		return nil, err
	}
	return verifiedOf(entries), nil
}

// first returns the failure of rows 7 to 9 at the lowest index, row 7's at an index both name, or,
// when neither failed, the failure of rows 1 to 6, which comes after both.
func first(release error, close, rest *Error) error {
	var r *Error
	if release != nil {
		r, _ = release.(*Error)
	}
	switch {
	case r != nil && (close == nil || r.Index <= close.Index):
		return r
	case close != nil:
		return close
	case rest != nil:
		return rest
	}
	return nil
}

// verifiedOf makes each entry the type only verification makes (record 0002).
func verifiedOf(entries []entry) []Verified {
	out := make([]Verified, len(entries))
	for i, e := range entries {
		out[i] = Verified(e)
	}
	return out
}

// A Start is a chain of a subset that starts after its first receipt: the pair's from and to, and
// the seq of the first receipt of the pair the subset holds, above 1 (receipt-6).
type Start struct {
	From, To string
	Seq      int64
}

// A Subset is what VerifySubset yields: the verified statements, each receipt's place among the
// questions of the subset, and the chains that start late, in the order their first receipts come
// (receipt-6, verify-1).
type Subset struct {
	Verified []Verified
	Places   []Place
	Starts   []Start
}

// VerifySubset checks a subset of a mediator's receipts, in the order the mediator wrote them, as
// Verify does, with two differences (receipt-6, verify-1): the first receipt of a pair may have any
// seq, with its prev of the form receipt-3 gives it, and a chain that starts above 1 is listed in
// Starts; and a receipt that belongs to no question of the subset, where Verify would fail release,
// has the place RoleOutside, since its question may be among the receipts left out.
func VerifySubset(mediator string, list [][]byte) (Subset, error) {
	if !isReference(mediator) {
		return Subset{}, &Error{Code: CodeReference, Index: -1}
	}
	entries, starts, fail := chain(mediator, list, true)
	places, err := releases(entries, true)
	if err := first(err, closeRows(entries, true), fail); err != nil {
		return Subset{}, err
	}
	return Subset{Verified: verifiedOf(entries), Places: places, Starts: starts}, nil
}

// An entry is one receipt of a list that passed rows 1 to 6: a receipt's statement, or a close
// (close-2), which is in no chain.
type entry struct {
	s Statement
	c *Close
}

// chain runs rows 1 to 6 over the list and returns the entries of the receipts before the first
// that fails, and that failure. For a subset, the first receipt of a pair may have any seq, and the
// chains that start above 1 are returned. A close has no seq or prev, and row 6 does not read it.
func chain(mediator string, list [][]byte, subset bool) ([]entry, []Start, *Error) {
	type link struct {
		seq int64
		id  string
	}
	last := map[[2]string]link{}
	var out []entry
	var starts []Start
	for i, r := range list {
		st, cl, code := one(r, mediator)
		if code != "" {
			return out, starts, &Error{Code: code, Index: i}
		}
		if cl != nil {
			out = append(out, entry{c: cl})
			continue
		}
		// Row 6.
		pair := [2]string{st.Predicate.From, st.Predicate.To}
		prev, seen := last[pair]
		seq, ok := st.placed()
		switch {
		case !ok:
			return out, starts, &Error{Code: CodeChain, Index: i}
		case !seen && subset:
			if seq != 1 {
				starts = append(starts, Start{From: pair[0], To: pair[1], Seq: seq})
			}
		case !seen && (seq != 1 || st.hasPrev):
			return out, starts, &Error{Code: CodeChain, Index: i}
		case seen && (seq != prev.seq+1 || st.prev != any(prev.id)):
			return out, starts, &Error{Code: CodeChain, Index: i}
		}
		last[pair] = link{seq: seq, id: st.ID}
		out = append(out, entry{s: st.Statement})
	}
	return out, starts, nil
}

// one runs rows 1 to 5 of section 8 over one receipt: a receipt's statement, or a close's, read
// against close-2 by the predicate type its payload names.
func one(receipt []byte, mediator string) (reading, *Close, Code) {
	payload, code := open(receipt, mediator)
	if code != "" {
		return reading{}, nil, code
	}
	if isClose(payload) {
		c, ok := readClose(payload)
		if !ok {
			return reading{}, nil, CodeStatementType
		}
		if c.mediator != any(mediator) {
			return reading{}, nil, CodeMediator
		}
		return reading{}, &c.Close, ""
	}
	st, ok := readStatement(payload)
	if !ok {
		return reading{}, nil, CodeStatementType
	}
	if st.mediator != any(mediator) {
		return reading{}, nil, CodeMediator
	}
	return st, nil, ""
}

// closeRows runs rows 8 and 9 of section 8 over the entries (close-4) and returns the first failure.
// Row 9: no receipt follows a close. Row 8, over a whole list: a closed closing is followed only by
// another, of the same at, or by a close; a close's at is that of the closed closings right before
// it; its chains are exactly the last receipt of every chain before it, ordered by pair; and each of
// its unfetched names a distinct receipt before it that queued a delivery to its to, one that
// passed. Over a subset, row 8 reads only that each chain the subset holds ends at the seq and
// receipt the close lists for it.
func closeRows(entries []entry, subset bool) *Error {
	last := map[[2]string]Final{}
	queued := map[string]string{}
	run, runAt, closed := false, "", false
	fail := func(i int) *Error { return &Error{Code: CodeClose, Index: i} }
	for i, e := range entries {
		if closed {
			return &Error{Code: CodeAfterClose, Index: i}
		}
		if c := e.c; c != nil {
			closed = true
			listed := map[[2]string]Final{}
			for _, f := range c.Chains {
				listed[[2]string{f.From, f.To}] = f
			}
			if subset {
				for pair, f := range last {
					if listed[pair] != f {
						return fail(i)
					}
				}
				continue
			}
			if run && runAt != c.At || !sameFinals(c.Chains, finals(last)) {
				return fail(i)
			}
			seen := map[string]bool{}
			for _, u := range c.Unfetched {
				if seen[u.Receipt] || queued[u.Receipt] != u.To {
					return fail(i)
				}
				seen[u.Receipt] = true
			}
			continue
		}
		p := e.s.Predicate
		isClosed := p.Kind == KindResolution && p.Reason == ReasonClosed
		if !subset && run && (!isClosed || p.At != runAt) {
			return fail(i)
		}
		if isClosed && !run {
			run, runAt = true, p.At
		}
		last[[2]string{p.From, p.To}] = Final{From: p.From, To: p.To, Seq: p.Seq, Receipt: e.s.ID}
		if p.Verdict == pass {
			queued[e.s.ID] = p.To
		}
	}
	return nil
}

// finals is the last receipt of each chain, ordered by pair: by from, then by to, in actor-reference
// order (close-2).
func finals(last map[[2]string]Final) []Final {
	out := make([]Final, 0, len(last))
	for _, f := range last {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return canonical.Less(out[i].From, out[j].From)
		}
		return canonical.Less(out[i].To, out[j].To)
	})
	return out
}

func sameFinals(a, b []Final) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	// RoleOutside is a receipt of a subset whose question is not in the subset: a resolution receipt,
	// or a crossing receipt an answer decided, that belongs to no question of the list (VerifySubset).
	RoleOutside
	// RoleClose is a close receipt (close-2): it belongs to no question, and no receipt follows it.
	RoleClose
)

// A Place is a receipt's place among the questions of its list: its role and, for every role but
// RoleGive, RoleOutside and RoleClose, the index of its question's receipt (receipt-6, record 0002).
type Place struct {
	Role     Role
	Question int
}

// Releases reads the questions of a list of verified statements by receipt-6's rule, the one rule
// every reader of receipts applies, and returns each receipt's place. A receipt that belongs to a
// question but has no place in it, and a resolution receipt that belongs to no question, fail release
// at their index (section 8, row 7). It takes Verified alone, so no caller reads the questions of
// receipts nobody verified (record 0002).
func Releases(list []Verified) ([]Place, error) {
	entries := make([]entry, len(list))
	for i, v := range list {
		entries[i] = entry(v)
	}
	return releases(entries, false)
}

// releases is Releases over entries, which Verify runs as its row 7 before it has any Verified.
// For a subset, a receipt that would belong to a question not in the list is RoleOutside, not a
// failure. A close is RoleClose.
func releases(entries []entry, subset bool) ([]Place, error) {
	type question struct {
		closing  int
		answer   string // NameAnswer or NameChanged when the closing is an answer that passed
		by       string // the answer's from
		decider  By     // the closing's by, which its release repeats (receipt-11)
		subject  Subject
		released bool
	}
	latest := map[Envelope]int{}
	questions := map[int]*question{}
	places := make([]Place, len(entries))
	fail := func(i int) ([]Place, error) { return nil, &Error{Code: CodeRelease, Index: i} }
	for i, e := range entries {
		if e.c != nil {
			places[i] = Place{Role: RoleClose, Question: -1}
			continue
		}
		s := e.s
		p := s.Predicate
		if p.Kind == KindCrossing && p.Verdict == escalate {
			latest[p.Envelope] = i
			questions[i] = &question{closing: -1}
			places[i] = Place{Role: RoleQuestion, Question: i}
			continue
		}
		qi, belongs := latest[p.Envelope]
		if !belongs {
			// A resolution receipt, and a crossing receipt an answer decided, belong to a question
			// (receipt-11): with none in the list they fail, or in a subset are outside it.
			if p.Kind == KindResolution || p.By.Decider == ByAnswer {
				if subset {
					places[i] = Place{Role: RoleOutside, Question: -1}
					continue
				}
				return fail(i)
			}
			places[i] = Place{Role: RoleGive, Question: -1}
			continue
		}
		q, ask := questions[qi], entries[qi].s
		if q.closing < 0 {
			if p.Kind != KindResolution || p.To != ask.Predicate.From {
				return fail(i)
			}
			switch p.Reason {
			case ReasonDeclined, ReasonCancelled, ReasonTimeout, ReasonUndeliverable, ReasonClosed:
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
			q.closing, q.decider = i, p.By
			places[i] = Place{Role: RoleClosing, Question: qi}
			continue
		}
		// A release repeats its closing's by: the answer that released it (receipt-11).
		if q.answer == "" || q.released || i != q.closing+1 || p.By != q.decider {
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

// An Item names a receipt, by its id, in a party's list of a report (receipt-7, record 0002).
type Item struct {
	Actor     string
	ReceiptID string
}

// A Missed names a kept envelope, by its seq, that no presented receipt echoes for its party
// (receipt-7, record 0002).
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
// reference, and that no party's records are given twice: a record that does not, or a second
// record for an actor already given, fails kept at the record's index in kept, and its error names
// no party, so text a caller passed never reaches the error.
func Reconcile(mediator, accord, card string, presented [][]byte, kept []Kept) (Report, error) {
	given := map[string]bool{}
	for i, k := range kept {
		if !isReference(k.Actor) || given[k.Actor] {
			return Report{}, &Error{Code: CodeKept, Index: i}
		}
		given[k.Actor] = true
	}
	// Row 1.
	verified, err := Verify(mediator, presented)
	if err != nil {
		return Report{}, err
	}
	// A close is presented with the receipts and checked against the same Accord and card; it echoes
	// no envelope and queued nothing, so the lists read only the others (close-2).
	var statements []Statement
	var places []Place
	all, _ := Releases(verified)
	index := map[int]int{}
	// Row 2.
	for i, v := range verified {
		a, c := v.s.Predicate.Accord, v.s.Predicate.Card
		if v.c != nil {
			a, c = v.c.Accord, v.c.Card
		}
		if a != accord || c != card {
			return Report{}, &Error{Code: CodePresented, Index: i}
		}
		if v.c == nil {
			index[i] = len(statements)
			statements = append(statements, v.s)
			places = append(places, all[i])
		}
	}
	parties := append([]Kept(nil), kept...)
	sort.SliceStable(parties, func(i, j int) bool { return canonical.Less(parties[i].Actor, parties[j].Actor) })
	// Row 3.
	delivered := make([][]string, len(parties))
	for pi, k := range parties {
		for i, r := range k.Delivered {
			st, cl, code := one(r, mediator)
			if code != "" || cl != nil || st.Predicate.Accord != accord || st.Predicate.Card != card ||
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
			actor = statements[index[places[i].Question]].Predicate.From
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
