package setup_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
)

// TestSealIsTheSaltedDigest holds a seal to crossing-3's salted digest of the declaration's
// canonical bytes, its sig included, under the salt, written as a typed hash; and the opening to
// exactly the declaration and the salt in hex (setup-contract-12, setup-contract-13). Changing the
// salt or one byte of the declaration changes the seal (inputs-as-parameters).
func TestSealIsTheSaltedDigest(t *testing.T) {
	t.Parallel()
	decl := []byte(`{ "terms":[], "actor":"ed25519:` + hex.EncodeToString(make([]byte, 32)) + `", "sig":"00" }`)
	canon, err := canonical.Form(decl)
	if err != nil {
		t.Fatal(err)
	}
	salt := ptesting.SaltOf("a")
	entry, opening, err := setup.Seal(decl, salt)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append(append([]byte(nil), salt...), canon...))
	want := `{"actor":"ed25519:` + hex.EncodeToString(make([]byte, 32)) + `","seal":"sha256:` + hex.EncodeToString(sum[:]) + `"}`
	if string(entry) != want {
		t.Fatalf("entry %s, want %s", entry, want)
	}
	if wantO := `{"declaration":` + string(canon) + `,"salt":"` + hex.EncodeToString(salt) + `"}`; string(opening) != wantO {
		t.Fatalf("opening %s, want %s", opening, wantO)
	}
	other, _, _ := setup.Seal(decl, ptesting.SaltOf("b"))
	changed, _, _ := setup.Seal([]byte(`{"terms":[],"actor":"ed25519:`+hex.EncodeToString(make([]byte, 32))+`","sig":"01"}`), salt)
	if string(other) == string(entry) || string(changed) == string(entry) {
		t.Fatal("the seal does not change with its salt or its declaration")
	}
	for name, c := range map[string]struct{ decl, salt []byte }{
		"a 31-byte salt":         {decl, salt[:31]},
		"a 33-byte salt":         {decl, append(append([]byte(nil), salt...), 0)},
		"a declaration not JSON": {[]byte(`{`), salt},
		"a list":                 {[]byte(`[]`), salt},
		"an actor not a string":  {[]byte(`{"actor":7}`), salt},
		"repeated member names":  {[]byte(`{"actor":"a","actor":"b"}`), salt},
	} {
		_, _, err := setup.Seal(c.decl, c.salt)
		var se *setup.Error
		if !errors.As(err, &se) || se.Code != setup.InvalidShape || err.Error() != "setup: invalid-shape" {
			t.Errorf("%s: %v, want invalid-shape", name, err)
		}
	}
}

// sealedAskAndLog is the ask-and-log Accord's set sealed again with every opening, as foundClear
// seals it, with the vectors' keys.
func sealedAskAndLog(t testing.TB) ([]byte, map[string][]byte, string) {
	t.Helper()
	clear, at := base(t, "ask-and-log")
	in := ptesting.Sealed(t, encode(t, clear), at)
	return in.Set, in.Openings.ByActor, at
}

