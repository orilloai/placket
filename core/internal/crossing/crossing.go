// Package crossing builds and checks the mediator's deliveries, and reads a part's body and salt,
// for crossing-2 and crossing-3: the body's bytes are the UTF-8 of text, the decoded bytes of raw or
// the canonical bytes of data; the salt is exactly 32 bytes in lowercase hex; and a delivery holds
// the receipt that passed a part and its id, and the part as it crossed. The mediator calls it and
// keeps only state (mediator-no-logic). It is written once, so the mediator reads a part's body the
// same way when it admits a give, when it shows a question and when it restores a delivery
// (docs/adr/0016-verified-receipts.md).
package crossing

import (
	"bytes"
	"encoding/hex"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
)

// SaltBytes is the length of every salt, in bytes (crossing-2, crossing-3).
const SaltBytes = 32

// Forms are the members a body may be given as in this version, in the order they are read
// (crossing-2). A url body is not a body of this version.
var Forms = []string{"text", "raw", "data"}

// Body returns the bytes of a body given as form, the value v: the UTF-8 of a text string, the
// decoded bytes of a raw string in standard base64 in its one encoding, or the canonical bytes of a
// data value that is not null (crossing-2). It reports false for any other form or value.
func Body(form string, v any) ([]byte, bool) {
	switch form {
	case "text":
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		return []byte(s), true
	case "raw":
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		return shape.Base64(s)
	case "data":
		if v == nil {
			return nil, false
		}
		return shape.Encode(v), true
	}
	return nil, false
}

// Salt returns the bytes of a salt written as s: exactly SaltBytes, in lowercase hex
// (crossing-2). It reports false for any other value.
func Salt(v any) ([]byte, bool) {
	s, ok := v.(string)
	if !ok || len(s) != 2*SaltBytes || !shape.LowerHex(s) {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return b, true
}

// Deliver writes a delivery (crossing-3): the receipt that passed the part and its id, and the
// part's members as it crossed. It returns nil when the receipt is not in canonical form.
func Deliver(part map[string]any, receipt []byte, id string) []byte {
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

// Held reads a delivery for the party to, given back to the mediator after a restart, and returns
// the index in st of the receipt that queued it: a delivery in canonical form with exactly a
// delivery's members (crossing-3), whose receipt is one of the list, a pass addressed to to,
// carried as that receipt's bytes, and whose kind, salt and body are the receipt's subject. byID
// maps each receipt's id to its index. It reads the receipts it is given; it decides nothing.
func Held(st []receipts.Statement, list [][]byte, byID map[string]int, to string, delivery []byte) (int, bool) {
	v, c, err := shape.Decode(delivery)
	if err != nil || !bytes.Equal(c, delivery) {
		return -1, false
	}
	o, ok := v.(shape.Object)
	if !ok {
		return -1, false
	}
	bodyName, bodies := "", 0
	for _, n := range Forms {
		if _, has := o[n]; has {
			bodyName = n
			bodies++
		}
	}
	if bodies != 1 || !shape.Only(o, "receiptId", "receipt", "messageId", "kind", "salt", bodyName, "", "re") {
		return -1, false
	}
	id, _ := o["receiptId"].(string)
	ri, ok := byID[id]
	if !ok {
		return -1, false
	}
	s := st[ri]
	if s.Predicate.Verdict != string(composition.Pass) || s.Predicate.To != to {
		return -1, false
	}
	kept, err := canonical.Form(list[ri])
	if err != nil || !bytes.Equal(shape.Encode(o["receipt"]), kept) {
		return -1, false
	}
	messageID, ok := o["messageId"].(string)
	if !ok || messageID == "" || o["kind"] != any(s.Subject.Name) {
		return -1, false
	}
	if re, has := o["re"]; has {
		if r, ok := re.(string); !ok || !shape.Hash(r) {
			return -1, false
		}
	}
	salt, ok := Salt(o["salt"])
	if !ok {
		return -1, false
	}
	body, ok := Body(bodyName, o[bodyName])
	if !ok || receipts.Digest(salt, body) != s.Subject.Digest {
		return -1, false
	}
	return ri, true
}
