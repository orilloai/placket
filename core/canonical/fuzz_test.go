package canonical_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"unicode/utf16"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/testing/seeds"
)

// FuzzTypedHash hashes any bytes. It never panics; it refuses exactly what the canonical form
// refuses, with the same error, and otherwise returns sha256: and the hex SHA-256 of the canonical
// bytes (canonical-3).
func FuzzTypedHash(f *testing.F) {
	for _, s := range []string{``, `{}`, `{ "b": 1, "a": [ true ] }`, `{"a":1,"a":2}`, `1.5`, "\xff", `"\ud800"`} {
		f.Add([]byte(s))
	}
	for _, in := range seeds.Inputs(f, "canonical") {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		h, err := canonical.TypedHash(in)
		form, ferr := canonical.Form(in)
		if ferr != nil {
			if err == nil || err.Error() != ferr.Error() || h != "" {
				t.Fatalf("got %q, %v; the form refuses with %v", h, err, ferr)
			}
			return
		}
		sum := sha256.Sum256(form)
		if err != nil || h != "sha256:"+hex.EncodeToString(sum[:]) {
			t.Fatalf("got %q, %v", h, err)
		}
	})
}

// FuzzLess orders any two strings. It never panics; the order is strict and total, and is the order
// of their UTF-16 code units, computed here apart (canonical-1).
func FuzzLess(f *testing.F) {
	for _, p := range [][2]string{
		{"a", "b"}, {"", "a"}, {"a", "a"}, {"￿", "😀"}, {"😀", ""}, {"\xff", "a"}, {"ab", "a"}, {"B", "a"},
	} {
		f.Add(p[0], p[1])
	}
	for _, s := range seeds.Strings(f, "founding") {
		f.Add(s, "ed25519:")
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		ab, ba := canonical.Less(a, b), canonical.Less(b, a)
		ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
		want := len(ua) < len(ub)
		for i := 0; i < len(ua) && i < len(ub); i++ {
			if ua[i] != ub[i] {
				want = ua[i] < ub[i]
				break
			}
		}
		if ab != want || (ab && ba) || canonical.Less(a, a) {
			t.Fatalf("Less(%q, %q) = %v, Less(%q, %q) = %v", a, b, ab, b, a, ba)
		}
	})
}
