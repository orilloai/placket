package signing

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

// The seeds the fuzz targets below share: references and objects of each form section 2 names, and
// bytes of no form at all.
var (
	fuzzRefs = []string{
		"", "ed25519:", "ed25519:" + strings.Repeat("0", 64), "ed25519:" + strings.Repeat("A", 64),
		"ed25519:" + strings.Repeat("f", 63), "ecdsa:" + strings.Repeat("1", 64),
		"ed25519:" + hex.EncodeToString(smallOrder[4]), "ed25519:" + strings.Repeat("ab", 32) + "\x00",
	}
	fuzzObjects = []string{
		`{"actor":"x","terms":[]}`, `{"sig":"00","seq":1}`, ` { "b" : 1 , "a" : [ ] } `, `[]`, `"x"`,
		`{"a":1,"a":2}`, `{"a":1.5}`, "\xff", ``, `{"\ud800":1}`,
	}
	fuzzTypes = []string{TypeDeclaration, TypeBinding, TypeEnvelope, TypeRegistration, "application/vnd.in-toto+json", "", "\x00"}
)

// FuzzNewKey makes a key of any seed. It never panics; a seed of 32 bytes makes the key RFC 8032
// gives it, and any other length is refused with the code seed.
func FuzzNewKey(f *testing.F) {
	for _, n := range []int{0, 1, 31, 32, 33, 64} {
		f.Add(bytes.Repeat([]byte{0x5a}, n))
	}
	f.Fuzz(func(t *testing.T, seed []byte) {
		k, err := NewKey(seed)
		if len(seed) != ed25519.SeedSize {
			if e, ok := err.(*Error); !ok || e.Code != CodeSeed || k.Reference() != "" {
				t.Fatalf("a seed of %d bytes: %v", len(seed), err)
			}
			return
		}
		want := "ed25519:" + hex.EncodeToString(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
		if err != nil || k.Reference() != want {
			t.Fatalf("got %q, %v, want %q", k.Reference(), err, want)
		}
		if _, ok := ParseReference(k.Reference()); !ok {
			t.Fatalf("a key's reference does not parse: %q", k.Reference())
		}
	})
}

// FuzzParseReference reads any string as a reference. It never panics, and what it accepts is
// ed25519: and the lowercase hex of the 32 bytes it returns.
func FuzzParseReference(f *testing.F) {
	for _, r := range fuzzRefs {
		f.Add(r)
	}
	f.Fuzz(func(t *testing.T, ref string) {
		public, ok := ParseReference(ref)
		if !ok {
			if public != nil {
				t.Fatalf("a refused reference returned %x", public)
			}
			return
		}
		if len(public) != ed25519.PublicKeySize || ref != "ed25519:"+hex.EncodeToString(public) {
			t.Fatalf("%q read as %x", ref, public)
		}
	})
}

// FuzzPAE encodes any payload type and payload. It never panics, and the encoding is DSSEv1, each
// length in decimal and each value, separated by spaces, as the test spells it.
func FuzzPAE(f *testing.F) {
	for _, ty := range fuzzTypes {
		f.Add(ty, []byte(`{}`))
	}
	f.Add("a b", []byte("1 2 "))
	f.Add("", []byte{})
	f.Fuzz(func(t *testing.T, payloadType string, payload []byte) {
		want := "DSSEv1 " + strconv.Itoa(len(payloadType)) + " " + payloadType + " " + strconv.Itoa(len(payload)) + " " + string(payload)
		if got := PAE(payloadType, payload); string(got) != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

// FuzzSigningBytes takes the signing bytes of any bytes under any type URI. It never panics; the
// bytes exist only for an object type URI, and are the encoding of that URI and of the object
// without its sig.
func FuzzSigningBytes(f *testing.F) {
	for i, o := range fuzzObjects {
		f.Add([]byte(o), fuzzTypes[i%len(fuzzTypes)])
	}
	f.Fuzz(func(t *testing.T, object []byte, typeURI string) {
		b, err := SigningBytes(object, typeURI)
		if err != nil {
			e, ok := err.(*Error)
			if !ok || (e.Code != CodeTypeURI && e.Code != CodeObject) || b != nil {
				t.Fatalf("got %x, %v", b, err)
			}
			if objectType(typeURI) != (e.Code == CodeObject) {
				t.Fatalf("the code %s under %q", e.Code, typeURI)
			}
			return
		}
		if !objectType(typeURI) || !bytes.HasPrefix(b, []byte("DSSEv1 "+strconv.Itoa(len(typeURI))+" "+typeURI+" ")) {
			t.Fatalf("signing bytes %q under %q", b, typeURI)
		}
	})
}

// FuzzSign signs any bytes under any type URI with a key of any seed. It never panics; a signature
// is 128 lowercase hex digits, made over the signing bytes, and the object carrying it verifies.
func FuzzSign(f *testing.F) {
	seed := sha256.Sum256([]byte("placket vectors: key proposer"))
	for i, o := range fuzzObjects {
		f.Add([]byte(o), fuzzTypes[i%len(fuzzTypes)], seed[:])
	}
	f.Add([]byte(`{}`), TypeEnvelope, []byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, object []byte, typeURI string, seed []byte) {
		k, _ := NewKey(seed)
		sig, err := Sign(object, typeURI, k)
		if err != nil {
			if _, ok := err.(*Error); !ok || sig != "" {
				t.Fatalf("got %q, %v", sig, err)
			}
			return
		}
		raw, _ := hex.DecodeString(sig)
		sb, _ := SigningBytes(object, typeURI)
		if len(sig) != 128 || hex.EncodeToString(raw) != sig || !ed25519.Verify(k.bytes().Public().(ed25519.PublicKey), sb, raw) {
			t.Fatalf("a signature %q", sig)
		}
		u, _, _ := unsigned(object)
		signed := `{"sig":"` + sig + `"}`
		if string(u) != `{}` {
			signed = string(u[:len(u)-1]) + `,"sig":"` + sig + `"}`
		}
		if !Verify([]byte(signed), typeURI, k.Reference()) {
			t.Fatalf("%s does not verify", signed)
		}
	})
}

// FuzzSignPayload signs any payload under any payload type with a key of any seed. It never panics;
// a payload type of signing-4's four is refused, and every other signature is the key's over the
// pre-authentication encoding.
func FuzzSignPayload(f *testing.F) {
	seed := sha256.Sum256([]byte("placket vectors: key mediator"))
	for _, ty := range fuzzTypes {
		f.Add(ty, []byte(`{"seq":1}`), seed[:])
	}
	f.Add("application/vnd.in-toto+json", []byte{}, []byte{})
	f.Fuzz(func(t *testing.T, payloadType string, payload, seed []byte) {
		k, kerr := NewKey(seed)
		sig, err := SignPayload(payloadType, payload, k)
		switch {
		case placketType(payloadType):
			if e, ok := err.(*Error); !ok || e.Code != CodeTypeURI || sig != nil {
				t.Fatalf("under %q: %x, %v", payloadType, sig, err)
			}
		case kerr != nil:
			if e, ok := err.(*Error); !ok || e.Code != CodeSeed || sig != nil {
				t.Fatalf("the zero key: %x, %v", sig, err)
			}
		default:
			if err != nil || !ed25519.Verify(k.bytes().Public().(ed25519.PublicKey), PAE(payloadType, payload), sig) {
				t.Fatalf("a payload signature %x, %v", sig, err)
			}
		}
	})
}
