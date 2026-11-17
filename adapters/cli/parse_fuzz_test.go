package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/orilloai/placket/adapters/internal/seeds"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/signing"
)

// The parsers here read bytes a window sent or a party handed over: a delivery, a kept.jsonl line, a
// founding set's card. Their fuzz tests are seeded from the vectors' messages, deliveries and sets,
// and from hand-written lines (fuzz-parsers).

func fuzzKey(t testing.TB) signing.Key {
	t.Helper()
	seed := sha256.Sum256([]byte("placket vectors: key applier"))
	k, err := signing.NewKey(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// FuzzDelivery reads any bytes as a delivery a window sent, as get reads it: it decodes, checks it
// against its receipt, reads its body and writes its kept.jsonl line. It never panics.
func FuzzDelivery(f *testing.F) {
	f.Add([]byte(`{"kind":"note","messageId":"m","receipt":{},"receiptId":"sha256:00","salt":"00","text":"a"}`))
	f.Add([]byte(`{"raw":"AP8=","text":"a"}`))
	f.Add([]byte(`{"data":null}`))
	for _, group := range []string{"crossing", "questions"} {
		for _, in := range seeds.Inputs(f, group, "delivery", "message") {
			f.Add(in)
		}
	}
	k := fuzzKey(f)
	s := &session{key: k, client: &local.Client{Key: k, Accord: "sha256:" + string(bytes.Repeat([]byte("0"), 64))}}
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = keptLine(b, nil)
		_ = keptLine(b, []byte(`{}`))
		var d delivery
		if json.Unmarshal(b, &d) != nil {
			return
		}
		_ = d.oneBody()
		_ = d.bytes()
		_ = s.about(d)
	})
}

// FuzzKernelOf reads any bytes as a founding set and its card, as the window and verify read them.
// It never panics.
func FuzzKernelOf(f *testing.F) {
	f.Add([]byte(`{"card":{"kernel":{"version":"0.1.0","hash":"sha256:00"}}}`))
	f.Add([]byte(`{"card":{"kernel":null}}`))
	f.Add([]byte(`{"kernel":{"version":7}}`))
	for _, in := range seeds.Inputs(f, "founding", "input", "founding") {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, card := range [][]byte{b, cardOf(b)} {
			if k, ok := kernelOf(card); ok {
				_ = otherModule(k)
			}
		}
	})
}

// FuzzKeptLines reads any bytes as a handed-over state directory's kept.jsonl, as released and get
// read it for the salts and parts a key fetched. It never panics.
func FuzzKeptLines(f *testing.F) {
	f.Add([]byte(`{"kind":"note","receiptId":"sha256:00","salt":"00","text":"a"}` + "\n"))
	f.Add([]byte(`{"receiptId":1,"salt":{}}` + "\n" + `[]` + "\n"))
	for _, in := range seeds.Inputs(f, "questions", "delivery") {
		f.Add(append(append([]byte(nil), in...), '\n'))
	}
	k := fuzzKey(f)
	f.Fuzz(func(t *testing.T, b []byte) {
		path := filepath.Join(t.TempDir(), "s")
		d, err := state.Open(path, k.Reference(), &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, state.FileKept), b, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = fetchedSalts(d)
		_, _ = keptCrossed(d)
	})
}
