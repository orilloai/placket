// Package receipts builds the mediator's signed receipts, verifies a list of them with the
// mediator's key alone, reads one without verifying it, recognises a question's receipts and its
// release, and reconciles presented receipts with what each party kept.
//
// It implements receipt-1 to receipt-11 and crossing-5: the receipt (receipt-1), its subject
// (receipt-2), its predicate (receipt-3, receipt-9), its decider (receipt-11), verdicts and reasons
// (receipt-4), chains
// (receipt-5), the verifier (receipt-6, section 8's table), reconciliation (receipt-7, section 9's
// table), reading without verifying (receipt-8), sequence numbers and times (receipt-10), and the
// echoed envelope (crossing-5). Releases is the one function that recognises a question's receipts
// and its release (release-in-resolve).
package receipts

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// The fixed values of a receipt and its statement (receipt-1). PredicateType ends in the protocol
// number, the card's spec (setup-contract-4); core/mediator's tests hold it to setup.Spec.
const (
	PayloadType   = "application/vnd.in-toto+json"
	StatementType = "https://in-toto.io/Statement/v1"
	PredicateType = "https://placket.tech/receipt/0.1"
)

// The kinds of receipt (receipt-3).
const (
	KindCrossing   = "crossing"
	KindResolution = "resolution"
)

// The reasons a receipt may carry besides composition's, all on resolution receipts only (receipt-4).
const (
	ReasonDeclined      = "declined"
	ReasonCancelled     = "cancelled"
	ReasonTimeout       = "timeout"
	ReasonUndeliverable = "undeliverable"
)

// ReasonClosed is the reason of a question's closing that a close wrote, a resolution reason only
// (receipt-4, close-1).
const ReasonClosed = "closed"

// The subject names of an answer and of a changed version (maybe-8, maybe-9).
const (
	NameAnswer  = "answer"
	NameChanged = "changed-version"
)

// Code is a code of the verifier (receipt-6) or of reconciliation (receipt-7), or a misuse of Build
// (record 0005).
type Code string

// The codes, in the order of the two tables.
const (
	CodeReference     Code = "reference"
	CodeNotCanonical  Code = "not-canonical"
	CodePayloadType   Code = "payload-type"
	CodeSignature     Code = "signature"
	CodeStatementType Code = "statement-type"
	CodeMediator      Code = "mediator"
	CodeChain         Code = "chain"
	CodeRelease       Code = "release"
	// CodeClose and CodeAfterClose are rows 8 and 9 (close-4): a close that does not match the list
	// before it, or a closed closing out of its place; and a receipt after a close.
	CodeClose      Code = "close"
	CodeAfterClose Code = "after-close"
	CodePresented  Code = "presented"
	CodeKept       Code = "kept"
	// CodeStatement is not a code of the protocol: Build was given a statement no verifier would
	// accept.
	CodeStatement Code = "statement"
)

// Error is a failed verification or reconciliation: its code, the index of the receipt it failed at,
// and for kept the party's reference. Index is -1 for reference and for a misuse of Build
// (receipt-6, receipt-7, record 0005).
type Error struct {
	Code  Code
	Index int
	Actor string
}

// Error returns the package, the code, and the index and the party's reference where they are
// given, and nothing of the input.
func (e *Error) Error() string {
	s := "receipts: " + string(e.Code)
	if e.Index >= 0 {
		s += " at index " + strconv.Itoa(e.Index)
	}
	if e.Actor != "" {
		s += " of " + e.Actor
	}
	return s
}

// A Subject names the part a receipt is about: its kind and its salted digest in lowercase hex
// (receipt-2, crossing-3).
type Subject struct {
	Name, Digest string
}

// An Envelope is an echoed envelope: the reference of the key that signed it, its seq and its hash
// (crossing-5).
type Envelope struct {
	Sender string
	Seq    int64
	Hash   string
}

// Decider is who decided what a receipt states, the form of its predicate's by (receipt-11).
type Decider string

// The four deciders (receipt-11): the terms, a judge of the card by its name, an answer by its call's
// typed hash, and the mediator with the event that made it decide.
const (
	ByTerms    Decider = "terms"
	ByJudge    Decider = "judge"
	ByAnswer   Decider = "answer"
	ByMediator Decider = "mediator"
)

// Event is what made the mediator decide (receipt-11): a question's deadline passed (maybe-5),
// nobody could be reached to answer it (maybe-4), or the Accord was closed (close-1). The list is
// closed.
type Event string

// The events of a mediator's decision (receipt-11).
const (
	EventDeadline      Event = "deadline"
	EventUndeliverable Event = "undeliverable"
	EventClose         Event = "close"
)

