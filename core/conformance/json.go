package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"unicode/utf8"
)

// maxDepth is the deepest nesting the reader accepts, the limit of canonical-8.
const maxDepth = 1000

// object is a JSON object as read: its member names in input order, and their values.
type object struct {
	names  []string
	values map[string]any
}

// number is a JSON number, kept as the text it was written in so that no value is rounded.
type number string

// read reads b as one JSON value, more strictly than encoding/json does: b must be well-formed
// UTF-8, hold exactly one value, nest no deeper than maxDepth, and repeat no member name in any
// object, a name compared after its escapes are read. The values are nil, bool, number, string,
// []any and *object.
//
// It reads a subset of what the canonical form refuses (canonical-4, canonical-6, canonical-7,
// canonical-8). The canonical form's other refusals come with its package, which the header check
// will use once it exists.
func read(b []byte) (any, error) {
	if !utf8.Valid(b) {
		at := 0
		for at < len(b) {
			r, n := utf8.DecodeRune(b[at:])
			if r == utf8.RuneError && n <= 1 {
				break
			}
			at += n
		}
		return nil, errors.New("invalid UTF-8 at byte " + strconv.Itoa(at))
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, err := readValue(d, b, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("data after the value, at byte " + strconv.FormatInt(d.InputOffset(), 10))
	}
	return v, nil
}

func readValue(d *json.Decoder, b []byte, depth int) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, notJSON(err)
	}
	switch t := t.(type) {
	case json.Delim:
		if depth == maxDepth {
			return nil, errors.New("nesting deeper than " + strconv.Itoa(maxDepth) + " levels, at byte " + strconv.FormatInt(d.InputOffset()-1, 10))
		}
		if t == '[' {
			list := []any{}
			for d.More() {
				v, err := readValue(d, b, depth+1)
				if err != nil {
					return nil, err
				}
				list = append(list, v)
			}
			if _, err := d.Token(); err != nil {
				return nil, notJSON(err)
			}
			return list, nil
		}
		o := &object{values: map[string]any{}}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return nil, notJSON(err)
			}
			name, ok := k.(string)
			if !ok {
				return nil, errors.New("a member name that is not a string, at byte " + strconv.FormatInt(d.InputOffset(), 10))
			}
			if _, seen := o.values[name]; seen {
				return nil, errors.New("the name " + strconv.Quote(name) + " repeated in one object, at byte " + strconv.Itoa(openingQuote(b, int(d.InputOffset()))))
			}
			v, err := readValue(d, b, depth+1)
			if err != nil {
				return nil, err
			}
			o.names = append(o.names, name)
			o.values[name] = v
		}
		if _, err := d.Token(); err != nil {
			return nil, notJSON(err)
		}
		return o, nil
	case json.Number:
		return number(t), nil
	default:
		return t, nil
	}
}

// notJSON words a decoder's error by where it stopped, not by the decoder's own text, which a new
// Go release may change.
func notJSON(err error) error {
	var syntax *json.SyntaxError
	switch {
	case err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("not JSON: the input ends too soon")
	case errors.As(err, &syntax):
		return errors.New("not JSON, at byte " + strconv.FormatInt(syntax.Offset, 10))
	}
	return errors.New("not JSON")
}

// openingQuote returns the offset of the quote that opens the string whose closing quote ends just
// before end: the nearest quote before it that no backslash escapes.
func openingQuote(b []byte, end int) int {
	for i := end - 2; i >= 0; i-- {
		if b[i] != '"' {
			continue
		}
		n := 0
		for j := i - 1; j >= 0 && b[j] == '\\'; j-- {
			n++
		}
		if n%2 == 0 {
			return i
		}
	}
	return 0
}
