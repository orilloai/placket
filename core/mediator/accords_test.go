package mediator_test

import (
	"encoding/json"
	"slices"
	"sort"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

// A party of a founding set a test builds: its key's name, the roles its binding gives it and its
// terms.
type party struct {
	name  string
	roles []string
	terms []map[string]any
}

func term(stance, dir string, with, kinds []string) map[string]any {
	return map[string]any{"stance": stance, "dir": dir, "with": selectors(with), "select": map[string]any{"kind": kinds}}
}

func maybe(dir string, with, kinds, to []string, timeout int64) map[string]any {
	t := term("maybe", dir, with, kinds)
	t["resolve"] = map[string]any{"to": selectors(to), "timeout": timeout}
	return t
}

func selectors(roles []string) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = "role:" + r
	}
	return out
}

// signed returns o with its sig, by k under typeURI.
func signed(t testing.TB, o map[string]any, typeURI string, k signing.Key) map[string]any {
	t.Helper()
	b, _ := json.Marshal(o)
	sig, err := signing.Sign(b, typeURI, k)
	if err != nil {
		t.Fatal(err)
	}
	c := map[string]any{"sig": sig}
	for n, v := range o {
		c[n] = v
	}
	return c
}

// buildSet builds a founding set of the parties under the policy's roles, with the mediator, the operator
// and the principal every test set uses, both lists in actor-reference order (setup-contract-10). The
// policy has one entry, the principal, which may grant the policy's roles and every party's, and each
// binding names the typed hash of that policy (setup-contract-2).
func buildSet(t testing.TB, roles []string, parties ...party) []byte {
	t.Helper()
	med, op, pr := key(t, "mediator"), key(t, "operator"), key(t, "principal")
	grants := append([]string{}, roles...)
	for _, p := range parties {
		for _, r := range p.roles {
			if !slices.Contains(grants, r) {
				grants = append(grants, r)
			}
		}
	}
	policy := map[string]any{"roles": append([]string{}, roles...),
		"principals": []any{map[string]any{"principal": pr.Reference(), "roles": grants}}}
	pb, _ := json.Marshal(policy)
	policyHash, err := canonical.TypedHash(pb)
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		actor      string
		decl, bind map[string]any
	}
	var entries []entry
	for _, p := range parties {
		k := key(t, p.name)
		terms := make([]any, len(p.terms))
		for i, x := range p.terms {
			terms[i] = x
		}
		entries = append(entries, entry{
			actor: k.Reference(),
			decl:  signed(t, map[string]any{"actor": k.Reference(), "terms": terms}, signing.TypeDeclaration, k),
			bind: signed(t, map[string]any{
				"actor": k.Reference(), "principal": pr.Reference(), "roles": p.roles, "policy": policyHash,
				"mediator": med.Reference(), "until": "2027-11-12T00:00:00Z",
			}, signing.TypeBinding, pr),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return canonical.Less(entries[i].actor, entries[j].actor) })
	var decls, binds []any
	for _, e := range entries {
		decls = append(decls, e.decl)
		binds = append(binds, e.bind)
	}
	kernel, _ := canonical.TypedHash([]byte(`"placket reference decision module 0.1"`))
	b, _ := json.Marshal(map[string]any{
		"policy":   policy,
		"mediator": map[string]any{"actor": med.Reference(), "operator": op.Reference(), "runs": "process"},
		"card": map[string]any{"spec": "0.1", "kernel": map[string]any{"version": "0.1.0", "hash": kernel},
			"judges": []any{}, "checks": []any{}},
		"declarations": decls,
		"bindings":     binds,
	})
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// askAndLogParties are the ask-and-log Accord's parties, as vectors/founding/ask-and-log.json holds
// them: the proposer asks the decider about a proposal to the applier within timeout seconds, gives
// notes freely, and gets answers and changed versions from the decider; the applier gets proposals,
// notes and changed versions; the decider gives answers and changed versions to both.
func askAndLogParties(timeout int64) []party {
	return []party{
		{"proposer", []string{"proposer"}, []map[string]any{
			maybe("give", []string{"applier"}, []string{"proposal"}, []string{"decider"}, timeout),
			term("want", "give", []string{"applier"}, []string{"note"}),
			term("want", "get", []string{"decider"}, []string{"answer", "changed-version"}),
		}},
		{"applier", []string{"applier"}, []map[string]any{
			term("want", "get", []string{"proposer"}, []string{"proposal", "note"}),
			term("want", "get", []string{"decider"}, []string{"changed-version"}),
		}},
		{"decider", []string{"decider"}, []map[string]any{
			term("want", "give", []string{"proposer", "applier"}, []string{"answer", "changed-version"}),
		}},
	}
}

var askAndLogRoles = []string{"proposer", "applier", "decider"}

// foundSet founds a set a test built, at the founding time of the vectors.
func foundSet(t testing.TB, s []byte) *mediator.Mediator {
	t.Helper()
	m, err := mediator.Found(s, key(t, "mediator"), "2026-11-12T08:00:00Z")
	if err != nil {
		t.Fatalf("the test's set does not found: %v", err)
	}
	return m
}
