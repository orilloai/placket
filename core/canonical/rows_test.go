package canonical_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
)

// TestRefusalsAtTheirOffsets holds refusals each of which one check alone decides, with the exact code
// and offset: whitespace is space, tab, line feed and carriage return only (canonical-7); a raw
// control character in a string (canonical-6); a lone low surrogate escape, found by row 1 before a
// grammar error earlier in the input (canonical-10, canonical-11); objects nested deeper than the
// limit (canonical-8); the last noncharacter of U+FDD0 to U+FDEF (canonical-9); and, between an
// ill-formed UTF-8 sequence and an unpaired surrogate escape, whichever comes first (canonical-10).
func TestRefusalsAtTheirOffsets(t *testing.T) {
	t.Parallel()
	deep := strings.Repeat(`{"a":`, canonical.MaxDepth+1) + "1" + strings.Repeat("}", canonical.MaxDepth+1)
	for _, c := range []struct {
		name string
		in   string
		code canonical.Code
		at   int
	}{
		{"a form feed between tokens", "[1,\x0c2]", canonical.InvalidJSON, 3},
		{"a form feed before a value", "\x0c1", canonical.InvalidJSON, 0},
		{"a form feed after a value", "1\x0c", canonical.InvalidJSON, 1},
		{"a vertical tab before a value", "\x0b1", canonical.InvalidJSON, 0},
		{"a raw 0x1f in a string", "[\"a\x1f\"]", canonical.InvalidJSON, 3},
		{"a raw 0x00 in a string", "\"\x00\"", canonical.InvalidJSON, 1},
		{"a lone low surrogate after a grammar error", `[1 2,"\udc00"]`, canonical.InvalidString, 6},
		{"a lone low surrogate before a high one", `"\udc00\ud800"`, canonical.InvalidString, 1},
		{"a lone low surrogate at the end of the range", `[1 2,"\udfff"]`, canonical.InvalidString, 6},
		{"two low surrogates after a grammar error", `[1 2,"\udc00\udfff"]`, canonical.InvalidString, 6},
		{"objects one deeper than the limit", deep, canonical.InvalidJSON, 5 * canonical.MaxDepth},
		{"U+FDEF escaped", `"\ufdef"`, canonical.InvalidString, 0},
		{"U+FDEF as UTF-8", "[\"\xef\xb7\xaf\"]", canonical.InvalidString, 1},
		{"ill-formed UTF-8 before an unpaired surrogate escape", "\"\xff\\ud800\"", canonical.InvalidString, 1},
		{"an unpaired surrogate escape before ill-formed UTF-8", "\"\\ud800\xff\"", canonical.InvalidString, 1},
		{"ill-formed UTF-8 outside a string before an unpaired surrogate escape", "[\xff,\"\\udc00\"]", canonical.InvalidString, 1},
		{"an unpaired surrogate escape before ill-formed UTF-8 outside a string", "[\"\\udc00\",\xff]", canonical.InvalidString, 2},
	} {
		out, err := canonical.Form([]byte(c.in))
		var ce *canonical.Error
		if out != nil || !errors.As(err, &ce) || ce.Code != c.code || ce.Offset != c.at {
			t.Fatalf("%s: got %q, %v; want %s at offset %d", c.name, out, err, c.code, c.at)
		}
	}
}

// TestAcceptedAtTheBoundaries holds the values just inside the checks above, and their canonical
// bytes: the four whitespace characters, the first character a string may hold raw, the code points
// beside U+FDD0 to U+FDEF and the surrogates, and objects nested to the limit (canonical-6 to
// canonical-9).
func TestAcceptedAtTheBoundaries(t *testing.T) {
	t.Parallel()
	limit := strings.Repeat(`{"a":`, canonical.MaxDepth) + "1" + strings.Repeat("}", canonical.MaxDepth)
	for _, c := range []struct {
		name    string
		in, out string
	}{
		{"the four whitespace characters", " \t\n\r[ \t\n\r1 \t\n\r] \t\n\r", "[1]"},
		{"a raw space and a raw delete", "\"\x20\x7f\"", "\"\x20\x7f\""},
		{"U+FDCF and U+FDF0", `"\ufdcf\ufdf0"`, "\"\xef\xb7\x8f\xef\xb7\xb0\""},
		{"U+D7FF and U+E000", `"\ud7ff\ue000"`, "\"\xed\x9f\xbf\xee\x80\x80\""},
		{"a surrogate pair", `"\ud83d\ude00"`, "\"\xf0\x9f\x98\x80\""},
		{"objects nested to the limit", limit, limit},
	} {
		out, err := canonical.Form([]byte(c.in))
		if err != nil || !bytes.Equal(out, []byte(c.out)) {
			t.Fatalf("%s: got %q, %v; want %q", c.name, out, err, c.out)
		}
	}
}

// TestControlEscapesInLowercase writes every control character canonical-1 escapes as \u00XX, given
// raw-escaped in either case: each comes out with lowercase hex digits, and the five with short
// escapes come out short (canonical-1).
func TestControlEscapesInLowercase(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, out string }{
		{`"\u000B\u000E\u000F\u001A\u001B\u001C\u001D\u001E\u001F"`, `"\u000b\u000e\u000f\u001a\u001b\u001c\u001d\u001e\u001f"`},
		{`"\u000b\u000e\u000f\u001a\u001b\u001c\u001d\u001e\u001f"`, `"\u000b\u000e\u000f\u001a\u001b\u001c\u001d\u001e\u001f"`},
		{`"\u0000\u0001\u0007\u0010\u007f"`, "\"\\u0000\\u0001\\u0007\\u0010\x7f\""},
		{`"\u0008\u0009\u000A\u000C\u000D"`, `"\b\t\n\f\r"`},
		{`{"\u001F":"\u001A"}`, `{"\u001f":"\u001a"}`},
	} {
		out, err := canonical.Form([]byte(c.in))
		if err != nil || !bytes.Equal(out, []byte(c.out)) {
			t.Fatalf("%s: got %q, %v; want %s", c.in, out, err, c.out)
		}
	}
}
