package receipts

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// ClosePredicateType is the predicate type of a close receipt (close-2). It ends in the protocol
// number, the card's spec (setup-contract-4), as each placket URI does.
const ClosePredicateType = "https://placket.tech/close/0.1"

// closeSubject is the name of a close's one subject: the Accord, by the digest of its founding set
// (close-2).
const closeSubject = "accord"

// A Close is what a close receipt states (close-2, record 0022): its id, the typed hash of its
// statement's canonical bytes; the Accord id, the card hash and the mediator's reference; its time;
// the last receipt of every chain of the list before it, ordered by pair; and each delivery not yet
// fetched at the close, in the order queued. A close is in no chain: it has no seq, no prev and no
// envelope.
type Close struct {
	ID, Accord, Card, Mediator, At string
	Chains                         []Final
	Unfetched                      []Unfetched
}

// A Final is the last receipt of one chain at a close: the pair's from and to, its seq and its id
// (close-2).
type Final struct {
	From, To string
	Seq      int64
	Receipt  string
}

// An Unfetched is a delivery not yet fetched at a close: the member it was queued for, and the id of
// the receipt that queued it (close-2).
type Unfetched struct {
	To, Receipt string
}

// copy returns c with lists of its own (results-are-copies).
func (c Close) copy() Close {
	c.Chains = append([]Final{}, c.Chains...)
	c.Unfetched = append([]Unfetched{}, c.Unfetched...)
	return c
}

// statement writes the close's statement (close-2): its one subject the Accord, by the hex of its
// id, and its predicate's six members, each list there even when empty.
func (c Close) statement() []byte {
	chains := []any{}
	for _, f := range c.Chains {
		chains = append(chains, shape.Object{"from": f.From, "to": f.To, "seq": f.Seq, "receipt": f.Receipt})
	}
	unfetched := []any{}
	for _, u := range c.Unfetched {
		unfetched = append(unfetched, shape.Object{"to": u.To, "receipt": u.Receipt})
	}
	return shape.Encode(shape.Object{
		"_type":         StatementType,
		"subject":       []any{shape.Object{"name": closeSubject, "digest": shape.Object{"sha256": strings.TrimPrefix(c.Accord, canonical.HashPrefix)}}},
		"predicateType": ClosePredicateType,
		"predicate": shape.Object{
			"accord": c.Accord, "card": c.Card, "mediator": c.Mediator, "at": c.At,
			"chains": chains, "unfetched": unfetched,
		},
	})
}

// BuildClose writes the close receipt of c, signed by the mediator's key, and returns it and its
// close, whose ID is the receipt's id (close-2, record 0022). The close's mediator is the key's
// reference, whatever it held, and its ID is ignored. A statement the verifier's row 4 would refuse
// is not written. Which chains and deliveries it lists is its caller's: the verifier's row 8 holds
// them to the list.
func BuildClose(c Close, key signing.Key) ([]byte, Verified, error) {
	c.Mediator = key.Reference()
	statement := c.statement()
	r, ok := readClose(statement)
	if !ok || !isReference(c.Mediator) {
		return nil, Verified{}, &Error{Code: CodeStatement, Index: -1}
	}
	sig, err := signing.SignPayload(PayloadType, statement, key)
	if err != nil {
		return nil, Verified{}, &Error{Code: CodeStatement, Index: -1}
	}
	receipt := shape.Encode(shape.Object{
		"payloadType": PayloadType,
		"payload":     base64.StdEncoding.EncodeToString(statement),
		"signatures":  []any{shape.Object{"keyid": c.Mediator, "sig": base64.StdEncoding.EncodeToString(sig)}},
	})
	closed := r.Close
	return receipt, Verified{c: &closed}, nil
}

// A closeReading is a close's statement read by row 4, with its predicate's mediator as found, for
// row 5.
type closeReading struct {
	Close
	mediator any
}

// isClose reports whether a statement's payload names the close's predicate type, so that it is
// read as a close and not as a receipt's statement. Bytes that do not decode are not a close.
func isClose(payload []byte) bool {
	v, _, err := shape.Decode(payload)
	if err != nil {
		return false
	}
	o, _ := v.(shape.Object)
	return o != nil && o["predicateType"] == ClosePredicateType
}

