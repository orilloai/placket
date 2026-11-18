// Command signatures writes the vectors of the signatures group: section 2 of the protocol, actor
// references, signing bytes, Ed25519 signatures and type URIs, and the receipt's payload signature.
// Whether each signature is valid is written here by hand from the text; the generator computes the
// bytes and signatures with the Go standard library alone (vectors/README.md).
//
// It imports no package of the core.
package main

import (
	"math/big"
	"strings"

	"github.com/orilloai/placket/vectors/signatures/kit"
)

// orderEight is the canonical encoding of a point of order 8 (signing-1).
const orderEight = "26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05"

// groupOrder is L, the order of the curve's prime subgroup (RFC 8032).
var groupOrder, _ = new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)

// plusL returns a signature whose S is replaced by S + L (signing-2).
func plusL(sig []byte) []byte {
	s := make([]byte, 32)
	for i := 0; i < 32; i++ {
		s[31-i] = sig[32+i] // little-endian to big-endian
	}
	n := new(big.Int).SetBytes(s)
	n.Add(n, groupOrder)
	b := n.FillBytes(make([]byte, 32))
	out := append([]byte(nil), sig[:32]...)
	for i := 0; i < 32; i++ {
		out = append(out, b[31-i])
	}
	return out
}

func declaration() kit.Obj {
	return kit.Declaration(kit.Proposer, kit.Term("want", "give", []string{"applier"}, []string{"note"}))
}

func binding(t string) kit.Obj {
	b := kit.Obj{
		"actor": kit.Proposer.Ref, "principal": kit.Principal.Ref, "roles": []any{"proposer"},
		"policy": kit.HashOf(kit.AskAndLog().Policy()), "mediator": kit.Mediator.Ref, "until": kit.Until,
	}
	return kit.Signed(b, t, kit.Principal)
}

func envelope(signer kit.Key) kit.Obj {
	m := kit.Canon(kit.Message("note-1", kit.HashOf(kit.AskAndLog().Build()), kit.Applier,
		kit.TextPart("note", kit.Salt("signatures note", 32), "A note.")))
	return kit.Envelope(1, m, signer)
}

// answerCall is the decider's answer call of the action on a question of the ask-and-log Accord
// (maybe-11), signed by signer.
func answerCall(action string, signer kit.Key) kit.Obj {
	digest := ""
	if action == "accepted" || action == "changed" {
		digest = kit.Sum(kit.FromHex(kit.Salt("signatures answer", 32)), []byte("accepted"))
	}
	return kit.AnswerCall(kit.HashOf(kit.AskAndLog().Build()), kit.HashOf("signatures: a question"), action, digest, true, signer)
}

// acceptance is a member's acceptance of its view of the ask-and-log Accord (setup-contract-15).
func acceptance(from kit.Key) kit.Obj {
	set := kit.AskAndLog().Build()
	return kit.Acceptance(kit.HashOf(set), kit.View(kit.Open(set), kit.HashOf(set), from.Ref), from)
}

func sign(name, tests string, input kit.Obj, t string, signer kit.Key) kit.Vector {
	return kit.Vector{Name: "signatures/" + name, Tests: tests, Operation: "sign", Members: kit.Obj{
		"keys": kit.Obj{signer.Name: signer.Seed}, "input": input, "typeUri": t, "signer": signer.Name,
		"expect": kit.Obj{
			"signingBytesHex": kit.HexOf(kit.SigningBytes(input, t)),
			"sig":             kit.HexOf(signer.Sign(kit.SigningBytes(input, t))),
		},
	}}
}

func verify(name, tests string, input any, t, reference string, valid bool) kit.Vector {
	m := kit.Obj{"typeUri": t, "reference": reference, "expect": kit.Obj{"valid": valid}}
	switch in := input.(type) {
	case string:
		m["inputText"] = in
	default:
		m["input"] = in
	}
	return kit.Vector{Name: "signatures/" + name, Tests: tests, Operation: "verify-signature", Members: m}
}

