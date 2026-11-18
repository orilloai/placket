package main

import (
	"sort"

	"github.com/orilloai/placket/vectors/signatures/kit"
)

// openingOf returns the opening, as a vector lists it, of one actor.
func openingOf(openings []any, actor kit.Key) kit.Obj {
	for _, o := range openings {
		if o.(kit.Obj)["actor"] == actor.Ref {
			return o.(kit.Obj)
		}
	}
	panic("founding: no opening of " + actor.Name)
}

// withOpening replaces one actor's opening by another item, or drops it when with is nil.
func withOpening(openings []any, actor kit.Key, with any) []any {
	out := []any{}
	for _, o := range kit.Copy(openings).([]any) {
		if o.(kit.Obj)["actor"] != actor.Ref {
			out = append(out, o)
		} else if with != nil {
			out = append(out, with)
		}
	}
	return out
}

// sortByFrom sorts acceptances by from, in actor-reference order.
func sortByFrom(l []any) {
	sort.SliceStable(l, func(i, j int) bool {
		return kit.Less16(l[i].(kit.Obj)["from"].(string), l[j].(kit.Obj)["from"].(string))
	})
}

// sealedVectors are the vectors of the seal, the opening and the acceptances (setup-contract-12,
// setup-contract-13, setup-contract-15, setup-contract-16), each on the ask-and-log set, sealed, with
// one defect. Each code is written here by hand from founding's check table.
func sealedVectors(at string) []kit.Vector {
	sealed, openings := kit.Seal(base())
	acceptances := kit.Acceptances(sealed)
	members := kit.ViewMembers(base())
	first, second := kit.KeyOf(members[0]), kit.KeyOf(members[1])
	decl := openingOf(openings, kit.Proposer)["opening"].(kit.Obj)["declaration"].(kit.Obj)
	salt := kit.SealSalt(kit.Proposer.Ref)
	code := func(c string) kit.Obj { return kit.Obj{"code": c} }
	var vs []kit.Vector
	// opening adds a vector whose proposer's opening is the item given, or none, with every
	// acceptance.
	opening := func(name, tests string, item any, c string) {
		vs = append(vs, vector(name, tests, sealed, at, withOpening(openings, kit.Proposer, item), acceptances, code(c)))
	}
	// list adds a vector of every opening and the acceptance list given.
	list := func(name, tests string, l any, c string) {
		vs = append(vs, vector(name, tests, sealed, at, openings, l, code(c)))
	}
	resign := func(o kit.Obj, k kit.Key) kit.Obj { return kit.Signed(kit.Unsigned(o), kit.TypeAcceptance, k) }
	with := func(i int, a any) []any {
		l := kit.Copy(acceptances).([]any)
		l[i] = a
		return l
	}
	firstAcc := func() kit.Obj { return kit.Copy(acceptances[0]).(kit.Obj) }

	// Row 4a (setup-contract-13).
	changed := kit.Copy(decl).(kit.Obj)
	changed["terms"].([]any)[1].(kit.Obj)["select"].(kit.Obj)["kind"] = []any{"nose"}
	opening("opening-one-byte-changed", "setup-contract-13: an opening whose declaration differs from the sealed one in one byte, note made nose, is seal-mismatch",
		kit.Item(kit.Proposer.Ref, kit.Opening(changed, salt)), "seal-mismatch")
	opening("opening-another-salt", "setup-contract-13: an opening whose salt is another 32 bytes is seal-mismatch",
		kit.Item(kit.Proposer.Ref, kit.Opening(decl, kit.Salt("founding another seal salt", 32))), "seal-mismatch")
	opening("opening-of-another-member", "setup-contract-13: the applier's opening held for the proposer's seal names another actor, and is seal-mismatch",
		kit.Item(kit.Proposer.Ref, openingOf(openings, kit.Applier)["opening"]), "seal-mismatch")
	opening("opening-missing", "setup-contract-13: a founder holding every opening but the proposer's is opening-missing", nil, "opening-missing")
	extra := kit.Opening(decl, salt)
	extra["note"] = "mine"
	opening("opening-extra-member", "setup-contract-13: an opening with a member besides declaration and salt is invalid-shape",
		kit.Item(kit.Proposer.Ref, extra), "invalid-shape")
	opening("opening-salt-31-bytes", "setup-contract-13: an opening whose salt is 31 bytes is invalid-shape",
		kit.Item(kit.Proposer.Ref, kit.Opening(decl, salt[:62])), "invalid-shape")
	declExtra := kit.Copy(decl).(kit.Obj)
	declExtra["note"] = "mine"
	opening("opening-declaration-extra-member", "setup-contract-13: an opened declaration with a member besides actor, terms and sig is invalid-shape",
		kit.Item(kit.Proposer.Ref, kit.Opening(declExtra, salt)), "invalid-shape")
	numbered := kit.Copy(decl).(kit.Obj)
	numbered["actor"] = int64(7)
	opening("opening-actor-a-number", "setup-contract-13: an opened declaration whose actor is the number 7 is invalid-shape",
		kit.Item(kit.Proposer.Ref, kit.Opening(numbered, salt)), "invalid-shape")
	dup := `{"declaration":` + string(kit.Canon(decl)) + `,"salt":"` + salt + `","salt":"` + salt + `"}`
	opening("opening-not-canonical", "setup-contract-13: an opening whose bytes repeat a member name, which the canonical form refuses, is invalid-shape",
		kit.Obj{"actor": kit.Proposer.Ref, "openingHex": kit.HexOf([]byte(dup))}, "invalid-shape")
	maybeBare := kit.Term("maybe", "give", []string{"applier"}, []string{"proposal"})
	bareSealed, bareOpenings := kit.Seal(withProposerTerms(maybeBare))
	vs = append(vs, vector("opening-maybe-without-resolve", "setup-contract-13, setup-contract-1: rows 5 to 8 read the opened declaration, so a maybe term with no resolve inside an opening that opens its seal is maybe-without-resolve",
		bareSealed, at, bareOpenings, nil, code("maybe-without-resolve")))

	// Row 2 reads each seal entry (setup-contract-9, setup-contract-12).
	entryExtra := kit.Copy(sealed).(kit.Obj)
	entryExtra["declarations"].([]any)[0].(kit.Obj)["terms"] = []any{}
	vs = append(vs, vector("seal-entry-extra-member", "setup-contract-9, setup-contract-12: a seal entry with a member besides actor and seal is invalid-shape",
		entryExtra, at, openings, nil, code("invalid-shape")))
	notHash := kit.Copy(sealed).(kit.Obj)
	e0 := notHash["declarations"].([]any)[0].(kit.Obj)
	e0["seal"] = e0["seal"].(string)[len("sha256:"):]
	vs = append(vs, vector("seal-not-a-hash", "setup-contract-12: a seal that is not a typed hash is invalid-shape", notHash, at, openings, nil, code("invalid-shape")))

	// Founding without the openings (setup-contract-16).
	notChecked := founds(sealed)
	notChecked["terms"] = "not-checked"
	vs = append(vs, vector("without-openings", "setup-contract-16: the sealed ask-and-log set with its acceptances and no opening founds to its id, its terms not checked",
		sealed, at, nil, acceptances, notChecked))
	guessed := kit.Signed(kit.Obj{"accord": kit.HashOf(sealed), "view": kit.HashOf("founding: a view guessed"), "from": first.Ref}, kit.TypeAcceptance, first)
	vs = append(vs, vector("without-openings-a-view-not-checked", "setup-contract-16: with no opening row 21 is not run, so an acceptance of a view other than its member's founds, its terms not checked",
		sealed, at, nil, with(0, guessed), notChecked))
	expired, _ := kit.Seal(replace(base(), "bindings", kit.Proposer, binding(kit.Proposer, kit.Principal, kit.Mediator, at, "proposer")))
	vs = append(vs, vector("without-openings-a-binding-expired", "setup-contract-16: with no opening the bindings are still checked, and one that expires at the founding time is binding-expired",
		expired, at, nil, nil, code("binding-expired")))
	vs = append(vs, vector("without-openings-a-forged-acceptance", "setup-contract-16: with no opening the acceptances are still checked, and one signed by another key is acceptance-signature",
		sealed, at, nil, with(0, func() kit.Obj { f := firstAcc(); f["sig"] = resign(f, second)["sig"]; return f }()), code("acceptance-signature")))

	// Rows 18 to 21 and the list before them (setup-contract-15).
	list("acceptance-missing", "setup-contract-15: a list with no acceptance from the second member in actor-reference order is acceptance-missing",
		[]any{kit.Copy(acceptances[0]), kit.Copy(acceptances[2])}, "acceptance-missing")
	forged := firstAcc()
	forged["sig"] = resign(forged, second)["sig"]
	list("acceptance-signed-by-another-key", "setup-contract-15: an acceptance from the first member signed by the second member's key is acceptance-signature",
		with(0, forged), "acceptance-signature")
	sigForm := firstAcc()
	sigForm["sig"] = "acceptance"
	list("acceptance-sig-not-hex", "setup-contract-15: an acceptance whose sig is not 128 lowercase hex is acceptance-signature, since row 19 reads the sig's form",
		with(0, sigForm), "acceptance-signature")
	otherAccord := firstAcc()
	otherAccord["accord"] = kit.HashOf(kit.Compose().Build())
	list("acceptance-of-another-accord", "setup-contract-15: an acceptance naming another Accord's id, signed by its member, is acceptance-accord",
		with(0, resign(otherAccord, first)), "acceptance-accord")
	list("acceptance-of-another-view", "setup-contract-15: an acceptance naming a view other than the member's recomputed one, signed by it, is acceptance-view",
		with(0, guessed), "acceptance-view")
	reversed := kit.Copy(acceptances).([]any)
	reversed[0], reversed[1] = reversed[1], reversed[0]
	list("acceptances-out-of-order", "setup-contract-15: a list not in strict from order is invalid-shape", reversed, "invalid-shape")
	list("acceptances-repeated", "setup-contract-15: a list holding one member's acceptance twice is invalid-shape",
		append([]any{kit.Copy(acceptances[0])}, kit.Copy(acceptances).([]any)...), "invalid-shape")
	stranger := kit.Signed(kit.Obj{"accord": kit.HashOf(sealed), "view": kit.HashOf("founding: a stranger's view"), "from": kit.Stranger.Ref}, kit.TypeAcceptance, kit.Stranger)
	withStranger := append(kit.Copy(acceptances).([]any), stranger)
	sortByFrom(withStranger)
	list("acceptance-from-a-stranger", "setup-contract-15: a list holding an acceptance from a key that is no member is invalid-shape", withStranger, "invalid-shape")
	more := firstAcc()
	more["at"] = at
	list("acceptance-extra-member", "setup-contract-15: an acceptance with a member besides accord, view, from and sig is invalid-shape", with(0, resign(more, first)), "invalid-shape")
	viewNotHash := firstAcc()
	viewNotHash["view"] = "a view"
	list("acceptance-view-not-a-hash", "setup-contract-15: an acceptance whose view is not a typed hash is invalid-shape", with(0, resign(viewNotHash, first)), "invalid-shape")
	list("acceptances-not-canonical", "setup-contract-15: acceptance bytes that repeat a member name are not-canonical",
		`[{"accord":"`+kit.HashOf(sealed)+`","accord":"`+kit.HashOf(sealed)+`"}]`, "not-canonical")
	list("acceptances-not-given", "setup-contract-15: a founder given every opening and no acceptance list has no bytes to read as the list, which the canonical form refuses: not-canonical",
		nil, "not-canonical")
	twoBad := with(0, guessed)
	f2 := kit.Copy(acceptances[1]).(kit.Obj)
	f2["sig"] = resign(kit.Copy(acceptances[1]).(kit.Obj), first)["sig"]
	twoBad[1] = f2
	list("acceptances-two-failing", "setup-contract-15, setup-contract-8: the first member's acceptance of another view comes before the second member's acceptance signed by another key, since all of one member's rows run first",
		twoBad, "acceptance-view")
	roleOpen := base()
	roleOpen["policy"].(kit.Obj)["roles"] = []any{"proposer", "applier", "decider", "auditor"}
	roleSealed, roleOpenings := kit.Seal(rebind(roleOpen))
	vs = append(vs, vector("a-role-nobody-holds-and-acceptances-not-canonical", "setup-contract-8, setup-contract-15: a policy role nobody holds, row 17, comes before an acceptance list that is not canonical",
		roleSealed, at, roleOpenings, "[1.5]", code("role-missing")))
	return vs
}
