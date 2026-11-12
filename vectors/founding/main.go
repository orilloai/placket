// Command founding writes the vectors of the founding group: section 3 of the protocol, founding an
// Accord from a founding set. Each refusal's code is written here by hand from founding's check
// table; the generator builds and signs the sets and computes the Accord ids and card hashes with the
// Go standard library alone (vectors/README.md).
//
// It imports no package of the core.
package main

import (
	"github.com/orilloai/placket/vectors/signatures/kit"
)

// A refusal set differs from the ask-and-log set in one defect, unless its vector says otherwise.

func base() kit.Obj { return kit.AskAndLog().Build() }

// list returns a set's list member as objects.
func list(set kit.Obj, name string) []kit.Obj {
	var out []kit.Obj
	for _, x := range set[name].([]any) {
		out = append(out, x.(kit.Obj))
	}
	return out
}

func put(set kit.Obj, name string, l []kit.Obj, sorted bool) kit.Obj {
	if sorted {
		kit.SortByActor(l)
	}
	a := make([]any, len(l))
	for i, x := range l {
		a[i] = x
	}
	set[name] = a
	return set
}

// replace swaps the object of one actor in a list for another.
func replace(set kit.Obj, name string, actor kit.Key, with kit.Obj) kit.Obj {
	l := list(set, name)
	for i, x := range l {
		if x["actor"] == actor.Ref {
			l[i] = with
		}
	}
	return put(set, name, l, true)
}

// remove drops the object of one actor from a list.
func remove(set kit.Obj, name string, actor kit.Key) kit.Obj {
	var l []kit.Obj
	for _, x := range list(set, name) {
		if x["actor"] != actor.Ref {
			l = append(l, x)
		}
	}
	return put(set, name, l, true)
}

// terms is the proposer's terms in the ask-and-log Accord.
func proposerTerms() []kit.Obj { return kit.AskAndLog().Parties[0].Terms }

func withProposerTerms(terms ...kit.Obj) kit.Obj {
	return replace(base(), "declarations", kit.Proposer, kit.Declaration(kit.Proposer, terms...))
}

func binding(actor, principal, mediator kit.Key, until string, roles ...string) kit.Obj {
	return kit.Binding(actor, principal, mediator, until, roles...)
}

func accepted(name, tests string, set kit.Obj, at string) kit.Vector {
	return kit.Vector{Name: "founding/" + name, Tests: tests, Operation: "found", Members: kit.Obj{
		"inputHex": kit.HexOf(kit.Canon(set)), "foundedAt": at,
		"expect": kit.Obj{"accord": kit.HashOf(set), "card": kit.HashOf(set["card"])},
	}}
}

func refused(name, tests string, set kit.Obj, code string) kit.Vector {
	return kit.Vector{Name: "founding/" + name, Tests: tests, Operation: "found", Members: kit.Obj{
		"input": set, "foundedAt": kit.FoundedAt, "expect": kit.Obj{"code": code},
	}}
}

