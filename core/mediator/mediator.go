// Package mediator holds one founded Accord's state and runs its operations: a give, judged at the
// door and decided by composition, whose receipt it writes and whose part it queues or holds as a
// question, and a fetch of what was queued. It calls the capability packages for everything they
// decide, and holds their results.
//
// It holds state and operations for setup-contract-6 (membership at every operation),
// crossing-1 to crossing-4 and crossing-6 (the door's check table, in order), crossing-7 (a rejection
// changes nothing), crossing-3 (deliveries, queued per receiver and fetched once), maybe-1 (asking:
// the held part and its question), receipt-5 (the chain of each pair) and receipt-10 (the same
// inputs at the same times write the same receipts). Decision record 0006 says what it holds and why.
package mediator

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// Extension is the crossing extension's URI (crossing-1).
const Extension = "https://placket.dev/crossing/0.1"

// The roles a crossing message may have (crossing-1).
const (
	RoleAgent = "ROLE_AGENT"
	RoleUser  = "ROLE_USER"
)

// MinSaltBytes is the shortest salt a part may have, in bytes (crossing-2).
const MinSaltBytes = 16

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

// Rejection is a rejected operation: the code of the first row that failed. A rejected operation
// changes nothing (crossing-7). It carries nothing of the input.
type Rejection struct{ Code RejectionCode }

func (r *Rejection) Error() string { return "mediator: rejected: " + string(r.Code) }

// ErrorCode says what the caller's program did wrong, or that the mediator failed.
type ErrorCode string

// The codes of Error. None is a code of the protocol.
const (
	// CodeAt: a time that does not keep the timestamp rule.
	CodeAt ErrorCode = "at"
	// CodeInternal: a receipt or delivery the mediator could not write. No input causes it.
	CodeInternal ErrorCode = "internal"
)

// Error is a misuse of the mediator by its caller's program, or an internal failure. Founding's
// refusals are setup's *Error, returned as they are.
type Error struct{ Code ErrorCode }

func (e *Error) Error() string { return "mediator: " + string(e.Code) }

// ErrOtherMediator is returned by Found for a set whose mediator.actor is not the key's reference:
// no set is wrong for that, so it is not a founding code (setup-contract-7).
var ErrOtherMediator = errors.New("mediator: the set names another mediator")

// An Outcome is a give's decision: its receipt's verdict, reason, id and bytes.
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
	rule     setup.Rule
	deadline string
}

// A Mediator is one founded Accord and its state. One goroutine owns it: its methods are not safe
// for concurrent use. It prints as a fixed text under every format verb, so neither its key nor a part
// it holds reaches a log by accident (secrets-never-print).
type Mediator struct {
	accord    *setup.Accord
	key       signing.Key
	receipts  [][]byte
	ids       []string
	chains    map[[2]string]link
	highest   map[string]int64
	queues    map[string][][]byte
	questions []*question
}

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
		queues:  map[string][][]byte{},
	}, nil
}

// Format prints the Mediator as a fixed text, whatever the verb.
func (Mediator) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("mediator.Mediator")) }

// GoString prints the Mediator as a fixed text.
func (Mediator) GoString() string { return "mediator.Mediator" }

// String prints the Mediator as a fixed text.
func (Mediator) String() string { return "mediator.Mediator" }

// Accord returns the Accord id (setup-contract-5).
func (m *Mediator) Accord() string { return m.accord.ID() }

// Card returns the card hash (setup-contract-4).
func (m *Mediator) Card() string { return m.accord.Card() }