// receipt signs a crossing receipt of the ask-and-log Accord under the given payload type.
func receipt(payloadType string, mutate func(sig []byte) []byte) kit.Obj {
	set := kit.AskAndLog().Build()
	st := kit.Canon(kit.Statement("note", kit.Sum([]byte("a note")), kit.Pred{
		Accord: kit.HashOf(set), Card: kit.HashOf(set["card"]), Kind: "crossing",
		From: kit.Proposer.Ref, To: kit.Applier.Ref, Seq: 1, At: "2026-11-12T09:00:00Z",
		Sender: kit.Proposer.Ref, ESeq: 1, EHash: kit.HashOf("a message"), Verdict: "pass", Reason: "ok", By: kit.ByTerms,
	}, kit.Mediator))
	sig := kit.Mediator.Sign(kit.PAE(payloadType, st))
	if mutate != nil {
		sig = mutate(sig)
	}
	return kit.Obj{
		"payloadType": payloadType, "payload": kit.B64(st),
		"signatures": []any{kit.Obj{"keyid": kit.Mediator.Ref, "sig": kit.B64(sig)}},
	}
}

func verifyReceipt(name, tests string, r kit.Obj, expect kit.Obj) kit.Vector {
	return kit.Vector{Name: "signatures/" + name, Tests: tests, Operation: "verify", Members: kit.Obj{
		"reference": kit.Mediator.Ref, "receipts": []any{kit.Obj{"receipt": r}}, "expect": expect,
	}}
}

