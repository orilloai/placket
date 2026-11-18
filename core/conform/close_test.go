package conform

import (
	"strings"
	"testing"

	"github.com/orilloai/placket/core/founding"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// closedJournal is a journal of the ask-and-log Accord with a proposal asked at 09:00 and closed by
// a close at the time at, and its founding set and Accord id.
func closedJournal(t *testing.T, at string) ([][]byte, founding.Inputs, string, mediator.Closed) {
	t.Helper()
	set, foundedAt := ptesting.Founding(t, "ask-and-log")
	key := ptesting.Key(t, "mediator")
	m, err := mediator.Found(set, key, foundedAt)
	if err != nil {
		t.Fatal(err)
	}
	p := ptesting.Key(t, "proposer")
	prop := message(t, m.Accord(), "p-1", "proposal", body)
	ask, err := m.Give(p.Reference(), prop, envelope(t, 1, prop, p), nil, "2026-11-12T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	q, _, err := m.Question(key.Reference(), ask.ReceiptID)
	if err != nil {
		t.Fatal(err)
	}
	c, _, closed, err := mediator.Close(set, m.Receipts(), []mediator.Open{{Question: q.ID, Deadline: q.Deadline, Roles: q.Roles}}, nil, foundedAt, key, at)
	if err != nil {
		t.Fatal(err)
	}
	return c.Receipts(), set, m.Accord(), closed
}

// TestClosedClosingConforms is U15 (verify-2 rows 2 and 3, close-4): a closed closing from the first
// answerer before its question's deadline conforms, and its decision is the mediator's, by the close,
// with no witness; one whose deadline is at or before its at, or from another member, is decision.
// The close goes through row 1 alone, a close of another Accord failing accord at its index; it is
// no decision, and the report names it. A close that is the first receipt founds the set at its at.
func TestClosedClosingConforms(t *testing.T) {
	t.Parallel()
	kernel := func(in founding.Inputs) string { return kernelOf(in.Set) }
	list, set, accord, closed := closedJournal(t, "2026-11-12T09:30:00Z")
	r, err := Check(Evidence{Accord: accord, Founding: set.Set, Openings: set.Openings, Acceptances: set.Acceptances, Receipts: list, Kernel: kernel(set)})
	if err != nil {
		t.Fatalf("a closed journal: %v", err)
	}
	if r.Close != closed.ID || len(r.Decisions) != 2 {
		t.Fatalf("report %+v; want the close %s and two decisions", r, closed.ID)
	}
	if d := r.Decisions[1]; d.Index != 1 || d.By != (receipts.By{Decider: receipts.ByMediator, Event: receipts.EventClose}) || d.Witness != "" || !d.Checked {
		t.Fatalf("the closed closing's decision %+v", d)
	}
	if r, err := Check(Evidence{Accord: accord, Mediator: ptesting.Key(t, "mediator").Reference(), Receipts: list}); err != nil || r.Close != closed.ID || len(r.Decisions) != 2 {
		t.Fatalf("without a founding set: %+v, %v", r, err)
	}

	// The same question closed after its deadline, 10:00: written by hand, since Close writes a
	// timeout there, with its close at the same time.
	ask, err := receipts.Decode(list[0])
	if err != nil {
		t.Fatal(err)
	}
	decider, proposer := ptesting.Key(t, "decider").Reference(), ptesting.Key(t, "proposer").Reference()
	for name, c := range map[string]struct{ from, at string }{
		"a closing at its deadline":     {decider, "2026-11-12T10:00:00Z"},
		"a closing after its deadline":  {decider, "2026-11-12T10:30:00Z"},
		"a closing from another member": {proposer, "2026-11-12T09:30:00Z"},
	} {
		s := newSigner(t)
		s.list = append(s.list, list[0])
		s.chains[[2]string{proposer, ask.Predicate.To}], s.ids[[2]string{proposer, ask.Predicate.To}] = 1, ask.ID
		id := s.add(t, ask.Subject, receipts.Predicate{Accord: accord, Card: ask.Predicate.Card, Kind: receipts.KindResolution,
			From: c.from, To: proposer, At: c.at, Envelope: ask.Predicate.Envelope, Verdict: "block", Reason: receipts.ReasonClosed,
			By: receipts.By{Decider: receipts.ByMediator, Event: receipts.EventClose}})
		finals := []receipts.Final{{From: proposer, To: ask.Predicate.To, Seq: 1, Receipt: ask.ID}, {From: c.from, To: proposer, Seq: 1, Receipt: id}}
		if c.from == proposer {
			finals[1] = receipts.Final{From: proposer, To: proposer, Seq: 1, Receipt: id}
		}
		r, _, err := receipts.BuildClose(receipts.Close{Accord: accord, Card: ask.Predicate.Card, At: c.at, Chains: sortFinals(finals)}, s.key)
		if err != nil {
			t.Fatal(err)
		}
		l := append(s.list, r)
		if _, err := Check(Evidence{Accord: accord, Founding: set.Set, Openings: set.Openings, Acceptances: set.Acceptances, Receipts: l, Kernel: kernel(set)}); !isCode(err, CodeDecision, 1) {
			t.Fatalf("%s: %v, want decision at index 1", name, err)
		}
	}

	// A close of another Accord, the only receipt.
	other := "sha256:" + strings.Repeat("ab", 32)
	cl, _, err := receipts.BuildClose(receipts.Close{Accord: other, Card: ask.Predicate.Card, At: "2026-11-12T09:30:00Z"}, ptesting.Key(t, "mediator"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Check(Evidence{Accord: accord, Founding: set.Set, Openings: set.Openings, Acceptances: set.Acceptances, Receipts: [][]byte{cl}, Kernel: kernel(set)}); !isCode(err, CodeAccord, 0) {
		t.Fatalf("a close of another Accord: %v, want accord at 0", err)
	}
	// The close of an Accord with no receipts: the set founds at the close's time.
	_, _, empty, err := mediator.Close(set, nil, nil, nil, "2026-11-12T08:00:00Z", ptesting.Key(t, "mediator"), "2026-11-12T09:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if r, err := Check(Evidence{Accord: accord, Founding: set.Set, Openings: set.Openings, Acceptances: set.Acceptances, Receipts: [][]byte{empty.Receipt}, Kernel: kernel(set)}); err != nil || r.Close != empty.ID || len(r.Decisions) != 0 {
		t.Fatalf("the close of an empty Accord: %+v, %v", r, err)
	}
}

// sortFinals orders chains by pair, as a close lists them; the references here are ASCII, so byte
// order is actor-reference order.
func sortFinals(l []receipts.Final) []receipts.Final {
	out := append([]receipts.Final(nil), l...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].From < out[i].From || out[j].From == out[i].From && out[j].To < out[i].To {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
