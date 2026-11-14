package pointer_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/internal/pointer"
)

// TestParseReadsAJSONPointer reads paths as RFC 6901 writes them: the empty path is the whole
// text, every token follows a slash, ~1 is a slash and ~0 a tilde, and any other tilde, or a path
// that does not start with a slash, is not a path (commands-6).
func TestParseReadsAJSONPointer(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		path   string
		tokens []string
	}{
		{"", []string{}},
		{"/", []string{""}},
		{"/a", []string{"a"}},
		{"/a/0/b", []string{"a", "0", "b"}},
		{"/a~1b", []string{"a/b"}},
		{"/m~0n", []string{"m~n"}},
		{"/~01", []string{"~1"}},
		{"//", []string{"", ""}},
		{"/\xc3\xa9", []string{"\xc3\xa9"}},
	} {
		got, ok := pointer.Parse(c.path)
		if !ok || strings.Join(got, "\x00") != strings.Join(c.tokens, "\x00") || len(got) != len(c.tokens) {
			t.Fatalf("Parse(%q) = %q, %v, want %q", c.path, got, ok, c.tokens)
		}
	}
	for _, p := range []string{"a", "a/b", "/~", "/~2", "/a~", "/~a", " /a"} {
		if got, ok := pointer.Parse(p); ok {
			t.Fatalf("Parse(%q) = %q, true", p, got)
		}
	}
}

// value reads the value at path in b, failing the test when the path is not one.
func value(t testing.TB, b, path string) ([]byte, bool) {
	t.Helper()
	tokens, ok := pointer.Parse(path)
	if !ok {
		t.Fatalf("Parse(%q) refused", path)
	}
	return pointer.Value([]byte(b), tokens)
}

// TestValueIsTheBytesAsTheBodySpellsThem selects values and checks their bytes: exactly the span
// the body spells, from the value's first byte to its last, a string with its quotes and escapes, a
// number as written, a fraction included, an object or array with the whitespace inside it, and
// never the whitespace around it (commands-6).
func TestValueIsTheBytesAsTheBodySpellsThem(t *testing.T) {
	t.Parallel()
	body := " {\"total\": 12.50 , \"name\":\"Ann\", \"lines\": [ {\"q\":1e3}, -0, [ ] ], \"a/b\": true, \"m~n\": null," +
		" \"\\u0061\\u00e9\": \"x\\ty\", \"\": 7, \"esc\": \"\\u0041\"}\n"
	for _, c := range []struct{ path, want string }{
		{"", body[1 : len(body)-1]},
		{"/total", "12.50"},
		{"/name", `"Ann"`},
		{"/lines", `[ {"q":1e3}, -0, [ ] ]`},
		{"/lines/0", `{"q":1e3}`},
		{"/lines/0/q", "1e3"},
		{"/lines/1", "-0"},
		{"/lines/2", "[ ]"},
		{"/a~1b", "true"},
		{"/m~0n", "null"},
		{"/a\xc3\xa9", `"x\ty"`},
		{"/", "7"},
		{"/esc", `"\u0041"`},
	} {
		got, ok := value(t, body, c.path)
		if !ok || !bytes.Equal(got, []byte(c.want)) {
			t.Fatalf("value at %q = %q, %v, want %q", c.path, got, ok, c.want)
		}
	}
	for _, p := range []string{"/Total", "/total/0", "/lines/3", "/lines/01", "/lines/-", "/lines/+1", "/lines/-1", "/lines/ 0", "/name/x", "/nothing", "/lines/0/q/x", "/ab"} {
		if got, ok := value(t, body, p); ok {
			t.Fatalf("value at %q = %q, true", p, got)
		}
	}
	if got, ok := value(t, `[0,1,2,3,4,5,6,7,8,9,10]`, "/10"); !ok || string(got) != "10" {
		t.Fatalf("element 10: %q, %v", got, ok)
	}
	if got, ok := value(t, `"\ud83d\ude00"`, ""); !ok || string(got) != `"\ud83d\ude00"` {
		t.Fatalf("a surrogate pair: %q, %v", got, ok)
	}
	if got, ok := value(t, "\x09\x0a\x0d 42 \x0a", ""); !ok || string(got) != "42" {
		t.Fatalf("a number with whitespace around it: %q, %v", got, ok)
	}
}

// TestValueNeverFromABodyThatIsNotAJSONText refuses every body that is not one JSON text in
// well-formed UTF-8 with no byte order mark, nested at most 1000 deep, with no escape of an
// unpaired surrogate and no object that repeats a name once escapes are read: such a body holds no
// value at any path, not even one the defect is far from (commands-6).
func TestValueNeverFromABodyThatIsNotAJSONText(t *testing.T) {
	t.Parallel()
	for _, b := range []string{
		``,
		`hello`,
		`{"a":1} {"b":2}`,
		`{"a":1,}`,
		`{"a":01}`,
		`{"a":.5}`,
		`{"a":"\q"}`,
		"{\"a\":\"\x01\"}",
		"\xef\xbb\xbf{\"a\":1}",
		"{\"a\":\"\xff\"}",
		"{\"a\":1,\"b\":\"\xc3\"}",
		`{"a":1,"b":"\ud800"}`,
		`{"a":1,"b":"\udc00\ud800"}`,
		`{"a":1,"b":"\ud83dx"}`,
		`{"a":1,"b":"\udc00"}`,
		`{"a":1,"b":"\ud83d\u0041"}`,
		`{"a":1,"b":"\ud83d\ud83d"}`,
		`{"a":1,"b":"\ud83d\ue000"}`,
		`{"a":1,"a":2}`,
		`{"a":1,"\u0061":2}`,
		`{"a":1,"b":{"c":1,"c":1}}`,
		`{"a":1,"b":[{"c":1,"c":1}]}`,
		strings.Repeat("[", 1001) + strings.Repeat("]", 1001),
		`{"a":1,"b":` + strings.Repeat("[", 1000) + strings.Repeat("]", 1000) + `}`,
		`[{"a":1},` + strings.Repeat("[", 999) + `{}` + strings.Repeat("]", 999) + `]`,
	} {
		if got, ok := value(t, b, "/a"); ok {
			t.Fatalf("value at /a of %q = %q, true", b, got)
		}
		if got, ok := value(t, b, ""); ok {
			t.Fatalf("the whole of %q = %q, true", b, got)
		}
	}
	deep := strings.Repeat("[", 1000) + strings.Repeat("]", 1000)
	if got, ok := value(t, deep, ""); !ok || string(got) != deep {
		t.Fatalf("depth 1000 refused: %v", ok)
	}
	if got, ok := value(t, `{"a":1,"A":2,"b":"\ufdd0"}`, "/A"); !ok || string(got) != "2" {
		t.Fatalf("names that differ in case: %q, %v", got, ok)
	}
}

// FuzzValue reads any bytes at any path. It never panics, and a value it returns is a span of the
// bytes it read.
func FuzzValue(f *testing.F) {
	f.Add([]byte(`{"total": 12.50, "lines": [ {"q":1} ]}`), "/lines/0/q")
	f.Add([]byte(`{"a":1,"a":2}`), "/a")
	f.Add([]byte(`"\ud800"`), "")
	f.Add([]byte("[[[[[[[[[[1]]]]]]]]]]"), "/0/0/0")
	f.Fuzz(func(t *testing.T, b []byte, path string) {
		tokens, ok := pointer.Parse(path)
		if !ok {
			return
		}
		v, ok := pointer.Value(b, tokens)
		if ok && !bytes.Contains(b, v) {
			t.Fatalf("value %q is not inside %q", v, b)
		}
	})
}
