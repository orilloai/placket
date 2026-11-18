// Package mediator holds one founded Accord's state and runs its operations: a give, judged at the
// door and decided by composition, whose receipt it writes and whose part it queues or holds as a
// question; a fetch of what was queued; an answer, the finding that nobody can answer, and the
// passing of time, which close a question and may release its part; and a reload of all of it from
// what a journal holds. It calls the capability packages for everything they decide, and holds their
// results.
//
// It holds state and operations for setup-contract-6 (membership at every operation),
// crossing-1 to crossing-4 and crossing-6 (the door's check table, in order), crossing-7 (a rejection
// changes nothing), crossing-3 (deliveries, queued per receiver and fetched once), maybe-1 (asking:
// the held part and its question), maybe-3 to maybe-10 (section 6's check table, the three answers,
// decline, cancel, undeliverable, timeout and release), maybe-11 and maybe-12 (the signed answer
// call, checked before anything is written, and carried by every delivery it queues), receipt-11
// (every receipt names its decider), crossing-5 (every closing and release echoes the question's
// envelope), receipt-5 (the chain of each pair) and receipt-10 (the same inputs at the same times
// write the same receipts), and close-1 to close-3 (closing the Accord: its open questions closed,
// the close receipt written last, and a closed Accord that takes no give or answer). Decision record
// 0006 says what it holds and why, record 0008 how a reload rebuilds it, and record 0022 how a close
// ends it.
package mediator

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/crossing"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// Extension is the crossing extension's URI (crossing-1). It ends in the protocol number, the
// card's spec (setup-contract-4), as each placket URI does.
const Extension = "https://placket.tech/crossing/0.1"

// The roles of a crossing message, with A2A 1.0's meaning (crossing-1): a member gives as an A2A
// client, ROLE_USER, and a part the mediator forms from an answer is the agent's, ROLE_AGENT. The
// door accepts only RoleUser from a member.
const (
	RoleAgent = "ROLE_AGENT"
	RoleUser  = "ROLE_USER"
)

// SaltBytes is the length of every salt, in bytes: a part's, and the salt of each part the mediator
// forms (crossing-2, crossing-3).
const SaltBytes = crossing.SaltBytes

// MinSaltBytes is the length of every salt, in bytes.
//
// Deprecated: use SaltBytes. A salt is exactly SaltBytes long; MinSaltBytes is removed at the
// release.
const MinSaltBytes = SaltBytes

// RejectionCode is a code a caller's operation is rejected with: what the caller did wrong, spelled
// as the door's check tables spell it (section 4).
type RejectionCode string

// The door's codes, in the order of its check table (section 4).
const (
	NotAMember        RejectionCode = "not-a-member"
	EnvelopeSignature RejectionCode = "envelope-signature"
	NotCanonical      RejectionCode = "not-canonical"
	EnvelopeHash      RejectionCode = "envelope-hash"
	EnvelopeSequence  RejectionCode = "envelope-sequence"
	ExtensionMissing  RejectionCode = "extension-missing"
	ReceiverUnknown   RejectionCode = "receiver-unknown"
	AccordMismatch    RejectionCode = "accord-mismatch"
	OnePart           RejectionCode = "one-part"
	OneBody           RejectionCode = "one-body"
	URLUnsupported    RejectionCode = "url-unsupported"
	PartMetadata      RejectionCode = "part-metadata"
)

// Section 6's codes for an answer, after not-a-member, in the order of its table (maybe-6, maybe-7,
// maybe-11).
const (
	// AnswerCall: the answer call does not read, names another Accord, or is not the caller's.
	AnswerCall     RejectionCode = RejectionCode(answer.CodeCall)
	NoOpenQuestion RejectionCode = "no-open-question"
	NotAnAnswerer  RejectionCode = "not-an-answerer"
	// AnswerBody: the answer's text and salt do not fit the call: for an accept or a change, a salt
	// that is not exactly SaltBytes, an empty version, or a text whose salted digest is not the
	// call's; for a decline or a cancel, any text or salt.
	AnswerBody RejectionCode = "answer-body"
)

