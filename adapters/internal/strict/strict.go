// Package strict decodes one JSON object into a struct whose json tags name every member the object
// may have, as written. encoding/json alone matches a member to a field without regard to case, so
// "Hash" would be read as "hash"; every door format names its members exactly, so the adapters read
// them through Decode.
package strict

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

// Decode reads b, one JSON object and nothing after it, into v, a pointer to a struct. It refuses an
// object with a member whose name is not, exactly, the name a json tag of the struct gives.
func Decode(b []byte, v any) bool {
	t := reflect.TypeOf(v)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Struct {
		return false
	}
	names := map[string]bool{}
	for i := 0; i < t.Elem().NumField(); i++ {
		if n, _, _ := strings.Cut(t.Elem().Field(i).Tag.Get("json"), ","); n != "" && n != "-" {
			names[n] = true
		}
	}
	var members map[string]json.RawMessage
	if len(bytes.TrimLeft(b, " \t\r\n")) == 0 || bytes.TrimLeft(b, " \t\r\n")[0] != '{' || json.Unmarshal(b, &members) != nil {
		return false
	}
	for n := range members {
		if !names[n] {
			return false
		}
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return false
	}
	_, err := d.Token()
	return errors.Is(err, io.EOF)
}

// Member returns the raw value of the member of the object b named name, exactly, and whether it
// has one.
func Member(b []byte, name string) (json.RawMessage, bool) {
	var members map[string]json.RawMessage
	if json.Unmarshal(b, &members) != nil {
		return nil, false
	}
	m, ok := members[name]
	return m, ok
}
