package conformance

import (
	"errors"

	"github.com/orilloai/placket/core/setup"
)

func init() { registered["found"] = runFound }

// runFound founds the set at foundedAt and compares the Accord id and card hash, or the code.
func runFound(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	in, ok := inputOf(m)
	if !ok {
		return bad("input", "the input does not decode")
	}
	a, err := setup.Found(in, m.str("foundedAt"))
	var got []byte
	var se *setup.Error
	switch {
	case err == nil:
		got = jsonOf(map[string]any{"accord": a.ID(), "card": a.Card()})
	case errors.As(err, &se):
		got = jsonOf(map[string]any{"code": se.Code})
	default:
		got = jsonOf(map[string]any{"error": err.Error()})
	}
	return []Part{compare("expect", got, m["expect"])}
}
