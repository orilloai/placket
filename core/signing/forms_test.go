package signing

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
)

// TestReferenceLength holds an actor reference's length: ed25519: and exactly 64 lowercase hex
// digits, so 62 or 66 digits, each an even run of hex, are not a reference (signing-1).
func TestReferenceLength(t *testing.T) {
	t.Parallel()
	digits := strings.Repeat("ab", 32)
	for _, c := range []struct {
		ref string
		ok  bool
	}{
		{ReferencePrefix + digits, true},
		{ReferencePrefix + digits + "ab", false},
		{ReferencePrefix + digits[:62], false},
		{ReferencePrefix + strings.Repeat("ab", 64), false},
	} {
		public, ok := ParseReference(c.ref)
		if ok != c.ok || (ok && hex.EncodeToString(public) != digits) || (!ok && public != nil) {
			t.Fatalf("ParseReference(%q) = %x, %v", c.ref, public, ok)
		}
	}
}

// TestSigningBytesOfAnObjectAreCanonical signs an object whose bytes a JSON encoder alone would get
// wrong: names and values holding <, & and U+2028, which canonical bytes carry raw, and a name
// outside the Basic Multilingual Plane, which sorts before U+FF61 by UTF-16 code units. The signing
// bytes are the pre-authentication encoding of the canonical bytes written here by hand, and the
// signature verifies over them by Ed25519 itself (signing-2).
func TestSigningBytesOfAnObjectAreCanonical(t *testing.T) {
	t.Parallel()
	object := []byte("{ \"\uff61\": [3, true], \"\U0001F600\": \"<&\u2028\", \"<&\u2028\": 1, \"sig\": \"00\" }")
	canonical := "{\"<&\xe2\x80\xa8\":1,\"\xf0\x9f\x98\x80\":\"<&\xe2\x80\xa8\",\"\xef\xbd\xa1\":[3,true]}"
	want := []byte("DSSEv1 35 " + TypeDeclaration + " 41 " + canonical)
	got, err := SigningBytes(object, TypeDeclaration)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
	k := testKey(t, "proposer")
	sig, err := Sign(object, TypeDeclaration, k)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(sig)
	public, _ := ParseReference(k.Reference())
	if !ed25519.Verify(public, want, raw) {
		t.Fatal("the signature does not verify over the hand-written signing bytes")
	}
}
