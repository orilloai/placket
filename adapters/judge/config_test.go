package judge_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/core/canonical"
)

// configOf is the canonical bytes of the config review, with the members of change set over it and
// those whose value is nil removed.
func configOf(t testing.TB, change map[string]any) []byte {
	t.Helper()
	m := map[string]any{"name": "review", "provider": "jev", "host": "h.example", "model": "m",
		"threshold": 80, "instruction": "Block a part that deletes data."}
	for k, v := range change {
		if v == nil {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// configCases are configs ReadConfig refuses, each with the code it refuses it with.
func configCases(t testing.TB) []struct {
	name string
	b    []byte
	code judge.Code
} {
	return []struct {
		name string
		b    []byte
		code judge.Code
	}{
		{"a scheme", configOf(t, map[string]any{"host": "https://h.example"}), judge.CodeConfigHost},
		{"a path", configOf(t, map[string]any{"host": "h.example/v1"}), judge.CodeConfigHost},
		{"a user", configOf(t, map[string]any{"host": "u@h.example"}), judge.CodeConfigHost},
		{"an IPv4 literal", configOf(t, map[string]any{"host": "127.0.0.1"}), judge.CodeConfigHost},
		{"an IPv4 literal and a port", configOf(t, map[string]any{"host": "127.0.0.1:8443"}), judge.CodeConfigHost},
		{"an IPv6 literal", configOf(t, map[string]any{"host": "[::1]:443"}), judge.CodeConfigHost},
		{"one label", configOf(t, map[string]any{"host": "localhost"}), judge.CodeConfigHost},
		{"a port of 0", configOf(t, map[string]any{"host": "h.example:0"}), judge.CodeConfigHost},
		{"a port with a leading zero", configOf(t, map[string]any{"host": "h.example:0443"}), judge.CodeConfigHost},
		{"an upper-case host", configOf(t, map[string]any{"host": "H.example"}), judge.CodeConfigHost},
		{"an empty host", configOf(t, map[string]any{"host": ""}), judge.CodeConfigHost},
		{"a threshold of 101", configOf(t, map[string]any{"threshold": 101}), judge.CodeConfigThreshold},
		{"a threshold of -1", configOf(t, map[string]any{"threshold": -1}), judge.CodeConfigThreshold},
		{"a fraction", []byte(`{"host":"h.example","instruction":"Block a part that deletes data.","model":"m","name":"review","provider":"jev","threshold":80.5}`), judge.CodeConfigCanonical},
		{"another provider", configOf(t, map[string]any{"provider": "other"}), judge.CodeConfigProvider},
		{"no model", configOf(t, map[string]any{"model": nil}), judge.CodeConfigShape},
		{"no threshold", configOf(t, map[string]any{"threshold": nil}), judge.CodeConfigShape},
		{"an unknown member", configOf(t, map[string]any{"timeout": 20}), judge.CodeConfigShape},
		{"a threshold as text", configOf(t, map[string]any{"threshold": "80"}), judge.CodeConfigShape},
		{"a name that is no label", configOf(t, map[string]any{"name": "Review"}), judge.CodeConfigShape},
		{"an empty instruction", configOf(t, map[string]any{"instruction": ""}), judge.CodeConfigShape},
		{"members out of order", []byte(`{"provider":"jev","host":"h.example","instruction":"Block a part that deletes data.","model":"m","name":"review","threshold":80}`), judge.CodeConfigCanonical},
		{"a trailing line feed", append(configOf(t, nil), '\n'), judge.CodeConfigCanonical},
		{"no JSON", []byte("review"), judge.CodeConfigCanonical},
		// Two faults at once: the first in ReadConfig's order decides (check-order).
		{"an unknown member and another provider", configOf(t, map[string]any{"provider": "other", "timeout": 20}), judge.CodeConfigShape},
		{"another provider and a scheme", configOf(t, map[string]any{"provider": "other", "host": "https://h.example"}), judge.CodeConfigProvider},
		{"a scheme and a threshold of 101", configOf(t, map[string]any{"host": "https://h.example", "threshold": 101}), judge.CodeConfigHost},
	}
}

// TestReadConfigRefuses reads configs that do not hold: each is refused with its code and no hash
// (docs/adr/0023-judges.md).
func TestReadConfigRefuses(t *testing.T) {
	t.Parallel()
	for _, c := range configCases(t) {
		cfg, h, err := judge.ReadConfig(c.b)
		var je *judge.Error
		if !errors.As(err, &je) || je.Code != c.code || h != "" || cfg != (judge.Config{}) {
			t.Fatalf("%s: got %+v, %q, %v; want %s", c.name, cfg, h, err, c.code)
		}
		if got := err.Error(); got != "judge: "+string(c.code) {
			t.Fatalf("%s: the error reads %q", c.name, got)
		}
	}
}

// TestReadConfigReads reads a config that holds: its members, and its typed hash, the card's hash for
// the judge (canonical-3). A host with a port holds.
func TestReadConfigReads(t *testing.T) {
	t.Parallel()
	for _, host := range []string{"h.example", "api.h-1.example:8443"} {
		b := configOf(t, map[string]any{"host": host, "threshold": 0})
		cfg, h, err := judge.ReadConfig(b)
		if err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		want, _ := canonical.TypedHash(b)
		if h != want || cfg != (judge.Config{Name: "review", Provider: "jev", Host: host, Model: "m",
			Instruction: "Block a part that deletes data.", Threshold: 0}) {
			t.Fatalf("%s: got %+v, %q", host, cfg, h)
		}
	}
	if _, _, err := judge.ReadConfig(configOf(t, map[string]any{"threshold": 100})); err != nil {
		t.Fatalf("a threshold of 100: %v", err)
	}
}
