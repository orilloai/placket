// Package shape holds the format rules every capability package reads values by: the timestamp rule,
// labels, kinds and role selectors, typed hashes and lowercase hex, standard base64 in its one
// encoding, and the reading and writing of JSON values through the canonical form. It decides no
// result of its own: each caller turns a value that fails a rule into the code of the check that read
// it. Decision record 0005 says why it exists.
package shape

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/orilloai/placket/core/canonical"
)

// MaxSafe is 2^53 - 1, the largest whole number of the value set (canonical-2).
const MaxSafe = 1<<53 - 1

// LastTime is the latest time the timestamp rule can write (maybe-1).
const LastTime = "9999-12-31T23:59:59Z"

const layout = "2006-01-02T15:04:05Z"

// Timestamp reports whether s keeps the one timestamp rule: YYYY-MM-DDTHH:MM:SSZ, a date the
// Gregorian calendar has, a time of day from 00:00:00 to 23:59:59, UTC, no leap second.
func Timestamp(s string) bool {
	if len(s) != len(layout) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		case 10:
			if c != 'T' {
				return false
			}
		case 13, 16:
			if c != ':' {
				return false
			}
		case 19:
			if c != 'Z' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	t, err := time.Parse(layout, s)
	return err == nil && t.Format(layout) == s
}

// AddSeconds returns the timestamp n seconds after ts; a time past LastTime is LastTime (maybe-1). A
// ts that does not keep the timestamp rule, or a negative n, which no timeout is, gives the empty
// string.
func AddSeconds(ts string, n int64) string {
	if !Timestamp(ts) || n < 0 {
		return ""
	}
	t, _ := time.Parse(layout, ts)
	last, _ := time.Parse(layout, LastTime)
	if n >= last.Unix()-t.Unix() {
		return LastTime
	}
	return time.Unix(t.Unix()+n, 0).UTC().Format(layout)
}

// Label reports whether s is a label or a kind: lowercase letters and digits in words joined by
// single hyphens, at most 64 characters (setup-contract-1).
func Label(s string) bool { return len(s) <= 64 && words(s, "-") }

// Gate reports whether s has the reserved gate's form: lowercase letters and digits in words joined
// by single hyphens or underscores, at most 64 characters (crossing-1).
func Gate(s string) bool { return len(s) <= 64 && words(s, "-_") }

func words(s, joins string) bool {
	if s == "" {
		return false
	}
	prevJoin := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			prevJoin = false
		case strings.IndexByte(joins, c) >= 0:
			if prevJoin {
				return false
			}
			prevJoin = true
		default:
			return false
		}
	}
	return !prevJoin
}

// Selector reports whether s is a role selector, role:<label> (setup-contract-1), and returns the
// label.
func Selector(s string) (string, bool) {
	label, ok := strings.CutPrefix(s, "role:")
	return label, ok && Label(label)
}

// Hash reports whether s has the form of a typed hash: sha256: and 64 lowercase hex (canonical-3).
func Hash(s string) bool {
	h, ok := strings.CutPrefix(s, canonical.HashPrefix)
	return ok && len(h) == 64 && LowerHex(h)
}

// LowerHex reports whether s is a non-empty even number of lowercase hex digits.
func LowerHex(s string) bool {
	if s == "" || len(s)%2 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Base64 decodes s as standard base64 with padding, in the one encoding that decodes and encodes back
// to the same text.
func Base64(s string) ([]byte, bool) {
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || base64.StdEncoding.EncodeToString(b) != s {
		return nil, false
	}
	return b, true
}

// An Object is a JSON object as Decode reads it.
type Object = map[string]any

// Decode reads bytes the canonical form accepts into JSON values: Object, []any, string, json.Number,
// bool and nil. It refuses what the canonical form refuses, with the canonical form's error.
func Decode(b []byte) (any, []byte, error) {
	c, err := canonical.Form(b)
	if err != nil {
		return nil, nil, err
	}
	d := json.NewDecoder(bytes.NewReader(c))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		// Canonical bytes always decode; this is not reachable from any input.
		return nil, nil, &canonical.Error{Code: canonical.InvalidJSON, Offset: 0}
	}
	return v, c, nil
}

// Encode writes a JSON value, of the types Decode returns and int64, in canonical form.
func Encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	c, err := canonical.Form(b)
	if err != nil {
		return nil
	}
	return c
}

// Int reads v as a whole number from 1 to 2^53 - 1.
func Int(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	if err != nil || i < 1 || i > MaxSafe {
		return 0, false
	}
	return i, true
}

// Only reports whether o holds no member but the names given, and all of the required ones, which
// are the names given before the first empty string.
func Only(o Object, names ...string) bool {
	required := true
	allowed := 0
	for _, n := range names {
		if n == "" {
			required = false
			continue
		}
		allowed++
		if _, ok := o[n]; required && !ok {
			return false
		}
	}
	for k := range o {
		found := false
		for _, n := range names {
			if n != "" && n == k {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Strings reads v as a non-empty list of distinct strings that each pass ok.
func Strings(v any, ok func(string) bool) ([]string, bool) {
	l, isList := v.([]any)
	if !isList || len(l) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(l))
	for _, x := range l {
		s, isString := x.(string)
		if !isString || !ok(s) || seen[s] {
			return nil, false
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, true
}
