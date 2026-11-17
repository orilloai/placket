// Package seeds reads the vectors for the seeds of the adapters' fuzz tests (fuzz-parsers), as the
// core's own helper does for the core, which the adapters cannot import. Only tests import it. Past
// this comment it is core/internal/testing/seeds/seeds.go byte for byte, which a test checks: a fix
// to one is made to both.
package seeds

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Inputs returns the inputs the vectors of vectors/<group>/ hand to an implementation, as bytes, for
// a fuzz test's seeds (fuzz-parsers): every member named input, message, envelope, receipt or
// founding as its JSON is written, every member whose name ends in Hex as the bytes its hex spells,
// and every member whose name ends in Text as its string's bytes, at any depth. Given names, it
// keeps only the members of those names, as their JSON is written, and of those names followed by
// Hex or Text. Each appears once, in a fixed order.
func Inputs(t testing.TB, group string, names ...string) [][]byte {
	t.Helper()
	only, named := map[string]bool{}, map[string]bool{}
	for _, n := range names {
		only[n], only[n+"Hex"], only[n+"Text"], named[n] = true, true, true, true
	}
	seen := map[string]bool{}
	for _, b := range read(t, group) {
		walk(b, "", func(name string, raw json.RawMessage) {
			switch {
			case len(only) > 0 && !only[name]:
			case name == "input" || name == "message" || name == "envelope" || name == "receipt" || name == "founding" ||
				named[name]:
				seen[string(raw)] = true
			case strings.HasSuffix(name, "Hex"):
				var s string
				if json.Unmarshal(raw, &s) == nil {
					if h, err := hex.DecodeString(s); err == nil {
						seen[string(h)] = true
					}
				}
			case strings.HasSuffix(name, "Text"):
				var s string
				if json.Unmarshal(raw, &s) == nil {
					seen[s] = true
				}
			}
		})
	}
	return sorted(seen)
}

// Strings returns every string value in the vectors of vectors/<group>/, at any depth, each once, in a
// fixed order, for the seeds of a fuzz test that takes a string (fuzz-parsers).
func Strings(t testing.TB, group string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, b := range read(t, group) {
		walk(b, "", func(_ string, raw json.RawMessage) {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				seen[s] = true
			}
		})
	}
	out := make([]string, 0, len(seen))
	for _, b := range sorted(seen) {
		out = append(out, string(b))
	}
	return out
}

func sorted(seen map[string]bool) [][]byte {
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][]byte, len(keys))
	for i, k := range keys {
		out[i] = []byte(k)
	}
	return out
}

// walk calls fn with the name and the JSON of every member below raw, depth first; a list's items
// take the list's name.
func walk(raw json.RawMessage, name string, fn func(string, json.RawMessage)) {
	raw = bytes.TrimSpace(raw)
	if name != "" {
		fn(name, raw)
	}
	switch {
	case len(raw) > 0 && raw[0] == '{':
		var o map[string]json.RawMessage
		if json.Unmarshal(raw, &o) == nil {
			for k, v := range o {
				walk(v, k, fn)
			}
		}
	case len(raw) > 0 && raw[0] == '[':
		var l []json.RawMessage
		if json.Unmarshal(raw, &l) == nil {
			for _, v := range l {
				walk(v, name, fn)
			}
		}
	}
}

// read reads every vector file of vectors/<group>/, found in the test's directory or one above it.
func read(t testing.TB, group string) []json.RawMessage {
	t.Helper()
	dir := "."
	var files []string
	for range 5 {
		files, _ = filepath.Glob(filepath.Join(dir, "vectors", group, "*.json"))
		if len(files) > 0 {
			break
		}
		dir = filepath.Join(dir, "..")
	}
	if len(files) == 0 {
		t.Fatalf("no vector of vectors/%s above the test's directory", group)
	}
	var out []json.RawMessage
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

// Values returns the string values of the members named names in the vectors of vectors/<group>/, at
// any depth, each once, in a fixed order, for the seeds of a fuzz test that takes one such value.
func Values(t testing.TB, group string, names ...string) []string {
	t.Helper()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	seen := map[string]bool{}
	for _, b := range read(t, group) {
		walk(b, "", func(name string, raw json.RawMessage) {
			var s string
			if want[name] && json.Unmarshal(raw, &s) == nil {
				seen[s] = true
			}
		})
	}
	out := make([]string, 0, len(seen))
	for _, b := range sorted(seen) {
		out = append(out, string(b))
	}
	return out
}
