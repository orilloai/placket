package receipts_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

const (
	accord = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	card   = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	mhash  = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
)

// chain builds n crossing receipts of one pair, each chained to the one before.
func chain(t testing.TB, med signing.Key, n int) ([][]byte, []string) {
	t.Helper()
	from, to := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference()
	var out [][]byte
	var ids []string
	prev := ""
	for i := 1; i <= n; i++ {
		r, v, err := receipts.Build(receipts.Subject{Name: "note", Digest: receipts.Digest([]byte("salt-of-sixteen!"), []byte{byte(i)})},
			receipts.Predicate{
				Accord: accord, Card: card, Kind: receipts.KindCrossing, From: from, To: to,
				Seq: int64(i), Prev: prev, At: "2026-11-12T09:00:00Z",
				Envelope: receipts.Envelope{Sender: from, Seq: int64(i), Hash: mhash},
				Verdict:  "pass", Reason: "ok",
			}, med)
		if err != nil {
			t.Fatal(err)
		}
		id := v.Statement().ID
		out, ids, prev = append(out, r), append(ids, id), id
	}
	return out, ids
}

// TestAForgedReceiptShowsAsARealOne is receipt-8's scenario: a receipt whose signature is not the
// mediator's decodes and shows exactly as the real one does, and the showing says it was not
// verified; verifying tells them apart.
func TestAForgedReceiptShowsAsARealOne(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	real, _ := chain(t, med, 1)
	var r map[string]any
	if err := json.Unmarshal(real[0], &r); err != nil {
		t.Fatal(err)
	}
	payload, _ := base64.StdEncoding.DecodeString(r["payload"].(string))
	forger := ptesting.Key(t, "stranger")
	sig, _ := signing.SignPayload(receipts.PayloadType, payload, forger)
	r["signatures"].([]any)[0].(map[string]any)["sig"] = base64.StdEncoding.EncodeToString(sig)
	forged, _ := json.Marshal(r)

	shownReal, err1 := receipts.Show(real[0])
	shownForged, err2 := receipts.Show(forged)
	if err1 != nil || err2 != nil || !reflect.DeepEqual(shownReal, shownForged) {
		t.Fatalf("a forged receipt shows differently:\n%q\n%q\n%v %v", shownReal, shownForged, err1, err2)
	}
	if shownForged[0] != receipts.NotVerified {
		t.Fatalf("the first line is %q", shownForged[0])
	}
	if _, err := receipts.Verify(med.Reference(), [][]byte{forged}); !isCode(err, receipts.CodeSignature, 0) {
		t.Fatalf("the forged receipt verifies: %v", err)
	}
	if _, err := receipts.Verify(med.Reference(), real); err != nil {
		t.Fatalf("the real receipt does not verify: %v", err)
	}
}

func isCode(err error, c receipts.Code, index int) bool {
	var re *receipts.Error
	return errors.As(err, &re) && re.Code == c && re.Index == index
}

// TestOneByteOfTheSignatureFails changes one byte of a receipt's decoded signature, encodes it
// again, and verifies: signature, at that receipt's index (receipt-6).
func TestOneByteOfTheSignatureFails(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, _ := chain(t, med, 3)
	for i := range list {
		for _, at := range []int{0, 31, 32, 63} {
			var r map[string]any
			if err := json.Unmarshal(list[i], &r); err != nil {
				t.Fatal(err)
			}
			entry := r["signatures"].([]any)[0].(map[string]any)
			sig, _ := base64.StdEncoding.DecodeString(entry["sig"].(string))
			sig[at] ^= 0x01
			entry["sig"] = base64.StdEncoding.EncodeToString(sig)
			changed, _ := json.Marshal(r)
			l := append([][]byte(nil), list...)
			l[i] = changed
			if _, err := receipts.Verify(med.Reference(), l); !isCode(err, receipts.CodeSignature, i) {
				t.Fatalf("receipt %d, byte %d: %v", i, at, err)
			}
		}
	}
}

// TestBuildDependsOnEachInput changes each input of Build in turn, the time and the key included,
// and each changes the receipt; the same inputs write the same bytes (inputs-as-parameters,
// receipt-10).
func TestBuildDependsOnEachInput(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	from := ptesting.Key(t, "proposer").Reference()
	sub := receipts.Subject{Name: "note", Digest: strings.Repeat("ab", 32)}
	pred := receipts.Predicate{
		Accord: accord, Card: card, Kind: receipts.KindCrossing, From: from, To: from, Seq: 1,
		At: "2026-11-12T09:00:00Z", Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: mhash}, Verdict: "pass", Reason: "ok",
	}
	base, v, err := receipts.Build(sub, pred, med)
	if err != nil {
		t.Fatal(err)
	}
	again, v2, _ := receipts.Build(sub, pred, med)
	id := v.Statement().ID
	if !bytes.Equal(base, again) || id != v2.Statement().ID {
		t.Fatal("the same inputs wrote two receipts")
	}
	at := pred
	at.At = "2026-11-12T09:00:01Z"
	salted := sub
	salted.Digest = strings.Repeat("ac", 32)
	for name, c := range map[string]func() ([]byte, receipts.Verified, error){
		"time":    func() ([]byte, receipts.Verified, error) { return receipts.Build(sub, at, med) },
		"subject": func() ([]byte, receipts.Verified, error) { return receipts.Build(salted, pred, med) },
		"key": func() ([]byte, receipts.Verified, error) {
			return receipts.Build(sub, pred, ptesting.Key(t, "other-mediator"))
		},
	} {
		r, i, err := c()
		if err != nil || bytes.Equal(r, base) || (name != "key" && i.Statement().ID == id) {
			t.Fatalf("a changed %s left the receipt as it was: %v", name, err)
		}
	}
}

