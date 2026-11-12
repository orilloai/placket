// Package canonical is the canonical form of the placket protocol: it turns JSON bytes into the one
// byte string every party hashes and signs, or refuses them with one code and one byte offset, and it
// computes typed hashes over that form.
//
// It implements canonical-1 to canonical-11: the canonical bytes (canonical-1), the value set
// (canonical-2), typed hashes (canonical-3), refusals and their codes (canonical-4), numbers by exact
// value (canonical-5), strings and UTF-8 (canonical-6), the grammar and its offsets (canonical-7),
// nesting (canonical-8), noncharacters (canonical-9), precedence among defects (canonical-10) and
// quoted spans (canonical-11).
package canonical

import (
	"sort"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// Code is a refusal code of the canonical form (canonical-4).
type Code string

// The five refusal codes, spelled as the protocol text spells them (canonical-4).
const (
	InvalidJSON      Code = "invalid-json"
	InvalidString    Code = "invalid-string"
	DuplicateKey     Code = "duplicate-key"
	FractionalNumber Code = "fractional-number"
	UnsafeInteger    Code = "unsafe-integer"
)

// Error is a refusal of the canonical form: one code and the byte offset it applies at, counted from
// the first byte of the input (canonical-4). It carries nothing of the input.
type Error struct {
	Code   Code
	Offset int
}

func (e *Error) Error() string {
	return "canonical: " + string(e.Code) + " at offset " + strconv.Itoa(e.Offset)
}

// MaxDepth is the deepest nesting of arrays and objects the canonical form accepts (canonical-8).
const MaxDepth = 1000

// maxSafe is 2^53 - 1, the largest magnitude of an integer in the value set (canonical-2).
const maxSafe = 1<<53 - 1

// Form puts input in canonical form (canonical-1), or refuses it with one code and offset by the
// table of section 1: a Unicode defect anywhere first (canonical-6, canonical-11), then the first token
// in input order with a defect, grammar before noncharacter before repeated name before number
// (canonical-10). A refusal outputs no bytes.
func Form(input []byte) ([]byte, error) {
	if off, ok := unicodeDefect(input); ok {
		return nil, &Error{Code: InvalidString, Offset: off}
	}
	p := parser{in: input}
	p.skipSpace()
	out, err := p.value(0)
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos != len(input) {
		return nil, &Error{Code: InvalidJSON, Offset: p.pos}
	}
	return out, nil
}

// unicodeDefect finds row 1's defect: the first byte of an ill-formed UTF-8 sequence, or the
// backslash of an unpaired surrogate escape inside a quoted span (canonical-11), whichever is first.
func unicodeDefect(in []byte) (int, bool) {
	bad := -1
	for i := 0; i < len(in); {
		if in[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, n := utf8.DecodeRune(in[i:])
		if r == utf8.RuneError && n == 1 {
			bad = i
			break
		}
		i += n
	}
	limit := len(in)
	if bad >= 0 {
		limit = bad
	}
	// The quoted spans, scanned from the first byte whether or not the grammar holds.
	inSpan := false
	for i := 0; i < limit; i++ {
		c := in[i]
		if !inSpan {
			if c == '"' {
				inSpan = true
			}
			continue
		}
		switch c {
		case '"':
			inSpan = false
		case '\\':
			if i+1 >= len(in) {
				i++
				continue
			}
			if in[i+1] != 'u' {
				i++
				continue
			}
			u, ok := hex4(in, i+2)
			if !ok || u < 0xD800 || u > 0xDFFF {
				i++
				continue
			}
			if u >= 0xDC00 {
				return i, true
			}
			if i+7 < len(in) && in[i+6] == '\\' && in[i+7] == 'u' {
				if l, ok := hex4(in, i+8); ok && l >= 0xDC00 && l <= 0xDFFF {
					i += 11
					continue
				}
			}
			return i, true
		}
	}
	if bad >= 0 {
		return bad, true
	}
	return 0, false
}

// hex4 reads four hex digits, of either case, at in[at:].
func hex4(in []byte, at int) (rune, bool) {
	if at+4 > len(in) {
		return 0, false
	}
	var r rune
	for _, c := range in[at : at+4] {
		d, ok := hexDigit(c)
		if !ok {
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

type parser struct {
	in  []byte
	pos int
}

func (p *parser) fail(code Code, at int) error { return &Error{Code: code, Offset: at} }

func (p *parser) skipSpace() {
	for p.pos < len(p.in) {
		switch p.in[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

// value reads one value at the current position, at the given depth of enclosing arrays and
// objects, and returns its canonical bytes.
func (p *parser) value(depth int) ([]byte, error) {
	if p.pos >= len(p.in) {
		return nil, p.fail(InvalidJSON, len(p.in))
	}
	switch c := p.in[p.pos]; {
	case c == '{':
		if depth >= MaxDepth {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		return p.object(depth + 1)
	case c == '[':
		if depth >= MaxDepth {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		return p.array(depth + 1)
	case c == '"':
		s, err := p.str()
		if err != nil {
			return nil, err
		}
		return appendString(nil, s), nil
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	case c == 't':
		return p.literal("true")
	case c == 'f':
		return p.literal("false")
	case c == 'n':
		return p.literal("null")
	}
	return nil, p.fail(InvalidJSON, p.pos)
}

func (p *parser) literal(word string) ([]byte, error) {
	for i := 0; i < len(word); i++ {
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		if p.in[p.pos] != word[i] {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		p.pos++
	}
	return []byte(word), nil
}

func (p *parser) array(depth int) ([]byte, error) {
	p.pos++ // [
	out := []byte{'['}
	p.skipSpace()
	if p.pos < len(p.in) && p.in[p.pos] == ']' {
		p.pos++
		return append(out, ']'), nil
	}
	for {
		v, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		out = append(out, v...)
		p.skipSpace()
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		switch p.in[p.pos] {
		case ',':
			p.pos++
			out = append(out, ',')
			p.skipSpace()
		case ']':
			p.pos++
			return append(out, ']'), nil
		default:
			return nil, p.fail(InvalidJSON, p.pos)
		}
	}
}

type member struct {
	name  []uint16
	bytes []byte // the canonical name and value, "name":value
}

func (p *parser) object(depth int) ([]byte, error) {
	p.pos++ // {
	p.skipSpace()
	if p.pos < len(p.in) && p.in[p.pos] == '}' {
		p.pos++
		return []byte("{}"), nil
	}
	var members []member
	seen := map[string]bool{}
	for {
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		if p.in[p.pos] != '"' {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		at := p.pos
		name, err := p.str()
		if err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, p.fail(DuplicateKey, at)
		}
		seen[name] = true
		p.skipSpace()
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		if p.in[p.pos] != ':' {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		p.pos++
		p.skipSpace()
		v, err := p.value(depth)
		if err != nil {
			return nil, err
		}
		b := appendString(nil, name)
		b = append(b, ':')
		b = append(b, v...)
		members = append(members, member{name: utf16.Encode([]rune(name)), bytes: b})
		p.skipSpace()
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		switch p.in[p.pos] {
		case ',':
			p.pos++
			p.skipSpace()
			continue
		case '}':
			p.pos++
		default:
			return nil, p.fail(InvalidJSON, p.pos)
		}
		break
	}
	sort.Slice(members, func(i, j int) bool { return less16(members[i].name, members[j].name) })
	n := 2
	for _, m := range members {
		n += len(m.bytes) + 1
	}
	out := make([]byte, 0, n)
	out = append(out, '{')
	for i, m := range members {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, m.bytes...)
	}
	return append(out, '}'), nil
}

// less16 orders two names by their UTF-16 code units (canonical-1).
func less16(a, b []uint16) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// Less reports whether a comes before b in the order of UTF-16 code units, the order of member names
// in canonical bytes (canonical-1) and of actor references (setup-contract-10).
func Less(a, b string) bool {
	return less16(utf16.Encode([]rune(a)), utf16.Encode([]rune(b)))
}

// str reads a string or member name, its escapes read, and judges its noncharacters once it closed
// (canonical-6, canonical-9). Row 1 has already found every unpaired surrogate escape.
func (p *parser) str() (string, error) {
	open := p.pos
	p.pos++
	var out []byte
	nonchar := false
	for {
		if p.pos >= len(p.in) {
			return "", p.fail(InvalidJSON, len(p.in))
		}
		c := p.in[p.pos]
		switch {
		case c == '"':
			p.pos++
			if nonchar {
				return "", p.fail(InvalidString, open)
			}
			return string(out), nil
		case c < 0x20:
			return "", p.fail(InvalidJSON, p.pos)
		case c == '\\':
			r, err := p.escape()
			if err != nil {
				return "", err
			}
			if noncharacter(r) {
				nonchar = true
			}
			out = utf8.AppendRune(out, r)
		case c < utf8.RuneSelf:
			out = append(out, c)
			p.pos++
		default:
			r, n := utf8.DecodeRune(p.in[p.pos:])
			if noncharacter(r) {
				nonchar = true
			}
			out = append(out, p.in[p.pos:p.pos+n]...)
			p.pos += n
		}
	}
}

// escape reads one escape at a backslash and returns the code point it writes.
func (p *parser) escape() (rune, error) {
	back := p.pos
	p.pos++
	if p.pos >= len(p.in) {
		return 0, p.fail(InvalidJSON, len(p.in))
	}
	c := p.in[p.pos]
	p.pos++
	switch c {
	case '"':
		return '"', nil
	case '\\':
		return '\\', nil
	case '/':
		return '/', nil
	case 'b':
		return '\b', nil
	case 'f':
		return '\f', nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case 'u':
	default:
		return 0, p.fail(InvalidJSON, p.pos-1)
	}
	u, err := p.hex()
	if err != nil {
		return 0, err
	}
	if u < 0xD800 || u > 0xDFFF {
		return u, nil
	}
	// Row 1 found no unpaired surrogate escape, so a high one is followed by a low one here; the
	// checks below keep the parser safe on its own.
	if u >= 0xDC00 || p.pos+1 >= len(p.in) || p.in[p.pos] != '\\' || p.in[p.pos+1] != 'u' {
		return 0, p.fail(InvalidString, back)
	}
	p.pos += 2
	l, err := p.hex()
	if err != nil {
		return 0, err
	}
	if l < 0xDC00 || l > 0xDFFF {
		return 0, p.fail(InvalidString, back)
	}
	return utf16.DecodeRune(u, l), nil
}

// hex reads the four hex digits of a \u escape.
func (p *parser) hex() (rune, error) {
	var r rune
	for i := 0; i < 4; i++ {
		if p.pos >= len(p.in) {
			return 0, p.fail(InvalidJSON, len(p.in))
		}
		d, ok := hexDigit(p.in[p.pos])
		if !ok {
			return 0, p.fail(InvalidJSON, p.pos)
		}
		r = r<<4 | rune(d)
		p.pos++
	}
	return r, nil
}

// noncharacter reports whether r is a Unicode noncharacter: U+FDD0 to U+FDEF, and the last two code
// points of every plane (canonical-9).
func noncharacter(r rune) bool {
	return (r >= 0xFDD0 && r <= 0xFDEF) || r&0xFFFE == 0xFFFE
}

// appendString writes s between quotes with exactly the escapes canonical-1 allows.
func appendString(out []byte, s string) []byte {
	const hexDigits = "0123456789abcdef"
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\b':
			out = append(out, '\\', 'b')
		case '\t':
			out = append(out, '\\', 't')
		case '\n':
			out = append(out, '\\', 'n')
		case '\f':
			out = append(out, '\\', 'f')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			if c < 0x20 {
				out = append(out, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
			} else {
				out = append(out, c)
			}
		}
	}
	return append(out, '"')
}

// number reads a number token and judges its exact value (canonical-2, canonical-5). The grammar is
// RFC 8259's: an optional minus, an integer part with no leading zero, an optional fraction and an
// optional exponent. The exponent's digits are read with saturation at 2^40.
func (p *parser) number() ([]byte, error) {
	start := p.pos
	neg := false
	if p.in[p.pos] == '-' {
		neg = true
		p.pos++
	}
	if p.pos >= len(p.in) {
		return nil, p.fail(InvalidJSON, len(p.in))
	}
	intStart := p.pos
	switch c := p.in[p.pos]; {
	case c == '0':
		p.pos++
	case c >= '1' && c <= '9':
		for p.pos < len(p.in) && isDigit(p.in[p.pos]) {
			p.pos++
		}
	default:
		return nil, p.fail(InvalidJSON, p.pos)
	}
	intDigits := p.in[intStart:p.pos]
	var fracDigits []byte
	if p.pos < len(p.in) && p.in[p.pos] == '.' {
		p.pos++
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		if !isDigit(p.in[p.pos]) {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		fs := p.pos
		for p.pos < len(p.in) && isDigit(p.in[p.pos]) {
			p.pos++
		}
		fracDigits = p.in[fs:p.pos]
	}
	const saturate = int64(1) << 40
	var exp int64
	if p.pos < len(p.in) && (p.in[p.pos] == 'e' || p.in[p.pos] == 'E') {
		p.pos++
		if p.pos >= len(p.in) {
			return nil, p.fail(InvalidJSON, len(p.in))
		}
		eneg := false
		if p.in[p.pos] == '+' || p.in[p.pos] == '-' {
			eneg = p.in[p.pos] == '-'
			p.pos++
			if p.pos >= len(p.in) {
				return nil, p.fail(InvalidJSON, len(p.in))
			}
		}
		if !isDigit(p.in[p.pos]) {
			return nil, p.fail(InvalidJSON, p.pos)
		}
		for p.pos < len(p.in) && isDigit(p.in[p.pos]) {
			if exp < saturate {
				exp = exp*10 + int64(p.in[p.pos]-'0')
				if exp > saturate {
					exp = saturate
				}
			}
			p.pos++
		}
		if eneg {
			exp = -exp
		}
	}

	// The significant digits, the integer part's and the fraction's, without leading or trailing
	// zeros; the value is digits * 10^e.
	digits := make([]byte, 0, len(intDigits)+len(fracDigits))
	digits = append(digits, intDigits...)
	digits = append(digits, fracDigits...)
	e := exp - int64(len(fracDigits))
	lead := 0
	for lead < len(digits) && digits[lead] == '0' {
		lead++
	}
	digits = digits[lead:]
	if len(digits) == 0 {
		return []byte("0"), nil
	}
	for digits[len(digits)-1] == '0' {
		digits = digits[:len(digits)-1]
		e++
	}
	// The magnitude is at least 10^(len-1+e) and below 10^(len+e).
	if int64(len(digits))-1+e >= 16 {
		return nil, p.fail(UnsafeInteger, start)
	}
	if e < 0 {
		return nil, p.fail(FractionalNumber, start)
	}
	var v uint64
	for _, d := range digits {
		v = v*10 + uint64(d-'0')
	}
	for ; e > 0; e-- {
		v *= 10
	}
	if v > maxSafe {
		return nil, p.fail(UnsafeInteger, start)
	}
	out := strconv.AppendUint(nil, v, 10)
	if neg {
		out = append([]byte{'-'}, out...)
	}
	return out, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
