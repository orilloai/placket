package receipts_test

import (
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// TestReconcileReadsPartiesInReferenceOrder gives Reconcile the kept records in descending order of
// the parties' references, and each party's envelopes in descending order of seq: the first party
// whose kept receipt fails is the first in actor-reference order, and every list of the report names
// the parties in that order, each party's missing envelopes in ascending order of seq, once (section
// 9, receipt-7).
func TestReconcileReadsPartiesInReferenceOrder(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	sender := key(t, "proposer").Reference()
	parties := []string{key(t, "applier").Reference(), key(t, "decider").Reference(), key(t, "stranger").Reference()}
	sort.Slice(parties, func(i, j int) bool { return canonical.Less(parties[i], parties[j]) })
	first, second := parties[0], parties[1]
	give := func(seq int64, to string) entry {
		return entry{receipts.Subject{Name: "note", Digest: digestOf(byte(seq))},
			predicateOf(receipts.KindCrossing, sender, to, "pass", "ok", receipts.Envelope{Sender: sender, Seq: seq, Hash: mhash})}
	}
	list := journal(t, med, []entry{give(1, first), give(2, second)})

	t.Run("the first failing party", func(t *testing.T) {
		t.Parallel()
		// Each party keeps the receipt addressed to the other, which fails kept at its index 0.
		_, err := receipts.Reconcile(med.Reference(), accord, card, list, []receipts.Kept{
			{Actor: second, Delivered: [][]byte{list[0]}},
			{Actor: first, Delivered: [][]byte{list[1]}},
		})
		var re *receipts.Error
		if !errors.As(err, &re) || *re != (receipts.Error{Code: receipts.CodeKept, Index: 0, Actor: first}) {
			t.Fatalf("got %v, want kept at index 0 of the first party in reference order", err)
		}
	})

	t.Run("the report's lists", func(t *testing.T) {
		t.Parallel()
		ids := []string{}
		for _, r := range list {
			s, err := receipts.Decode(r)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, s.ID)
		}
		other := func(seq int64) receipts.Envelope { return receipts.Envelope{Seq: seq, Hash: mhash} }
		kept := []receipts.Kept{
			{Actor: second, Delivered: [][]byte{list[1]}, Envelopes: []receipts.Envelope{other(9), other(8), other(9)}},
			{Actor: first, Delivered: [][]byte{list[0]}, Envelopes: []receipts.Envelope{other(7), other(3)}},
		}
		// Presented empty: every kept receipt is removed and nothing is echoed.
		rep, err := receipts.Reconcile(med.Reference(), accord, card, nil, kept)
		want := receipts.Report{
			Removed: []receipts.Item{{Actor: first, ReceiptID: ids[0]}, {Actor: second, ReceiptID: ids[1]}},
			Missing: []receipts.Missed{{Actor: first, Seq: 3}, {Actor: first, Seq: 7}, {Actor: second, Seq: 8}, {Actor: second, Seq: 9}},
		}
		if err != nil || !reflect.DeepEqual(rep, want) {
			t.Fatalf("got %+v, %v\nwant %+v", rep, err, want)
		}
		// Presented in full, kept with nothing delivered: each pass is unrecorded for its party.
		kept[0].Delivered, kept[1].Delivered = nil, nil
		rep, err = receipts.Reconcile(med.Reference(), accord, card, list, kept)
		if err != nil || !reflect.DeepEqual(rep.Unrecorded, []receipts.Item{{Actor: first, ReceiptID: ids[0]}, {Actor: second, ReceiptID: ids[1]}}) {
			t.Fatalf("unrecorded: got %+v, %v", rep.Unrecorded, err)
		}
	})
}
