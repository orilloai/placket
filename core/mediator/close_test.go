package mediator_test

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// closeOf closes the record of m, with its open questions open as given, at the time at, its
// deliveries in the order queued.
func closeOf(t *testing.T, m *mediator.Mediator, open []mediator.Open, at string) (*mediator.Mediator, []mediator.Outcome, mediator.Closed) {
	t.Helper()
	set, foundedAt := askAndLog(t)
	c, outs, closed, err := mediator.Close(set, m.Receipts(), open, queuedOf(m), foundedAt, ptesting.Key(t, "mediator"), at)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	return c, outs, closed
}

// closeOfReceipt reads a close receipt without verifying it.
func closeOfReceipt(t *testing.T, r []byte) receipts.Close {
	t.Helper()
	c, err := receipts.DecodeClose(r)
	if err != nil {
		t.Fatalf("DecodeClose: %v", err)
	}
	return c
}

// TestCloseClosesOpenQuestionsFirst is U1 and U2 (close-1): three questions open, opened Q1, Q2 and
// Q3, of which only Q2's deadline is at or before the close's time. The close writes Q2's timeout at
// its deadline, by the deadline, then a closed closing of Q1 and one of Q3, at the close's time, by
// the close, each from the first answerer to the asker, of the asked part and echoing its envelope;
// and it builds the close receipt last. Every receipt verifies.
func TestCloseClosesOpenQuestionsFirst(t *testing.T) {
	t.Parallel()
	m := found(t)
	q1 := ask(t, m, 1, "p-1", "2026-11-12T09:00:00Z")
	q2 := ask(t, m, 2, "p-2", "2026-11-12T09:10:00Z")
	q3 := ask(t, m, 3, "p-3", "2026-11-12T09:20:00Z")
	// Deadlines are per rule: Q2's rule gave it the earliest.
	open := []mediator.Open{
		{Question: q1, Deadline: "2026-11-12T11:30:00Z", Roles: []string{"decider"}},
		{Question: q2, Deadline: "2026-11-12T10:40:00Z", Roles: []string{"decider"}},
		{Question: q3, Deadline: "2026-11-12T11:30:00Z", Roles: []string{"decider"}},
	}
	const at = "2026-11-12T11:00:00Z"
	c, outs, closed := closeOf(t, m, open, at)
	if len(outs) != 3 {
		t.Fatalf("%d outcomes, want 3", len(outs))
	}
	asker, decider := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "decider").Reference()
	want := []struct {
		question, reason, at string
		by                   receipts.By
	}{
		{q2, receipts.ReasonTimeout, "2026-11-12T10:40:00Z", receipts.By{Decider: receipts.ByMediator, Event: receipts.EventDeadline}},
		{q1, receipts.ReasonClosed, at, receipts.By{Decider: receipts.ByMediator, Event: receipts.EventClose}},
		{q3, receipts.ReasonClosed, at, receipts.By{Decider: receipts.ByMediator, Event: receipts.EventClose}},
	}
	for i, w := range want {
		o := outs[i]
		s := statement(t, c, o.ReceiptID)
		ask := statement(t, m, w.question)
		p := s.Predicate
		if o.Verdict != "block" || o.Reason != w.reason || p.At != w.at || p.By != w.by || p.Kind != receipts.KindResolution ||
			p.From != decider || p.To != asker || s.Subject != ask.Subject || p.Envelope != ask.Predicate.Envelope {
			t.Fatalf("outcome %d: %+v, %+v; want %s of %s at %s", i, o, s, w.reason, w.question, w.at)
		}
	}
	list := c.Receipts()
	if len(list) != 7 || !bytes.Equal(list[6], closed.Receipt) {
		t.Fatalf("%d receipts, the last not the close", len(list))
	}
	for i, o := range outs {
		if !bytes.Equal(list[3+i], o.Receipt) {
			t.Fatalf("receipt %d is not outcome %d", 3+i, i)
		}
	}
	cl := closeOfReceipt(t, closed.Receipt)
	if cl.ID != closed.ID || cl.At != at || cl.Accord != m.Accord() || cl.Card != m.Card() {
		t.Fatalf("the close %+v", cl)
	}
	if got, ok := c.Closed(); !ok || !reflect.DeepEqual(got, closed) {
		t.Fatalf("Closed: %+v, %v", got, ok)
	}
	if p := c.Pending(); len(p) != 0 {
		t.Fatalf("pending %v after the close", p)
	}
	if _, err := receipts.Verify(ptesting.Key(t, "mediator").Reference(), list); err != nil {
		t.Fatalf("the closed list does not verify: %v", err)
	}
	if _, ok := m.Closed(); ok {
		t.Fatal("the mediator the record came from is closed")
	}
}

