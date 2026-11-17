package cli

import (
	"encoding/json"
	"testing"

	"github.com/orilloai/placket/adapters/internal/seeds"
)

// FuzzReadTerms reads any bytes as a terms file. It never panics, and what it accepts is one object
// whose only member, terms, is a list.
func FuzzReadTerms(f *testing.F) {
	f.Add([]byte(`{"terms":[]}`))
	f.Add([]byte(`{"terms":[{"stance":"want"}],"actor":"x"}`))
	f.Add([]byte(`{"terms":{}}`))
	for _, in := range seeds.Inputs(f, "founding", "declarations") {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		terms, ok := readTerms(b)
		if !ok {
			return
		}
		var l []json.RawMessage
		if json.Unmarshal(terms, &l) != nil {
			t.Fatalf("accepted %q", b)
		}
	})
}
