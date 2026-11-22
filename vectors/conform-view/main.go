// Command conform-view writes the vectors of the conform-view group: the conform vectors that give a
// member's view, by which a member checks its own receipts against the view it signed (verify-1 rows
// 0a and 4a, verify-2). Each vector gives a verifier the Accord id, its sealed founding set, its
// acceptances, no opening, a mediator's receipts and one member's view, and expects the first
// failure's code and index, or that the receipts conform with the number of receipts read against
// the view. Every code, index and count, and every row of a view the vectors read, is written here by
// hand from the text; the generator builds the sets, the views and the receipts with the Go standard
// library and the kit alone, and refuses to write a vector whose view does not hold a row stated
// here (vectors/README.md).
//
// It imports no package of the core.
package main

import (
	"fmt"
	"os"

	"github.com/orilloai/placket/vectors/signatures/kit"
)

// The members: A, the proposer's key, which asks; B, the applier's, which receives what A asks
// about; and D, the decider's, which answers.
var (
	a, b, d = kit.Proposer, kit.Applier, kit.Decider
	passOK  = kit.Verdict{Verdict: "pass", Reason: "ok"}
)

func party(k kit.Key, role string, terms ...kit.Obj) kit.Party {
	return kit.Party{Key: k, Roles: []string{role}, Terms: terms}
}

func roles(r ...string) []string { return r }

// pair is the Accord of the view vectors, under the judged card. A gives B proposals, asked about by
// the decider within 3600 seconds; reports, asked so too, with jev to decide on both sides; plans,
// on whose rule the two sides differ, 60 and 120 seconds; notes, which cross; and memos, which B
// refuses. B gives A notes. A gets D's answers and changed versions, and B gets D's changed
// versions. No term of B's names sketch.
//
// B's view, by the text: from A, proposal asked to decider within 3600, report asked so with jev to
// decide, plan blocked, note crosses, memo blocked; to A, note crosses; from D, changed-version
// crosses.
func pair() kit.Set {
	decide := kit.Judges("jev", "decide")
	return kit.Set{
		Mediator: kit.Mediator, Operator: kit.Operator, Principal: kit.Principal, Card: kit.JudgedCard(),
		Roles: []string{"proposer", "applier", "decider"},
		Parties: []kit.Party{
			party(a, "proposer",
				kit.Maybe("give", roles("applier"), roles("proposal"), roles("decider"), 3600),
				kit.JudgedMaybe("give", roles("applier"), roles("report"), roles("decider"), 3600, decide),
				kit.Maybe("give", roles("applier"), roles("plan"), roles("decider"), 60),
				kit.Term("want", "give", roles("applier"), roles("note", "memo")),
				kit.Term("want", "get", roles("decider"), roles("answer", "changed-version")),
				kit.Term("want", "get", roles("applier"), roles("note"))),
			party(b, "applier",
				kit.Term("want", "get", roles("proposer"), roles("proposal", "note", "memo")),
				kit.Term("no", "get", roles("proposer"), roles("memo")),
				kit.JudgedMaybe("get", roles("proposer"), roles("report"), roles("decider"), 3600, decide),
				kit.Maybe("get", roles("proposer"), roles("plan"), roles("decider"), 120),
				kit.Term("want", "get", roles("decider"), roles("changed-version")),
				kit.Term("want", "give", roles("proposer"), roles("note"))),
			party(d, "decider",
				kit.Term("want", "give", roles("proposer", "applier"), roles("answer", "changed-version"))),
		},
	}
}

// formed is pair in which A gets D's answers and changed versions under a maybe term of its own,
// asked about by the decider within 60 seconds, and also may give B an answer, asked about so, which
// B gets. A's view, by the text: from D, answer and changed-version asked to decider within 60; to
// B, answer asked to decider within 60, proposal asked to decider within 3600.
func formed() kit.Set {
	s := pair()
	s.Parties[0].Terms[3] = kit.Maybe("get", roles("decider"), roles("answer", "changed-version"), roles("decider"), 60)
	s.Parties[0].Terms = append(s.Parties[0].Terms, kit.Maybe("give", roles("applier"), roles("answer"), roles("decider"), 60))
	s.Parties[1].Terms = append(s.Parties[1].Terms, kit.Term("want", "get", roles("proposer"), roles("answer")))
	return s
}

