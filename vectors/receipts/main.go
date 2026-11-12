// Command receipts writes the vectors of the receipts group: sections 7, 8 and 9 of the protocol,
// receipts and their chains, verifying receipts and reconciling kept records. Every code, index,
// verdict and list is written here by hand from the text; the generator builds and signs the
// receipts with the Go standard library alone (vectors/README.md).
//
// It imports no package of the core.
package main

import (
	"strings"

	"github.com/orilloai/placket/vectors/signatures/kit"
)

var (
	askAndLog = kit.AskAndLog().Build()
	compose   = kit.Compose().Build()
	accord    = kit.HashOf(askAndLog)
	card      = kit.HashOf(askAndLog["card"])
)

func at(n int) string {
	return "2026-11-12T10:" + string(rune('0'+n/10)) + string(rune('0'+n%10)) + ":00Z"
}

func note(id string) kit.Obj {
	return kit.Message(id, accord, kit.Applier, kit.TextPart("note", kit.Salt("receipts "+id, 16), "The note "+id+"."))
}

// A journal writes receipts one after another as a mediator would, each chained in its pair.
type journal struct {
	s *kit.Scene
	n int
}

func newJournal(set kit.Obj) *journal { return &journal{s: kit.NewScene(set, kit.Mediator)} }

// envelope is an echoed envelope.
type envelope struct {
	sender kit.Key
	seq    int64
	hash   string
}

func env(sender kit.Key, seq int64, label string) envelope {
	return envelope{sender, seq, kit.HashOf("receipts message " + label)}
}

// subject is a receipt's subject.
type subject struct{ name, digest string }

func part(name, label string) subject {
	return subject{name, kit.Sum([]byte("receipts part " + label))}
}

func (j *journal) write(kind string, from, to kit.Key, sub subject, e envelope, verdict, reason string) kit.Obj {
	r, _ := j.s.Write(kind, from, to, sub.name, sub.digest, at(j.n), e.sender, e.seq, e.hash, verdict, reason)
	j.n++
	return r
}

func (j *journal) crossing(from, to kit.Key, sub subject, e envelope, verdict, reason string) kit.Obj {
	return j.write("crossing", from, to, sub, e, verdict, reason)
}

func (j *journal) resolution(from, to kit.Key, sub subject, e envelope, verdict, reason string) kit.Obj {
	return j.write("resolution", from, to, sub, e, verdict, reason)
}

func list(rs ...kit.Obj) []any {
	out := make([]any, len(rs))
	for i, r := range rs {
		out[i] = kit.Obj{"receipt": r}
	}
	return out
}

func verify(name, tests string, receipts []any, expect kit.Obj) kit.Vector {
	return kit.Vector{Name: "receipts/" + name, Tests: tests, Operation: "verify", Members: kit.Obj{
		"reference": kit.Mediator.Ref, "receipts": receipts, "expect": expect,
	}}
}

func failsAt(code string, i int) kit.Obj { return kit.Obj{"code": code, "index": i} }

func verified(n int) kit.Obj { return kit.Obj{"verified": n} }

// resign replaces a receipt's statement and signs it again with the mediator's key.
func resign(r kit.Obj, change func(st kit.Obj)) kit.Obj {
	st := decode(r)
	change(st)
	out, _ := kit.Receipt(kit.Canon(st), kit.Mediator)
	return out
}

// decode reads a receipt's statement into objects.
func decode(r kit.Obj) kit.Obj { return kit.Parse(kit.StatementOf(r)).(kit.Obj) }

func predicate(st kit.Obj) kit.Obj { return st["predicate"].(kit.Obj) }

func idOf(r kit.Obj) string { return kit.Hash(kit.StatementOf(r)) }

