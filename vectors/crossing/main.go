// Command crossing writes the vectors of the crossing group: sections 4 and 5 of the protocol, giving
// and getting a part at the door, and composition's verdict. Every rejection code, verdict and reason
// is written here by hand from the door's and composition's check tables; the generator builds and
// signs the messages, envelopes and receipts with the Go standard library alone (vectors/README.md).
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
)

// at returns the time of the scenario's n-th step: one minute apart from 09:00.
func at(n int) string {
	return "2026-11-12T09:" + string(rune('0'+n/10)) + string(rune('0'+n%10)) + ":00Z"
}

func scene(set kit.Obj) *kit.Scene {
	return kit.NewScene(set, kit.Mediator, kit.Proposer, kit.Applier, kit.Decider, kit.Stranger)
}

func note(id, text string) kit.Obj {
	return kit.Message(id, kit.HashOf(askAndLog), kit.Applier, kit.TextPart("note", kit.Salt("crossing "+id, 32), text))
}

func of(set kit.Obj, id, kind string) kit.Obj {
	return kit.Message(id, kit.HashOf(set), kit.Applier, kit.TextPart(kind, kit.Salt("crossing "+id, 32), "The "+kind+" "+id+"."))
}

// signedBy returns the message and its envelope at seq, signed by the key, as JSON values.
func signedBy(m kit.Obj, seq int64, k kit.Key) (kit.Obj, kit.Obj) {
	return kit.Obj{"message": m}, kit.Obj{"envelope": kit.Envelope(seq, kit.Canon(m), k)}
}

// text gives message bytes as text, with the envelope over the bytes given as the hashed message.
func text(message string, hashed []byte, seq int64, k kit.Key) (kit.Obj, kit.Obj) {
	return kit.Obj{"messageText": message}, kit.Obj{"envelope": kit.Envelope(seq, hashed, k)}
}

