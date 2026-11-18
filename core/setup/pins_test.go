package setup_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// founded is the code a set founds with: none.
const founded setup.Code = ""

// encode returns the canonical bytes of v.
func encode(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// foundCode founds set at the time at and returns the refusal's code, or founded.
func foundCode(t testing.TB, set []byte, at string) setup.Code {
	t.Helper()
	a, err := setup.Found(set, at)
	if err == nil {
		if a == nil {
			t.Fatal("no Accord and no error")
		}
		return founded
	}
	var se *setup.Error
	if !errors.As(err, &se) {
		t.Fatalf("not a founding refusal: %v", err)
	}
	return se.Code
}

// base decodes the accepted founding vector name into a set to change, and the time it founds at.
// It first holds that the decoded set encodes back to the vector's bytes and founds, so a case that
// changes it is refused for its change alone.
func base(t testing.TB, name string) (map[string]any, string) {
	t.Helper()
	v, ok := vectors(t)[name]
	if !ok || v.Expect.Code != "" {
		t.Fatalf("%s: no accepted vector", name)
	}
	raw := v.set()
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var set map[string]any
	if err := d.Decode(&set); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if got := encode(t, set); !bytes.Equal(got, raw) {
		t.Fatalf("%s: re-encoded as %x, want %x", name, got, raw)
	}
	if c := foundCode(t, raw, v.FoundedAt); c != founded {
		t.Fatalf("%s: the unchanged set is refused with %s", name, c)
	}
	return set, v.FoundedAt
}

// declarationOf returns the declaration in set whose actor is the vectors' key of name.
func declarationOf(t testing.TB, set map[string]any, name string) map[string]any {
	t.Helper()
	ref := ptesting.Key(t, name).Reference()
	for _, x := range set["declarations"].([]any) {
		if d := x.(map[string]any); d["actor"] == ref {
			return d
		}
	}
	t.Fatalf("no declaration by %s", name)
	return nil
}

// termsOf returns the terms of d.
func termsOf(d map[string]any) []map[string]any {
	var out []map[string]any
	for _, x := range d["terms"].([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

// sign signs d, a declaration, again with the vectors' key of name, so a change to it leaves its
// signature good.
func sign(t testing.TB, d map[string]any, name string) {
	t.Helper()
	sig, err := signing.Sign(encode(t, d), signing.TypeDeclaration, ptesting.Key(t, name))
	if err != nil {
		t.Fatal(err)
	}
	d["sig"] = sig
}

// aHash is a well-formed typed hash: sha256: and 64 lowercase hex digits.
var aHash = canonical.HashPrefix + strings.Repeat("ab", 32)

// TestTwoDeclarationsByOneActor holds that the declarations are in strictly ascending actor-reference
// order: a declaration repeated next to itself, signed and bound, is invalid-shape at section 3 row
// 2, before any declaration's own rows (setup-contract-10, setup-contract-9).
func TestTwoDeclarationsByOneActor(t *testing.T) {
	t.Parallel()
	set, at := base(t, "ask-and-log")
	ds := set["declarations"].([]any)
	set["declarations"] = append([]any{ds[0], ds[0]}, ds[1:]...)
	if c := foundCode(t, encode(t, set), at); c != setup.InvalidShape {
		t.Fatalf("got %q, want %s", c, setup.InvalidShape)
	}
}

// TestMembersNotListed holds that an object of the set with a member its format does not list is
// refused: at the top level, in the policy and in a declaration it is invalid-shape at section 3 row
// 2 (setup-contract-9); in a term's select and in a term's resolve, the declaration signed again
// after the change, it is term-invalid at row 6, after the signature of row 5 (setup-contract-1).
func TestMembersNotListed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit func(t *testing.T, set map[string]any)
		want setup.Code
	}{
		{"top level", func(t *testing.T, set map[string]any) {
			set["extra"] = "x"
		}, setup.InvalidShape},
		{"policy", func(t *testing.T, set map[string]any) {
			set["policy"].(map[string]any)["extra"] = "x"
		}, setup.InvalidShape},
		{"declaration", func(t *testing.T, set map[string]any) {
			d := declarationOf(t, set, "proposer")
			d["extra"] = "x"
			sign(t, d, "proposer")
		}, setup.InvalidShape},
		{"select", func(t *testing.T, set map[string]any) {
			d := declarationOf(t, set, "proposer")
			termsOf(d)[1]["select"].(map[string]any)["extra"] = "x"
			sign(t, d, "proposer")
		}, setup.TermInvalid},
		{"resolve", func(t *testing.T, set map[string]any) {
			d := declarationOf(t, set, "proposer")
			r, ok := termsOf(d)[0]["resolve"].(map[string]any)
			if !ok {
				t.Fatal("the proposer's first term has no resolve")
			}
			r["extra"] = "x"
			sign(t, d, "proposer")
		}, setup.TermInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, "ask-and-log")
			tc.edit(t, set)
			if c := foundCode(t, encode(t, set), at); c != tc.want {
				t.Fatalf("got %q, want %s", c, tc.want)
			}
		})
	}
}

// TestOperatorIsAnActorReference holds that the mediator's operator is an actor reference: any
// other value is invalid-shape at section 3 row 2, and another actor's reference founds
// (setup-contract-7, setup-contract-9).
func TestOperatorIsAnActorReference(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		operator any
		want     setup.Code
	}{
		{"another actor", ptesting.Key(t, "other-mediator").Reference(), founded},
		{"a name", "operator", setup.InvalidShape},
		{"a prefix alone", signing.ReferencePrefix, setup.InvalidShape},
		{"a number", json.Number("7"), setup.InvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, "ask-and-log")
			set["mediator"].(map[string]any)["operator"] = tc.operator
			if c := foundCode(t, encode(t, set), at); c != tc.want {
				t.Fatalf("got %q, want %q", c, tc.want)
			}
		})
	}
}