// By is a predicate's decider (receipt-11): its Decider, and exactly the field that Decider names,
// Judge for a judge, Answer for an answer, Event for the mediator; none for the terms.
type By struct {
	Decider Decider
	Judge   string
	Answer  string
	Event   Event
}

// value writes b as the predicate's by: "terms", or an object of one member.
func (b By) value() any {
	switch b.Decider {
	case ByTerms:
		return string(ByTerms)
	case ByJudge:
		return shape.Object{string(ByJudge): b.Judge}
	case ByAnswer:
		return shape.Object{string(ByAnswer): b.Answer}
	case ByMediator:
		return shape.Object{string(ByMediator): string(b.Event)}
	}
	return nil
}

// readBy reads a predicate's by for form (receipt-11): "terms"; or an object of exactly one member,
// judge with a label, answer with a typed hash, or mediator with deadline, undeliverable or close.
func readBy(v any) (By, bool) {
	if s, ok := v.(string); ok {
		return By{Decider: ByTerms}, s == string(ByTerms)
	}
	o, ok := v.(shape.Object)
	if !ok || len(o) != 1 {
		return By{}, false
	}
	for k, x := range o {
		s, ok := x.(string)
		if !ok {
			return By{}, false
		}
		switch Decider(k) {
		case ByJudge:
			return By{Decider: ByJudge, Judge: s}, shape.Label(s)
		case ByAnswer:
			return By{Decider: ByAnswer, Answer: s}, shape.Hash(s)
		case ByMediator:
			e := Event(s)
			return By{Decider: ByMediator, Event: e}, e == EventDeadline || e == EventUndeliverable || e == EventClose
		}
	}
	return By{}, false
}

// A Predicate is what a receipt states (receipt-3). Prev is empty exactly when Seq is 1.
type Predicate struct {
	Accord, Card, Mediator string
	Kind                   string
	From, To               string
	Seq                    int64
	Prev                   string
	At                     string
	Envelope               Envelope
	Verdict, Reason        string
	By                     By
}

// A Statement is a receipt's statement as read from its payload, and the receipt's id, the typed hash
// of its canonical bytes (receipt-1).
type Statement struct {
	ID        string
	Subject   Subject
	Predicate Predicate
}

// Verified is a receipt's statement that passed Verify, or one this package built and signed; or a
// close that did (close-2). Only this package makes one that holds a statement or a close, so a
// caller holding a Verified holds one that was checked (verified-by-type, record 0016). The zero
// Verified, which any package can write, holds neither: its Statement is the zero Statement, with no
// id, and it is no close.
type Verified struct {
	s Statement
	c *Close
}

// Statement returns a copy of the verified statement (record 0016). For a close it is the zero
// Statement: a close is read with Close.
func (v Verified) Statement() Statement { return v.s }

// Close returns a copy of the verified close, and true, when v is one (close-2, record 0022).
func (v Verified) Close() (Close, bool) {
	if v.c == nil {
		return Close{}, false
	}
	return v.c.copy(), true
}