// AccordClosed is the rejection of a member's give or answer once the Accord is closed, checked
// right after membership (close-3, sections 4 and 6).
const AccordClosed RejectionCode = "closed"

// Rejection is a rejected operation: the code of the first row that failed. A rejected operation
// changes nothing (crossing-7). It carries nothing of the input.
type Rejection struct{ Code RejectionCode }

// Error returns the package and the code, and nothing of the input.
func (r *Rejection) Error() string { return "mediator: rejected: " + string(r.Code) }

// ErrorCode says what the caller's program did wrong, or that the mediator failed
// (rejection-vs-error, record 0006).
type ErrorCode string

// The codes of Error. None is a code of the protocol (record 0006, record 0008).
const (
	// CodeAt: a time that does not keep the timestamp rule.
	CodeAt ErrorCode = "at"
	// CodeInternal: a receipt or delivery the mediator could not write. No input causes it.
	CodeInternal ErrorCode = "internal"
	// CodeUnfounded: an operation on a Mediator that neither Found nor Reload made.
	CodeUnfounded ErrorCode = "unfounded"
	// CodeJournal: a reloaded receipt that does not verify, or that names another Accord or card.
	CodeJournal ErrorCode = "journal"
	// CodeHeld: a reloaded delivery that no receipt of the list queued for its party.
	CodeHeld ErrorCode = "held"
	// CodeQuestion: a reloaded open entry that names no question the receipts leave open, or a
	// question they leave open that no entry names.
	CodeQuestion ErrorCode = "question"
	// CodeClosed: a Close of a record that already ends in its close (close-3).
	CodeClosed ErrorCode = "closed"
	// CodeClose: a Reload of a record cut inside a close, its closed closings with no close after
	// them, which only Close completes (close-1).
	CodeClose ErrorCode = "close"
)

// Error is a misuse of the mediator by its caller's program, a record a reload refuses, or an
// internal failure. Founding's refusals are setup's *Error, returned as they are. Index is the index
// of the receipt or entry at fault, for journal, held and question, and for at when the time is an
// open question's deadline; it is -1 otherwise. Err is the verifier's *Error a journal error wraps.
type Error struct {
	Code  ErrorCode
	Index int
	Err   error
}

// Error returns the package, the code and the index, and nothing of the input.
func (e *Error) Error() string {
	if e.Index < 0 {
		return "mediator: " + string(e.Code)
	}
	return "mediator: " + string(e.Code) + ", index " + strconv.Itoa(e.Index)
}

// Unwrap returns the verifier's error a journal error wraps, or nil (record 0008).
func (e *Error) Unwrap() error { return e.Err }

// The misuse codes of an answer's own inputs, from before the answer was a signed call.
//
// Deprecated: nothing returns them. A salt, a version or an action that does not fit is now the
// rejection AnswerBody or AnswerCall (docs/adr/0021-who-decided-and-the-answer-call.md). They are
// removed at the release.
const (
	CodeSalt   ErrorCode = "salt"
	CodeAnswer ErrorCode = "answer"
	CodeAction ErrorCode = "action"
)

// failure is an Error with no index.
func failure(code ErrorCode) *Error { return &Error{Code: code, Index: -1} }

// ErrOtherMediator is returned by Found for a set whose mediator.actor is not the key's reference:
// no set is wrong for that, so it is not a founding code (setup-contract-7).
var ErrOtherMediator = errors.New("mediator: the set names another mediator")

// An Outcome is a receipt the mediator wrote: its verdict, reason, id and bytes (receipt-1,
// record 0006).
type Outcome struct {
	Verdict   string
	Reason    string
	ReceiptID string
	Receipt   []byte
}

type link struct {
	seq int64
	id  string
}

// A question is a held part a maybe term asks about (maybe-1).
type question struct {
	id       string
	asker    string
	receiver string
	delivery map[string]any // the part as it was given, as its delivery would carry it
	envelope receipts.Envelope
	subject  receipts.Subject
	roles    []string // the role labels of the rule that may answer it
	deadline string
	receipt  []byte // the ask's receipt
}

// A queued delivery, and the id of the receipt that queued it.
type queued struct {
	id       string
	delivery []byte
}

