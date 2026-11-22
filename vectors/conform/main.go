// Command conform writes the vectors of the conform group: section 8's conformance table (verify-1
// to verify-3). Each vector gives a verifier the Accord id, its founding set, a mediator's receipts,
// answer calls and, for the body row, a delivery, and expects the first failure's code and index, or
// that the receipts conform with the answers witnessed and unwitnessed counted. Every code, index and
// count is written here by hand from the text; the generator builds and signs the receipts and calls
// with the Go standard library alone (vectors/README.md). The journals are written by the kit's
// scene, which writes whatever verdict a generator gives it, so a receipt the terms would never
// write is still signed by the mediator's key.
//
// It imports no package of the core.
package main

import "github.com/orilloai/placket/vectors/signatures/kit"

var (
	askAndLog = kit.AskAndLog().Build()
	compose   = kit.Compose().Build()
	judged    = kit.Judged().Build()
	passOK    = kit.Verdict{Verdict: "pass", Reason: "ok"}
	decider   = kit.Decider
	proposer  = kit.Proposer
)

func at(n int) string { return "2026-11-12T09:0" + string(rune('0'+n)) + ":00Z" }

func message(set kit.Obj, id, kind string, to kit.Key) kit.Obj {
	return kit.Message(id, kit.HashOf(set), to, kit.TextPart(kind, kit.Salt("conform "+id, 32), "The "+kind+" "+id+"."))
}

func scene(set kit.Obj) *kit.Scene {
	return kit.NewScene(set, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider, kit.Stranger)
}

func salt(label string) string { return kit.Salt("conform answer "+label, 32) }

// An evidence is a conform vector's input: the receipts a scene wrote, the calls given, and the
// founding set and kernel the verifier trusts.
type evidence struct {
	set      kit.Obj
	accord   string
	receipts []kit.Obj
	calls    []kit.Obj
	kernel   string
	subset   bool
	delivery kit.Obj
	// sealed gives the verifier no opening; opened, when set, gives it only these members' openings,
	// as a dispute's verifier holds them; otherwise it holds every one (setup-contract-16).
	sealed bool
	opened []kit.Key
}

func of(set kit.Obj, s *kit.Scene, calls ...kit.Obj) evidence {
	return evidence{set: set, accord: kit.HashOf(set), receipts: s.Written(), calls: calls, kernel: kit.KernelHash}
}

func (e evidence) vector(name, tests string, expect kit.Obj) kit.Vector {
	rs := make([]any, len(e.receipts))
	for i, r := range e.receipts {
		rs[i] = kit.Obj{"receipt": r}
	}
	cs := make([]any, len(e.calls))
	for i, c := range e.calls {
		cs[i] = kit.Obj{"call": c}
	}
	m := kit.Obj{"accord": e.accord, "founding": e.set, "acceptances": kit.Acceptances(e.set), "receipts": rs, "answers": cs, "kernel": e.kernel, "expect": expect}
	switch {
	case e.sealed:
	case e.opened != nil:
		var l []any
		for _, o := range kit.Openings(e.set) {
			for _, k := range e.opened {
				if o.(kit.Obj)["actor"] == k.Ref {
					l = append(l, o)
				}
			}
		}
		m["openings"] = l
	default:
		m["openings"] = kit.Openings(e.set)
	}
	if e.subset {
		m["subset"] = true
	}
	if e.delivery != nil {
		m["delivery"] = e.delivery
	}
	return kit.Vector{Name: "conform/" + name, Tests: tests, Operation: "conform", Members: m}
}

// notChecked marks a conforms expectation as found without every opening (setup-contract-16).
func notChecked(o kit.Obj) kit.Obj { o["terms"] = "not-checked"; return o }

func fails(code string, index int) kit.Obj { return kit.Obj{"code": code, "index": index} }

func conforms(witnessed, unwitnessed, unmatched int, module bool) kit.Obj {
	return kit.Obj{"conforms": true, "witnessed": witnessed, "unwitnessed": unwitnessed, "unmatched": unmatched, "module": module}
}

