package signing

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
)

func testKey(t testing.TB, name string) Key {
	t.Helper()
	seed := sha256.Sum256([]byte("placket vectors: key " + name))
	k, err := NewKey(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// The curve's constants, for checking the small-order list by arithmetic of its own.
var (
	fp, _  = new(big.Int).SetString("57896044618658097711785492504343953926634992332820282019728792003956564819949", 10)
	curveD = func() *big.Int {
		n := new(big.Int).Mul(big.NewInt(-121665), new(big.Int).ModInverse(big.NewInt(121666), fp))
		return n.Mod(n, fp)
	}()
	sqrtM1   = new(big.Int).Exp(big.NewInt(2), new(big.Int).Div(new(big.Int).Sub(fp, big.NewInt(1)), big.NewInt(4)), fp)
	identity = [2]*big.Int{big.NewInt(0), big.NewInt(1)}
)

// decode decodes a point by RFC 8032, section 5.1.3, or reports that it is not one.
func decode(b []byte) ([2]*big.Int, bool) {
	le := make([]byte, 32)
	for i := range le {
		le[i] = b[31-i]
	}
	sign := le[0] >> 7
	le[0] &= 0x7f
	y := new(big.Int).SetBytes(le)
	if y.Cmp(fp) >= 0 {
		return [2]*big.Int{}, false
	}
	y2 := new(big.Int).Mul(y, y)
	u := new(big.Int).Sub(y2, big.NewInt(1))
	v := new(big.Int).Add(new(big.Int).Mul(curveD, y2), big.NewInt(1))
	x2 := new(big.Int).Mul(u, new(big.Int).ModInverse(v, fp))
	x2.Mod(x2, fp)
	x := new(big.Int).Exp(x2, new(big.Int).Div(new(big.Int).Add(fp, big.NewInt(3)), big.NewInt(8)), fp)
	if new(big.Int).Mod(new(big.Int).Sub(new(big.Int).Mul(x, x), x2), fp).Sign() != 0 {
		x.Mul(x, sqrtM1).Mod(x, fp)
	}
	if new(big.Int).Mod(new(big.Int).Sub(new(big.Int).Mul(x, x), x2), fp).Sign() != 0 {
		return [2]*big.Int{}, false
	}
	if x.Sign() == 0 && sign == 1 {
		return [2]*big.Int{}, false
	}
	if uint(x.Bit(0)) != uint(sign) {
		x.Sub(fp, x)
	}
	return [2]*big.Int{x, y}, true
}

// add adds two points of the curve.
func add(p, q [2]*big.Int) [2]*big.Int {
	x1y2 := new(big.Int).Mul(p[0], q[1])
	y1x2 := new(big.Int).Mul(p[1], q[0])
	x1x2 := new(big.Int).Mul(p[0], q[0])
	y1y2 := new(big.Int).Mul(p[1], q[1])
	dxy := new(big.Int).Mul(curveD, new(big.Int).Mul(x1x2, y1y2))
	dxy.Mod(dxy, fp)
	x := new(big.Int).Mul(new(big.Int).Add(x1y2, y1x2), new(big.Int).ModInverse(new(big.Int).Add(big.NewInt(1), dxy), fp))
	y := new(big.Int).Mul(new(big.Int).Add(y1y2, x1x2), new(big.Int).ModInverse(new(big.Int).Mod(new(big.Int).Sub(big.NewInt(1), dxy), fp), fp))
	return [2]*big.Int{x.Mod(x, fp), y.Mod(y, fp)}
}

// TestSmallOrderListIsTheTorsionSubgroup checks the list keySigns refuses against arithmetic of the
// test's own: eight distinct points, each of order 1, 2, 4 or 8, which is the whole torsion subgroup
// of the curve, so no small-order key is missing from the list (signing-1).
func TestSmallOrderListIsTheTorsionSubgroup(t *testing.T) {
	t.Parallel()
	if len(smallOrder) != 8 {
		t.Fatalf("%d points, want 8", len(smallOrder))
	}
	seen := map[string]bool{}
	for _, b := range smallOrder {
		p, ok := decode(b)
		if !ok {
			t.Fatalf("%x is not a canonical encoding of a point", b)
		}
		key := p[0].String() + "," + p[1].String()
		if seen[key] {
			t.Fatalf("%x twice", b)
		}
		seen[key] = true
		q := p
		for i := 0; i < 3; i++ {
			q = add(q, q)
		}
		if q[0].Cmp(identity[0]) != 0 || q[1].Cmp(identity[1]) != 0 {
			t.Fatalf("%x: [8]P is not the identity", b)
		}
		if keySigns(ed25519.PublicKey(b)) {
			t.Fatalf("%x signs", b)
		}
	}
}

// TestNonCanonicalKeysSignNothing checks the keys whose 32 bytes are not the canonical encoding of
// their point: y at or above p, and x zero with its sign bit set (signing-1).
func TestNonCanonicalKeysSignNothing(t *testing.T) {
	t.Parallel()
	good := testKey(t, "proposer")
	public, _ := ParseReference(good.Reference())
	if !keySigns(public) {
		t.Fatal("a key from a seed does not sign")
	}
	for _, h := range []string{
		"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // y = p
		"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f", // y = p + 1, the identity again
		"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // y = 2^255 - 1
		"0100000000000000000000000000000000000000000000000000000000000080", // x = 0 with its sign bit
		"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", // y = p - 1, x = 0, sign bit
	} {
		b, _ := hex.DecodeString(h)
		if keySigns(b) {
			t.Fatalf("%s signs", h)
		}
	}
}

// TestPayloadSignaturesRefuseTheFourTypeURIs is signing-4's scenario: a payload signature whose
// payload type is a placket type URI is refused both ways, and no object is signed or verified under
// a payload type that is not one of them.
func TestPayloadSignaturesRefuseTheFourTypeURIs(t *testing.T) {
	t.Parallel()
	k := testKey(t, "proposer")
	envelope := []byte(`{"hash":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","seq":1}`)
	for _, uri := range []string{TypeDeclaration, TypeBinding, TypeEnvelope, TypeRegistration} {
		sig, err := SignPayload(uri, envelope, k)
		var se *Error
		if sig != nil || !errors.As(err, &se) || se.Code != CodeTypeURI {
			t.Fatalf("SignPayload under %s: %x, %v", uri, sig, err)
		}
		// A raw Ed25519 signature over the very pre-authentication encoding an object signature uses.
		raw := ed25519.Sign(k.bytes(), PAE(uri, envelope))
		if VerifyPayload(uri, envelope, raw, k.Reference()) {
			t.Fatalf("VerifyPayload under %s accepted a signature", uri)
		}
	}
	// The receipt's payload type signs and verifies as a payload, and never as an object.
	const intoto = "application/vnd.in-toto+json"
	sig, err := SignPayload(intoto, envelope, k)
	if err != nil || !VerifyPayload(intoto, envelope, sig, k.Reference()) {
		t.Fatalf("a payload under %s: %v", intoto, err)
	}
	if _, err := Sign(envelope, intoto, k); !isTypeURI(err) {
		t.Fatalf("Sign under the payload type %s: got %v, want %s", intoto, err, CodeTypeURI)
	}
	if _, err := Sign(envelope, TypeRegistration, k); !isTypeURI(err) {
		t.Fatalf("Sign under the reserved registration type URI: got %v, want %s", err, CodeTypeURI)
	}
	obj, _ := Sign(envelope, TypeEnvelope, k)
	signed := []byte(strings.Replace(string(envelope), `"seq":1`, `"seq":1,"sig":"`+obj+`"`, 1))
	if !Verify(signed, TypeEnvelope, k.Reference()) || Verify(signed, intoto, k.Reference()) {
		t.Fatal("an envelope verifies under the in-toto payload type, or not under its own")
	}
}

// TestKeyNeverPrints prints a Key under every verb, a verb it does not know included, and finds
// neither its seed nor its private key (secrets-never-print).
func TestKeyNeverPrints(t *testing.T) {
	t.Parallel()
	seed := sha256.Sum256([]byte("placket vectors: key proposer"))
	k, _ := NewKey(seed[:])
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T", "%z"} {
		out := fmt.Sprintf(verb, k) + fmt.Sprintf(verb, &k) + fmt.Sprintf(verb, []Key{k})
		for _, secret := range []string{hex.EncodeToString(seed[:]), hex.EncodeToString(k.bytes()), fmt.Sprintf("%X", seed[:]), string(seed[:8])} {
			if strings.Contains(out, secret) {
				t.Fatalf("%s prints the key: %q", verb, out)
			}
		}
	}
	if got := fmt.Sprintf("%v|%#v|%s", k, k, k); got != "signing.Key|signing.Key|signing.Key" {
		t.Fatalf("got %q", got)
	}
}

// TestSigningDependsOnEachInput changes the object, the type URI and the key in turn, and each
// changes the signature (inputs-as-parameters).
func TestSigningDependsOnEachInput(t *testing.T) {
	t.Parallel()
	a, b := testKey(t, "proposer"), testKey(t, "applier")
	obj := []byte(`{"actor":"x","terms":[]}`)
	base, err := Sign(obj, TypeDeclaration, a)
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]func() (string, error){
		"object":   func() (string, error) { return Sign([]byte(`{"actor":"y","terms":[]}`), TypeDeclaration, a) },
		"type URI": func() (string, error) { return Sign(obj, TypeBinding, a) },
		"key":      func() (string, error) { return Sign(obj, TypeDeclaration, b) },
	} {
		s, err := got()
		if err != nil || s == base {
			t.Fatalf("a changed %s left the signature %s, %v", name, s, err)
		}
	}
	// The sig member is not signed, and whitespace is not either.
	again, _ := Sign([]byte(` { "terms" : [ ] , "sig" : "00", "actor" : "x" } `), TypeDeclaration, a)
	if again != base {
		t.Fatal("a sig member or whitespace changed the signature")
	}
}

