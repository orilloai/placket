package local_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
)

// TestTheDoorChecksInOrder sends calls that each fail one of the door's checks, and calls that fail
// two: each is refused with the code of the first that fails (door-check-order), and none changes
// the receipts, the journal or the nonces the door remembers. A stranger's well-signed call whose
// message is not canonical reaches the mediator, whose first row is membership.
func TestTheDoorChecksInOrder(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	p := key(t, "proposer")
	now := w.clock.Now()
	nonce := bytes.Repeat([]byte{7}, 16)
	sign := func(op, accord string, at time.Time, args map[string]any) []byte {
		b, err := local.SignCall(p, op, accord, at, nonce, args)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	good := sign(local.OpGet, w.accord, now, nil)
	// A bad signature: the call's caller changed after it was signed.
	forged := bytes.Replace(good, []byte(p.Reference()), []byte(key(t, "decider").Reference()), 1)
	other := "sha256:" + strings.Repeat("0", 64)
	cases := []struct {
		name string
		line []byte
		want local.DoorCode
	}{
		{"over the size", bytes.Repeat([]byte{'a'}, local.MaxCall+1), local.TooLarge},
		{"over the size, and no JSON", append([]byte(`{"call":`), bytes.Repeat([]byte{' '}, local.MaxCall)...), local.TooLarge},
		{"not JSON", []byte(`{"call"`), local.BadCall},
		{"no sig", []byte(`{"call":{}}`), local.BadCall},
		{"an op there is none of", sign("check", w.accord, now, nil), local.BadCall},
		{"a call of a repeated member", bytes.Replace(good, []byte(`"op":"get"`), []byte(`"op":"get","op":"get"`), 1), local.BadCall},
		{"a bad signature", forged, local.BadSignature},
		{"a bad signature, another Accord and stale", bytes.Replace(sign(local.OpGet, other, now.Add(-time.Hour), nil), []byte(p.Reference()), []byte(key(t, "decider").Reference()), 1), local.BadSignature},
		{"another Accord", sign(local.OpGet, other, now, nil), local.WrongAccord},
		{"another Accord and stale", sign(local.OpGet, other, now.Add(-61*time.Second), nil), local.WrongAccord},
		{"stale, late", sign(local.OpGet, w.accord, now.Add(-61*time.Second), nil), local.Stale},
		{"stale, early", sign(local.OpGet, w.accord, now.Add(61*time.Second), nil), local.Stale},
	}
	before := w.receipts(t)
	journal := snapshot(t, w.journal)
	c := w.client(t, "proposer")
	for _, x := range cases {
		rep, err := c.Send(x.line)
		if err != nil || rep.Door != x.want || rep.Rejection != "" || rep.Error != "" {
			t.Fatalf("%s: got door %q, rejection %q, error %q, %v", x.name, rep.Door, rep.Rejection, rep.Error, err)
		}
	}
	if !bytes.Equal(w.receipts(t), before) || len(snapshot(t, w.journal)) != len(journal) {
		t.Fatal("a refused call wrote to the journal")
	}
	// The nonce of every refused call is still free: the same nonce now passes, once.
	if rep, err := c.Send(sign(local.OpGet, w.accord, now.Add(60*time.Second), nil)); err != nil || rep.Door != "" || rep.Rejection != "" {
		t.Fatalf("a fresh call: door %q, %v", rep.Door, err)
	}
	if rep, _ := c.Send(sign(local.OpGet, w.accord, now, nil)); rep.Door != local.Replayed {
		t.Fatalf("a repeated nonce: got %q", rep.Door)
	}
	// A stranger's call that the door lets through: the mediator's first row refuses it, before it
	// reads the message, which the canonical form would refuse.
	s := key(t, "stranger")
	msg := []byte(`{"messageId":"m","messageId":"m"}`)
	h, _ := canonical.TypedHash([]byte(`{"messageId":"m"}`))
	line, _ := local.SignCall(s, local.OpGive, w.accord, now, bytes.Repeat([]byte{8}, 16), local.GiveArgs(msg, envelope(t, 1, h, s)))
	if rep, err := c.Send(line); err != nil || rep.Rejection != mediator.NotAMember {
		t.Fatalf("a stranger: got rejection %q, door %q, %v", rep.Rejection, rep.Door, err)
	}
}

// TestTheDoorPassesTheBytes gives, as a member with a valid envelope signature, a message the
// canonical form refuses: the door hands the bytes on as decoded, and the reply is the mediator's
// rejection not-canonical, not a door code (door-passes-bytes).
func TestTheDoorPassesTheBytes(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	p := key(t, "proposer")
	for _, msg := range [][]byte{
		[]byte(`{"messageId":"m","messageId":"n"}`),
		[]byte("{\"messageId\":\"\xff\"}"),
	} {
		h, _ := canonical.TypedHash([]byte(`{}`))
		rep := w.call(t, "proposer", local.OpGive, local.GiveArgs(msg, envelope(t, 1, h, p)))
		if rep.Rejection != mediator.NotCanonical || rep.Door != "" {
			t.Fatalf("%q: got rejection %q, door %q", msg, rep.Rejection, rep.Door)
		}
	}
	if len(w.receipts(t)) != 0 {
		t.Fatal("a rejected give wrote a receipt")
	}
}

// TestParseCallReadsOnlyItsForm reads calls whose members are each wrong in form: each is bad-call,
// and a call that is right in form parses whatever its signature.
func TestParseCallReadsOnlyItsForm(t *testing.T) {
	t.Parallel()
	p := key(t, "proposer")
	good, _ := local.SignCall(p, local.OpResolve, "a", time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC), make([]byte, 16), local.ResolveArgs("q", mediator.Changed, "v"))
	c, code := local.ParseCall(good)
	if code != "" || c.Op != local.OpResolve || c.Question != "q" || c.Action != "changed" || c.Answer != "v" || !c.HasAnswer || !c.Verified() {
		t.Fatalf("got %q", code)
	}
	var o map[string]any
	_ = json.Unmarshal(good, &o)
	call := o["call"].(map[string]any)
	for name, value := range map[string]any{"at": "2026-11-12T09:00:00.5Z", "nonce": strings.Repeat("A", 32), "args": []any{}, "caller": 1} {
		was := call[name]
		call[name] = value
		b, _ := json.Marshal(o)
		if _, code := local.ParseCall(b); code != local.BadCall {
			t.Fatalf("%s %v: got %q", name, value, code)
		}
		call[name] = was
	}
	o["sig"] = "not base64!"
	b, _ := json.Marshal(o)
	if _, code := local.ParseCall(b); code != local.BadCall {
		t.Fatalf("a sig that is not base64: got %q", code)
	}
	o["sig"] = base64.StdEncoding.EncodeToString(make([]byte, 64))
	b, _ = json.Marshal(o)
	if c, code := local.ParseCall(b); code != "" || c.Verified() {
		t.Fatalf("a wrong sig: got %q, verified %v", code, c.Verified())
	}
}

