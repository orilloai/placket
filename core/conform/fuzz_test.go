package conform

import (
	"testing"

	"github.com/orilloai/placket/core/internal/testing/seeds"
)

// FuzzCheck checks any receipt and any call against the worked journal's founding set. It never
// panics (fuzz-parsers, core-no-panic). Its seeds are the receipts and calls of the conform vectors.
func FuzzCheck(f *testing.F) {
	w := journal(f)
	for _, b := range seeds.Inputs(f, "conform", "receipt", "call") {
		f.Add(b, b)
	}
	f.Add(w.list[2], w.call)
	f.Fuzz(func(t *testing.T, receipt, call []byte) {
		e := w.evidence()
		e.Receipts = append(append([][]byte(nil), w.list...), receipt)
		e.Answers = [][]byte{call}
		_, _ = Check(e)
		e.Subset = true
		_, _ = Check(e)
		e.Founding = nil
		e.Mediator = "ed25519:00"
		_, _ = Check(e)
	})
}

// FuzzBody checks any delivery against the worked journal's report. It never panics (fuzz-parsers,
// core-no-panic). Its seeds are the deliveries of the conform vectors and the journal's own.
func FuzzBody(f *testing.F) {
	w := journal(f)
	r, err := Check(w.evidence())
	if err != nil {
		f.Fatal(err)
	}
	for _, b := range seeds.Inputs(f, "conform", "delivery") {
		f.Add(b)
	}
	for _, d := range w.deliveries {
		f.Add(d)
	}
	f.Fuzz(func(t *testing.T, delivery []byte) {
		_ = Body(r, delivery)
	})
}
