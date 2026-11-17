package mediator_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// askAndLog is the ask-and-log founding set of vectors/founding/.
func askAndLog(t testing.TB) ([]byte, string) {
	t.Helper()
	return ptesting.Founding(t, "ask-and-log")
}

func found(t testing.TB) *mediator.Mediator {
	t.Helper()
	set, at := askAndLog(t)
	m, err := mediator.Found(set, ptesting.Key(t, "mediator"), at)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

const extension = "https://placket.tech/crossing/0.1"

// saltOf returns s's bytes as a salt of SaltBytes: cut, or padded with zero bytes.
func saltOf(s string) []byte {
	b := make([]byte, mediator.SaltBytes)
	copy(b, s)
	return b
}

// message builds a crossing message of one text part from a member, in ROLE_USER, to a receiver.
func message(t testing.TB, accord, id, to, kind, text string, change func(m map[string]any)) []byte {
	t.Helper()
	m := map[string]any{
		"messageId": id, "role": mediator.RoleUser, "extensions": []any{extension},
		"metadata": map[string]any{extension: map[string]any{"accord": accord, "to": to}},
		"parts": []any{map[string]any{"text": text, "metadata": map[string]any{extension: map[string]any{
			"kind": kind, "salt": hex.EncodeToString(saltOf("a salt of 32 bytes " + id)),
		}}}},
	}
	if change != nil {
		change(m)
	}
	b, _ := json.Marshal(m)
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// envelope signs an envelope over message bytes at seq.
func envelope(t testing.TB, seq int64, msg []byte, k signing.Key) []byte {
	t.Helper()
	h, _ := canonical.TypedHash(msg)
	b, _ := json.Marshal(map[string]any{"seq": seq, "hash": h})
	sig, err := signing.Sign(b, signing.TypeEnvelope, k)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(map[string]any{"seq": seq, "hash": h, "sig": sig})
	return out
}

func rejected(err error, code mediator.RejectionCode) bool {
	var r *mediator.Rejection
	return errors.As(err, &r) && r.Code == code
}

// TestTwoMediatorsOneRecord is receipt-10's scenario: two mediators founded from one set with one key
// at one time, given the same gives with the same salts at the same times, write the same receipts
// byte for byte.
func TestTwoMediatorsOneRecord(t *testing.T) {
	t.Parallel()
	a, b := found(t), found(t)
	p, ap, d := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier"), ptesting.Key(t, "decider")
	type give struct {
		at     string
		from   signing.Key
		seq    int64
		to     signing.Key
		kind   string
		code   mediator.RejectionCode
		accord string
	}
	gives := []give{
		{"2026-11-12T09:00:00Z", p, 1, ap, "note", "", ""},
		{"2026-11-12T09:01:00Z", p, 2, ap, "proposal", "", ""},
		{"2026-11-12T09:02:00Z", ap, 1, p, "note", "", ""},
		{"2026-11-12T09:03:00Z", p, 3, ap, "tests", "", ""},
		{"2026-11-12T09:04:00Z", d, 1, p, "answer", "", ""},
		{"2026-11-12T09:05:00Z", p, 3, ap, "note", mediator.EnvelopeSequence, ""},
		{"2026-11-12T09:06:00Z", p, 4, ap, "note", mediator.AccordMismatch, strings.Repeat("0", 71)},
		{"2026-11-12T09:07:00Z", p, 9, ap, "note", "", ""},
	}
	for i, g := range gives {
		acc := a.Accord()
		if g.accord != "" {
			acc = "sha256:" + g.accord[7:]
		}
		msg := message(t, acc, "m-"+string(rune('a'+i)), g.to.Reference(), g.kind, "Part "+string(rune('a'+i)), nil)
		env := envelope(t, g.seq, msg, g.from)
		oa, errA := a.Give(g.from.Reference(), msg, env, g.at)
		ob, errB := b.Give(g.from.Reference(), msg, env, g.at)
		if g.code != "" {
			if !rejected(errA, g.code) || !rejected(errB, g.code) {
				t.Fatalf("give %d: %v and %v, want %s", i, errA, errB, g.code)
			}
			continue
		}
		if errA != nil || errB != nil || !bytes.Equal(oa.Receipt, ob.Receipt) || oa.ReceiptID != ob.ReceiptID {
			t.Fatalf("give %d: the two mediators differ: %v, %v", i, errA, errB)
		}
	}
	ra, rb := a.Receipts(), b.Receipts()
	if len(ra) != 6 || len(ra) != len(rb) {
		t.Fatalf("%d and %d receipts", len(ra), len(rb))
	}
	for i := range ra {
		if !bytes.Equal(ra[i], rb[i]) {
			t.Fatalf("receipt %d differs", i)
		}
	}
	if _, err := receipts.Verify(ptesting.Key(t, "mediator").Reference(), ra); err != nil {
		t.Fatalf("the receipts do not verify: %v", err)
	}
	for _, k := range []signing.Key{p, ap, d} {
		for {
			da, _ := a.Get(k.Reference())
			db, _ := b.Get(k.Reference())
			if !bytes.Equal(da, db) {
				t.Fatalf("%s's deliveries differ", k.Reference())
			}
			if da == nil {
				break
			}
		}
	}
	if pa, pb := a.Pending(), b.Pending(); len(pa) != 1 || pa[0] != pb[0] {
		t.Fatalf("pending %v and %v", pa, pb)
	}
}

// TestAStrangersBytesAreNotRead gives as a caller with no declaration message and envelope bytes
// that are not JSON: not-a-member (crossing-7, crossing-6).
func TestAStrangersBytesAreNotRead(t *testing.T) {
	t.Parallel()
	m := found(t)
	_, err := m.Give(ptesting.Key(t, "stranger").Reference(), []byte("not json {"), []byte{0xff}, "2026-11-12T09:00:00Z")
	if !rejected(err, mediator.NotAMember) || err.Error() != "mediator: rejected: not-a-member" {
		t.Fatalf("got %v", err)
	}
	if _, err := m.Get(ptesting.Key(t, "stranger").Reference()); !rejected(err, mediator.NotAMember) {
		t.Fatalf("a stranger's fetch: %v", err)
	}
	if len(m.Receipts()) != 0 {
		t.Fatal("a rejection wrote a receipt")
	}
}

// TestARejectedGiveChangesNothing gives a part the door rejects: no receipt is written, nothing is
// queued, and the sender's next accepted give takes the same seq and is the first of its pair
// (crossing-7).
func TestARejectedGiveChangesNothing(t *testing.T) {
	t.Parallel()
	m := found(t)
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	bad := message(t, m.Accord(), "bad", ap.Reference(), "note", "Two parts.", func(x map[string]any) {
		x["parts"] = append(x["parts"].([]any), x["parts"].([]any)[0])
	})
	if _, err := m.Give(p.Reference(), bad, envelope(t, 1, bad, p), "2026-11-12T09:00:00Z"); !rejected(err, mediator.OnePart) {
		t.Fatalf("got %v", err)
	}
	if len(m.Receipts()) != 0 || len(m.Pending()) != 0 {
		t.Fatal("a rejected give wrote a receipt or opened a question")
	}
	if d, err := m.Get(ap.Reference()); d != nil || err != nil {
		t.Fatalf("a rejected give queued %s, %v", d, err)
	}
	good := message(t, m.Accord(), "good", ap.Reference(), "note", "One part.", nil)
	out, err := m.Give(p.Reference(), good, envelope(t, 1, good, p), "2026-11-12T09:01:00Z")
	if err != nil || out.Verdict != "pass" {
		t.Fatalf("got %+v, %v", out, err)
	}
	st, err := verifyAll(ptesting.Key(t, "mediator").Reference(), m.Receipts())
	if err != nil || len(st) != 1 || st[0].Predicate.Envelope.Seq != 1 || st[0].Predicate.Seq != 1 || st[0].Predicate.Prev != "" {
		t.Fatalf("got %+v, %v", st, err)
	}
}

// TestTheSenderIsTheCaller gives a message whose extension names a from, and a give whose envelope
// another member signed: extension-missing and envelope-signature, since the caller is the sender
// and no member of a message can name another (crossing-1, crossing-6).
func TestTheSenderIsTheCaller(t *testing.T) {
	t.Parallel()
	m := found(t)
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	for _, from := range []string{p.Reference(), ap.Reference(), "anyone"} {
		msg := message(t, m.Accord(), "from", ap.Reference(), "note", "A note.", func(x map[string]any) {
			x["metadata"].(map[string]any)[extension].(map[string]any)["from"] = from
		})
		if _, err := m.Give(p.Reference(), msg, envelope(t, 1, msg, p), "2026-11-12T09:00:00Z"); !rejected(err, mediator.ExtensionMissing) {
			t.Fatalf("a from of %s: %v", from, err)
		}
	}
	msg := message(t, m.Accord(), "signed", ap.Reference(), "note", "A note.", nil)
	if _, err := m.Give(p.Reference(), msg, envelope(t, 1, msg, ap), "2026-11-12T09:00:00Z"); !rejected(err, mediator.EnvelopeSignature) {
		t.Fatalf("an envelope by another member: %v", err)
	}
	if len(m.Receipts()) != 0 {
		t.Fatal("a rejection wrote a receipt")
	}
}

// TestReadsAreCopies changes what Receipts, Pending, Get and a give's outcome return, and reads the
// mediator again (results-are-copies).
func TestReadsAreCopies(t *testing.T) {
	t.Parallel()
	m := found(t)
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	note := message(t, m.Accord(), "n", ap.Reference(), "note", "A note.", nil)
	out, err := m.Give(p.Reference(), note, envelope(t, 1, note, p), "2026-11-12T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	prop := message(t, m.Accord(), "q", ap.Reference(), "proposal", "A proposal.", nil)
	if _, err := m.Give(p.Reference(), prop, envelope(t, 2, prop, p), "2026-11-12T09:01:00Z"); err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), m.Receipts()[0]...)
	out.Receipt[0] = 'x'
	r := m.Receipts()
	r[0][0] = 'x'
	r[1] = nil
	pend := m.Pending()
	pend[0] = "changed"
	if !bytes.Equal(m.Receipts()[0], want) || m.Receipts()[1] == nil || m.Pending()[0] == "changed" {
		t.Fatal("a read shares the mediator's state")
	}
	d, _ := m.Get(ap.Reference())
	d[0] = 'x'
	if again, _ := m.Get(ap.Reference()); again != nil {
		t.Fatal("a delivery was handed out twice")
	}
}

// TestFoundOnlyItsOwnSets founds a set naming another mediator, and one founding refuses.
func TestFoundOnlyItsOwnSets(t *testing.T) {
	t.Parallel()
	set, at := askAndLog(t)
	if _, err := mediator.Found(set, ptesting.Key(t, "other-mediator"), at); !errors.Is(err, mediator.ErrOtherMediator) {
		t.Fatalf("got %v", err)
	}
	_, err := mediator.Found(set[1:], ptesting.Key(t, "mediator"), at)
	var se *setup.Error
	if !errors.As(err, &se) || se.Code != setup.NotCanonical {
		t.Fatalf("got %v", err)
	}
	m := found(t)
	if _, err := m.Give(ptesting.Key(t, "proposer").Reference(), nil, nil, "2026-11-12 09:00:00"); err == nil || err.Error() != "mediator: at" {
		t.Fatalf("a time off the timestamp rule: %v", err)
	}
}

// TestErrorTextQuotesNoInput triggers every rejection of the door with a planted body, salt, key,
// message and payload, and reads each error: the package and the code alone
// (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	const secret = "s3cr3t-body-salt-seed-key-payload"
	m := found(t)
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	plant := func(f func(x map[string]any)) func(x map[string]any) {
		return func(x map[string]any) {
			x["messageId"] = secret
			if f != nil {
				f(x)
			}
		}
	}
	part := func(x map[string]any) map[string]any { return x["parts"].([]any)[0].(map[string]any) }
	cases := []struct {
		code   mediator.RejectionCode
		caller signing.Key
		change func(x map[string]any)
		env    func(msg []byte) []byte
		raw    []byte
	}{
		{mediator.NotAMember, ptesting.Key(t, "stranger"), nil, nil, nil},
		{mediator.EnvelopeSignature, p, nil, func(msg []byte) []byte { return envelope(t, 1, msg, ap) }, nil},
		{mediator.NotCanonical, p, nil, nil, []byte(`{"messageId":"` + secret + `","messageId":1}`)},
		{mediator.EnvelopeHash, p, nil, func(msg []byte) []byte { return envelope(t, 1, []byte("{}"), p) }, nil},
		{mediator.EnvelopeSequence, p, nil, func(msg []byte) []byte { return envelope(t, 0, msg, p) }, nil},
		{mediator.ExtensionMissing, p, func(x map[string]any) { x[secret] = secret }, nil, nil},
		{mediator.ReceiverUnknown, p, func(x map[string]any) { x["metadata"].(map[string]any)[extension].(map[string]any)["to"] = secret }, nil, nil},
		{mediator.AccordMismatch, p, func(x map[string]any) { x["metadata"].(map[string]any)[extension].(map[string]any)["accord"] = secret }, nil, nil},
		{mediator.OnePart, p, func(x map[string]any) { x["parts"] = secret }, nil, nil},
		{mediator.OneBody, p, func(x map[string]any) { part(x)["data"] = secret }, nil, nil},
		{mediator.URLUnsupported, p, func(x map[string]any) { delete(part(x), "text"); part(x)["url"] = secret }, nil, nil},
		{mediator.PartMetadata, p, func(x map[string]any) {
			part(x)["metadata"].(map[string]any)[extension].(map[string]any)["salt"] = hex.EncodeToString([]byte(secret)[:15])
		}, nil, nil},
	}
	for _, c := range cases {
		msg := c.raw
		if msg == nil {
			msg = message(t, m.Accord(), "x", ap.Reference(), "note", secret, plant(c.change))
		}
		env := envelope(t, 1, msg, c.caller)
		if c.env != nil {
			env = c.env(msg)
		}
		_, err := m.Give(c.caller.Reference(), msg, env, "2026-11-12T09:00:00Z")
		if !rejected(err, c.code) || err.Error() != "mediator: rejected: "+string(c.code) || strings.Contains(err.Error(), secret) {
			t.Fatalf("got %v, want %s", err, c.code)
		}
	}
}

