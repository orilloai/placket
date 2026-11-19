package conformance

import (
	"encoding/json"
	"errors"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/conform"
	vreceipts "github.com/orilloai/placket/core/receipts"
)

func init() { registered["conform"] = runConform }

// runConform checks the receipts for conformance to the Accord (verify-1, verify-2, verify-3) with the
// founding set, the answer calls, the subset and the kernel hash the vector trusts, then the body of
// its delivery when it gives one, and compares the first failure's code and index, or the counts of
// the answers witnessed and unwitnessed, the calls matching no receipt, whether the module was
// trusted and, with a view, the number of receipts read against it (verify-2).
func runConform(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	list, ok := receiptList(m["receipts"])
	if !ok {
		return bad("receipts", "a receipt does not decode")
	}
	e := conform.Evidence{Accord: m.str("accord"), Mediator: m.str("reference"), Receipts: list, Kernel: m.str("kernel")}
	if m.has("founding") {
		set, err := canonical.Form(m["founding"])
		if err != nil {
			return bad("founding", "the founding set is not accepted by the canonical form")
		}
		e.Founding = set
		in, ok := inputsOf(m, set)
		if !ok {
			return bad("openings", "an opening or the acceptances do not decode")
		}
		// A verifier holds the openings it was given: some members' or every member's
		// (setup-contract-16).
		e.Openings, e.Acceptances = in.Openings, in.Acceptances
		e.Openings.Some = e.Openings.ByActor != nil
	}
	if m.has("answers") {
		var items []members
		if json.Unmarshal(m["answers"], &items) != nil {
			return bad("answers", "the answers do not decode")
		}
		for _, it := range items {
			b, ok := bytesOf(it, "call", "callText", "callHex")
			if !ok {
				return bad("answers", "an answer call does not decode")
			}
			e.Answers = append(e.Answers, b)
		}
	}
	if m.has("subset") {
		_ = json.Unmarshal(m["subset"], &e.Subset)
	}
	if m.has("view") {
		// A member's view, its bytes its canonical form (verify-1 row 0a).
		v, err := canonical.Form(m["view"])
		if err != nil {
			return bad("view", "the view is not accepted by the canonical form")
		}
		e.View = v
	}
	r, err := conform.Check(e)
	if err == nil && m.has("delivery") {
		d, ferr := canonical.Form(m["delivery"])
		if ferr != nil {
			return bad("delivery", "the delivery is not accepted by the canonical form")
		}
		err = conform.Body(r, d)
	}
	var got []byte
	var ce *conform.Error
	var re *vreceipts.Error
	switch {
	case errors.As(err, &ce):
		got = jsonOf(map[string]any{"code": ce.Code, "index": ce.Index})
	case errors.As(err, &re):
		got = jsonOf(map[string]any{"code": re.Code, "index": re.Index})
	case err != nil:
		got = jsonOf(map[string]any{"error": err.Error()})
	default:
		o := map[string]any{"conforms": true, "module": r.Module, "unmatched": r.Unmatched}
		if len(e.Founding) > 0 && !r.Terms {
			o["terms"] = "not-checked"
		}
		counts := map[conform.Witness]int{}
		for _, d := range r.Decisions {
			counts[d.Witness]++
		}
		o["witnessed"], o["unwitnessed"] = counts[conform.Witnessed], counts[conform.Unwitnessed]
		if m.has("delivery") {
			o["body"] = true
		}
		if m.has("view") {
			o["viewed"] = len(r.Viewed)
		}
		got = jsonOf(o)
	}
	return []Part{compare("expect", got, m["expect"])}
}
