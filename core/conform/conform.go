// Package conform checks a mediator's receipts for conformance to their Accord: that every decision
// a receipt states is one the Accord let its decider make. It checks validity first, by the verifier
// of section 8, and then the rows of section 8's second table, receipt by receipt, without ever
// running a judge again or reading a body: a judge's outcome is taken as the receipt claims it, and a
// body is read only by Body, for the one delivery a caller gives.
//
// It implements verify-1 (what a verifier is given: the Accord id, its founding set, the receipts,
// and optionally answer calls, a subset and a delivery), verify-2 (the conformance table: founding,
// Accord, parties, decision, witness and body) and verify-3 (an answer is witnessed by a call that
// checks, and is otherwise unwitnessed, never a failure). Its decision row re-runs composition.Decide,
// the one decision module, only when the card's kernel hash is the one its caller trusts
// (one-decision-module, docs/adr/0021-who-decided-and-the-answer-call.md).
package conform

import (
	"strconv"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/crossing"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

// Evidence is what a verifier is given (verify-1): the Accord id; the mediator's reference, used
// only when no founding set is given, and otherwise empty or the set's; the founding set; the
// receipts in the order the mediator wrote them; answer calls, each a call's bytes; whether the
// receipts are a subset; and Kernel, the kernel hash whose module this verifier trusts to decide
// again.
type Evidence struct {
	Accord, Mediator  string
	Founding          []byte
	Receipts, Answers [][]byte
	Subset            bool
	Kernel            string
}

// Witness is what a receipt an answer decided says of its answer (verify-3): witnessed by a call
// that checks, or unwitnessed, when no call given matches it. A receipt no answer decided has none.
type Witness string

// The two witnesses (verify-3).
const (
	Witnessed   Witness = "witnessed"
	Unwitnessed Witness = "unwitnessed"
)

// A Decision is one receipt's decider, by its index in the list (receipt-11, verify-2): its by, the
// witness of an answer's receipt, and whether its rows were checked, false for a receipt outside the
// subset's questions.
type Decision struct {
	Index   int
	By      receipts.By
	Witness Witness
	Checked bool
}

// A Report is what Check found (verify-2): the mediator the receipts were verified under, the
// verified subset, each receipt's decision, whether the card's module is the trusted one, and with a
// founding set its operator, how many members that operator is principal of, the number of members,
// and the number of answer calls given that match no receipt.
type Report struct {
	Mediator                     string
	Subset                       receipts.Subset
	Decisions                    []Decision
	Module                       bool
	Operator                     string
	OperatorPrincipalOf, Members int
	Unmatched                    int
}

// Code is a conformance code of section 8's second table (verify-2).
type Code string

// The conformance codes, in the order of the table.
const (
	CodeFounding Code = "founding"
	CodeAccord   Code = "accord"
	CodeParty    Code = "party"
	CodeDecision Code = "decision"
	CodeWitness  Code = "witness"
	CodeBody     Code = "body"
)

// Error is a receipt that does not conform: the code of the first row that failed, and the
// receipt's index, -1 when no receipt is at fault (verify-2). It holds nothing of the evidence.
type Error struct {
	Code  Code
	Index int
}

// Error returns the package, the code and the index, and nothing of the evidence.
func (e *Error) Error() string {
	if e.Index < 0 {
		return "conform: " + string(e.Code)
	}
	return "conform: " + string(e.Code) + " at index " + strconv.Itoa(e.Index)
}

// earliest is the founding time of a set whose list has no receipt to found it at.
const earliest = "0001-01-01T00:00:00Z"

// A question is what the decision and witness rows read of a question of the list.
type question struct {
	index     int
	ask       receipts.Statement
	rule      bool     // whether the rule is known: the module decided the ask again
	answerers []string // in actor-reference order
	deadline  string
	closing   int
	release   int
}

// Check checks the evidence (verify-1, verify-2). Without a founding set it checks validity alone,
// under the mediator's reference it is given: receipts.Verify, or receipts.VerifySubset for a subset,
// whose *receipts.Error it returns. With one, row 0 comes first: the set founds at the first
// receipt's time, its typed hash is the Accord id, and the mediator, when given, is its
// mediator.actor; a failure is founding. Then validity under the set's mediator, and then rows 1 to
// 4, per receipt in list order, every row of one receipt before the next; the first failure is a
// *Error with its code and the receipt's index. Row 3 runs only when the card's kernel hash is
// Kernel; rows 1, 2 and 4 run whatever the module. Row 5 is Body's.
func Check(e Evidence) (Report, error) {
	if len(e.Founding) == 0 {
		sub, err := verify(e.Mediator, e.Receipts, e.Subset)
		if err != nil {
			return Report{}, err
		}
		r := Report{Mediator: e.Mediator, Subset: sub}
		for i, v := range sub.Verified {
			r.Decisions = append(r.Decisions, Decision{Index: i, By: v.Statement().Predicate.By})
		}
		return r, nil
	}
	// Row 0: the founding set is the Accord's.
	at := earliest
	if len(e.Receipts) > 0 {
		if s, err := receipts.Decode(e.Receipts[0]); err == nil {
			at = s.Predicate.At
		}
	}
	a, err := setup.Found(e.Founding, at)
	if id, herr := canonical.TypedHash(e.Founding); err != nil || herr != nil || id != e.Accord ||
		(e.Mediator != "" && e.Mediator != a.Mediator()) {
		return Report{}, &Error{Code: CodeFounding, Index: -1}
	}
	sub, err := verify(a.Mediator(), e.Receipts, e.Subset)
	if err != nil {
		return Report{}, err
	}
	r := Report{Mediator: a.Mediator(), Subset: sub, Module: kernelOf(e.Founding) == e.Kernel && e.Kernel != "",
		Operator: a.Operator(), Members: len(a.Members())}
	for _, m := range a.Members() {
		if a.Principal(m) == r.Operator {
			r.OperatorPrincipalOf++
		}
	}
	c := checker{e: e, a: a, r: &r, questions: map[int]*question{}, calls: map[string][]byte{}, used: map[string]bool{}}
	for _, b := range e.Answers {
		if id := answer.ID(b); id != "" {
			if _, seen := c.calls[id]; !seen {
				c.calls[id] = b
			}
		}
	}
	for i := range sub.Verified {
		if err := c.receipt(i); err != nil {
			return Report{}, err
		}
	}
	for _, b := range e.Answers {
		if id := answer.ID(b); id == "" || !c.used[id] {
			r.Unmatched++
		}
	}
	return r, nil
}

// verify is the validity the evidence asks for: of a whole list, or of a subset.
func verify(mediator string, list [][]byte, subset bool) (receipts.Subset, error) {
	if subset {
		return receipts.VerifySubset(mediator, list)
	}
	v, err := receipts.Verify(mediator, list)
	if err != nil {
		return receipts.Subset{}, err
	}
	places, err := receipts.Releases(v)
	if err != nil {
		return receipts.Subset{}, err
	}
	return receipts.Subset{Verified: v, Places: places}, nil
}

// kernelOf reads the card's kernel hash from a founding set founding has accepted.
func kernelOf(founding []byte) string {
	v, _, err := shape.Decode(founding)
	if err != nil {
		return ""
	}
	set, _ := v.(shape.Object)
	card, _ := set["card"].(shape.Object)
	kernel, _ := card["kernel"].(shape.Object)
	h, _ := kernel["hash"].(string)
	return h
}

// A checker runs rows 1 to 4 over the verified list.
type checker struct {
	e         Evidence
	a         *setup.Accord
	r         *Report
	questions map[int]*question
	calls     map[string][]byte
	used      map[string]bool
}

func (c *checker) statement(i int) receipts.Statement { return c.r.Subset.Verified[i].Statement() }

// receipt runs rows 1 to 4 over the receipt at index i, and records its decision.
func (c *checker) receipt(i int) error {
	s := c.statement(i)
	p := s.Predicate
	place := c.r.Subset.Places[i]
	d := Decision{Index: i, By: p.By, Checked: place.Role != receipts.RoleOutside}
	fail := func(code Code) error { return &Error{Code: code, Index: i} }
	// Row 1: the Accord and the card.
	if p.Accord != c.e.Accord || p.Card != c.a.Card() {
		return fail(CodeAccord)
	}
	// Row 2: the parties are members, or the sender the mediator where it closes a question (maybe-4,
	// maybe-5).
	byMediator := p.Kind == receipts.KindResolution && (p.Reason == receipts.ReasonTimeout || p.Reason == receipts.ReasonUndeliverable) &&
		p.From == c.r.Mediator
	if !c.a.Member(p.To) || (!c.a.Member(p.From) && !byMediator) {
		return fail(CodeParty)
	}
	if place.Role == receipts.RoleQuestion {
		c.questions[i] = &question{index: i, ask: s, closing: -1, release: -1}
	}
	var q *question
	if place.Role == receipts.RoleClosing || place.Role == receipts.RoleRelease {
		q = c.questions[place.Question]
		if place.Role == receipts.RoleClosing {
			q.closing = i
		} else {
			q.release = i
		}
	}
	// Row 3: the decision, by the module the verifier trusts.
	if c.r.Module && d.Checked {
		if !c.decided(i, s, place, q) {
			return fail(CodeDecision)
		}
	}
	// Row 4: the witness of an answer.
	if p.By.Decider == receipts.ByAnswer && d.Checked {
		w, ok := c.witness(i, s, place, q)
		if !ok {
			return fail(CodeWitness)
		}
		d.Witness = w
	} else if p.By.Decider == receipts.ByAnswer {
		d.Witness = Unwitnessed
		if _, has := c.calls[p.By.Answer]; has {
			c.used[p.By.Answer] = true
		}
	}
	c.r.Decisions = append(c.r.Decisions, d)
	return nil
}

// decided is row 3 for the receipt at index i: decided again as the module decides it, with the
// judgment its by claims, it is the decision the receipt states (verify-2).
func (c *checker) decided(i int, s receipts.Statement, place receipts.Place, q *question) bool {
	p := s.Predicate
	switch place.Role {
	case receipts.RoleGive, receipts.RoleQuestion:
		var j *composition.Judgment
		if p.By.Decider == receipts.ByJudge {
			j = &composition.Judgment{Name: p.By.Judge, Verdict: composition.Block}
			if p.Verdict == string(composition.Pass) {
				j.Verdict = composition.Pass
			}
		}
		d := composition.Decide(c.a, composition.Crossing{From: p.From, To: p.To, Kind: s.Subject.Name, Judge: j})
		if string(d.Verdict) != p.Verdict || d.Reason != p.Reason || (p.By.Decider == receipts.ByJudge) != (d.Judge == p.By.Judge && d.Judge != "") {
			return false
		}
		if place.Role == receipts.RoleQuestion {
			qq := c.questions[i]
			qq.rule, qq.deadline = true, shape.AddSeconds(p.At, d.Rule.Timeout)
			qq.answerers = c.answerers(d.Rule.To)
		}
		return true
	case receipts.RoleClosing:
		if !q.rule {
			return false
		}
		first := c.r.Mediator
		if len(q.answerers) > 0 {
			first = q.answerers[0]
		}
		switch p.Reason {
		case receipts.ReasonTimeout:
			return p.At == q.deadline && p.From == first
		case receipts.ReasonUndeliverable:
			return p.At < q.deadline && p.From == first
		case receipts.ReasonDeclined, receipts.ReasonCancelled:
			return p.At < q.deadline && len(q.answerers) > 0 && p.From == first
		}
		if p.At >= q.deadline || !contains(q.answerers, p.From) {
			return false
		}
		d := composition.Decide(c.a, composition.Crossing{From: p.From, To: p.To, Kind: s.Subject.Name, Formed: true})
		return string(d.Verdict) == p.Verdict && d.Reason == p.Reason
	case receipts.RoleRelease:
		closing := c.statement(q.closing).Predicate
		if p.At != closing.At {
			return false
		}
		if p.Kind == receipts.KindCrossing {
			d := composition.Decide(c.a, composition.Crossing{From: p.From, To: p.To, Kind: s.Subject.Name})
			if d.Verdict == composition.Block {
				return p.Verdict == string(d.Verdict) && p.Reason == d.Reason
			}
			return p.Verdict == string(composition.Pass) && p.Reason == composition.ReasonOK
		}
		d := composition.Decide(c.a, composition.Crossing{From: p.From, To: p.To, Kind: s.Subject.Name, Formed: true})
		return string(d.Verdict) == p.Verdict && d.Reason == p.Reason
	}
	return true
}

// witness is row 4 for a receipt an answer decided (verify-3): a call given whose typed hash is the
// receipt's by.answer must read, name this Accord and the receipt's question, take the action the
// receipt records, name its digest, be signed by the member who may take that action, and ask for
// release exactly where a release receipt follows. With no such call the answer is unwitnessed.
func (c *checker) witness(i int, s receipts.Statement, place receipts.Place, q *question) (Witness, bool) {
	p := s.Predicate
	b, has := c.calls[p.By.Answer]
	if !has {
		return Unwitnessed, true
	}
	c.used[p.By.Answer] = true
	call, err := answer.Read(b)
	if err != nil || call.Accord != c.e.Accord || q == nil || call.Question != q.ask.ID {
		return "", false
	}
	if place.Role == receipts.RoleRelease {
		// A release follows only an answer whose call asks for one.
		return Witnessed, call.Release && (call.Action == answer.Accepted || call.Action == answer.Changed)
	}
	asker := q.ask.Predicate.From
	switch p.Reason {
	case receipts.ReasonDeclined:
		// With the rule unknown, the module not this build's, the answerers are not known either: the
		// signer must at least be a member.
		signer := contains(q.answerers, call.From) || (!q.rule && c.a.Member(call.From))
		return Witnessed, call.Action == answer.Declined && signer
	case receipts.ReasonCancelled:
		return Witnessed, call.Action == answer.Cancelled && call.From == asker
	}
	want := answer.Accepted
	if s.Subject.Name == receipts.NameChanged {
		want = answer.Changed
	}
	if call.Action != want || call.Digest != s.Subject.Digest || call.From != p.From {
		return "", false
	}
	// A passing answer whose call asks for release has its release receipt right after it, unless it
	// is a change whose receiver is its answerer or its asker, which releases nothing (maybe-10). In a
	// subset a receipt left out is no evidence that it was never written, so this is not checked.
	if p.Verdict == string(composition.Pass) && call.Release && !c.e.Subset {
		receiver := q.ask.Predicate.To
		nothing := want == answer.Changed && (receiver == p.From || receiver == asker)
		next := i + 1
		released := next < len(c.r.Subset.Places) && c.r.Subset.Places[next].Role == receipts.RoleRelease &&
			c.r.Subset.Places[next].Question == q.index
		if !nothing && !released {
			return "", false
		}
	}
	return Witnessed, true
}

// answerers are the members whose bindings give them a role of roles, in actor-reference order
// (section 6).
func (c *checker) answerers(roles []string) []string {
	var out []string
	for _, m := range c.a.Members() {
		for _, r := range c.a.Roles(m) {
			if contains(roles, r) {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// Body is row 5 of verify-2, and the only place it runs: the delivery's receipt is one of the
// report's, by its id, and the salted digest of the delivery's body under its salt is that receipt's
// subject digest, of its kind (crossing-3). It reads only the delivery and the receipt the report
// verified. A delivery whose receipt is not in the report fails body at index -1, and one whose body
// or salt does not make the digest at its receipt's index.
func Body(r Report, delivery []byte) error {
	v, _, err := shape.Decode(delivery)
	o, ok := v.(shape.Object)
	if err != nil || !ok {
		return &Error{Code: CodeBody, Index: -1}
	}
	id, _ := o["receiptId"].(string)
	index := -1
	for i, x := range r.Subset.Verified {
		if x.Statement().ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return &Error{Code: CodeBody, Index: -1}
	}
	fail := &Error{Code: CodeBody, Index: index}
	st := r.Subset.Verified[index].Statement()
	if inner := shape.Encode(o["receipt"]); inner == nil {
		return fail
	} else if s, err := receipts.Decode(inner); err != nil || s.ID != id {
		return fail
	}
	salt, ok := crossing.Salt(o["salt"])
	if !ok || o["kind"] != any(st.Subject.Name) {
		return fail
	}
	form, bodies := "", 0
	for _, f := range crossing.Forms {
		if _, has := o[f]; has {
			form, bodies = f, bodies+1
		}
	}
	body, ok := crossing.Body(form, o[form])
	if bodies != 1 || !ok || receipts.Digest(salt, body) != st.Subject.Digest {
		return fail
	}
	return nil
}
