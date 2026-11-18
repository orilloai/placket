package ptesting

import (
	"sort"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// A Party is a member of a set a test writes: the name of its vectors' key, the roles its binding
// gives it and its terms, each a term as a JSON object.
type Party struct {
	Name  string
	Roles []string
	Terms []shape.Object
}

// Term is a term of a stance and direction toward the roles with, selecting kinds (setup-contract-1).
func Term(stance, dir string, with, kinds []string) shape.Object {
	return shape.Object{"stance": stance, "dir": dir, "with": selectors(with), "select": shape.Object{"kind": strs(kinds)}}
}

// Maybe is a maybe term with its rule: the roles to and the timeout, and the judges, name and use
// pairs, when any are given.
func Maybe(dir string, with, kinds, to []string, timeout int64, judges ...string) shape.Object {
	t := Term("maybe", dir, with, kinds)
	r := shape.Object{"to": selectors(to), "timeout": timeout}
	if len(judges) > 0 {
		var l []any
		for i := 0; i+1 < len(judges); i += 2 {
			l = append(l, shape.Object{"name": judges[i], "use": judges[i+1]})
		}
		r["judges"] = l
	}
	t["resolve"] = r
	return t
}

func selectors(roles []string) []any {
	out := make([]any, len(roles))
	for i, r := range roles {
		out[i] = "role:" + r
	}
	return out
}

func strs(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}

// ClearSet writes a founding set with its declarations in the clear, for Sealed: one principal, the
// vectors' key principal, may grant every role and binds every party until 2099; the mediator is the
// vectors' mediator, run by operator; the card lists the judges given, each a name pinned at a
// version and hash, and the policy requires no role.
func ClearSet(t testing.TB, parties []Party, judges ...string) []byte {
	t.Helper()
	med, op, pr := Key(t, "mediator"), Key(t, "operator"), Key(t, "principal")
	var all []string
	for _, p := range parties {
		for _, r := range p.Roles {
			if !contains(all, r) {
				all = append(all, r)
			}
		}
	}
	policy := shape.Object{"roles": []any{}, "principals": []any{shape.Object{"principal": pr.Reference(), "roles": strs(all)}}}
	policyHash, err := canonical.TypedHash(shape.Encode(policy))
	if err != nil {
		t.Fatal(err)
	}
	var cardJudges []any
	for _, j := range judges {
		cardJudges = append(cardJudges, shape.Object{"name": j, "version": "1.0.0", "hash": canonical.HashPrefix + "ab" + hex62})
	}
	if cardJudges == nil {
		cardJudges = []any{}
	}
	type pair struct{ decl, bind shape.Object }
	var ps []pair
	for _, p := range parties {
		k := Key(t, p.Name)
		terms := make([]any, len(p.Terms))
		for i, x := range p.Terms {
			terms[i] = x
		}
		ps = append(ps, pair{
			signedObject(t, shape.Object{"actor": k.Reference(), "terms": terms}, signing.TypeDeclaration, k),
			signedObject(t, shape.Object{"actor": k.Reference(), "principal": pr.Reference(), "roles": strs(p.Roles),
				"policy": policyHash, "mediator": med.Reference(), "until": "2099-01-01T00:00:00Z"}, signing.TypeBinding, pr),
		})
	}
	sort.Slice(ps, func(i, j int) bool { return canonical.Less(ps[i].decl["actor"].(string), ps[j].decl["actor"].(string)) })
	var decls, binds []any
	for _, p := range ps {
		decls, binds = append(decls, p.decl), append(binds, p.bind)
	}
	return shape.Encode(shape.Object{
		"policy":   policy,
		"mediator": shape.Object{"actor": med.Reference(), "operator": op.Reference(), "runs": "process"},
		"card": shape.Object{"spec": "0.1", "kernel": shape.Object{"version": "0.1.0", "hash": canonical.HashPrefix + "ab" + hex62},
			"judges": cardJudges},
		"declarations": decls, "bindings": binds,
	})
}

// hex62 is 62 lowercase hex digits, so "ab" before it makes a digest's 64.
const hex62 = "00000000000000000000000000000000000000000000000000000000000000"

func signedObject(t testing.TB, o shape.Object, typeURI string, k signing.Key) shape.Object {
	t.Helper()
	sig, err := signing.Sign(shape.Encode(o), typeURI, k)
	if err != nil {
		t.Fatal(err)
	}
	o["sig"] = sig
	return o
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
