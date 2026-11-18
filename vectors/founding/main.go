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

func base() kit.Obj { return kit.AskAndLog().Open() }

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

// binding is a binding signed for the ask-and-log set's policy.
func binding(actor, principal, mediator kit.Key, until string, roles ...string) kit.Obj {
	return kit.Binding(actor, principal, mediator, kit.HashOf(base()["policy"]), until, roles...)
}

// keys are every key a set of this group names, by reference.
func keys() map[string]kit.Key {
	out := map[string]kit.Key{}
	for _, k := range []kit.Key{kit.Mediator, kit.Operator, kit.Principal, kit.OtherPrincipal, kit.OtherMediator,
		kit.Proposer, kit.Applier, kit.Decider, kit.Lead, kit.Stranger} {
		out[k.Ref] = k
	}
	return out
}

// rebind signs every binding of set again, by its principal, for the set's policy as it now is, so a
// change to the policy leaves every binding good.
func rebind(set kit.Obj) kit.Obj {
	k := keys()
	var l []kit.Obj
	for _, b := range list(set, "bindings") {
		var roles []string
		for _, r := range b["roles"].([]any) {
			roles = append(roles, r.(string))
		}
		l = append(l, kit.Binding(k[b["actor"].(string)], k[b["principal"].(string)], k[b["mediator"].(string)],
			kit.HashOf(set["policy"]), b["until"].(string), roles...))
	}
	return put(set, "bindings", l, true)
}

// withEntries sets the policy's entries of set and signs every binding again for that policy.
func withEntries(set kit.Obj, es ...any) kit.Obj {
	set["policy"].(kit.Obj)["principals"] = es
	return rebind(set)
}

// twoPrincipals is the ask-and-log Accord with a principal per side: the principal may grant
// proposer and applier and binds the proposer and the applier; the other principal may grant decider
// and binds the decider.
func twoPrincipals() kit.Set {
	s := kit.AskAndLog()
	s.Grants = []kit.Grant{
		{Principal: kit.Principal, Roles: []string{"proposer", "applier"}},
		{Principal: kit.OtherPrincipal, Roles: []string{"decider"}},
	}
	s.Parties[2].By = kit.OtherPrincipal
	return s
}

// vector writes a found vector of a sealed set, given as hex when it founds and as a JSON value when
// it is refused, with the openings and the acceptances given, each left out when nil; acceptances
// given as a string are given as text.
func vector(name, tests string, sealed kit.Obj, at string, openings []any, acceptances any, expect kit.Obj) kit.Vector {
	m := kit.Obj{"foundedAt": at, "expect": expect}
	if _, ok := expect["accord"]; ok {
		m["inputHex"] = kit.HexOf(kit.Canon(sealed))
	} else {
		m["input"] = sealed
	}
	if openings != nil {
		m["openings"] = openings
	}
	switch a := acceptances.(type) {
	case nil:
	case string:
		m["acceptancesText"] = a
	default:
		m["acceptances"] = a
	}
	return kit.Vector{Name: "founding/" + name, Tests: tests, Operation: "found", Members: m}
}

// founds is the expectation of a set that founds: its Accord id and card hash.
func founds(sealed kit.Obj) kit.Obj {
	return kit.Obj{"accord": kit.HashOf(sealed), "card": kit.HashOf(sealed["card"])}
}

// accepted seals a set given in the clear and founds it with every opening and every acceptance.
func accepted(name, tests string, open kit.Obj, at string) kit.Vector {
	sealed, openings := kit.Seal(open)
	return vector(name, tests, sealed, at, openings, kit.Acceptances(sealed), founds(sealed))
}