// TestErrorTextQuotesNoInput triggers each code of Error with a planted seed and object, and reads
// the text (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t-body-salt-seed-key-payload"
	k := testKey(t, "proposer")
	_, e1 := NewKey([]byte(secret))
	_, e2 := Sign([]byte(`{"text":"`+secret+`"}`), "urn:"+secret, k)
	_, e3 := SigningBytes([]byte(`{"text":"`+secret+`",}`), TypeEnvelope)
	_, e4 := SignPayload(TypeEnvelope, []byte(secret), k)
	for _, c := range []struct {
		err  error
		text string
	}{
		{e1, "signing: seed"}, {e2, "signing: type-uri"}, {e3, "signing: object"}, {e4, "signing: type-uri"},
	} {
		if c.err == nil || c.err.Error() != c.text || strings.Contains(c.err.Error(), secret) {
			t.Fatalf("got %v, want %q", c.err, c.text)
		}
	}
}

// FuzzVerify verifies any object bytes under any reference and type URI. It never panics, and what
// verifies is an object whose signature its key made.
func FuzzVerify(f *testing.F) {
	k := testKey(f, "proposer")
	obj := []byte(`{"actor":"` + k.Reference() + `","terms":[]}`)
	sig, _ := Sign(obj, TypeDeclaration, k)
	signed := bytes.Replace(obj, []byte(`"terms"`), []byte(`"sig":"`+sig+`","terms"`), 1)
	f.Add(signed, TypeDeclaration, k.Reference())
	f.Add(signed, TypeEnvelope, k.Reference())
	f.Add([]byte(`{"sig":7}`), TypeBinding, "ed25519:"+strings.Repeat("0", 64))
	f.Add([]byte(`[]`), TypeRegistration, "ecdsa:")
	f.Add([]byte("\xff"), "", "")
	f.Fuzz(func(t *testing.T, object []byte, typeURI, ref string) {
		if Verify(object, typeURI, ref) {
			if _, err := SigningBytes(object, typeURI); err != nil {
				t.Fatalf("verified bytes with no signing bytes: %v", err)
			}
		}
		_, _ = SigningBytes(object, typeURI)
		_, _ = ParseReference(ref)
	})
}

// FuzzVerifyPayload verifies any payload, signature and reference, and signs any payload. It never
// panics.
func FuzzVerifyPayload(f *testing.F) {
	k := testKey(f, "mediator")
	sig, _ := SignPayload("application/vnd.in-toto+json", []byte("{}"), k)
	f.Add("application/vnd.in-toto+json", []byte("{}"), sig, k.Reference(), []byte("seed"))
	f.Add(TypeEnvelope, []byte("{}"), sig[:63], "ed25519:", make([]byte, 32))
	f.Fuzz(func(t *testing.T, payloadType string, payload, sig []byte, ref string, seed []byte) {
		_ = VerifyPayload(payloadType, payload, sig, ref)
		_ = PAE(payloadType, payload)
		if key, err := NewKey(seed); err == nil {
			s, err := SignPayload(payloadType, payload, key)
			if err == nil && !VerifyPayload(payloadType, payload, s, key.Reference()) {
				t.Fatal("a payload signature does not verify")
			}
		}
	})
}

// isTypeURI reports whether err is this package's Error with code type-uri.
func isTypeURI(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == CodeTypeURI && err.Error() == "signing: type-uri"
}