// Digest returns the salted digest of a part: the lowercase hex SHA-256 of its salt's bytes followed
// by its body's bytes (crossing-3). The salt comes first, in the order of crossing-3's formula: an
// exception to the data-first order of inputs-as-parameters, recorded in record 0016.
func Digest(salt, body []byte) string {
	h := sha256.New()
	h.Write(salt)
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func (p Predicate) value() shape.Object {
	o := shape.Object{
		"accord": p.Accord, "card": p.Card, "mediator": p.Mediator, "kind": p.Kind,
		"from": p.From, "to": p.To, "seq": p.Seq, "at": p.At,
		"envelope": shape.Object{"sender": p.Envelope.Sender, "seq": p.Envelope.Seq, "hash": p.Envelope.Hash},
		"verdict":  p.Verdict, "reason": p.Reason, "by": p.By.value(),
	}
	if p.Prev != "" {
		o["prev"] = p.Prev
	}
	return o
}

// Build writes the receipt of subject and predicate, signed by the mediator's key, and returns it and
// its statement, whose ID is the receipt's id (receipt-1, record 0016). The
// predicate's mediator is the key's reference, whatever it held. A statement the verifier's row 4
// would refuse is not written.
func Build(subject Subject, predicate Predicate, key signing.Key) ([]byte, Verified, error) {
	predicate.Mediator = key.Reference()
	statement := shape.Encode(shape.Object{
		"_type":         StatementType,
		"subject":       []any{shape.Object{"name": subject.Name, "digest": shape.Object{"salted-sha256": subject.Digest}}},
		"predicateType": PredicateType,
		"predicate":     predicate.value(),
	})
	r, ok := readStatement(statement)
	if _, placed := r.placed(); !ok || !placed {
		return nil, Verified{}, &Error{Code: CodeStatement, Index: -1}
	}
	sig, err := signing.SignPayload(PayloadType, statement, key)
	if err != nil {
		return nil, Verified{}, &Error{Code: CodeStatement, Index: -1}
	}
	receipt := shape.Encode(shape.Object{
		"payloadType": PayloadType,
		"payload":     base64.StdEncoding.EncodeToString(statement),
		"signatures":  []any{shape.Object{"keyid": predicate.Mediator, "sig": base64.StdEncoding.EncodeToString(sig)}},
	})
	return receipt, Verified{s: r.Statement}, nil
}

func typedHash(canonicalBytes []byte) string {
	sum := sha256.Sum256(canonicalBytes)
	return canonical.HashPrefix + hex.EncodeToString(sum[:])
}

// isCanonical reports whether b is its own canonical form: bytes the canonical form accepts and
// would write back unchanged, as a receipt's payload must be (receipt-1).
func isCanonical(b []byte) bool {
	c, err := canonical.Form(b)
	return err == nil && bytes.Equal(c, b)
}

// open reads one receipt through rows 1 to 3 of section 8 under the mediator's reference. The
// receipt's own bytes need only be accepted by the canonical form; its payload must be canonical.
func open(receipt []byte, mediator string) ([]byte, Code) {
	v, _, err := shape.Decode(receipt)
	if err != nil {
		return nil, CodeNotCanonical
	}
	o, _ := v.(shape.Object)
	var payload []byte
	if s, ok := o["payload"].(string); ok {
		if b, ok := shape.Base64(s); ok {
			if !isCanonical(b) {
				return nil, CodeNotCanonical
			}
			payload = b
		}
	}
	// Row 2.
	if o == nil || o["payloadType"] != PayloadType {
		return nil, CodePayloadType
	}
	// Row 3.
	if payload == nil || !shape.Only(o, "payloadType", "payload", "signatures") {
		return nil, CodeSignature
	}
	sigs, ok := o["signatures"].([]any)
	if !ok || len(sigs) != 1 {
		return nil, CodeSignature
	}
	entry, ok := sigs[0].(shape.Object)
	if !ok || !shape.Only(entry, "keyid", "sig") || entry["keyid"] != mediator {
		return nil, CodeSignature
	}
	s, _ := entry["sig"].(string)
	sig, ok := shape.Base64(s)
	if !ok || len(sig) != 64 || !signing.VerifyPayload(PayloadType, payload, sig, mediator) {
		return nil, CodeSignature
	}
	return payload, ""
}

// A reading is a statement read by row 4, with the members of its predicate rows 5 and 6 read, as
// found.
type reading struct {
	Statement
	mediator, seq, prev any
	hasPrev             bool
}

// named reports whether the predicate's mediator has the form receipt-3 gives it, an actor
// reference, whichever mediator it names.
func (r reading) named() bool {
	s, ok := r.mediator.(string)
	return ok && isReference(s)
}

// placed reads the predicate's seq as receipt-10 gives it, a whole number from 1 to 2^53 - 1, and
// reports whether its prev has the form receipt-3 gives it: a typed hash, present exactly when seq is
// above 1. It reads the predicate alone, not the receipts before it.
func (r reading) placed() (int64, bool) {
	seq, ok := shape.Int(r.seq)
	if !ok || r.hasPrev != (seq > 1) {
		return 0, false
	}
	if r.hasPrev {
		if s, ok := r.prev.(string); !ok || !shape.Hash(s) {
			return 0, false
		}
	}
	return seq, true
}

// readStatement reads a statement by row 4 of section 8: its members and values (receipt-1,
// receipt-2), its predicate's (receipt-3, receipt-4, receipt-9, receipt-10) but mediator, seq and
// prev, and a crossing receipt's envelope sender equal to its from (receipt-3).
func readStatement(payload []byte) (reading, bool) {
	var out reading
	v, c, err := shape.Decode(payload)
	if err != nil {
		return out, false
	}
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "_type", "subject", "predicateType", "predicate") ||
		o["_type"] != StatementType || o["predicateType"] != PredicateType {
		return out, false
	}
	subjects, ok := o["subject"].([]any)
	if !ok || len(subjects) != 1 {
		return out, false
	}
	sub, ok := subjects[0].(shape.Object)
	if !ok || !shape.Only(sub, "name", "digest") {
		return out, false
	}
	name, ok := sub["name"].(string)
	if !ok || !shape.Label(name) {
		return out, false
	}
	dg, ok := sub["digest"].(shape.Object)
	if !ok || !shape.Only(dg, "salted-sha256") {
		return out, false
	}
	digest, ok := dg["salted-sha256"].(string)
	if !ok || len(digest) != 64 || !shape.LowerHex(digest) {
		return out, false
	}
	out.Subject = Subject{Name: name, Digest: digest}

	p, ok := o["predicate"].(shape.Object)
	if !ok || !shape.Only(p, "accord", "card", "kind", "from", "to", "at", "envelope", "verdict", "reason", "by", "",
		"mediator", "seq", "prev") {
		return out, false
	}
	var pr Predicate
	str := func(k string, valid func(string) bool) bool {
		s, ok := p[k].(string)
		if ok && valid(s) {
			switch k {
			case "accord":
				pr.Accord = s
			case "card":
				pr.Card = s
			case "from":
				pr.From = s
			case "to":
				pr.To = s
			case "at":
				pr.At = s
			}
		}
		return ok && valid(s)
	}
	if !str("accord", shape.Hash) || !str("card", shape.Hash) || !str("from", isReference) ||
		!str("to", isReference) || !str("at", shape.Timestamp) {
		return out, false
	}
	pr.Kind, _ = p["kind"].(string)
	pr.Verdict, _ = p["verdict"].(string)
	pr.Reason, _ = p["reason"].(string)
	if !pairs(pr.Kind, pr.Verdict, pr.Reason) {
		return out, false
	}
	if pr.By, ok = readBy(p["by"]); !ok || !fits(pr.Kind, pr.Verdict, pr.Reason, pr.By) {
		return out, false
	}
	e, ok := p["envelope"].(shape.Object)
	if !ok || !shape.Only(e, "sender", "seq", "hash") {
		return out, false
	}
	sender, ok := e["sender"].(string)
	if !ok || !isReference(sender) {
		return out, false
	}
	eseq, ok := shape.Int(e["seq"])
	if !ok {
		return out, false
	}
	ehash, ok := e["hash"].(string)
	if !ok || !shape.Hash(ehash) {
		return out, false
	}
	pr.Envelope = Envelope{Sender: sender, Seq: eseq, Hash: ehash}
	if pr.Kind == KindCrossing && sender != pr.From {
		return out, false
	}
	out.mediator, out.seq, out.prev = p["mediator"], p["seq"], p["prev"]
	_, out.hasPrev = p["prev"]
	pr.Mediator, _ = p["mediator"].(string)
	pr.Seq, _ = shape.Int(p["seq"])
	pr.Prev, _ = p["prev"].(string)
	out.Predicate = pr
	out.ID = typedHash(c)
	return out, true
}

