package canonical_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
)

// FuzzForm feeds the canonical form any bytes. It never panics; what it accepts is its own canonical
// form, which the form accepts unchanged, and whose typed hash it computes; what it refuses it
// refuses with one of the five codes, at an offset inside the input or at its length.
func FuzzForm(f *testing.F) {
	for _, s := range []string{
		``, `{}`, `[]`, `{ "b": 1, "a": [ true ] }`, `"😀"`, `[1.0,1e2,-0,0e999999999]`,
		`{"a":1,"a":2}`, `"﷐\q"`, `[1 2, "\ud800"]`, `[\ud800]`, "\"\xff\"", "\xef\xbb\xbf{}",
		`12345678901234567.5`, `1e9223372036854775808`, `[1,2,tru]`, `{"a":{"b":[null,false,"\u0000"]}}`,
		strings.Repeat("[", 1001), `-`, `1e`, `1.`, `"\u12"`, `{"a" 1}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := canonical.Form(in)
		if err != nil {
			var ce *canonical.Error
			if !errors.As(err, &ce) {
				t.Fatalf("a refusal that is not a canonical.Error: %v", err)
			}
			switch ce.Code {
			case canonical.InvalidJSON, canonical.InvalidString, canonical.DuplicateKey, canonical.FractionalNumber, canonical.UnsafeInteger:
			default:
				t.Fatalf("an unknown code %q", ce.Code)
			}
			if ce.Offset < 0 || ce.Offset > len(in) {
				t.Fatalf("offset %d outside an input of %d bytes", ce.Offset, len(in))
			}
			if out != nil {
				t.Fatalf("a refusal output %d bytes", len(out))
			}
			return
		}
		again, err := canonical.Form(out)
		if err != nil || !bytes.Equal(again, out) {
			t.Fatalf("the canonical form of canonical bytes changed them: %q to %q, %v", out, again, err)
		}
		h1, err1 := canonical.TypedHash(in)
		h2, err2 := canonical.TypedHash(out)
		if err1 != nil || err2 != nil || h1 != h2 || len(h1) != len("sha256:")+64 {
			t.Fatalf("typed hashes %q and %q, %v, %v", h1, h2, err1, err2)
		}
	})
}

// TestErrorTextQuotesNoInput triggers each of the five codes on an input that holds a planted
// secret, and reads each error's text: it is the package, the code and the offset, and nothing of
// the input (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t-body-salt-seed-key"
	for _, c := range []struct {
		in   string
		code canonical.Code
		text string
	}{
		{`{"` + secret + `":1,` + "\xff}", canonical.InvalidString, "canonical: invalid-string at offset 31"},
		{`{"` + secret + `":1,}`, canonical.InvalidJSON, "canonical: invalid-json at offset 31"},
		{`{"` + secret + `":1,"` + secret + `":2}`, canonical.DuplicateKey, "canonical: duplicate-key at offset 31"},
		{`{"` + secret + `":0.5}`, canonical.FractionalNumber, "canonical: fractional-number at offset 29"},
		{`{"` + secret + `":1e99}`, canonical.UnsafeInteger, "canonical: unsafe-integer at offset 29"},
	} {
		_, err := canonical.Form([]byte(c.in))
		var ce *canonical.Error
		if !errors.As(err, &ce) || ce.Code != c.code {
			t.Fatalf("%q: got %v, want %s", c.in, err, c.code)
		}
		if err.Error() != c.text || strings.Contains(err.Error(), secret) {
			t.Fatalf("%q: error text %q, want %q", c.in, err.Error(), c.text)
		}
	}
}

// TestLessOrdersByUTF16 checks the order of names and references: by UTF-16 code units, so a
// character outside the Basic Multilingual Plane comes before U+FB01 (canonical-1).
func TestLessOrdersByUTF16(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		a, b string
		less bool
	}{
		{"\U0001F600", "ﬁ", true},
		{"ﬁ", "\U0001F600", false},
		{"a", "ab", true},
		{"ab", "a", false},
		{"a", "a", false},
		{"B", "a", true},
	} {
		if got := canonical.Less(c.a, c.b); got != c.less {
			t.Fatalf("Less(%q, %q) = %v, want %v", c.a, c.b, got, c.less)
		}
	}
}

// TestTypedHashRefusesWhatTheFormRefuses checks that no typed hash is computed from bytes the
// canonical form refuses (canonical-3).
func TestTypedHashRefusesWhatTheFormRefuses(t *testing.T) {
	t.Parallel()
	h, err := canonical.TypedHash([]byte(`{"a":1,"a":2}`))
	var ce *canonical.Error
	if h != "" || !errors.As(err, &ce) || ce.Code != canonical.DuplicateKey || ce.Offset != 7 {
		t.Fatalf("got %q, %v", h, err)
	}
	h, err = canonical.TypedHash([]byte(`{ }`))
	if err != nil || h != "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a" {
		t.Fatalf("got %q, %v", h, err)
	}
}
