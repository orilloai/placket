package ptesting

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
)

// Founding reads the found vector vectors/founding/<name>.json and returns its founding inputs: the
// sealed set's bytes, from inputHex; every opening it gives, each as a JSON value under its actor;
// and its acceptances, as a JSON value; and its foundedAt. It looks for vectors/ in the test's
// directory and the ones above it, so a test of any core package finds it.
func Founding(t testing.TB, name string) (founding.Inputs, string) {
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
		Openings  []struct {
			Actor   string          `json:"actor"`
			Opening json.RawMessage `json:"opening"`
		} `json:"openings"`
		Acceptances json.RawMessage `json:"acceptances"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	set, err := hex.DecodeString(v.InputHex)
	if err != nil || len(set) == 0 {
		t.Fatalf("%s: no founding set in inputHex", rel)
	}
	in := founding.Inputs{Set: set, Acceptances: []byte(v.Acceptances)}
	if v.Openings != nil {
		in.Openings = setup.Openings{ByActor: map[string][]byte{}}
		for _, o := range v.Openings {
			if len(o.Opening) == 0 {
				t.Fatalf("%s: an opening of %s is not given as a JSON value", rel, o.Actor)
			}
			in.Openings.ByActor[o.Actor] = []byte(o.Opening)
		}
	}
	return in, v.FoundedAt
}
