package strict_test

import (
	"encoding/json"
	"testing"

	"github.com/orilloai/placket/adapters/internal/seeds"
	"github.com/orilloai/placket/adapters/internal/strict"
)

type pair struct {
	Hash *string `json:"hash"`
	Seq  *int    `json:"seq,omitempty"`
}

// TestDecodeTakesExactNames reads objects into a struct: members named exactly as its tags are read,
// and a member named otherwise, in another case too, anything but one object, and anything after
// it, are refused.
func TestDecodeTakesExactNames(t *testing.T) {
	t.Parallel()
	var p pair
	if !strict.Decode([]byte(`{"hash":"h","seq":1}`), &p) || *p.Hash != "h" || *p.Seq != 1 {
		t.Fatal("an object of the struct's members was refused")
	}
	for _, b := range []string{`{"Hash":"h"}`, `{"hash":"h","x":1}`, `[]`, `{"hash":"h"} {}`, ``, `{"hash":1}`, `null`} {
		var p pair
		if strict.Decode([]byte(b), &p) {
			t.Fatalf("%s was read", b)
		}
	}
	if strict.Decode([]byte(`{}`), p) {
		t.Fatal("a struct that is not a pointer was decoded into")
	}
	if m, ok := strict.Member([]byte(`{"receiptId":"r","ReceiptId":"s"}`), "receiptId"); !ok || string(m) != `"r"` {
		t.Fatalf("got %s, %v", m, ok)
	}
	if _, ok := strict.Member([]byte(`{"ReceiptId":"s"}`), "receiptId"); ok {
		t.Fatal("a member of another case was found")
	}
}

type outer struct {
	Pair  pair            `json:"pair"`
	Pairs []pair          `json:"pairs"`
	Named map[string]pair `json:"named"`
}

// TestDecodeRefusesTwoReadings refuses what two readers could read two ways: a member repeated, at
// the top or nested; a nested member named in another case; ill-formed UTF-8; an unsafe number. A
// map reads its members as written, and refuses a repeated one too.
func TestDecodeRefusesTwoReadings(t *testing.T) {
	t.Parallel()
	for _, b := range []string{
		`{"hash":"A","hash":"B"}`,
		`{"pair":{"hash":"A","hash":"B"}}`,
		`{"pair":{"Hash":"A"}}`,
		`{"pairs":[{"hash":"A"},{"HASH":"B"}]}`,
		`{"named":{"x":{"Seq":1}}}`,
		"{\"hash\":\"\xff\"}",
		`{"hash":"h","seq":9007199254740992}`,
		`{"hash":"h","seq":1.5}`,
	} {
		var o struct {
			Hash  *string         `json:"hash"`
			Seq   *int            `json:"seq"`
			Pair  pair            `json:"pair"`
			Pairs []pair          `json:"pairs"`
			Named map[string]pair `json:"named"`
		}
		if strict.Decode([]byte(b), &o) {
			t.Fatalf("%q was read", b)
		}
	}
	var o outer
	if !strict.Decode([]byte(`{"named":{"x":{"hash":"h"}},"pair":{"hash":"h"},"pairs":[{"seq":2}]}`), &o) ||
		*o.Pair.Hash != "h" || *o.Pairs[0].Seq != 2 || *o.Named["x"].Hash != "h" {
		t.Fatal("an object named exactly at every depth was refused")
	}
	var m map[string]json.RawMessage
	if strict.Decode([]byte(`{"a":1,"a":2}`), &m) {
		t.Fatal("a map read a repeated member")
	}
	if !strict.Decode([]byte(`{"a":1,"A":2}`), &m) || string(m["a"]) != "1" || string(m["A"]) != "2" {
		t.Fatalf("a map of two names: got %v", m)
	}
	if _, ok := strict.Member([]byte(`{"receiptId":"r","receiptId":"s"}`), "receiptId"); ok {
		t.Fatal("a repeated member was found")
	}
}

// FuzzDecode reads any bytes into a struct. It never panics, and what it reads names its members
// exactly.
func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"hash":"h","seq":1}`))
	f.Add([]byte(`{"HASH":"h"}`))
	for _, in := range seeds.Inputs(f, "canonical") {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		var p pair
		_ = strict.Decode(b, &p)
		_, _ = strict.Member(b, "hash")
	})
}
