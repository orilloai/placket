package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// What another party wrote reaches a person's terminal in two ways: as the fields of get's line on
// standard error, which are always escaped, and as a body, an answer, a question or a log on standard
// output, which are escaped only when standard output is a terminal, so that a pipe or a file gets
// the exact bytes. Escaped, a byte that is not UTF-8 is \xNN, and so is an ASCII control character
// or, in a field, a space; a backslash in a field is \\; any other rune that prints nothing, such as
// a C1 control or a direction override, is \uNNNN. A terminal is never handed a control sequence of
// another party's to act on.

// isTerminal is Env.Terminal's default: whether w is a file that is a character device.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// escape writes b with every rune that is not graphic escaped, but the ASCII bytes in keep; with
// asJSON, b is JSON and every escape is \uNNNN, so that it stays the same JSON.
func escape(b []byte, keep string, asJSON bool) []byte {
	var out strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		switch {
		case r == utf8.RuneError && n <= 1:
			fmt.Fprintf(&out, `\x%02x`, b[0])
		case r < utf8.RuneSelf && strings.ContainsRune(keep, r):
			out.WriteRune(r)
		case r == '\\':
			out.WriteString(`\\`)
		case (r < 0x20 || r == 0x7f || r == ' ') && !asJSON:
			fmt.Fprintf(&out, `\x%02x`, r)
		case r < 0x20 || r == 0x7f || !unicode.IsGraphic(r):
			switch {
			case r > 0xffff && asJSON:
				r1, r2 := utf16.EncodeRune(r)
				fmt.Fprintf(&out, `\u%04x\u%04x`, r1, r2)
			case r > 0xffff:
				fmt.Fprintf(&out, `\U%08x`, r)
			default:
				fmt.Fprintf(&out, `\u%04x`, r)
			}
		default:
			out.Write(b[:n])
		}
		b = b[n:]
	}
	return []byte(out.String())
}

// field is one field of a line on standard error: no control character, space or backslash of its
// own, so it can neither end the line nor pose as another field; a backslash is \\.
func field(s string) string { return string(escape([]byte(s), "", false)) }

// screen is what standard output is handed of another party's bytes: the bytes themselves, or, on a
// terminal, the bytes escaped but for the line feed, the tab, the space and the backslash.
func (r *run) screen(b []byte) []byte {
	if !r.env.Terminal(r.stdout) {
		return b
	}
	return escape(b, "\n\t \\", false)
}

// screenJSON is screen for a line of JSON, which it leaves the same JSON: the tab, the line feed and
// the carriage return can only be JSON's white space between values, since a string holds none.
func (r *run) screenJSON(b []byte) []byte {
	if !r.env.Terminal(r.stdout) {
		return b
	}
	return escape(b, " \\\t\n\r", true)
}
