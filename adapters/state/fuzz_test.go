package state_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/internal/seeds"
	"github.com/orilloai/placket/adapters/state"
)

// FuzzSalts reads any bytes as salts.jsonl, a file another party may hand over with its state
// directory. It never panics; what it reads is one salt per line, and what it refuses is error state
// salts.jsonl.
func FuzzSalts(f *testing.F) {
	f.Add([]byte(`{"receiptId":"` + hash("a") + `","salt":"` + strings.Repeat("01", 16) + `"}` + "\n"))
	f.Add([]byte(`{"receiptId":"x","salt":"y","receiptId":"z"}` + "\n"))
	f.Add([]byte(`{"receiptId":null,"salt":""}` + "\n"))
	f.Add([]byte("\n\n"))
	for _, s := range seeds.Values(f, "crossing", "salt") {
		f.Add([]byte(`{"receiptId":"` + hash("a") + `","salt":"` + s + `"}` + "\n"))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d := open(t, filepath.Join(t.TempDir(), "s"), &bytes.Buffer{})
		if err := os.WriteFile(filepath.Join(d.Path(), state.FileSalts), b, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := d.Salts()
		if err != nil {
			var se *state.Error
			if !errors.As(err, &se) || se.Code != state.CodeState || se.File != state.FileSalts {
				t.Fatalf("refused as %v", err)
			}
			return
		}
		if len(got) > bytes.Count(b, []byte("\n")) {
			t.Fatalf("%d salts from %d lines", len(got), bytes.Count(b, []byte("\n")))
		}
	})
}