// TestCloseListsChainsAndUnfetched is U4 and U5 (close-2): the close lists every chain once, at its
// last receipt, ordered by from and then by to, and every delivery not yet fetched, by its receiver
// and the receipt that queued it, in the order queued, the one fetched left out. It is written at
// once: nothing waits for a fetch.
func TestCloseListsChainsAndUnfetched(t *testing.T) {
	t.Parallel()
	b := record(t)
	c, _, closed := closeOf(t, b.m, b.open, "2026-11-12T11:00:00Z")
	cl := closeOfReceipt(t, closed.Receipt)

	last := map[[2]string]receipts.Final{}
	list := c.Receipts()
	for _, r := range list[:len(list)-1] {
		s, err := receipts.Decode(r)
		if err != nil {
			t.Fatal(err)
		}
		p := s.Predicate
		last[[2]string{p.From, p.To}] = receipts.Final{From: p.From, To: p.To, Seq: p.Seq, Receipt: s.ID}
	}
	if len(cl.Chains) != len(last) || len(last) != 2 {
		t.Fatalf("%d chains, want %d, two", len(cl.Chains), len(last))
	}
	for i, f := range cl.Chains {
		if last[[2]string{f.From, f.To}] != f {
			t.Fatalf("chain %d: %+v, want %+v", i, f, last[[2]string{f.From, f.To}])
		}
		if i > 0 {
			p := cl.Chains[i-1]
			if !canonical.Less(p.From, f.From) && (p.From != f.From || !canonical.Less(p.To, f.To)) {
				t.Fatalf("chains out of pair order: %+v before %+v", p, f)
			}
		}
	}
	var want []receipts.Unfetched
	for _, r := range list {
		if s, err := receipts.Decode(r); err == nil {
			if q, ok := b.m.Queued(s.ID); ok {
				want = append(want, receipts.Unfetched{To: q.To, Receipt: s.ID})
			}
		}
	}
	applier := ptesting.Key(t, "applier").Reference()
	if !reflect.DeepEqual(cl.Unfetched, want) || len(want) != 4 || want[2].To != applier {
		t.Fatalf("unfetched %+v, want %+v, four, the third the release to the applier", cl.Unfetched, want)
	}
	note, err := receipts.Decode(list[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range cl.Unfetched {
		if u.Receipt == note.ID {
			t.Fatal("the fetched note is listed")
		}
	}
}

// TestClosedRefusesGiveAndResolve is U6's core (close-3): on a closed Accord a member's give and
// answer are rejected with closed, right after membership, and a stranger's with not-a-member;
// nothing is written. The passing of time and the finding that nobody can answer write nothing,
// no question is open, and a closed question's result still reads.
func TestClosedRefusesGiveAndResolve(t *testing.T) {
	t.Parallel()
	b := record(t)
	c, _, _ := closeOf(t, b.m, b.open, "2026-11-12T10:35:00Z")
	before := c.Receipts()
	p, ap, stranger := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier"), ptesting.Key(t, "stranger")
	msg := message(t, c.Accord(), "n-9", ap.Reference(), "note", "After the close.", nil)
	if _, err := c.Give(p.Reference(), msg, envelope(t, 9, msg, p), nil, "2026-11-12T10:36:00Z"); !rejected(err, mediator.AccordClosed) {
		t.Fatalf("a member's give: %v, want closed", err)
	}
	if _, err := c.Give(p.Reference(), msg, []byte("not an envelope"), nil, "2026-11-12T10:36:00Z"); !rejected(err, mediator.AccordClosed) {
		t.Fatalf("a member's give under a bad envelope: %v, want closed", err)
	}
	if _, err := c.Give(stranger.Reference(), msg, envelope(t, 9, msg, stranger), nil, "2026-11-12T10:36:00Z"); !rejected(err, mediator.NotAMember) {
		t.Fatalf("a stranger's give: %v, want not-a-member", err)
	}
	d := ptesting.Key(t, "decider").Reference()
	if _, err := c.Resolve(d, accept(t, c, d, b.first, false), "2026-11-12T10:36:00Z"); !rejected(err, mediator.AccordClosed) {
		t.Fatalf("an answer: %v, want closed", err)
	}
	if _, err := c.Resolve(d, mediator.Resolution{Call: []byte("{}")}, "2026-11-12T10:36:00Z"); !rejected(err, mediator.AccordClosed) {
		t.Fatalf("an answer whose call does not read: %v, want closed", err)
	}
	if _, err := c.Resolve(stranger.Reference(), accept(t, c, stranger.Reference(), b.first, false), "2026-11-12T10:36:00Z"); !rejected(err, mediator.NotAMember) {
		t.Fatalf("a stranger's answer: %v, want not-a-member", err)
	}
	if outs, err := c.Tick("2026-11-12T23:00:00Z"); err != nil || len(outs) != 0 {
		t.Fatalf("a tick: %+v, %v", outs, err)
	}
	if _, err := c.Undeliverable(b.first, "2026-11-12T10:36:00Z"); !rejected(err, mediator.NoOpenQuestion) {
		t.Fatalf("undeliverable: %v, want no-open-question", err)
	}
	unchanged(t, c, before, []string{})
	if outs, ok, err := c.Result(p.Reference(), b.accepted); err != nil || !ok || len(outs) != 2 || outs[0].ReceiptID != b.acceptOuts[0].ReceiptID {
		t.Fatalf("the result of a closed question: %+v, %v, %v", outs, ok, err)
	}
	if outs, ok, err := c.Result(p.Reference(), b.first); err != nil || !ok || len(outs) != 1 || outs[0].Reason != receipts.ReasonClosed {
		t.Fatalf("the result of a question the close closed: %+v, %v, %v", outs, ok, err)
	}
	if _, _, err := c.Question(d, b.first); err != nil {
		t.Fatalf("a question read on a closed Accord: %v", err)
	}
}

// TestClosedGetHandsOutThenClose is U7's core (close-3): on a closed Accord each member's fetch
// still hands out what was queued, in order, then nothing, and the close stays the same; nothing is
// written.
func TestClosedGetHandsOutThenClose(t *testing.T) {
	t.Parallel()
	b := record(t)
	c, _, closed := closeOf(t, b.m, b.open, "2026-11-12T11:00:00Z")
	before := c.Receipts()
	applier := ptesting.Key(t, "applier").Reference()
	want, ok := b.m.Queued(b.acceptOuts[1].ReceiptID)
	if !ok {
		t.Fatal("the release was not queued")
	}
	d, err := c.Get(applier)
	if err != nil || !bytes.Equal(d, want.Delivery) {
		t.Fatalf("the first fetch: %s, %v", d, err)
	}
	for i := 0; i < 2; i++ {
		if d, err := c.Get(applier); err != nil || d != nil {
			t.Fatalf("fetch %d: %s, %v; want nothing", i+2, d, err)
		}
		if got, ok := c.Closed(); !ok || !reflect.DeepEqual(got, closed) {
			t.Fatalf("Closed after fetch %d: %+v, %v", i+2, got, ok)
		}
	}
	if _, err := c.Get(ptesting.Key(t, "stranger").Reference()); !rejected(err, mediator.NotAMember) {
		t.Fatalf("a stranger's fetch: %v, want not-a-member", err)
	}
	unchanged(t, c, before, []string{})
	got, _ := c.Closed()
	got.Receipt[0] = 'x'
	if again, _ := c.Closed(); again.Receipt[0] == 'x' {
		t.Fatal("Closed shares its bytes with the mediator")
	}
}

// TestReloadOfClosedRecordWritesNothing is U8's core (close-3): a record that ends in its close
// reloads as a closed Accord, writing nothing, and refuses a give with closed; closing it again is
// closed, and nothing reopens it.
func TestReloadOfClosedRecordWritesNothing(t *testing.T) {
	t.Parallel()
	b := record(t)
	c, _, closed := closeOf(t, b.m, b.open, "2026-11-12T11:00:00Z")
	set, foundedAt := askAndLog(t)
	med := ptesting.Key(t, "mediator")
	r, outs, err := mediator.Reload(set, c.Receipts(), nil, queuedOf(c), foundedAt, med, "2026-11-13T09:00:00Z")
	if err != nil || len(outs) != 0 {
		t.Fatalf("Reload of a closed record: %+v, %v", outs, err)
	}
	if got, ok := r.Closed(); !ok || !reflect.DeepEqual(got, closed) || !reflect.DeepEqual(r.Receipts(), c.Receipts()) {
		t.Fatalf("the reloaded close: %+v, %v", got, ok)
	}
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	msg := message(t, r.Accord(), "n-9", ap.Reference(), "note", "After the close.", nil)
	if _, err := r.Give(p.Reference(), msg, envelope(t, 9, msg, p), nil, "2026-11-13T09:00:00Z"); !rejected(err, mediator.AccordClosed) {
		t.Fatalf("a give after the reload: %v, want closed", err)
	}
	if _, _, _, err := mediator.Close(set, c.Receipts(), nil, queuedOf(c), foundedAt, med, "2026-11-13T09:00:00Z"); !errorCode(err, mediator.CodeClosed) {
		t.Fatalf("a second close: %v, want closed", err)
	}
	// Open entries for questions the close closed are no entries of an open question.
	if _, _, err := mediator.Reload(set, c.Receipts(), b.open, queuedOf(c), foundedAt, med, "2026-11-13T09:00:00Z"); !errorCode(err, mediator.CodeQuestion) {
		t.Fatalf("a closed record with open entries: %v, want question", err)
	}
	// Close checks the record as Reload does, with its codes.
	if _, _, _, err := mediator.Close(set, b.m.Receipts(), nil, queuedOf(b.m), foundedAt, med, "2026-11-13T09:00:00Z"); !errorCode(err, mediator.CodeQuestion) {
		t.Fatalf("a record with no entry for its open questions: %v, want question", err)
	}
	if _, _, _, err := mediator.Close(set, b.m.Receipts(), b.open, queuedOf(b.m), foundedAt, med, "2026-11-13T25:00:00Z"); !errorCode(err, mediator.CodeAt) {
		t.Fatalf("a time the timestamp rule refuses: %v, want at", err)
	}
}

// TestCloseAfterTornClose is U14's core (close-1): a record cut after its closed closings and before
// the close is refused by Reload with close, and Close completes it at the closings' time, writing
// the close receipt alone; a record cut between two closed closings is completed with the closing
// still to write and then the close, both at the time of the closing already written. Each completed
// close is the one the whole write would have made, byte for byte.
func TestCloseAfterTornClose(t *testing.T) {
	t.Parallel()
	b := record(t)
	const at = "2026-11-12T10:35:00Z"
	c, outs, closed := closeOf(t, b.m, b.open, at)
	if len(outs) != 2 || outs[0].Reason != receipts.ReasonClosed || outs[1].Reason != receipts.ReasonClosed {
		t.Fatalf("the whole close: %+v", outs)
	}
	full := c.Receipts()
	set, foundedAt := askAndLog(t)
	med := ptesting.Key(t, "mediator")
	const later = "2026-11-13T09:00:00Z"
	beforeClose := full[:len(full)-1]
	if _, _, err := mediator.Reload(set, beforeClose, nil, queuedOf(b.m), foundedAt, med, later); !errorCode(err, mediator.CodeClose) {
		t.Fatalf("Reload of a torn close: %v, want close", err)
	}
	r, rest, got, err := mediator.Close(set, beforeClose, nil, queuedOf(b.m), foundedAt, med, later)
	if err != nil || len(rest) != 0 || !reflect.DeepEqual(got, closed) || !reflect.DeepEqual(r.Receipts(), full) {
		t.Fatalf("completing a close cut before its receipt: %+v, %+v, %v", rest, got, err)
	}
	between := full[:len(full)-2]
	open := []mediator.Open{b.open[1]}
	if _, _, err := mediator.Reload(set, between, open, queuedOf(b.m), foundedAt, med, later); !errorCode(err, mediator.CodeClose) {
		t.Fatalf("Reload of a close cut between its closings: %v, want close", err)
	}
	r, rest, got, err = mediator.Close(set, between, open, queuedOf(b.m), foundedAt, med, later)
	if err != nil || len(rest) != 1 || !bytes.Equal(rest[0].Receipt, outs[1].Receipt) || !reflect.DeepEqual(got, closed) ||
		!reflect.DeepEqual(r.Receipts(), full) {
		t.Fatalf("completing a close cut between its closings: %+v, %+v, %v", rest, got, err)
	}
}

// TestCloseOfAnEmptyRecord closes an Accord with no receipts: the close is the first receipt, with
// both lists empty, and it verifies.
func TestCloseOfAnEmptyRecord(t *testing.T) {
	t.Parallel()
	set, foundedAt := askAndLog(t)
	med := ptesting.Key(t, "mediator")
	c, outs, closed, err := mediator.Close(set, nil, nil, nil, foundedAt, med, foundedAt)
	if err != nil || len(outs) != 0 {
		t.Fatalf("Close: %+v, %v", outs, err)
	}
	cl := closeOfReceipt(t, closed.Receipt)
	if len(cl.Chains) != 0 || len(cl.Unfetched) != 0 || len(c.Receipts()) != 1 {
		t.Fatalf("the close of an empty record: %+v", cl)
	}
	var e *mediator.Error
	if _, _, _, err := mediator.Close(founding.Inputs{Set: []byte("{}")}, nil, nil, nil, foundedAt, med, foundedAt); errors.As(err, &e) {
		t.Fatalf("a set that does not found: %v, want founding's error", err)
	}
}

// FuzzClose closes a record with one receipt replaced, its open entry and one queued delivery
// changed, at any time. It never panics, and a mediator it returns is closed, with receipts that
// verify and end in its close (core-no-panic).
func FuzzClose(f *testing.F) {
	m, q := opened(f)
	q2 := ask(f, m, 2, "p-2", "2026-11-12T09:02:00Z")
	list := m.Receipts()
	sealed, foundedAt := askAndLog(f)
	set := sealed.Set
	f.Add(set, foundedAt, uint8(9), list[0], q, "2026-11-12T10:00:00Z", q2, "decider", []byte(nil), "2026-11-12T09:30:00Z")
	f.Add(set, foundedAt, uint8(0), list[1], q, "2026-11-12T10:00:00Z", q2, "decider", []byte(`{}`), "2026-11-12T11:00:00Z")
	f.Add([]byte(`{}`), "", uint8(0), []byte(`{}`), "", "", "", "", []byte(nil), "")
	med := ptesting.Key(f, "mediator")
	f.Fuzz(func(t *testing.T, set []byte, foundedAt string, at uint8, receipt []byte, question, deadline, question2, role string, delivery []byte, closeAt string) {
		l := append([][]byte(nil), list...)
		if int(at) < len(l) {
			l[at] = receipt
		}
		open := []mediator.Open{{Question: question, Deadline: deadline, Roles: []string{role}},
			{Question: question2, Deadline: "2026-11-12T10:02:00Z", Roles: []string{role}}}
		var queued []mediator.Queued
		if delivery != nil {
			queued = []mediator.Queued{{To: ptesting.Key(t, "applier").Reference(), Delivery: delivery}}
		}
		in := sealed
		in.Set = set
		c, outs, closed, err := mediator.Close(in, l, open, queued, foundedAt, med, closeAt)
		if err != nil {
			if c != nil || outs != nil || closed.Receipt != nil {
				t.Fatal("a refused close returned a mediator")
			}
			return
		}
		got := c.Receipts()
		if _, err := receipts.Verify(med.Reference(), got); err != nil || !bytes.Equal(got[len(got)-1], closed.Receipt) {
			t.Fatalf("a closed mediator's receipts do not verify, or do not end in its close: %v", err)
		}
	})
}