// refused seals a set given in the clear and founds it with every opening and no acceptance, for a
// defect rows 1 to 17 find.
func refused(name, tests string, open kit.Obj, code string) kit.Vector {
	sealed, openings := kit.Seal(open)
	return vector(name, tests, sealed, kit.FoundedAt, openings, nil, kit.Obj{"code": code})
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
	}.Open()

	otherOperator := base()
	otherOperator["mediator"].(kit.Obj)["operator"] = kit.OtherMediator.Ref

	noRoles := base()
	noRoles["policy"].(kit.Obj)["roles"] = []any{}
	noRoles = rebind(noRoles)

	// Two entries that both list decider, the decider bound by the second.
	oneRoleTwice := twoPrincipals()
	oneRoleTwice.Grants[0].Roles = []string{"proposer", "applier", "decider"}

	// The proposer holds auditor beside proposer, which its principal may grant and the policy does
	// not require; the policy requires proposer alone.
	notRequired := twoPrincipals()
	notRequired.Roles = []string{"proposer"}
	notRequired.Grants = []kit.Grant{
		{Principal: kit.Principal, Roles: []string{"proposer", "auditor"}},
		{Principal: kit.OtherPrincipal, Roles: []string{"applier", "decider"}},
	}
	notRequired.Parties[0].Roles = []string{"proposer", "auditor"}
	notRequired.Parties[1].By = kit.OtherPrincipal

	// The proposer's binding by the principal, whose entry lists proposer and applier, gives decider.
	ungranted := twoPrincipals()
	ungranted.Parties[0].Roles = []string{"proposer", "decider"}
	// The proposer's binding, by the policy's principal, is signed for the policy with no roles.
	otherPolicy := replace(base(), "bindings", kit.Proposer,
		kit.Binding(kit.Proposer, kit.Principal, kit.Mediator, kit.HashOf(noRoles["policy"]), kit.Until, "proposer"))
	// The first declaration's binding gives decider, which its principal may not grant, and names
	// another mediator; then the same with another policy's hash in place of the mediator.
	twoSet := twoPrincipals().Open()
	firstTwo := keyOf(list(twoSet, "declarations")[0]["actor"].(string))
	firstBy := kit.Principal
	if firstTwo.Ref == kit.Decider.Ref {
		firstBy = kit.OtherPrincipal
	}
	ungrantedRole := "decider"
	if firstBy.Ref == kit.OtherPrincipal.Ref {
		ungrantedRole = "proposer"
	}
	roleAndMediator := replace(twoPrincipals().Open(), "bindings", firstTwo,
		kit.Binding(firstTwo, firstBy, kit.OtherMediator, kit.HashOf(twoSet["policy"]), kit.Until, ungrantedRole))
	roleAndPolicy := replace(twoPrincipals().Open(), "bindings", firstTwo,
		kit.Binding(firstTwo, firstBy, kit.Mediator, kit.HashOf(base()["policy"]), kit.Until, ungrantedRole))
	// The proposer's binding gives decider, which its principal may not grant, and its signature is
	// the proposer's good binding's, so the signature fails first.
	badSigRole := kit.Binding(kit.Proposer, kit.Principal, kit.Mediator, kit.HashOf(twoSet["policy"]), kit.Until, "proposer", "decider")
	badSigRole["sig"] = kit.Binding(kit.Proposer, kit.Principal, kit.Mediator, kit.HashOf(twoSet["policy"]), kit.Until, "proposer", "applier")["sig"]
	badSigRole = replace(twoPrincipals().Open(), "bindings", kit.Proposer, badSigRole)
	// The proposer's binding, by the policy's principal, is signed for the policy with no roles and
	// names another mediator; then the same binding expired at founding in place of the mediator.
	policyAndMediator := replace(base(), "bindings", kit.Proposer,
		kit.Binding(kit.Proposer, kit.Principal, kit.OtherMediator, kit.HashOf(noRoles["policy"]), kit.Until, "proposer"))
	policyAndExpired := replace(base(), "bindings", kit.Proposer,
		kit.Binding(kit.Proposer, kit.Principal, kit.Mediator, kit.HashOf(noRoles["policy"]), at, "proposer"))

	// Defects of shape in the policy's entries and in a binding, the bindings signed again after each
	// change to the policy, so only the shape is wrong.
	entry := func(p kit.Key, roles ...any) kit.Obj { return kit.Obj{"principal": p.Ref, "roles": roles} }
	all := []any{"proposer", "applier", "decider"}
	noPrincipals := withEntries(base())
	emptyRoles := withEntries(base(), entry(kit.Principal))
	repeatedRole := withEntries(base(), entry(kit.Principal, "proposer", "applier", "decider", "decider"))
	notALabel := withEntries(base(), entry(kit.Principal, "proposer", "applier", "Decider"))
	notAReference := withEntries(base(), kit.Obj{"principal": "principal", "roles": all})
	twoEntriesOnePrincipal := withEntries(base(), entry(kit.Principal, "proposer", "applier"), entry(kit.Principal, "decider"))
	extraMember := entry(kit.Principal, all...)
	extraMember["actors"] = []any{kit.Proposer.Ref}
	entryExtra := withEntries(base(), extraMember)
	noPolicy := kit.Unsigned(binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer"))
	delete(noPolicy, "policy")
	bindingNoPolicy := replace(base(), "bindings", kit.Proposer, kit.Signed(noPolicy, kit.TypeBinding, kit.Principal))
	policyNotHash := replace(base(), "bindings", kit.Proposer,
		kit.Binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Hash(kit.Canon(base()["policy"]))[len("sha256:"):], kit.Until, "proposer"))

	// Defects in terms, each declaration signed after the defect so only the defect is wrong.
	selector := kit.Term("want", "give", []string{"applier"}, []string{"note"})
	selector["with"] = []any{"decider"}
	maybeBare := kit.Term("maybe", "give", []string{"applier"}, []string{"proposal"})
	check := kit.Term("want", "give", []string{"applier"}, []string{"note"})
	check["checks"] = []any{kit.Obj{"name": "forbidden", "corpus": "tests"}}
	// A maybe term naming jev, a judge the ask-and-log card does not list.
	judgeTerm := kit.JudgedMaybe("give", []string{"applier"}, []string{"note"}, []string{"decider"}, 60, kit.Judges("jev", "decide"))
	judgeTwice := kit.JudgedMaybe("give", []string{"applier"}, []string{"note"}, []string{"decider"}, 60, kit.Judges("jev", "decide", "jev", "advise"))
	judgeUse := kit.JudgedMaybe("give", []string{"applier"}, []string{"note"}, []string{"decider"}, 60, kit.Judges("jev", "decides"))
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
	noDeclaration = rebind(noDeclaration)
	noBinding := remove(base(), "bindings", kit.Decider)

	cardCheck := base()
	cardCheck["card"].(kit.Obj)["checks"] = []any{kit.Obj{"name": "forbidden", "version": "0.1.0", "hash": kit.HashOf("a check")}}
	// A card listing jev twice under two hashes: a repeated name, not a repeated entry.
	judgeNameTwice := base()
	judgeNameTwice["card"].(kit.Obj)["judges"] = []any{kit.Jev, kit.Obj{"name": "jev", "version": "1.0.0", "hash": kit.HashOf("another build of jev")}}
	// A policy whose supersedes names a close that is not a typed hash, the bindings signed again.
	badClose := kit.Superseding().Open()
	badClose["policy"].(kit.Obj)["supersedes"].(kit.Obj)["close"] = "a close receipt"
	badClose = rebind(badClose)
	cardModels := base()
	cardSpec := base()
	cardSpec["card"].(kit.Obj)["spec"] = "0.2"
	cardModels["card"].(kit.Obj)["models"] = []any{}

	auditor := base()
	auditor["policy"].(kit.Obj)["roles"] = []any{"proposer", "applier", "decider", "auditor"}
	auditor = rebind(auditor)

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

	// Two defects in each pair of neighbouring rows the text orders that one set can fail together.
	// Rows 9 and 10 cannot: a declaration with no binding has no principal to read.
	containerTwoBindings := put(base(), "bindings", append(list(base(), "bindings"),
		binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer", "applier")), true)
	containerTwoBindings["mediator"].(kit.Obj)["runs"] = "container"
	twoBindingsAlice := put(base(), "bindings", append(list(base(), "bindings"),
		binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer", "applier")), true)
	aliceDecl := kit.Declaration(kit.Stranger, kit.Term("want", "get", []string{"proposer"}, []string{"note"}))
	aliceDecl["actor"] = "alice"
	twoBindingsAlice = put(twoBindingsAlice, "declarations", append(list(twoBindingsAlice, "declarations"), aliceDecl), true)
	maybeThenBadTerm := withProposerTerms(maybeBare, badTerm)
	judgeThenMaybe := withProposerTerms(judgeTerm, maybeBare)
	judgeNoBinding := remove(withProposerTerms(judgeTerm), "bindings", kit.Proposer)
	badTermNoBinding := remove(withProposerTerms(badTerm), "bindings", kit.Proposer)
	// The binding's principal is unlisted and its signature is another binding's.
	unlistedBadSig := binding(kit.Proposer, kit.OtherPrincipal, kit.Mediator, kit.Until, "proposer")
	unlistedBadSig["sig"] = binding(kit.Proposer, kit.Principal, kit.Mediator, kit.Until, "proposer")["sig"]
	unlistedBadSig = replace(base(), "bindings", kit.Proposer, unlistedBadSig)
	// The binding names another mediator and expired at founding, signed by the policy's principal.
	otherMediatorExpired := replace(base(), "bindings", kit.Proposer, binding(kit.Proposer, kit.Principal, kit.OtherMediator, at, "proposer"))
	// The proposer's binding expired and the decider's declaration is gone, its binding kept.
	expiredNoDeclaration := remove(untilAt(at), "declarations", kit.Decider)
	// The decider's declaration is gone, its binding kept, and the policy still lists its role.
	noDeclarationRole := remove(base(), "declarations", kit.Decider)

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
		accepted("compose", "setup-contract-5: the set of the composition scenarios founds", kit.Compose().Open(), at),
		accepted("own-no", "setup-contract-5: the set of the scenario in which the sender refuses founds", kit.OwnNo().Open(), at),
		accepted("long-wait", "setup-contract-5: the set whose maybe term waits the largest timeout founds", kit.LongWait().Open(), at),
		accepted("unanswered", "setup-contract-5: a set whose maybe rule names a role no binding gives founds, since only the policy's roles must be held", kit.Unanswered().Open(), at),
		accepted("judged", "setup-contract-1, setup-contract-4: the set of the judge scenarios, whose card lists jev and whose maybe terms list it to decide or to advise, founds", kit.Judged().Open(), at),
		accepted("supersedes", "setup-contract-11: a set whose policy supersedes an earlier Accord, by its id and its close receipt's id, founds", kit.Superseding().Open(), at),
		accepted("answer-under-maybe", "setup-contract-5: the set in which the asker gets answers under a maybe term founds", kit.AnswerUnderMaybe().Open(), at),
		accepted("two-waits", "setup-contract-5: the set of the close scenarios, whose two maybe terms wait 3600 and 60 seconds, founds", kit.TwoWaits().Open(), at),
		accepted("one-binding-two-roles", "setup-contract-2: one binding gives an actor two of the policy's three roles, the third held by another", twoRoles, at),
		accepted("operator-changes-the-id", "setup-contract-7: a set differing from ask-and-log only in its operator founds to another id", otherOperator, at),
		accepted("policy-without-roles", "setup-contract-5: a policy whose roles are empty founds", noRoles, at),
		accepted("two-principals", "setup-contract-2: a principal per side, one that may grant proposer and applier and binds those two, the other that may grant decider and binds the decider, founds", twoPrincipals().Open(), at),
		accepted("two-entries-one-role", "setup-contract-9: two policy entries that both list decider found, the decider bound by the second", oneRoleTwice.Open(), at),
		accepted("a-role-held-not-required", "setup-contract-5: a binding gives proposer and auditor, both its principal's entry lists, under a policy that requires proposer alone, and the set founds", notRequired.Open(), at),
		refused("binding-role-not-granted", "setup-contract-3: a binding by the principal whose entry lists proposer and applier gives decider, and is binding-role", ungranted.Open(), "binding-role"),
		refused("binding-other-policy", "setup-contract-3: a binding signed for the hash of another policy is binding-policy", otherPolicy, "binding-policy"),
		refused("ungranted-role-and-other-mediator", "setup-contract-8: the first declaration's binding gives a role its principal may not grant and names another mediator, and the role comes first", roleAndMediator, "binding-role"),
		refused("ungranted-role-and-other-policy", "setup-contract-8: a binding that gives a role its principal may not grant and names another policy's hash is binding-role, which comes first", roleAndPolicy, "binding-role"),
		refused("bad-binding-signature-and-ungranted-role", "setup-contract-8: a binding's bad signature comes before a role its principal may not grant", badSigRole, "binding-signature"),
		refused("other-policy-and-other-mediator", "setup-contract-8: a binding signed for another policy's hash is binding-policy, which comes before the other mediator it names", policyAndMediator, "binding-policy"),
		refused("other-policy-and-expired", "setup-contract-8: a binding signed for another policy's hash is binding-policy, which comes before its expiry", policyAndExpired, "binding-policy"),
		refused("principals-empty", "setup-contract-9: a policy whose principals is empty is invalid-shape", noPrincipals, "invalid-shape"),
		refused("entry-roles-empty", "setup-contract-9: a policy entry whose roles is empty is invalid-shape", emptyRoles, "invalid-shape"),
		refused("entry-roles-repeated", "setup-contract-9: a policy entry whose roles repeats a label is invalid-shape", repeatedRole, "invalid-shape"),
		refused("entry-role-not-a-label", "setup-contract-9: a policy entry with a role that is not a label is invalid-shape", notALabel, "invalid-shape"),
		refused("entry-principal-not-a-reference", "setup-contract-9: a policy entry whose principal is not an actor reference is invalid-shape", notAReference, "invalid-shape"),
		refused("two-entries-one-principal", "setup-contract-9: two policy entries for one principal are invalid-shape", twoEntriesOnePrincipal, "invalid-shape"),
		refused("entry-unlisted-member", "setup-contract-9: a policy entry with a member other than principal and roles is invalid-shape", entryExtra, "invalid-shape"),
		refused("binding-without-policy", "setup-contract-9: a binding with no policy, signed so, is invalid-shape", bindingNoPolicy, "invalid-shape"),
		refused("binding-policy-not-a-hash", "setup-contract-9: a binding whose policy is not a typed hash is invalid-shape", policyNotHash, "invalid-shape"),

		refused("selector-not-a-role", "setup-contract-1: a with selector that is not role:<label> is term-invalid", withProposerTerms(selector), "term-invalid"),
		refused("maybe-without-resolve", "setup-contract-1: a maybe term with no resolve is maybe-without-resolve", withProposerTerms(maybeBare), "maybe-without-resolve"),
		refused("term-names-a-check", "setup-contract-1: a term with checks holds a member a term does not list, and is term-invalid", withProposerTerms(check), "term-invalid"),
		refused("judge-unknown", "setup-contract-1: a maybe term whose resolve names jev, under a card whose judges are empty, is judge-unknown", withProposerTerms(judgeTerm), "judge-unknown"),
		refused("a-judge-named-twice", "setup-contract-1: a resolve whose judges list jev twice is term-invalid", withProposerTerms(judgeTwice), "term-invalid"),
		refused("a-judge-of-another-use", "setup-contract-1: a judge whose use is neither decide nor advise is term-invalid", withProposerTerms(judgeUse), "term-invalid"),
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
		refused("card-loads-a-check", "setup-contract-4: a card with a checks member, a member the card does not list, is invalid-shape", cardCheck, "invalid-shape"),
		refused("card-judge-name-twice", "setup-contract-4: a card whose judges list jev twice, under two hashes, is invalid-shape", judgeNameTwice, "invalid-shape"),
		refused("supersedes-close-not-a-hash", "setup-contract-11: a policy whose supersedes names a close that is not a typed hash is invalid-shape", badClose, "invalid-shape"),
		refused("card-unlisted-member", "setup-contract-4: a card with a member models is invalid-shape", cardModels, "invalid-shape"),
		refused("a-card-of-another-spec", "setup-contract-4: a card whose spec is 0.2, a protocol number this version does not know, is invalid-shape", cardSpec, "invalid-shape"),
		refused("policy-role-nobody-holds", "setup-contract-5: a policy role no binding gives is role-missing", auditor, "role-missing"),
		refused("runs-container", "setup-contract-7: a mediator whose runs is container is invalid-shape", container, "invalid-shape"),
		refused("two-defects-in-one-declaration", "setup-contract-8: the first declaration's bad signature comes before its expired binding", twoInOne, "declaration-signature"),
		refused("defects-in-two-declarations", "setup-contract-8: the first declaration's expired binding comes before the second's malformed term", twoDecls, "binding-expired"),
		refused("unlisted-principal-and-expired", "setup-contract-8: a binding's unlisted principal comes before its expiry", unlistedExpired, "binding-principal"),
		refused("bad-binding-signature-and-other-mediator", "setup-contract-8: a binding's bad signature comes before the other mediator it names", otherMediatorBadSig, "binding-signature"),
		refused("bad-declaration-signature-and-term", "setup-contract-8: a declaration's bad signature comes before its malformed term", editedBadTerm, "declaration-signature"),
		refused("shape-and-two-bindings-one-actor", "setup-contract-8: a set's defect of shape comes before two bindings for one actor", containerTwoBindings, "invalid-shape"),
		refused("two-bindings-one-actor-and-an-actor", "setup-contract-8: two bindings for one actor come before a declaration whose actor is no reference", twoBindingsAlice, "binding-duplicate"),
		refused("maybe-without-resolve-and-a-bad-term", "setup-contract-8: a malformed term comes before a maybe term with no resolve listed ahead of it", maybeThenBadTerm, "term-invalid"),
		refused("a-judge-and-maybe-without-resolve", "setup-contract-8: a maybe term with no resolve comes before a judge the card lacks named by a term listed ahead of it", judgeThenMaybe, "maybe-without-resolve"),
		refused("a-judge-and-no-binding", "setup-contract-8: a judge the card lacks comes before the declaration's missing binding", judgeNoBinding, "judge-unknown"),
		refused("a-bad-term-and-no-binding", "setup-contract-8: a malformed term comes before the declaration's missing binding", badTermNoBinding, "term-invalid"),
		refused("unlisted-principal-and-bad-signature", "setup-contract-8: a binding's unlisted principal comes before its bad signature", unlistedBadSig, "binding-principal"),
		refused("other-mediator-and-expired", "setup-contract-8: the other mediator a binding names comes before its expiry", otherMediatorExpired, "binding-mediator"),
		refused("expired-and-a-binding-without-declaration", "setup-contract-8: a declaration's expired binding comes before a binding with no declaration", expiredNoDeclaration, "binding-expired"),
		refused("a-binding-without-declaration-and-a-role-nobody-holds", "setup-contract-8: a binding with no declaration comes before a policy role no declaring actor holds", noDeclarationRole, "declaration-missing"),
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
	vs = append(vs, sealedVectors(at)...)
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