// TestComponentVersionLength holds that a card component's version is at most 64 characters: the
// kernel's version of 64 founds and one of 65 is invalid-shape at section 3 row 2 (setup-contract-4,
// setup-contract-9).
func TestComponentVersionLength(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		version string
		want    setup.Code
	}{
		{"64 characters", "0.1." + strings.Repeat("a", 60), founded},
		{"65 characters", "0.1." + strings.Repeat("a", 61), setup.InvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, "ask-and-log")
			set["card"].(map[string]any)["kernel"].(map[string]any)["version"] = tc.version
			if c := foundCode(t, encode(t, set), at); c != tc.want {
				t.Fatalf("got %q, want %q", c, tc.want)
			}
		})
	}
}

// TestCardJudges holds that each entry of the card's judges is read for shape: an object with
// exactly a label name, a version and a typed hash founds, and an entry with anything else is
// invalid-shape at section 3 row 2 (setup-contract-4, setup-contract-9).
func TestCardJudges(t *testing.T) {
	t.Parallel()
	judge := func(change func(j map[string]any)) any {
		j := map[string]any{"name": "a-judge", "version": "0.1.0", "hash": aHash}
		if change != nil {
			change(j)
		}
		return j
	}
	cases := []struct {
		name  string
		entry any
		want  setup.Code
	}{
		{"well formed", judge(nil), founded},
		{"not an object", "a-judge", setup.InvalidShape},
		{"a member not listed", judge(func(j map[string]any) { j["extra"] = "x" }), setup.InvalidShape},
		{"no hash", judge(func(j map[string]any) { delete(j, "hash") }), setup.InvalidShape},
		{"a name not a label", judge(func(j map[string]any) { j["name"] = "A Judge" }), setup.InvalidShape},
		{"a version not a version", judge(func(j map[string]any) { j["version"] = "0..1" }), setup.InvalidShape},
		{"a hash not a typed hash", judge(func(j map[string]any) { j["hash"] = strings.Repeat("ab", 32) }), setup.InvalidShape},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, "ask-and-log")
			set["card"].(map[string]any)["judges"] = []any{tc.entry}
			if c := foundCode(t, encode(t, set), at); c != tc.want {
				t.Fatalf("got %q, want %q", c, tc.want)
			}
		})
	}
}

// TestTermChecksList holds that a term has no checks member: an empty list, a value that is not a
// list and a list of one well-formed check, the declaration signed again after the change, are each
// term-invalid at section 3 row 6, a member the term does not list (setup-contract-1).
func TestTermChecksList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		checks any
		want   setup.Code
	}{
		{"empty", []any{}, setup.TermInvalid},
		{"not a list", "forbidden", setup.TermInvalid},
		{"one check", []any{map[string]any{"name": "forbidden", "corpus": "tests"}}, setup.TermInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, "ask-and-log")
			d := declarationOf(t, set, "proposer")
			termsOf(d)[1]["checks"] = tc.checks
			sign(t, d, "proposer")
			if c := foundCode(t, encode(t, set), at); c != tc.want {
				t.Fatalf("got %q, want %q", c, tc.want)
			}
		})
	}
}

// TestRolesAreTheBindings holds that each member of a founded Accord holds exactly the roles its
// binding gives, in the binding's order, for every accepted founding vector: one binding with two
// roles gives both (setup-contract-2), and a policy whose roles are empty, which section 3 row 15
// passes with no role held (setup-contract-5), still leaves each member the roles of its binding,
// which composition matches a term's with against.
func TestRolesAreTheBindings(t *testing.T) {
	t.Parallel()
	names := []string{"ask-and-log", "compose", "one-binding-two-roles", "operator-changes-the-id", "policy-without-roles", "two-principals", "two-entries-one-role", "a-role-held-not-required"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, name)
			a, err := setup.Found(encode(t, set), at)
			if err != nil {
				t.Fatal(err)
			}
			bound := map[string][]string{}
			for _, x := range set["bindings"].([]any) {
				b := x.(map[string]any)
				var roles []string
				for _, r := range b["roles"].([]any) {
					roles = append(roles, r.(string))
				}
				bound[b["actor"].(string)] = roles
			}
			if len(a.Members()) == 0 {
				t.Fatal("no member")
			}
			for _, m := range a.Members() {
				if got, want := a.Roles(m), bound[m]; len(want) == 0 || !slices.Equal(got, want) {
					t.Fatalf("%s holds %q, want %q", m, got, want)
				}
			}
		})
	}
	t.Run("two roles of one binding", func(t *testing.T) {
		t.Parallel()
		set, at := base(t, "one-binding-two-roles")
		a, err := setup.Found(encode(t, set), at)
		if err != nil {
			t.Fatal(err)
		}
		got := a.Roles(ptesting.Key(t, "proposer").Reference())
		if want := []string{"proposer", "decider"}; !slices.Equal(got, want) {
			t.Fatalf("the proposer holds %q, want %q", got, want)
		}
	})
	t.Run("an empty policy", func(t *testing.T) {
		t.Parallel()
		set, at := base(t, "policy-without-roles")
		if roles := set["policy"].(map[string]any)["roles"].([]any); len(roles) != 0 {
			t.Fatalf("the policy names %d roles", len(roles))
		}
		a, err := setup.Found(encode(t, set), at)
		if err != nil {
			t.Fatal(err)
		}
		got := a.Roles(ptesting.Key(t, "applier").Reference())
		if want := []string{"applier"}; !slices.Equal(got, want) {
			t.Fatalf("the applier holds %q, want %q", got, want)
		}
	})
}
