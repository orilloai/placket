package ptesting

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Founding reads the found vector vectors/founding/<name>.json and returns its founding set's bytes,
// from inputHex, and its foundedAt. It looks for vectors/ in the test's directory and the ones above
// it, so a test of any core package finds it.
func Founding(t testing.TB, name string) ([]byte, string) {
	t.Helper()
	rel := filepath.Join("vectors", "founding", name+".json")
	dir := "."
	var b []byte
	var err error
	for range 5 {
		if b, err = os.ReadFile(filepath.Join(dir, rel)); err == nil {
			break
		}
		dir = filepath.Join(dir, "..")
	}
	if err != nil {
		t.Fatalf("no %s above the test's directory: %v", rel, err)
	}
	var v struct {
		InputHex  string `json:"inputHex"`
		FoundedAt string `json:"foundedAt"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	set, err := hex.DecodeString(v.InputHex)
	if err != nil || len(set) == 0 {
		t.Fatalf("%s: no founding set in inputHex", rel)
	}
	return set, v.FoundedAt
}
