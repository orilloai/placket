// Package pointer reads the value at a path inside a body, for released --at (commands-6). A path is
// a JSON Pointer (RFC 6901). A body holds values only when it is one JSON text (RFC 8259) in
// well-formed UTF-8 with no byte order mark, nested at most 1000 deep, with no escape of an unpaired
// surrogate and no object that holds two members of one name once their escapes are read. Numbers
// are read by the grammar alone, never by their value, so a fraction is a value like any other. The
// bytes of a value are the body's own bytes from the value's first byte to its last: Placket
// normalises nothing, and a caller compares those bytes.
package pointer

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// maxDepth is the deepest an array or object may nest, as in canonical-8.
const maxDepth = 1000

// Parse reads the path p into its reference tokens: none for the empty path, the whole body; else
// one after each slash, with ~1 read as a slash and ~0 as a tilde. It reports false for a path that
// does not start with a slash, or with a tilde before anything but 0 or 1.
func Parse(p string) ([]string, bool) {
	if p == "" {
		return []string{}, true
	}
	if p[0] != '/' {
		return nil, false
	}
	parts := strings.Split(p[1:], "/")
	for i, part := range parts {
		var b strings.Builder
		for j := 0; j < len(part); j++ {
			if part[j] != '~' {
				b.WriteByte(part[j])
				continue
			}
			if j+1 == len(part) {
				return nil, false
			}
			switch part[j+1] {
			case '0':
				b.WriteByte('~')
			case '1':
				b.WriteByte('/')
			default:
				return nil, false
			}
			j++
		}
		parts[i] = b.String()
	}
	return parts, true
}

// Value returns a copy of the bytes of the value the tokens select in the body b, as b spells
// them, and whether b holds one there. A body that is not a JSON text of the package comment holds
// no value at any path.
func Value(b []byte, tokens []string) ([]byte, bool) {
	if !utf8.Valid(b) {
		return nil, false
	}
	r := &reader{b: b, tokens: tokens}
	r.space()
	if !r.value(0, true, 0) {
		return nil, false
	}
	r.space()
	if r.i != len(b) || !r.found {
		return nil, false
	}
	return append([]byte(nil), b[r.start:r.end]...), true
}

// A reader reads one JSON text, and keeps the span of the value at its tokens when it meets it.
type reader struct {
	b          []byte
	i          int
	tokens     []string
	start, end int
	found      bool
}

// peek is the next byte, or 0 at the end.
func (r *reader) peek() byte {
	if r.i < len(r.b) {
		return r.b[r.i]
	}
	return 0
}

// space skips JSON whitespace.
func (r *reader) space() {
	for r.i < len(r.b) {
		switch r.b[r.i] {
		case ' ', '\t', '\n', '\r':
			r.i++
		default:
			return
		}
	}
}

// value reads one value inside depth levels of nesting. on says the value lies on the path, its
// first n tokens matched; when all of them did, its span is the result.
func (r *reader) value(depth int, on bool, n int) bool {
	start := r.i
	var ok bool
	switch c := r.peek(); {
	case c == '{':
		ok = r.object(depth+1, on, n)
	case c == '[':
		ok = r.array(depth+1, on, n)
	case c == '"':
		_, ok = r.str()
	case c == '-' || isDigit(c):
		ok = r.number()
	case c == 't':
		ok = r.literal("true")
	case c == 'f':
		ok = r.literal("false")
	case c == 'n':
		ok = r.literal("null")
	}
	if ok && on && n == len(r.tokens) {
		r.start, r.end, r.found = start, r.i, true
	}
	return ok
}

// next reports whether the token after the first n is tok.
func (r *reader) next(on bool, n int, tok string) bool {
	return on && n < len(r.tokens) && r.tokens[n] == tok
}

func (r *reader) object(depth int, on bool, n int) bool {
	if depth > maxDepth {
		return false
	}
	r.i++
	r.space()
	if r.peek() == '}' {
		r.i++
		return true
	}
	names := map[string]bool{}
	for {
		r.space()
		if r.peek() != '"' {
			return false
		}
		name, ok := r.str()
		if !ok || names[name] {
			return false
		}
		names[name] = true
		r.space()
		if r.peek() != ':' {
			return false
		}
		r.i++
		r.space()
		if !r.value(depth, r.next(on, n, name), n+1) {
			return false
		}
		r.space()
		switch r.peek() {
		case ',':
			r.i++
		case '}':
			r.i++
			return true
		default:
			return false
		}
	}
}

func (r *reader) array(depth int, on bool, n int) bool {
	if depth > maxDepth {
		return false
	}
	r.i++
	r.space()
	if r.peek() == ']' {
		r.i++
		return true
	}
	for index := 0; ; index++ {
		r.space()
		if !r.value(depth, r.next(on, n, strconv.Itoa(index)), n+1) {
			return false
		}
		r.space()
		switch r.peek() {
		case ',':
			r.i++
		case ']':
			r.i++
			return true
		default:
			return false
		}
	}
}

// str reads a string and returns it with its escapes read. It refuses a raw control character, an
// escape RFC 8259 does not allow, and an escape of an unpaired surrogate.
func (r *reader) str() (string, bool) {
	r.i++
	var b strings.Builder
	for r.i < len(r.b) {
		c := r.b[r.i]
		switch {
		case c == '"':
			r.i++
			return b.String(), true
		case c < 0x20:
			return "", false
		case c != '\\':
			b.WriteByte(c)
			r.i++
			continue
		}
		r.i++
		switch e := r.peek(); e {
		case '"', '\\', '/':
			b.WriteByte(e)
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			u, ok := r.hex4()
			if !ok {
				return "", false
			}
			switch {
			case utf16.IsSurrogate(rune(u)) && u < 0xdc00:
				if r.i+2 >= len(r.b) || r.b[r.i+1] != '\\' || r.b[r.i+2] != 'u' {
					return "", false
				}
				r.i += 2
				low, ok := r.hex4()
				if !ok || low < 0xdc00 || low > 0xdfff {
					return "", false
				}
				b.WriteRune(utf16.DecodeRune(rune(u), rune(low)))
			case utf16.IsSurrogate(rune(u)):
				return "", false
			default:
				b.WriteRune(rune(u))
			}
		default:
			return "", false
		}
		r.i++
	}
	return "", false
}

// hex4 reads the four hex digits after a \u, leaving the reader on the last of them.
func (r *reader) hex4() (uint16, bool) {
	if r.i+4 >= len(r.b) {
		return 0, false
	}
	v, err := strconv.ParseUint(string(r.b[r.i+1:r.i+5]), 16, 16)
	if err != nil {
		return 0, false
	}
	r.i += 4
	return uint16(v), true
}

// number reads a number by RFC 8259's grammar.
func (r *reader) number() bool {
	if r.peek() == '-' {
		r.i++
	}
	switch c := r.peek(); {
	case c == '0':
		r.i++
	case isDigit(c):
		r.digits()
	default:
		return false
	}
	if r.peek() == '.' {
		r.i++
		if !isDigit(r.peek()) {
			return false
		}
		r.digits()
	}
	if c := r.peek(); c == 'e' || c == 'E' {
		r.i++
		if c := r.peek(); c == '+' || c == '-' {
			r.i++
		}
		if !isDigit(r.peek()) {
			return false
		}
		r.digits()
	}
	return true
}

func (r *reader) digits() {
	for isDigit(r.peek()) {
		r.i++
	}
}

func (r *reader) literal(word string) bool {
	if !bytes.HasPrefix(r.b[r.i:], []byte(word)) {
		return false
	}
	r.i += len(word)
	return true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