// Receipts returns every receipt the mediator wrote, in order, as copies.
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
	if !m.accord.Member(caller) {
		return nil, &Rejection{Code: NotAMember}
	}
	q := m.queues[caller]
	if len(q) == 0 {
		return nil, nil
	}
	d := q[0]
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
// nothing (crossing-7). A give that passes is decided by composition, and its receipt is written
// before any state changes: a pass queues the part for its receiver, an escalate holds it as a
// question (maybe-1), and a block keeps nothing.
func (m *Mediator) Give(caller string, message, envelope []byte, at string) (Outcome, error) {
	if !shape.Timestamp(at) {
		return Outcome{}, &Error{Code: CodeAt}
	}
	g, code := m.admit(caller, message, envelope)
	if code != "" {
		return Outcome{}, &Rejection{Code: code}
	}
	d := composition.Decide(m.accord, composition.Crossing{From: caller, To: g.to, Kind: g.kind})
	pair := [2]string{caller, g.to}
	last := m.chains[pair]
	next := link{seq: last.seq + 1}
	pred := receipts.Predicate{
		Accord: m.accord.ID(), Card: m.accord.Card(), Kind: receipts.KindCrossing,
		From: caller, To: g.to, Seq: next.seq, Prev: last.id, At: at,
		Envelope: receipts.Envelope{Sender: caller, Seq: g.seq, Hash: g.hash},
		Verdict:  string(d.Verdict), Reason: d.Reason,
	}
	subject := receipts.Subject{Name: g.kind, Digest: g.digest}
	receipt, id, err := receipts.Build(subject, pred, m.key)
	if err != nil {
		return Outcome{}, &Error{Code: CodeInternal}
	}
	next.id = id
	var delivery []byte
	var held *question
	switch d.Verdict {
	case composition.Pass:
		if delivery = deliver(g.delivery, receipt, id); delivery == nil {
			return Outcome{}, &Error{Code: CodeInternal}
		}
	case composition.Escalate:
		held = &question{
			id: id, asker: caller, receiver: g.to, delivery: g.delivery,
			envelope: pred.Envelope, subject: subject, rule: d.Rule,
			deadline: shape.AddSeconds(at, d.Rule.Timeout),
		}
	}

	// Every receipt and delivery is built; only now does the state change.
	m.receipts = append(m.receipts, receipt)
	m.ids = append(m.ids, id)
	m.chains[pair] = next
	m.highest[caller] = g.seq
	if delivery != nil {
		m.queues[g.to] = append(m.queues[g.to], delivery)
	}
	if held != nil {
		m.questions = append(m.questions, held)
	}
	return Outcome{Verdict: string(d.Verdict), Reason: d.Reason, ReceiptID: id, Receipt: append([]byte(nil), receipt...)}, nil
}

// deliver writes a delivery (crossing-3): the receipt that passed the part and its id, and the
// part's members as it crossed.
func deliver(part map[string]any, receipt []byte, id string) []byte {
	r, _, err := shape.Decode(receipt)
	if err != nil {
		return nil
	}
	o := shape.Object{"receiptId": id, "receipt": r}
	for k, v := range part {
		o[k] = v
	}
	return shape.Encode(o)
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
	if !ok || messageID == "" || (mo["role"] != RoleAgent && mo["role"] != RoleUser) {
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
	for _, b := range []string{"text", "raw", "data", "url"} {
		if _, has := part[b]; has {
			bodyName = b
			bodies++
		}
	}
	if bodies != 1 || !shape.Only(part, bodyName, "", "metadata") {
		return g, OneBody
	}
	var body []byte
	switch bodyName {
	case "text":
		s, ok := part["text"].(string)
		if !ok {
			return g, OneBody
		}
		body = []byte(s)
	case "raw":
		s, ok := part["raw"].(string)
		if !ok {
			return g, OneBody
		}
		if body, ok = shape.Base64(s); !ok {
			return g, OneBody
		}
	case "data":
		if part["data"] == nil {
			return g, OneBody
		}
		body = shape.Encode(part["data"])
	case "url":
		// Row 11: a url body is not a body of this version, whatever its value.
		return g, URLUnsupported
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
	saltHex, ok := pmx["salt"].(string)
	if !ok || len(saltHex) < 2*MinSaltBytes || !shape.LowerHex(saltHex) {
		return g, PartMetadata
	}
	salt := make([]byte, len(saltHex)/2)
	for i := range salt {
		n, _ := strconv.ParseUint(saltHex[2*i:2*i+2], 16, 8)
		salt[i] = byte(n)
	}
	g.kind = kind
	g.digest = receipts.Digest(salt, body)
	g.delivery = map[string]any{"messageId": messageID, "kind": kind, "salt": saltHex, bodyName: part[bodyName]}
	if re != "" {
		g.delivery["re"] = re
	}
	return g, ""
}
