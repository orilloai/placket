// Command view writes the vectors of the view group: section 3 of the protocol, a member's view of
// the merged Accord (setup-contract-14). Each view's crossings and answers, and each outcome, are
// written here by hand from the text; the generator builds and seals the sets and computes the
// views' bytes and typed hashes with the Go standard library and the kit alone, and refuses to write
// a view the kit's builder does not agree with (vectors/README.md).
//
// It imports no package of the core.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/orilloai/placket/vectors/signatures/kit"
)

// The parties of the view sets, each holding the role of its letter: A is the proposer's key, B the
// applier's, C the decider's and D the lead's.
var (
	a, b, c, d = kit.Proposer, kit.Applier, kit.Decider, kit.Lead
)

// A row is one crossing of a view, by hand: the counterpart, the direction from the viewer, the
// kind, the outcome and, for an asked row, its rule's roles, its timeout and its judges.
type row struct {
	with    kit.Key
	dir     string
	kind    string
	outcome string
	to      []string
	timeout int64
	judges  []any
}

// An ask is one row of a view's answers, by hand.
type ask struct {
	asker, receiver kit.Key
	kind            string
	timeout         int64
}

func set(card kit.Obj, roles []string, parties ...kit.Party) kit.Set {
	return kit.Set{Mediator: kit.Mediator, Operator: kit.Operator, Principal: kit.Principal, Roles: roles, Parties: parties, Card: card}
}

func party(k kit.Key, role string, terms ...kit.Obj) kit.Party {
	return kit.Party{Key: k, Roles: []string{role}, Terms: terms}
}

// expected builds the view the text gives the viewer from the rows and asks written by hand, in the
// text's order: crossings by counterpart in actor-reference order, then direction and kind; answers
// by asker, receiver and kind.
func expected(open kit.Obj, accord string, viewer kit.Key, rows []row, asks []ask) kit.Obj {
	roles := map[string]any{}
	for _, x := range open["bindings"].([]any) {
		bo := x.(kit.Obj)
		roles[bo["actor"].(string)] = bo["roles"]
	}
	sort.Slice(rows, func(i, j int) bool {
		x, y := rows[i], rows[j]
		if x.with.Ref != y.with.Ref {
			return kit.Less16(x.with.Ref, y.with.Ref)
		}
		if x.dir != y.dir {
			return x.dir < y.dir
		}
		return x.kind < y.kind
	})
	crossings := []any{}
	for _, r := range rows {
		o := kit.Obj{"counterpart": r.with.Ref, "roles": kit.Copy(roles[r.with.Ref]), "dir": r.dir, "kind": r.kind, "outcome": r.outcome}
		if r.outcome == "asked" {
			to := []any{}
			for _, t := range r.to {
				to = append(to, "role:"+t)
			}
			o["to"], o["timeout"] = to, r.timeout
			if r.judges != nil {
				o["judges"] = r.judges
			}
		}
		crossings = append(crossings, o)
	}
	sort.Slice(asks, func(i, j int) bool {
		x, y := asks[i], asks[j]
		if x.asker.Ref != y.asker.Ref {
			return kit.Less16(x.asker.Ref, y.asker.Ref)
		}
		if x.receiver.Ref != y.receiver.Ref {
			return kit.Less16(x.receiver.Ref, y.receiver.Ref)
		}
		return x.kind < y.kind
	})
	answers := []any{}
	for _, x := range asks {
		answers = append(answers, kit.Obj{"asker": x.asker.Ref, "receiver": x.receiver.Ref, "kind": x.kind, "timeout": x.timeout})
	}
	return kit.Obj{"accord": accord, "viewer": viewer.Ref, "salt": kit.SealSalt(viewer.Ref), "crossings": crossings, "answers": answers}
}

// vector writes a view vector of the set for the viewer, its expected view the one written by hand.
func vector(name, tests string, s kit.Set, viewer kit.Key, rows []row, asks []ask) kit.Vector {
	sealed, openings := kit.Seal(s.Open())
	open, accord := kit.Open(sealed), kit.HashOf(sealed)
	want := expected(open, accord, viewer, rows, asks)
	if got := kit.View(open, accord, viewer.Ref); string(kit.Canon(got)) != string(kit.Canon(want)) {
		fmt.Fprintf(os.Stderr, "view: %s: the kit builds\n%s\nand the text gives\n%s\n", name, kit.Canon(got), kit.Canon(want))
		os.Exit(1)
	}
	return kit.Vector{Name: "view/" + name, Tests: tests, Operation: "view", Members: kit.Obj{
		"inputHex": kit.HexOf(kit.Canon(sealed)), "foundedAt": kit.FoundedAt, "openings": openings, "viewer": viewer.Ref,
		"expect": kit.Obj{"view": want, "hash": kit.HashOf(want)},
	}}
}