func main() {
	d := declaration()
	withSig := kit.Unsigned(d)
	withSig["sig"] = strings.Repeat("0", 128)
	ref := kit.Proposer.Ref
	hexPart := strings.TrimPrefix(ref, "ed25519:")

	smallOrder := kit.Unsigned(d)
	smallOrder["actor"] = "ed25519:" + orderEight
	smallOrder["sig"] = d["sig"]

	edited := kit.Copy(d).(kit.Obj)
	edited["terms"].([]any)[0].(kit.Obj)["stance"] = "need"

	upperSig := kit.Copy(d).(kit.Obj)
	upperSig["sig"] = strings.ToUpper(d["sig"].(string))

	malleated := kit.Copy(d).(kit.Obj)
	malleated["sig"] = kit.HexOf(plusL(kit.FromHex(d["sig"].(string))))

	spaced := strings.Replace(string(kit.Canon(d)), ",", " ,\n ", -1)

	vs := []kit.Vector{
		sign("sign-declaration", "signing-2: a declaration's signing bytes and its actor's signature under the declaration type URI",
			kit.Unsigned(d), kit.TypeDeclaration, kit.Proposer),
		sign("sign-binding", "signing-2: a binding's signing bytes and its principal's signature under the binding type URI",
			kit.Unsigned(binding(kit.TypeBinding)), kit.TypeBinding, kit.Principal),
		sign("sign-envelope", "signing-2: an envelope's signing bytes and its sender's signature under the envelope type URI",
			kit.Unsigned(envelope(kit.Proposer)), kit.TypeEnvelope, kit.Proposer),
		sign("sign-ignores-sig", "signing-2: a sig member present when an object is signed is ignored",
			withSig, kit.TypeDeclaration, kit.Proposer),

		verify("reference-forms", "signing-1: ed25519: and 64 lowercase hex names an actor, and its signature verifies",
			d, kit.TypeDeclaration, ref, true),
		verify("reference-uppercase", "signing-1: a reference in uppercase hex names no actor, so nothing verifies under it",
			d, kit.TypeDeclaration, "ed25519:"+strings.ToUpper(hexPart), false),
		verify("reference-short", "signing-1: a reference of 62 hex digits names no actor",
			d, kit.TypeDeclaration, "ed25519:"+hexPart[:62], false),
		verify("reference-other-prefix", "signing-1: a reference with the prefix ecdsa: names no actor",
			d, kit.TypeDeclaration, "ecdsa:"+hexPart, false),
		verify("small-order-key", "signing-1: a key that is a point of order 8 signs nothing",
			smallOrder, kit.TypeDeclaration, "ed25519:"+orderEight, false),
		verify("binding-verifies", "signing-2: a binding verifies under the binding type URI by its principal",
			binding(kit.TypeBinding), kit.TypeBinding, kit.Principal.Ref, true),
		verify("binding-under-declaration-uri", "signing-2: a binding signed under the declaration type URI fails under the binding's",
			binding(kit.TypeDeclaration), kit.TypeBinding, kit.Principal.Ref, false),
		verify("a-binding-under-the-old-domain", "signing-2, signing-4: a binding signed under the binding type URI on another host fails under the binding's",
			binding("https://placket.dev/binding/0.1"), kit.TypeBinding, kit.Principal.Ref, false),
		verify("envelope-verifies", "signing-2: an envelope verifies under the envelope type URI by its sender",
			envelope(kit.Proposer), kit.TypeEnvelope, kit.Proposer.Ref, true),
		verify("envelope-by-another-key", "signing-2: an envelope signed by another key fails under the caller's",
			envelope(kit.Applier), kit.TypeEnvelope, kit.Proposer.Ref, false),
		verify("malleated-signature", "signing-2: a valid signature whose S is replaced by S + L fails",
			malleated, kit.TypeDeclaration, ref, false),
		verify("edited-after-signing", "signing-2: a declaration edited after it was signed fails",
			edited, kit.TypeDeclaration, ref, false),
		verify("sig-uppercase", "signing-2: a sig in uppercase hex is not of the signature's form and fails",
			upperSig, kit.TypeDeclaration, ref, false),
		verify("sig-missing", "signing-2: an object with no sig fails",
			kit.Unsigned(d), kit.TypeDeclaration, ref, false),
		verify("canonical-form-is-signed", "signing-2: the signing bytes are over the canonical form, so whitespace in the input changes nothing",
			spaced, kit.TypeDeclaration, ref, true),
		verify("type-uri-of-the-operation", "signing-2: a declaration's signature fails under the envelope type URI",
			d, kit.TypeEnvelope, ref, false),

		sign("answer-sign-accepted", "maybe-11, signing-4: an accept's call, its digest and release, signed by its answerer under the answer type URI",
			kit.Unsigned(answerCall("accepted", kit.Decider)), kit.TypeAnswer, kit.Decider),
		sign("answer-sign-cancelled", "maybe-11, signing-4: a cancel's call, with no digest and no release, signed by its asker under the answer type URI",
			kit.Unsigned(answerCall("cancelled", kit.Proposer)), kit.TypeAnswer, kit.Proposer),
		verify("answer-verifies", "maybe-11, signing-2: an answer call verifies under the answer type URI by its from",
			answerCall("changed", kit.Decider), kit.TypeAnswer, kit.Decider.Ref, true),
		verify("answer-by-another-key", "maybe-11, signing-2: an answer call fails under a key other than the one that signed it",
			answerCall("declined", kit.Decider), kit.TypeAnswer, kit.Proposer.Ref, false),
		verify("answer-under-the-envelope-uri", "signing-2, signing-4: an answer call's signature fails under the envelope type URI",
			answerCall("accepted", kit.Decider), kit.TypeEnvelope, kit.Decider.Ref, false),
		verifyReceipt("receipt-answer-payload-type", "signing-4: a receipt under the answer type URI as its payload type is refused",
			receipt(kit.TypeAnswer, nil), kit.Obj{"code": "payload-type", "index": 0}),
		sign("acceptance-sign", "setup-contract-15, signing-4: a member's acceptance of its view, signed by the member under the acceptance type URI",
			kit.Unsigned(acceptance(kit.Applier)), kit.TypeAcceptance, kit.Applier),
		verify("acceptance-verifies", "setup-contract-15, signing-2: an acceptance verifies under the acceptance type URI by its from",
			acceptance(kit.Applier), kit.TypeAcceptance, kit.Applier.Ref, true),
		verify("acceptance-by-another-key", "setup-contract-15, signing-2: an acceptance fails under a key other than the one that signed it",
			acceptance(kit.Applier), kit.TypeAcceptance, kit.Proposer.Ref, false),
		verify("acceptance-under-the-declaration-uri", "signing-2, signing-4: an acceptance's signature fails under the declaration type URI",
			acceptance(kit.Applier), kit.TypeDeclaration, kit.Applier.Ref, false),
		verifyReceipt("receipt-acceptance-payload-type", "signing-4: a receipt under the acceptance type URI as its payload type is refused",
			receipt(kit.TypeAcceptance, nil), kit.Obj{"code": "payload-type", "index": 0}),

		verifyReceipt("receipt-verifies", "signing-4: a receipt's signature is a payload signature under application/vnd.in-toto+json",
			receipt(kit.PayloadType, nil), kit.Obj{"verified": 1}),
		verifyReceipt("receipt-malleated-signature", "signing-2: a receipt signature whose S is replaced by S + L fails signature",
			receipt(kit.PayloadType, plusL), kit.Obj{"code": "signature", "index": 0}),
		verifyReceipt("receipt-placket-payload-type", "signing-4: a receipt under a placket type URI as its payload type is refused",
			receipt(kit.TypeEnvelope, nil), kit.Obj{"code": "payload-type", "index": 0}),
	}
	kit.Main("signatures", vs)
}
