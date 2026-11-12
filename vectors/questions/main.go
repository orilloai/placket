// Command questions writes the vectors of the questions group: section 6 of the protocol. A give that
// composition decides escalate opens a question (maybe-1); an answer accepts, changes, declines or
// cancels it, the mediator finds it undeliverable, or its deadline passes (maybe-3 to maybe-9); and an
// accept or a change that passes may release (maybe-10). Every verdict, reason, rejection code,
// deadline and first answerer is written here by hand from the text; the generator builds and signs
// the messages, envelopes and receipts with the Go standard library alone (vectors/README.md).
//
// In the ask-and-log Accord only the decider holds decider, so the decider is every question's first
// answerer, and a proposal's deadline is its give's time plus 3600 seconds.
//
// It imports no package of the core.
package main

import "github.com/orilloai/placket/vectors/signatures/kit"

var (
	askAndLog  = kit.AskAndLog().Build()
	compose    = kit.Compose().Build()
	longWait   = kit.LongWait().Build()
	unanswered = kit.Unanswered().Build()
)

const asked = "2026-11-12T09:00:00Z"

func messageIn(set kit.Obj, id, kind, text string) kit.Obj {
	return kit.Message(id, kit.HashOf(set), kit.Applier, kit.TextPart(kind, kit.Salt("questions "+id, 16), text))
}

func message(id, kind, text string) kit.Obj { return messageIn(askAndLog, id, kind, text) }

func sceneOf(set kit.Obj) *kit.Scene {
	return kit.NewScene(set, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider)
}

func scene() *kit.Scene { return sceneOf(askAndLog) }

// proposal gives the proposer's proposal to the applier at the time, escalate, maybe, and returns
// the question's id.
func proposal(s *kit.Scene, set kit.Obj, at string, seq int64, id string) string {
	return s.Give(at, kit.Proposer, seq, messageIn(set, id, "proposal", "Rename total to amount_due in "+id+"."), kit.Applier, "escalate", "maybe")
}

// salt is the salt the mediator mints for an answer, given by the vector (conformance-vectors-2).
func salt(label string) string { return kit.Salt("questions answer "+label, 16) }

var (
	passOK      = kit.Verdict{Verdict: "pass", Reason: "ok"}
	blockMatch  = kit.Verdict{Verdict: "block", Reason: "no-match"}
	decider     = kit.Decider
	proposerKey = kit.Proposer
	stranger    = kit.Stranger
)

