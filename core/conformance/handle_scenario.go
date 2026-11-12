package conformance

import (
	"encoding/hex"
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
		case "resolve":
			parts = append(parts, compare(name, resolveStep(med, s, ref), s["expect"]))
		case "tick":
			outs, err := med.Tick(s.str("at"))
			var got []byte
			if err != nil {
				got = outcome(mediator.Outcome{}, err)
			} else {
				list := []any{}
				for _, o := range outs {
					list = append(list, json.RawMessage(outcome(o, nil)))
				}
				got = jsonOf(map[string]any{"receipts": list})
			}
			parts = append(parts, compare(name, got, s["expect"]))
		default:
			return append(parts, Part{Name: name, Pending: "a " + s.str("op") + " step, which the core cannot run"})
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

// outcome writes a receipt's outcome, or a rejection, as a vector states it.
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

// resolveStep runs a resolve step (conformance-vectors-6): an answer by the member named in by, or by
// the question's first answerer when by is absent, or the mediator's finding that nobody can answer.
// It writes the resolution receipt's outcome as a vector states it, with the release receipt's
// outcome under release when one was written, or the rejection.
func resolveStep(med *mediator.Mediator, s members, ref func(string) string) []byte {
	question, at := s.str("question"), s.str("at")
	if s.str("action") == "undeliverable" {
		return outcome(med.Undeliverable(question, at))
	}
	var by string
	if s.has("by") {
		by = ref(s.str("by"))
	} else if l, ok := med.Answerers(question); ok && len(l) > 0 {
		by = l[0]
	}
	var content struct {
		Answer string `json:"answer"`
	}
	_ = json.Unmarshal(s["content"], &content)
	salt, _ := hex.DecodeString(s.str("salt"))
	var release bool
	_ = json.Unmarshal(s["release"], &release)
	outs, err := med.Resolve(by, question, mediator.Resolution{
		Action: mediator.Action(s.str("action")), Answer: content.Answer, Salt: salt, Release: release,
	}, at)
	if err != nil || len(outs) == 0 {
		return outcome(mediator.Outcome{}, err)
	}
	o := map[string]any{"verdict": outs[0].Verdict, "reason": outs[0].Reason, "receiptId": outs[0].ReceiptID, "receipt": json.RawMessage(outs[0].Receipt)}
	if len(outs) > 1 {
		o["release"] = json.RawMessage(outcome(outs[1], nil))
	}
	return jsonOf(o)
}

// receiptID reads a receipt's id without verifying it.
func receiptID(r []byte) string {
	s, err := vreceipts.Decode(r)
	if err != nil {
		return ""
	}
	return s.ID
}
