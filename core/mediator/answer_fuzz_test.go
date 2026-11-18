package mediator_test

import (
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// opened is a mediator with one open question, asked at 09:00 with its deadline at 10:00, and the
// question's id.
func opened(t testing.TB) (*mediator.Mediator, string) {
	t.Helper()
	m := found(t)
	return m, ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
}

// written checks that a call that returned an error wrote nothing, and that every receipt verifies.
func written(t *testing.T, m *mediator.Mediator, before int, err error) {
	t.Helper()
	if err != nil && len(m.Receipts()) != before {
		t.Fatal("a refused call wrote a receipt")
	}
	if _, err := receipts.Verify(ptesting.Key(t, "mediator").Reference(), m.Receipts()); err != nil {
		t.Fatalf("the receipts do not verify: %v", err)
	}
}

// FuzzResolve answers an open question with any caller, call, text, salt and time. It never panics;
// a refusal writes nothing, and every receipt written verifies (core-no-panic). Its seeds are signed
// calls of each action, by the decider and the proposer, and the answer texts of the vectors.
func FuzzResolve(f *testing.F) {
	m, q := opened(f)
	d, p := ptesting.Key(f, "decider").Reference(), ptesting.Key(f, "proposer").Reference()
	salt := []byte("an answer's salt of thirty-two b")
	for _, r := range []mediator.Resolution{accept(f, m, d, q, true), change(f, m, d, q, false), decline(f, m, d, q), cancel(f, m, p, q), accept(f, m, p, q, false)} {
		f.Add(d, r.Call, r.Text, r.Salt, "2026-11-12T09:01:00Z")
		f.Add(p, r.Call, r.Text, r.Salt, "2026-11-12T10:00:00Z")
	}
	f.Add(d, decline(f, m, d, q).Call, "x", salt, "2026-11-12T09:01:00Z")
	f.Add("", []byte(nil), "", []byte(nil), "")
	for _, s := range seeds.Values(f, "questions", "answer") {
		f.Add(d, []byte(s), s, salt, "2026-11-12T09:01:00Z")
	}
	f.Fuzz(func(t *testing.T, caller string, call []byte, text string, salt []byte, at string) {
		m, _ := opened(t)
		before := len(m.Receipts())
		outs, err := m.Resolve(caller, mediator.Resolution{Call: call, Text: text, Salt: salt}, at)
		written(t, m, before, err)
		if err == nil && (len(outs) == 0 || len(m.Receipts()) != before+len(outs) || len(m.Pending()) != 0) {
			t.Fatalf("an answer wrote %d receipts for %d outcomes", len(m.Receipts())-before, len(outs))
		}
		for _, n := range []string{"proposer", "applier", "decider"} {
			_, _ = m.Get(ptesting.Key(t, n).Reference())
		}
	})
}

// FuzzUndeliverable finds any id undeliverable at any time. It never panics; a refusal writes
// nothing, and every receipt verifies.
func FuzzUndeliverable(f *testing.F) {
	_, q := opened(f)
	f.Add(q, "2026-11-12T09:01:00Z")
	f.Add(q, "2026-11-12T10:00:00Z")
	f.Add(q, "9999-12-31T23:59:59Z")
	f.Add("", "")
	for _, s := range seeds.Values(f, "questions", "at") {
		f.Add(q, s)
	}
	f.Fuzz(func(t *testing.T, questionID, at string) {
		m, _ := opened(t)
		before := len(m.Receipts())
		_, err := m.Undeliverable(questionID, at)
		written(t, m, before, err)
	})
}

// FuzzTick passes time to any time. It never panics; a refusal writes nothing, and every receipt
// verifies.
func FuzzTick(f *testing.F) {
	f.Add("2026-11-12T09:59:59Z")
	f.Add("2026-11-12T10:00:00Z")
	f.Add("9999-12-31T23:59:59Z")
	f.Add("0000-01-01T00:00:00Z")
	f.Add("")
	for _, s := range seeds.Values(f, "questions", "at") {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, at string) {
		m, _ := opened(t)
		before := len(m.Receipts())
		outs, err := m.Tick(at)
		written(t, m, before, err)
		if err == nil && len(m.Receipts()) != before+len(outs) {
			t.Fatal("a tick wrote receipts it did not return")
		}
	})
}

