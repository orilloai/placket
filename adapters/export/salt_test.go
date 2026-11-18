package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// TestAKeptPartNeedsA32ByteSalt logs one crossing whose receipt the mediator signed over a text and a
// salt of n bytes, with the fetched part kept under that salt: under 16 or 33 bytes the part is left
// out of the row and the note names its receipt; under 32 its text is in the row (log-1, crossing-2).
func TestAKeptPartNeedsA32ByteSalt(t *testing.T) {
	t.Parallel()
	key := func(name string) signing.Key {
		seed := sha256.Sum256([]byte("placket vectors: key " + name))
		k, err := signing.NewKey(seed[:])
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	med, from, to := key("mediator"), key("proposer").Reference(), key("applier").Reference()
	hash := func(s string) string { h, _ := canonical.TypedHash([]byte(`"` + s + `"`)); return h }
	const text = "A note."
	for n, kept := range map[int]bool{16: false, 33: false, 32: true} {
		salt := bytes.Repeat([]byte{9}, n)
		r, _, err := receipts.Build(receipts.Subject{Name: "note", Digest: receipts.Digest(salt, []byte(text))}, receipts.Predicate{
			Accord: hash("accord"), Card: hash("card"), Kind: receipts.KindCrossing, From: from, To: to, Seq: 1,
			At: "2026-11-12T09:00:00Z", Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: hash("message")},
			Verdict: "pass", Reason: "ok", By: receipts.By{Decider: receipts.ByTerms}}, med)
		if err != nil {
			t.Fatal(err)
		}
		st, _ := receipts.Decode(r)
		line, _ := json.Marshal(map[string]any{"kind": "note", "messageId": "m-1", "receipt": json.RawMessage(r), "receiptId": st.ID,
			"salt": hex.EncodeToString(salt), "text": text})
		line, _ = canonical.Form(line)
		var notes []string
		l, err := BuildAll(med.Reference(), [][]byte{r}, [][]byte{line}, nil, func(s string) { notes = append(notes, s) })
		if err != nil || len(l.Rows) != 1 {
			t.Fatalf("%d bytes: %+v, %v", n, l, err)
		}
		if got := l.Rows[0].Text == text; got != kept {
			t.Fatalf("a salt of %d bytes: the row's text is %q", n, l.Rows[0].Text)
		}
		if named := len(notes) == 1 && strings.Contains(notes[0], st.ID); named == kept {
			t.Fatalf("a salt of %d bytes: notes %q", n, notes)
		}
	}
}