func main() {
	var vs []kit.Vector
	add := func(name, tests string, s *kit.Scene, set kit.Obj) {
		vs = append(vs, s.Vector("questions/"+name, tests, set))
	}
	{
		s := scene()
		s.Give(asked, kit.Proposer, 1, message("proposal-1", "proposal", "Rename total to amount_due."), kit.Applier, "escalate", "maybe")
		s.Get(kit.Applier)
		add("a-proposal-is-asked-about",
			"maybe-1: the proposer's proposal under a maybe term is escalate, maybe; its question is open, and the applier's fetch returns nothing", s, askAndLog)
	}
	{
		s := scene()
		s.Give(asked, kit.Proposer, 1, message("proposal-1", "proposal", "First."), kit.Applier, "escalate", "maybe")
		s.Give("2026-11-12T09:05:00Z", kit.Proposer, 2, message("note-1", "note", "Meanwhile."), kit.Applier, "pass", "ok")
		s.Give("2026-11-12T09:10:00Z", kit.Proposer, 3, message("proposal-2", "proposal", "Second."), kit.Applier, "escalate", "maybe")
		add("open-questions-in-the-order-opened",
			"maybe-1: two asked proposals are open in the order they were opened, and a note between them is queued", s, askAndLog)
	}
	{
		s := scene()
		s.Give(asked, kit.Proposer, 1, message("proposal-1", "proposal", "Held."), kit.Applier, "escalate", "maybe")
		s.Get(kit.Applier)
		s.Get(kit.Proposer)
		s.Get(kit.Decider)
		add("the-asked-part-is-held",
			"maybe-1: nothing is queued for anyone when a part is asked about; the held part leaves only by a release", s, askAndLog)
	}

	// Deadlines and the passing of time (maybe-1, maybe-5, maybe-7).
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "deadline")
		s.Tick("2026-11-12T09:59:59Z")
		s.Tick("2026-11-12T10:00:00Z", kit.Timeout{Question: q, Deadline: "2026-11-12T10:00:00Z", From: decider})
		add("the-deadline-of-an-asked-proposal",
			"maybe-1, maybe-5: a proposal asked at 09:00:00 under a timeout of 3600 has its deadline at 10:00:00: a tick a second before writes nothing, and a tick at 10:00:00 writes its timeout, block, timeout, from the decider", s, askAndLog)
	}
	{
		s := sceneOf(longWait)
		q := proposal(s, longWait, asked, 1, "last")
		s.Tick("9999-12-31T23:59:58Z")
		s.Tick("9999-12-31T23:59:59Z", kit.Timeout{Question: q, Deadline: "9999-12-31T23:59:59Z", From: decider})
		add("a-deadline-past-the-last-time",
			"maybe-1, maybe-5: under a timeout of 9007199254740991 the deadline is 9999-12-31T23:59:59Z, which a tick a second before does not reach and a tick at that time does", s, longWait)
	}
	for _, c := range []struct {
		name, tests, tick string
		closes            bool
	}{
		{"one-receipt-at-the-deadline", "maybe-5, conformance-vectors-7: a question with deadline 2026-11-02T09:00:01Z and a tick at 09:00:05 write exactly one receipt, block, timeout, whose at is the deadline", "2026-11-02T09:00:05Z", true},
		{"before-the-deadline", "maybe-5: a tick at 2026-11-02T09:00:00Z, before the deadline 09:00:01, writes nothing and the question stays open", "2026-11-02T09:00:00Z", false},
	} {
		s := scene()
		s.FoundedAt = "2026-11-02T08:00:00Z"
		q := proposal(s, askAndLog, "2026-11-02T08:00:01Z", 1, "early")
		if c.closes {
			s.Tick(c.tick, kit.Timeout{Question: q, Deadline: "2026-11-02T09:00:01Z", From: decider})
		} else {
			s.Tick(c.tick)
		}
		add(c.name, c.tests, s, askAndLog)
	}
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "late")
		s.Refuse("2026-11-12T10:00:00Z", q, "accepted", &decider, "accepted", salt("late"), "no-open-question")
		s.Tick("2026-11-12T10:00:00Z", kit.Timeout{Question: q, Deadline: "2026-11-12T10:00:00Z", From: decider})
		add("an-answer-at-the-deadline",
			"maybe-7: the decider's accept at the deadline, before a tick has closed the question, is no-open-question, and the next tick writes the timeout", s, askAndLog)
	}

	// Accepts (maybe-3, maybe-8).
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "accept")
		s.Resolve(kit.Answer{At: "2026-11-12T09:10:00Z", Question: q, Action: "accepted", Text: "accepted", Salt: salt("accept"),
			By: decider, Verdict: passOK})
		s.Get(kit.Proposer)
		s.Get(kit.Applier)
		add("an-accept-that-passes",
			"maybe-3, maybe-8, conformance-vectors-2: the decider's accept with the text accepted is one resolution receipt from the decider to the proposer, pass, ok, of kind answer, echoing the question's envelope, under the step's salt; the proposer fetches the answer, messageId answer: and the question's id, re that id, and the applier nothing", s, askAndLog)
	}
	{
		s := sceneOf(compose)
		q := s.Give(asked, kit.Proposer, 1, messageIn(compose, "memo-1", "memo", "A memo."), kit.Applier, "escalate", "maybe")
		s.Resolve(kit.Answer{At: "2026-11-12T09:00:30Z", Question: q, Action: "accepted", Text: "accepted", Salt: salt("compose"),
			By: decider, Verdict: blockMatch, Release: true})
		s.Get(kit.Proposer)
		s.Get(kit.Applier)
		add("an-accept-the-askers-terms-do-not-take",
			"maybe-3, maybe-10: an accept asking for release, where the proposer has no term to get an answer, is block, no-match; no release receipt follows, nothing is queued, and the question is closed", s, compose)
	}
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "twice")
		s.Resolve(kit.Answer{At: "2026-11-12T09:10:00Z", Question: q, Action: "accepted", Text: "accepted", Salt: salt("twice"),
			By: decider, Verdict: passOK})
		s.Refuse("2026-11-12T09:11:00Z", q, "declined", &decider, "", "", "no-open-question")
		s.Refuse("2026-11-12T09:12:00Z", q, "undeliverable", nil, "", "", "no-open-question")
		add("a-second-answer",
			"maybe-7: after the decider's accept, its decline and the mediator's finding that the question cannot be delivered are each no-open-question and write nothing", s, askAndLog)
	}

	// Only an answerer answers (maybe-6).
	{
		s := kit.NewScene(askAndLog, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider, kit.Stranger)
		q := proposal(s, askAndLog, asked, 1, "own")
		s.Refuse("2026-11-12T09:10:00Z", q, "accepted", &stranger, "accepted", salt("own"), "not-a-member")
		s.Refuse("2026-11-12T09:11:00Z", q, "accepted", &proposerKey, "accepted", salt("own"), "not-an-answerer")
		s.Get(kit.Applier)
		add("the-proposer-answers-its-own-proposal",
			"maybe-6: a stranger's accept is not-a-member, and the proposer's, who does not hold decider, is not-an-answerer; neither writes anything, and the question stays open", s, askAndLog)
	}

	// Decline, cancel and undeliverable (maybe-4).
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "decline")
		s.Close("2026-11-12T09:10:00Z", q, "declined", &decider, decider)
		s.Get(kit.Applier)
		add("a-decline-releases-nothing",
			"maybe-4, maybe-10: the decider's decline is one receipt from the decider to the proposer, block, declined, of kind proposal with the asked part's digest; no release receipt follows, and the applier's fetch returns nothing", s, askAndLog)
	}
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "cancel")
		s.Close("2026-11-12T09:10:00Z", q, "cancelled", nil, decider)
		s.Get(kit.Applier)
		add("a-cancel",
			"maybe-4, conformance-vectors-6: a cancel with no by, answered as the first answerer, the decider, is one receipt from the decider to the proposer, block, cancelled, naming the asked part, and nothing is queued", s, askAndLog)
	}
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "unreached")
		s.Close("2026-11-12T09:10:00Z", q, "undeliverable", nil, decider)
		add("an-undeliverable-question",
			"maybe-4: the mediator's finding that a question with an answerer cannot be delivered is from that first answerer, the decider, to the proposer, block, undeliverable", s, askAndLog)
	}
	{
		s := sceneOf(unanswered)
		q := proposal(s, unanswered, asked, 1, "nobody")
		s.Close("2026-11-12T09:00:00Z", q, "undeliverable", nil, kit.Mediator)
		s.Get(kit.Applier)
		add("a-question-nobody-can-answer",
			"maybe-4: a question whose rule names auditor, a role no member holds, closed as undeliverable, is from the mediator's reference to the proposer, block, undeliverable", s, unanswered)
	}

	// A change (maybe-9).
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "change")
		s.Resolve(kit.Answer{At: "2026-11-12T09:10:00Z", Question: q, Action: "changed", Text: "Rename total to amount_due.", Salt: salt("change"),
			By: decider, OmitBy: true, Verdict: passOK})
		s.Get(kit.Proposer)
		s.Get(kit.Applier)
		add("a-change-returned-to-the-asker",
			"maybe-9, conformance-vectors-6: a change with no by and no release, answered as the decider, is one resolution receipt from the decider to the proposer, pass, ok, of kind changed-version; the proposer fetches the version, messageId change: and the question's id, re that id", s, askAndLog)
	}

	// Release (maybe-10, conformance-vectors-6). After an accept, the proposal is decided again from
	// the proposer to the applier: the proposer's maybe term makes it escalate once more, which is
	// not a block, so the release is pass, ok. After a change, the version is decided from the decider
	// to the applier, who wants to get changed-version from the decider: pass, ok.
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "release")
		s.Resolve(kit.Answer{At: "2026-11-12T09:10:00Z", Question: q, Action: "accepted", Text: "accepted", Salt: salt("release"),
			By: decider, Verdict: passOK, Release: true, Released: &passOK})
		s.Get(kit.Applier)
		add("an-accept-releases-the-asked-part",
			"maybe-10, conformance-vectors-6: an accept asking for release writes the resolution from the decider to the proposer, pass, ok, of kind answer, then a crossing receipt from the proposer to the applier, pass, ok, of kind proposal with the asked part's digest, echoing the question's envelope; the applier fetches the proposal as given", s, askAndLog)
	}
	{
		s := scene()
		q := proposal(s, askAndLog, asked, 1, "version")
		s.Resolve(kit.Answer{At: "2026-11-12T09:10:00Z", Question: q, Action: "changed", Text: "Rename total to amount_due.", Salt: salt("version"),
			By: decider, Verdict: passOK, Release: true, Released: &passOK})
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		add("a-change-released-to-the-receiver",
			"maybe-10, conformance-vectors-6: a change asking for release writes two resolution receipts, from the decider to the proposer and to the applier, pass, ok, of kind changed-version under one digest, both echoing the question's envelope; the version is queued for both, and the proposal never for the applier", s, askAndLog)
	}
	kit.Main("questions", vs)
}
