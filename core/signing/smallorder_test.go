package signing

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"math/big"
	"strconv"
	"testing"
)

// groupOrder is L, the order of the curve's prime subgroup (RFC 8032).
var groupOrder = new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 252), func() *big.Int {
	c, _ := new(big.Int).SetString("27742317777372353535851937790883648493", 10)
	return c
}())

// encode encodes a point by RFC 8032, section 5.1.2: y in little-endian, the low bit of x on top.
func encode(p [2]*big.Int) []byte {
	be := p[1].FillBytes(make([]byte, 32))
	out := make([]byte, 32)
	for i := range out {
		out[i] = be[31-i]
	}
	out[31] |= byte(p[0].Bit(0)) << 7
	return out
}

// forgery finds a signature over message that the bare Ed25519 equation accepts under the
// small-order key a: R = [S]B - [j]A for some j, chosen so that [k]A = [j]A for the challenge k the
// equation computes, where S is the scalar of a key made from a seed of the test's own. Under the
// identity, (enc([S]B), S) holds for any message; under the other seven, a few tries find one.
func forgery(t *testing.T, a []byte, message []byte) []byte {
	t.Helper()
	ap, ok := decode(a)
	if !ok {
		t.Fatalf("%x is not a point", a)
	}
	multiples := [][2]*big.Int{identity}
	for j := 1; j < 8; j++ {
		multiples = append(multiples, add(multiples[j-1], ap))
	}
	for i := 0; i < 256; i++ {
		seed := sha512.Sum512([]byte("placket tests: a forgery under a small-order key " + strconv.Itoa(i)))
		h := sha512.Sum512(seed[:32])
		h[0] &= 248
		h[31] &= 127
		h[31] |= 64
		le := h[:32]
		be := make([]byte, 32)
		for k := range be {
			be[k] = le[31-k]
		}
		s := new(big.Int).Mod(new(big.Int).SetBytes(be), groupOrder)
		sb, ok := decode(ed25519.NewKeyFromSeed(seed[:32]).Public().(ed25519.PublicKey))
		if !ok {
			t.Fatal("a public key that does not decode")
		}
		sBytes := s.FillBytes(make([]byte, 32))
		sLE := make([]byte, 32)
		for k := range sLE {
			sLE[k] = sBytes[31-k]
		}
		for _, m := range multiples {
			neg := [2]*big.Int{new(big.Int).Mod(new(big.Int).Neg(m[0]), fp), m[1]}
			sig := append(encode(add(sb, neg)), sLE...)
			if ed25519.Verify(a, message, sig) {
				return sig
			}
		}
	}
	t.Fatalf("no forgery under %x", a)
	return nil
}

// TestSmallOrderKeysVerifyNoForgery is signing-1's guard where it matters: under each of the eight
// small-order keys, a signature the bare equation accepts, over a receipt's payload and over an
// object's signing bytes, verifies nothing, as a payload or as an object.
func TestSmallOrderKeysVerifyNoForgery(t *testing.T) {
	t.Parallel()
	const intoto = "application/vnd.in-toto+json"
	payload := []byte(`{"_type":"https://in-toto.io/Statement/v1","predicate":{"verdict":"pass"}}`)
	for _, a := range smallOrder {
		ref := ReferencePrefix + hex.EncodeToString(a)
		sig := forgery(t, a, PAE(intoto, payload))
		if VerifyPayload(intoto, payload, sig, ref) {
			t.Fatalf("under %s, a forged payload signature verifies", ref)
		}

		object := []byte(`{"actor":"` + ref + `","terms":[]}`)
		sb, err := SigningBytes(object, TypeDeclaration)
		if err != nil {
			t.Fatal(err)
		}
		osig := forgery(t, a, sb)
		signed := []byte(`{"actor":"` + ref + `","sig":"` + hex.EncodeToString(osig) + `","terms":[]}`)
		if Verify(signed, TypeDeclaration, ref) {
			t.Fatalf("under %s, a forged declaration verifies", ref)
		}
	}
}
