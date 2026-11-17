package mediator_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/mediator"
)

// askWith asks the decider about a proposal whose part has the body given, as name and value, at
// 09:00, and returns the question's id.
func askWith(t testing.TB, m *mediator.Mediator, seq int64, id, name string, value any) string {
	t.Helper()
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	msg := message(t, m.Accord(), id, ap.Reference(), "proposal", "", func(o map[string]any) {
		part := o["parts"].([]any)[0].(map[string]any)
		delete(part, "text")
		part[name] = value
	})
	out, err := m.Give(p.Reference(), msg, envelope(t, seq, msg, p), "2026-11-12T09:00:00Z")
	if err != nil || out.Verdict != "escalate" {
		t.Fatalf("the ask: %+v, %v", out, err)
	}
	return out.ReceiptID
}

// TestQuestionReadsAnOpenQuestion reads an open question as the mediator's own reference, which is
// no member, and as its answerer: each gets the question's id, asker, receiver, kind, form, deadline,
// roles and part. A text part is its text, a raw part its decoded bytes and a data part its canonical
// bytes (maybe-1).
func TestQuestionReadsAnOpenQuestion(t *testing.T) {
	t.Parallel()
	p, ap, d := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference(), ptesting.Key(t, "decider").Reference()
	med := ptesting.Key(t, "mediator").Reference()
	m := found(t)
	text := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	raw := askWith(t, m, 2, "p-2", "raw", base64.StdEncoding.EncodeToString([]byte{0x00, 0xff, 0x10}))
	data := askWith(t, m, 3, "p-3", "data", map[string]any{"b": []any{1, "two"}, "a": true})
	cases := []struct {
		id, form string
		part     []byte
	}{
		{text, "text", []byte("Proposal p-1.")},
		{raw, "raw", []byte{0x00, 0xff, 0x10}},
		{data, "data", []byte(`{"a":true,"b":[1,"two"]}`)},
	}
	for _, c := range cases {
		for _, caller := range []string{med, d} {
			q, ok, err := m.Question(caller, c.id)
			if err != nil || !ok {
				t.Fatalf("%s as %s: %v, %v", c.form, caller, ok, err)
			}
			want := mediator.Question{ID: c.id, Asker: p, Receiver: ap, Kind: "proposal", Form: c.form,
				Deadline: "2026-11-12T10:00:00Z", Roles: []string{"decider"}, Part: c.part}
			if !reflect.DeepEqual(q, want) {
				t.Fatalf("%s as %s: got %q %q %q %q %q %q %q %x", c.form, caller, q.ID, q.Asker, q.Receiver, q.Kind, q.Form, q.Deadline, q.Roles, q.Part)
			}
		}
	}
}

// TestQuestionRowsInOrder is section 6's order for a read: a stranger is not-a-member whatever the
// id; a closed or unknown question is false with no error, to a member who is not an answerer too;
// and a member who is not an answerer of an open question is not-an-answerer. The mediator's own
// reference reads nothing once the question is closed: it is no member.
func TestQuestionRowsInOrder(t *testing.T) {
	t.Parallel()
	p, d := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "decider").Reference()
	stranger, med := ptesting.Key(t, "stranger").Reference(), ptesting.Key(t, "mediator").Reference()
	m := found(t)
	open := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	closed := ask(t, m, 2, "p-2", "2026-11-12T09:00:00Z")
	if _, err := m.Resolve(d, closed, mediator.Resolution{Action: mediator.Declined}, "2026-11-12T09:01:00Z"); err != nil {
		t.Fatal(err)
	}
	unknown := "sha256:" + strings.Repeat("ab", 32)
	none := func(name string, q mediator.Question, ok bool, err error) {
		t.Helper()
		if ok || err != nil || !reflect.DeepEqual(q, mediator.Question{}) {
			t.Fatalf("%s: got %v, %v", name, ok, err)
		}
	}
	for _, id := range []string{open, closed, unknown} {
		q, ok, err := m.Question(stranger, id)
		if !rejected(err, mediator.NotAMember) || ok || !reflect.DeepEqual(q, mediator.Question{}) {
			t.Fatalf("a stranger: got %v, %v", ok, err)
		}
	}
	for _, caller := range []string{p, d} {
		q, ok, err := m.Question(caller, closed)
		none("closed", q, ok, err)
		q, ok, err = m.Question(caller, unknown)
		none("unknown", q, ok, err)
	}
	q, ok, err := m.Question(p, open)
	if !rejected(err, mediator.NotAnAnswerer) || ok || !reflect.DeepEqual(q, mediator.Question{}) {
		t.Fatalf("the asker: got %v, %v", ok, err)
	}
	if _, _, err := m.Question(med, closed); !rejected(err, mediator.NotAMember) {
		t.Fatalf("the mediator on a closed question: got %v", err)
	}
	var z mediator.Mediator
	if _, ok, err := z.Question(med, open); ok || !errorCode(err, mediator.CodeUnfounded) {
		t.Fatalf("a zero-value mediator: got %v, %v", ok, err)
	}
}

