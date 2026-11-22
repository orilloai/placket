// Package conform checks a mediator's receipts for conformance to their Accord: that every decision
// a receipt states is one the Accord let its decider make. It checks validity first, by the verifier
// of section 8, and then the rows of section 8's second table, receipt by receipt, without ever
// running a judge again or reading a body: a judge's outcome is taken as the receipt claims it, and a
// body is read only by Body, for the one delivery a caller gives.
//
// It implements verify-1 (what a verifier is given: the Accord id, its founding set, the receipts,
// and optionally answer calls, a subset and a delivery), verify-2 (the conformance table: founding,
// Accord, parties, decision, witness and body) and verify-3 (an answer is witnessed by a call that
// checks, contradicted by one that does not fit, a failure, and with no call unwitnessed, never a
// failure). Its decision row re-runs composition.Decide,
// the one decision module, only when the card's kernel hash is the one its caller trusts
// (one-decision-module, docs/adr/0021-who-decided-and-the-answer-call.md). With one member's view,
// rows 0a and 4a check that the view is the one the member accepted and that each receipt of the
// member's matches the view's rows, reading the view and never the terms (view.go,
// docs/adr/0026-verify-against-a-view.md).
package conform

import (
	"strconv"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/internal/crossing"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

// Evidence is what a verifier is given (verify-1): the Accord id; the mediator's reference, used
// only when no founding set is given, and otherwise empty or the set's; the founding set, its
// acceptances and the openings the verifier holds, none, some or every member's (setup-contract-16);
// the receipts in the order the mediator wrote them; answer calls, each a call's bytes; whether the
// receipts are a subset; Kernel, the kernel hash whose module this verifier trusts to decide
// again; and View, one member's view as that member kept it, read only with a founding set
// (verify-1 rows 0a and 4a).
type Evidence struct {
	Accord, Mediator  string
	Founding          []byte
	Acceptances       []byte
	Openings          setup.Openings
	Receipts, Answers [][]byte
	Subset            bool
	Kernel            string
	View              []byte
}

// Witness is what a receipt an answer decided says of its answer (verify-3): witnessed by a call
// that checks, unwitnessed, when no call given matches it, or contradicted, when the call given for
// it does not fit, which fails row 4. A receipt no answer decided has none.
type Witness string

// The three witnesses (verify-3). Contradicted is an outcome under the code witness, never a
// witness of the answer: a receipt contradicted always fails row 4.
const (
	Witnessed    Witness = "witnessed"
	Unwitnessed  Witness = "unwitnessed"
	Contradicted Witness = "contradicted"
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
	Mediator string
	// Close is the id of the close receipt the list holds, and empty when it holds none (close-4).
	Close                        string
	Subset                       receipts.Subset
	Decisions                    []Decision
	Module                       bool
	Operator                     string
	OperatorPrincipalOf, Members int
	Unmatched                    int
	// Terms is whether every member's seal was opened, so the decision row could read every
	// crossing's terms (setup-contract-16, verify-1).
	Terms bool
	// Viewer is the viewer of the view given, and Viewed each receipt of the viewer's read against
	// it, in list order (verify-1 row 4a, verify-2); both are empty without a view.
	Viewer string
	Viewed []Viewed
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

// FoundedAt is the time a founding set is founded at for the list (verify-1 row 0,
// setup-contract-16): the first receipt's at, a close's when the close is the first, read without
// verifying. With no receipt, or a first that reads as neither, there is no time to found at, and it
// is the earliest time, 0001-01-01T00:00:00Z, so that no binding's until is checked, whenever the
// verifier runs.
func FoundedAt(list [][]byte) string {
	if len(list) == 0 {
		return earliest
	}
	if s, err := receipts.Decode(list[0]); err == nil {
		return s.Predicate.At
	}
	if c, err := receipts.DecodeClose(list[0]); err == nil {
		return c.At
	}
	return earliest
}

// A question is what the decision and witness rows read of a question of the list.
type question struct {
	index     int
	ask       receipts.Statement
	rule      bool     // whether the rule is known: the module decided the ask again
	answerers []string // in actor-reference order
	deadline  string
	closing   int
	release   int
	// witness is the witness of the question's closing, which its release reads (verify-3).
	witness Witness
}

// Check checks the evidence (verify-1, verify-2). Without a founding set it checks validity alone,
// under the mediator's reference it is given: receipts.Verify, or receipts.VerifySubset for a subset,
// whose *receipts.Error it returns. With one, row 0 comes first: the set founds at the first
// receipt's time, its typed hash is the Accord id, and the mediator, when given, is its
// mediator.actor; a failure is founding. With a view, row 0a next: the view is the one its viewer
// accepted, or it is acceptance-view. Then validity under the set's mediator, and then rows 1 to 4,
// and with a view row 4a, per receipt in list order, every row of one receipt before the next; the
// first failure is a *Error with its code and the receipt's index. Row 3 runs only when the card's
// kernel hash is Kernel; rows 1, 2, 4 and 4a run whatever the module. Row 5 is Body's.
//
// A validity or founding failure returns an empty Report. A conformance failure returns the Report
// of the whole list with it: every receipt's decision, each witness the calls show, and the count of
// calls matching no receipt, so that a log that does not conform can still be read decision by
// decision. Only the first failure is returned.
func Check(e Evidence) (Report, error) {
	if len(e.Founding) == 0 {
		sub, err := verify(e.Mediator, e.Receipts, e.Subset)
		if err != nil {
			return Report{}, err
		}
		r := Report{Mediator: e.Mediator, Subset: sub}
		for i, v := range sub.Verified {
			if c, ok := v.Close(); ok {
				r.Close = c.ID
				continue
			}
			r.Decisions = append(r.Decisions, Decision{Index: i, By: v.Statement().Predicate.By,
				Checked: sub.Places[i].Role != receipts.RoleOutside})
		}
		return r, nil
	}
	// Row 0: the founding set is the Accord's, founded at the first receipt's time, a close's when the
	// close is the first (close-4), or with no receipt at none.
	a, err := founding.Found(founding.Inputs{Set: e.Founding, Openings: e.Openings, Acceptances: e.Acceptances}, FoundedAt(e.Receipts))
	if id, herr := canonical.TypedHash(e.Founding); err != nil || herr != nil || id != e.Accord ||
		(e.Mediator != "" && e.Mediator != a.Mediator()) {
		return Report{}, &Error{Code: CodeFounding, Index: -1}
	}
	// Row 0a: the view given is the one its viewer accepted, before validity.
	var v view
	if e.View != nil {
		var ok bool
		if v, ok = readView(e.View, a, e.Accord, e.Acceptances); !ok {
			return Report{}, &Error{Code: CodeAcceptanceView, Index: -1}
		}
	}
	sub, err := verify(a.Mediator(), e.Receipts, e.Subset)
	if err != nil {
		return Report{}, err
	}
	r := Report{Mediator: a.Mediator(), Subset: sub, Module: kernelOf(e.Founding) == e.Kernel && e.Kernel != "",
		Operator: a.Operator(), Members: len(a.Members()), Terms: a.TermsChecked()}
	for _, m := range a.Members() {
		if a.Principal(m) == r.Operator {
			r.OperatorPrincipalOf++
		}
	}
	c := checker{e: e, a: a, r: &r, questions: map[int]*question{}, calls: map[string][]byte{}, used: map[string]bool{},
		view: v, viewQuestions: map[int]*viewQuestion{}}
	if e.View != nil {
		r.Viewer, r.Viewed = v.viewer, []Viewed{}
	}
	for _, b := range e.Answers {
		if id := callID(b); id != "" {
			if _, seen := c.calls[id]; !seen {
				c.calls[id] = b
			}
		}
	}
	var first error
	for i := range sub.Verified {
		if code := c.receipt(i); code != "" && first == nil {
			first = &Error{Code: code, Index: i}
		}
	}
	for _, b := range e.Answers {
		if id := callID(b); id == "" || !c.used[id] {
			r.Unmatched++
		}
	}
	return r, first
}

// callID is the typed hash of a call given in its canonical bytes, and empty for any other bytes. A
// call is matched to a receipt only as it was signed: answer.ID would give a copy re-encoded with
// other whitespace or member order the same hash, though only the canonical bytes read.
func callID(b []byte) string {
	if c, err := canonical.Form(b); err != nil || string(c) != string(b) {
		return ""
	}
	return answer.ID(b)
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
	// view is the view row 0a read, with no viewer when none was given, and viewQuestions the
	// questions of the viewer's, by the index of the ask.
	view          view
	viewQuestions map[int]*viewQuestion
}

func (c *checker) statement(i int) receipts.Statement { return c.r.Subset.Verified[i].Statement() }

// receipt runs rows 1 to 4 over the receipt at index i, records its decision, and returns the code
// of the first row that failed, or none. A row after a failed one is not run, but the receipt's
// question is recorded and its decision appended whatever failed, so the receipts after it are read
// as the list holds them. A close goes through row 1 alone, and is no decision (close-4).
func (c *checker) receipt(i int) Code {
	if cl, ok := c.r.Subset.Verified[i].Close(); ok {
		c.r.Close = cl.ID
		if cl.Accord != c.e.Accord || cl.Card != c.a.Card() {
			return CodeAccord
		}
		return ""
	}
	s := c.statement(i)
	p := s.Predicate
	place := c.r.Subset.Places[i]
	d := Decision{Index: i, By: p.By, Checked: place.Role != receipts.RoleOutside}
	if place.Role == receipts.RoleQuestion {
		c.questions[i] = &question{index: i, ask: s, closing: -1, release: -1}
	}
	var q *question
	if place.Role == receipts.RoleClosing || place.Role == receipts.RoleRelease {
		q = c.questions[place.Question]
		switch {
		case q == nil:
		case place.Role == receipts.RoleClosing:
			q.closing = i
		default:
			q.release = i
		}
	}
	var failed Code
	// Row 1: the Accord and the card.
	if p.Accord != c.e.Accord || p.Card != c.a.Card() {
		failed = CodeAccord
	}
	// Row 2: the parties are members, or the sender the mediator where it closes a question (maybe-4,
	// maybe-5, close-1).
	byMediator := p.Kind == receipts.KindResolution && (p.Reason == receipts.ReasonTimeout || p.Reason == receipts.ReasonUndeliverable ||
		p.Reason == receipts.ReasonClosed) && p.From == c.r.Mediator
	if failed == "" && (!c.a.Member(p.To) || (!c.a.Member(p.From) && !byMediator)) {
		failed = CodeParty
	}
	// Row 3: the decision, by the module the verifier trusts, over terms the verifier holds: it runs
	// only when every member whose terms the decision reads was opened (verify-1, setup-contract-16).
	if failed == "" && c.r.Module && d.Checked && c.reads(s, q) && !c.decided(i, s, place, q) {
		failed = CodeDecision
	}
	// Row 4: the witness of an answer. It is read even after a failed row, for the report.
	if p.By.Decider == receipts.ByAnswer && d.Checked {
		w := c.witness(i, s, place, q)
		if w == Contradicted && failed == "" {
			failed = CodeWitness
		}
		if q != nil && place.Role == receipts.RoleClosing {
			q.witness = w
		}
		d.Witness = w
	} else if p.By.Decider == receipts.ByAnswer {
		d.Witness = Unwitnessed
		if _, has := c.calls[p.By.Answer]; has {
			c.used[p.By.Answer] = true
		}
	}
	// Row 4a: a receipt of the viewer's against its view. It is read even after a failed row, so
	// that every one is shown.
	if c.view.viewer != "" && d.Checked {
		if read, matches := c.viewed(i, s, place); read && !matches && failed == "" {
			failed = CodeViewConformance
		}
	}
	c.r.Decisions = append(c.r.Decisions, d)
	return failed
}

// reads reports whether the verifier holds the terms row 3 reads for a receipt: those of each member
// among its from and to and, for a question's closing or release, among the question's asker and
// receiver. The mediator, a sender of some closings, is no member and has no terms.
func (c *checker) reads(s receipts.Statement, q *question) bool {
	who := []string{s.Predicate.From, s.Predicate.To}
	if q != nil {
		who = append(who, q.ask.Predicate.From, q.ask.Predicate.To)
	}
	for _, m := range who {
		if c.a.Member(m) && !c.a.Opened(m) {
			return false
		}
	}
	return true
}

// decided is row 3 for the receipt at index i: decided again as the module decides it, with the
// judgment its by claims, it is the decision the receipt states (verify-2).
func (c *checker) decided(i int, s receipts.Statement, place receipts.Place, q *question) bool {
	p := s.Predicate
	if q == nil && (place.Role == receipts.RoleClosing || place.Role == receipts.RoleRelease) {
		return false
	}
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
		case receipts.ReasonUndeliverable, receipts.ReasonClosed:
			// That a closed closing's at is its close's is validity's row 8 (close-4).
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
// release exactly where a release receipt follows. A release reads its closing's call, so a release
// after a closing that is contradicted is contradicted too. With no such call the answer is
// unwitnessed; with one that does not fit it is contradicted, and fails row 4.
func (c *checker) witness(i int, s receipts.Statement, place receipts.Place, q *question) Witness {
	p := s.Predicate
	b, has := c.calls[p.By.Answer]
	if !has {
		return Unwitnessed
	}
	c.used[p.By.Answer] = true
	call, err := answer.Read(b)
	if err != nil || call.Accord != c.e.Accord || q == nil || call.Question != q.ask.ID {
		return Contradicted
	}
	if place.Role == receipts.RoleRelease {
		// A release follows only an answer whose call asks for one, and fits it.
		return fits(q.witness != Contradicted && call.Release && (call.Action == answer.Accepted || call.Action == answer.Changed))
	}
	asker := q.ask.Predicate.From
	switch p.Reason {
	case receipts.ReasonDeclined:
		// With the rule unknown, the module not this build's, the answerers are not known either: the
		// signer must at least be a member.
		signer := contains(q.answerers, call.From) || (!q.rule && c.a.Member(call.From))
		return fits(call.Action == answer.Declined && signer)
	case receipts.ReasonCancelled:
		return fits(call.Action == answer.Cancelled && call.From == asker)
	}
	want := answer.Accepted
	if s.Subject.Name == receipts.NameChanged {
		want = answer.Changed
	}
	if call.Action != want || call.Digest != s.Subject.Digest || call.From != p.From {
		return Contradicted
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
			return Contradicted
		}
	}
	return Witnessed
}

// fits is the witness of a call given that fits its receipt, or not.
func fits(ok bool) Witness {
	if ok {
		return Witnessed
	}
	return Contradicted
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

// Body is row 5 of verify-1, and the only place it runs: the delivery is one by its format, then its
// receipt is one of the report's, by its id, and the salted digest of the delivery's body bytes under
// its salt is that receipt's subject digest, of its kind (crossing-3). It reads only the delivery and
// the receipt the report verified. A delivery that does not have exactly a delivery's members fails
// body at index -1, and so does one whose receipt is not in the report, an empty receiptId included,
// since a close is no receipt a delivery names; one whose body or salt does not make the digest fails
// at its receipt's index. A pass says the bytes are the part the receipt names, not their form, nor
// that the receipt let the part cross.
func Body(r Report, delivery []byte) error {
	v, _, err := shape.Decode(delivery)
	o, ok := v.(shape.Object)
	if err != nil || !ok {
		return &Error{Code: CodeBody, Index: -1}
	}
	form, ok := crossing.Shaped(o)
	if !ok {
		return &Error{Code: CodeBody, Index: -1}
	}
	id, _ := o["receiptId"].(string)
	index := -1
	for i, x := range r.Subset.Verified {
		// A close's statement is the zero one: a delivery never names a close.
		if _, isClose := x.Close(); !isClose && x.Statement().ID == id {
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
	body, ok := crossing.Body(form, o[form])
	if !ok || receipts.Digest(salt, body) != st.Subject.Digest {
		return fail
	}
	return nil
}
