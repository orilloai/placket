package composition_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// withTerms founds the ask-and-log set with terms added to members' declarations, each declaration
// signed again by its actor.
func withTerms(t *testing.T, added map[string][]map[string]any) *setup.Accord {
	t.Helper()
	b, err := os.ReadFile("../../vectors/founding/ask-and-log.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		InputHex  string `json:"inputHex"`
		FoundedAt string `json:"foundedAt"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(v.InputHex)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var set map[string]any
	if err := d.Decode(&set); err != nil {
		t.Fatal(err)
	}
	for name, terms := range added {
		k := ptesting.Key(t, name)
		found := false
		for _, x := range set["declarations"].([]any) {
			decl := x.(map[string]any)
			if decl["actor"] != k.Reference() {
				continue
			}
			found = true
			for _, term := range terms {
				decl["terms"] = append(decl["terms"].([]any), term)
			}
			obj, _ := json.Marshal(decl)
			sig, err := signing.Sign(obj, signing.TypeDeclaration, k)
			if err != nil {
				t.Fatal(err)
			}
			decl["sig"] = sig
		}
		if !found {
			t.Fatalf("no declaration of the %s", name)
		}
	}
	j, _ := json.Marshal(set)
	c, err := canonical.Form(j)
	if err != nil {
		t.Fatal(err)
	}
	a, err := setup.Found(c, v.FoundedAt)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func term(dir, kind, with string) map[string]any {
	return map[string]any{"dir": dir, "select": map[string]any{"kind": []any{kind}}, "stance": "want", "with": []any{"role:" + with}}
}

// TestATermMatchesOnlyInItsDirection is composition-1's dir, on each side: a sender whose only term
// for the kind and the receiver's role is a get, and a receiver whose only term for the kind and the
// sender's role is a give, match nothing, however well the other side matches.
func TestATermMatchesOnlyInItsDirection(t *testing.T) {
	t.Parallel()
	a := withTerms(t, map[string][]map[string]any{
		// The applier already gets notes from the proposer; the proposer now gets notes from the
		// applier too. A note from the applier to the proposer has the receiver's get, and only a
		// get on the sender's side.
		"proposer": {term("get", "note", "applier"), term("give", "memo", "applier")},
		// A memo from the proposer to the applier has the sender's give, and only a give on the
		// receiver's side.
		"applier": {term("give", "memo", "proposer")},
	})
	p, ap := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference()
	for name, c := range map[string]composition.Crossing{
		"a sender with only a get":    {From: ap, To: p, Kind: "note"},
		"a receiver with only a give": {From: p, To: ap, Kind: "memo"},
	} {
		got := composition.Decide(a, c)
		if got.Verdict != composition.Block || got.Reason != composition.ReasonNoMatch {
			t.Fatalf("%s: got %+v, want block, no-match", name, got)
		}
	}
	// Each side's term matches in its own direction: the note the other way passes.
	if got := composition.Decide(a, composition.Crossing{From: p, To: ap, Kind: "note"}); got.Verdict != composition.Pass {
		t.Fatalf("a note from the proposer: got %+v", got)
	}
}
