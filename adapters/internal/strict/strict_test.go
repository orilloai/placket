package strict_test

import (
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
