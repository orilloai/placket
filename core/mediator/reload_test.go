package mediator_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// give gives a part of kind from one named party to another at seq and at, and returns its outcome.
func give(t testing.TB, m *mediator.Mediator, from, to string, seq int64, id, kind, at string) mediator.Outcome {
	t.Helper()
	f, r := key(t, from), key(t, to)
	msg := message(t, m.Accord(), id, r.Reference(), kind, "Part "+id+".", nil)
	out, err := m.Give(f.Reference(), msg, envelope(t, seq, msg, f), at)
	if err != nil {
		t.Fatalf("give %s: %v", id, err)
	}
	return out
}

// queuedOf returns every delivery still queued in m, by the receipts in order.
func queuedOf(m *mediator.Mediator) []mediator.Queued {
	var out []mediator.Queued
	for _, r := range m.Receipts() {
		st, err := receipts.Decode(r)
		if err != nil {
			continue
		}
		if q, ok := m.Queued(st.ID); ok {
			out = append(out, q)
		}
	}
	return out
}

// before is what a stopped mediator left: gives from the proposer to the applier and from the
// decider to the proposer, the applier's note fetched, an accepted proposal whose release the
// applier has not fetched, a changed one, and two questions still open, asked at 09:40 and 10:30
// with deadlines 10:40 and 11:30.
type before struct {
	m                      *mediator.Mediator
	accepted, changed      string // the two closed questions
	first, second          string // the two open questions
	acceptOuts, changeOuts []mediator.Outcome
	open                   []mediator.Open
}

func record(t *testing.T) before {
	t.Helper()
	var b before
	b.m = found(t)
	d := key(t, "decider").Reference()
	give(t, b.m, "proposer", "applier", 1, "n-1", "note", "2026-11-12T08:30:00Z")
	give(t, b.m, "decider", "proposer", 1, "a-1", "answer", "2026-11-12T08:40:00Z")
	if got, _ := b.m.Get(key(t, "applier").Reference()); got == nil {
		t.Fatal("the note was not queued")
	}
	var err error
	b.accepted = ask(t, b.m, 2, "p-1", "2026-11-12T09:00:00Z")
	if b.acceptOuts, err = b.m.Resolve(d, b.accepted, accept(true), "2026-11-12T09:10:00Z"); err != nil || len(b.acceptOuts) != 2 {
		t.Fatalf("the accept: %+v, %v", b.acceptOuts, err)
	}
	b.changed = ask(t, b.m, 3, "p-2", "2026-11-12T09:20:00Z")
	if b.changeOuts, err = b.m.Resolve(d, b.changed, change(false), "2026-11-12T09:30:00Z"); err != nil || len(b.changeOuts) != 1 {
		t.Fatalf("the change: %+v, %v", b.changeOuts, err)
	}
	b.first = ask(t, b.m, 4, "p-3", "2026-11-12T09:40:00Z")
	b.second = ask(t, b.m, 5, "p-4", "2026-11-12T10:30:00Z")
	b.open = []mediator.Open{
		{Question: b.first, Deadline: "2026-11-12T10:40:00Z", Roles: []string{"decider"}},
		{Question: b.second, Deadline: "2026-11-12T11:30:00Z", Roles: []string{"decider"}},
	}
	return b
}

// reload reloads the record at the time at, giving its deliveries in the reverse of the order they
// were queued in.
func reload(t *testing.T, b before, at string) (*mediator.Mediator, []mediator.Outcome) {
	t.Helper()
	set, foundedAt := askAndLog(t)
	q := queuedOf(b.m)
	for i, j := 0, len(q)-1; i < j; i, j = i+1, j-1 {
		q[i], q[j] = q[j], q[i]
	}
	r, outs, err := mediator.Reload(set, foundedAt, key(t, "mediator"), b.m.Receipts(), b.open, q, at)
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	return r, outs
}

