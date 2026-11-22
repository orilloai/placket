// Command close writes the vectors of the close group: section 13 of the protocol, closing an Accord
// (close-1 to close-4), and the rows 8 and 9 it adds to section 8. Its scenario vectors close a
// mediator's Accord in a close step and then give, answer and fetch on the closed Accord; its verify
// vectors check closed lists, and closes that do not match the receipts before them. Every verdict,
// reason, rejection code, deadline, first answerer, verify code and index is written here by hand
// from the text; the generator builds and signs the messages, envelopes and receipts, the close
// receipts included, with the Go standard library alone (vectors/README.md).
//
// In the ask-and-log Accord only the decider holds decider, so the decider is every question's first
// answerer; a proposal's deadline is its give's time plus 3600 seconds, and in the two-waits Accord a
// brief's its give's time plus 60.
//
// It imports no package of the core.
package main

import "github.com/orilloai/placket/vectors/signatures/kit"

var (
	askAndLog = kit.AskAndLog().Build()
	twoWaits  = kit.TwoWaits().Build()
	passOK    = kit.Verdict{Verdict: "pass", Reason: "ok"}
	decider   = kit.Decider
	stranger  = kit.Stranger
)

// at is 09:mm:00 on the day of every vector.
func at(mm int) string {
	return "2026-11-12T09:" + string(rune('0'+mm/10)) + string(rune('0'+mm%10)) + ":00Z"
}

func message(set kit.Obj, id, kind string, to kit.Key) kit.Obj {
	return kit.Message(id, kit.HashOf(set), to, kit.TextPart(kind, kit.Salt("close "+id, 32), "The "+kind+" "+id+"."))
}

func scene(set kit.Obj) *kit.Scene {
	return kit.NewScene(set, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider, kit.Stranger)
}

func salt(label string) string { return kit.Salt("close answer "+label, 32) }

// asHex is a message and its envelope as a rejected give step gives them, in hex.
func asHex(m kit.Obj, seq int64, signer kit.Key) (kit.Obj, kit.Obj) {
	b := kit.Canon(m)
	return kit.Obj{"messageHex": kit.HexOf(b)}, kit.Obj{"envelopeHex": kit.HexOf(kit.Canon(kit.Envelope(seq, b, signer)))}
}

func list(rs []kit.Obj) []any {
	out := make([]any, len(rs))
	for i, r := range rs {
		out[i] = kit.Obj{"receipt": r}
	}
	return out
}

func verify(name, tests string, receipts []kit.Obj, expect kit.Obj) kit.Vector {
	return kit.Vector{Name: "close/" + name, Tests: tests, Operation: "verify", Members: kit.Obj{
		"reference": kit.Mediator.Ref, "receipts": list(receipts), "expect": expect,
	}}
}

func failsAt(code string, i int) kit.Obj { return kit.Obj{"code": code, "index": i} }

func verified(n int) kit.Obj { return kit.Obj{"verified": n} }

func idOf(r kit.Obj) string { return kit.Hash(kit.StatementOf(r)) }

