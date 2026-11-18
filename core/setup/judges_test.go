package setup_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// TestCardJudgeNameTwice founds a card that lists jev twice under two hashes, which a schema's
// uniqueItems does not catch: invalid-shape at section 3 row 2. Two judges of two names found, and
// the Accord returns them in card order (setup-contract-4, U14).
func TestCardJudgeNameTwice(t *testing.T) {
	t.Parallel()
	judge := func(name, hash string) map[string]any {
		return map[string]any{"name": name, "version": "1.0.0", "hash": "sha256:" + strings.Repeat(hash, 32)}
	}
	set, at := base(t, "ask-and-log")
	set["card"].(map[string]any)["judges"] = []any{judge("jev", "ab"), judge("jev", "cd")}
	if c := foundCode(t, encode(t, set), at); c != setup.InvalidShape {
		t.Fatalf("one name twice: got %q", c)
	}
	set["card"].(map[string]any)["judges"] = []any{judge("lint", "cd"), judge("jev", "ab")}
	a, err := foundClear(t, encode(t, set), at)
	if err != nil {
		t.Fatal(err)
	}
	want := []setup.CardJudge{{Name: "lint", Version: "1.0.0", Hash: "sha256:" + strings.Repeat("cd", 32)},
		{Name: "jev", Version: "1.0.0", Hash: "sha256:" + strings.Repeat("ab", 32)}}
	if got := a.CardJudges(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CardJudges %+v, want %+v", got, want)
	}
	got := a.CardJudges()
	got[0].Name = "changed"
	if a.CardJudges()[0].Name != "lint" {
		t.Fatal("CardJudges shares the Accord's state")
	}
}

// TestTermJudges reads a maybe term's resolve.judges (setup-contract-1, U13): judges the card names
// found, in order, with their uses; a name the card lacks is judge-unknown; a name twice, a use not
// listed, an empty list and a member not listed are term-invalid, each declaration signed again.
func TestTermJudges(t *testing.T) {
	t.Parallel()
	card := []any{
		map[string]any{"name": "jev", "version": "1.0.0", "hash": aHash},
		map[string]any{"name": "lint", "version": "1.0.0", "hash": aHash},
	}
	jd := func(pairs ...string) []any {
		var out []any
		for i := 0; i+1 < len(pairs); i += 2 {
			out = append(out, map[string]any{"name": pairs[i], "use": pairs[i+1]})
		}
		return out
	}
	cases := []struct {
		name   string
		judges any
		want   setup.Code
	}{
		{"two judges", jd("lint", "advise", "jev", "decide"), founded},
		{"a judge the card lacks", jd("other", "decide"), setup.JudgeUnknown},
		{"one name twice", jd("jev", "decide", "jev", "advise"), setup.TermInvalid},
		{"a use not listed", jd("jev", "decides"), setup.TermInvalid},
		{"no judge", []any{}, setup.TermInvalid},
		{"not a list", "jev", setup.TermInvalid},
		{"a member not listed", []any{map[string]any{"name": "jev", "use": "decide", "weight": 1}}, setup.TermInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			set, at := base(t, "ask-and-log")
			set["card"].(map[string]any)["judges"] = card
			d := declarationOf(t, set, "proposer")
			termsOf(d)[0]["resolve"].(map[string]any)["judges"] = tc.judges
			sign(t, d, "proposer")
			a, err := foundClear(t, encode(t, set), at)
			if c := foundCode(t, encode(t, set), at); c != tc.want {
				t.Fatalf("got %q, want %q", c, tc.want)
			}
			if tc.want != founded {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			r := a.Terms(ptesting.Key(t, "proposer").Reference())[0].Resolve
			want := []setup.Judge{{Name: "lint", Use: setup.Advise}, {Name: "jev", Use: setup.Decide}}
			if r == nil || !reflect.DeepEqual(r.Judges, want) {
				t.Fatalf("the rule's judges: %+v", r)
			}
		})
	}
}

// TestSupersedes founds a policy that supersedes an earlier Accord, each binding signed again for
// it: the Accord names both ids; a close that is not a typed hash is invalid-shape; a set with none
// supersedes nothing (setup-contract-11, U19). The operator is the set's mediator.operator.
func TestSupersedes(t *testing.T) {
	t.Parallel()
	earlier, closed := "sha256:"+strings.Repeat("ab", 32), "sha256:"+strings.Repeat("cd", 32)
	for _, c := range []struct {
		name  string
		close any
		want  setup.Code
	}{
		{"two typed hashes", closed, founded},
		{"a close that is no typed hash", "a close receipt", setup.InvalidShape},
	} {
		set, at := base(t, "ask-and-log")
		set["policy"].(map[string]any)["supersedes"] = map[string]any{"accord": earlier, "close": c.close}
		resign(t, set)
		if got := foundCode(t, encode(t, set), at); got != c.want {
			t.Fatalf("%s: got %q, want %q", c.name, got, c.want)
		}
		if c.want != founded {
			continue
		}
		a, _ := foundClear(t, encode(t, set), at)
		if acc, cl, ok := a.Supersedes(); !ok || acc != earlier || cl != closed {
			t.Fatalf("Supersedes = %q, %q, %v", acc, cl, ok)
		}
		if a.Operator() != ptesting.Key(t, "operator").Reference() {
			t.Fatalf("Operator = %q", a.Operator())
		}
	}
	set, at := base(t, "ask-and-log")
	a, _ := foundClear(t, encode(t, set), at)
	if _, _, ok := a.Supersedes(); ok {
		t.Fatal("a policy with no supersedes supersedes something")
	}
}

// resign signs every binding of set again for its policy as it now stands, by the principal.
func resign(t *testing.T, set map[string]any) {
	t.Helper()
	h, err := canonical.TypedHash(encode(t, set["policy"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range set["bindings"].([]any) {
		b := x.(map[string]any)
		b["policy"] = h
		delete(b, "sig")
		sig, err := signing.Sign(encode(t, b), signing.TypeBinding, ptesting.Key(t, "principal"))
		if err != nil {
			t.Fatal(err)
		}
		b["sig"] = sig
	}
}