func isReference(s string) bool {
	_, ok := signing.ParseReference(s)
	return ok
}

// pairs reports whether a kind, a verdict and a reason pair as receipt-4 says.
func pairs(kind, verdict, reason string) bool {
	if kind != KindCrossing && kind != KindResolution {
		return false
	}
	resolutionOnly := reason == ReasonDeclined || reason == ReasonCancelled || reason == ReasonTimeout ||
		reason == ReasonUndeliverable || reason == ReasonClosed
	_, judged := judgeReason(reason)
	if !resolutionOnly && !judged && reason != composition.ReasonOK && reason != composition.ReasonNoMatch &&
		reason != composition.ReasonMaybe {
		return false
	}
	if (kind == KindCrossing && resolutionOnly) || (kind == KindResolution && judged) ||
		(judged && verdict != string(composition.Block)) {
		return false
	}
	switch composition.Verdict(verdict) {
	case composition.Pass:
		return reason == composition.ReasonOK
	case composition.Escalate:
		return kind == KindCrossing && reason == composition.ReasonMaybe
	case composition.Block:
		return reason != composition.ReasonOK
	}
	return false
}

// judgeReason reads r as a judge's block, the reason composition.ReasonJudge writes: judge: and a
// label, the judge's card name (receipt-4).
func judgeReason(r string) (string, bool) {
	name, ok := cutPrefix(r, composition.ReasonJudge(""))
	return name, ok && shape.Label(name)
}

