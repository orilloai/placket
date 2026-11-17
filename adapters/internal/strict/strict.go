// Package strict decodes one JSON object into a struct whose json tags name every member the object
// may have, as written. encoding/json alone keeps the last of two members of one name, and matches a
// member to a field without regard to case, so "Hash" would be read as "hash" and two readers could
// see two values in one object (parse-strict). Every door format names its members exactly, so the
// adapters read them through Decode.
package strict

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/orilloai/placket/core/canonical"
)

// Decode reads b, one JSON object and nothing after it, into v, a pointer to a struct or to a map
// keyed by string. It refuses bytes the canonical form refuses (a repeated member, ill-formed UTF-8,
// an unsafe number), and an object, at any depth a struct describes, with a member whose name is not,
// exactly, the name a json tag of the struct gives.
func Decode(b []byte, v any) bool {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Pointer {
		return false
	}
	if k := t.Elem().Kind(); k != reflect.Struct && (k != reflect.Map || t.Elem().Key().Kind() != reflect.String) {
		return false
	}
	if len(bytes.TrimLeft(b, " \t\r\n")) == 0 || bytes.TrimLeft(b, " \t\r\n")[0] != '{' {
		return false
	}
	if _, err := canonical.Form(b); err != nil || !exact(b, t.Elem()) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return false
	}
	_, err := d.Token()
	return errors.Is(err, io.EOF)
}

// exact reports whether every object in raw that t describes as a struct names only members its json
// tags name, exactly. A value of another shape than t is left to the decoder to refuse.
func exact(raw json.RawMessage, t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		var members map[string]json.RawMessage
		if json.Unmarshal(raw, &members) != nil {
			return true
		}
		fields := map[string]reflect.Type{}
		tagged(t, fields)
		for n, m := range members {
			ft, ok := fields[n]
			if !ok || !exact(m, ft) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return true
		}
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return true
		}
		for _, m := range items {
			if !exact(m, t.Elem()) {
				return false
			}
		}
	case reflect.Map:
		var members map[string]json.RawMessage
		if json.Unmarshal(raw, &members) != nil {
			return true
		}
		for _, m := range members {
			if !exact(m, t.Elem()) {
				return false
			}
		}
	}
	return true
}

// tagged adds to fields the member name of every field of the struct t that a json tag names, with
// the fields of an embedded struct with no tag, as encoding/json reads them.
func tagged(t reflect.Type, fields map[string]reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		n, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		switch {
		case n == "-":
		case n != "":
			fields[n] = f.Type
		case f.Anonymous && f.Type.Kind() == reflect.Struct:
			tagged(f.Type, fields)
		}
	}
}

// Member returns the raw value of the member of the object b named name, exactly, and whether it
// has one. Bytes the canonical form refuses, a repeated member among them, have no member.
func Member(b []byte, name string) (json.RawMessage, bool) {
	if _, err := canonical.Form(b); err != nil {
		return nil, false
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(b, &members) != nil {
		return nil, false
	}
	m, ok := members[name]
	return m, ok
}
