package mediator

import (
	"sort"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// Closed is an Accord's close receipt: its id and its bytes (close-2, record 0022).
type Closed struct {
	ID      string
	Receipt []byte
}

// Closed returns a copy of the close receipt of a closed Accord, and true; false while the Accord is
// open (close-3, results-are-copies).
func (m *Mediator) Closed() (Closed, bool) {
	if !m.founded() || m.closed == nil {
		return Closed{}, false
	}
	return Closed{ID: m.closed.ID, Receipt: append([]byte(nil), m.closed.Receipt...)}, true
}

// Close closes an Accord from the record a journal keeps (close-1, record 0022). It takes Reload's
// record and checks it in Reload's order, with Reload's codes; a record that already ends in its
// close is closed. Then it writes, in one batch built before any state changes: first a timeout for
// each open question whose deadline is at or before at, at its deadline, as Tick would, in the order
// the questions were opened (maybe-5); then a closed closing for each question still open, in the
// order they were opened, from its first answerer, or the mediator when it has none, to its asker,
// block, closed, naming the asked part, echoing its envelope and naming the mediator, with the event
// close, as its decider; and last the close receipt, which lists the last receipt of every chain,
// ordered by pair, and every delivery not yet fetched, in the order queued (close-2). It never waits
// for a fetch.
//
// A record cut inside a close, its closed closings with no close after them, is completed at the
// time of the closings already written, not at at: every question still open is closed at that time,
// and the close receipt written at it. It returns the closed mediator, the closings' outcomes in
// order, and the close.
func Close(in founding.Inputs, list [][]byte, open []Open, deliveries []Queued, foundedAt string, key signing.Key, at string) (*Mediator, []Outcome, Closed, error) {
	r, err := rebuild(in, list, open, deliveries, foundedAt, key, at)
	if err != nil {
		return nil, nil, Closed{}, err
	}
	m := r.m
	if m.closed != nil {
		return nil, nil, Closed{}, failure(CodeClosed)
	}
	when := at
	if r.torn != "" {
		when = r.torn
	}
	b := m.batch()
	var still []*question
	for _, qi := range r.order {
		q := r.question(qi)
		if q.deadline <= when {
			if err := b.close(q, m.firstAnswerer(q.roles), receipts.ReasonTimeout, q.deadline, byDeadline); err != nil {
				return nil, nil, Closed{}, err
			}
			continue
		}
		still = append(still, q)
	}
	for _, q := range still {
		if err := b.close(q, m.firstAnswerer(q.roles), receipts.ReasonClosed, when, byClose); err != nil {
			return nil, nil, Closed{}, err
		}
	}

	chains := map[[2]string]link{}
	for pair, l := range m.chains {
		chains[pair] = l
	}
	for pair, l := range b.chains {
		chains[pair] = l
	}
	c := receipts.Close{Accord: m.accord.ID(), Card: m.accord.Card(), At: when, Chains: finals(chains), Unfetched: m.unfetched()}
	receipt, verified, err := receipts.BuildClose(c, key)
	if err != nil {
		return nil, nil, Closed{}, failure(CodeInternal)
	}
	cl, _ := verified.Close()
	m.commit(b)
	m.receipts = append(m.receipts, receipt)
	m.statements = append(m.statements, verified)
	m.closed = &Closed{ID: cl.ID, Receipt: append([]byte(nil), receipt...)}
	out, _ := m.Closed()
	return m, b.outs, out, nil
}

// finals is the last receipt of each chain, ordered by pair: by from, then by to, in actor-reference
// order (close-2).
func finals(chains map[[2]string]link) []receipts.Final {
	out := make([]receipts.Final, 0, len(chains))
	for pair, l := range chains {
		out = append(out, receipts.Final{From: pair[0], To: pair[1], Seq: l.seq, Receipt: l.id})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return canonical.Less(out[i].From, out[j].From)
		}
		return canonical.Less(out[i].To, out[j].To)
	})
	return out
}

// unfetched is every delivery not yet fetched, by its receiver and the id of the receipt that queued
// it, in the order queued: the order of those receipts (close-2, crossing-3).
func (m *Mediator) unfetched() []receipts.Unfetched {
	index := map[string]int{}
	for i, v := range m.statements {
		if id := v.Statement().ID; id != "" {
			index[id] = i
		}
	}
	out := []receipts.Unfetched{}
	for to, l := range m.queues {
		for _, x := range l {
			out = append(out, receipts.Unfetched{To: to, Receipt: x.id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return index[out[i].Receipt] < index[out[j].Receipt] })
	return out
}
