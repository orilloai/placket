package conformance

import (
	"encoding/hex"
	"encoding/json"

	"github.com/orilloai/placket/core/signing"
)

func init() {
	registered["sign"] = runSign
	registered["verify-signature"] = runVerifySignature
}

// runSign signs the input as the key named signer under typeUri and compares the signing bytes and
// the signature.
func runSign(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	keys, ok := keysOf(m)
	key, named := keys[m.str("signer")]
	if !ok || !named {
		return bad("signer", "the signer names no key")
	}
	in, ok := inputOf(m)
	if !ok {
		return bad("input", "the input does not decode")
	}
	sb, err := signing.SigningBytes(in, m.str("typeUri"))
	if err != nil {
		return bad("expect", "signing bytes: "+err.Error())
	}
	sig, err := signing.Sign(in, m.str("typeUri"), key)
	if err != nil {
		return bad("expect", "sign: "+err.Error())
	}
	return []Part{compare("expect", jsonOf(map[string]any{"signingBytesHex": hex.EncodeToString(sb), "sig": sig}), m["expect"])}
}

// runVerifySignature verifies the input under typeUri by reference and compares valid.
func runVerifySignature(v Vector) []Part {
	m, _ := membersOf(v.Bytes)
	in, ok := inputOf(m)
	if !ok {
		return bad("input", "the input does not decode")
	}
	valid := signing.Verify(in, m.str("typeUri"), m.str("reference"))
	return []Part{compare("expect", jsonOf(map[string]any{"valid": valid}), m["expect"])}
}

// keysOf reads a vector's keys: each name's Ed25519 seed, in hex (conformance-vectors-2).
func keysOf(m members) (map[string]signing.Key, bool) {
	var seeds map[string]string
	if m.has("keys") && json.Unmarshal(m["keys"], &seeds) != nil {
		return nil, false
	}
	out := map[string]signing.Key{}
	for name, s := range seeds {
		seed, err := hex.DecodeString(s)
		if err != nil {
			return nil, false
		}
		k, err := signing.NewKey(seed)
		if err != nil {
			return nil, false
		}
		out[name] = k
	}
	return out, true
}
