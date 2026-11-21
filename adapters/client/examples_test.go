package client_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// updateExamples signs the delivery examples again, under exampleSeed, from their bodies, in place
// of checking them.
var updateExamples = flag.Bool("update-examples", false, "sign spec/schema/testdata/delivery/accepted/ again")

// The delivery examples of the schema, and the seed of the fixed test key their receipts are signed
// under: a key of these examples alone.
const (
	examplesDir = "../../spec/schema/testdata/delivery/accepted"
	exampleSeed = "6578616d706c657320617265207369676e656420756e64657220746869732031"
)

// TestDeliveryExamplesMatchTheirBodies reads each accepted delivery example as a receiver would:
// its receipt is the current predicate type's, signed under the examples' key, its id is the
// receipt's (receipt-1), it names the delivery's kind and its body by the salted digest crossing-3
// defines, and its by names the answer call inside the delivery exactly when one is there.
func TestDeliveryExamplesMatchTheirBodies(t *testing.T) {
	seed, err := hex.DecodeString(exampleSeed)
	if err != nil {
		t.Fatal(err)
	}
	key, err := signing.NewKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(examplesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no delivery examples in %s: %v", examplesDir, err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var d client.Delivery
			if err := json.Unmarshal(b, &d); err != nil {
				t.Fatal(err)
			}
			if *updateExamples {
				b = signExample(t, b, d, key)
				if err := os.WriteFile(f, b, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(b, &d); err != nil {
					t.Fatal(err)
				}
			}
			checkExample(t, d, key.Reference())
		})
	}
}

func checkExample(t *testing.T, d client.Delivery, mediator string) {
	t.Helper()
	var env struct{ Payload string }
	if err := json.Unmarshal(d.Receipt, &env); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var statement struct {
		PredicateType string `json:"predicateType"`
	}
	if err := json.Unmarshal(payload, &statement); err != nil {
		t.Fatal(err)
	}
	if statement.PredicateType != "https://placket.tech/receipt/0.1" {
		t.Fatalf("predicate type %q, want https://placket.tech/receipt/0.1", statement.PredicateType)
	}
	if _, err := receipts.VerifySubset(mediator, [][]byte{d.Receipt}); err != nil {
		t.Fatalf("the receipt does not verify under the examples' key %s: %v", mediator, err)
	}
	st, err := receipts.Decode(d.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	salt, ok := client.SaltBytes(d.Salt)
	if !ok {
		t.Fatalf("salt %q is not %d bytes of hex", d.Salt, 32)
	}
	if !d.OneBody() {
		t.Fatal("the example holds other than one body")
	}
	if got, want := st.Subject.Digest, receipts.Digest(salt, d.Bytes()); got != want {
		t.Fatalf("the receipt names digest %s, but the body's salted digest is %s", got, want)
	}
	if st.ID != d.ReceiptID {
		t.Fatalf("receiptId %s, but the receipt's id is %s", d.ReceiptID, st.ID)
	}
	if st.Subject.Name != d.Kind {
		t.Fatalf("the receipt names kind %q, the delivery %q", st.Subject.Name, d.Kind)
	}
	if len(d.Answer) > 0 {
		if st.Predicate.By.Decider != receipts.ByAnswer || st.Predicate.By.Answer != answer.ID(d.Answer) {
			t.Fatalf("the receipt's by is %+v, not the answer call inside the delivery, %s", st.Predicate.By, answer.ID(d.Answer))
		}
	} else if st.Predicate.By.Decider == receipts.ByAnswer {
		t.Fatal("the receipt's by names an answer, but the delivery holds no answer call")
	}
}

// signExample returns the example b with its receipt written again under key: the predicate the
// receipt held, its subject the delivery's kind and the salted digest of its body, by the answer
// call inside the delivery when there is one, else by the terms, and its receiptId the new
// receipt's. Every other byte of b stays as it was.
func signExample(t *testing.T, b []byte, d client.Delivery, key signing.Key) []byte {
	t.Helper()
	var env struct {
		Payload    string
		Signatures []struct{ Keyid, Sig string }
	}
	if err := json.Unmarshal(d.Receipt, &env); err != nil || len(env.Signatures) != 1 {
		t.Fatalf("the receipt is no envelope of one signature: %v", err)
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Predicate struct {
			Accord, At, Card, From, Kind, Prev, Reason, To, Verdict string
			Seq                                                     int64
			Envelope                                                struct {
				Hash, Sender string
				Seq          int64
			}
		}
	}
	if err := json.Unmarshal(payload, &old); err != nil {
		t.Fatal(err)
	}
	p := old.Predicate
	by := receipts.By{Decider: receipts.ByTerms}
	if len(d.Answer) > 0 {
		by = receipts.By{Decider: receipts.ByAnswer, Answer: answer.ID(d.Answer)}
	}
	salt, ok := client.SaltBytes(d.Salt)
	if !ok {
		t.Fatalf("salt %q is not 32 bytes of hex", d.Salt)
	}
	receipt, v, err := receipts.Build(receipts.Subject{Name: d.Kind, Digest: receipts.Digest(salt, d.Bytes())}, receipts.Predicate{
		Accord: p.Accord, Card: p.Card, Kind: p.Kind, From: p.From, To: p.To, Seq: p.Seq, Prev: p.Prev, At: p.At,
		Envelope: receipts.Envelope{Sender: p.Envelope.Sender, Seq: p.Envelope.Seq, Hash: p.Envelope.Hash},
		Verdict:  p.Verdict, Reason: p.Reason, By: by,
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	var now struct {
		Payload    string
		Signatures []struct{ Keyid, Sig string }
	}
	if err := json.Unmarshal(receipt, &now); err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, r := range [][2]string{
		{d.ReceiptID, v.Statement().ID},
		{env.Payload, now.Payload},
		{env.Signatures[0].Keyid, now.Signatures[0].Keyid},
		{env.Signatures[0].Sig, now.Signatures[0].Sig},
	} {
		if strings.Count(s, `"`+r[0]+`"`) != 1 {
			t.Fatalf("the example does not hold %q once", r[0])
		}
		s = strings.Replace(s, `"`+r[0]+`"`, `"`+r[1]+`"`, 1)
	}
	if !bytes.HasSuffix([]byte(s), []byte("\n")) {
		s += "\n"
	}
	return []byte(s)
}