// A Mediator is one founded Accord and its state. One goroutine owns it: its methods are not safe
// for concurrent use. It prints as a fixed text under every format verb, so neither its key nor a part
// it holds reaches a log by accident (secrets-never-print).
type Mediator struct {
	accord     *setup.Accord
	key        signing.Key
	receipts   [][]byte
	statements []receipts.Verified
	chains     map[[2]string]link
	highest    map[string]int64
	queues     map[string][]queued
	questions  []*question
	closed     *Closed
}

// founded reports whether Found or Reload made the mediator.
func (m *Mediator) founded() bool { return m != nil && m.accord != nil }

// Found founds the set at the founding time at, as the mediator whose key is key. A set founding
// refuses returns setup's *Error; a set that names another mediator returns ErrOtherMediator
// (setup-contract-7).
func Found(set []byte, key signing.Key, at string) (*Mediator, error) {
	a, err := setup.Found(set, at)
	if err != nil {
		return nil, err
	}
	if a.Mediator() != key.Reference() {
		return nil, ErrOtherMediator
	}
	return &Mediator{
		accord:  a,
		key:     key,
		chains:  map[[2]string]link{},
		highest: map[string]int64{},
		queues:  map[string][]queued{},
	}, nil
}

// Format prints the Mediator as a fixed text, whatever the verb.
func (Mediator) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("mediator.Mediator")) }

// GoString prints the Mediator as a fixed text.
func (Mediator) GoString() string { return "mediator.Mediator" }

// String prints the Mediator as a fixed text.
func (Mediator) String() string { return "mediator.Mediator" }

// Accord returns the Accord id (setup-contract-5), or the empty string for a Mediator no one founded.
func (m *Mediator) Accord() string {
	if !m.founded() {
		return ""
	}
	return m.accord.ID()
}

// Card returns the card hash (setup-contract-4), or the empty string for a Mediator no one founded.
func (m *Mediator) Card() string {
	if !m.founded() {
		return ""
	}
	return m.accord.Card()
}

// Receipts returns every receipt the mediator wrote, in order, as copies (results-are-copies,
// record 0006).
func (m *Mediator) Receipts() [][]byte {
	out := make([][]byte, len(m.receipts))
	for i, r := range m.receipts {
		out[i] = append([]byte(nil), r...)
	}
	return out
}

// Pending returns the ids of the open questions, in the order they were opened (maybe-1).
func (m *Mediator) Pending() []string {
	out := make([]string, 0, len(m.questions))
	for _, q := range m.questions {
		out = append(out, q.id)
	}
	return out
}

// Get returns the first delivery queued for caller and removes it from the queue, or nil when none
// is queued (crossing-3). A caller that is not a member is rejected with not-a-member
// (setup-contract-6). A fetch writes no receipt.
func (m *Mediator) Get(caller string) ([]byte, error) {
	if !m.founded() {
		return nil, failure(CodeUnfounded)
	}
	if !m.accord.Member(caller) {
		return nil, &Rejection{Code: NotAMember}
	}
	q := m.queues[caller]
	if len(q) == 0 {
		return nil, nil
	}
	d := q[0].delivery
	// The queue's array keeps no reference to a fetched delivery: the mediator holds no body or salt
	// of it once fetched (crossing-3).
	q[0] = queued{}
	if len(q) == 1 {
		delete(m.queues, caller)
	} else {
		m.queues[caller] = q[1:]
	}
	return d, nil
}

// A give, as the door admitted it.
type give struct {
	seq      int64
	hash     string
	to       string
	kind     string
	digest   string
	delivery map[string]any // the delivery's members but the receipt and its id
}