// FuzzGive gives any message and envelope bytes as any caller at any time, after a good give. It
// never panics; a rejection writes nothing, and every receipt written verifies.
func FuzzGive(f *testing.F) {
	m0 := found(f)
	p, ap := ptesting.Key(f, "proposer"), ptesting.Key(f, "applier")
	note := message(f, m0.Accord(), "n", ap.Reference(), "note", "A note.", nil)
	prop := message(f, m0.Accord(), "q", ap.Reference(), "proposal", "A proposal.", nil)
	f.Add(note, envelope(f, 2, note, p), p.Reference(), "2026-11-12T09:01:00Z")
	f.Add(prop, envelope(f, 5, prop, p), p.Reference(), "2026-11-12T09:01:00Z")
	f.Add(note, envelope(f, 1, note, p), ap.Reference(), "2026-11-12T09:01:00Z")
	f.Add([]byte(`{}`), []byte(`{"seq":1}`), "", "")
	med := ptesting.Key(f, "mediator").Reference()
	for _, in := range seeds.Inputs(f, "crossing", "message") {
		f.Add(in, envelope(f, 2, note, p), p.Reference(), "2026-11-12T09:01:00Z")
	}
	f.Fuzz(func(t *testing.T, message, envelope []byte, caller, at string) {
		m := found(t)
		first := note
		if _, err := m.Give(p.Reference(), first, envelopeFor(t, first, p), "2026-11-12T09:00:00Z"); err != nil {
			t.Fatal(err)
		}
		before := len(m.Receipts())
		_, err := m.Give(caller, message, envelope, at)
		if err != nil && len(m.Receipts()) != before {
			t.Fatal("a rejected give wrote a receipt")
		}
		if _, err := receipts.Verify(med, m.Receipts()); err != nil {
			t.Fatalf("the receipts do not verify: %v", err)
		}
		_, _ = m.Get(caller)
	})
}

func envelopeFor(t testing.TB, msg []byte, k signing.Key) []byte { return envelope(t, 1, msg, k) }

// FuzzFound founds any set with the mediator's key. It never panics.
func FuzzFound(f *testing.F) {
	set, at := askAndLog(f)
	f.Add(set, at)
	f.Add([]byte(`{}`), at)
	for _, in := range seeds.Inputs(f, "founding") {
		f.Add(in, at)
	}
	k := ptesting.Key(f, "mediator")
	f.Fuzz(func(t *testing.T, set []byte, at string) {
		m, err := mediator.Found(set, k, at)
		if err == nil && (len(m.Receipts()) != 0 || len(m.Pending()) != 0 || m.Accord() == "") {
			t.Fatal("a mediator founded with state")
		}
	})
}

// verifyAll verifies list under mediator and returns the statements, for tests that read them.
func verifyAll(mediator string, list [][]byte) ([]receipts.Statement, error) {
	verified, err := receipts.Verify(mediator, list)
	if err != nil {
		return nil, err
	}
	out := make([]receipts.Statement, len(verified))
	for i, v := range verified {
		out[i] = v.Statement()
	}
	return out, nil
}
