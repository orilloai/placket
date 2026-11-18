package shape

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/testing/seeds"
)

// Each fuzz target below takes any string or bytes. None panics, and each result is checked against a
// statement of the rule written apart from the code: a regular expression, or another reading of the
// value (core-no-panic).

var (
	labelForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	gateForm  = regexp.MustCompile(`^[a-z0-9]+([-_][a-z0-9]+)*$`)
	hashForm  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	hexForm   = regexp.MustCompile(`^([0-9a-f]{2})+$`)
	timeForm  = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
)

// FuzzTimestamp holds the timestamp rule: what it accepts has the one form and adds zero seconds to
// itself, and a second later is a timestamp too, or the last time (maybe-1).
func FuzzTimestamp(f *testing.F) {
	for _, s := range []string{
		"2026-11-12T09:00:00Z", "2028-02-29T23:59:59Z", "2027-02-29T00:00:00Z", "2026-12-31T23:59:60Z",
		"2026-12-31T24:00:00Z", "2026-11-12T09:00:00+00:00", "2026-11-12T09:00:00.5Z", LastTime, "0000-01-01T00:00:00Z",
		"", "2026-11-12t09:00:00z", "\uff12026-11-12T09:00:00Z",
	} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "questions") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !Timestamp(s) {
			return
		}
		if !timeForm.MatchString(s) || AddSeconds(s, 0) != s {
			t.Fatalf("Timestamp accepts %q", s)
		}
		if next := AddSeconds(s, 1); !Timestamp(next) || (next <= s && s != LastTime) {
			t.Fatalf("a second after %q is %q", s, next)
		}
	})
}

// FuzzAddSeconds adds any number of seconds to any string. A timestamp plus a number from 0 is a
// timestamp no earlier, and at most the last time; anything but a timestamp, or a negative number,
// gives no time (maybe-1).
func FuzzAddSeconds(f *testing.F) {
	for _, c := range []struct {
		s string
		n int64
	}{
		{"2026-11-12T09:00:00Z", 3600}, {"2026-11-12T09:00:00Z", MaxSafe}, {"9999-12-31T23:59:58Z", 2},
		{"2026-11-12T09:00:00Z", -1}, {"2026-11-12T09:00:00Z", -1 << 63}, {"2026-11-12T09:00:00Z", 1<<63 - 1},
		{"not a time", 1}, {"", 0}, {"0000-01-01T00:00:00Z", -1}, {"0000-01-01T0:00:00Z", -1 << 63},
		{"2026-11-12T9:00:00Z", 1}, {"2026-11-12T09:00:00.5Z", 1},
	} {
		f.Add(c.s, c.n)
	}
	for _, s := range seeds.Strings(f, "questions") {
		f.Add(s, int64(3600))
	}
	f.Fuzz(func(t *testing.T, s string, n int64) {
		out := AddSeconds(s, n)
		if !Timestamp(s) {
			if out != "" {
				t.Fatalf("AddSeconds(%q, %d) = %q", s, n, out)
			}
			return
		}
		if n < 0 {
			if out != "" {
				t.Fatalf("AddSeconds(%q, %d) = %q, want no time", s, n, out)
			}
			return
		}
		if !Timestamp(out) || out < s || out > LastTime {
			t.Fatalf("AddSeconds(%q, %d) = %q", s, n, out)
		}
	})
}

// FuzzLabel holds the form of labels and kinds: words of lowercase letters and digits joined by single
// hyphens, at most 64 characters (setup-contract-1).
func FuzzLabel(f *testing.F) {
	for _, s := range []string{"a", "a-b-1", "-a", "a-", "a--b", "A", "a_b", "", strings.Repeat("a", 64), strings.Repeat("a", 65), "\u00e9"} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "founding") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if Label(s) != (len(s) <= 64 && labelForm.MatchString(s)) {
			t.Fatalf("Label(%q) = %v", s, Label(s))
		}
	})
}

// FuzzGate holds the reserved gate's form: words of lowercase letters and digits joined by single
// hyphens or underscores, at most 64 characters (crossing-1).
func FuzzGate(f *testing.F) {
	for _, s := range []string{"out_you", "out-you", "Out_You", "out__you", "_out", "out_", "a-_b", "", strings.Repeat("a_", 32)} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "founding") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if Gate(s) != (len(s) <= 64 && gateForm.MatchString(s)) {
			t.Fatalf("Gate(%q) = %v", s, Gate(s))
		}
	})
}