// closedList is the list the verify vectors share: a note to the applier at 09:00, which passed and
// is never fetched; a proposal asked at 09:01; a second note at 09:02; and the proposal's closing at
// 09:30, closed, from the decider. It returns the scene, which has written the list and its close,
// and the four receipts before the close.
func closedList() (*kit.Scene, []kit.Obj) {
	s := scene(askAndLog)
	s.Give(at(0), kit.Proposer, 1, message(askAndLog, "note-1", "note", kit.Applier), kit.Applier, "pass", "ok")
	q := s.Give(at(1), kit.Proposer, 2, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
	s.Give(at(2), kit.Proposer, 3, message(askAndLog, "note-2", "note", kit.Applier), kit.Applier, "pass", "ok")
	s.CloseAccord(at(30), nil, []kit.Closing{{Question: q, From: decider}})
	w := s.Written()
	return s, w[:4]
}

// closeOf signs a close of the shared list at the time at, with the chains and unfetched given.
func closeOf(at string, chains, unfetched []any) kit.Obj {
	st := kit.CloseStatement(kit.HashOf(askAndLog), kit.HashOf(askAndLog["card"]), kit.Mediator.Ref, at, chains, unfetched)
	r, _ := kit.Receipt(kit.Canon(st), kit.Mediator)
	return r
}

func main() {
	var vs []kit.Vector
	add := func(name, tests string, s *kit.Scene, set kit.Obj) {
		vs = append(vs, s.Vector("close/"+name, tests, set))
	}

	// close-1: the order of a close.
	{
		s := scene(twoWaits)
		q1 := s.Give(at(0), kit.Proposer, 1, message(twoWaits, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		q2 := s.Give(at(10), kit.Proposer, 2, message(twoWaits, "brief-1", "brief", kit.Applier), kit.Applier, "escalate", "maybe")
		q3 := s.Give(at(20), kit.Proposer, 3, message(twoWaits, "proposal-2", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		s.CloseAccord(at(30),
			[]kit.Timeout{{Question: q2, Deadline: "2026-11-12T09:11:00Z", From: decider}},
			[]kit.Closing{{Question: q1, From: decider}, {Question: q3, From: decider}})
		add("three-questions-one-past-its-deadline",
			"close-1, close-2, conformance-vectors-3: three questions opened Q1 at 09:00, Q2, a brief, at 09:10 and Q3 at 09:20, with deadlines 10:00, 09:11 and 10:20; a close at 09:30 writes Q2's timeout at 09:11 by the deadline, then a closing of Q1 and one of Q3, block, closed, from the decider at 09:30 by the close, and the close receipt last; no question stays open", s, twoWaits)
	}

	// close-2: every chain and every delivery not fetched.
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, message(askAndLog, "note-1", "note", kit.Applier), kit.Applier, "pass", "ok")
		s.Give(at(1), kit.Decider, 1, message(askAndLog, "answer-1", "answer", kit.Proposer), kit.Proposer, "pass", "ok")
		s.Give(at(2), kit.Decider, 2, message(askAndLog, "version-1", "changed-version", kit.Applier), kit.Applier, "pass", "ok")
		q := s.Give(at(3), kit.Proposer, 2, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		s.Resolve(kit.Answer{At: at(4), Question: q, Action: "accepted", Text: "accepted", Salt: salt("accept"),
			By: decider, Verdict: passOK, Release: true, Released: &passOK})
		s.Get(kit.Applier)
		s.CloseAccord(at(30), nil, nil)
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		add("every-chain-and-every-delivery-not-fetched",
			"close-2, close-3: three chains, the proposer's to the applier at seq 3, the decider's to the proposer at seq 2 and the decider's to the applier at seq 1, and four deliveries not fetched, the decider's answer and version, the accept and its release; the close, written at once with no question open, pins each chain's last receipt in pair order and lists the four in the order queued, and the applier then fetches the version, the release, and the close", s, askAndLog)
	}

	// close-3: a give and an answer after the close.
	{
		s := scene(askAndLog)
		q := s.Give(at(0), kit.Proposer, 1, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		s.CloseAccord(at(30), nil, []kit.Closing{{Question: q, From: decider}})
		note := message(askAndLog, "note-1", "note", kit.Applier)
		m, e := asHex(note, 2, kit.Proposer)
		s.Reject(at(31), kit.Proposer, "closed", m, e)
		m, e = asHex(note, 1, stranger)
		s.Reject(at(31), stranger, "not-a-member", m, e)
		text := "accepted"
		s.RefuseCall(at(31), s.Call(q, "accepted", text, salt("late"), false, decider), &decider, text, salt("late"), "closed")
		s.RefuseCall(at(31), s.Call(q, "accepted", text, salt("stranger"), false, stranger), &stranger, text, salt("stranger"), "not-a-member")
		s.Tick("2026-11-12T23:00:00Z")
		add("a-give-and-an-answer-after-the-close",
			"close-3, section 4 row 1a, section 6 row 1a: after the close the proposer's give and the decider's accept are closed, a stranger's give and answer not-a-member, and nothing is written; a tick past every deadline writes nothing", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, message(askAndLog, "note-1", "note", kit.Applier), kit.Applier, "pass", "ok")
		s.CloseAccord(at(30), nil, nil)
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		s.Get(kit.Proposer)
		s.GetRejected(stranger, "not-a-member")
		add("fetches-after-the-close",
			"close-3, crossing-3: a closed Accord with one note queued for the applier: its first fetch returns the note, the second and third the same close receipt, the proposer's fetch the close, and a stranger's fetch is not-a-member", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.CloseAccord(at(0), nil, nil)
		s.Get(kit.Decider)
		add("an-accord-with-no-receipts",
			"close-2, close-4: an Accord with no receipts is closed: its close is the first receipt, with both lists empty, and a fetch returns it", s, askAndLog)
	}

	// close-4: verifying against a close.
	s, before := closedList()
	all := s.Written()
	closed, ask := all[4], before[1]
	asker, receiver := kit.Proposer.Ref, kit.Applier.Ref
	finals := func(change func([]any) []any) []any {
		f := append([]any{}, kit.Copy(s.Finals()).([]any)...)
		return change(f)
	}
	unfetched := s.Unfetched()
	with := func(c kit.Obj) []kit.Obj { return append(append([]kit.Obj{}, before...), c) }
	same := func(f []any) []any { return f }
	vs = append(vs,
		verify("a-closed-list-verifies", "close-4, section 8 rows 8 and 9: a list whose closing with reason closed is right before its close, at its at, and whose close pins both chains and the note never fetched, verifies", all, verified(5)),
		verify("a-close-cut-short-verifies", "close-1, close-4, receipt-6: a list cut after its closing with reason closed and before its close, as a write cut short leaves it, verifies", before, verified(4)),
		verify("a-receipt-after-the-close", "close-4, section 8 row 9: a note after the close, chained to the one before it, fails after-close at its index", append(append([]kit.Obj{}, all...), func() kit.Obj {
			t, _ := closedList()
			r, _ := t.Write("crossing", kit.Proposer, kit.Applier, "note", kit.Sum([]byte("close: after")), at(31), kit.Proposer, 4, kit.HashOf("close: after"), "pass", "ok", kit.ByTerms)
			return r
		}()), failsAt("after-close", 5)),
		verify("a-second-close", "close-4, section 8 row 9: a close after the close fails after-close at its index", append(append([]kit.Obj{}, all...), closed), failsAt("after-close", 5)),
		verify("a-close-payload-with-a-space-added", "receipt-1, close-2, section 8 row 1: a close whose payload has one space after the opening brace, signed by the mediator over those bytes, is not-canonical at its index", with(func() kit.Obj {
			r, _ := kit.Receipt(append([]byte("{ "), kit.StatementOf(closed)[1:]...), kit.Mediator)
			return r
		}()), failsAt("not-canonical", 4)),
		verify("a-seq-that-is-not-its-chains-last", "close-4, section 8 row 8: a close whose chain from the proposer to the applier names seq 2, the ask, not the note at seq 3, fails close at its index", with(closeOf(at(30), finals(func(f []any) []any {
			for i, x := range f {
				if x.(kit.Obj)["from"] == asker && x.(kit.Obj)["to"] == receiver {
					f[i] = kit.Final(asker, receiver, 2, idOf(ask))
				}
			}
			return f
		}), unfetched)), failsAt("close", 4)),
		verify("a-chain-left-out", "close-4, section 8 row 8: a close that lists one of the list's two chains fails close at its index", with(closeOf(at(30), finals(func(f []any) []any { return f[:1] }), unfetched)), failsAt("close", 4)),
		verify("a-chain-the-list-does-not-hold", "close-4, section 8 row 8: a close that lists a chain from the applier to the proposer, which the list does not hold, fails close at its index", with(closeOf(at(30), finals(func(f []any) []any {
			return append(f, kit.Final(receiver, asker, 1, idOf(before[0])))
		}), unfetched)), failsAt("close", 4)),
		verify("chains-out-of-pair-order", "close-4, section 8 row 8: a close whose two chains are listed out of pair order fails close at its index", with(closeOf(at(30), finals(func(f []any) []any { return []any{f[1], f[0]} }), unfetched)), failsAt("close", 4)),
		verify("an-unfetched-that-queued-nothing", "close-4, section 8 row 8: a close whose unfetched names the ask, which queued nothing, fails close at its index", with(closeOf(at(30), finals(same), []any{kit.Obj{"to": receiver, "receipt": idOf(ask)}})), failsAt("close", 4)),
		verify("an-unfetched-to-another-member", "close-4, section 8 row 8: a close whose unfetched names the note as queued for the decider, not the applier, fails close at its index", with(closeOf(at(30), finals(same), []any{kit.Obj{"to": kit.Decider.Ref, "receipt": idOf(before[0])}})), failsAt("close", 4)),
		verify("an-unfetched-named-twice", "close-4, section 8 row 8: a close whose unfetched names the note twice fails close at its index", with(closeOf(at(30), finals(same), append(append([]any{}, unfetched...), unfetched...))), failsAt("close", 4)),
		verify("an-at-other-than-its-closings", "close-4, section 8 row 8: a close at 09:31 after a closing with reason closed at 09:30 fails close at its index", with(closeOf(at(31), finals(same), unfetched)), failsAt("close", 4)),
		verify("a-closed-closing-followed-by-a-give", "close-4, section 8 row 8: a closing with reason closed followed by a note, and no close, fails close at the note's index", append(append([]kit.Obj{}, before...), func() kit.Obj {
			t, _ := closedList()
			r, _ := t.Write("crossing", kit.Proposer, kit.Applier, "note", kit.Sum([]byte("close: after a closing")), at(31), kit.Proposer, 4, kit.HashOf("close: after a closing"), "pass", "ok", kit.ByTerms)
			return r
		}()), failsAt("close", 4)),
	)

	// close-4: closings with reason closed at two times. Two proposals are asked, the first closed at
	// 09:30 and the second at 09:31, both from the decider; the second fails close at its own index,
	// whether it ends the list or the close follows it.
	{
		s := scene(askAndLog)
		q1 := s.Give(at(1), kit.Proposer, 1, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		p2 := message(askAndLog, "proposal-2", "proposal", kit.Applier)
		s.Give(at(2), kit.Proposer, 2, p2, kit.Applier, "escalate", "maybe")
		s.CloseAccord(at(30), nil, []kit.Closing{{Question: q1, From: decider}})
		w := s.Written()
		second, _ := s.Write("resolution", decider, kit.Proposer, "proposal", kit.Digest(kit.PartOf(p2)), at(31), kit.Proposer, 2, kit.Hash(kit.Canon(p2)), "block", "closed", kit.ByClose)
		two := append(append([]kit.Obj{}, w[:3]...), second)
		vs = append(vs,
			verify("closed-closings-at-two-times", "close-4, section 8 row 8: two closings with reason closed, at 09:30 and at 09:31, ending the list, as a close cut short leaves it: the second fails close at its own index", two, failsAt("close", 3)),
			verify("closed-closings-at-two-times-then-a-close", "close-4, section 8 row 8: two closings with reason closed, at 09:30 and at 09:31, followed by a close at 09:30: the second closing fails close at its own index, before the close is read", append(two, w[3]), failsAt("close", 3)),
		)
	}
	kit.Main("close", vs)
}