// TestBuildRefusesWhatAVerifierRefuses builds statements row 4 would refuse, and a broken chain
// link, and gets no receipt.
func TestBuildRefusesWhatAVerifierRefuses(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	from := ptesting.Key(t, "proposer").Reference()
	good := receipts.Predicate{
		Accord: accord, Card: card, Kind: receipts.KindCrossing, From: from, To: from, Seq: 1,
		At: "2026-11-12T09:00:00Z", Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: mhash}, Verdict: "pass", Reason: "ok",
	}
	sub := receipts.Subject{Name: "note", Digest: strings.Repeat("ab", 32)}
	for name, change := range map[string]func(p *receipts.Predicate){
		"block with ok":       func(p *receipts.Predicate) { p.Verdict = "block" },
		"a resolution reason": func(p *receipts.Predicate) { p.Verdict, p.Reason = "block", "declined" },
		"a leap second":       func(p *receipts.Predicate) { p.At = "2026-11-12T23:59:60Z" },
		"another sender":      func(p *receipts.Predicate) { p.Envelope.Sender = ptesting.Key(t, "applier").Reference() },
		"a prev at seq 1":     func(p *receipts.Predicate) { p.Prev = mhash },
		"no prev at seq 2":    func(p *receipts.Predicate) { p.Seq = 2 },
		"seq 0":               func(p *receipts.Predicate) { p.Seq = 0 },
	} {
		p := good
		change(&p)
		r, v, err := receipts.Build(sub, p, med)
		var re *receipts.Error
		if r != nil || v != (receipts.Verified{}) || !errors.As(err, &re) || re.Code != receipts.CodeStatement || err.Error() != "receipts: statement" {
			t.Fatalf("%s: got %d bytes, %v", name, len(r), err)
		}
	}
}

// TestErrorTextQuotesNoInput triggers the verifier's and reconciliation's codes on receipts whose
// subject, envelope and payload hold planted values, and reads each error: the package, the code,
// the index and, for kept, the party's reference (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	const secret = "s3cr3t-body-salt-seed-key-payload"
	list, _ := chain(t, med, 2)
	withSecret := func(r []byte) []byte {
		return bytes.Replace(r, []byte(`"payloadType":`), []byte(`"`+secret+`":1,"payloadType":`), 1)
	}
	applier := ptesting.Key(t, "applier").Reference()
	for _, c := range []struct {
		err  error
		text string
	}{
		{verifyErr(receipts.Verify(secret, list)), "receipts: reference"},
		{verifyErr(receipts.Verify(med.Reference(), [][]byte{[]byte(`{"` + secret + `":0.5}`)})), "receipts: not-canonical at index 0"},
		{verifyErr(receipts.Verify(med.Reference(), [][]byte{[]byte(`{"payloadType":"` + secret + `"}`)})), "receipts: payload-type at index 0"},
		{verifyErr(receipts.Verify(med.Reference(), [][]byte{list[0], withSecret(list[1])})), "receipts: signature at index 1"},
		{verifyErr(receipts.Verify(med.Reference(), [][]byte{list[1]})), "receipts: chain at index 0"},
		{verifyErr(receipts.Verify(ptesting.Key(t, "proposer").Reference(), list)), "receipts: signature at index 0"},
		{reconcileErr(receipts.Reconcile(med.Reference(), mhash, card, list, nil)), "receipts: presented at index 0"},
		{reconcileErr(receipts.Reconcile(med.Reference(), accord, card, list[:1],
			[]receipts.Kept{{Actor: applier, Delivered: [][]byte{withSecret(list[1])}}})), "receipts: kept at index 0 of " + applier},
		{reconcileErr(receipts.Reconcile(med.Reference(), accord, card, list,
			[]receipts.Kept{{Actor: applier}, {Actor: secret + "\nINJECT"}})), "receipts: kept at index 1"},
		{reconcileErr(receipts.Reconcile(secret, accord, card, list,
			[]receipts.Kept{{Actor: secret}})), "receipts: kept at index 0"},
	} {
		if c.err == nil || c.err.Error() != c.text || strings.Contains(c.err.Error(), secret) {
			t.Fatalf("got %v, want %q", c.err, c.text)
		}
	}
}

func verifyErr(_ []receipts.Verified, err error) error { return err }

func reconcileErr(_ receipts.Report, err error) error { return err }