// TestReloadCarriesOn reloads a stopped mediator (record 0008) at 11:00: the earlier open question
// closes first, timed out at its deadline of 10:40, then the later one, undeliverable at 11:00. Then
// the reloaded mediator carries on: each chain and each sender's seq continue from the record, the
// applier fetches the released proposal once, the proposer its deliveries in the order queued, no
// closed question can be answered, Result shows each closing to the proposer and to nobody else, and
// the whole list verifies.
func TestReloadCarriesOn(t *testing.T) {
	t.Parallel()
	b := record(t)
	p, ap, d := key(t, "proposer").Reference(), key(t, "applier").Reference(), key(t, "decider").Reference()
	list := b.m.Receipts()
	r, outs := reload(t, b, "2026-11-12T11:00:00Z")

	if len(outs) != 2 || outs[0].Reason != receipts.ReasonTimeout || outs[1].Reason != receipts.ReasonUndeliverable ||
		outs[0].Verdict != "block" || outs[1].Verdict != "block" {
		t.Fatalf("the closings: %+v", outs)
	}
	for i, c := range []struct{ q, at string }{{b.first, "2026-11-12T10:40:00Z"}, {b.second, "2026-11-12T11:00:00Z"}} {
		st := statement(t, r, outs[i].ReceiptID)
		ask := statement(t, r, c.q)
		if st.Predicate.At != c.at || st.Predicate.From != d || st.Predicate.To != p || st.Predicate.Envelope != ask.Predicate.Envelope ||
			st.Subject != ask.Subject || st.Predicate.Kind != receipts.KindResolution {
			t.Fatalf("closing %d: %+v", i, st)
		}
	}
	want := append(append([][]byte(nil), list...), outs[0].Receipt, outs[1].Receipt)
	if !reflect.DeepEqual(r.Receipts(), want) || len(r.Pending()) != 0 {
		t.Fatalf("the reloaded mediator holds %d receipts and pending %v", len(r.Receipts()), r.Pending())
	}

	// The applier fetches the released proposal once, as the stopped mediator queued it.
	released, ok := b.m.Queued(b.acceptOuts[1].ReceiptID)
	if !ok {
		t.Fatal("the release was not queued before the stop")
	}
	if got, err := r.Get(ap); err != nil || !bytes.Equal(got, released.Delivery) {
		t.Fatalf("the applier's fetch: %s, %v", got, err)
	}
	if got, err := r.Get(ap); got != nil || err != nil {
		t.Fatalf("a second fetch: %s, %v", got, err)
	}
	// The proposer's three deliveries, the decider's given answer, the accepted answer and the version,
	// come in the order they were queued, whatever order they were given back in.
	var mine []mediator.Queued
	for _, q := range queuedOf(b.m) {
		if q.To == p {
			mine = append(mine, q)
		}
	}
	if len(mine) != 3 {
		t.Fatalf("the proposer held %d deliveries", len(mine))
	}
	for i, q := range mine {
		if got, err := r.Get(p); err != nil || !bytes.Equal(got, q.Delivery) {
			t.Fatalf("the proposer's fetch %d: %s, %v", i, got, err)
		}
	}

	// A closed question takes no answer.
	for _, q := range []string{b.first, b.second, b.accepted} {
		if _, err := r.Resolve(d, q, accept(true), "2026-11-12T11:01:00Z"); !rejected(err, mediator.NoOpenQuestion) {
			t.Fatalf("an answer to %s: %v", q, err)
		}
	}

	// Result: each closing to its asker, the accept's resolution then its release.
	for _, c := range []struct {
		q    string
		want []mediator.Outcome
	}{
		{b.accepted, b.acceptOuts}, {b.changed, b.changeOuts}, {b.first, outs[:1]}, {b.second, outs[1:]},
	} {
		got, ok, err := r.Result(p, c.q)
		if err != nil || !ok || !reflect.DeepEqual(got, c.want) {
			t.Fatalf("Result of %s: %+v, %v, %v", c.q, got, ok, err)
		}
	}
	if got, ok, err := r.Result(ap, b.accepted); got != nil || ok || err != nil {
		t.Fatalf("Result to another member: %+v, %v, %v", got, ok, err)
	}
	if _, _, err := r.Result(key(t, "stranger").Reference(), b.accepted); !rejected(err, mediator.NotAMember) {
		t.Fatalf("Result to a stranger: %v", err)
	}

	// Every sender's sequence and every chain carry on.
	last := map[[2]string]receipts.Statement{}
	st, err := receipts.Verify(key(t, "mediator").Reference(), r.Receipts())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		last[[2]string{s.Predicate.From, s.Predicate.To}] = s
	}
	if _, err := r.Give(p, msgFor(t, r, "n-5", ap), envelope(t, 5, msgFor(t, r, "n-5", ap), key(t, "proposer")), "2026-11-12T11:02:00Z"); !rejected(err, mediator.EnvelopeSequence) {
		t.Fatalf("the proposer's last envelope again: %v", err)
	}
	if _, err := r.Give(d, msgFor(t, r, "a-2", p), envelope(t, 1, msgFor(t, r, "a-2", p), key(t, "decider")), "2026-11-12T11:02:00Z"); !rejected(err, mediator.EnvelopeSequence) {
		t.Fatalf("the decider's last envelope again: %v", err)
	}
	for _, c := range []struct {
		from, to string
		seq      int64
		kind     string
	}{{"proposer", "applier", 6, "note"}, {"decider", "proposer", 2, "answer"}} {
		out := give(t, r, c.from, c.to, c.seq, c.kind+"-next", c.kind, "2026-11-12T11:03:00Z")
		prev := last[[2]string{key(t, c.from).Reference(), key(t, c.to).Reference()}]
		got := statement(t, r, out.ReceiptID)
		if out.Verdict != "pass" || got.Predicate.Seq != prev.Predicate.Seq+1 || got.Predicate.Prev != prev.ID || got.Predicate.Envelope.Seq != c.seq {
			t.Fatalf("the next give from %s to %s: %+v after %+v", c.from, c.to, got.Predicate, prev.Predicate)
		}
	}
	if _, err := receipts.Verify(key(t, "mediator").Reference(), r.Receipts()); err != nil {
		t.Fatalf("the whole list does not verify: %v", err)
	}
}

