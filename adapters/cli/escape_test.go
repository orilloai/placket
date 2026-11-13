package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// FuzzEscape escapes any bytes as a field, as a body and as JSON. It never panics; a field holds
// only graphic runes and no space; a body holds nothing but graphic runes, the line feed, the tab
// and the space; and JSON of valid UTF-8 stays the same JSON, with nothing but graphic runes and
// JSON's white space.
func FuzzEscape(f *testing.F) {
	f.Add([]byte("a\x1b]52;c;aGk=\x07b\x1b[2J\r\n\tc\u009b1m\\d \u202ee\xff"))
	f.Add([]byte(`{"a":"x\u001b","b":["` + "\u0085\u2028\U000e0001\x7f" + `"]}`))
	f.Add([]byte("plain text, line one\nline two"))
	f.Add([]byte("1E700"))
	f.Fuzz(func(t *testing.T, b []byte) {
		fd := field(string(b))
		if !utf8.ValidString(fd) || strings.IndexFunc(fd, func(r rune) bool { return !unicode.IsGraphic(r) || r == ' ' }) >= 0 {
			t.Fatalf("field %q of %q", fd, b)
		}
		body := escape(b, "\n\t \\", false)
		if !utf8.Valid(body) || strings.IndexFunc(string(body), func(r rune) bool { return !unicode.IsGraphic(r) && r != '\n' && r != '\t' }) >= 0 {
			t.Fatalf("body %q of %q", body, b)
		}
		if !utf8.Valid(b) || !json.Valid(b) {
			return
		}
		js := escape(b, " \\\t\n\r", true)
		before, berr := decodeJSON(b)
		after, aerr := decodeJSON(js)
		if berr != nil || aerr != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("JSON %q of %q", js, b)
		}
		if strings.IndexFunc(string(js), func(r rune) bool { return !unicode.IsGraphic(r) && !strings.ContainsRune("\t\n\r", r) }) >= 0 {
			t.Fatalf("JSON %q of %q", js, b)
		}
	})
}

// decodeJSON reads one JSON value, its numbers as their text, so that a number too large for a
// float64 reads too.
func decodeJSON(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	err := d.Decode(&v)
	return v, err
}

// TestFieldsAndBodiesEscape spells out each escape, as a field, as a body and as JSON.
func TestFieldsAndBodiesEscape(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, field, body string }{
		{"a b", `a\x20b`, "a b"},
		{`a\b`, `a\\b`, `a\b`},
		{"a\nb\tc\rd", `a\x0ab\x09c\x0dd`, "a\nb\tc" + `\x0d` + "d"},
		{"\x1b[2J\x7f", `\x1b[2J\x7f`, `\x1b[2J\x7f`},
		{"\u009b\u202e", `\u009b\u202e`, `\u009b\u202e`},
		{"\U000e0001", `\U000e0001`, `\U000e0001`},
		{"\xff\xc3", `\xff\xc3`, `\xff\xc3`},
		{"\u00e9 \u2713", "\u00e9" + `\x20` + "\u2713", "\u00e9 \u2713"},
	}
	for _, c := range cases {
		if got := field(c.in); got != c.field {
			t.Fatalf("field %q: got %q, want %q", c.in, got, c.field)
		}
		if got := string(escape([]byte(c.in), "\n\t \\", false)); got != c.body {
			t.Fatalf("body %q: got %q, want %q", c.in, got, c.body)
		}
	}
	in := `{"a":"` + "\x7f\U000e0001 \u0085" + `\\"}`
	if got := string(escape([]byte(in), " \\\t\n\r", true)); got != `{"a":"\u007f\udb40\udc01 \u0085\\"}` {
		t.Fatalf("JSON: got %s", got)
	}
}
