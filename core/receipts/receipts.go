// Package receipts builds the mediator's signed receipts, verifies a list of them with the
// mediator's key alone, reads one without verifying it, recognises a question's receipts and its
// release, and reconciles presented receipts with what each party kept.
//
// It implements receipt-1 to receipt-10 and crossing-5: the receipt (receipt-1), its subject
// (receipt-2), its predicate (receipt-3, receipt-9), verdicts and reasons (receipt-4), chains
// (receipt-5), the verifier (receipt-6, section 8's table), reconciliation (receipt-7, section 9's
// table), reading without verifying (receipt-8), sequence numbers and times (receipt-10), and the
// echoed envelope (crossing-5). Releases is the one function that recognises a question's receipts
// and its release (release-in-resolve).
package receipts

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// The fixed values of a receipt and its statement (receipt-1).
const (
	PayloadType   = "application/vnd.in-toto+json"
	StatementType = "https://in-toto.io/Statement/v1"
	PredicateType = "https://placket.dev/receipt/0.1"
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

// The subject names of an answer and of a changed version (maybe-8, maybe-9).
const (
	NameAnswer  = "answer"
	NameChanged = "changed-version"
)

// Code is a code of the verifier (section 8) or of reconciliation (section 9), or a misuse of Build.
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
	CodePresented     Code = "presented"
	CodeKept          Code = "kept"
	// CodeStatement is not a code of the protocol: Build was given a statement no verifier would
	// accept.
	CodeStatement Code = "statement"
)

// Error is a failed verification or reconciliation: its code, the index of the receipt it failed at,
// and for kept the party's reference. Index is -1 for reference and for a misuse of Build.
type Error struct {
	Code  Code
	Index int
	Actor string
}

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
}

// A Statement is a receipt's statement as read from its payload, and the receipt's id, the typed hash
// of its canonical bytes (receipt-1).
type Statement struct {
	ID        string
	Subject   Subject
	Predicate Predicate
}

// Verified is a receipt's statement that passed Verify, or one this package built and signed. Only
// this package makes one, so a caller holding a Verified holds a statement that was checked
// (verified-by-type; docs/adr/0016-verified-receipts.md).
type Verified struct {
	s Statement
}

// Statement returns a copy of the verified statement (docs/adr/0016-verified-receipts.md).
func (v Verified) Statement() Statement { return v.s }

// Digest returns the salted digest of a part: the lowercase hex SHA-256 of its salt's bytes followed
// by its body's bytes (crossing-3). The salt comes first, in the order of crossing-3's formula: an
// exception to the data-first order of inputs-as-parameters, recorded in
// docs/adr/0016-verified-receipts.md.
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
		"verdict":  p.Verdict, "reason": p.Reason,
	}
	if p.Prev != "" {
		o["prev"] = p.Prev
	}
	return o
}

// Build writes the receipt of subject and predicate, signed by the mediator's key, and returns it and
// its statement, whose ID is the receipt's id (receipt-1; docs/adr/0016-verified-receipts.md). The
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

// open reads one receipt through rows 1 to 3 of section 8 under the mediator's reference.
func open(receipt []byte, mediator string) ([]byte, Code) {
	v, _, err := shape.Decode(receipt)
	if err != nil {
		return nil, CodeNotCanonical
	}
	o, _ := v.(shape.Object)
	var payload []byte
	if s, ok := o["payload"].(string); ok {
		if b, ok := shape.Base64(s); ok {
			if _, err := canonical.Form(b); err != nil {
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
	if !ok || !shape.Only(p, "accord", "card", "kind", "from", "to", "at", "envelope", "verdict", "reason", "",
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
	resolutionOnly := reason == ReasonDeclined || reason == ReasonCancelled || reason == ReasonTimeout || reason == ReasonUndeliverable
	if !resolutionOnly && reason != composition.ReasonOK && reason != composition.ReasonNoMatch &&
		reason != composition.ReasonMaybe && !checkReason(reason) {
		return false
	}
	if kind == KindCrossing && resolutionOnly {
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

// checkReason reports whether r is check: and a label, the reserved reason of a failing check
// (receipt-4), read for shape only.
func checkReason(r string) bool {
	name, ok := cutPrefix(r, "check:")
	return ok && shape.Label(name)
}

func cutPrefix(s, p string) (string, bool) {
	if len(s) >= len(p) && s[:len(p)] == p {
		return s[len(p):], true
	}
	return "", false
}

// Decode reads a receipt's statement from its payload without verifying anything: not its
// signature, which mediator it names, its place in its chain or its place among questions
// (receipt-8). It does hold every member to its form, as the verifier's rows 4 to 6 read it alone: a
// mediator that is not an actor reference is refused with mediator, and a seq or prev of the wrong
// form with chain, so nothing of an unverified receipt reaches its reader unread. Whatever presents
// what it returns must say that the receipt was not verified.
func Decode(receipt []byte) (Statement, error) {
	v, _, err := shape.Decode(receipt)
	if err != nil {
		return Statement{}, &Error{Code: CodeNotCanonical, Index: 0}
	}
	o, _ := v.(shape.Object)
	s, _ := o["payload"].(string)
	payload, ok := shape.Base64(s)
	if !ok {
		return Statement{}, &Error{Code: CodeSignature, Index: 0}
	}
	if _, err := canonical.Form(payload); err != nil {
		return Statement{}, &Error{Code: CodeNotCanonical, Index: 0}
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
// that it was not verified (receipt-8). A forged receipt shows exactly as a real one does.
func Show(receipt []byte) ([]string, error) {
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
