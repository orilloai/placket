package cli_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// rewrite is a delivery saved by get --json with its delivery's members changed: each set, or removed
// when its value is nil.
func rewrite(t *testing.T, saved string, set map[string]json.RawMessage) string {
	t.Helper()
	var o, d map[string]json.RawMessage
	if err := json.Unmarshal([]byte(saved), &o); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(o["delivery"], &d); err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		if v == nil {
			delete(d, k)
		} else {
			d[k] = v
		}
	}
	o["delivery"], _ = json.Marshal(d)
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	return string(b) + "\n"
}

// TestVerifyADeliveryAlone checks the applier's delivery of a released proposal with the founding set
// and no journal (verify-5, commands-8): it exits 0 with the valid line of one receipt, the signed
// line of the release, the body line and the subset's complete line, and with --json the object
// holds the signed line under decisions. The delivery with its answer left out, or swapped for
// another call, exits 1 with body; with its receipt's signature changed it is error verify
// signature; and with no founding set, with --subset, or with neither --receipts nor --delivery, it
// is a usage error.
func TestVerifyADeliveryAlone(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	ids := journalIDs(t, a, a.journal())
	release := ids[2]
	got := must(t, a.as("applier", "get", "--json")...)
	saved := filepath.Join(a.dir, "delivery.json")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(saved, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(got.stdout)
	founding := filepath.Join(a.dir, "founding.json")
	alone := func(extra ...string) result {
		t.Helper()
		return run(t, env(), append([]string{"verify", "--accord", a.id, "--founding", founding, "--delivery", saved}, extra...)...)
	}
	signed := fmt.Sprintf(cli.LineSigned, release, a.refs["decider"], a.refs["principal"], "decider")
	r := alone()
	for _, l := range []string{fmt.Sprintf(cli.LineValid, 1, a.refs["mediator"]), signed, fmt.Sprintf(cli.LineOutsideSubset, 1),
		cli.LineConformSealed, cli.LineSubsetNotComplete, fmt.Sprintf(cli.LineBody, release)} {
		if !slices.Contains(r.lines(), l) {
			t.Fatalf("the delivery alone: no line %q in %v", l, r)
		}
	}
	if r.code != 0 {
		t.Fatalf("the delivery alone: %v", r)
	}
	r = alone("--json")
	var o struct {
		Exit      int `json:"exit"`
		Verified  int `json:"verified"`
		Decisions []struct {
			ReceiptID string `json:"receiptId"`
			Line      string `json:"line"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || o.Exit != 0 || o.Verified != 1 || len(o.Decisions) != 1 ||
		o.Decisions[0].ReceiptID != release || o.Decisions[0].Line != signed {
		t.Fatalf("the delivery alone with --json: %+v, %v, %v", o, err, r)
	}

	// The delivery's answer left out, and swapped for another call: body.
	var d struct {
		Delivery struct {
			Answer  json.RawMessage `json:"answer"`
			Receipt json.RawMessage `json:"receipt"`
		} `json:"delivery"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &d); err != nil || len(d.Delivery.Answer) == 0 {
		t.Fatalf("the delivery holds no answer: %v", err)
	}
	other := json.RawMessage(strings.Replace(string(d.Delivery.Answer), `"release":true`, `"release":false`, 1))
	for name, set := range map[string]map[string]json.RawMessage{
		"without its answer":  {"answer": nil},
		"with another answer": {"answer": other},
	} {
		write(rewrite(t, got.stdout, set))
		r := alone()
		if r.code != 1 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineNotConform, "body", 1)) ||
			!slices.Contains(r.lines(), fmt.Sprintf(cli.LineBodyNot, 1)) || slices.Contains(r.lines(), signed) {
			t.Fatalf("the delivery %s: %v", name, r)
		}
	}

	// The receipt's signature changed: the receipt is not the mediator's.
	var receipt map[string]json.RawMessage
	if err := json.Unmarshal(d.Delivery.Receipt, &receipt); err != nil {
		t.Fatal(err)
	}
	sig, _ := json.Marshal([]map[string]string{{"keyid": a.refs["mediator"], "sig": base64.StdEncoding.EncodeToString(make([]byte, 64))}})
	receipt["signatures"] = sig
	forged, _ := json.Marshal(receipt)
	write(rewrite(t, got.stdout, map[string]json.RawMessage{"receipt": forged}))
	if r := alone(); r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{"error verify signature receipts.jsonl line 1"}) {
		t.Fatalf("the receipt's signature changed: %v", r)
	}

	write(got.stdout)
	for name, args := range map[string][]string{
		"with no founding set": {"verify", "--mediator", a.refs["mediator"], "--delivery", saved},
		"with --subset":        {"verify", "--accord", a.id, "--founding", founding, "--delivery", saved, "--subset"},
		"with --answers":       {"verify", "--accord", a.id, "--founding", founding, "--delivery", saved, "--answers", saved},
	} {
		if r := run(t, env(), args...); r.code != cli.ExitTool || r.stderr != "error usage\n" {
			t.Fatalf("the delivery alone %s: %v", name, r)
		}
	}
	if r := run(t, env(), "verify", "--accord", a.id, "--founding", founding); r.code != cli.ExitTool ||
		r.stderr != "usage: placket verify: --receipts is required\nerror usage\n" {
		t.Fatalf("neither --receipts nor --delivery: %v", r)
	}
}