func main() {
	l := func(s ...string) []string { return s }
	// A refuses to give a secret and gives notes; B gets notes and drafts, and says nothing of secrets.
	// Where A refuses drafts, and where A says nothing of them, B's draft row is the same.
	refusing := set(nil, l("a", "b"),
		party(a, "a", kit.Term("want", "give", l("b"), l("note")), kit.Term("no", "give", l("b"), l("secret", "draft"))),
		party(b, "b", kit.Term("want", "get", l("a"), l("note", "draft"))))
	silent := set(nil, l("a", "b"),
		party(a, "a", kit.Term("want", "give", l("b"), l("note"))),
		party(b, "b", kit.Term("want", "get", l("a"), l("note", "draft"))))
	// A and B both ask about memos and plans before C answers; their plan rules differ in timeout.
	maybes := set(nil, l("a", "b", "c"),
		party(a, "a", kit.Maybe("give", l("b"), l("memo", "plan"), l("c"), 60)),
		party(b, "b", kit.Maybe("get", l("a"), l("memo"), l("c"), 60), kit.Maybe("get", l("a"), l("plan"), l("c"), 120)),
		party(c, "c", kit.Term("want", "give", l("a"), l("answer"))))
	// Both sides list jev to decide on reports; on reviews only A lists it.
	decide := kit.Judges("jev", "decide")
	judged := set(kit.JudgedCard(), l("a", "b", "c"),
		party(a, "a", kit.JudgedMaybe("give", l("b"), l("report", "review"), l("c"), 3600, decide)),
		party(b, "b", kit.JudgedMaybe("get", l("a"), l("report"), l("c"), 3600, decide), kit.Maybe("get", l("a"), l("review"), l("c"), 3600)),
		party(c, "c", kit.Term("want", "give", l("a"), l("answer"))))
	// A and C ask each other about contracts, answered by an approver, D, who is a side of no
	// crossing of contracts and gives both answers.
	approve := func(k kit.Key, role, other string) kit.Party {
		return party(k, role, kit.Maybe("give", l(other), l("contract"), l("approver"), 300), kit.Maybe("get", l(other), l("contract"), l("approver"), 300),
			kit.Term("want", "get", l("approver"), l("answer")))
	}
	answering := set(nil, l("a", "c", "approver"), approve(a, "a", "c"), approve(c, "c", "a"),
		party(d, "approver", kit.Term("want", "give", l("a", "c"), l("answer"))))

	vs := []kit.Vector{
		vector("a-refusal-seen-by-its-own-side", "setup-contract-14: A's own terms refuse secrets and drafts toward B, so A's view has both rows blocked beside notes that cross",
			refusing, a, []row{{with: b, dir: "give", kind: "note", outcome: "crosses"}, {with: b, dir: "give", kind: "secret", outcome: "blocked"}, {with: b, dir: "give", kind: "draft", outcome: "blocked"}}, nil),
		vector("a-kind-only-the-other-side-names", "setup-contract-14: B's terms name no secret, so B's view has no row for the kind A refuses, and its draft row, which A refuses, is blocked with no reason",
			refusing, b, []row{{with: a, dir: "get", kind: "note", outcome: "crosses"}, {with: a, dir: "get", kind: "draft", outcome: "blocked"}}, nil),
		vector("blocked-when-the-other-side-is-silent", "setup-contract-14: where A says nothing of drafts, B's draft row is blocked, the same row as where A refuses them",
			silent, b, []row{{with: a, dir: "get", kind: "note", outcome: "crosses"}, {with: a, dir: "get", kind: "draft", outcome: "blocked"}}, nil),
		vector("maybe-rules-that-differ", "setup-contract-14, composition-5: memo rules agree and are asked, plan rules differ in timeout and show as blocked, in A's view",
			maybes, a, []row{{with: b, dir: "give", kind: "memo", outcome: "asked", to: l("c"), timeout: 60}, {with: b, dir: "give", kind: "plan", outcome: "blocked"}}, nil),
		vector("maybe-rules-that-differ-other-side", "setup-contract-14, composition-5: B's view of the same Accord shows the memo asked and the plan blocked",
			maybes, b, []row{{with: a, dir: "get", kind: "memo", outcome: "asked", to: l("c"), timeout: 60}, {with: a, dir: "get", kind: "plan", outcome: "blocked"}}, nil),
		vector("only-asked-crossings-are-answered", "setup-contract-14: C answers the memos A and B ask about, and not the plans, whose rules differ",
			maybes, c, []row{{with: a, dir: "give", kind: "answer", outcome: "blocked"}}, []ask{{a, b, "memo", 60}}),
		vector("asked-with-the-judges-consented-to", "setup-contract-14, composition-7: reports are asked with who answers, the timeout and jev to decide, which both sides list; reviews, which only A lists jev on, are asked with no judge",
			judged, a, []row{{with: b, dir: "give", kind: "report", outcome: "asked", to: l("c"), timeout: 3600, judges: decide}, {with: b, dir: "give", kind: "review", outcome: "asked", to: l("c"), timeout: 3600}}, nil),
		vector("the-same-view-again", "setup-contract-14: the same openings give the same view, byte for byte, and the same typed hash as asked-with-the-judges-consented-to",
			judged, a, []row{{with: b, dir: "give", kind: "report", outcome: "asked", to: l("c"), timeout: 3600, judges: decide}, {with: b, dir: "give", kind: "review", outcome: "asked", to: l("c"), timeout: 3600}}, nil),
		vector("an-answerer-lists-what-it-answers", "setup-contract-14: D holds approver and is a side of no crossing of contracts, and its view lists under answers the A to C and C to A contracts with their timeout, and no term of A's or C's",
			answering, d, []row{{with: a, dir: "give", kind: "answer", outcome: "crosses"}, {with: c, dir: "give", kind: "answer", outcome: "crosses"}}, []ask{{a, c, "contract", 300}, {c, a, "contract", 300}}),
		vector("an-asker-sees-who-answers", "setup-contract-14: A's view shows its contracts with C asked, answered by approver within 300 seconds, and its answers from D crossing",
			answering, a, []row{
				{with: c, dir: "give", kind: "contract", outcome: "asked", to: l("approver"), timeout: 300},
				{with: c, dir: "get", kind: "contract", outcome: "asked", to: l("approver"), timeout: 300},
				{with: d, dir: "get", kind: "answer", outcome: "crosses"}}, nil),
	}
	// The ask-and-log Accord, whose found vector's acceptances sign these views.
	p, ap, dc := kit.Proposer, kit.Applier, kit.Decider
	vs = append(vs,
		vector("ask-and-log-applier", "setup-contract-14: the applier's view of the ask-and-log Accord: notes cross, proposals are asked by the decider within 3600 seconds, and changed versions cross",
			kit.AskAndLog(), ap, []row{{with: p, dir: "get", kind: "note", outcome: "crosses"}, {with: p, dir: "get", kind: "proposal", outcome: "asked", to: l("decider"), timeout: 3600},
				{with: dc, dir: "get", kind: "changed-version", outcome: "crosses"}}, nil),
		vector("ask-and-log-decider", "setup-contract-14: the decider's view of the ask-and-log Accord: its answers cross to the proposer and not to the applier, its changed versions cross to both, and it answers the proposer's proposals to the applier",
			kit.AskAndLog(), dc, []row{{with: p, dir: "give", kind: "answer", outcome: "crosses"}, {with: p, dir: "give", kind: "changed-version", outcome: "crosses"},
				{with: ap, dir: "give", kind: "answer", outcome: "blocked"}, {with: ap, dir: "give", kind: "changed-version", outcome: "crosses"}}, []ask{{p, ap, "proposal", 3600}}),
	)
	sealed, openings := kit.Seal(silent.Open())
	vs = append(vs, kit.Vector{Name: "view/an-opening-missing", Tests: "setup-contract-14, setup-contract-13: a view is built from every opening, and a founder holding one member's opening and not the other's is opening-missing", Operation: "view", Members: kit.Obj{
		"inputHex": kit.HexOf(kit.Canon(sealed)), "foundedAt": kit.FoundedAt, "openings": []any{openings[0]}, "viewer": a.Ref,
		"expect": kit.Obj{"code": "opening-missing"},
	}})
	kit.Main("view", vs)
}
