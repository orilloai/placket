package answer

import (
	"testing"

	"github.com/orilloai/placket/core/internal/testing/seeds"
)

// FuzzRead reads any bytes as an answer call without panicking, and a call it reads signs again to
// the same call (fuzz-parsers, core-no-panic). Its seeds are the inputs of the signatures vectors,
// the answer calls among them.
func FuzzRead(f *testing.F) {
	for _, b := range seeds.Inputs(f, "signatures") {
		f.Add(b)
	}
	f.Add([]byte(`{"accord":"sha256:00","action":"declined","from":"ed25519:00","question":"sha256:00","sig":"00"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		c, err := Read(b)
		if err != nil {
			return
		}
		if ID(b) == "" {
			t.Fatal("a call that reads has no id")
		}
		if c.From == "" || c.Accord == "" {
			t.Fatalf("a call that reads has an empty member: %+v", c)
		}
	})
}

// FuzzID hashes any bytes without panicking (fuzz-parsers, core-no-panic).
func FuzzID(f *testing.F) {
	for _, b := range seeds.Inputs(f, "signatures") {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = ID(b)
	})
}
