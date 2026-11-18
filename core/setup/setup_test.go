package setup_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/internal/shape"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
)

// foundVector is a found vector of vectors/founding/.
type foundVector struct {
	Input     json.RawMessage `json:"input"`
	InputText *string         `json:"inputText"`
	InputHex  *string         `json:"inputHex"`
	FoundedAt string          `json:"foundedAt"`
	Openings  []struct {
		Actor       string          `json:"actor"`
		Opening     json.RawMessage `json:"opening"`
		OpeningHex  *string         `json:"openingHex"`
		OpeningText *string         `json:"openingText"`
	} `json:"openings"`
	Acceptances     json.RawMessage `json:"acceptances"`
	AcceptancesHex  *string         `json:"acceptancesHex"`
	AcceptancesText *string         `json:"acceptancesText"`
	Expect          struct {
		Accord, Card, Code, Terms string
	} `json:"expect"`
}

// inputs are the vector's founding inputs: its set, every opening it gives, or none, and its
// acceptances (setup-contract-15, setup-contract-16).
func (v foundVector) inputs() founding.Inputs {
	in := founding.Inputs{Set: v.set(), Acceptances: v.Acceptances}
	switch {
	case v.AcceptancesHex != nil:
		in.Acceptances, _ = hex.DecodeString(*v.AcceptancesHex)
	case v.AcceptancesText != nil:
		in.Acceptances = []byte(*v.AcceptancesText)
	}
	if v.Openings != nil {
		in.Openings = setup.Openings{ByActor: map[string][]byte{}}
		for _, o := range v.Openings {
			b := []byte(o.Opening)
			switch {
			case o.OpeningHex != nil:
				b, _ = hex.DecodeString(*o.OpeningHex)
			case o.OpeningText != nil:
				b = []byte(*o.OpeningText)
			}
			in.Openings.ByActor[o.Actor] = b
		}
	}
	return in
}

// clear is the vector's set with each seal entry replaced by the declaration its opening holds, so a
// test can change a declaration and seal the set again (foundClear).
func (v foundVector) clear(t testing.TB) []byte {
	t.Helper()
	in := v.inputs()
	val, _, err := shape.Decode(in.Set)
	if err != nil {
		t.Fatal(err)
	}
	set := val.(shape.Object)
	for i, x := range set["declarations"].([]any) {
		e := x.(shape.Object)
		ov, _, err := shape.Decode(in.Openings.ByActor[e["actor"].(string)])
		if err != nil {
			t.Fatalf("the opening of %v does not read", e["actor"])
		}
		set["declarations"].([]any)[i] = ov.(shape.Object)["declaration"]
	}
	return shape.Encode(set)
}

// foundClear seals a set written with its declarations in the clear and runs rows 1 to 17 over it
// with every opening (setup-contract-12, setup-contract-13).
func foundClear(t testing.TB, clear []byte, at string) (*setup.Accord, error) {
	t.Helper()
	in := ptesting.Sealed(t, clear, at)
	return setup.Found(in.Set, in.Openings, at)
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
		_, err := founding.Found(v.inputs(), v.FoundedAt)
		var se *setup.Error
		if !errors.As(err, &se) || string(se.Code) != v.Expect.Code {
			t.Fatalf("%s: got %v, want %s", name, err, v.Expect.Code)
		}
		if err.Error() != "setup: "+v.Expect.Code {
			t.Fatalf("%s: error text %q", name, err.Error())
		}
		codes[v.Expect.Code] = true
	}
	_, err := setup.Found([]byte(`{}`), setup.Openings{}, "2026-11-12T08:00:00")
	if err == nil || err.Error() != "setup: at" {
		t.Fatalf("a founding time that breaks the timestamp rule: %v", err)
	}
	if len(codes) != 23 {
		t.Fatalf("the vectors trigger %d of the 23 founding codes", len(codes))
	}
}

// TestAccordReadsAreCopies changes everything an Accord's reads return and reads them again
// (results-are-copies).
func TestAccordReadsAreCopies(t *testing.T) {
	t.Parallel()
	v := vectors(t)["ask-and-log"]
	a, err := founding.Found(v.inputs(), v.FoundedAt)
	if err != nil {
		t.Fatal(err)
	}
	salt := a.Salt(a.Members()[0])
	salt[0] ^= 1
	if a.Salt(a.Members()[0])[0] == salt[0] {
		t.Fatal("the salt changed")
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

// FuzzFound founds any bytes at any time, without the openings. It never panics; a founded Accord
// has an id and a card hash, members who each hold a role, and no term read (setup-contract-16).
func FuzzFound(f *testing.F) {
	for _, v := range vectors(f) {
		f.Add(v.set(), v.FoundedAt)
	}
	f.Add([]byte(`{"policy":{},"policy":{}}`), "2026-11-12T08:00:00Z")
	f.Add([]byte(`[]`), "2026-02-29T08:00:00Z")
	f.Fuzz(func(t *testing.T, set []byte, at string) {
		a, err := setup.Found(set, setup.Openings{}, at)
		if err != nil {
			var se *setup.Error
			if !errors.As(err, &se) || a != nil {
				t.Fatalf("got %v, %v", a, err)
			}
			return
		}
		if len(a.ID()) != 71 || len(a.Card()) != 71 || len(a.Members()) == 0 || a.TermsChecked() {
			t.Fatalf("a founded Accord without an id, a card hash or members, or with its terms checked")
		}
		for _, m := range a.Members() {
			if len(a.Roles(m)) == 0 || len(a.Terms(m)) != 0 || a.Opened(m) {
				t.Fatalf("a member without a role, or with its terms read")
			}
		}
	})
}

// FuzzFoundOpening founds the ask-and-log set with any bytes in place of its first member's
// opening. It never panics; a refusal is a founding code, and a founded Accord has every member
// opened, each with a role and a term (setup-contract-13).
func FuzzFoundOpening(f *testing.F) {
	v := vectors(f)["ask-and-log"]
	in := v.inputs()
	first := ""
	for _, o := range v.Openings {
		if first == "" || o.Actor < first {
			first = o.Actor
		}
		f.Add([]byte(o.Opening))
	}
	f.Add([]byte(`{"declaration":{},"salt":""}`))
	f.Add([]byte(`{"declaration":{"actor":7},"salt":"00"}`))
	f.Fuzz(func(t *testing.T, opening []byte) {
		byActor := map[string][]byte{}
		for k, b := range in.Openings.ByActor {
			byActor[k] = b
		}
		byActor[first] = opening
		a, err := setup.Found(in.Set, setup.Openings{ByActor: byActor}, v.FoundedAt)
		if err != nil {
			var se *setup.Error
			if !errors.As(err, &se) || a != nil {
				t.Fatalf("got %v, %v", a, err)
			}
			return
		}
		if !a.TermsChecked() {
			t.Fatal("founded with a member not opened")
		}
		for _, m := range a.Members() {
			if len(a.Roles(m)) == 0 || len(a.Terms(m)) == 0 || len(a.Salt(m)) != setup.SaltBytes {
				t.Fatalf("a member without a role, a term or its salt")
			}
		}
	})
}
