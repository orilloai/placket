package setup_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/setup"
)

// foundVector is a found vector of vectors/founding/.
type foundVector struct {
	Input     json.RawMessage `json:"input"`
	InputText *string         `json:"inputText"`
	InputHex  *string         `json:"inputHex"`
	FoundedAt string          `json:"foundedAt"`
	Expect    struct {
		Accord, Card, Code string
	} `json:"expect"`
}

func (v foundVector) set() []byte {
	switch {
	case v.InputHex != nil:
		b, _ := hex.DecodeString(*v.InputHex)
		return b
	case v.InputText != nil:
		return []byte(*v.InputText)
	}
	return v.Input
}

// vectors reads every found vector of the founding group.
func vectors(t testing.TB) map[string]foundVector {
	t.Helper()
	files, _ := filepath.Glob("../../vectors/founding/*.json")
	if len(files) == 0 {
		t.Fatal("no vector in vectors/founding/")
	}
	out := map[string]foundVector{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v foundVector
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".json")] = v
	}
	return out
}

// TestErrorTextQuotesNoInput founds every refused set of the founding vectors, which hold keys,
// signatures and terms, and reads each error: its text is the package and the code alone
// (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	codes := map[string]bool{}
	for name, v := range vectors(t) {
		if v.Expect.Code == "" {
			continue
		}
		_, err := setup.Found(v.set(), v.FoundedAt)
		var se *setup.Error
		if !errors.As(err, &se) || string(se.Code) != v.Expect.Code {
			t.Fatalf("%s: got %v, want %s", name, err, v.Expect.Code)
		}
		if err.Error() != "setup: "+v.Expect.Code {
			t.Fatalf("%s: error text %q", name, err.Error())
		}
		codes[v.Expect.Code] = true
	}
	_, err := setup.Found([]byte(`{}`), "2026-11-12T08:00:00")
	if err == nil || err.Error() != "setup: at" {
		t.Fatalf("a founding time that breaks the timestamp rule: %v", err)
	}
	if len(codes) != 15 {
		t.Fatalf("the vectors trigger %d of the 15 founding codes", len(codes))
	}
}

// TestAccordReadsAreCopies changes everything an Accord's reads return and reads them again
// (results-are-copies).
func TestAccordReadsAreCopies(t *testing.T) {
	t.Parallel()
	v := vectors(t)["ask-and-log"]
	a, err := setup.Found(v.set(), v.FoundedAt)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != v.Expect.Accord || a.Card() != v.Expect.Card {
		t.Fatalf("got %s and %s", a.ID(), a.Card())
	}
	m := a.Members()
	if len(m) != 3 {
		t.Fatalf("%d members", len(m))
	}
	before := a.Terms(m[0])
	roles := a.Roles(m[0])
	m[0] = "changed"
	roles[0] = "changed"
	terms := a.Terms(a.Members()[0])
	terms[0].Kinds[0] = "changed"
	terms[0].With[0] = "changed"
	if terms[0].Resolve != nil {
		terms[0].Resolve.To[0] = "changed"
	}
	terms[0].Stance = "changed"
	after := a.Terms(a.Members()[0])
	if a.Members()[0] == "changed" || a.Roles(a.Members()[0])[0] == "changed" {
		t.Fatal("a member or a role changed")
	}
	b1, _ := json.Marshal(before)
	b2, _ := json.Marshal(after)
	if string(b1) != string(b2) {
		t.Fatalf("the terms changed: %s, then %s", b1, b2)
	}
	if !a.Member(a.Members()[1]) || a.Member(a.Mediator()) {
		t.Fatal("membership is wrong")
	}
}

// FuzzFound founds any bytes at any time. It never panics; a founded Accord has an id and a card
// hash, and members who each hold a role.
func FuzzFound(f *testing.F) {
	for _, v := range vectors(f) {
		f.Add(v.set(), v.FoundedAt)
	}
	f.Add([]byte(`{"policy":{},"policy":{}}`), "2026-11-12T08:00:00Z")
	f.Add([]byte(`[]`), "2026-02-29T08:00:00Z")
	f.Fuzz(func(t *testing.T, set []byte, at string) {
		a, err := setup.Found(set, at)
		if err != nil {
			var se *setup.Error
			if !errors.As(err, &se) || a != nil {
				t.Fatalf("got %v, %v", a, err)
			}
			return
		}
		if len(a.ID()) != 71 || len(a.Card()) != 71 || len(a.Members()) == 0 {
			t.Fatalf("a founded Accord without an id, a card hash or members")
		}
		for _, m := range a.Members() {
			if len(a.Roles(m)) == 0 || len(a.Terms(m)) == 0 {
				t.Fatalf("a member without a role or a term")
			}
		}
	})
}