// readClose reads a close's statement by row 4 of section 8 (close-2): exactly its four members, with
// _type and predicateType its own; one subject, the Accord, whose sha256 digest is the hex of the
// predicate's accord; and a predicate of exactly accord, card, mediator, at, chains and unfetched,
// each of its form, but mediator, which row 5 reads. Each chain entry is exactly from, to, seq and
// receipt, and each unfetched entry exactly to and receipt. Which chains and deliveries it lists is
// row 8's.
func readClose(payload []byte) (closeReading, bool) {
	var out closeReading
	v, c, err := shape.Decode(payload)
	if err != nil {
		return out, false
	}
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "_type", "subject", "predicateType", "predicate") ||
		o["_type"] != StatementType || o["predicateType"] != ClosePredicateType {
		return out, false
	}
	subjects, ok := o["subject"].([]any)
	if !ok || len(subjects) != 1 {
		return out, false
	}
	sub, ok := subjects[0].(shape.Object)
	if !ok || !shape.Only(sub, "name", "digest") || sub["name"] != closeSubject {
		return out, false
	}
	dg, ok := sub["digest"].(shape.Object)
	if !ok || !shape.Only(dg, "sha256") {
		return out, false
	}
	digest, _ := dg["sha256"].(string)
	p, ok := o["predicate"].(shape.Object)
	if !ok || !shape.Only(p, "accord", "card", "at", "chains", "unfetched", "", "mediator") {
		return out, false
	}
	str := func(k string, valid func(string) bool) (string, bool) {
		s, ok := p[k].(string)
		return s, ok && valid(s)
	}
	var cl Close
	if cl.Accord, ok = str("accord", shape.Hash); !ok || digest != strings.TrimPrefix(cl.Accord, canonical.HashPrefix) {
		return out, false
	}
	if cl.Card, ok = str("card", shape.Hash); !ok {
		return out, false
	}
	if cl.At, ok = str("at", shape.Timestamp); !ok {
		return out, false
	}
	chains, ok := p["chains"].([]any)
	if !ok {
		return out, false
	}
	cl.Chains = []Final{}
	for _, x := range chains {
		e, ok := x.(shape.Object)
		if !ok || !shape.Only(e, "from", "to", "seq", "receipt") {
			return out, false
		}
		from, ok1 := e["from"].(string)
		to, ok2 := e["to"].(string)
		seq, ok3 := shape.Int(e["seq"])
		id, ok4 := e["receipt"].(string)
		if !ok1 || !ok2 || !ok3 || !ok4 || !isReference(from) || !isReference(to) || !shape.Hash(id) {
			return out, false
		}
		cl.Chains = append(cl.Chains, Final{From: from, To: to, Seq: seq, Receipt: id})
	}
	unfetched, ok := p["unfetched"].([]any)
	if !ok {
		return out, false
	}
	cl.Unfetched = []Unfetched{}
	for _, x := range unfetched {
		e, ok := x.(shape.Object)
		if !ok || !shape.Only(e, "to", "receipt") {
			return out, false
		}
		to, ok1 := e["to"].(string)
		id, ok2 := e["receipt"].(string)
		if !ok1 || !ok2 || !isReference(to) || !shape.Hash(id) {
			return out, false
		}
		cl.Unfetched = append(cl.Unfetched, Unfetched{To: to, Receipt: id})
	}
	out.mediator = p["mediator"]
	cl.Mediator, _ = p["mediator"].(string)
	cl.ID = typedHash(c)
	out.Close = cl
	return out, true
}

// DecodeClose reads a close receipt's statement from its payload without verifying anything: not its
// signature, which mediator it names, or whether its lists match any receipts (receipt-8, close-2).
// It holds every member to its form, as the verifier's rows 4 and 5 read it alone: a statement that
// is not a close's is refused with statement-type, and a mediator that is not an actor reference with
// mediator. Whatever presents what it returns must say that the receipt was not verified.
func DecodeClose(receipt []byte) (Close, error) {
	payload, err := payloadOf(receipt)
	if err != nil {
		return Close{}, err
	}
	r, ok := readClose(payload)
	if !ok {
		return Close{}, &Error{Code: CodeStatementType, Index: 0}
	}
	if s, ok := r.mediator.(string); !ok || !isReference(s) {
		return Close{}, &Error{Code: CodeMediator, Index: 0}
	}
	return r.Close, nil
}

// payloadOf reads a receipt's payload, as Decode and DecodeClose do, without verifying anything: the
// receipt's canonical bytes, and its payload in standard base64 in its one encoding, canonical.
func payloadOf(receipt []byte) ([]byte, error) {
	v, _, err := shape.Decode(receipt)
	if err != nil {
		return nil, &Error{Code: CodeNotCanonical, Index: 0}
	}
	o, _ := v.(shape.Object)
	s, _ := o["payload"].(string)
	payload, ok := shape.Base64(s)
	if !ok {
		return nil, &Error{Code: CodeSignature, Index: 0}
	}
	if _, err := canonical.Form(payload); err != nil {
		return nil, &Error{Code: CodeNotCanonical, Index: 0}
	}
	return payload, nil
}

// showClose is the words of a close (receipt-8, close-2): its id, Accord, card, mediator and time,
// then one line per chain and one per delivery not yet fetched.
func showClose(c Close) []string {
	lines := []string{
		NotVerified,
		"close " + c.ID,
		"accord " + c.Accord + ", card " + c.Card,
		"mediator " + c.Mediator + ", at " + c.At,
	}
	for _, f := range c.Chains {
		lines = append(lines, "chain from "+f.From+" to "+f.To+", seq "+strconv.FormatInt(f.Seq, 10)+", receipt "+f.Receipt)
	}
	for _, u := range c.Unfetched {
		lines = append(lines, "unfetched to "+u.To+", receipt "+u.Receipt)
	}
	return lines
}