func main() {
	at := kit.FoundedAt
	orderEight := "ed25519:26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05"

	// One binding giving the proposer two of the policy's three roles, the third held by the applier.
	twoRoles := kit.Set{
		Mediator: kit.Mediator, Operator: kit.Operator, Principal: kit.Principal,
		Roles: []string{"proposer", "applier", "decider"},
		Parties: []kit.Party{
			{Key: kit.Proposer, Roles: []string{"proposer", "decider"}, Terms: proposerTerms()},
			{Key: kit.Applier, Roles: []string{"applier"}, Terms: kit.AskAndLog().Parties[1].Terms},
		},
	}.Build()

	otherOperator := base()
	otherOperator["mediator"].(kit.Obj)["operator"] = kit.OtherMediator.Ref

	noRoles := base()
	noRoles["policy"].(kit.Obj)["roles"] = []any{}

	// Defects in terms, each declaration signed after the defect so only the defect is wrong.
	selector := kit.Term("want", "give", []string{"applier"}, []string{"note"})
	selector["with"] = []any{"decider"}
	maybeBare := kit.Term("maybe", "give", []string{"applier"}, []string{"proposal"})
	check := kit.Term("want", "give", []string{"applier"}, []string{"note"})
	check["checks"] = []any{kit.Obj{"name": "forbidden", "corpus": "tests"}}
	noDir := kit.Term("want", "give", []string{"applier"}, []string{"note"})
	delete(noDir, "dir")
	resolveOnWant := kit.Term("want", "give", []string{"applier"}, []string{"note"})
	resolveOnWant["resolve"] = kit.Obj{"to": []any{"role:decider"}, "timeout": 60}

	edited := base()
	for _, d := range list(edited, "declarations") {
		if d["actor"] == kit.Proposer.Ref {
			d["terms"].([]any)[1].(kit.Obj)["stance"] = "need"
		}
	}

	untilAt := func(until string) kit.Obj {
		return replace(base(), "bindings", kit.Proposer, binding(kit.Proposer, kit.Principal, kit.Mediator, until, "proposer"))
	}

	unlisted := replace(base(), "bindings", kit.Proposer,
		binding(kit.Proposer, kit.OtherPrincipal, kit.Mediator, kit.Until, "proposer"))
	otherMediator := replace(base(), "bindings", kit.Proposer,
		binding(kit.Proposer, kit.Principal, kit.OtherMediator, kit.Until, "proposer"))
	// Two declarations and three bindings: the decider's declaration is gone and its role is not in
	// the policy, so only the binding with no declaration is wrong.
	noDeclaration := remove(base(), "declarations", kit.Decider)
	noDeclaration["policy"].(kit.Obj)["roles"] = []any{"proposer", "applier"}
	noBinding := remove(base(), "bindings", kit.Decider)

	cardCheck := base()
	cardCheck["card"].(kit.Obj)["checks"] = []any{kit.Obj{"name": "forbidden", "version": "0.1.0", "hash": kit.HashOf("a check")}}
	cardModels := base()
	cardModels["card"].(kit.Obj)["models"] = []any{}

	auditor := base()
	auditor["policy"].(kit.Obj)["roles"] = []any{"proposer", "applier", "decider", "auditor"}

	container := base()
	container["mediator"].(kit.Obj)["runs"] = "container"

	// The first declaration in actor-reference order, with a bad signature and an expired binding.
	first := list(base(), "declarations")[0]
	firstKey := keyOf(first["actor"].(string))
	twoInOne := replace(base(), "bindings", firstKey, binding(firstKey, kit.Principal, kit.Mediator, at, roleOf(firstKey)))
	for _, d := range list(twoInOne, "declarations") {
		if d["actor"] == firstKey.Ref {
			d["sig"] = list(base(), "declarations")[1]["sig"]
		}
	}
	// The first declaration's binding expired and the second declaration's term malformed.
	second := keyOf(list(base(), "declarations")[1]["actor"].(string))
	twoDecls := replace(base(), "bindings", firstKey, binding(firstKey, kit.Principal, kit.Mediator, at, roleOf(firstKey)))
	badTerm := kit.Term("want", "get", []string{"proposer"}, []string{"note"})
	badTerm["stance"] = "perhaps"
	twoDecls = replace(twoDecls, "declarations", second, kit.Declaration(second, badTerm))

	// Two defects in one binding, or in one declaration, each pair of rows the text orders.
	// The binding's principal is unlisted and it expired at founding; other-principal signed it.
	unlistedExpired := replace(base(), "bindings", kit.Proposer, binding(kit.Proposer, kit.OtherPrincipal, kit.Mediator, at, "proposer"))
	// The binding names another mediator and its signature is another binding's.
	otherMediatorBadSig := binding(kit.Proposer, kit.Principal, kit.OtherMediator, kit.Until, "proposer")
	otherMediatorBadSig["sig"] = binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer")["sig"]
	otherMediatorBadSig = replace(base(), "bindings", kit.Proposer, otherMediatorBadSig)
	// The declaration's term became malformed after it was signed.
	editedBadTerm := base()
	for _, d := range list(editedBadTerm, "declarations") {
		if d["actor"] == kit.Proposer.Ref {
			d["terms"].([]any)[1].(kit.Obj)["stance"] = "perhaps"
		}
	}

	actorNumber := base()
	numberDecl := kit.Declaration(kit.Stranger, kit.Term("want", "get", []string{"proposer"}, []string{"note"}))
	numberDecl["actor"] = 7
	l := list(actorNumber, "declarations")
	actorNumber = put(actorNumber, "declarations", append([]kit.Obj{numberDecl}, l...), false)

	actorName := base()
	nameDecl := kit.Declaration(kit.Stranger, kit.Term("want", "get", []string{"proposer"}, []string{"note"}))
	nameDecl["actor"] = "alice"
	actorName = put(actorName, "declarations", append(list(actorName, "declarations"), nameDecl), true)

	outOfOrder := base()
	ds := list(outOfOrder, "declarations")
	for i, j := 0, len(ds)-1; i < j; i, j = i+1, j-1 {
		ds[i], ds[j] = ds[j], ds[i]
	}
	outOfOrder = put(outOfOrder, "declarations", ds, false)

	twoBindings := base()
	twoBindings = put(twoBindings, "bindings", append(list(twoBindings, "bindings"),
		binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer", "applier")), true)

	smallOrder := base()
	so := kit.Declaration(kit.Stranger, kit.Term("want", "get", []string{"proposer"}, []string{"note"}))
	so["actor"] = orderEight
	smallOrder = put(smallOrder, "declarations", append(list(smallOrder, "declarations"), so), true)

	wrongURI := replace(base(), "bindings", kit.Proposer, kit.Signed(kit.Unsigned(binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer")),
		kit.TypeDeclaration, kit.Principal))

	vs := []kit.Vector{
		accepted("ask-and-log", "setup-contract-5: the ask-and-log set founds to the typed hash of its canonical bytes", base(), at),
		accepted("ask-and-log-later", "setup-contract-5: the same set founded at another time founds to the same Accord id", base(), "2027-06-01T12:30:00Z"),
		accepted("compose", "setup-contract-5: the set of the composition scenarios founds", kit.Compose().Build(), at),
		accepted("one-binding-two-roles", "setup-contract-2: one binding gives an actor two of the policy's three roles, the third held by another", twoRoles, at),
		accepted("operator-changes-the-id", "setup-contract-7: a set differing from ask-and-log only in its operator founds to another id", otherOperator, at),
		accepted("policy-without-roles", "setup-contract-5: a policy whose roles are empty founds", noRoles, at),

		refused("selector-not-a-role", "setup-contract-1: a with selector that is not role:<label> is term-invalid", withProposerTerms(selector), "term-invalid"),
		refused("maybe-without-resolve", "setup-contract-1: a maybe term with no resolve is maybe-without-resolve", withProposerTerms(maybeBare), "maybe-without-resolve"),
		refused("term-names-a-check", "setup-contract-1: a term that names a check is check-unknown, since the card loads no check", withProposerTerms(check), "check-unknown"),
		refused("resolve-on-a-want", "setup-contract-1: a resolve on a term that is not a maybe is term-invalid", withProposerTerms(resolveOnWant), "term-invalid"),
		refused("declaration-edited", "setup-contract-1: a declaration whose terms changed after it was signed is declaration-signature", edited, "declaration-signature"),
		refused("binding-expires-at-founding", "setup-contract-2: a binding whose until equals the founding time is binding-expired", untilAt(at), "binding-expired"),
		refused("until-second-60", "setup-contract-2: an until with second 60 is invalid-shape", untilAt("2026-12-31T23:59:60Z"), "invalid-shape"),
		refused("until-hour-24", "setup-contract-2: an until with hour 24 is invalid-shape", untilAt("2026-12-31T24:00:00Z"), "invalid-shape"),
		refused("until-day-not-in-calendar", "setup-contract-2: an until on a day the calendar lacks is invalid-shape", untilAt("2027-02-29T00:00:00Z"), "invalid-shape"),
		refused("binding-unlisted-principal", "setup-contract-3: a binding signed by a principal the policy does not list is binding-principal", unlisted, "binding-principal"),
		refused("binding-other-mediator", "setup-contract-3: a binding naming another mediator is binding-mediator", otherMediator, "binding-mediator"),
		refused("binding-without-declaration", "setup-contract-3: two declarations and three bindings is declaration-missing", noDeclaration, "declaration-missing"),
		refused("declaration-without-binding", "setup-contract-3: a declaration with no binding is binding-missing", noBinding, "binding-missing"),
		refused("card-loads-a-check", "setup-contract-4: a card whose checks lists one check is invalid-shape", cardCheck, "invalid-shape"),
		refused("card-unlisted-member", "setup-contract-4: a card with a member models is invalid-shape", cardModels, "invalid-shape"),
		refused("policy-role-nobody-holds", "setup-contract-5: a policy role no binding gives is role-missing", auditor, "role-missing"),
		refused("runs-container", "setup-contract-7: a mediator whose runs is container is invalid-shape", container, "invalid-shape"),
		refused("two-defects-in-one-declaration", "setup-contract-8: the first declaration's bad signature comes before its expired binding", twoInOne, "declaration-signature"),
		refused("defects-in-two-declarations", "setup-contract-8: the first declaration's expired binding comes before the second's malformed term", twoDecls, "binding-expired"),
		refused("unlisted-principal-and-expired", "setup-contract-8: a binding's unlisted principal comes before its expiry", unlistedExpired, "binding-principal"),
		refused("bad-binding-signature-and-other-mediator", "setup-contract-8: a binding's bad signature comes before the other mediator it names", otherMediatorBadSig, "binding-signature"),
		refused("bad-declaration-signature-and-term", "setup-contract-8: a declaration's bad signature comes before its malformed term", editedBadTerm, "declaration-signature"),
		{Name: "founding/repeated-member", Tests: "setup-contract-9: a repeated member name is not-canonical", Operation: "found", Members: kit.Obj{
			"inputText": `{"policy":{},"policy":{}}`, "foundedAt": at, "expect": kit.Obj{"code": "not-canonical"}}},
		refused("actor-a-number", "setup-contract-9: a declaration's actor that is the number 7 is invalid-shape", actorNumber, "invalid-shape"),
		refused("actor-not-a-reference", "setup-contract-9: a declaration's actor that is the string alice is actor-reference", actorName, "actor-reference"),
		refused("term-missing-dir", "setup-contract-9: a term with no dir is term-invalid, not invalid-shape", withProposerTerms(noDir), "term-invalid"),
		refused("declarations-out-of-order", "setup-contract-10: declarations in descending order of actor are invalid-shape", outOfOrder, "invalid-shape"),
		refused("two-bindings-one-actor", "setup-contract-10: two bindings for one actor, next to each other, are binding-duplicate", twoBindings, "binding-duplicate"),
		refused("small-order-actor", "signing-1: a declaration whose actor is a point of order 8 is declaration-signature", smallOrder, "declaration-signature"),
		refused("binding-under-declaration-uri", "signing-2: a binding signed under the declaration type URI is binding-signature", wrongURI, "binding-signature"),
	}
	kit.Main("founding", vs)
}

func keyOf(ref string) kit.Key {
	for _, k := range []kit.Key{kit.Proposer, kit.Applier, kit.Decider} {
		if k.Ref == ref {
			return k
		}
	}
	panic("founding: no key for " + ref)
}

func roleOf(k kit.Key) string {
	for _, p := range kit.AskAndLog().Parties {
		if p.Key.Ref == k.Ref {
			return p.Roles[0]
		}
	}
	panic("founding: no role for " + k.Name)
}
