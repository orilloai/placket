package conformance

import (
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, want string }{
		{`{"a":[1,"x",true,null,{}]}`, `{"a":[1,"x",true,null,{}]}`},
		{` 1.50 `, `1.50`},
		{`{"b":1,"a":2}`, `{"a":2,"b":1}`},
		{`{"a":1,"b":{"a":2}}`, `{"a":1,"b":{"a":2}}`},
		{`"\u00e9"`, `"é"`},
		{``, `not JSON: the input ends too soon`},
		{`[1,`, `not JSON, at byte 3`},
		{`{"a"`, `not JSON: the input ends too soon`},
		{`{"a":1}{}`, `data after the value, at byte 8`},
		{`1 2`, `data after the value, at byte 3`},
		{`{"a":1,"a":2}`, `the name "a" repeated in one object, at byte 7`},
		{`{"a":1,"\u0061":2}`, `the name "a" repeated in one object, at byte 7`},
		{`{"x":{"a\"":1,"a\"":2}}`, `the name "a\"" repeated in one object, at byte 14`},
		{`[{"a":1},{"a":1,"a":1}]`, `the name "a" repeated in one object, at byte 16`},
		{`{"a":1,"A":2}`, `{"A":2,"a":1}`},
		{"\"\xff\"", `invalid UTF-8 at byte 1`},
		{"\xef\xbb\xbf{}", `not JSON, at byte 3`},
		{`{1:2}`, `not JSON, at byte 2`},
		{`[1,]`, `not JSON, at byte 3`},
		{strings.Repeat("[", 1000) + strings.Repeat("]", 1000), strings.Repeat("[", 1000) + strings.Repeat("]", 1000)},
		{strings.Repeat("[", 1001) + strings.Repeat("]", 1001), `nesting deeper than 1000 levels, at byte 1000`},
	} {
		got := ""
		v, err := read([]byte(c.in))
		if err != nil {
			got = err.Error()
		} else {
			got = show(v)
		}
		if got != c.want {
			t.Errorf("read(%q):\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}

// FuzzRead checks that no input makes the reader stop, and that what it reads it reads again the
// same way once written back.
func FuzzRead(f *testing.F) {
	for _, s := range []string{`{}`, `{"a":[1,2,{"b":null}]}`, `{"a":1,"a":2}`, "\"\xff\"", `[[[[`, `-0.0e-0`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		v, err := read(b)
		if err != nil {
			return
		}
		again, err := read([]byte(show(v)))
		if err != nil {
			t.Fatalf("%q read, but its value written back does not: %v", b, err)
		}
		if !equal(v, again) {
			t.Fatalf("%q read differently once written back", b)
		}
	})
}
