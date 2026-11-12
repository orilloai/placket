package conformance

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/orilloai/placket/core/canonical"
)

// registered holds the handler of each operation the core can run. Each handler's file adds its own
// as the package starts, so a handler lands with the code it calls.
var registered = Handlers{}

// Default returns the handlers of the operations the core can run: every operation of
// conformance-vectors-4, and every step of a scenario, give, get, resolve and tick
// (conformance-vectors-3). A step whose op the vector format does not have is reported pending,
// which fails its vector.
func Default() Handlers {
	out := Handlers{}
	for op, h := range registered {
		out[op] = h
	}
	return out
}

// members reads a JSON object's members as raw bytes, as written.
type members map[string]json.RawMessage

func membersOf(b []byte) (members, bool) {
	var m members
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return nil, false
	}
	return m, true
}

func (m members) str(name string) string {
	var s string
	_ = json.Unmarshal(m[name], &s)
	return s
}

func (m members) has(name string) bool {
	_, ok := m[name]
	return ok
}

// bytesOf reads a byte input given in one of its forms (conformance-vectors-8): a JSON value, whose
// bytes are its canonical form; text, whose bytes are its UTF-8; or hex.
func bytesOf(m members, value, text, hexName string) ([]byte, bool) {
	switch {
	case m.has(value):
		b, err := canonical.Form(m[value])
		return b, err == nil
	case m.has(text):
		var s string
		if json.Unmarshal(m[text], &s) != nil {
			return nil, false
		}
		return []byte(s), true
	case m.has(hexName):
		b, err := hex.DecodeString(m.str(hexName))
		return b, err == nil
	}
	return nil, false
}

func inputOf(m members) ([]byte, bool) { return bytesOf(m, "input", "inputText", "inputHex") }

// same compares two JSON values exactly, by their canonical bytes.
func same(a, b []byte) bool {
	ca, errA := canonical.Form(a)
	cb, errB := canonical.Form(b)
	return errA == nil && errB == nil && bytes.Equal(ca, cb)
}

func showBytes(b []byte) string {
	c, err := canonical.Form(b)
	if err != nil {
		return strconv.Quote(string(b))
	}
	return string(c)
}

// compare is one part: matched when got and want are the same JSON value.
func compare(name string, got, want []byte) Part {
	if same(got, want) {
		return Part{Name: name}
	}
	return Part{Name: name, Failure: "got " + showBytes(got) + ", want " + showBytes(want)}
}

func jsonOf(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func bad(name, why string) []Part { return []Part{{Name: name, Failure: why}} }
