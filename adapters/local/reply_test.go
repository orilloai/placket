package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
)

// replyOf is the canonical bytes of a reply written as a map.
func replyOf(t testing.TB, o map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestParseReplyReadsOnlyWhatAWindowSends reads replies a window does not send. What a reply names
// reaches a person's terminal, so a refusal's code is a label, an outcome's receipt id, verdict and
// reason are those of its receipt, and an open question's id is a typed hash, its asker an actor
// reference, its kind a label, its form one of the three with that one body, its receipt an object
// and its salt hex. Each is refused, and the reply a window sends is read as sent.
func TestParseReplyReadsOnlyWhatAWindowSends(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	rep := w.give(t, "proposer", "applier", "m1", "note", "a note")
	o := rep.Outcome
	if o == nil {
		t.Fatalf("give: %+v", rep)
	}
	asker := key(t, "proposer").Reference()
	outcome := func(change func(map[string]any)) map[string]any {
		m := map[string]any{"verdict": o.Verdict, "reason": o.Reason, "receiptId": o.ReceiptID, "receipt": json.RawMessage(o.Receipt)}
		if change != nil {
			change(m)
		}
		return m
	}
	question := func(change func(map[string]any)) map[string]any {
		m := map[string]any{"id": o.ReceiptID, "asker": asker, "kind": "proposal", "deadline": "2026-11-12T10:00:00Z", "form": "text",
			"text": "a\x1b]52;c;aGk=\x07b", "size": 9, "salt": strings.Repeat("ab", 32), "receipt": json.RawMessage(o.Receipt)}
		if change != nil {
			change(m)
		}
		return map[string]any{"ok": true, "questions": []any{m}}
	}
	other := "sha256:" + strings.Repeat("0", 64)

	if got, ok := local.ParseReply(replyOf(t, map[string]any{"ok": true, "outcome": outcome(nil)})); !ok ||
		got.Outcome.ReceiptID != o.ReceiptID || got.Outcome.Reason != o.Reason || got.Outcome.Verdict != o.Verdict || string(got.Outcome.Receipt) != string(o.Receipt) {
		t.Fatalf("the window's own reply: %v", ok)
	}
	if got, ok := local.ParseReply(replyOf(t, map[string]any{"ok": true, "outcome": outcome(nil), "release": outcome(nil)})); !ok || got.Release == nil {
		t.Fatalf("an outcome and a release: %v", ok)
	}
	if got, ok := local.ParseReply(replyOf(t, question(nil))); !ok || len(got.Questions) != 1 || *got.Questions[0].Message != "a\x1b]52;c;aGk=\x07b" {
		t.Fatalf("a question whose part holds a control sequence: %v", ok)
	}
	for _, code := range []string{"not-a-member", "stale", "journal-locked"} {
		for _, by := range []string{"rejection", "error", "door"} {
			if _, ok := local.ParseReply(replyOf(t, map[string]any{"ok": false, by: code})); !ok {
				t.Fatalf("%s %s: refused", by, code)
			}
		}
	}

	cases := []struct {
		name  string
		reply map[string]any
	}{
		{"a rejection holding an OSC 52 sequence", map[string]any{"ok": false, "rejection": "\x1b]52;c;aGk=\x07"}},
		{"an error holding a space", map[string]any{"ok": false, "error": "journal locked"}},
		{"an error holding a line feed", map[string]any{"ok": false, "error": "journal\nok"}},
		{"a door code holding a C1 control", map[string]any{"ok": false, "door": "stale\u009b2J"}},
		{"an empty door code", map[string]any{"ok": false, "door": ""}},
		{"a reason holding an escape", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["reason"] = "ok\x1b[2J" })}},
		{"a reason not its receipt's", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["reason"] = "no-match" })}},
		{"a verdict not its receipt's", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["verdict"] = "block" })}},
		{"a verdict holding an escape", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["verdict"] = "pass\x1b[2J" })}},
		{"a receipt id not its receipt's", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["receiptId"] = other })}},
		{"a receipt id that is a path", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["receiptId"] = "sha256:../../x" })}},
		{"a receipt that is no receipt", map[string]any{"ok": true, "outcome": outcome(func(m map[string]any) { m["receipt"] = map[string]any{"a": 1} })}},
		{"a release whose reason holds an escape", map[string]any{"ok": true, "outcome": outcome(nil),
			"release": outcome(func(m map[string]any) { m["reason"] = "maybe\x1b]52;c;aGk=\x07" })}},
		{"a question whose id is no typed hash", question(func(m map[string]any) { m["id"] = "q\x1b[2J" })},
		{"a question whose asker is no reference", question(func(m map[string]any) { m["asker"] = "\u202eproposer" })},
		{"a question whose kind is a path", question(func(m map[string]any) { m["kind"] = "/../../x" })},
		{"a question of no form", question(func(m map[string]any) { m["form"] = "html" })},
		{"a question of a form its body is not", question(func(m map[string]any) { m["form"] = "raw" })},
		{"a question with two bodies", question(func(m map[string]any) { m["data"] = 1 })},
		{"a question with no receipt", question(func(m map[string]any) { delete(m, "receipt") })},
		{"a question whose salt is no hex", question(func(m map[string]any) { m["salt"] = "zz" })},
		{"a question with no size", question(func(m map[string]any) { delete(m, "size") })},
		{"more of no questions", map[string]any{"ok": true, "more": true}},
		{"more false", map[string]any{"ok": true, "more": false, "questions": []any{}}},
	}
	for _, c := range cases {
		if _, ok := local.ParseReply(replyOf(t, c.reply)); ok {
			t.Fatalf("%s: read", c.name)
		}
	}
}
