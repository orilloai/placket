// Command questions writes the vectors of the questions group: section 6 of the protocol, as far as
// this version's core runs it: asking (maybe-1). A give that composition decides escalate writes its
// receipt, holds the part as a question and queues nothing. Every verdict and reason is written here
// by hand; the generator builds and signs the messages, envelopes and receipts with the Go standard
// library alone (vectors/README.md).
//
// It imports no package of the core.
package main

import "github.com/orilloai/placket/vectors/signatures/kit"

var askAndLog = kit.AskAndLog().Build()

func message(id, kind, text string) kit.Obj {
	return kit.Message(id, kit.HashOf(askAndLog), kit.Applier, kit.TextPart(kind, kit.Salt("questions "+id, 16), text))
}

func scene() *kit.Scene {
	return kit.NewScene(askAndLog, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider)
}

func main() {
	var vs []kit.Vector
	{
		s := scene()
		s.Give("2026-11-12T09:00:00Z", kit.Proposer, 1, message("proposal-1", "proposal", "Rename total to amount_due."), kit.Applier, "escalate", "maybe")
		s.Get(kit.Applier)
		vs = append(vs, s.Vector("questions/a-proposal-is-asked-about",
			"maybe-1: the proposer's proposal under a maybe term is escalate, maybe; its question is open, and the applier's fetch returns nothing", askAndLog))
	}
	{
		s := scene()
		s.Give("2026-11-12T09:00:00Z", kit.Proposer, 1, message("proposal-1", "proposal", "First."), kit.Applier, "escalate", "maybe")
		s.Give("2026-11-12T09:05:00Z", kit.Proposer, 2, message("note-1", "note", "Meanwhile."), kit.Applier, "pass", "ok")
		s.Give("2026-11-12T09:10:00Z", kit.Proposer, 3, message("proposal-2", "proposal", "Second."), kit.Applier, "escalate", "maybe")
		vs = append(vs, s.Vector("questions/open-questions-in-the-order-opened",
			"maybe-1: two asked proposals are open in the order they were opened, and a note between them is queued", askAndLog))
	}
	{
		s := scene()
		s.Give("2026-11-12T09:00:00Z", kit.Proposer, 1, message("proposal-1", "proposal", "Held."), kit.Applier, "escalate", "maybe")
		s.Get(kit.Applier)
		s.Get(kit.Proposer)
		s.Get(kit.Decider)
		vs = append(vs, s.Vector("questions/the-asked-part-is-held",
			"maybe-1: nothing is queued for anyone when a part is asked about; the held part leaves only by a release", askAndLog))
	}
	kit.Main("questions", vs)
}
