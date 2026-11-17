package export

import (
	"bytes"
	"testing"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// TestAnswerSignedFitsTheClosing gives a log of one question and its closing, decided by the answer
// call answers.jsonl holds, for each call in turn: answer_signed is yes only when the call is signed
// by the party its action is for and records what the closing does. An accept is the closing's
// from's, with the closing's digest; a decline is the closing's from's; a cancel is the asker's. A
// call signed by any other key, or of another action or digest, reads as not signed (log-1).
func TestAnswerSignedFitsTheClosing(t *testing.T) {
	t.Parallel()
	key := func(b byte) signing.Key {
		t.Helper()
		k, err := signing.NewKey(bytes.Repeat([]byte{b}, 32))
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	med, asker, receiver, answerer, stranger := key(1), key(2), key(3), key(4), key(5)
	accord, card := "sha256:"+hex64("a"), "sha256:"+hex64("c")
	env := receipts.Envelope{Sender: asker.Reference(), Seq: 1, Hash: "sha256:" + hex64("e")}
	part := receipts.Subject{Name: "proposal", Digest: hex64("d")}
	ask, askV, err := receipts.Build(part, receipts.Predicate{Accord: accord, Card: card, Mediator: med.Reference(),
		Kind: receipts.KindCrossing, From: asker.Reference(), To: receiver.Reference(), Seq: 1, At: "2026-11-12T09:00:00Z",
		Envelope: env, Verdict: "escalate", Reason: "maybe", By: receipts.By{Decider: receipts.ByTerms}}, med)
	if err != nil {
		t.Fatal(err)
	}
	question := askV.Statement().ID
	text := hex64("f")
	cases := []struct {
		name            string
		signer          signing.Key
		call            answer.Call
		verdict, reason string
		subject         receipts.Subject
		want            string
	}{
		{"an accept by the closing's from", answerer, answer.Call{Action: answer.Accepted, Digest: text, Release: true},
			"pass", "ok", receipts.Subject{Name: receipts.NameAnswer, Digest: text}, Signed},
		{"an accept by another key", stranger, answer.Call{Action: answer.Accepted, Digest: text, Release: true},
			"pass", "ok", receipts.Subject{Name: receipts.NameAnswer, Digest: text}, ""},
		{"an accept of another digest", answerer, answer.Call{Action: answer.Accepted, Digest: hex64("0"), Release: true},
			"pass", "ok", receipts.Subject{Name: receipts.NameAnswer, Digest: text}, ""},
		{"a change for an accept", answerer, answer.Call{Action: answer.Changed, Digest: text, Release: true},
			"pass", "ok", receipts.Subject{Name: receipts.NameAnswer, Digest: text}, ""},
		{"a decline by the closing's from", answerer, answer.Call{Action: answer.Declined},
			"block", "declined", part, Signed},
		{"a decline by another key", stranger, answer.Call{Action: answer.Declined},
			"block", "declined", part, ""},
		{"a cancel for a decline", asker, answer.Call{Action: answer.Cancelled},
			"block", "declined", part, ""},
		{"a cancel by the asker", asker, answer.Call{Action: answer.Cancelled},
			"block", "cancelled", part, Signed},
		{"a cancel by the closing's from", answerer, answer.Call{Action: answer.Cancelled},
			"block", "cancelled", part, ""},
	}
	for _, c := range cases {
		call := c.call
		call.Accord, call.Question, call.From = accord, question, c.signer.Reference()
		signed, err := answer.Sign(call, c.signer)
		if err != nil {
			t.Fatal(err)
		}
		closing, _, err := receipts.Build(c.subject, receipts.Predicate{Accord: accord, Card: card, Mediator: med.Reference(),
			Kind: receipts.KindResolution, From: answerer.Reference(), To: asker.Reference(), Seq: 1, At: "2026-11-12T09:10:00Z",
			Envelope: env, Verdict: c.verdict, Reason: c.reason, By: receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(signed)}}, med)
		if err != nil {
			t.Fatal(err)
		}
		lg, err := Build(med.Reference(), [][]byte{ask, closing}, nil, [][]byte{signed}, func(string) {})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(lg.Rows) != 1 || lg.Rows[0].AnswerSigned != c.want {
			t.Fatalf("%s: rows %+v", c.name, lg.Rows)
		}
	}
}

// hex64 is 64 hex digits, the one digit repeated.
func hex64(d string) string { return string(bytes.Repeat([]byte(d), 64)) }
