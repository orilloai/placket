package conformance

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	vreceipts "github.com/orilloai/placket/core/receipts"
)

func init() { registered["scenario"] = runScenario }

// runScenario founds one mediator and applies the steps in order, comparing each step's outcome,
// then the final receipts, deliveries and open questions, and verifies the final receipts under the
// mediator's reference (conformance-vectors-3).
func runScenario(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	keys, ok := keysOf(m)
	if !ok {
		return bad("keys", "a key does not decode")
	}
	key, ok := keys[m.str("mediator")]
	if !ok {
		return bad("mediator", "the mediator names no key")
	}
	set, err := canonical.Form(m["founding"])
	if err != nil {
		return bad("founding", "the founding set is not accepted by the canonical form")
	}
	med, err := mediator.Found(set, key, m.str("foundedAt"))
	if err != nil {
		return bad("founding", "the set does not found: "+err.Error())
	}
	ref := func(name string) string { return keys[name].Reference() }

	var steps []members
	if json.Unmarshal(m["steps"], &steps) != nil {
		return bad("steps", "the steps do not decode")
	}
	var parts []Part
	for i, s := range steps {
		name := "step-" + strconv.Itoa(i)
		switch s.str("op") {
		case "give":
			msg, ok1 := bytesOf(s, "message", "messageText", "messageHex")
			env, ok2 := bytesOf(s, "envelope", "envelopeText", "envelopeHex")
			if !ok1 || !ok2 {
				return append(parts, Part{Name: name, Failure: "the message or envelope does not decode"})
			}
			out, err := med.Give(ref(s.str("as")), msg, env, s.str("at"))
			parts = append(parts, compare(name, outcome(out, err), s["expect"]))
		case "get":
			d, err := med.Get(ref(s.str("as")))
			var got []byte
			var rej *mediator.Rejection
			switch {
			case errors.As(err, &rej):
				got = jsonOf(map[string]any{"rejection": rej.Code})
			case err != nil:
				got = jsonOf(map[string]any{"error": err.Error()})
			case d == nil:
				got = []byte(`{"none":true}`)
			default:
				got = append(append([]byte(`{"delivery":`), d...), '}')
			}
			parts = append(parts, compare(name, got, s["expect"]))
		default:
			return append(parts, Part{Name: name, Pending: "a " + s.str("op") + " step, which answers or times out a question"})
		}
	}

	var final members
	if json.Unmarshal(m["final"], &final) != nil {
		return append(parts, Part{Name: "final", Failure: "final does not decode"})
	}
	written := med.Receipts()
	var list []any
	for _, r := range written {
		id := receiptID(r)
		list = append(list, map[string]any{"receiptId": id, "receipt": json.RawMessage(r)})
	}
	if list == nil {
		list = []any{}
	}
	parts = append(parts, compare("final.receipts", jsonOf(list), final["receipts"]))

	delivered := map[string]any{}
	names := make([]string, 0, len(keys))
	for n := range keys {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		var q []any
		for {
			d, err := med.Get(ref(n))
			if err != nil || d == nil {
				break
			}
			q = append(q, json.RawMessage(d))
		}
		if q != nil {
			delivered[n] = q
		}
	}
	parts = append(parts, compare("final.delivered", jsonOf(delivered), final["delivered"]))
	pending := med.Pending()
	parts = append(parts, compare("final.pending", jsonOf(pending), final["pending"]))

	if _, err := vreceipts.Verify(key.Reference(), written); err != nil {
		parts = append(parts, Part{Name: "final.verified", Failure: "the receipts written do not verify: " + err.Error()})
	} else {
		parts = append(parts, Part{Name: "final.verified"})
	}
	return parts
}

// outcome writes a give's result as a vector states it.
func outcome(o mediator.Outcome, err error) []byte {
	var rej *mediator.Rejection
	switch {
	case errors.As(err, &rej):
		return jsonOf(map[string]any{"rejection": rej.Code})
	case err != nil:
		return jsonOf(map[string]any{"error": err.Error()})
	}
	return jsonOf(map[string]any{"verdict": o.Verdict, "reason": o.Reason, "receiptId": o.ReceiptID, "receipt": json.RawMessage(o.Receipt)})
}

// receiptID reads a receipt's id without verifying it.
func receiptID(r []byte) string {
	s, err := vreceipts.Decode(r)
	if err != nil {
		return ""
	}
	return s.ID
}
