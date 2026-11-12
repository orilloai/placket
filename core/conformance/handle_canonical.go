package conformance

import (
	"encoding/hex"
	"errors"

	"github.com/orilloai/placket/core/canonical"
)

func init() { registered["canonical"] = runCanonical }

// runCanonical puts the input in canonical form and compares canonicalHex and hash, or code and
// offset (conformance-vectors-4).
func runCanonical(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	in, ok := inputOf(m)
	if !ok {
		return bad("input", "the input does not decode")
	}
	out, err := canonical.Form(in)
	var got []byte
	var ce *canonical.Error
	switch {
	case err == nil:
		h, _ := canonical.TypedHash(out)
		got = jsonOf(map[string]any{"canonicalHex": hex.EncodeToString(out), "hash": h})
	case errors.As(err, &ce):
		got = jsonOf(map[string]any{"code": ce.Code, "offset": ce.Offset})
	default:
		got = jsonOf(map[string]any{"error": err.Error()})
	}
	return []Part{compare("expect", got, m["expect"])}
}