// FuzzTheReads reads any id as any caller with Answerers, Queued and Result. None panics.
func FuzzTheReads(f *testing.F) {
	m, q := opened(f)
	f.Add(ptesting.Key(f, "proposer").Reference(), q)
	f.Add(ptesting.Key(f, "decider").Reference(), string(m.Receipts()[0][:8]))
	f.Add("", "")
	for _, s := range seeds.Values(f, "questions", "question") {
		f.Add(ptesting.Key(f, "proposer").Reference(), s)
	}
	f.Fuzz(func(t *testing.T, caller, id string) {
		m, _ := opened(t)
		_, _ = m.Answerers(id)
		_, _ = m.Queued(id)
		_, _, _ = m.Result(caller, id)
	})
}

// FuzzReload reloads a record with one receipt replaced, one open entry and one queued delivery
// changed, at any time. It never panics, and a mediator it returns holds receipts that verify.
func FuzzReload(f *testing.F) {
	m, q := opened(f)
	d := ptesting.Key(f, "decider").Reference()
	if _, err := m.Resolve(d, accept(f, m, d, q, true), "2026-11-12T09:01:00Z"); err != nil {
		f.Fatal(err)
	}
	q2 := ask(f, m, 2, "p-2", "2026-11-12T09:02:00Z")
	list := m.Receipts()
	var queued []mediator.Queued
	for _, r := range list {
		if st, err := receipts.Decode(r); err == nil {
			if qd, ok := m.Queued(st.ID); ok {
				queued = append(queued, qd)
			}
		}
	}
	sealed, foundedAt := askAndLog(f)
	set := sealed.Set
	f.Add(set, foundedAt, uint8(0), list[0], q2, "2026-11-12T10:02:00Z", "decider", queued[0].To, queued[0].Delivery, "2026-11-12T11:00:00Z")
	f.Add(set, foundedAt, uint8(9), []byte(nil), q2, "2026-11-12T10:02:00Z", "", queued[1].To, queued[1].Delivery, "2026-11-12T10:00:00Z")
	f.Add(set, foundedAt, uint8(3), list[1], q, "x", "decider", "", []byte(`{}`), "2026-11-12T11:00:00Z")
	f.Add([]byte(`{}`), "", uint8(0), []byte(`{}`), "", "", "", "", []byte(nil), "")
	med := ptesting.Key(f, "mediator")
	for _, in := range seeds.Inputs(f, "founding") {
		f.Add(in, foundedAt, uint8(0), list[0], q2, "2026-11-12T10:02:00Z", "decider", queued[0].To, queued[0].Delivery, "2026-11-12T11:00:00Z")
	}
	for _, in := range seeds.Inputs(f, "receipts", "receipt") {
		f.Add(set, foundedAt, uint8(0), in, q2, "2026-11-12T10:02:00Z", "decider", queued[0].To, in, "2026-11-12T11:00:00Z")
	}
	f.Fuzz(func(t *testing.T, set []byte, foundedAt string, at uint8, receipt []byte, question, deadline, role, to string, delivery []byte, reloadAt string) {
		l := append([][]byte(nil), list...)
		if int(at) < len(l) {
			l[at] = receipt
		}
		qd := append([]mediator.Queued(nil), queued...)
		qd[0] = mediator.Queued{To: to, Delivery: delivery}
		in := sealed
		in.Set = set
		r, outs, err := mediator.Reload(in, l, []mediator.Open{{Question: question, Deadline: deadline, Roles: []string{role}}}, qd, foundedAt, med, reloadAt)
		if err != nil {
			if r != nil || outs != nil {
				t.Fatal("a refused reload returned a mediator")
			}
			return
		}
		if _, err := receipts.Verify(med.Reference(), r.Receipts()); err != nil {
			t.Fatalf("a reloaded mediator's receipts do not verify: %v", err)
		}
	})
}
