package mediator_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/answer"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// secretsIn are the spellings of a planted value an error text or a print must not hold.
func secretsIn(values ...string) []string {
	var out []string
	for _, v := range values {
		out = append(out, v, hex.EncodeToString([]byte(v)), strings.ToUpper(hex.EncodeToString([]byte(v))))
	}
	return out
}

// TestAnswerErrorTextQuotesNoInput triggers every error and rejection of Resolve, Undeliverable,
// Tick, Result and Reload with a planted answer's text, version, salt, body and key, and reads each
// error's text: the package, the code and an index alone (error-text-no-content).
func TestAnswerErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	const text, version, body = "the planted answer text", "the planted version text", "the planted body text"
	salt := []byte("the planted salt of the answer!!") // SaltBytes long
	seed := sha256.Sum256([]byte("placket vectors: key mediator"))
	secrets := secretsIn(text, version, body, string(salt), string(seed[:]))
	m := found(t)
	p, d := ptesting.Key(t, "proposer"), ptesting.Key(t, "decider")
	msg := message(t, m.Accord(), "p-1", ptesting.Key(t, "applier").Reference(), "proposal", body, nil)
	ask, err := m.Give(p.Reference(), msg, envelope(t, 1, msg, p), nil, "2026-11-12T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	q := ask.ReceiptID
	var z mediator.Mediator
	set, foundedAt := askAndLog(t)
	list := m.Receipts()
	edited := [][]byte{append([]byte(nil), list[0]...)}
	edited[0][len(edited[0])-10] ^= 1
	open := []mediator.Open{{Question: q, Deadline: "2026-11-12T10:00:00Z", Roles: []string{"decider"}}}
	reloadErr := func(l [][]byte, o []mediator.Open, qd []mediator.Queued, at string) error {
		_, _, err := mediator.Reload(set, l, o, qd, foundedAt, ptesting.Key(t, "mediator"), at)
		return err
	}
	first := func(_ []mediator.Outcome, err error) error { return err }
	other := "sha256:" + strings.Repeat("ab", 32)
	// accepted and changed sign the decider's or the proposer's call over the planted text and salt.
	accepted := func(ref, q, text string) []byte {
		return callBy(t, m, ref, q, answer.Accepted, receipts.Digest(salt, []byte(text)), true)
	}
	changed := func(ref, q, text string) []byte {
		return callBy(t, m, ref, q, answer.Changed, receipts.Digest(salt, []byte(text)), true)
	}
	withBody := func(call any, text string, salt []byte) mediator.Resolution {
		switch c := call.(type) {
		case []byte:
			return mediator.Resolution{Call: c, Text: text, Salt: salt}
		case mediator.Resolution:
			return mediator.Resolution{Call: c.Call, Text: text, Salt: salt}
		}
		return mediator.Resolution{}
	}
	firstOf := func(_ mediator.Outcome, err error) error { return err }
	result := func(_ []mediator.Outcome, _ bool, err error) error { return err }
	cases := []struct {
		want string
		err  error
	}{
		{"mediator: at", first(m.Resolve(d.Reference(), withBody(accepted(d.Reference(), q, text), text, salt), text))},
		{"mediator: rejected: not-a-member", first(m.Resolve(text, withBody(changed(d.Reference(), q, version), version, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: answer-call", first(m.Resolve(d.Reference(), mediator.Resolution{Call: []byte(text), Text: text, Salt: salt}, "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: answer-call", first(m.Resolve(p.Reference(), withBody(accepted(d.Reference(), q, text), text, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: no-open-question", first(m.Resolve(d.Reference(), withBody(accepted(d.Reference(), other, text), text, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: not-an-answerer", first(m.Resolve(p.Reference(), withBody(changed(p.Reference(), q, version), version, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: answer-body", first(m.Resolve(d.Reference(), withBody(accepted(d.Reference(), q, text), text, salt[:15]), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: answer-body", first(m.Resolve(d.Reference(), withBody(accepted(d.Reference(), q, text), version, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: answer-body", first(m.Resolve(d.Reference(), withBody(changed(d.Reference(), q, ""), "", salt), "2026-11-12T09:01:00Z"))},
		{"mediator: rejected: answer-body", first(m.Resolve(d.Reference(), withBody(decline(t, m, d.Reference(), q), text, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: unfounded", first(z.Resolve(d.Reference(), withBody(accepted(d.Reference(), q, text), text, salt), "2026-11-12T09:01:00Z"))},
		{"mediator: at", firstOf(m.Undeliverable(q, text))},
		{"mediator: rejected: no-open-question", firstOf(m.Undeliverable(text, "2026-11-12T09:01:00Z"))},
		{"mediator: unfounded", firstOf(z.Undeliverable(q, "2026-11-12T09:01:00Z"))},
		{"mediator: at", first(m.Tick(text))},
		{"mediator: unfounded", first(z.Tick("2026-11-12T09:01:00Z"))},
		{"mediator: rejected: not-a-member", result(m.Result(text, q))},
		{"mediator: unfounded", result(z.Result(p.Reference(), q))},
		{"mediator: at", reloadErr(list, open, nil, text)},
		{"mediator: journal, index 0", reloadErr(edited, open, nil, "2026-11-12T09:30:00Z")},
		{"mediator: at, index 0", reloadErr(list, []mediator.Open{{Question: q, Deadline: text, Roles: []string{text}}}, nil, "2026-11-12T09:30:00Z")},
		{"mediator: question, index 0", reloadErr(list, []mediator.Open{{Question: text, Deadline: "2026-11-12T10:00:00Z"}}, nil, "2026-11-12T09:30:00Z")},
		{"mediator: question", reloadErr(list, nil, nil, "2026-11-12T09:30:00Z")},
		{"mediator: held, index 0", reloadErr(list, open, []mediator.Queued{{To: text, Delivery: []byte(`{"text":"` + body + `","salt":"` + hex.EncodeToString(salt) + `"}`)}}, "2026-11-12T09:30:00Z")},
	}
	for i, c := range cases {
		if c.err == nil || c.err.Error() != c.want {
			t.Fatalf("case %d: got %v, want %q", i, c.err, c.want)
		}
		for _, s := range secrets {
			if strings.Contains(c.err.Error(), s) {
				t.Fatalf("case %d: the error %q holds a planted value", i, c.err.Error())
			}
		}
	}
}

// TestAZeroValueMediator calls every operation of a Mediator no Found or Reload made: each with an
// error result returns *Error unfounded, the reads return false or nothing, and none panics
// (record 0006).
func TestAZeroValueMediator(t *testing.T) {
	t.Parallel()
	var z mediator.Mediator
	p := ptesting.Key(t, "proposer").Reference()
	q := "sha256:" + strings.Repeat("ab", 32)
	unfounded := func(name string, err error) {
		t.Helper()
		if !errorCode(err, mediator.CodeUnfounded) || err.Error() != "mediator: unfounded" {
			t.Fatalf("%s: got %v", name, err)
		}
	}
	_, err := z.Give(p, []byte(`{}`), []byte(`{}`), nil, "2026-11-12T09:00:00Z")
	unfounded("Give", err)
	_, err = z.Get(p)
	unfounded("Get", err)
	_, err = z.Resolve(p, accept(t, &z, p, q, true), "2026-11-12T09:00:00Z")
	unfounded("Resolve", err)
	_, err = z.Undeliverable(q, "2026-11-12T09:00:00Z")
	unfounded("Undeliverable", err)
	_, err = z.Tick("2026-11-12T09:00:00Z")
	unfounded("Tick", err)
	_, ok, err := z.Result(p, q)
	unfounded("Result", err)
	if ok {
		t.Fatal("Result found a question")
	}
	if l, ok := z.Answerers(q); l != nil || ok {
		t.Fatalf("Answerers: %v, %v", l, ok)
	}
	if d, ok := z.Queued(q); ok || d.To != "" || d.Delivery != nil {
		t.Fatalf("Queued: %v", ok)
	}
	if z.Accord() != "" || z.Card() != "" || len(z.Receipts()) != 0 || len(z.Pending()) != 0 {
		t.Fatal("a zero-value mediator holds state")
	}
}

// TestResolutionAndQueuedNeverPrint prints a Resolution holding an answer's text and salt, and a
// Queued holding a delivery's body and salt, under every verb, as values and pointers, and finds
// none of them: each prints one fixed text (secrets-never-print).
func TestResolutionAndQueuedNeverPrint(t *testing.T) {
	t.Parallel()
	const text, body = "the answer that must not print", "the body that must not print"
	salt := []byte("a salt that must not print")
	r := mediator.Resolution{Call: []byte(`{"answer":"` + text + `"}`), Text: text, Salt: salt}
	q := mediator.Queued{To: ptesting.Key(t, "applier").Reference(), Delivery: []byte(`{"text":"` + body + `"}`)}
	secrets := secretsIn(text, body, string(salt))
	secrets = append(secrets, fmt.Sprint(salt)[1:20])
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%z"} {
		for _, h := range []any{r, &r, q, &q, []any{r, q}, struct{ R mediator.Resolution }{r}} {
			out := fmt.Sprintf(verb, h)
			for _, s := range secrets {
				if strings.Contains(out, s) {
					t.Fatalf("%s prints a secret: %q", verb, out)
				}
			}
		}
	}
	if got := fmt.Sprintf("%v|%#v|%x|%s|%s", r, r, r, r.String(), r.GoString()); got != "mediator.Resolution|mediator.Resolution|mediator.Resolution|mediator.Resolution|mediator.Resolution" {
		t.Fatalf("a Resolution prints %q", got)
	}
	if got := fmt.Sprintf("%v|%#v|%x|%s|%s", q, q, q, q.String(), q.GoString()); got != "mediator.Queued|mediator.Queued|mediator.Queued|mediator.Queued|mediator.Queued" {
		t.Fatalf("a Queued prints %q", got)
	}
}