func copyOpenings(m map[string][]byte) map[string][]byte {
	out := map[string][]byte{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// reopen writes an opening of declaration d, as a JSON value, under the salt given in hex.
func reopen(t *testing.T, d any, salt string) []byte {
	t.Helper()
	return encode(t, map[string]any{"declaration": d, "salt": salt})
}

// TestTheOpeningOpensItsSeal holds row 4a and its codes (setup-contract-13): a missing opening is
// opening-missing; one whose declaration changed by one byte, whose salt is another, or which is
// another member's, is seal-mismatch; one with a member beside declaration and salt, a 31-byte salt,
// a declaration with a member it does not list or an actor that is not a string, or bytes the
// canonical form refuses, is invalid-shape; and rows 5 to 8 read the opened declaration, so a maybe
// without its rule, sealed and signed, is maybe-without-resolve.
func TestTheOpeningOpensItsSeal(t *testing.T) {
	t.Parallel()
	set, openings, at := sealedAskAndLog(t)
	proposer := ptesting.Key(t, "proposer").Reference()
	applier := ptesting.Key(t, "applier").Reference()
	v, _, _ := shape.Decode(openings[proposer])
	o := v.(shape.Object)
	decl, salt := o["declaration"].(shape.Object), o["salt"].(string)
	if c := code(t, set, setup.Openings{ByActor: openings}, at); c != founded {
		t.Fatalf("the sealed set is refused with %s", c)
	}

	edited := map[string]any{}
	for k, x := range decl {
		edited[k] = x
	}
	edited["sig"] = "ff" + decl["sig"].(string)[2:]
	withExtra := map[string]any{"declaration": decl, "salt": salt, "note": "x"}
	declExtra := map[string]any{"checks": []any{}}
	for k, x := range decl {
		declExtra[k] = x
	}
	declActor := map[string]any{}
	for k, x := range decl {
		declActor[k] = x
	}
	declActor["actor"] = 7
	noResolve := map[string]any{}
	for k, x := range decl {
		noResolve[k] = x
	}
	terms := append([]any(nil), decl["terms"].([]any)...)
	first := map[string]any{}
	for k, x := range terms[0].(shape.Object) {
		first[k] = x
	}
	delete(first, "resolve")
	terms[0] = first
	noResolve["terms"] = terms
	delete(noResolve, "sig")
	sign(t, noResolve, "proposer")
	resealed, reopened, err := setup.Seal(encode(t, noResolve), ptesting.SaltOf(proposer))
	if err != nil {
		t.Fatal(err)
	}
	// The set with the proposer's seal entry replaced by the one of its declaration without a rule.
	sv, _, _ := shape.Decode(set)
	so := sv.(shape.Object)
	ev, _, _ := shape.Decode(resealed)
	for i, x := range so["declarations"].([]any) {
		if x.(shape.Object)["actor"] == proposer {
			so["declarations"].([]any)[i] = ev
		}
	}
	noResolveSet := shape.Encode(so)

	for name, c := range map[string]struct {
		set     []byte
		opening []byte
		drop    bool
		want    setup.Code
	}{
		"no opening":                       {set, nil, true, setup.OpeningMissing},
		"a declaration one byte changed":   {set, reopen(t, edited, salt), false, setup.SealMismatch},
		"another salt":                     {set, reopen(t, decl, hex.EncodeToString(ptesting.SaltOf("other"))), false, setup.SealMismatch},
		"another member's opening":         {set, openings[applier], false, setup.SealMismatch},
		"an extra member":                  {set, encode(t, withExtra), false, setup.InvalidShape},
		"a 31-byte salt":                   {set, reopen(t, decl, salt[:62]), false, setup.InvalidShape},
		"a salt in uppercase hex":          {set, reopen(t, decl, "AB"+salt[2:]), false, setup.InvalidShape},
		"a declaration with checks":        {set, reopen(t, declExtra, salt), false, setup.InvalidShape},
		"a declaration whose actor is 7":   {set, reopen(t, declActor, salt), false, setup.InvalidShape},
		"bytes the canonical form refuses": {set, []byte(`{"salt":"00","salt":"00"}`), false, setup.InvalidShape},
		"no salt":                          {set, encode(t, map[string]any{"declaration": decl}), false, setup.InvalidShape},
		"a maybe without its rule":         {noResolveSet, reopened, false, setup.MaybeWithoutResolve},
	} {
		byActor := copyOpenings(openings)
		if c.drop {
			delete(byActor, proposer)
		} else {
			byActor[proposer] = c.opening
		}
		if got := code(t, c.set, setup.Openings{ByActor: byActor}, at); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

// TestFoundingWithoutTheOpenings holds setup-contract-16: with no openings the set founds with no
// term read, every member a member with its roles; with some, only the members opened have terms
// and salts, and a member with no opening is not opening-missing; with every opening, every member
// is opened. The Accord id is the same in each.
func TestFoundingWithoutTheOpenings(t *testing.T) {
	t.Parallel()
	set, openings, at := sealedAskAndLog(t)
	proposer := ptesting.Key(t, "proposer").Reference()
	none, err := setup.Found(set, setup.Openings{}, at)
	if err != nil {
		t.Fatal(err)
	}
	some := copyOpenings(openings)
	delete(some, proposer)
	part, err := setup.Found(set, setup.Openings{ByActor: some, Some: true}, at)
	if err != nil {
		t.Fatal(err)
	}
	all, err := setup.Found(set, setup.Openings{ByActor: openings}, at)
	if err != nil {
		t.Fatal(err)
	}
	if none.ID() != all.ID() || part.ID() != all.ID() {
		t.Fatal("the Accord id depends on the openings")
	}
	if none.TermsChecked() || part.TermsChecked() || !all.TermsChecked() {
		t.Fatalf("terms checked: none %v, some %v, every %v", none.TermsChecked(), part.TermsChecked(), all.TermsChecked())
	}
	for _, m := range all.Members() {
		if !none.Member(m) || len(none.Roles(m)) == 0 || none.Terms(m) != nil || none.Opened(m) || none.Salt(m) != nil {
			t.Fatalf("without openings, %s is read wrong", m)
		}
		opened := m != proposer
		if part.Opened(m) != opened || (part.Terms(m) != nil) != opened || (part.Salt(m) != nil) != opened {
			t.Fatalf("with some openings, %s opened %v", m, part.Opened(m))
		}
		if !all.Opened(m) || string(all.Salt(m)) != string(ptesting.SaltOf(m)) {
			t.Fatalf("with every opening, %s is not opened with its salt", m)
		}
	}
	if c := code(t, set, setup.Openings{ByActor: some}, at); c != setup.OpeningMissing {
		t.Fatalf("every opening but one, not as some: %s, want opening-missing", c)
	}
}

// code runs rows 1 to 17 and returns the code, or founded.
func code(t *testing.T, set []byte, o setup.Openings, at string) setup.Code {
	t.Helper()
	_, err := setup.Found(set, o, at)
	if err == nil {
		return founded
	}
	var se *setup.Error
	if !errors.As(err, &se) {
		t.Fatalf("not a founding refusal: %v", err)
	}
	return se.Code
}

// FuzzSeal seals any bytes under any salt. It never panics; a refusal is invalid-shape, and a seal
// entry and its opening found under the opening's own seal: the entry names the declaration's actor
// and its seal is the salted digest of the opening's declaration (setup-contract-12,
// setup-contract-13).
func FuzzSeal(f *testing.F) {
	_, openings, _ := sealedAskAndLog(f)
	for _, b := range openings {
		v, _, _ := shape.Decode(b)
		o := v.(shape.Object)
		salt, _ := hex.DecodeString(o["salt"].(string))
		f.Add(shape.Encode(o["declaration"]), salt)
	}
	f.Add([]byte(`{"actor":"x"}`), make([]byte, setup.SaltBytes))
	f.Add([]byte(`{"actor":7}`), make([]byte, setup.SaltBytes))
	f.Add([]byte(`{"actor":"x"}`), make([]byte, setup.SaltBytes-1))
	f.Add([]byte(`{"actor":"x","actor":"y"}`), make([]byte, setup.SaltBytes))
	f.Add([]byte(`[]`), []byte(nil))
	f.Fuzz(func(t *testing.T, declaration, salt []byte) {
		entry, opening, err := setup.Seal(declaration, salt)
		if err != nil {
			var se *setup.Error
			if !errors.As(err, &se) || se.Code != setup.InvalidShape || entry != nil || opening != nil {
				t.Fatalf("got %q, %q, %v", entry, opening, err)
			}
			return
		}
		ev, _, err := shape.Decode(entry)
		if err != nil {
			t.Fatalf("the entry is not canonical: %v", err)
		}
		ov, _, err := shape.Decode(opening)
		if err != nil {
			t.Fatalf("the opening is not canonical: %v", err)
		}
		e, o := ev.(shape.Object), ov.(shape.Object)
		d := o["declaration"].(shape.Object)
		if e["actor"] != d["actor"] || o["salt"] != hex.EncodeToString(salt) {
			t.Fatal("the entry or the opening is not the declaration's")
		}
		sum := sha256.Sum256(append(append([]byte(nil), salt...), shape.Encode(d)...))
		if e["seal"] != canonical.HashPrefix+hex.EncodeToString(sum[:]) {
			t.Fatal("the seal is not the salted digest of the opened declaration")
		}
	})
}
