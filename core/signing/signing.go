// Package signing names actors by their Ed25519 keys and signs and verifies placket's objects and
// other payloads under DSSE 1.0's pre-authentication encoding.
//
// It implements signing-1 (actor references, and keys that sign nothing), signing-2 (signing bytes
// and cofactorless verification) and signing-4 (the five type URIs, and payload signatures that can
// never stand for a placket object's). It has no codes of the protocol: an operation that checks a
// signature returns its own code when Verify fails.
package signing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/orilloai/placket/core/internal/shape"
)

// The five type URIs of signing-4. Each ends in the protocol number, the card's spec
// (setup-contract-4); core/mediator's tests hold them to setup.Spec. TypeAnswer is the answer call's
// (maybe-11, docs/adr/0009-who-decided.md), and TypeAcceptance a member's
// acceptance of its view (setup-contract-15, docs/adr/0006-founding.md).
const (
	TypeDeclaration = "https://placket.tech/declaration/0.1"
	TypeBinding     = "https://placket.tech/binding/0.1"
	TypeEnvelope    = "https://placket.tech/envelope/0.1"
	TypeAnswer      = "https://placket.tech/answer/0.1"
	TypeAcceptance  = "https://placket.tech/acceptance/0.1"
)

// ReferencePrefix starts every actor reference (signing-1).
const ReferencePrefix = "ed25519:"

// Code says how a caller misused this package. None is a code of the protocol (record 0002).
type Code string

// The codes of Error.
const (
	// CodeSeed: a key's seed is not 32 bytes.
	CodeSeed Code = "seed"
	// CodeTypeURI: an object signed under a type URI this version does not sign objects under, or a
	// payload signed under one of the five type URIs of signing-4.
	CodeTypeURI Code = "type-uri"
	// CodeObject: the bytes to sign are not accepted by the canonical form, or are not an object.
	CodeObject Code = "object"
)

// Error is a misuse of this package by its caller (record 0002).
type Error struct{ Code Code }

// Error returns the package and the code, and nothing of the input.
func (e *Error) Error() string { return "signing: " + string(e.Code) }

// A Key is an actor's private key (signing-1, signing-2, record 0002). It
// prints as a fixed text under every format verb, so no key reaches a log by accident. It holds the
// key behind a function: fmt does not call Format on a value inside another struct's unexported
// field, and prints a function there as an address under every verb. A pointer to the key would not
// do: under a verb a pointer does not take, such as %s, fmt prints what the pointer points at.
type Key struct {
	private func() ed25519.PrivateKey
}

// bytes returns the key's private key, or nil for the zero Key.
func (k Key) bytes() ed25519.PrivateKey {
	if k.private == nil {
		return nil
	}
	return k.private()
}

// NewKey makes the key whose Ed25519 seed is seed, 32 bytes (RFC 8032; signing-1, record 0002).
func NewKey(seed []byte) (Key, error) {
	if len(seed) != ed25519.SeedSize {
		return Key{}, &Error{Code: CodeSeed}
	}
	private := ed25519.NewKeyFromSeed(seed)
	return Key{private: func() ed25519.PrivateKey { return private }}, nil
}

// Reference returns the key's actor reference (signing-1). The zero Key has none.
func (k Key) Reference() string {
	private := k.bytes()
	if len(private) != ed25519.PrivateKeySize {
		return ""
	}
	return ReferencePrefix + hex.EncodeToString(private.Public().(ed25519.PublicKey))
}

// Format prints the Key as a fixed text, whatever the verb (secrets-never-print, record 0002).
func (k Key) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("signing.Key")) }

// GoString prints the Key as a fixed text (secrets-never-print, record 0002).
func (k Key) GoString() string { return "signing.Key" }

// String prints the Key as a fixed text (secrets-never-print, record 0002).
func (k Key) String() string { return "signing.Key" }

// ParseReference reads ref as an actor reference: ed25519: and 64 lowercase hex digits (signing-1).
// It reads the form only; whether the key can sign is Verify's to find.
func ParseReference(ref string) (ed25519.PublicKey, bool) {
	h, ok := strings.CutPrefix(ref, ReferencePrefix)
	if !ok || len(h) != 64 || !shape.LowerHex(h) {
		return nil, false
	}
	b, _ := hex.DecodeString(h)
	return ed25519.PublicKey(b), true
}

