package receipts_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// signedOver is the receipt med signs over payload, the bytes as given, whatever their form.
func signedOver(t testing.TB, med signing.Key, payload []byte) []byte {
	t.Helper()
	sig, err := signing.SignPayload(receipts.PayloadType, payload, med)
	if err != nil {
		t.Fatal(err)
	}
	return canonicalJSON(t, map[string]any{
		"payloadType": receipts.PayloadType,
		"payload":     base64.StdEncoding.EncodeToString(payload),
		"signatures":  []any{map[string]any{"keyid": med.Reference(), "sig": base64.StdEncoding.EncodeToString(sig)}},
	})
}

// replaceOnce replaces the one occurrence of old in b, and fails when b holds it other than once.
func replaceOnce(t testing.TB, b []byte, old, new string) []byte {
	t.Helper()
	if n := bytes.Count(b, []byte(old)); n != 1 {
		t.Fatalf("%q occurs %d times in %s", old, n, b)
	}
	return bytes.Replace(b, []byte(old), []byte(new), 1)
}

// spaced is canonical bytes of an object with one space added after its opening brace.
func spaced(t testing.TB, b []byte) []byte {
	t.Helper()
	if len(b) < 2 || b[0] != '{' {
		t.Fatalf("not an object: %s", b)
	}
	return append([]byte("{ "), b[1:]...)
}

// rewritings are the payloads the canonical form reads and would write back as canonical, a
// statement's canonical bytes: one space added, two members in another order, and the predicate's
// seq, 2, written 2.0.
func rewritings(t testing.TB, canonicalStatement []byte) map[string][]byte {
	t.Helper()
	first := `{"_type":"` + receipts.StatementType + `",`
	if !bytes.HasPrefix(canonicalStatement, []byte(first)) {
		t.Fatalf("the statement does not open with _type: %s", canonicalStatement)
	}
	rest := canonicalStatement[len(first) : len(canonicalStatement)-1]
	out := map[string][]byte{
		"a space after the opening brace": spaced(t, canonicalStatement),
		"_type after the other members":   []byte("{" + string(rest) + `,"_type":"` + receipts.StatementType + `"}`),
		"seq written 2.0":                 replaceOnce(t, canonicalStatement, `"seq":2,"to":`, `"seq":2.0,"to":`),
	}
	for name, b := range out {
		c, err := canonical.Form(b)
		if err != nil || !bytes.Equal(c, canonicalStatement) {
			t.Fatalf("%s: the canonical form does not read it as the statement: %v", name, err)
		}
	}
	return out
}

// TestAPayloadNotInItsCanonicalForm holds receipt-1 and section 8 row 1: a payload the canonical
// form reads but would rewrite, signed by the mediator over exactly those bytes, fails not-canonical
// at its receipt's index, and reading it without verifying refuses it too, so no id is yielded for
// it. The same statement in canonical form verifies.
func TestAPayloadNotInItsCanonicalForm(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, _ := chain(t, med, 2)
	if _, err := receipts.Verify(med.Reference(), list); err != nil {
		t.Fatalf("the list does not verify: %v", err)
	}
	for name, payload := range rewritings(t, mustPayload(t, list[1])) {
		r := signedOver(t, med, payload)
		if _, err := receipts.Verify(med.Reference(), [][]byte{list[0], r}); !isCode(err, receipts.CodeNotCanonical, 1) {
			t.Fatalf("%s: Verify: %v, want not-canonical at index 1", name, err)
		}
		if _, err := receipts.VerifySubset(med.Reference(), [][]byte{r}); !isCode(err, receipts.CodeNotCanonical, 0) {
			t.Fatalf("%s: VerifySubset: %v, want not-canonical at index 0", name, err)
		}
		if st, err := receipts.Decode(r); !isCode(err, receipts.CodeNotCanonical, 0) {
			t.Fatalf("%s: Decode: %+v, %v, want not-canonical", name, st, err)
		}
		if lines, err := receipts.Show(r); !isCode(err, receipts.CodeNotCanonical, 0) || lines != nil {
			t.Fatalf("%s: Show: %q, %v, want not-canonical", name, lines, err)
		}
	}
}

// TestACloseWhosePayloadIsNotCanonical holds the same rule for a close receipt (close-2, receipt-1):
// its payload with one space added, signed by the mediator, fails not-canonical at its index in the
// list, and DecodeClose refuses it.
func TestACloseWhosePayloadIsNotCanonical(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := closedLog(t, med)
	c := buildClose(t, closeOfLog(t, ids), med)
	if _, err := receipts.Verify(med.Reference(), append(append([][]byte{}, list...), c)); err != nil {
		t.Fatalf("the closed list does not verify: %v", err)
	}
	r := signedOver(t, med, spaced(t, mustPayload(t, c)))
	if _, err := receipts.Verify(med.Reference(), append(append([][]byte{}, list...), r)); !isCode(err, receipts.CodeNotCanonical, len(list)) {
		t.Fatalf("Verify: %v, want not-canonical at index %d", err, len(list))
	}
	if cl, err := receipts.DecodeClose(r); !isCode(err, receipts.CodeNotCanonical, 0) {
		t.Fatalf("DecodeClose: %+v, %v, want not-canonical", cl, err)
	}
}

// TestAReEncodedEnvelopeVerifies holds receipt-1's loose envelope: a receipt whose envelope is
// written with its members in another order and with spaces, its payload string untouched, verifies,
// and its id, from Verify and from Decode, is the receipt's as written, so the next receipt's prev
// still names it.
func TestAReEncodedEnvelopeVerifies(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := chain(t, med, 2)
	var env struct {
		Payload    string          `json:"payload"`
		Signatures json.RawMessage `json:"signatures"`
	}
	if err := json.Unmarshal(list[0], &env); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	reencoded := []byte(`{ "signatures": ` + string(env.Signatures) + `, "payloadType": "` + receipts.PayloadType + `", "payload": ` + string(payload) + " }")
	if c, err := canonical.Form(reencoded); err != nil || !bytes.Equal(c, list[0]) || bytes.Equal(reencoded, list[0]) {
		t.Fatalf("the envelope is not a re-encoding of the receipt: %v", err)
	}
	v, err := receipts.Verify(med.Reference(), [][]byte{reencoded, list[1]})
	if err != nil || len(v) != 2 || v[0].Statement().ID != ids[0] || v[1].Statement().ID != ids[1] {
		t.Fatalf("Verify: %d statements, %v", len(v), err)
	}
	if st, err := receipts.Decode(reencoded); err != nil || st.ID != ids[0] {
		t.Fatalf("Decode: %+v, %v, want the id %s", st, err, ids[0])
	}
}
