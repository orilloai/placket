package receipts_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// receiptSeeds are receipt bytes of each form a reader meets: a real receipt, one edited, one of
// another statement type, and bytes that are no receipt.
func receiptSeeds(f *testing.F) [][]byte {
	list, _ := chain(f, key(f, "mediator"), 2)
	return [][]byte{
		list[0], list[1],
		[]byte(`{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`),
		[]byte(`{"payload":"eyJhIjoxLCJhIjoyfQ==","payloadType":"application/vnd.in-toto+json","signatures":[]}`),
		[]byte(`{"payload":"!!","payloadType":"x","signatures":[]}`),
		[]byte(`[]`), []byte("\xff"), nil,
	}
}

// FuzzDecode reads any bytes as a receipt without verifying. It never panics; a refusal is an
// Error of index 0, and a statement read has a typed hash as its id. Show refuses
// exactly what Decode refuses, and otherwise says first that nothing was verified.
func FuzzDecode(f *testing.F) {
	for _, r := range receiptSeeds(f) {
		f.Add(r)
	}
	f.Fuzz(func(t *testing.T, receipt []byte) {
		st, err := receipts.Decode(receipt)
		lines, serr := receipts.Show(receipt)
		if err != nil {
			var re *receipts.Error
			if !errors.As(err, &re) || re.Index != 0 || serr == nil || serr.Error() != err.Error() || lines != nil {
				t.Fatalf("Decode: %v; Show: %v, %q", err, serr, lines)
			}
			return
		}
		if serr != nil || len(lines) < 8 || lines[0] != receipts.NotVerified || lines[1] != "receipt "+st.ID {
			t.Fatalf("Show: %q, %v", lines, serr)
		}
		if h, ok := strings.CutPrefix(st.ID, canonical.HashPrefix); !ok || len(h) != 64 || strings.ToLower(h) != h {
			t.Fatalf("an id %q", st.ID)
		}
	})
}

// FuzzReconcile reconciles any presented receipt, after a valid one, against any kept receipt and
// envelope, under any mediator, Accord and card. It never panics; a refusal is an Error, and a report
// names only parties that kept a record.
func FuzzReconcile(f *testing.F) {
	med := key(f, "mediator")
	list, _ := chain(f, med, 2)
	party := key(f, "applier").Reference()
	f.Add(med.Reference(), accord, card, list[1], list[1], party, int64(2), mhash)
	f.Add(med.Reference(), accord, card, list[1], list[0], party, int64(1), mhash)
	f.Add("mediator", "", "", []byte(`{}`), []byte("\xff"), "", int64(-1), "")
	f.Fuzz(func(t *testing.T, mediator, acc, crd string, presented, kept []byte, actor string, seq int64, hash string) {
		report, err := receipts.Reconcile(mediator, acc, crd, [][]byte{list[0], presented},
			[]receipts.Kept{{Actor: actor, Delivered: [][]byte{kept}, Envelopes: []receipts.Envelope{{Seq: seq, Hash: hash}}}})
		if err != nil {
			var re *receipts.Error
			if !errors.As(err, &re) {
				t.Fatalf("a refusal that is not an Error: %v", err)
			}
			return
		}
		for _, it := range append(report.Removed, report.Unrecorded...) {
			if it.Actor != actor {
				t.Fatalf("a report names %q", it.Actor)
			}
		}
		for _, m := range report.Missing {
			if m.Actor != actor || m.Seq != seq {
				t.Fatalf("a report misses %+v", m)
			}
		}
	})
}

// FuzzDigest digests any salt and body. It never panics, and the digest is the lowercase hex SHA-256
// of the salt's bytes followed by the body's (crossing-3), computed here apart.
func FuzzDigest(f *testing.F) {
	f.Add([]byte("salt-of-sixteen!"), []byte("A note."))
	f.Add([]byte{}, []byte{})
	f.Add([]byte("ab"), []byte("c"))
	f.Add([]byte("a"), []byte("bc"))
	f.Fuzz(func(t *testing.T, salt, body []byte) {
		sum := sha256.Sum256(append(append([]byte(nil), salt...), body...))
		if got := receipts.Digest(salt, body); got != hex.EncodeToString(sum[:]) {
			t.Fatalf("got %s", got)
		}
	})
}