// PAE is DSSE 1.0's pre-authentication encoding of a payload type and a payload: DSSEv1, the type's
// length, the type, the payload's length and the payload, separated by spaces (signing-2).
func PAE(payloadType string, payload []byte) []byte {
	var b bytes.Buffer
	b.WriteString("DSSEv1 ")
	b.WriteString(strconv.Itoa(len(payloadType)))
	b.WriteByte(' ')
	b.WriteString(payloadType)
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(len(payload)))
	b.WriteByte(' ')
	b.Write(payload)
	return b.Bytes()
}

// objectType reports whether t is one of the five type URIs of signing-4, under which this version
// signs and verifies objects: the declaration, the binding, the envelope, the answer call and the
// acceptance.
func objectType(t string) bool {
	return t == TypeDeclaration || t == TypeBinding || t == TypeEnvelope || t == TypeAnswer || t == TypeAcceptance
}

// SigningBytes returns the signing bytes of object under typeURI: the pre-authentication encoding of
// the type URI and the canonical bytes of the object with its sig member removed (signing-2).
func SigningBytes(object []byte, typeURI string) ([]byte, error) {
	if !objectType(typeURI) {
		return nil, &Error{Code: CodeTypeURI}
	}
	b, _, err := unsigned(object)
	if err != nil {
		return nil, err
	}
	return PAE(typeURI, b), nil
}

// unsigned returns the canonical bytes of object without its sig member, and the sig member.
func unsigned(object []byte) ([]byte, any, error) {
	v, _, err := shape.Decode(object)
	if err != nil {
		return nil, nil, &Error{Code: CodeObject}
	}
	o, ok := v.(shape.Object)
	if !ok {
		return nil, nil, &Error{Code: CodeObject}
	}
	sig, has := o["sig"]
	delete(o, "sig")
	if !has {
		sig = nil
	}
	return shape.Encode(o), sig, nil
}

// Sign signs object under typeURI with key and returns the signature as 128 lowercase hex digits,
// the value of the object's sig member (signing-2). A sig member already in the object is ignored.
func Sign(object []byte, typeURI string, key Key) (string, error) {
	if len(key.bytes()) != ed25519.PrivateKeySize {
		return "", &Error{Code: CodeSeed}
	}
	b, err := SigningBytes(object, typeURI)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(ed25519.Sign(key.bytes(), b)), nil
}

// Verify reports whether object's sig verifies under typeURI by the actor signer, by the rows of
// section 2: signer is an actor reference, its key can sign, sig is 128 lowercase hex, and the
// equation holds over the signing bytes. Bytes the canonical form refuses, and anything but an
// object, verify nothing.
func Verify(object []byte, typeURI string, signer string) bool {
	public, ok := ParseReference(signer)
	if !ok || !keySigns(public) || !objectType(typeURI) {
		return false
	}
	b, sig, err := unsigned(object)
	if err != nil {
		return false
	}
	s, ok := sig.(string)
	if !ok || len(s) != 128 || !shape.LowerHex(s) {
		return false
	}
	raw, _ := hex.DecodeString(s)
	return equation(public, PAE(typeURI, b), raw)
}

// SignPayload signs payload under its payload type with key and returns the 64-byte signature
// (signing-4). A payload type of signing-4's five is refused: no payload signature can be taken
// for a placket object's.
func SignPayload(payloadType string, payload []byte, key Key) ([]byte, error) {
	if objectType(payloadType) {
		return nil, &Error{Code: CodeTypeURI}
	}
	if len(key.bytes()) != ed25519.PrivateKeySize {
		return nil, &Error{Code: CodeSeed}
	}
	return ed25519.Sign(key.bytes(), PAE(payloadType, payload)), nil
}

// VerifyPayload reports whether sig, 64 bytes, verifies over payload under its payload type by the
// actor signer (signing-2, signing-4). A payload type of signing-4's five verifies nothing.
func VerifyPayload(payloadType string, payload, sig []byte, signer string) bool {
	public, ok := ParseReference(signer)
	if !ok || !keySigns(public) || objectType(payloadType) {
		return false
	}
	return equation(public, PAE(payloadType, payload), sig)
}

// equation checks an Ed25519 signature without a cofactor: S below the group order, and R's bytes
// equal to the encoding of [S]B - [k]A (RFC 8032, signing-2).
func equation(public ed25519.PublicKey, message, sig []byte) bool {
	if len(public) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(public, message, sig)
}