// Give gives the part the message carries, under the envelope, from caller at the time at. The door
// runs its rows in order (section 4, crossing-6) and a give that fails one is rejected and changes
// nothing (crossing-7). A give that passes is decided by composition, with the judgment judge, or
// none when it is nil (composition-7), and its receipt is written before any state changes, naming
// the terms or the judge as its decider (receipt-11): a pass queues the part for its receiver, an
// escalate holds it as a question (maybe-1), and a block keeps nothing. Running a judge is not the
// mediator's: judge is the judgment its caller settled on.
func (m *Mediator) Give(caller string, message, envelope []byte, judge *composition.Judgment, at string) (Outcome, error) {
	if !m.founded() {
		return Outcome{}, failure(CodeUnfounded)
	}
	if !shape.Timestamp(at) {
		return Outcome{}, failure(CodeAt)
	}
	// Right after membership: a closed Accord takes no give (close-3).
	if m.closed != nil && m.accord.Member(caller) {
		return Outcome{}, &Rejection{Code: AccordClosed}
	}
	g, code := m.admit(caller, message, envelope)
	if code != "" {
		return Outcome{}, &Rejection{Code: code}
	}
	var j *composition.Judgment
	if judge != nil {
		c := *judge
		j = &c
	}
	d := composition.Decide(m.accord, composition.Crossing{From: caller, To: g.to, Kind: g.kind, Judge: j})
	by := receipts.By{Decider: receipts.ByTerms}
	if d.Judge != "" {
		by = receipts.By{Decider: receipts.ByJudge, Judge: d.Judge}
	}
	pair := [2]string{caller, g.to}
	last := m.chains[pair]
	next := link{seq: last.seq + 1}
	pred := receipts.Predicate{
		Accord: m.accord.ID(), Card: m.accord.Card(), Kind: receipts.KindCrossing,
		From: caller, To: g.to, Seq: next.seq, Prev: last.id, At: at,
		Envelope: receipts.Envelope{Sender: caller, Seq: g.seq, Hash: g.hash},
		Verdict:  string(d.Verdict), Reason: d.Reason, By: by,
	}
	subject := receipts.Subject{Name: g.kind, Digest: g.digest}
	receipt, verified, err := receipts.Build(subject, pred, m.key)
	if err != nil {
		return Outcome{}, failure(CodeInternal)
	}
	id := verified.Statement().ID
	next.id = id
	pred.Mediator = m.key.Reference()
	var delivery []byte
	var held *question
	switch d.Verdict {
	case composition.Pass:
		if delivery = crossing.Deliver(g.delivery, receipt, id); delivery == nil {
			return Outcome{}, failure(CodeInternal)
		}
	case composition.Escalate:
		held = &question{
			id: id, asker: caller, receiver: g.to, delivery: g.delivery,
			envelope: pred.Envelope, subject: subject, roles: d.Rule.To,
			deadline: shape.AddSeconds(at, d.Rule.Timeout), receipt: append([]byte(nil), receipt...),
		}
	}

	// Every receipt and delivery is built; only now does the state change.
	m.receipts = append(m.receipts, receipt)
	m.statements = append(m.statements, verified)
	m.chains[pair] = next
	m.highest[caller] = g.seq
	if delivery != nil {
		m.queues[g.to] = append(m.queues[g.to], queued{id: id, delivery: delivery})
	}
	if held != nil {
		m.questions = append(m.questions, held)
	}
	return Outcome{Verdict: string(d.Verdict), Reason: d.Reason, ReceiptID: id, Receipt: append([]byte(nil), receipt...)}, nil
}

