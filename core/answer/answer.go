// Package answer signs and reads the answer call: the one signed call by which a member accepts,
// changes, declines or cancels a question, and which the receipts that close and release the
// question name by its typed hash.
//
// It implements maybe-11 (the answer call, its members and its signature under the fourth object
// type of signing-4) and receipt-11's answer form, whose value is the call's typed hash
// (canonical-3). The answer's text and salt travel beside the call and are never inside it: the call
// holds the answer's salted digest (crossing-3). Decision record 0021 says why it is a package of
// its own.
package answer

import (
	"bytes"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// TypeURI is the answer call's type URI, signing-4's fourth (maybe-11). It is signing.TypeAnswer,
// written once (constants-once).
const TypeURI = signing.TypeAnswer

// Action is what an answer does to a question (section 6), spelled as the vector format spells it
// (conformance-vectors-6, maybe-11).
type Action string

// The four actions of an answer: an accept (maybe-3), a change (maybe-9), and a decline and a cancel
// (maybe-4, maybe-6).
const (
	Accepted  Action = "accepted"
	Changed   Action = "changed"
	Declined  Action = "declined"
	Cancelled Action = "cancelled"
)

// A Call is what an answer call says (maybe-11): the Accord's id, the question's id, the action, for
// an accept or a change the salted digest of the answer's text under the answerer's salt and whether
// it asks for release, and the signer's actor reference. It holds no text and no salt.
type Call struct {
	Accord, Question string
	Action           Action
	Digest           string
	Release          bool
	From             string
}

// Code is the code of a call that does not read (maybe-11).
type Code string

// CodeCall is the one code: the call's bytes, its shape or its signature do not hold. It is the
// spelling of section 6's row 2, answer-call.
const CodeCall Code = "answer-call"

// Error is a call that does not read: its code, and nothing of the call (error-text-no-content).
type Error struct{ Code Code }

// Error returns the package and the code, and nothing of the input.
func (e *Error) Error() string { return "answer: " + string(e.Code) }

// value is the call's members, sig aside.
func (c Call) value() shape.Object {
	o := shape.Object{"accord": c.Accord, "question": c.Question, "action": string(c.Action), "from": c.From}
	if c.Action == Accepted || c.Action == Changed {
		o["digest"] = c.Digest
		o["release"] = c.Release
	}
	return o
}

// Sign returns the canonical bytes of the call signed with key under TypeURI (maybe-11, signing-2).
// Its From is the key's reference, whatever it held. A call whose shape Read would refuse is not
// signed.
func Sign(c Call, key signing.Key) ([]byte, error) {
	c.From = key.Reference()
	o := c.value()
	unsigned := shape.Encode(o)
	if unsigned == nil {
		return nil, &Error{Code: CodeCall}
	}
	sig, err := signing.Sign(unsigned, TypeURI, key)
	if err != nil {
		return nil, &Error{Code: CodeCall}
	}
	o["sig"] = sig
	b := shape.Encode(o)
	if _, err := Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// Read reads an answer call (maybe-11, section 6's row 2): its bytes are canonical; it is an object
// with exactly accord, question, action, from and sig, and digest and release exactly for an accept
// or a change; accord and question are typed hashes, action one of the four, digest 64 lowercase hex,
// release a boolean, from an actor reference and sig 128 lowercase hex; and sig verifies under from
// and TypeURI (signing-2). Whether the call fits an Accord, a question or a caller is its reader's to
// check. Every failure is *Error, code answer-call.
func Read(b []byte) (Call, error) {
	fail := func() (Call, error) { return Call{}, &Error{Code: CodeCall} }
	v, c, err := shape.Decode(b)
	if err != nil || !bytes.Equal(c, b) {
		return fail()
	}
	o, ok := v.(shape.Object)
	if !ok {
		return fail()
	}
	var call Call
	action, _ := o["action"].(string)
	call.Action = Action(action)
	switch call.Action {
	case Accepted, Changed:
		if !shape.Only(o, "accord", "question", "action", "from", "sig", "digest", "release") {
			return fail()
		}
		d, ok := o["digest"].(string)
		if !ok || len(d) != 64 || !shape.LowerHex(d) {
			return fail()
		}
		r, ok := o["release"].(bool)
		if !ok {
			return fail()
		}
		call.Digest, call.Release = d, r
	case Declined, Cancelled:
		if !shape.Only(o, "accord", "question", "action", "from", "sig") {
			return fail()
		}
	default:
		return fail()
	}
	call.Accord, _ = o["accord"].(string)
	call.Question, _ = o["question"].(string)
	call.From, _ = o["from"].(string)
	if !shape.Hash(call.Accord) || !shape.Hash(call.Question) {
		return fail()
	}
	if _, ok := signing.ParseReference(call.From); !ok || !signing.Verify(b, TypeURI, call.From) {
		return fail()
	}
	return call, nil
}

// ID returns the call's typed hash, the value of a receipt's by.answer (receipt-11, canonical-3): the
// typed hash of the call's canonical bytes, sig included. Bytes the canonical form refuses have none,
// and return the empty string.
func ID(b []byte) string {
	h, err := canonical.TypedHash(b)
	if err != nil {
		return ""
	}
	return h
}