func msgFor(t testing.TB, m *mediator.Mediator, id, to string) []byte {
	t.Helper()
	return message(t, m.Accord(), id, to, "note", "Part "+id+".", nil)
}

// built signs a receipt of subject and predicate, the next of its pair after list, under the
// mediator's key, so that only what the test changes is wrong with it.
func built(t *testing.T, list [][]byte, subject receipts.Subject, p receipts.Predicate) []byte {
	t.Helper()
	st, err := receipts.Verify(key(t, "mediator").Reference(), list)
	if err != nil {
		t.Fatal(err)
	}
	p.Seq, p.Prev = 1, ""
	for _, s := range st {
		if s.Predicate.From == p.From && s.Predicate.To == p.To {
			p.Seq, p.Prev = s.Predicate.Seq+1, s.ID
		}
	}
	r, _, err := receipts.Build(subject, p, key(t, "mediator"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// withText returns a delivery's bytes with its text replaced, in canonical form.
func withText(t *testing.T, delivery []byte, text string) []byte {
	t.Helper()
	return withMembers(t, delivery, map[string]any{"text": text})
}

// withMembers returns a delivery's bytes with the members given replaced, in canonical form.
func withMembers(t *testing.T, delivery []byte, members map[string]any) []byte {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal(delivery, &d); err != nil {
		t.Fatal(err)
	}
	for k, v := range members {
		d[k] = v
	}
	b, _ := json.Marshal(d)
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestReloadRefusesEachFault changes one thing at a time in a good record: each change makes Reload
// return no mediator and the error record 0008 gives it, with its index where it has one.
func TestReloadRefusesEachFault(t *testing.T) {
	t.Parallel()
	b := record(t)
	set, foundedAt := askAndLog(t)
	med := key(t, "mediator")
	list, queued := b.m.Receipts(), queuedOf(b.m)
	ask := statement(t, b.m, b.first)
	other := buildSet(t, askAndLogRoles, askAndLogParties(60)...)
	another := "sha256:" + strings.Repeat("0", 64)
	edited := append([][]byte(nil), list...)
	edited[3] = append([]byte(nil), list[3]...)
	edited[3][len(edited[3])-10] ^= 1
	// The applier's one delivery, the released proposal, is moved to the end of the list.
	var released mediator.Queued
	var rest []mediator.Queued
	for _, q := range queued {
		if q.To == key(t, "applier").Reference() {
			released = q
		} else {
			rest = append(rest, q)
		}
	}
	if released.To == "" || len(rest) != len(queued)-1 {
		t.Fatalf("the applier holds no one delivery: %d of %d", len(queued)-len(rest), len(queued))
	}
	queued = append(rest, released)
	otherMediator := key(t, "other-mediator")
	cases := []struct {
		name   string
		set    []byte
		key    *signing.Key
		list   [][]byte
		open   []mediator.Open
		queued []mediator.Queued
		at     string
		code   mediator.ErrorCode
		index  int
		inner  receipts.Code // the verifier's code a journal error wraps
		other  error         // an error that is not mediator's
	}{
		{name: "a bad reload time", at: "2026-11-12T11:00:00", code: mediator.CodeAt, index: -1},
		{name: "another mediator's key", key: &otherMediator, other: mediator.ErrOtherMediator},
		{name: "a set founding refuses", set: set[1:], other: &setup.Error{Code: setup.NotCanonical}},
		{name: "another founding set", set: other, code: mediator.CodeJournal, index: 0},
		{name: "a receipt of another Accord", list: append(append([][]byte(nil), list...), built(t, list,
			receipts.Subject{Name: "note", Digest: strings.Repeat("ab", 32)},
			receipts.Predicate{Accord: another, Card: ask.Predicate.Card, Kind: receipts.KindCrossing, From: ask.Predicate.From,
				To: ask.Predicate.To, At: "2026-11-12T10:50:00Z", Envelope: receipts.Envelope{Sender: ask.Predicate.From, Seq: 6, Hash: another},
				Verdict: "pass", Reason: "ok"})), code: mediator.CodeJournal, index: len(list)},
		{name: "a receipt of another card", list: append(append([][]byte(nil), list...), built(t, list,
			receipts.Subject{Name: "note", Digest: strings.Repeat("ab", 32)},
			receipts.Predicate{Accord: ask.Predicate.Accord, Card: another, Kind: receipts.KindCrossing, From: ask.Predicate.From,
				To: ask.Predicate.To, At: "2026-11-12T10:50:00Z", Envelope: receipts.Envelope{Sender: ask.Predicate.From, Seq: 6, Hash: another},
				Verdict: "pass", Reason: "ok"})), code: mediator.CodeJournal, index: len(list)},
		{name: "a receipt edited in one byte", list: edited, code: mediator.CodeJournal, index: 3, inner: receipts.CodeSignature},
		{name: "a release with no answer before it", list: append(append([][]byte(nil), list...), built(t, list, ask.Subject,
			receipts.Predicate{Accord: ask.Predicate.Accord, Card: ask.Predicate.Card, Kind: receipts.KindCrossing, From: ask.Predicate.From,
				To: ask.Predicate.To, At: "2026-11-12T10:50:00Z", Envelope: ask.Predicate.Envelope, Verdict: "pass", Reason: "ok"})),
			code: mediator.CodeJournal, index: len(list), inner: receipts.CodeRelease},
		{name: "a delivery for a party the receipt queued nothing for", queued: append(append([]mediator.Queued(nil), queued[:len(queued)-1]...),
			mediator.Queued{To: key(t, "proposer").Reference(), Delivery: released.Delivery}), code: mediator.CodeHeld, index: len(queued) - 1},
		{name: "a delivery of a receipt that queued nothing", queued: append(append([]mediator.Queued(nil), queued[:len(queued)-1]...),
			mediator.Queued{To: released.To, Delivery: withMembers(t, released.Delivery, map[string]any{
				"receiptId": b.accepted, "receipt": json.RawMessage(list[2])})}), code: mediator.CodeHeld, index: len(queued) - 1},
		{name: "a delivery that carries another receipt", queued: append(append([]mediator.Queued(nil), queued[:len(queued)-1]...),
			mediator.Queued{To: released.To, Delivery: withMembers(t, released.Delivery, map[string]any{
				"receipt": json.RawMessage(list[0])})}), code: mediator.CodeHeld, index: len(queued) - 1},
		{name: "a delivery given twice", queued: append(append([]mediator.Queued(nil), queued...), released), code: mediator.CodeHeld, index: len(queued)},
		{name: "a delivery whose body misses its digest", queued: append(append([]mediator.Queued(nil), queued[:len(queued)-1]...),
			mediator.Queued{To: released.To, Delivery: withText(t, released.Delivery, "Another body.")}), code: mediator.CodeHeld, index: len(queued) - 1},
		{name: "a delivery not in canonical form", queued: []mediator.Queued{{To: released.To, Delivery: append([]byte(" "), released.Delivery...)}},
			code: mediator.CodeHeld, index: 0},
		{name: "an open question no entry names", open: b.open[1:], code: mediator.CodeQuestion, index: -1},
		{name: "an open entry for a closed question", open: append(append([]mediator.Open(nil), b.open...),
			mediator.Open{Question: b.accepted, Deadline: "2026-11-12T10:00:00Z", Roles: []string{"decider"}}), code: mediator.CodeQuestion, index: 2},
		{name: "an open question given twice", open: append(append([]mediator.Open(nil), b.open...), b.open[0]), code: mediator.CodeQuestion, index: 2},
		{name: "a malformed deadline", open: []mediator.Open{b.open[0], {Question: b.second, Deadline: "2026-11-12T11:30:00+00:00", Roles: []string{"decider"}}},
			code: mediator.CodeAt, index: 1},
	}
	for _, c := range cases {
		s, k, l, o, q, at := set, med, list, b.open, queued, "2026-11-12T11:00:00Z"
		if c.set != nil {
			s = c.set
		}
		if c.key != nil {
			k = *c.key
		}
		if c.list != nil {
			l = c.list
		}
		if c.open != nil {
			o = c.open
		}
		if c.queued != nil {
			q = c.queued
		}
		if c.at != "" {
			at = c.at
		}
		r, outs, err := mediator.Reload(s, foundedAt, k, l, o, q, at)
		if r != nil || outs != nil {
			t.Fatalf("%s: a mediator was returned", c.name)
		}
		if c.other != nil {
			var se *setup.Error
			if !errors.Is(err, c.other) && !(errors.As(err, &se) && se.Code == setup.NotCanonical) {
				t.Fatalf("%s: got %v, want %v", c.name, err, c.other)
			}
			continue
		}
		var e *mediator.Error
		if !errors.As(err, &e) || e.Code != c.code || e.Index != c.index {
			t.Fatalf("%s: got %#v, want %s at %d", c.name, err, c.code, c.index)
		}
		var re *receipts.Error
		switch {
		case c.inner != "" && (!errors.As(err, &re) || re.Code != c.inner || re.Index != c.index):
			t.Fatalf("%s: the wrapped error is %v, want %s at %d", c.name, errors.Unwrap(err), c.inner, c.index)
		case c.inner == "" && errors.Unwrap(err) != nil:
			t.Fatalf("%s: an error that wraps %v", c.name, errors.Unwrap(err))
		}
	}
}

// TestReloadDecidesNothingAgain is one-decision-module on reload: the recorded deadline, 09:30, and
// roles, applier and auditor, are not what the proposal's term gives, and a reload after 09:30, or
// at it, writes the timeout at 09:30 from the applier, the first member holding a recorded role; a
// reload before it, with only auditor recorded, which nobody holds, is undeliverable from the
// mediator.
func TestReloadDecidesNothingAgain(t *testing.T) {
	t.Parallel()
	m := found(t)
	q := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	set, foundedAt := askAndLog(t)
	open := []mediator.Open{{Question: q, Deadline: "2026-11-12T09:30:00Z", Roles: []string{"applier", "auditor"}}}
	r, outs, err := mediator.Reload(set, foundedAt, key(t, "mediator"), m.Receipts(), open, nil, "2026-11-12T09:45:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonTimeout {
		t.Fatalf("got %+v, %v", outs, err)
	}
	st := statement(t, r, outs[0].ReceiptID)
	if st.Predicate.At != "2026-11-12T09:30:00Z" || st.Predicate.From != key(t, "applier").Reference() {
		t.Fatalf("the timeout: %+v", st.Predicate)
	}
	r, outs, err = mediator.Reload(set, foundedAt, key(t, "mediator"), m.Receipts(), open, nil, "2026-11-12T09:30:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonTimeout {
		t.Fatalf("a reload at the deadline: %+v, %v", outs, err)
	}
	if st := statement(t, r, outs[0].ReceiptID); st.Predicate.At != "2026-11-12T09:30:00Z" {
		t.Fatalf("the timeout at the deadline: %+v", st.Predicate)
	}
	open[0].Roles = []string{"auditor"}
	r, outs, err = mediator.Reload(set, foundedAt, key(t, "mediator"), m.Receipts(), open, nil, "2026-11-12T09:15:00Z")
	if err != nil || len(outs) != 1 || outs[0].Reason != receipts.ReasonUndeliverable {
		t.Fatalf("got %+v, %v", outs, err)
	}
	if st := statement(t, r, outs[0].ReceiptID); st.Predicate.At != "2026-11-12T09:15:00Z" || st.Predicate.From != key(t, "mediator").Reference() {
		t.Fatalf("the undeliverable: %+v", st.Predicate)
	}
}

// TestTwoMediatorsOneRecordThroughAReload is receipt-10 across a stop: two mediators founded alike,
// each stopped and reloaded at one time, then given one more note, a proposal changed under one salt
// and released, and a proposal left to time out, hold byte-identical receipts and deliveries.
func TestTwoMediatorsOneRecordThroughAReload(t *testing.T) {
	t.Parallel()
	run := func() ([][]byte, []mediator.Outcome, *mediator.Mediator) {
		b := record(t)
		r, outs := reload(t, b, "2026-11-12T11:00:00Z")
		give(t, r, "proposer", "applier", 6, "n-6", "note", "2026-11-12T11:01:00Z")
		q := ask(t, r, 7, "p-7", "2026-11-12T11:02:00Z")
		if _, err := r.Resolve(key(t, "decider").Reference(), q, change(true), "2026-11-12T11:03:00Z"); err != nil {
			t.Fatal(err)
		}
		ask(t, r, 8, "p-8", "2026-11-12T11:04:00Z")
		if ticks, err := r.Tick("2026-11-12T12:04:00Z"); err != nil || len(ticks) != 1 {
			t.Fatalf("the tick: %+v, %v", ticks, err)
		}
		return r.Receipts(), outs, r
	}
	ra, oa, ma := run()
	rb, ob, mb := run()
	if !reflect.DeepEqual(ra, rb) || !reflect.DeepEqual(oa, ob) {
		t.Fatal("the two records differ")
	}
	for _, n := range []string{"proposer", "applier", "decider"} {
		for {
			da, _ := ma.Get(key(t, n).Reference())
			db, _ := mb.Get(key(t, n).Reference())
			if !bytes.Equal(da, db) {
				t.Fatalf("%s's deliveries differ", n)
			}
			if da == nil {
				break
			}
		}
	}
	if _, err := receipts.Verify(key(t, "mediator").Reference(), ra); err != nil {
		t.Fatal(err)
	}
}