// worked is the ask-and-log journal: a note that passes, a proposal asked about, and the decider's
// accept with release; and the decider's call. Receipts 0 to 3: the note, the ask, the accept and
// the release.
func worked() (*kit.Scene, kit.Obj) {
	s := scene(askAndLog)
	s.Give(at(0), kit.Proposer, 1, message(askAndLog, "note-1", "note", kit.Applier), kit.Applier, "pass", "ok")
	q := s.Give(at(1), kit.Proposer, 2, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
	call := s.Resolve(kit.Answer{At: at(2), Question: q, Action: "accepted", Text: "accepted", Salt: salt("worked"),
		By: decider, Verdict: passOK, Release: true, Released: &passOK})
	return s, call
}

// within is the ask-and-log Accord in which the proposer's proposals go to the party named and are
// asked about by the decider, and that party gets them.
func within(receiver kit.Key) kit.Obj {
	s := kit.AskAndLog()
	role := map[string]string{kit.Decider.Name: "decider", kit.Proposer.Name: "proposer"}[receiver.Name]
	s.Parties[0].Terms[0] = kit.Maybe("give", []string{role}, []string{"proposal"}, []string{"decider"}, 3600)
	for i := range s.Parties {
		if s.Parties[i].Key.Ref == receiver.Ref {
			s.Parties[i].Terms = append(s.Parties[i].Terms, kit.Term("want", "get", []string{"proposer"}, []string{"proposal"}))
		}
	}
	return s.Build()
}

func main() {
	var vs []kit.Vector
	add := func(v kit.Vector) { vs = append(vs, v) }

	{
		s, call := worked()
		add(of(askAndLog, s, call).vector("a-worked-journal-conforms",
			"verify-1, verify-2, verify-3: a note, an ask and an accept with release conform, and the accept and its release are witnessed by the decider's call", conforms(2, 0, 0, true)))
		add(of(askAndLog, s).vector("an-answer-unwitnessed",
			"verify-3: the same journal with no call is unwitnessed, never a failure", conforms(0, 2, 0, true)))
		add(of(askAndLog, s, call, kit.AnswerCall(kit.HashOf(askAndLog), kit.HashOf("conform: no question"), "declined", "", false, decider)).vector("a-call-matching-no-receipt",
			"verify-2: a call that matches no receipt is counted and fails nothing", conforms(2, 0, 1, true)))
		e := of(askAndLog, s, call)
		e.receipts = e.receipts[2:]
		e.subset = true
		add(e.vector("a-subset-conforms", "verify-1, receipt-6: the accept and its release alone, given as a subset, are outside their question's checks and conform", conforms(0, 2, 0, true)))
		e = of(askAndLog, s, call)
		e.accord = kit.HashOf(compose)
		add(e.vector("a-founding-set-of-another-accord", "verify-2 row 0: a founding set whose typed hash is not the Accord id given is founding", fails("founding", -1)))
		e = of(askAndLog, s, call)
		e.delivery = s.Delivered(kit.Applier)[1]
		add(e.vector("a-body-that-is-the-part", "verify-2 row 5: the released proposal's delivery, its body under its salt, is the part its receipt names", kit.Obj{
			"conforms": true, "witnessed": 2, "unwitnessed": 0, "unmatched": 0, "module": true, "body": true}))
		changed := kit.Copy(e.delivery).(kit.Obj)
		changed["text"] = "The proposal proposal-1!"
		e.delivery = changed
		add(e.vector("a-body-changed", "verify-2 row 5: the released proposal's delivery with its text changed is body, at its receipt's index", fails("body", 3)))
		// Row 5 reads the delivery's format first: a member missing, or one it does not list, is no
		// delivery, at index -1, though its body is the part.
		missing := kit.Copy(s.Delivered(kit.Applier)[1]).(kit.Obj)
		delete(missing, "messageId")
		e.delivery = missing
		add(e.vector("a-delivery-missing-a-member", "verify-1 row 5: the released proposal's delivery with no messageId is no delivery, body at index -1, though its body is the part", fails("body", -1)))
		unknown := kit.Copy(s.Delivered(kit.Applier)[1]).(kit.Obj)
		unknown["topic"] = "merges"
		e.delivery = unknown
		add(e.vector("a-delivery-with-an-unknown-member", "verify-1 row 5: the released proposal's delivery with a member topic is no delivery, body at index -1, though its body is the part", fails("body", -1)))
	}
	{
		// Row 1 before row 2: a receipt of another Accord from a stranger.
		s := scene(compose)
		s.Give(at(0), kit.Stranger, 1, message(compose, "brief-1", "brief", kit.Applier), kit.Applier, "pass", "ok")
		e := of(askAndLog, s)
		add(e.vector("another-accord-from-a-stranger", "verify-2 rows 1 and 2: a receipt of another Accord from a stranger is accord, the earlier row", fails("accord", 0)))
		s = scene(askAndLog)
		s.Give(at(0), kit.Stranger, 1, message(askAndLog, "note-1", "note", kit.Applier), kit.Applier, "pass", "ok")
		add(of(askAndLog, s).vector("a-stranger-gives", "verify-2 row 2: a receipt from a key that is no member is party", fails("party", 0)))
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, message(askAndLog, "tests-1", "tests", kit.Applier), kit.Applier, "pass", "ok")
		add(of(askAndLog, s).vector("a-pass-the-terms-block", "verify-2 row 3: a pass ok of a kind no term names, which the terms block, is decision", fails("decision", 0)))
		e := of(askAndLog, s)
		e.kernel = kit.HashOf("conform: another module")
		add(e.vector("another-module", "verify-2: under a card whose kernel is not the trusted one the decision row is skipped, and the same receipt conforms", conforms(0, 0, 0, false)))
	}
	{
		// Founding without the openings, and with some (setup-contract-16, verify-1): the decision row
		// reads only terms whose seals are opened.
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, message(askAndLog, "tests-1", "tests", kit.Applier), kit.Applier, "pass", "ok")
		e := of(askAndLog, s)
		e.sealed = true
		add(e.vector("sealed-a-pass-the-terms-block", "verify-1, setup-contract-16: with no opening the terms are not checked, the decision row is skipped, and a pass the terms would block conforms", notChecked(conforms(0, 0, 0, true))))
		e = of(askAndLog, s)
		e.opened = []kit.Key{kit.Proposer}
		add(e.vector("sealed-one-side-opened", "verify-1, setup-contract-16: with the sender's opening alone the receiver's terms are sealed, so the decision row is skipped for that pass and it conforms", notChecked(conforms(0, 0, 0, true))))
		e = of(askAndLog, s)
		e.opened = []kit.Key{kit.Proposer, kit.Applier}
		add(e.vector("sealed-both-sides-opened", "verify-1, setup-contract-16: with the openings of both sides of the pass, though not the decider's, the decision row reads it again and it is decision", fails("decision", 0)))
		w, call := worked()
		e = of(askAndLog, w, call)
		e.sealed = true
		add(e.vector("sealed-a-worked-journal-conforms", "verify-1, verify-3, setup-contract-16: the worked journal with no opening conforms with its terms not checked, and the decider's call witnesses the accept and its release", notChecked(conforms(2, 0, 0, true))))
	}
	{
		s := scene(askAndLog)
		q := s.Give(at(0), kit.Proposer, 1, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		call := s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: salt("proposer"),
			By: proposer, Verdict: passOK})
		add(of(askAndLog, s, call).vector("an-accept-from-a-non-answerer", "verify-2 row 3: an accept from the proposer, who holds no answering role, is decision", fails("decision", 1)))
	}
	{
		s := scene(askAndLog)
		q := s.Give(at(0), kit.Proposer, 1, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		s.Tick("2026-11-12T10:00:05Z", kit.Timeout{Question: q, Deadline: "2026-11-12T10:00:05Z", From: decider})
		add(of(askAndLog, s).vector("a-timeout-not-at-its-deadline", "verify-2 row 3: a timeout whose at is not the question's deadline, 10:00:00, is decision", fails("decision", 1)))
	}
	{
		s := scene(judged)
		s.GiveJudged(at(0), kit.Proposer, 1, message(judged, "digest-1", "digest", kit.Applier), kit.Applier, kit.Judgment("jev", "pass"), "pass", "ok", kit.ByJudge("jev"))
		add(of(judged, s).vector("a-judge-pass-whose-judge-advises", "verify-2 row 3: a pass by jev where both sides grant jev only to advise is decision", fails("decision", 0)))
		s = scene(judged)
		s.GiveJudged(at(0), kit.Proposer, 1, message(judged, "report-1", "report", kit.Applier), kit.Applier, kit.Judgment("jev", "block"), "block", "judge:jev", kit.ByJudge("jev"))
		add(of(judged, s).vector("a-judge-block-conforms", "verify-2 row 3: a block by jev where both sides grant jev to decide conforms, the judgment taken as the receipt claims it", conforms(0, 0, 0, true)))
	}
	{
		// The witness row (row 4), each on an accept with release of the worked question.
		witness := func(name, tests string, call kit.Obj, release bool, released *kit.Verdict, expect kit.Obj) {
			s := scene(askAndLog)
			q := s.Give(at(0), kit.Proposer, 1, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
			c := call
			if c == nil {
				c = s.Call(q, "accepted", "accepted", salt("witness"), release, decider)
			} else {
				c["question"] = q
				c = kit.Signed(kit.Unsigned(c), kit.TypeAnswer, keyOf(c["sig"]))
			}
			s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: salt("witness"),
				By: decider, Verdict: passOK, Release: release, Released: released, Call: c})
			add(of(askAndLog, s, c).vector(name, tests, expect))
		}
		digest := kit.Sum(kit.FromHex(salt("witness")), []byte("accepted"))
		forged := kit.Obj{"accord": kit.HashOf(askAndLog), "action": "accepted", "digest": digest, "from": decider.Ref, "release": true, "sig": "proposer"}
		witness("a-call-signed-by-another-key", "verify-2 row 4: the matched call names the decider but the proposer's key signed it, so it does not read: witness", forged, true, &passOK, fails("witness", 1))
		other := kit.Obj{"accord": kit.HashOf(compose), "action": "accepted", "digest": digest, "from": decider.Ref, "release": true, "sig": "decider"}
		witness("a-call-of-another-accord", "verify-2 row 4: the matched call names another Accord's id: witness", other, true, &passOK, fails("witness", 1))
		witness("a-release-the-call-did-not-ask-for", "verify-2 row 4: a release receipt whose matched call has release false: witness, at the release", nil, false, &passOK, fails("witness", 2))
		witness("a-release-missing", "verify-2 row 4: a passing accept whose matched call has release true and no release receipt after it: witness, at the accept", nil, true, nil, fails("witness", 1))
	}
	for _, receiver := range []kit.Key{kit.Decider, kit.Proposer} {
		set := within(receiver)
		s := scene(set)
		q := s.Give(at(0), kit.Proposer, 1, message(set, "proposal-1", "proposal", receiver), receiver, "escalate", "maybe")
		call := s.Resolve(kit.Answer{At: at(1), Question: q, Action: "changed", Text: "Rename total to amount_due.", Salt: salt("within"),
			By: decider, Verdict: passOK, Release: true})
		name := map[string]string{kit.Decider.Name: "a-change-to-its-answerer-releases-nothing", kit.Proposer.Name: "a-change-to-its-asker-releases-nothing"}[receiver.Name]
		add(of(set, s, call).vector(name, "verify-2 row 4, maybe-10: a passing change with release true whose question's receiver is the "+receiver.Name+" releases nothing, and conforms with no release receipt", conforms(1, 0, 0, true)))
	}

	// The close's closings and the close itself (verify-2 rows 1 to 3, close-4).
	for _, c := range []struct{ name, tests, at string }{
		{"close-a-closed-closing-conforms", "verify-2 rows 2 and 3, close-1, close-4: a closing with reason closed from the decider, the first answerer, at 09:30, before its deadline of 10:00, conforms, by the mediator with the event close; the close after it goes through rows 0 and 1 alone and is no decision", "2026-11-12T09:30:00Z"},
		{"close-a-closing-at-its-deadline", "verify-2 row 3, close-1: a closing with reason closed whose at, 10:00, is its question's deadline is decision; its close at the same time is valid", "2026-11-12T10:00:00Z"},
	} {
		s := scene(askAndLog)
		q := s.Give(at(0), kit.Proposer, 1, message(askAndLog, "proposal-1", "proposal", kit.Applier), kit.Applier, "escalate", "maybe")
		s.CloseAccord(c.at, nil, []kit.Closing{{Question: q, From: decider}})
		expect := conforms(0, 0, 0, true)
		if c.at == "2026-11-12T10:00:00Z" {
			expect = fails("decision", 1)
		}
		add(of(askAndLog, s).vector(c.name, c.tests, expect))
	}
	{
		other := kit.HashOf("conform: another Accord")
		st := kit.CloseStatement(other, kit.HashOf(askAndLog["card"]), kit.Mediator.Ref, at(5), nil, nil)
		r, _ := kit.Receipt(kit.Canon(st), kit.Mediator)
		e := evidence{set: askAndLog, accord: kit.HashOf(askAndLog), receipts: []kit.Obj{r}, kernel: kit.KernelHash}
		add(e.vector("close-a-close-of-another-accord", "verify-2 rows 0 and 1, close-4: a close that names another Accord, the only receipt of the list, founds the set at its at and is accord at its index", fails("accord", 0)))
	}
	kit.Main("conform", vs)
}

// keyOf is the key a witness vector's call is signed by, named in its sig until it is signed.
func keyOf(name any) kit.Key {
	if name == "proposer" {
		return kit.Proposer
	}
	return kit.Decider
}
