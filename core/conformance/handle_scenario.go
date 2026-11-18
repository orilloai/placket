package conformance

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/mediator"
	vreceipts "github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

func init() { registered["scenario"] = runScenario }

// runScenario founds one mediator and applies the steps in order, comparing each step's outcome,
// then the final receipts, deliveries and open questions, and verifies the final receipts under the
// mediator's reference (conformance-vectors-3). A close step closes the Accord and the steps after
// it run on the closed mediator.
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
	in, ok := inputsOf(m, set)
	if !ok {
		return bad("openings", "an opening or the acceptances do not decode")
	}
	med, err := mediator.Found(in, key, m.str("foundedAt"))
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
			var judge *composition.Judgment
			if s.has("judge") {
				var j struct{ Name, Verdict string }
				if json.Unmarshal(s["judge"], &j) != nil {
					return append(parts, Part{Name: name, Failure: "the judge does not decode"})
				}
				judge = &composition.Judgment{Name: j.Name, Verdict: composition.Verdict(j.Verdict)}
			}
			out, err := med.Give(ref(s.str("as")), msg, env, judge, s.str("at"))
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
				// Once the Accord is closed, a fetch that finds nothing queued returns the close (close-3).
				if c, ok := med.Closed(); ok {
					got = jsonOf(map[string]any{"close": closeOf(c)})
				}
			default:
				got = append(append([]byte(`{"delivery":`), d...), '}')
			}
			parts = append(parts, compare(name, got, s["expect"]))
		case "resolve":
			if why := pairing(s); why != "" {
				return append(parts, Part{Name: name, Failure: "a malformed vector: " + why})
			}
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
		case "close":
			closed, got := closeStep(med, in, key, m.str("foundedAt"), s.str("at"))
			if closed != nil {
				med = closed
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

// closeStep runs a close step (conformance-vectors-3, close-1): it closes the Accord from the record
// the running mediator holds, as a journal would keep it: every receipt in order, each open question
// with its deadline and roles as the mediator's own reference reads them, and each delivery not yet
// fetched, in the order of the receipts that queued it. It returns the closed mediator, or nil, and
// the closings' outcomes and the close receipt as a vector states them, or the error.
func closeStep(med *mediator.Mediator, in founding.Inputs, key signing.Key, foundedAt, at string) (*mediator.Mediator, []byte) {
	list := med.Receipts()
	var open []mediator.Open
	for _, id := range med.Pending() {
		q, ok, err := med.Question(key.Reference(), id)
		if err != nil || !ok {
			return nil, jsonOf(map[string]any{"error": "the open question " + id + " does not read"})
		}
		open = append(open, mediator.Open{Question: q.ID, Deadline: q.Deadline, Roles: q.Roles})
	}
	var deliveries []mediator.Queued
	for _, r := range list {
		if q, ok := med.Queued(receiptID(r)); ok {
			deliveries = append(deliveries, q)
		}
	}
	closed, outs, c, err := mediator.Close(in, list, open, deliveries, foundedAt, key, at)
	if err != nil {
		return nil, outcome(mediator.Outcome{}, err)
	}
	receipts := []any{}
	for _, o := range outs {
		receipts = append(receipts, json.RawMessage(outcome(o, nil)))
	}
	return closed, jsonOf(map[string]any{"receipts": receipts, "close": closeOf(c)})
}

// closeOf writes a close receipt and its id as a vector states it (close-2).
func closeOf(c mediator.Closed) map[string]any {
	return map[string]any{"receiptId": c.ID, "receipt": json.RawMessage(c.Receipt)}
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

// resolveStep runs a resolve step (conformance-vectors-6): an answer step, the signed call given as
// callHex with the answer's text and salt, by the member named in by, or by the call's from when by
// is absent; or the mediator's finding that nobody can answer the question, which takes no caller. It
// writes the resolution receipt's outcome as a vector states it, with the release receipt's outcome
// under release when one was written, or the rejection.
func resolveStep(med *mediator.Mediator, s members, ref func(string) string) []byte {
	at := s.str("at")
	if s.has("undeliverable") {
		return outcome(med.Undeliverable(s.str("question"), at))
	}
	call, _ := hex.DecodeString(s.str("callHex"))
	var by string
	if s.has("by") {
		by = ref(s.str("by"))
	} else if c, err := answer.Read(call); err == nil {
		by = c.From
	}
	var salt []byte
	if s.has("salt") {
		salt, _ = hex.DecodeString(s.str("salt"))
	}
	outs, err := med.Resolve(by, mediator.Resolution{Call: call, Text: s.str("answer"), Salt: salt}, at)
	if err != nil || len(outs) == 0 {
		return outcome(mediator.Outcome{}, err)
	}
	o := map[string]any{"verdict": outs[0].Verdict, "reason": outs[0].Reason, "receiptId": outs[0].ReceiptID, "receipt": json.RawMessage(outs[0].Receipt)}
	if len(outs) > 1 {
		o["release"] = json.RawMessage(outcome(outs[1], nil))
	}
	return jsonOf(o)
}

// pairing checks an answer step's expected outcome against the action and the release of its call,
// which the schema cannot read inside callHex (conformance-vectors-6): an accept or a change expects
// no reason of a decline, a cancel, a timeout or an undeliverable; a decline expects block, declined;
// a cancel expects block, cancelled; and an expected release only when the call asks for release. A
// rejection, an undeliverable step and a call that does not read pair with anything. It returns why
// the step is malformed, or the empty string.
func pairing(s members) string {
	if !s.has("callHex") {
		return ""
	}
	b, err := hex.DecodeString(s.str("callHex"))
	if err != nil {
		return "callHex is not hex"
	}
	c, err := answer.Read(b)
	if err != nil {
		return ""
	}
	var e struct {
		Verdict, Reason string
		Release         json.RawMessage
		Rejection       string
	}
	if json.Unmarshal(s["expect"], &e) != nil || e.Rejection != "" {
		return ""
	}
	switch c.Action {
	case answer.Accepted, answer.Changed:
		switch e.Reason {
		case vreceipts.ReasonDeclined, vreceipts.ReasonCancelled, vreceipts.ReasonTimeout, vreceipts.ReasonUndeliverable:
			return "an " + string(c.Action) + " call expects the reason " + e.Reason
		}
	case answer.Declined:
		if e.Verdict != "block" || e.Reason != vreceipts.ReasonDeclined {
			return "a declined call expects " + e.Verdict + " " + e.Reason
		}
	case answer.Cancelled:
		if e.Verdict != "block" || e.Reason != vreceipts.ReasonCancelled {
			return "a cancelled call expects " + e.Verdict + " " + e.Reason
		}
	}
	if len(e.Release) > 0 && !c.Release {
		return "a release is expected of a call that does not ask for one"
	}
	return ""
}

// receiptID reads a receipt's id without verifying it, a close receipt's included (close-2).
func receiptID(r []byte) string {
	if s, err := vreceipts.Decode(r); err == nil {
		return s.ID
	}
	if c, err := vreceipts.DecodeClose(r); err == nil {
		return c.ID
	}
	return ""
}