// unselected is pair in which A gets only D's changed versions, so no term of A's selects D's
// answers. A's view, by the text: from D, changed-version crosses, and no row for answer.
func unselected() kit.Set {
	s := pair()
	s.Parties[0].Terms[4] = kit.Term("want", "get", roles("decider"), roles("changed-version"))
	return s
}

var (
	pairSet       = pair().Build()
	formedSet     = formed().Build()
	unselectedSet = unselected().Build()
	otherSet      = kit.Compose().Build()
)

func at(n int) string { return "2026-11-12T09:0" + string(rune('0'+n)) + ":00Z" }

func message(set kit.Obj, id, kind string, to kit.Key) kit.Obj {
	return kit.Message(id, kit.HashOf(set), to, kit.TextPart(kind, kit.Salt("conform-view "+id, 32), "The "+kind+" "+id+"."))
}

func scene(set kit.Obj) *kit.Scene {
	return kit.NewScene(set, kit.Mediator, a, b, d, kit.Stranger)
}

func salt(label string) string { return kit.Salt("conform-view answer "+label, 32) }

// viewOf is the viewer's view of a sealed set, as the kit builds it from the text, under the set's
// Accord id.
func viewOf(set kit.Obj, viewer kit.Key) kit.Obj {
	return kit.View(kit.Open(set), kit.HashOf(set), viewer.Ref)
}

// has refuses to go on unless the viewer's view of the set has the row written here by hand: the
// counterpart, the direction, the kind and the outcome, and for an asked row its timeout.
func has(set kit.Obj, viewer, with kit.Key, dir, kind, outcome string, timeout int64) {
	for _, x := range viewOf(set, viewer)["crossings"].([]any) {
		r := x.(kit.Obj)
		if r["counterpart"] == with.Ref && r["dir"] == dir && r["kind"] == kind {
			if r["outcome"] != outcome || (outcome == "asked" && r["timeout"] != timeout) {
				fmt.Fprintf(os.Stderr, "conform-view: %s's row %s %s %s is %v, not %s\n", viewer.Name, dir, kind, with.Name, r["outcome"], outcome)
				os.Exit(1)
			}
			return
		}
	}
	if outcome != "" {
		fmt.Fprintf(os.Stderr, "conform-view: %s's view has no row %s %s %s\n", viewer.Name, dir, kind, with.Name)
		os.Exit(1)
	}
}

// hasNo refuses to go on if the viewer's view has a row for the crossing.
func hasNo(set kit.Obj, viewer, with kit.Key, dir, kind string) {
	for _, x := range viewOf(set, viewer)["crossings"].([]any) {
		r := x.(kit.Obj)
		if r["counterpart"] == with.Ref && r["dir"] == dir && r["kind"] == kind {
			fmt.Fprintf(os.Stderr, "conform-view: %s's view has a row %s %s %s\n", viewer.Name, dir, kind, with.Name)
			os.Exit(1)
		}
	}
}

// An evidence is a conform-view vector's input: the set, the receipts a scene wrote and the view.
type evidence struct {
	set      kit.Obj
	receipts []kit.Obj
	view     kit.Obj
}

func of(set kit.Obj, s *kit.Scene, viewer kit.Key) evidence {
	return evidence{set: set, receipts: s.Written(), view: viewOf(set, viewer)}
}

func (e evidence) vector(name, tests string, expect kit.Obj) kit.Vector {
	rs := make([]any, len(e.receipts))
	for i, r := range e.receipts {
		rs[i] = kit.Obj{"receipt": r}
	}
	return kit.Vector{Name: "conform-view/" + name, Tests: tests, Operation: "conform", Members: kit.Obj{
		"accord": kit.HashOf(e.set), "founding": e.set, "acceptances": kit.Acceptances(e.set), "receipts": rs,
		"answers": []any{}, "kernel": kit.KernelHash, "view": e.view, "expect": expect,
	}}
}

func fails(code string, index int) kit.Obj { return kit.Obj{"code": code, "index": index} }

// conforms is a journal with no call given and no opening that conforms, its answers unwitnessed,
// with viewed receipts read against the view.
func conforms(unwitnessed, viewed int) kit.Obj {
	return kit.Obj{"conforms": true, "witnessed": 0, "unwitnessed": unwitnessed, "unmatched": 0, "module": true,
		"terms": "not-checked", "viewed": viewed}
}