// TestVerifyYieldsTheStatements verifies a chain and reads back what was built.
func TestVerifyYieldsTheStatements(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := chain(t, med, 3)
	verified, err := receipts.Verify(med.Reference(), list)
	if err != nil || len(verified) != 3 {
		t.Fatalf("got %d statements, %v", len(verified), err)
	}
	for i, v := range verified {
		s := v.Statement()
		if s.ID != ids[i] || s.Predicate.Seq != int64(i+1) || s.Predicate.Mediator != med.Reference() || s.Subject.Name != "note" {
			t.Fatalf("statement %d: %+v", i, s)
		}
		if (i == 0) != (s.Predicate.Prev == "") || (i > 0 && s.Predicate.Prev != ids[i-1]) {
			t.Fatalf("statement %d's prev is %q", i, s.Predicate.Prev)
		}
	}
	places, err := receipts.Releases(verified)
	if err != nil || len(places) != 3 || places[0].Role != receipts.RoleGive {
		t.Fatalf("places %+v, %v", places, err)
	}
}

// FuzzVerify verifies any receipt after a valid one, under any mediator reference, and reads it
// without verifying. It never panics.
func FuzzVerify(f *testing.F) {
	med := ptesting.Key(f, "mediator")
	list, _ := chain(f, med, 2)
	f.Add(list[1], med.Reference())
	f.Add([]byte(`{"payloadType":"application/vnd.in-toto+json","payload":"e30=","signatures":[]}`), med.Reference())
	f.Add([]byte("\xff"), "mediator")
	f.Fuzz(func(t *testing.T, receipt []byte, mediator string) {
		st, err := receipts.Verify(mediator, [][]byte{list[0], receipt})
		if err == nil && len(st) != 2 {
			t.Fatalf("%d statements of 2 receipts", len(st))
		}
		_, _ = receipts.Decode(receipt)
		_, _ = receipts.Show(receipt)
		_, _ = receipts.Reconcile(mediator, accord, card, [][]byte{list[0], receipt},
			[]receipts.Kept{{Actor: mediator, Delivered: [][]byte{receipt}, Envelopes: []receipts.Envelope{{Seq: 1, Hash: mhash}}}})
	})
}

// FuzzBuild builds a receipt of any strings. It never panics, and what it builds verifies.
func FuzzBuild(f *testing.F) {
	med := ptesting.Key(f, "mediator")
	from := ptesting.Key(f, "proposer").Reference()
	f.Add("note", strings.Repeat("ab", 32), from, "crossing", "pass", "ok", "2026-11-12T09:00:00Z")
	f.Add("answer", "", "", "resolution", "escalate", "maybe", "")
	f.Fuzz(func(t *testing.T, name, digest, party, kind, verdict, reason, at string) {
		r, _, err := receipts.Build(receipts.Subject{Name: name, Digest: digest}, receipts.Predicate{
			Accord: accord, Card: card, Kind: kind, From: party, To: party, Seq: 1, At: at,
			Envelope: receipts.Envelope{Sender: party, Seq: 1, Hash: mhash}, Verdict: verdict, Reason: reason,
		}, med)
		if err != nil {
			return
		}
		if _, err := receipts.Verify(med.Reference(), [][]byte{r}); err != nil && kind == receipts.KindCrossing {
			t.Fatalf("a built receipt does not verify: %v", err)
		}
	})
}

// TestVerifiedIsWhatWasChecked holds verified-by-type: Build and Verify hand back the same statement
// for one receipt, as Verified, and the statement a caller reads is a copy it cannot write back.
func TestVerifiedIsWhatWasChecked(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	from := ptesting.Key(t, "proposer").Reference()
	r, built, err := receipts.Build(receipts.Subject{Name: "note", Digest: strings.Repeat("ab", 32)}, receipts.Predicate{
		Accord: accord, Card: card, Kind: receipts.KindCrossing, From: from, To: from, Seq: 1,
		At: "2026-11-12T09:00:00Z", Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: mhash}, Verdict: "pass", Reason: "ok",
	}, med)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := receipts.Verify(med.Reference(), [][]byte{r})
	if err != nil || len(verified) != 1 {
		t.Fatalf("got %d statements, %v", len(verified), err)
	}
	if verified[0] != built {
		t.Fatalf("Verify read %+v, Build made %+v", verified[0].Statement(), built.Statement())
	}
	sum := sha256.Sum256(mustPayload(t, r))
	if id := built.Statement().ID; id != "sha256:"+hex.EncodeToString(sum[:]) || built.Statement().Predicate.Mediator != med.Reference() {
		t.Fatalf("Build's statement: %+v", built.Statement())
	}
	s := built.Statement()
	s.Predicate.Verdict, s.ID = "block", "changed"
	if got := built.Statement(); got.Predicate.Verdict != "pass" || got.ID == "changed" {
		t.Fatalf("a changed copy changed the statement: %+v", got)
	}
}

func mustPayload(t *testing.T, receipt []byte) []byte {
	t.Helper()
	var env struct{ Payload string }
	if err := json.Unmarshal(receipt, &env); err != nil {
		t.Fatal(err)
	}
	b, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