// TestQuestionIsACopy changes what Question returned and reads the question again, unchanged
// (results-are-copies); and a read writes nothing.
func TestQuestionIsACopy(t *testing.T) {
	t.Parallel()
	d := ptesting.Key(t, "decider").Reference()
	m := found(t)
	id := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	before := m.Receipts()
	q, _, _ := m.Question(d, id)
	q.Roles[0] = "proposer"
	q.Part[0] = 'X'
	again, _, _ := m.Question(d, id)
	if again.Roles[0] != "decider" || !bytes.Equal(again.Part, []byte("Proposal p-1.")) {
		t.Fatalf("a changed result changed the question: %q, %q", again.Roles, again.Part)
	}
	if !reflect.DeepEqual(m.Receipts(), before) || !reflect.DeepEqual(m.Pending(), []string{id}) {
		t.Fatal("a read wrote")
	}
}

// TestQuestionNeverPrints prints a Question holding a part under every verb, as a value, a pointer
// and inside a slice and a struct, and finds neither the part nor its hex: it prints one fixed text
// (secrets-never-print).
func TestQuestionNeverPrints(t *testing.T) {
	t.Parallel()
	m := found(t)
	id := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	q, _, _ := m.Question(ptesting.Key(t, "decider").Reference(), id)
	const part = "Proposal p-1."
	secrets := []string{part[:8], hex.EncodeToString([]byte(part))[:16], strings.ToUpper(hex.EncodeToString([]byte(part)))[:16],
		fmt.Sprint([]byte(part[:4]))[1:12]}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T", "%z"} {
		for _, h := range []any{q, &q, []mediator.Question{q}, struct{ Q mediator.Question }{q}} {
			out := fmt.Sprintf(verb, h)
			for _, s := range secrets {
				if strings.Contains(out, s) {
					t.Fatalf("%s prints the part: %q", verb, out)
				}
			}
		}
	}
	if got := fmt.Sprintf("%v|%+v|%#v|%s|%x", q, &q, q, q, q); got != strings.Repeat("mediator.Question|", 4)+"mediator.Question" {
		t.Fatalf("got %q", got)
	}
	if q.String() != "mediator.Question" || q.GoString() != "mediator.Question" {
		t.Fatalf("String %q, GoString %q", q.String(), q.GoString())
	}
}

// FuzzQuestion reads any id as any caller. It never panics and never writes.
func FuzzQuestion(f *testing.F) {
	m, q := opened(f)
	f.Add(ptesting.Key(f, "decider").Reference(), q)
	f.Add(ptesting.Key(f, "mediator").Reference(), q)
	f.Add(ptesting.Key(f, "proposer").Reference(), q)
	f.Add(ptesting.Key(f, "stranger").Reference(), string(m.Receipts()[0][:8]))
	f.Add("", "")
	for _, s := range seeds.Values(f, "questions", "question") {
		f.Add(ptesting.Key(f, "decider").Reference(), s)
	}
	f.Fuzz(func(t *testing.T, caller, id string) {
		m, _ := opened(t)
		before := len(m.Receipts())
		_, _, _ = m.Question(caller, id)
		if len(m.Receipts()) != before || len(m.Pending()) != 1 {
			t.Fatal("a read wrote")
		}
	})
}
