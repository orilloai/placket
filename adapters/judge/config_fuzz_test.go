package judge_test

import (
	"testing"

	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/core/canonical"
)

// FuzzReadConfig reads any bytes as a judge's config, seeded from the config tests' cases: it never
// panics, and a config it reads has the typed hash of the bytes and a threshold from 0 to 100
// (fuzz-parsers).
func FuzzReadConfig(f *testing.F) {
	f.Add(configOf(f, nil))
	for _, c := range configCases(f) {
		f.Add(c.b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		cfg, h, err := judge.ReadConfig(b)
		if err != nil {
			if h != "" {
				t.Fatal("a refused config has a hash")
			}
			return
		}
		want, herr := canonical.TypedHash(b)
		if herr != nil || h != want || cfg.Threshold < 0 || cfg.Threshold > 100 || cfg.Provider != "jev" {
			t.Fatalf("read %+v, %q", cfg, h)
		}
	})
}