// fits reports whether a predicate's by fits its kind, verdict and reason, by the table of receipt-11
// that the verifier's row 4 reads. A judge's block names the judge its reason names. Whether an
// answer's receipt is a release, and whose answer, is row 7's to read across the list.
func fits(kind, verdict, reason string, by By) bool {
	if name, judged := judgeReason(reason); judged {
		return kind == KindCrossing && by.Decider == ByJudge && by.Judge == name
	}
	pass, block := verdict == string(composition.Pass), verdict == string(composition.Block)
	if kind == KindCrossing {
		switch {
		case verdict == string(composition.Escalate):
			return by.Decider == ByTerms
		case pass:
			return by.Decider == ByTerms || by.Decider == ByJudge || by.Decider == ByAnswer
		case block:
			return by.Decider == ByTerms || by.Decider == ByAnswer
		}
		return false
	}
	switch reason {
	case ReasonTimeout:
		return by.Decider == ByMediator && by.Event == EventDeadline
	case ReasonUndeliverable:
		return by.Decider == ByMediator && by.Event == EventUndeliverable
	case ReasonClosed:
		return block && by.Decider == ByMediator && by.Event == EventClose
	}
	return (pass || block) && by.Decider == ByAnswer
}

func cutPrefix(s, p string) (string, bool) {
	if len(s) >= len(p) && s[:len(p)] == p {
		return s[len(p):], true
	}
	return "", false
}

// showBy is the line that names a receipt's decider (receipt-8, receipt-11): by terms, by judge and
// its name, by answer and its call's typed hash, or by mediator and its event.
func showBy(b By) string {
	switch b.Decider {
	case ByJudge:
		return "by judge " + b.Judge
	case ByAnswer:
		return "by answer " + b.Answer
	case ByMediator:
		return "by mediator " + string(b.Event)
	}
	return "by " + string(b.Decider)
}

// Decode reads a receipt's statement from its payload without verifying anything: not its
// signature, which mediator it names, its place in its chain or its place among questions
// (receipt-8). It does hold every member to its form, as the verifier's rows 4 to 6 read it alone: a
// mediator that is not an actor reference is refused with mediator, and a seq or prev of the wrong
// form with chain, so nothing of an unverified receipt reaches its reader unread. Whatever presents
// what it returns must say that the receipt was not verified.
//
// A close receipt holds no receipt's statement: Decode refuses it with statement-type, and
// DecodeClose reads it (close-2).
//
// It reads neither the payload type nor the envelope's other members (rows 2 and 3), so its code
// can differ from the verifier's for one receipt: a wrong payload type with a payload that is not
// base64 is the verifier's payload-type and Decode's signature.
func Decode(receipt []byte) (Statement, error) {
	payload, err := payloadOf(receipt)
	if err != nil {
		return Statement{}, err
	}
	st, ok := readStatement(payload)
	if !ok {
		return Statement{}, &Error{Code: CodeStatementType, Index: 0}
	}
	if !st.named() {
		return Statement{}, &Error{Code: CodeMediator, Index: 0}
	}
	if _, ok := st.placed(); !ok {
		return Statement{}, &Error{Code: CodeChain, Index: 0}
	}
	return st.Statement, nil
}

// NotVerified is the line Show writes first: what follows was read from a receipt without verifying
// it (receipt-8).
const NotVerified = "not verified: this receipt was decoded without checking its signature, its mediator or its chain"

// Show decodes a receipt without verifying it and returns its words, one line each, the first saying
// that it was not verified (receipt-8). A forged receipt shows exactly as a real one does. A close
// receipt shows as a close, with one line per chain and per delivery not yet fetched (close-2).
func Show(receipt []byte) ([]string, error) {
	if payload, err := payloadOf(receipt); err == nil && isClose(payload) {
		c, err := DecodeClose(receipt)
		if err != nil {
			return nil, err
		}
		return showClose(c), nil
	}
	s, err := Decode(receipt)
	if err != nil {
		return nil, err
	}
	p := s.Predicate
	lines := []string{
		NotVerified,
		"receipt " + s.ID,
		p.Kind + " from " + p.From + " to " + p.To,
		"verdict " + p.Verdict + ", reason " + p.Reason,
		showBy(p.By),
		"subject " + s.Subject.Name + ", salted-sha256 " + s.Subject.Digest,
		"accord " + p.Accord + ", card " + p.Card,
		"mediator " + p.Mediator + ", seq " + strconv.FormatInt(p.Seq, 10) + ", at " + p.At,
		"envelope of " + p.Envelope.Sender + ", seq " + strconv.FormatInt(p.Envelope.Seq, 10) + ", hash " + p.Envelope.Hash,
	}
	if p.Prev != "" {
		lines = append(lines, "prev "+p.Prev)
	}
	return lines, nil
}
