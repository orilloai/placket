package client

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/internal/seeds"
	"github.com/orilloai/placket/core/signing"
)

// FuzzDelivery reads any bytes as a delivery a window sent, as Get reads it: it decodes, checks it
// against its receipt, reads its body and writes its kept.jsonl line. It never panics. It is seeded
// from the vectors' messages and deliveries, and from hand-written ones (fuzz-parsers).
func FuzzDelivery(f *testing.F) {
	f.Add([]byte(`{"kind":"note","messageId":"m","receipt":{},"receiptId":"sha256:00","salt":"00","text":"a"}`))
	f.Add([]byte(`{"raw":"AP8=","text":"a"}`))
	f.Add([]byte(`{"data":null}`))
	for _, group := range []string{"crossing", "questions"} {
		for _, in := range seeds.Inputs(f, group, "delivery", "message") {
			f.Add(in)
		}
	}
	seed := sha256.Sum256([]byte("placket vectors: key applier"))
	k, err := signing.NewKey(seed[:])
	if err != nil {
		f.Fatal(err)
	}
	c := &Client{key: k, m: Mediator{Accord: "sha256:" + strings.Repeat("0", 64)}}
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = keptLine(b)
		var d Delivery
		if json.Unmarshal(b, &d) != nil {
			return
		}
		_ = d.OneBody()
		_ = d.Bytes()
		_ = c.about(d)
	})
}