// asked opens a question: A's proposal to B at 09:00, whose deadline under B's asked row is 10:00.
func asked(s *kit.Scene, set kit.Obj) string {
	return s.Give(at(0), a, 1, message(set, "proposal-1", "proposal", b), b, "escalate", "maybe")
}

func main() {
	// The rows of the views the vectors read, by hand (setup-contract-14).
	has(pairSet, b, a, "get", "proposal", "asked", 3600)
	has(pairSet, b, a, "get", "report", "asked", 3600)
	has(pairSet, b, a, "get", "note", "crosses", 0)
	has(pairSet, b, a, "get", "memo", "blocked", 0)
	has(pairSet, b, a, "get", "plan", "blocked", 0)
	has(pairSet, b, a, "give", "note", "crosses", 0)
	has(pairSet, b, d, "get", "changed-version", "crosses", 0)
	hasNo(pairSet, b, a, "give", "sketch")
	has(pairSet, a, b, "give", "proposal", "asked", 3600)
	has(pairSet, a, d, "get", "answer", "crosses", 0)
	has(pairSet, d, a, "give", "answer", "crosses", 0)
	has(formedSet, a, d, "get", "answer", "asked", 60)
	has(formedSet, a, d, "get", "changed-version", "asked", 60)
	has(formedSet, a, b, "give", "answer", "asked", 60)
	has(unselectedSet, a, b, "give", "proposal", "asked", 3600)
	has(unselectedSet, a, d, "get", "changed-version", "crosses", 0)
	hasNo(unselectedSet, a, d, "get", "answer")

	var vs []kit.Vector
	add := func(v kit.Vector) { vs = append(vs, v) }

	// Every receipt of B's against its row. A note to B, B's note to A, A's memo to B blocked by
	// B's refusal, B's sketch to A, which no term of B's selects, A's proposal asked and D's accept
	// with its release: 7 read. With A's give of tests to D, a crossing B is no side of, B reads the
	// same 7.
	journal := func(tests bool) *kit.Scene {
		s := scene(pairSet)
		s.Give(at(0), a, 1, message(pairSet, "note-1", "note", b), b, "pass", "ok")
		s.Give(at(0), b, 1, message(pairSet, "note-2", "note", a), a, "pass", "ok")
		s.Give(at(1), a, 2, message(pairSet, "memo-1", "memo", b), b, "block", "no-match")
		s.Give(at(1), b, 2, message(pairSet, "sketch-1", "sketch", a), a, "block", "no-match")
		seq := int64(3)
		if tests {
			s.Give(at(1), a, seq, message(pairSet, "tests-1", "tests", d), d, "pass", "ok")
			seq++
		}
		q := s.Give(at(2), a, seq, message(pairSet, "proposal-1", "proposal", b), b, "escalate", "maybe")
		s.Resolve(kit.Answer{At: at(3), Question: q, Action: "accepted", Text: "accepted", Salt: salt("own"), By: d, Verdict: passOK, Release: true, Released: &passOK})
		return s
	}
	add(of(pairSet, journal(false), b).vector("a-members-receipts-match-its-view",
		"verify-2: B's notes to and from A cross, A's memo is blocked, B's sketch with no row is block no-match, and A's proposal is asked, accepted and released, each as B's view says", conforms(2, 7)))
	add(of(pairSet, journal(true), b).vector("a-crossing-of-two-others-is-not-read",
		"verify-2: A's give of tests to D, which passed though no term names tests, is a crossing B is no side of, so B's view reads the other 7 receipts and they conform", conforms(2, 7)))
	add(of(pairSet, journal(true), a).vector("an-askers-give-with-no-row",
		"verify-2: A's view reads its gives, those it receives and its question, and A's give of tests to D, which A's terms do not select, passed: view-conformance at its index", fails("view-conformance", 4)))
	add(of(pairSet, journal(false), a).vector("an-askers-question-matches-its-view",
		"verify-2: A's view reads its notes, memo and sketch, its question, the accept, whose answer from D crosses as A's row says, and the release: 7 read", conforms(2, 7)))
	add(of(pairSet, journal(false), d).vector("an-answerers-own-closing-is-not-read",
		"verify-2: D's view reads no receipt of this journal: D gives and receives nothing, and its accept closes a question it neither asked nor received", conforms(2, 0)))

	// A crossing the view does not allow.
	{
		s := scene(pairSet)
		s.Give(at(0), a, 1, message(pairSet, "memo-1", "memo", b), b, "pass", "ok")
		add(of(pairSet, s, b).vector("a-crossing-the-view-shows-blocked",
			"verify-2: a pass ok of A's memo, which B's view shows blocked, is view-conformance at its index", fails("view-conformance", 0)))
		s = scene(pairSet)
		s.Give(at(0), b, 1, message(pairSet, "sketch-1", "sketch", a), a, "block", "no-match")
		add(of(pairSet, s, b).vector("a-give-with-no-row-blocked-no-match",
			"verify-2, composition-2: B's sketch to A, which no term of B's selects, blocked no-match, conforms", conforms(0, 1)))
		s = scene(pairSet)
		s.Give(at(0), b, 1, message(pairSet, "sketch-1", "sketch", a), a, "pass", "ok")
		add(of(pairSet, s, b).vector("a-give-with-no-row-that-passed",
			"verify-2, composition-2: B's sketch to A, which no term of B's selects, passed, is view-conformance", fails("view-conformance", 0)))
		s = scene(pairSet)
		s.Give(at(0), b, 1, message(pairSet, "sketch-1", "sketch", a), a, "block", "maybe")
		add(of(pairSet, s, b).vector("a-give-with-no-row-blocked-maybe",
			"verify-2, composition-2: B's sketch to A, which no term of B's selects, blocked maybe, is view-conformance: with no row only no-match matches", fails("view-conformance", 0)))
		s = scene(pairSet)
		s.Give(at(0), a, 1, message(pairSet, "plan-1", "plan", b), b, "block", "maybe")
		add(of(pairSet, s, b).vector("a-blocked-row-blocked-maybe",
			"verify-2, composition-5: A's plan, on whose rule the sides differ and which B's view shows blocked, blocked maybe, conforms: a blocked row gives no reason", conforms(0, 1)))
		s = scene(pairSet)
		s.GiveJudged(at(0), a, 1, message(pairSet, "note-1", "note", b), b, kit.Judgment("jev", "pass"), "pass", "ok", kit.ByJudge("jev"))
		add(of(pairSet, s, b).vector("a-crossing-row-passed-by-a-judge",
			"verify-2: A's note, which B's view shows crossing, passed by jev, is view-conformance: a crossing row is the terms' pass", fails("view-conformance", 0)))
		s = scene(pairSet)
		s.Give(at(0), a, 1, message(pairSet, "memo-1", "memo", b), b, "escalate", "maybe")
		add(of(pairSet, s, b).vector("a-blocked-row-asked",
			"verify-2: A's memo, which B's view shows blocked, asked about, is view-conformance: no question opens on a blocked row", fails("view-conformance", 0)))
		s = scene(pairSet)
		s.Give(at(0), a, 1, message(pairSet, "note-1", "note", b), b, "block", "no-match")
		add(of(pairSet, s, b).vector("a-crossing-row-blocked",
			"verify-2: A's note, which B's view shows crossing, blocked, is view-conformance", fails("view-conformance", 0)))
	}

	// Every legitimate close of an asked row conforms, read by B, the question's receiver. The ask
	// is at 09:00 and B's row's timeout 3600, so the deadline is 10:00; D, the only member holding
	// decider, is the first answerer.
	closes := []struct {
		name, tests string
		play        func(s *kit.Scene, q string)
		expect      kit.Obj
	}{
		{"an-accept-with-its-release", "verify-2: D's accept before the deadline and the release of A's proposal to B, pass ok, conform with B's asked row",
			func(s *kit.Scene, q string) {
				s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: salt("accept"), By: d, Verdict: passOK, Release: true, Released: &passOK})
			}, conforms(2, 3)},
		{"a-change-with-its-release", "verify-2: D's change before the deadline, and its version released to B, which B's row of changed versions from D shows crossing, conform",
			func(s *kit.Scene, q string) {
				s.Resolve(kit.Answer{At: at(1), Question: q, Action: "changed", Text: "Rename total to amount_due.", Salt: salt("change"), By: d, Verdict: passOK, Release: true, Released: &passOK})
			}, conforms(2, 3)},
		{"a-decline", "verify-2: a decline from D, the first answerer, before the deadline conforms",
			func(s *kit.Scene, q string) { s.Close(at(1), q, "declined", &d, d) }, conforms(1, 2)},
		{"a-cancel-by-the-asker", "verify-2: A's cancel, whose receipt is from D, the first answerer, before the deadline conforms",
			func(s *kit.Scene, q string) { s.Close(at(1), q, "cancelled", &a, d) }, conforms(1, 2)},
		{"an-undeliverable", "verify-2: an undeliverable from D, the first answerer, before the deadline conforms",
			func(s *kit.Scene, q string) { s.Close(at(1), q, "undeliverable", nil, d) }, conforms(0, 2)},
		{"a-timeout-at-the-deadline", "verify-2, maybe-1: a timeout from D at 10:00, the ask's at plus the row's timeout, conforms",
			func(s *kit.Scene, q string) {
				s.Tick("2026-11-12T10:00:05Z", kit.Timeout{Question: q, Deadline: "2026-11-12T10:00:00Z", From: d})
			}, conforms(0, 2)},
		{"a-closed-closing", "verify-2, close-1: a closing with reason closed from D at 09:30, before the deadline, conforms; the close is not read",
			func(s *kit.Scene, q string) {
				s.CloseAccord("2026-11-12T09:30:00Z", nil, []kit.Closing{{Question: q, From: d}})
			}, conforms(0, 2)},
		{"an-accept-from-a-key-outside-to", "verify-2: an accept from A, which holds no role B's asked row's to names, is view-conformance at the closing, though B is no side of its answer to A",
			func(s *kit.Scene, q string) {
				s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: salt("outside"), By: a, Verdict: passOK})
			}, fails("view-conformance", 1)},
		{"an-undeliverable-at-the-deadline", "verify-2: an undeliverable from D at 10:00, the deadline, is view-conformance at the closing",
			func(s *kit.Scene, q string) { s.Close("2026-11-12T10:00:00Z", q, "undeliverable", nil, d) }, fails("view-conformance", 1)},
		{"a-timeout-before-the-deadline", "verify-2: a timeout at 09:59:59, before the deadline of 10:00, is view-conformance at the closing",
			func(s *kit.Scene, q string) {
				s.Tick("2026-11-12T10:00:05Z", kit.Timeout{Question: q, Deadline: "2026-11-12T09:59:59Z", From: d})
			}, fails("view-conformance", 1)},
		{"a-timeout-after-the-deadline", "verify-2: a timeout at 10:00:05, after the deadline of 10:00, is view-conformance at the closing",
			func(s *kit.Scene, q string) {
				s.Tick("2026-11-12T10:00:05Z", kit.Timeout{Question: q, Deadline: "2026-11-12T10:00:05Z", From: d})
			}, fails("view-conformance", 1)},
		{"an-accept-at-the-deadline", "verify-2: an accept from D at 10:00, the deadline, is view-conformance at the closing",
			func(s *kit.Scene, q string) {
				s.Resolve(kit.Answer{At: "2026-11-12T10:00:00Z", Question: q, Action: "accepted", Text: "accepted", Salt: salt("late"), By: d, Verdict: passOK})
			}, fails("view-conformance", 1)},
		{"an-accept-after-the-deadline", "verify-2: an accept from D at 10:00:01, after the deadline, is view-conformance at the closing",
			func(s *kit.Scene, q string) {
				s.Resolve(kit.Answer{At: "2026-11-12T10:00:01Z", Question: q, Action: "accepted", Text: "accepted", Salt: salt("later"), By: d, Verdict: passOK})
			}, fails("view-conformance", 1)},
		{"a-decline-from-another-member", "verify-2: a decline whose receipt is from B, not D, the first answerer, is view-conformance at the closing",
			func(s *kit.Scene, q string) { s.Close(at(1), q, "declined", &d, b) }, fails("view-conformance", 1)},
		{"a-blocked-release-after-an-accept", "verify-2, maybe-10: a release of A's proposal to B blocked no-match after D's accept is view-conformance: the asked part decided again is asked, so the release passes",
			func(s *kit.Scene, q string) {
				blocked := kit.Verdict{Verdict: "block", Reason: "no-match"}
				s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: salt("blocked"), By: d, Verdict: passOK, Release: true, Released: &blocked})
			}, fails("view-conformance", 2)},
	}
	for _, c := range closes {
		s := scene(pairSet)
		c.play(s, asked(s, pairSet))
		add(of(pairSet, s, b).vector(c.name, c.tests, c.expect))
	}
	{
		// A release a minute after its accept: its receipt names the accept's call, but not its time.
		s := scene(pairSet)
		m := message(pairSet, "proposal-1", "proposal", b)
		q := s.Give(at(0), a, 1, m, b, "escalate", "maybe")
		call := s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: salt("apart"), By: d, Verdict: passOK, Release: true})
		s.Write("crossing", a, b, "proposal", kit.Digest(kit.PartOf(m)), at(2), a, 1, kit.Hash(kit.Canon(m)), "pass", "ok", kit.ByAnswer(call))
		add(of(pairSet, s, b).vector("a-release-after-its-closing", "verify-2, maybe-10: a release at 09:02 of an accept at 09:01 is view-conformance at the release: a release is written in its answer's operation", fails("view-conformance", 2)))
	}
	{
		// A judge's decision on an asked row that lists it to decide, and one on a row that does not.
		s := scene(pairSet)
		s.GiveJudged(at(0), a, 1, message(pairSet, "report-1", "report", b), b, kit.Judgment("jev", "pass"), "pass", "ok", kit.ByJudge("jev"))
		s.GiveJudged(at(1), a, 2, message(pairSet, "report-2", "report", b), b, kit.Judgment("jev", "block"), "block", "judge:jev", kit.ByJudge("jev"))
		add(of(pairSet, s, b).vector("a-judges-pass-and-block-the-row-lists",
			"verify-2, composition-7: jev's pass and jev's block of A's reports, which B's view shows asked with jev to decide, conform", conforms(0, 2)))
		s = scene(pairSet)
		s.GiveJudged(at(0), a, 1, message(pairSet, "proposal-1", "proposal", b), b, kit.Judgment("jev", "pass"), "pass", "ok", kit.ByJudge("jev"))
		add(of(pairSet, s, b).vector("a-judges-pass-the-row-does-not-list",
			"verify-2, composition-7: jev's pass of A's proposal, which B's view shows asked with no judge, is view-conformance", fails("view-conformance", 0)))
	}

	// A part the mediator formed from an answer, read by A, whose rows of answers and changed
	// versions from D are asked: only a block, maybe, conforms; and A's own give of an answer is a
	// give.
	for _, c := range []struct {
		name, tests, action string
		verdict             kit.Verdict
		expect              kit.Obj
	}{
		{"a-formed-answer-passed-under-an-asked-row", "verify-2, composition-5: D's accept, whose answer to A meets A's asked row, passed, is view-conformance at the closing", "accepted", passOK, fails("view-conformance", 1)},
		{"a-formed-answer-blocked-under-an-asked-row", "verify-2, composition-5: D's accept, whose answer to A meets A's asked row, blocked maybe, conforms", "accepted", kit.Verdict{Verdict: "block", Reason: "maybe"}, conforms(1, 2)},
		{"a-formed-version-passed-under-an-asked-row", "verify-2, composition-5: D's change, whose version to A meets A's asked row, passed, is view-conformance at the closing", "changed", passOK, fails("view-conformance", 1)},
		{"a-formed-version-blocked-under-an-asked-row", "verify-2, composition-5: D's change, whose version to A meets A's asked row, blocked maybe, conforms", "changed", kit.Verdict{Verdict: "block", Reason: "maybe"}, conforms(1, 2)},
		{"a-formed-answer-blocked-no-match-under-an-asked-row", "verify-2, composition-5: D's accept, whose answer to A meets A's asked row, blocked no-match, is view-conformance", "accepted", kit.Verdict{Verdict: "block", Reason: "no-match"}, fails("view-conformance", 1)},
	} {
		s := scene(formedSet)
		q := asked(s, formedSet)
		s.Resolve(kit.Answer{At: at(1), Question: q, Action: c.action, Text: "Rename total.", Salt: salt(c.name), By: d, Verdict: c.verdict})
		add(of(formedSet, s, a).vector(c.name, c.tests, c.expect))
	}
	{
		s := scene(formedSet)
		s.Give(at(0), a, 1, message(formedSet, "answer-1", "answer", b), b, "escalate", "maybe")
		add(of(formedSet, s, a).vector("a-members-own-answer-asked", "verify-2: A's own give of kind answer to B, which A's view shows asked, is a give, and asked it conforms", conforms(0, 1)))
		s = scene(formedSet)
		s.Give(at(0), a, 1, message(formedSet, "answer-1", "answer", b), b, "block", "maybe")
		add(of(formedSet, s, a).vector("a-members-own-answer-blocked", "verify-2: A's own give of kind answer to B, which A's view shows asked, blocked maybe as a formed part would be, is view-conformance", fails("view-conformance", 0)))
	}

	// A part the mediator formed from an answer, read by A, whose view has no row for answers from
	// D: as for a give with no row, only a block, no-match, conforms.
	for _, c := range []struct {
		name, tests string
		verdict     kit.Verdict
		expect      kit.Obj
	}{
		{"a-formed-answer-with-no-row-blocked-maybe", "verify-2, composition-2: D's accept, whose answer to A no term of A's selects, blocked maybe, is view-conformance at the closing: a formed part with no row matches only as a block no-match, as a give with no row does", kit.Verdict{Verdict: "block", Reason: "maybe"}, fails("view-conformance", 1)},
		{"a-formed-answer-with-no-row-blocked-no-match", "verify-2, composition-2: D's accept, whose answer to A no term of A's selects, blocked no-match, conforms", kit.Verdict{Verdict: "block", Reason: "no-match"}, conforms(1, 2)},
	} {
		s := scene(unselectedSet)
		q := asked(s, unselectedSet)
		s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "Rename total.", Salt: salt(c.name), By: d, Verdict: c.verdict})
		add(of(unselectedSet, s, a).vector(c.name, c.tests, c.expect))
	}

	// A view that is not the one accepted, before any receipt and before validity.
	{
		s := scene(pairSet)
		s.Give(at(0), a, 1, message(pairSet, "note-1", "note", b), b, "pass", "ok")
		e := of(pairSet, s, b)
		changed := kit.Copy(e.view).(kit.Obj)
		// The first row that crosses, now blocked: still a view's shape, not B's view.
		for _, x := range changed["crossings"].([]any) {
			if r := x.(kit.Obj); r["outcome"] == "crosses" {
				r["outcome"] = "blocked"
				break
			}
		}
		if kit.HashOf(changed) == kit.HashOf(e.view) {
			fmt.Fprintln(os.Stderr, "conform-view: the changed view is the view")
			os.Exit(1)
		}
		e.view = changed
		add(e.vector("a-view-with-one-row-changed", "verify-1 row 0a: B's view with one row's outcome changed is not the view B accepted: acceptance-view", fails("acceptance-view", -1)))
		e = of(pairSet, s, b)
		e.view = kit.View(kit.Open(pairSet), kit.HashOf(otherSet), b.Ref)
		add(e.vector("a-view-of-another-accord", "verify-1 row 0a: B's view built under another Accord id is acceptance-view", fails("acceptance-view", -1)))
		e = of(pairSet, s, b)
		e.view = viewOf(pairSet, a)
		e.view["viewer"] = b.Ref
		add(e.vector("another-members-rows", "verify-1 row 0a: A's view named as B's is not the view B accepted: acceptance-view", fails("acceptance-view", -1)))
		// The second note alone of A's two: chain at index 0 in a whole list.
		s2 := scene(pairSet)
		s2.Give(at(0), a, 1, message(pairSet, "note-1", "note", b), b, "pass", "ok")
		s2.Give(at(1), a, 2, message(pairSet, "note-2", "note", b), b, "pass", "ok")
		e = of(pairSet, s2, b)
		e.view = changed
		e.receipts = e.receipts[1:]
		add(e.vector("a-view-not-accepted-before-validity", "verify-1 row 0a and section 8: a changed view and a list whose first receipt has seq 2, which validity refuses with chain, are acceptance-view, the earlier row", fails("acceptance-view", -1)))
	}
	// Row 4a after row 1: a receipt of another Accord, which also has no row in B's view, is accord.
	{
		s := kit.NewScene(otherSet, kit.Mediator, a, b)
		s.Give(at(0), a, 1, message(otherSet, "brief-1", "brief", b), b, "pass", "ok")
		e := evidence{set: pairSet, receipts: s.Written(), view: viewOf(pairSet, b)}
		add(e.vector("another-accord-before-its-view", "verify-1 rows 1 and 4a: a receipt of another Accord, of a kind B's view has no row for, is accord, the earlier row", fails("accord", 0)))
	}
	kit.Main("conform-view", vs)
}
