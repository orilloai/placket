package conformance

import (
	"encoding/json"
	"errors"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
)

func init() {
	registered["found"] = runFound
	registered["view"] = runView
}

// runFound founds the set at foundedAt with the vector's openings and acceptances, and compares the
// Accord id and card hash, with terms not-checked when a seal was not opened (setup-contract-16), or
// the code.
func runFound(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	set, ok := inputOf(m)
	if !ok {
		return bad("input", "the input does not decode")
	}
	in, ok := inputsOf(m, set)
	if !ok {
		return bad("openings", "an opening or the acceptances do not decode")
	}
	a, err := founding.Found(in, m.str("foundedAt"))
	var got []byte
	var se *setup.Error
	switch {
	case err == nil:
		o := map[string]any{"accord": a.ID(), "card": a.Card()}
		if !a.TermsChecked() {
			o["terms"] = "not-checked"
		}
		got = jsonOf(o)
	case errors.As(err, &se):
		got = jsonOf(map[string]any{"code": se.Code})
	default:
		got = jsonOf(map[string]any{"error": err.Error()})
	}
	return []Part{compare("expect", got, m["expect"])}
}

// runView runs rows 1 to 17 over the set at foundedAt with every opening, and builds the view of
// the member viewer names (setup-contract-14); it compares the view and its typed hash, or the code.
func runView(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	set, ok := inputOf(m)
	if !ok {
		return bad("input", "the input does not decode")
	}
	in, ok := inputsOf(m, set)
	if !ok {
		return bad("openings", "an opening does not decode")
	}
	var got []byte
	var se *setup.Error
	a, err := setup.Found(in.Set, in.Openings, m.str("foundedAt"))
	var view []byte
	if err == nil {
		view, err = founding.View(a, m.str("viewer"))
	}
	switch {
	case err == nil:
		h, _ := canonical.TypedHash(view)
		got = jsonOf(map[string]any{"view": json.RawMessage(view), "hash": h})
	case errors.As(err, &se):
		got = jsonOf(map[string]any{"code": se.Code})
	default:
		got = jsonOf(map[string]any{"error": err.Error()})
	}
	return []Part{compare("expect", got, m["expect"])}
}
