package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// TestShowRefusesAPayloadNotInCanonicalForm shows the journal's first receipt with one space added
// after its statement's opening brace, signed again by the Accord's mediator over those bytes:
// show is tool error verify not-canonical, and prints no receipt and no receipt id, in words or in
// --json (receipt-1, receipt-8). The receipt as the mediator wrote it shows.
func TestShowRefusesAPayloadNotInCanonicalForm(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	b, err := os.ReadFile(filepath.Join(a.journal(), "receipts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := bytes.Split(b, []byte("\n"))[0]
	must(t, "show", "--receipt", writeFile(t, a.dir, "first.receipt", first))

	var envelope struct{ Payload string }
	if err := json.Unmarshal(first, &envelope); err != nil {
		t.Fatal(err)
	}
	payload, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil || len(payload) < 2 || payload[0] != '{' {
		t.Fatalf("payload %q: %v", payload, err)
	}
	spaced := append([]byte("{ "), payload[1:]...)
	med, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := signing.SignPayload(receipts.PayloadType, spaced, med)
	if err != nil {
		t.Fatal(err)
	}
	r, err := json.Marshal(map[string]any{
		"payloadType": receipts.PayloadType,
		"payload":     base64.StdEncoding.EncodeToString(spaced),
		"signatures":  []any{map[string]any{"keyid": med.Reference(), "sig": base64.StdEncoding.EncodeToString(sig)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := writeFile(t, a.dir, "spaced.receipt", r)

	const refused = "error verify not-canonical receipts.jsonl line 1\n"
	if got := run(t, env(), "show", "--receipt", p); got.code != cli.ExitTool || got.stdout != "" || got.stderr != refused {
		t.Fatalf("show: %v", got)
	}
	got := run(t, env(), "show", "--receipt", p, "--json")
	if got.code != cli.ExitTool || got.stderr != refused {
		t.Fatalf("show --json: %v", got)
	}
	o := object(t, got)
	is(t, o, "error", `"verify"`)
	is(t, o, "detail", `["not-canonical","receipts.jsonl","line 1"]`)
	if _, ok := o["receiptId"]; ok {
		t.Fatalf("show --json names a receipt id: %v", got)
	}
}