// FuzzSelector holds the form of a role selector, role: and a label, and that it returns the label
// (setup-contract-1).
func FuzzSelector(f *testing.F) {
	for _, s := range []string{"role:proposer", "role:", "role:A", "Role:a", "role:role:a", "proposer", ""} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "founding") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		label, ok := Selector(s)
		rest, prefixed := strings.CutPrefix(s, "role:")
		if ok != (prefixed && Label(rest)) || (ok && label != rest) {
			t.Fatalf("Selector(%q) = %q, %v", s, label, ok)
		}
	})
}

// FuzzHash holds the form of a typed hash: sha256: and 64 lowercase hex digits (canonical-3).
func FuzzHash(f *testing.F) {
	for _, s := range []string{
		"sha256:" + strings.Repeat("0a", 32), "sha256:" + strings.Repeat("0A", 32), "sha256:" + strings.Repeat("a", 63),
		"sha256:" + strings.Repeat("a", 65), "sha512:" + strings.Repeat("a", 64), strings.Repeat("a", 64), "",
	} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "receipts") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if Hash(s) != hashForm.MatchString(s) {
			t.Fatalf("Hash(%q) = %v", s, Hash(s))
		}
	})
}

// FuzzLowerHex holds the form of lowercase hex: a non-empty even number of digits 0-9 and a-f.
func FuzzLowerHex(f *testing.F) {
	for _, s := range []string{"00", "0a", "0A", "abc", "", "zz", "0123456789abcdef"} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "crossing") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if LowerHex(s) != hexForm.MatchString(s) {
			t.Fatalf("LowerHex(%q) = %v", s, LowerHex(s))
		}
	})
}

// FuzzBase64 holds standard base64's one encoding: what decodes encodes back to the same text, and
// the encoding of any bytes decodes to them.
func FuzzBase64(f *testing.F) {
	for _, s := range []string{"YQ==", "YR==", "YQ", "YQ=", "Y Q==", "YQ==\n", "", "////", "-_-_", "YWJj"} {
		f.Add(s)
	}
	for _, s := range seeds.Strings(f, "receipts") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, ok := Base64(s)
		if ok != (base64.StdEncoding.EncodeToString(b) == s && (len(b) > 0 || s == "")) || (!ok && b != nil) {
			t.Fatalf("Base64(%q) = %x, %v", s, b, ok)
		}
		if again, ok := Base64(base64.StdEncoding.EncodeToString([]byte(s))); !ok || !bytes.Equal(again, []byte(s)) {
			t.Fatalf("the encoding of %q decodes to %q, %v", s, again, ok)
		}
	})
}

// FuzzDecode reads any bytes. It refuses exactly what the canonical form refuses, with the same
// error; what it accepts comes back with its canonical bytes, and writing the value it read gives
// those bytes again (canonical-1).
func FuzzDecode(f *testing.F) {
	for _, s := range []string{
		`{}`, `{"b":[1,true,null],"a":"\u001f"}`, `{"a":1,"a":2}`, `1.5`, "\xff", `"\ud800"`, `[1e2,-0]`, ``,
		"{\"<&\u2028\":\"\U0001F600\"}", `9007199254740992`,
	} {
		f.Add([]byte(s))
	}
	for _, s := range seeds.Inputs(f, "canonical") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		v, c, err := Decode(in)
		form, ferr := canonical.Form(in)
		if ferr != nil {
			if err == nil || err.Error() != ferr.Error() || v != nil || c != nil {
				t.Fatalf("got %v, %q, %v; the form refuses with %v", v, c, err, ferr)
			}
			return
		}
		if err != nil || !bytes.Equal(c, form) || !bytes.Equal(Encode(v), form) {
			t.Fatalf("%q: got %q, %v; written again %q", in, c, err, Encode(v))
		}
	})
}

// FuzzMediaType reads any string as a media type without panicking; one it takes has one slash and
// no uppercase letter (fuzz-parsers, core-no-panic).
func FuzzMediaType(f *testing.F) {
	for _, s := range []string{"application/json", "Application/JSON", "json", "a/b/c", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if MediaType(s) && (strings.Count(s, "/") != 1 || strings.ToLower(s) != s) {
			t.Fatalf("MediaType(%q) holds", s)
		}
	})
}