// admit runs the door's rows over a give, in order (section 4), and returns what they read, or the
// code of the first that failed. It reads the mediator's state: its members, its Accord id and each
// sender's highest accepted seq.
func (m *Mediator) admit(caller string, message, envelope []byte) (give, RejectionCode) {
	var g give
	// Row 1: the caller is a member (setup-contract-6).
	if !m.accord.Member(caller) {
		return g, NotAMember
	}
	// Row 2: the envelope's bytes, its members and its signature under the caller's key.
	ev, _, err := shape.Decode(envelope)
	if err != nil {
		return g, EnvelopeSignature
	}
	eo, ok := ev.(shape.Object)
	if !ok || !shape.Only(eo, "seq", "hash", "sig") || !signing.Verify(envelope, signing.TypeEnvelope, caller) {
		return g, EnvelopeSignature
	}
	// Row 3: the message's canonical bytes.
	mv, canon, err := shape.Decode(message)
	if err != nil {
		return g, NotCanonical
	}
	// Row 4: the envelope names those bytes (crossing-4).
	h, _ := canonical.TypedHash(canon)
	if eo["hash"] != any(h) {
		return g, EnvelopeHash
	}
	g.hash = h
	// Row 5: a fresh sequence number (crossing-4).
	seq, ok := shape.Int(eo["seq"])
	if !ok || seq <= m.highest[caller] {
		return g, EnvelopeSequence
	}
	g.seq = seq
	// Row 6: the message's frame (crossing-1).
	mo, ok := mv.(shape.Object)
	if !ok || !shape.Only(mo, "messageId", "role", "extensions", "metadata", "", "parts") {
		return g, ExtensionMissing
	}
	messageID, ok := mo["messageId"].(string)
	if !ok || messageID == "" || mo["role"] != RoleUser {
		return g, ExtensionMissing
	}
	exts, ok := mo["extensions"].([]any)
	if !ok || len(exts) != 1 || exts[0] != Extension {
		return g, ExtensionMissing
	}
	meta, ok := mo["metadata"].(shape.Object)
	if !ok || !shape.Only(meta, Extension) {
		return g, ExtensionMissing
	}
	x, ok := meta[Extension].(shape.Object)
	if !ok || !shape.Only(x, "", "accord", "to", "re", "topic", "gate") {
		return g, ExtensionMissing
	}
	var re string
	if v, has := x["re"]; has {
		if re, ok = v.(string); !ok || !shape.Hash(re) {
			return g, ExtensionMissing
		}
	}
	if v, has := x["topic"]; has {
		if s, ok := v.(string); !ok || s == "" {
			return g, ExtensionMissing
		}
	}
	if v, has := x["gate"]; has {
		if s, ok := v.(string); !ok || !shape.Gate(s) {
			return g, ExtensionMissing
		}
	}
	// Row 7: the receiver is a member.
	to, ok := x["to"].(string)
	if !ok || !m.accord.Member(to) {
		return g, ReceiverUnknown
	}
	g.to = to
	// Row 8: this Accord.
	if x["accord"] != any(m.accord.ID()) {
		return g, AccordMismatch
	}
	// Row 9: one part.
	parts, ok := mo["parts"].([]any)
	if !ok || len(parts) != 1 {
		return g, OnePart
	}
	// Row 10: one body (crossing-2).
	part, ok := parts[0].(shape.Object)
	if !ok {
		return g, OneBody
	}
	bodyName, bodies := "", 0
	for _, b := range append(append([]string(nil), crossing.Forms...), "url") {
		if _, has := part[b]; has {
			bodyName = b
			bodies++
		}
	}
	if bodies != 1 || !shape.Only(part, bodyName, "", "metadata", "mediaType") {
		return g, OneBody
	}
	mediaType, hasMediaType := part["mediaType"].(string)
	if _, has := part["mediaType"]; has && (!hasMediaType || !shape.MediaType(mediaType)) {
		return g, OneBody
	}
	if bodyName == "url" {
		// Row 11: a url body is not a body of this version, whatever its value.
		return g, URLUnsupported
	}
	body, ok := crossing.Body(bodyName, part[bodyName])
	if !ok {
		return g, OneBody
	}
	// Row 12: the part's metadata.
	pm, ok := part["metadata"].(shape.Object)
	if !ok || !shape.Only(pm, Extension) {
		return g, PartMetadata
	}
	pmx, ok := pm[Extension].(shape.Object)
	if !ok || !shape.Only(pmx, "kind", "salt") {
		return g, PartMetadata
	}
	kind, ok := pmx["kind"].(string)
	if !ok || !shape.Label(kind) {
		return g, PartMetadata
	}
	salt, ok := crossing.Salt(pmx["salt"])
	if !ok {
		return g, PartMetadata
	}
	saltHex, _ := pmx["salt"].(string)
	g.kind = kind
	g.digest = receipts.Digest(salt, body)
	g.delivery = map[string]any{"messageId": messageID, "kind": kind, "salt": saltHex, bodyName: part[bodyName]}
	if re != "" {
		g.delivery["re"] = re
	}
	if hasMediaType {
		g.delivery["mediaType"] = mediaType
	}
	return g, ""
}
