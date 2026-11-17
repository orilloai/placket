package mediator_test

import (
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
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

// FuzzResolve answers an open question with any caller, id, action, text, salt, release and time. It
// never panics; a refusal writes nothing, and every receipt written verifies (core-no-panic).
func FuzzResolve(f *testing.F) {
	_, q := opened(f)
	d, p := ptesting.Key(f, "decider").Reference(), ptesting.Key(f, "proposer").Reference()
	f.Add(d, q, "accepted", "accepted", []byte("an answer's salt"), true, "2026-11-12T09:01:00Z")
	f.Add(d, q, "changed", "A version.", []byte("an answer's salt"), true, "2026-11-12T09:01:00Z")
	f.Add(d, q, "changed", "", []byte("an answer's salt"), false, "2026-11-12T09:01:00Z")
	f.Add(d, q, "declined", "", []byte(nil), true, "2026-11-12T09:01:00Z")
	f.Add(d, q, "cancelled", "x", []byte("short"), false, "2026-11-12T10:00:00Z")
	f.Add(p, q, "accepted", "accepted", []byte("an answer's salt"), false, "2026-11-12T09:01:00Z")
	f.Add("", "", "", "", []byte(nil), false, "")
	f.Fuzz(func(t *testing.T, caller, questionID, action, answer string, salt []byte, release bool, at string) {
		m, _ := opened(t)
		before := len(m.Receipts())
		outs, err := m.Resolve(caller, questionID, mediator.Resolution{Action: mediator.Action(action), Answer: answer, Salt: salt, Release: release}, at)
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
	if _, err := m.Resolve(d, q, accept(true), "2026-11-12T09:01:00Z"); err != nil {
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
	set, foundedAt := askAndLog(f)
	f.Add(set, foundedAt, uint8(0), list[0], q2, "2026-11-12T10:02:00Z", "decider", queued[0].To, queued[0].Delivery, "2026-11-12T11:00:00Z")
	f.Add(set, foundedAt, uint8(9), []byte(nil), q2, "2026-11-12T10:02:00Z", "", queued[1].To, queued[1].Delivery, "2026-11-12T10:00:00Z")
	f.Add(set, foundedAt, uint8(3), list[1], q, "x", "decider", "", []byte(`{}`), "2026-11-12T11:00:00Z")
	f.Add([]byte(`{}`), "", uint8(0), []byte(`{}`), "", "", "", "", []byte(nil), "")
	med := ptesting.Key(f, "mediator")
	f.Fuzz(func(t *testing.T, founding []byte, foundedAt string, at uint8, receipt []byte, question, deadline, role, to string, delivery []byte, reloadAt string) {
		l := append([][]byte(nil), list...)
		if int(at) < len(l) {
			l[at] = receipt
		}
		qd := append([]mediator.Queued(nil), queued...)
		qd[0] = mediator.Queued{To: to, Delivery: delivery}
		r, outs, err := mediator.Reload(founding, l, []mediator.Open{{Question: question, Deadline: deadline, Roles: []string{role}}}, qd, foundedAt, med, reloadAt)
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