// FuzzParseCall reads any bytes as a call. It never panics, and a call it reads is at most MaxCall
// bytes with a known op.
func FuzzParseCall(f *testing.F) {
	p := key(f, "proposer")
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	for _, op := range []string{local.OpGive, local.OpGet, local.OpQuestions, local.OpResolve, local.OpResult} {
		args := map[string]any{}
		switch op {
		case local.OpGive:
			args = local.GiveArgs([]byte("{}"), []byte("{}"))
		case local.OpResolve:
			args = local.ResolveArgs("q", mediator.Accepted, "accepted")
		case local.OpResult:
			args = map[string]any{"question": "q"}
		}
		b, _ := local.SignCall(p, op, "a", at, make([]byte, 16), args)
		f.Add(b)
	}
	f.Add([]byte(`{"call":{},"sig":""}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		c, code := local.ParseCall(b)
		if code != "" {
			return
		}
		if len(b) > local.MaxCall {
			t.Fatal("a call over the size was read")
		}
		switch c.Op {
		case local.OpGive, local.OpGet, local.OpQuestions, local.OpResolve, local.OpResult:
		default:
			t.Fatalf("op %q", c.Op)
		}
		_ = c.Verified()
	})
}

// FuzzParseReply reads any bytes as a reply. It never panics.
func FuzzParseReply(f *testing.F) {
	f.Add([]byte(`{"ok":true}`))
	f.Add([]byte(`{"ok":false,"door":"stale"}`))
	f.Add([]byte(`{"ok":true,"outcome":{"reason":"ok","receipt":{},"receiptId":"x","verdict":"pass"},"salt":"00"}`))
	f.Add([]byte(`{"answerCall":{},"delivery":{"a":1},"ok":true}`))
	f.Add([]byte(`{"ok":true,"questions":[{"asker":"a","deadline":"d","form":"raw","id":"i","kind":"k","size":3}]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = local.ParseReply(b)
	})
}

// FuzzVerifyAnswerCall checks any bytes as an answer call. It never panics, and what it accepts is a
// resolve call that accepts or changes.
func FuzzVerifyAnswerCall(f *testing.F) {
	d := key(f, "decider")
	line, _ := local.SignCall(d, local.OpResolve, "a", time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC), make([]byte, 16), local.ResolveArgs("q", mediator.Accepted, "yes"))
	c, _ := local.ParseCall(line)
	f.Add(local.AnswerCall(c))
	f.Add([]byte(`{"call":"e30=","sig":""}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		a, ok := local.VerifyAnswerCall(b, d.Reference())
		if ok && a.Action != "accepted" && a.Action != "changed" {
			t.Fatalf("accepted action %q", a.Action)
		}
	})
}