func main() {
	var vs []kit.Vector

	// Scenarios: the chains a mediator writes.
	{
		s := kit.NewScene(askAndLog, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider)
		for i := 1; i <= 3; i++ {
			s.Give(at(i), kit.Proposer, int64(i), note("note-"+string(rune('0'+i))), kit.Applier, "pass", "ok")
		}
		vs = append(vs, s.Vector("receipts/three-gives-are-echoed",
			"crossing-5, receipt-5: three gives are echoed by seq and hash; the first of the pair has seq 1 and no prev, each later one the one before as prev", askAndLog))
	}
	{
		s := kit.NewScene(askAndLog, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider)
		s.Give(at(0), kit.Proposer, 1, note("note-1"), kit.Applier, "pass", "ok")
		back := kit.Message("back-1", accord, kit.Proposer, kit.TextPart("note", kit.Salt("receipts back", 16), "Back."))
		s.Give(at(1), kit.Applier, 1, back, kit.Proposer, "block", "no-match")
		proposal := kit.Message("decider-proposal", accord, kit.Applier, kit.TextPart("proposal", kit.Salt("receipts decider", 16), "A proposal."))
		s.Give(at(2), kit.Decider, 1, proposal, kit.Applier, "block", "no-match")
		s.Give(at(3), kit.Proposer, 2, note("note-2"), kit.Applier, "pass", "ok")
		vs = append(vs, s.Vector("receipts/each-pair-its-own-chain",
			"receipt-2, receipt-5: a blocked give names the attempted part; each ordered pair has its own chain, so the proposer's second note has seq 2", askAndLog))
	}

	// Verifying: section 8 row by row.
	j := newJournal(askAndLog)
	a := j.crossing(kit.Proposer, kit.Applier, part("note", "a"), env(kit.Proposer, 1, "a"), "pass", "ok")
	b := j.crossing(kit.Proposer, kit.Applier, part("note", "b"), env(kit.Proposer, 2, "b"), "pass", "ok")
	c := j.crossing(kit.Proposer, kit.Applier, part("note", "c"), env(kit.Proposer, 3, "c"), "pass", "ok")

	edited := kit.Copy(b).(kit.Obj)
	est := decode(b)
	predicate(est)["verdict"], predicate(est)["reason"] = "block", "no-match"
	edited["payload"] = kit.B64(kit.Canon(est))

	otherKey := kit.Copy(b).(kit.Obj)
	otherKey["signatures"].([]any)[0].(kit.Obj)["sig"] = kit.B64(kit.OtherMediator.Sign(kit.PAE(kit.PayloadType, kit.StatementOf(b))))

	namesOther := resign(a, func(st kit.Obj) { predicate(st)["mediator"] = kit.OtherMediator.Ref })

	badType := kit.Copy(c).(kit.Obj)
	badType["payloadType"] = "application/json"

	vs = append(vs,
		verify("a-chain-verifies", "receipt-6: three receipts of one pair verify", list(a, b, c), verified(3)),
		verify("an-empty-list-verifies", "receipt-6: an empty list verifies", []any{}, verified(0)),
		verify("a-list-cut-short-verifies", "receipt-6: a list cut short after any receipt still verifies", list(a, b), verified(2)),
		verify("an-edited-receipt", "receipt-6: a second receipt whose payload was replaced by its statement with the verdict changed fails signature at 1",
			list(a, edited, c), failsAt("signature", 1)),
		verify("signed-again-by-another-key", "receipt-6: a receipt signed again by another key fails signature", list(a, otherKey), failsAt("signature", 1)),
		verify("a-receipt-that-names-another-mediator", "receipt-6: a receipt signed by the given key that names another mediator fails mediator at 0",
			list(namesOther), failsAt("mediator", 0)),
		verify("a-removed-middle-receipt", "receipt-6: three receipts of one pair with the second removed fail chain at 1", list(a, c), failsAt("chain", 1)),
		verify("the-first-failure-of-the-first-failing-receipt", "receipt-6, section 8: a bad payloadType at 2 comes before bytes that are not JSON at 3",
			append(list(a, b, badType), kit.Obj{"receiptHex": kit.HexOf([]byte("not json"))}), failsAt("payload-type", 2)),
		kit.Vector{Name: "receipts/a-mediator-that-is-not-a-reference", Tests: "receipt-6, section 8 row 0: the mediator mediator is not a reference, and no receipt is read",
			Operation: "verify", Members: kit.Obj{"reference": "mediator", "receipts": append(list(a), kit.Obj{"receiptHex": "00"}), "expect": kit.Obj{"code": "reference"}}},
	)

	// Row 1 to row 3, one defect each.
	twoSigs := kit.Copy(a).(kit.Obj)
	twoSigs["signatures"] = append(twoSigs["signatures"].([]any), kit.Copy(twoSigs["signatures"].([]any)[0]))
	extra := kit.Copy(a).(kit.Obj)
	extra["note"] = "extra"
	keyid := kit.Copy(a).(kit.Obj)
	keyid["signatures"].([]any)[0].(kit.Obj)["keyid"] = kit.OtherMediator.Ref
	short := kit.Copy(a).(kit.Obj)
	short["signatures"].([]any)[0].(kit.Obj)["sig"] = kit.B64(kit.FromB64(short["signatures"].([]any)[0].(kit.Obj)["sig"].(string))[:63])
	notB64 := kit.Copy(a).(kit.Obj)
	notB64["payload"] = "!" + notB64["payload"].(string)[1:]
	dupPayload := kit.Copy(a).(kit.Obj)
	dupPayload["payload"] = kit.B64([]byte(`{"a":1,"a":2}`))
	dupText := strings.Replace(string(kit.Canon(a)), `"payloadType":`, `"payloadType":"x","payloadType":`, 1)
	vs = append(vs,
		verify("receipt-bytes-not-canonical", "section 8 row 1: receipt bytes with a repeated name are not-canonical", []any{kit.Obj{"receiptText": dupText}}, failsAt("not-canonical", 0)),
		verify("payload-bytes-not-canonical", "section 8 row 1: a payload whose bytes repeat a name is not-canonical", list(dupPayload), failsAt("not-canonical", 0)),
		verify("payload-not-in-base64", "section 8 row 3: a payload that is not standard base64 fails signature", list(notB64), failsAt("signature", 0)),
		verify("two-signatures", "section 8 row 3: two signature entries fail signature", list(twoSigs), failsAt("signature", 0)),
		verify("a-member-the-receipt-does-not-list", "section 8 row 3: a member beside the three fails signature", list(extra), failsAt("signature", 0)),
		verify("a-keyid-of-another-mediator", "section 8 row 3: a keyid other than the given mediator's fails signature", list(keyid), failsAt("signature", 0)),
		verify("a-signature-of-63-bytes", "section 8 row 3: a sig that is not 64 bytes fails signature", list(short), failsAt("signature", 0)),
	)

	// Row 4: the statement and the predicate (receipt-1 to receipt-4, receipt-9, receipt-10).
	maybeBlock := resign(a, func(st kit.Obj) { predicate(st)["verdict"], predicate(st)["reason"] = "block", "maybe" })
	okBlock := resign(a, func(st kit.Obj) { predicate(st)["verdict"], predicate(st)["reason"] = "block", "ok" })
	declined := resign(a, func(st kit.Obj) { predicate(st)["verdict"], predicate(st)["reason"] = "block", "declined" })
	result := resign(a, func(st kit.Obj) { predicate(st)["result"] = "pass" })
	otherSender := resign(a, func(st kit.Obj) { predicate(st)["envelope"].(kit.Obj)["sender"] = kit.Decider.Ref })
	wrongType := resign(a, func(st kit.Obj) { st["_type"] = "https://in-toto.io/Statement/v0.1" })
	twoSubjects := resign(a, func(st kit.Obj) { st["subject"] = append(st["subject"].([]any), kit.Copy(st["subject"].([]any)[0])) })
	leap := resign(a, func(st kit.Obj) { predicate(st)["at"] = "2026-11-12T23:59:60Z" })
	escalateResolution := resign(a, func(st kit.Obj) {
		predicate(st)["kind"], predicate(st)["verdict"], predicate(st)["reason"] = "resolution", "escalate", "maybe"
	})
	checkReason := resign(a, func(st kit.Obj) { predicate(st)["verdict"], predicate(st)["reason"] = "block", "check:forbidden" })
	vs = append(vs,
		verify("a-block-with-reason-maybe", "receipt-4: a crossing predicate with verdict block and reason maybe is accepted", list(maybeBlock), verified(1)),
		verify("a-block-with-reason-ok", "receipt-4: a crossing predicate with verdict block and reason ok is refused", list(okBlock), failsAt("statement-type", 0)),
		verify("a-resolution-reason-on-a-crossing-receipt", "receipt-4: a crossing predicate with reason declined is refused", list(declined), failsAt("statement-type", 0)),
		verify("a-reserved-check-reason", "receipt-4: check:<name> is read for shape only and accepted", list(checkReason), verified(1)),
		verify("an-escalate-resolution", "receipt-4: a resolution receipt's verdict is pass or block, never escalate", list(escalateResolution), failsAt("statement-type", 0)),
		verify("an-unlisted-predicate-member", "receipt-9: a predicate with a member result is refused", list(result), failsAt("statement-type", 0)),
		verify("a-gives-receipt-echoes-another-members-envelope", "receipt-6: a crossing receipt from the proposer whose envelope's sender is the decider fails statement-type at 0",
			list(otherSender), failsAt("statement-type", 0)),
		verify("a-statement-of-another-type", "receipt-1: a statement whose _type is not in-toto's Statement v1 fails statement-type", list(wrongType), failsAt("statement-type", 0)),
		verify("two-subjects", "receipt-2: a subject list of two entries fails statement-type", list(twoSubjects), failsAt("statement-type", 0)),
		verify("a-time-with-second-60", "receipt-10: an at with second 60 fails statement-type", list(leap), failsAt("statement-type", 0)),
	)

	// Row 6: chains.
	firstWithPrev := resign(a, func(st kit.Obj) { predicate(st)["prev"] = idOf(c) })
	startsAt2 := resign(a, func(st kit.Obj) { predicate(st)["seq"] = 2; predicate(st)["prev"] = idOf(c) })
	wrongPrev := resign(b, func(st kit.Obj) { predicate(st)["prev"] = idOf(c) })
	vs = append(vs,
		verify("a-first-receipt-with-a-prev", "receipt-5: the first receipt of a pair with a prev fails chain", list(firstWithPrev), failsAt("chain", 0)),
		verify("a-first-receipt-at-seq-2", "receipt-5: the first receipt of a pair at seq 2 fails chain", list(startsAt2), failsAt("chain", 0)),
		verify("a-prev-that-is-not-the-one-before", "receipt-5: a second receipt whose prev is not the first's id fails chain at 1", list(a, wrongPrev), failsAt("chain", 1)),
	)

	// Row 7: questions and releases (receipt-6's rule).
	asked := part("proposal", "asked")
	qenv := env(kit.Proposer, 4, "asked")
	ask := func(j *journal) kit.Obj {
		return j.crossing(kit.Proposer, kit.Applier, asked, qenv, "escalate", "maybe")
	}
	answer := part("answer", "answer")
	version := part("changed-version", "version")
	{
		j := newJournal(askAndLog)
		q := ask(j)
		rel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")
		vs = append(vs, verify("a-release-with-no-answer-before-it", "receipt-6: an ask followed by a release that echoes its envelope, with no resolution, fails release at 1",
			list(q, rel), failsAt("release", 1)))
	}
	{
		j := newJournal(askAndLog)
		q1 := ask(j)
		q2 := ask(j)
		dec := j.resolution(kit.Decider, kit.Proposer, asked, qenv, "block", "declined")
		vs = append(vs, verify("a-second-ask-with-an-earlier-questions-envelope",
			"receipt-6: a second ask echoing an earlier question's envelope opens its own question, and the decline closes it", list(q1, q2, dec), verified(3)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Proposer, answer, qenv, "pass", "ok")
		rel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")
		extraRel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")
		vs = append(vs,
			verify("an-accept-and-its-release", "receipt-6: an ask, an accepted answer to the asker, and the release from the asker to the receiver verify",
				list(q, ans, rel), verified(3)),
			verify("a-second-release", "receipt-6: a second release of one question fails release", list(q, ans, rel, extraRel), failsAt("release", 3)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Proposer, answer, qenv, "pass", "ok")
		rel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "block", "no-match")
		vs = append(vs, verify("a-blocked-release", "receipt-6: a release with verdict block is a blocked release and verifies", list(q, ans, rel), verified(3)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ch := j.resolution(kit.Decider, kit.Proposer, version, qenv, "pass", "ok")
		rel := j.resolution(kit.Decider, kit.Applier, version, qenv, "pass", "ok")
		vs = append(vs, verify("a-change-and-its-release", "receipt-6: an ask, a change to the asker and its release from the answerer to the receiver under one digest verify",
			list(q, ch, rel), verified(3)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ch := j.resolution(kit.Decider, kit.Proposer, version, qenv, "pass", "ok")
		rel := j.resolution(kit.Decider, kit.Applier, part("changed-version", "another"), qenv, "pass", "ok")
		vs = append(vs, verify("a-change-released-under-another-digest", "receipt-6: a change's release must name the version the answer named", list(q, ch, rel), failsAt("release", 2)))
	}
	{
		j := newJournal(askAndLog)
		q := j.crossing(kit.Proposer, kit.Decider, asked, qenv, "escalate", "maybe")
		ch := j.resolution(kit.Decider, kit.Proposer, version, qenv, "pass", "ok")
		rel := j.resolution(kit.Decider, kit.Decider, version, qenv, "pass", "ok")
		vs = append(vs, verify("a-change-released-to-its-answerer", "receipt-6, maybe-10: a change releases nothing when the receiver is the answerer", list(q, ch, rel), failsAt("release", 2)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		dec := j.resolution(kit.Decider, kit.Proposer, asked, qenv, "block", "declined")
		rel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")
		vs = append(vs,
			verify("a-decline-closes-a-question", "receipt-6: a decline addressed to the asker naming the asked part closes its question", list(q, dec), verified(2)),
			verify("a-release-after-a-decline", "receipt-6: a release after a closing that is not an answer fails release", list(q, dec, rel), failsAt("release", 2)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		dec := j.resolution(kit.Decider, kit.Proposer, part("proposal", "other"), qenv, "block", "declined")
		vs = append(vs, verify("a-decline-of-another-part", "receipt-6: a decline whose subject is not the asked part fails release", list(q, dec), failsAt("release", 1)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Applier, answer, qenv, "pass", "ok")
		vs = append(vs, verify("a-closing-addressed-to-the-receiver", "receipt-6: a question's first receipt must be a resolution addressed to the asker", list(q, ans), failsAt("release", 1)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Proposer, answer, qenv, "pass", "ok")
		other := j.crossing(kit.Proposer, kit.Applier, part("note", "between"), env(kit.Proposer, 5, "between"), "pass", "ok")
		rel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")
		vs = append(vs, verify("a-release-not-right-after-its-answer", "receipt-6: a release that does not follow its answer at once fails release",
			list(q, ans, other, rel), failsAt("release", 3)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Proposer, answer, qenv, "block", "no-match")
		rel := j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")
		vs = append(vs, verify("a-release-after-a-blocked-answer", "receipt-6: an answer that was blocked releases nothing", list(q, ans, rel), failsAt("release", 2)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Proposer, answer, qenv, "pass", "ok")
		rel := j.resolution(kit.Decider, kit.Applier, answer, qenv, "pass", "ok")
		vs = append(vs, verify("an-accepts-release-of-the-wrong-form", "receipt-6: an accept's release is a crossing receipt from the asker, not a resolution",
			list(q, ans, rel), failsAt("release", 2)))
	}
	{
		j := newJournal(askAndLog)
		res := j.resolution(kit.Decider, kit.Proposer, asked, qenv, "block", "declined")
		vs = append(vs, verify("a-resolution-that-belongs-to-no-question", "receipt-6: a resolution receipt that belongs to no question fails release", list(res), failsAt("release", 0)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		cl := j.resolution(kit.Decider, kit.Proposer, part("note", "wrong-name"), qenv, "pass", "ok")
		vs = append(vs, verify("a-closing-of-the-wrong-subject", "receipt-6: an answer's subject name is answer or changed-version", list(q, cl), failsAt("release", 1)))
	}
	{
		j := newJournal(askAndLog)
		q := ask(j)
		ans := j.resolution(kit.Decider, kit.Proposer, answer, qenv, "pass", "ok")
		dup := kit.Obj{"receiptText": strings.Replace(string(kit.Canon(ans)), `"payloadType":`, `"payloadType":"x","payloadType":`, 1)}
		vs = append(vs, verify("a-release-failure-before-a-later-row", "section 8: every row of one receipt runs before the next, so a release failure at 1 comes before bytes refused at 2",
			append(list(q, j.crossing(kit.Proposer, kit.Applier, asked, qenv, "pass", "ok")), dup), failsAt("release", 1)))
	}

	vs = append(vs, reconcile()...)
	kit.Main("receipts", vs)
}

func reconcileVector(name, tests string, presented []any, kept kit.Obj, expect kit.Obj, acc, crd string) kit.Vector {
	return kit.Vector{Name: "receipts/" + name, Tests: tests, Operation: "reconcile", Members: kit.Obj{
		"reference": kit.Mediator.Ref, "accord": acc, "card": crd,
		"parties":  kit.Obj{"proposer": kit.Proposer.Ref, "applier": kit.Applier.Ref, "decider": kit.Decider.Ref},
		"receipts": presented, "kept": kept, "expect": expect,
	}}
}

func keptOf(envs []any, delivered ...kit.Obj) kit.Obj {
	if envs == nil {
		envs = []any{}
	}
	return kit.Obj{"envelopes": envs, "delivered": list(delivered...)}
}

func kenv(seq int64, hash string) kit.Obj { return kit.Obj{"seq": seq, "hash": hash} }

func lists(removed, unrecorded, missing []any) kit.Obj {
	for _, l := range []*[]any{&removed, &unrecorded, &missing} {
		if *l == nil {
			*l = []any{}
		}
	}
	return kit.Obj{"removed": removed, "unrecorded": unrecorded, "missing": missing}
}

func item(actor, id string) kit.Obj { return kit.Obj{"actor": actor, "receiptId": id} }

func reconcile() []kit.Vector {
	var vs []kit.Vector
	j := newJournal(askAndLog)
	e1, e2 := env(kit.Proposer, 1, "r1"), env(kit.Proposer, 2, "r2")
	r1 := j.crossing(kit.Proposer, kit.Applier, part("note", "r1"), e1, "pass", "ok")
	r2 := j.crossing(kit.Proposer, kit.Applier, part("note", "r2"), e2, "pass", "ok")

	vs = append(vs,
		reconcileVector("a-removed-last-receipt", "receipt-7: a journal whose last pass to the applier was removed verifies, and the applier's kept receipt is removed",
			list(r1), kit.Obj{"applier": keptOf(nil, r1, r2)}, lists([]any{item("applier", idOf(r2))}, nil, nil), accord, card),
		reconcileVector("an-unfetched-delivery", "receipt-7: a pass to the applier it has not fetched is unrecorded for the applier",
			list(r1, r2), kit.Obj{"applier": keptOf(nil, r1)}, lists(nil, []any{item("applier", idOf(r2))}, nil), accord, card),
		reconcileVector("a-submission-with-no-receipt", "receipt-7: kept envelopes at seq 1 and twice at seq 2 against a journal echoing only seq 1 list seq 2 as missing, once",
			list(r1), kit.Obj{"proposer": keptOf([]any{kenv(1, e1.hash), kenv(2, e2.hash), kenv(2, e2.hash)})},
			lists(nil, nil, []any{kit.Obj{"actor": "proposer", "seq": 2}}), accord, card),
		reconcileVector("an-echo-matches-seq-and-hash", "receipt-7: a kept envelope whose seq is echoed with another hash is missing",
			list(r1, r2), kit.Obj{"proposer": keptOf([]any{kenv(1, e2.hash), kenv(2, e2.hash)})},
			lists(nil, nil, []any{kit.Obj{"actor": "proposer", "seq": 1}}), accord, card),
		reconcileVector("everything-matches", "receipt-7: records that match the journal list nothing",
			list(r1, r2), kit.Obj{"proposer": keptOf([]any{kenv(1, e1.hash), kenv(2, e2.hash)}), "applier": keptOf(nil, r1, r2)},
			lists(nil, nil, nil), accord, card),
	)

	// A pass to the applier from another Accord under the same mediator.
	other := newJournal(compose)
	foreign := other.crossing(kit.Proposer, kit.Applier, part("brief", "foreign"), env(kit.Proposer, 1, "foreign"), "pass", "ok")
	blocked := newJournal(askAndLog).crossing(kit.Proposer, kit.Applier, part("tests", "blocked"), env(kit.Proposer, 1, "blocked"), "block", "no-match")
	vs = append(vs,
		reconcileVector("a-kept-receipt-from-another-accord", "receipt-7: a kept pass to the applier from another Accord fails kept, naming the applier and its index",
			list(r1), kit.Obj{"applier": keptOf(nil, r1, foreign)}, kit.Obj{"code": "kept", "actor": "applier", "index": 1}, accord, card),
		reconcileVector("a-kept-receipt-to-another-party", "section 9 row 3: a kept receipt addressed to another party fails kept",
			list(r1), kit.Obj{"proposer": keptOf(nil, r1)}, kit.Obj{"code": "kept", "actor": "proposer", "index": 0}, accord, card),
		reconcileVector("a-kept-receipt-that-blocked", "section 9 row 3: a kept receipt whose verdict is not pass fails kept",
			list(r1), kit.Obj{"applier": keptOf(nil, blocked)}, kit.Obj{"code": "kept", "actor": "applier", "index": 0}, accord, card),
		reconcileVector("a-presented-receipt-from-another-accord", "section 9 row 2: a presented receipt of another Accord fails presented at its index",
			list(foreign), kit.Obj{"applier": keptOf(nil)}, kit.Obj{"code": "presented", "index": 0}, accord, card),
		reconcileVector("a-presented-list-that-fails", "section 9 row 1: a presented list that fails a row of section 8 fails with that row's code and index",
			list(r2), kit.Obj{"applier": keptOf(nil)}, kit.Obj{"code": "chain", "index": 0}, accord, card),
		// Two rows of section 9 failing at once: the earlier decides. A presented list that fails a row
		// of section 8 comes before a presented receipt of another Accord and before a kept receipt that
		// fails, and a presented receipt of another Accord before a kept receipt that fails.
		reconcileVector("a-failing-list-and-another-accord", "section 9 rows 1 and 2: a list whose second receipt breaks its chain and whose first is of another Accord fails chain at 1",
			list(foreign, r2), kit.Obj{"applier": keptOf(nil)}, kit.Obj{"code": "chain", "index": 1}, accord, card),
		reconcileVector("a-failing-list-and-a-failing-kept-receipt", "section 9 rows 1 and 3: a list that fails chain comes before a kept receipt that blocked",
			list(r2), kit.Obj{"applier": keptOf(nil, blocked)}, kit.Obj{"code": "chain", "index": 0}, accord, card),
		reconcileVector("another-accord-and-a-failing-kept-receipt", "section 9 rows 2 and 3: a presented receipt of another Accord comes before a kept receipt that blocked",
			list(foreign), kit.Obj{"applier": keptOf(nil, blocked)}, kit.Obj{"code": "presented", "index": 0}, accord, card),
		// Parties are read in actor-reference order: the proposer's reference sorts before the
		// applier's, though its name sorts after.
		reconcileVector("kept-receipts-that-fail-for-two-parties", "section 9 row 3: when two parties' kept receipts fail, the party first in actor-reference order is named",
			list(r1), kit.Obj{"applier": keptOf(nil, blocked), "proposer": keptOf(nil, r1)}, kit.Obj{"code": "kept", "actor": "proposer", "index": 0}, accord, card),
		kit.Vector{Name: "receipts/a-reconciling-mediator-that-is-not-a-reference", Tests: "section 9 row 1: a mediator that is not a reference fails reference",
			Operation: "reconcile", Members: kit.Obj{
				"reference": "mediator", "accord": accord, "card": card, "parties": kit.Obj{"applier": kit.Applier.Ref},
				"receipts": list(r1), "kept": kit.Obj{"applier": keptOf(nil)}, "expect": kit.Obj{"code": "reference"},
			}},
	)

	// A change released to the applier: the release receipt is addressed to the applier and echoes the
	// proposer's envelope, so it counts as an echo for the proposer only.
	q := newJournal(askAndLog)
	qe := env(kit.Proposer, 1, "question")
	asked := part("proposal", "question")
	version := part("changed-version", "question-version")
	ask := q.crossing(kit.Proposer, kit.Applier, asked, qe, "escalate", "maybe")
	change := q.resolution(kit.Decider, kit.Proposer, version, qe, "pass", "ok")
	release := q.resolution(kit.Decider, kit.Applier, version, qe, "pass", "ok")
	vs = append(vs, reconcileVector("a-release-counts-as-an-echo-for-the-asker-only",
		"maybe-10, receipt-7: a change's release to the applier echoes the proposer's envelope and counts for the proposer only, never the applier",
		list(ask, change, release),
		kit.Obj{
			"proposer": keptOf([]any{kenv(1, qe.hash)}, change),
			"applier":  keptOf([]any{kenv(1, qe.hash)}, release),
		},
		lists(nil, nil, []any{kit.Obj{"actor": "applier", "seq": 1}}), accord, card))
	// The same question, with the decider, who answered and released it, keeping the question's
	// envelope too: the change and its release are both from the decider, and neither counts as an
	// echo for it. Missing lists parties in actor-reference order: the decider's reference sorts
	// before the applier's.
	vs = append(vs, reconcileVector("a-resolution-counts-as-an-echo-for-the-asker-only",
		"receipt-7, crossing-5: a change and its release, both from the decider, echo the proposer's envelope and count for the proposer only, never for the decider that answered or the applier",
		list(ask, change, release),
		kit.Obj{
			"proposer": keptOf([]any{kenv(1, qe.hash)}, change),
			"applier":  keptOf([]any{kenv(1, qe.hash)}, release),
			"decider":  keptOf([]any{kenv(1, qe.hash)}),
		},
		lists(nil, nil, []any{kit.Obj{"actor": "decider", "seq": 1}, kit.Obj{"actor": "applier", "seq": 1}}), accord, card))
	return vs
}