func main() {
	var vs []kit.Vector
	add := func(name, tests string, s *kit.Scene, set kit.Obj) {
		vs = append(vs, s.Vector("crossing/"+name, tests, set))
	}

	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, note("note-1", "A note."), kit.Applier, "pass", "ok")
		add("a-give-then-the-end-state", "conformance-vectors-3: one give that passes; final holds its receipt and its delivery, and nothing is pending", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, note("note-1", "A note."), kit.Applier, "pass", "ok")
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		add("a-delivery-is-fetched-once", "crossing-3: the receiver's first fetch returns the delivery and its second returns nothing", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, of(askAndLog, "tests-1", "tests"), kit.Applier, "block", "no-match")
		s.Get(kit.Applier)
		add("no-delivery-for-a-block", "crossing-3, composition-2: a kind no term names is block, no-match, and the receiver's fetch returns nothing", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := kit.Message("stranger-1", kit.HashOf(askAndLog), kit.Applier, kit.TextPart("note", kit.Salt("stranger", 32), "From nobody."))
		msg, env := signedBy(m, 1, kit.Stranger)
		s.Reject(at(0), kit.Stranger, "not-a-member", msg, env)
		s.GetRejected(kit.Stranger, "not-a-member")
		add("a-stranger", "setup-contract-6: a caller with no declaration is not-a-member when it gives and when it fetches", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := kit.Message("mediator-1", kit.HashOf(askAndLog), kit.Applier, kit.TextPart("note", kit.Salt("mediator", 32), "From the mediator."))
		msg, env := signedBy(m, 1, kit.Mediator)
		s.Reject(at(0), kit.Mediator, "not-a-member", msg, env)
		s.GetRejected(kit.Mediator, "not-a-member")
		add("the-mediators-own-key", "setup-contract-6: the mediator's key, which has no declaration, is not-a-member", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("other-accord", "Elsewhere.")
		kit.Ext(m)["accord"] = kit.HashOf(compose)
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "accord-mismatch", msg, env)
		add("a-message-from-another-accord", "crossing-1: a correctly signed message naming another Accord's id is accord-mismatch", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("unknown", "To nobody.")
		kit.Ext(m)["to"] = kit.Stranger.Ref
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "receiver-unknown", msg, env)
		add("an-unknown-receiver", "crossing-1: a to naming a key with no declaration is receiver-unknown", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("context", "With a context.")
		m["contextId"] = "c-1"
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "extension-missing", msg, env)
		for i, from := range []kit.Key{kit.Proposer, kit.Applier} {
			m := note("from-"+from.Name, "Claiming a sender.")
			kit.Ext(m)["from"] = from.Ref
			msg, env := signedBy(m, 1, kit.Proposer)
			s.Reject(at(1+i), kit.Proposer, "extension-missing", msg, env)
		}
		s.Give(at(3), kit.Proposer, 1, note("note-1", "A note."), kit.Applier, "pass", "ok")
		add("a-member-the-frame-does-not-list", "crossing-1, crossing-6: a contextId, and a from naming the caller or another member, are extension-missing", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		good := note("gate-1", "Gated.")
		kit.Ext(good)["gate"] = "out_you"
		s.Give(at(0), kit.Proposer, 1, good, kit.Applier, "pass", "ok")
		bad := note("gate-2", "Gated badly.")
		kit.Ext(bad)["gate"] = "Out_You"
		msg, env := signedBy(bad, 2, kit.Proposer)
		s.Reject(at(1), kit.Proposer, "extension-missing", msg, env)
		add("a-reserved-gate", "crossing-1: a gate out_you is read for shape only and decided as without it; Out_You is extension-missing", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		cases := []func(p kit.Obj){
			func(p kit.Obj) { p["data"] = kit.Obj{"also": "data"} },
			func(p kit.Obj) { delete(p, "text") },
			func(p kit.Obj) { delete(p, "text"); p["raw"] = "not base64!" },
			func(p kit.Obj) { delete(p, "text"); p["raw"] = "YR==" },
			func(p kit.Obj) { delete(p, "text"); p["data"] = nil },
			func(p kit.Obj) { p["text"] = 7 },
			func(p kit.Obj) { p["kind"] = "note" },
		}
		for i, change := range cases {
			m := note("body-"+string(rune('a'+i)), "A body.")
			change(kit.PartOf(m))
			msg, env := signedBy(m, 1, kit.Proposer)
			s.Reject(at(i), kit.Proposer, "one-body", msg, env)
		}
		add("one-body", "crossing-2: two bodies, none, a raw not in standard base64 or not in its one encoding, a null data, a text not a string and an unlisted member are one-body", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("url", "Unused.")
		p := kit.PartOf(m)
		delete(p, "text")
		p["url"] = "https://example.com/a"
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "url-unsupported", msg, env)
		m2 := note("url-2", "Unused.")
		kit.PartOf(m2)["url"] = 7
		msg, env = signedBy(m2, 1, kit.Proposer)
		s.Reject(at(1), kit.Proposer, "one-body", msg, env)
		add("a-url-body", "crossing-2: a part whose one body is url is url-unsupported; a url beside a text is two bodies, one-body", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		cases := []func(meta kit.Obj){
			func(meta kit.Obj) { meta["salt"] = kit.Salt("short", 31) },
			func(meta kit.Obj) { meta["salt"] = kit.Salt("long", 33) },
			func(meta kit.Obj) { meta["salt"] = kit.Salt("sixteen", 16) },
			func(meta kit.Obj) { meta["salt"] = strings.ToUpper(kit.Salt("upper", 32)) },
			func(meta kit.Obj) { delete(meta, "kind") },
			func(meta kit.Obj) { meta["kind"] = "Note" },
			func(meta kit.Obj) { meta["topic"] = "x" },
		}
		for i, change := range cases {
			m := note("meta-"+string(rune('a'+i)), "A part.")
			change(kit.PartOf(m)["metadata"].(kit.Obj)[kit.Extension].(kit.Obj))
			msg, env := signedBy(m, 1, kit.Proposer)
			s.Reject(at(i), kit.Proposer, "part-metadata", msg, env)
		}
		m := note("meta-none", "A part.")
		delete(kit.PartOf(m), "metadata")
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(len(cases)), kit.Proposer, "part-metadata", msg, env)
		add("part-metadata", "crossing-2: a salt of 31, 33 or 16 bytes, an uppercase salt, a missing or malformed kind, an unlisted member and no metadata are part-metadata", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("edited", "As signed.")
		env := kit.Envelope(1, kit.Canon(m), kit.Proposer)
		kit.PartOf(m)["text"] = "As signeD."
		s.Reject(at(0), kit.Proposer, "envelope-hash", kit.Obj{"message": m}, kit.Obj{"envelope": env})
		add("an-edited-message", "crossing-4: a message changed by one byte after its envelope was signed is envelope-hash", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, note("note-1", "First."), kit.Applier, "pass", "ok")
		msg, env := signedBy(note("note-2", "Again."), 1, kit.Proposer)
		s.Reject(at(1), kit.Proposer, "envelope-sequence", msg, env)
		msg, env = signedBy(note("note-3", "Zero."), 0, kit.Proposer)
		s.Reject(at(2), kit.Proposer, "envelope-sequence", msg, env)
		add("a-repeated-sequence-number", "crossing-4: a give at seq 1 after seq 1 was accepted, and one at seq 0, are envelope-sequence", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, note("note-1", "First."), kit.Applier, "pass", "ok")
		s.Give(at(1), kit.Proposer, 3, note("note-3", "Third."), kit.Applier, "pass", "ok")
		add("a-skipped-sequence-number", "crossing-4: after seq 1, a give at seq 3 is decided and its receipt echoes seq 3", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		good := kit.Canon(note("dup", "Twice."))
		dup := strings.Replace(string(good), `"messageId":"dup"`, `"messageId":"dup","messageId":"dup"`, 1)
		msg, env := text(dup, good, 1, kit.Applier)
		s.Reject(at(0), kit.Proposer, "envelope-signature", msg, env)
		add("a-repeated-member-behind-a-bad-signature", "crossing-6: a message repeating a member name under an envelope that does not verify is envelope-signature", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("topic", "With a topic.")
		kit.Ext(m)["topic"] = "the-topic"
		good := kit.Canon(m)
		dup := strings.Replace(string(good), `"topic":"the-topic"`, `"topic":"wrong","topic":"the-topic"`, 1)
		msg, env := text(dup, good, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "not-canonical", msg, env)
		add("a-repeated-member-under-a-good-signature", "crossing-6: a message with topic twice, the second copy correct, under an envelope that verifies is not-canonical", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("seq-twice", "Once.")
		env := kit.Envelope(1, kit.Canon(m), kit.Proposer)
		e := string(kit.Canon(env))
		e = strings.Replace(e, `"seq":1`, `"seq":1,"seq":1`, 1)
		s.Reject(at(0), kit.Proposer, "envelope-signature", kit.Obj{"message": m}, kit.Obj{"envelopeText": e})
		add("a-repeated-seq-in-the-envelope", "crossing-6: envelope bytes with seq twice are envelope-signature, not not-canonical", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("two-defects", "Elsewhere, twice.")
		kit.Ext(m)["accord"] = kit.HashOf(compose)
		m["parts"] = append(m["parts"].([]any), kit.TextPart("note", kit.Salt("second", 32), "Second."))
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "accord-mismatch", msg, env)
		add("two-content-defects", "crossing-6: a message naming another Accord with two parts is accord-mismatch, the earlier row", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		signed := note("signed", "Signed for.")
		other := note("other", "Not signed for.")
		kit.Ext(other)["accord"] = kit.HashOf(compose)
		env := kit.Envelope(1, kit.Canon(signed), kit.Proposer)
		s.Reject(at(0), kit.Proposer, "envelope-hash", kit.Obj{"message": other}, kit.Obj{"envelope": env})
		add("a-message-not-the-one-signed-for", "crossing-6: another message under an envelope signed over one is envelope-hash, and its content is not read", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		defects := []struct {
			code   string
			change func(m kit.Obj)
		}{
			{"extension-missing", func(m kit.Obj) { m["role"] = "ROLE_MEDIATOR" }},
			{"receiver-unknown", func(m kit.Obj) { kit.Ext(m)["to"] = kit.Stranger.Ref }},
			{"accord-mismatch", func(m kit.Obj) { kit.Ext(m)["accord"] = kit.HashOf(compose) }},
			{"one-part", func(m kit.Obj) { m["parts"] = []any{} }},
			{"one-body", func(m kit.Obj) { delete(kit.PartOf(m), "text") }},
			{"part-metadata", func(m kit.Obj) {
				kit.PartOf(m)["metadata"].(kit.Obj)[kit.Extension].(kit.Obj)["salt"] = kit.Salt("six", 8)
			}},
		}
		for i, d := range defects {
			m := note("defect-"+string(rune('a'+i)), "Defective.")
			d.change(m)
			msg, env := signedBy(m, 1, kit.Proposer)
			s.Reject(at(i), kit.Proposer, d.code, msg, env)
		}
		s.Give(at(6), kit.Proposer, 1, note("note-1", "Good."), kit.Applier, "pass", "ok")
		add("six-rejections-then-a-pass", "crossing-7: six gives at seq 1 with defects of rows 6 to 12 change nothing; the seventh at seq 1 is the first of its pair", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Reject(at(0), kit.Stranger, "not-a-member", kit.Obj{"messageHex": kit.HexOf([]byte("not json {"))},
			kit.Obj{"envelopeHex": kit.HexOf([]byte{0xff, 0x00})})
		add("a-strangers-bytes-are-not-read", "crossing-7: a stranger's give whose bytes are not JSON is not-a-member, not not-canonical", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		msg, env := signedBy(note("by-another", "Signed by another."), 1, kit.Applier)
		s.Reject(at(0), kit.Proposer, "envelope-signature", msg, env)
		add("an-envelope-by-another-key", "signing-2, crossing-6: an envelope signed by another member's key is envelope-signature under the caller's", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		raw := kit.Message("raw-1", kit.HashOf(askAndLog), kit.Applier, kit.RawPart("note", kit.Salt("raw", 32), []byte{0x00, 0xff, 0x10, 'n'}))
		data := kit.Message("data-1", kit.HashOf(askAndLog), kit.Applier,
			kit.DataPart("note", kit.Salt("data", 32), kit.Obj{"z": []any{1, "two", true}, "a": kit.Obj{}}))
		re := note("re-1", "In reply.")
		kit.Ext(re)["re"] = kit.HashOf("an earlier receipt")
		kit.Ext(re)["topic"] = "replies"
		s.Give(at(0), kit.Proposer, 1, raw, kit.Applier, "pass", "ok")
		s.Give(at(1), kit.Proposer, 2, data, kit.Applier, "pass", "ok")
		s.Give(at(2), kit.Proposer, 3, re, kit.Applier, "pass", "ok")
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		add("bodies-roles-and-replies", "crossing-1, crossing-2, crossing-3: raw and data bodies, each with a 32-byte salt, and a re with a topic cross; a delivery carries re and no topic", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		agent := note("agent-1", "In the agent's role.")
		agent["role"] = "ROLE_AGENT"
		msg, env := signedBy(agent, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "extension-missing", msg, env)
		s.Give(at(1), kit.Proposer, 1, note("agent-1", "In the agent's role."), kit.Applier, "pass", "ok")
		add("a-give-in-the-agents-role", "crossing-1: a member's give with role ROLE_AGENT is extension-missing; the same give with ROLE_USER passes", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		m := note("old-1", "Under the old host.")
		const old = "https://placket.dev/crossing/0.1"
		m["extensions"] = []any{old}
		for _, meta := range []kit.Obj{m["metadata"].(kit.Obj), kit.PartOf(m)["metadata"].(kit.Obj)} {
			meta[old] = meta[kit.Extension]
			delete(meta, kit.Extension)
		}
		msg, env := signedBy(m, 1, kit.Proposer)
		s.Reject(at(0), kit.Proposer, "extension-missing", msg, env)
		add("the-old-crossing-extension", "crossing-1: a give whose extension and metadata are under the crossing extension's URI on another host is extension-missing, and nothing is written", s, askAndLog)
	}
	{
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, of(askAndLog, "proposal-1", "proposal"), kit.Applier, "escalate", "maybe")
		add("the-appliers-get-term", "composition-1: the applier's get term names role:proposer, which the proposer's binding gives, so it matches", s, askAndLog)
	}
	{
		// Each give fails two neighbouring rows of the door's table, and the earlier row decides; none
		// changes anything, so the last give at seq 2 is the second of the pair (crossing-6).
		s := scene(askAndLog)
		s.Give(at(0), kit.Proposer, 1, note("note-1", "First."), kit.Applier, "pass", "ok")
		steps := []struct {
			code   string
			seq    int64
			edit   bool // the message changes after its envelope is signed
			change func(m kit.Obj)
		}{
			// Rows 4 and 5: a message changed after signing, at a seq already accepted.
			{"envelope-hash", 1, true, nil},
			// Rows 5 and 6: a seq already accepted, and a member the frame does not list.
			{"envelope-sequence", 1, false, func(m kit.Obj) { m["contextId"] = "c-1" }},
			// Rows 6 and 7: an re, a topic or a gate of the wrong form, and a receiver who is no member.
			{"extension-missing", 2, false, func(m kit.Obj) { kit.Ext(m)["re"] = "receipt-1"; kit.Ext(m)["to"] = kit.Stranger.Ref }},
			{"extension-missing", 2, false, func(m kit.Obj) { kit.Ext(m)["topic"] = ""; kit.Ext(m)["to"] = kit.Stranger.Ref }},
			{"extension-missing", 2, false, func(m kit.Obj) { kit.Ext(m)["gate"] = "Out_You"; kit.Ext(m)["to"] = kit.Stranger.Ref }},
			// Rows 7 and 8: a receiver who is no member, and another Accord.
			{"receiver-unknown", 2, false, func(m kit.Obj) { kit.Ext(m)["to"] = kit.Stranger.Ref; kit.Ext(m)["accord"] = kit.HashOf(compose) }},
			// Rows 9 and 10: two parts, the first with no body.
			{"one-part", 2, false, func(m kit.Obj) {
				delete(kit.PartOf(m), "text")
				m["parts"] = append(m["parts"].([]any), kit.TextPart("note", kit.Salt("second", 32), "Second."))
			}},
			// Rows 10 and 11: a url body beside a member the part does not list.
			{"one-body", 2, false, func(m kit.Obj) {
				p := kit.PartOf(m)
				delete(p, "text")
				p["url"], p["note"] = "https://example.com/a", "x"
			}},
			// Rows 11 and 12: a url body with a salt too short, and a url that is no string with no
			// metadata at all.
			{"url-unsupported", 2, false, func(m kit.Obj) {
				p := kit.PartOf(m)
				delete(p, "text")
				p["url"] = "https://example.com/a"
				p["metadata"].(kit.Obj)[kit.Extension].(kit.Obj)["salt"] = kit.Salt("short", 8)
			}},
			{"url-unsupported", 2, false, func(m kit.Obj) {
				p := kit.PartOf(m)
				delete(p, "text")
				delete(p, "metadata")
				p["url"] = 7
			}},
		}
		for i, st := range steps {
			m := note("pair-"+string(rune('a'+i)), "Two defects.")
			if st.change != nil {
				st.change(m)
			}
			env := kit.Envelope(st.seq, kit.Canon(m), kit.Proposer)
			if st.edit {
				kit.PartOf(m)["text"] = "Two defectS."
			}
			s.Reject(at(1+i), kit.Proposer, st.code, kit.Obj{"message": m}, kit.Obj{"envelope": env})
		}
		s.Give(at(1+len(steps)), kit.Proposer, 2, note("note-2", "Second."), kit.Applier, "pass", "ok")
		add("two-defects-at-the-door", "crossing-6: a give failing rows 4 and 5, 5 and 6, 6 and 7 by its re, topic or gate, 7 and 8, 9 and 10, 10 and 11, or 11 and 12 is rejected with the earlier row's code, and none changes the next seq", s, askAndLog)
	}
	{
		own := kit.OwnNo().Build()
		s := kit.NewScene(own, kit.Mediator, kit.Proposer, kit.Applier)
		s.Give(at(0), kit.Proposer, 1, of(own, "draft-1", "draft"), kit.Applier, "block", "no-match")
		s.Get(kit.Applier)
		add("the-senders-own-no", "composition-3: the sender's own matching no give term beside its matching want, with the receiver's want, is block, no-match", s, own)
	}
	compo := []struct{ name, tests, kind, verdict, reason string }{
		{"a-role-the-binding-does-not-give", "composition-1: a get term naming role:auditor does not match a sender the binding does not make an auditor", "audit", "block", "no-match"},
		{"no-get-term", "composition-2: a sender's matching give term and no get term of the receiver is block, no-match", "sketch", "block", "no-match"},
		{"no-beside-want", "composition-3: a matching no term beside a matching want term is block, no-match", "draft", "block", "no-match"},
		{"maybe-rules-that-agree", "composition-5: maybe rules naming one set of roles in two orders, with one timeout, are escalate, maybe", "memo", "escalate", "maybe"},
		{"maybe-rules-that-differ", "composition-5: maybe rules with timeouts 60 and 120 are block, maybe, and no question opens", "plan", "block", "maybe"},
		{"need-meets-want", "composition-6: a need give term and a want get term compose as two wants, pass, ok", "brief", "pass", "ok"},
	}
	for _, c := range compo {
		s := scene(compose)
		s.Give(at(0), kit.Proposer, 1, of(compose, c.kind+"-1", c.kind), kit.Applier, c.verdict, c.reason)
		add(c.name, c.tests, s, compose)
	}
	{
		// The proposer gets answers from the decider under a maybe term, so the decider's accept, a
		// part the mediator formed, meets a maybe term: block, maybe, and no second question opens.
		set := kit.AnswerUnderMaybe().Build()
		s := scene(set)
		q := s.Give(at(0), kit.Proposer, 1, of(set, "proposal-1", "proposal"), kit.Applier, "escalate", "maybe")
		s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: kit.Salt("crossing answer-1", 32),
			By: kit.Decider, Verdict: kit.Verdict{Verdict: "block", Reason: "maybe"}})
		s.Get(kit.Proposer)
		add("an-answer-that-meets-a-maybe-term", "composition-5: an accept decided from the decider to a proposer that gets answers under a maybe term is block, maybe; nothing is queued and no question opens", s, set)
	}
	// Judges (composition-7), under the judged Accord. Each give hands the decision jev's judgment; it
	// decides only where both sides' every matching term is a maybe listing jev to decide.
	judged := kit.Judged().Build()
	judge := func(name, tests, kind string, judgment kit.Obj, verdict, reason string, by any) {
		s := scene(judged)
		s.GiveJudged(at(0), kit.Proposer, 1, of(judged, kind+"-1", kind), kit.Applier, judgment, verdict, reason, by)
		s.Get(kit.Applier)
		add(name, tests, s, judged)
	}
	judge("judge-pass", "composition-7, receipt-11, conformance-vectors-3: both sides list jev to decide, and its pass is pass, ok, by the judge jev; the part is queued", "report",
		kit.Judgment("jev", "pass"), "pass", "ok", kit.ByJudge("jev"))
	judge("judge-block", "composition-7, receipt-4, receipt-11: both sides list jev to decide, and its block is block, judge:jev, by the judge jev; nothing is queued", "report",
		kit.Judgment("jev", "block"), "block", "judge:jev", kit.ByJudge("jev"))
	judge("judge-one-side", "composition-7: only the sender's maybe term lists jev, so its pass is no judgment and the part is asked, by the terms", "review",
		kit.Judgment("jev", "pass"), "escalate", "maybe", kit.ByTerms)
	judge("judges-differ", "composition-5, composition-7: the two sides list jev with uses that differ, so the part is asked and no judge is consulted", "survey",
		kit.Judgment("jev", "pass"), "escalate", "maybe", kit.ByTerms)
	judge("judge-advise", "composition-7: both sides list jev to advise, so its judgment decides nothing and the part is asked", "digest",
		kit.Judgment("jev", "block"), "escalate", "maybe", kit.ByTerms)
	judge("judge-beside-a-want", "composition-7: the sender's want term matches beside its maybe term, so not every matching term is a maybe and the part is asked", "ledger",
		kit.Judgment("jev", "pass"), "escalate", "maybe", kit.ByTerms)
	{
		// A judged release (maybe-10): the asked report is accepted with release, decided again with no
		// judgment, and passes by the answer, never by the judge.
		s := scene(judged)
		q := s.GiveJudged(at(0), kit.Proposer, 1, of(judged, "report-2", "report"), kit.Applier, nil, "escalate", "maybe", kit.ByTerms)
		pass := kit.Verdict{Verdict: "pass", Reason: "ok"}
		s.Resolve(kit.Answer{At: at(1), Question: q, Action: "accepted", Text: "accepted", Salt: kit.Salt("crossing judged answer", 32),
			By: kit.Decider, Verdict: pass, Release: true, Released: &pass})
		s.Get(kit.Applier)
		add("a-judge-never-acts-at-release", "maybe-10, composition-7: a report asked with no judgment and accepted with release is decided again with none, so its release is pass, ok, by the answer", s, judged)
	}
	{
		// A part's media type (crossing-2, crossing-3).
		s := scene(askAndLog)
		typed := note("typed-1", `{"total":1250}`)
		kit.PartOf(typed)["mediaType"] = "application/json"
		s.Give(at(0), kit.Proposer, 1, typed, kit.Applier, "pass", "ok")
		for i, mt := range []any{"Application/JSON", "json", "text/plain; charset=utf-8", 7} {
			m := note("typed-bad-"+string(rune('a'+i)), "A note.")
			kit.PartOf(m)["mediaType"] = mt
			msg, env := signedBy(m, 2, kit.Proposer)
			s.Reject(at(1+i), kit.Proposer, "one-body", msg, env)
		}
		s.Get(kit.Applier)
		add("a-part-with-a-media-type", "crossing-2, crossing-3: a part with mediaType application/json passes and its delivery carries it; Application/JSON, json, a type with a parameter, and a number are each one-body", s, askAndLog)
	}
	{
		// The digest names bytes, not form (crossing-3): a text, a raw and a data body of the same
		// bytes, under one salt, name one digest.
		s := scene(askAndLog)
		same := kit.Salt("crossing same-bytes", 32)
		body := `{"total":1250}`
		for i, p := range []kit.Obj{
			kit.TextPart("note", same, body),
			kit.RawPart("note", same, []byte(body)),
			kit.DataPart("note", same, kit.Obj{"total": int64(1250)}),
		} {
			m := kit.Message("same-"+string(rune('a'+i)), kit.HashOf(askAndLog), kit.Applier, p)
			s.Give(at(i), kit.Proposer, int64(1+i), m, kit.Applier, "pass", "ok")
		}
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		s.Get(kit.Applier)
		add("the-same-bytes-in-three-forms", "crossing-3: a text, a raw and a data body of the same bytes under one salt each pass, their receipts name one digest, and each delivery carries its body in the form it crossed", s, askAndLog)
	}
	kit.Main("crossing", vs)
}
