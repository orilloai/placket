package receipts_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// canonicalJSON writes a JSON value in canonical form.
func canonicalJSON(t testing.TB, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(raw)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// signed returns the receipt med signs over a statement given as a JSON value, whatever the
// statement holds, and the receipt's id (receipt-1).
func signed(t testing.TB, med signing.Key, statement map[string]any) ([]byte, string) {
	t.Helper()
	payload := canonicalJSON(t, statement)
	sig, err := signing.SignPayload(receipts.PayloadType, payload, med)
	if err != nil {
		t.Fatal(err)
	}
	id, err := canonical.TypedHash(payload)
	if err != nil {
		t.Fatal(err)
	}
	return canonicalJSON(t, map[string]any{
		"payloadType": receipts.PayloadType,
		"payload":     base64.StdEncoding.EncodeToString(payload),
		"signatures":  []any{map[string]any{"keyid": med.Reference(), "sig": base64.StdEncoding.EncodeToString(sig)}},
	}), id
}

// statementOf writes the statement of a subject and a predicate as a JSON value, with prev present
// only when the predicate holds one.
func statementOf(s receipts.Subject, p receipts.Predicate) map[string]any {
	pred := map[string]any{
		"accord": p.Accord, "card": p.Card, "mediator": p.Mediator, "kind": p.Kind,
		"from": p.From, "to": p.To, "seq": p.Seq, "at": p.At,
		"envelope": map[string]any{"sender": p.Envelope.Sender, "seq": p.Envelope.Seq, "hash": p.Envelope.Hash},
		"verdict":  p.Verdict, "reason": p.Reason,
	}
	if p.Prev != "" {
		pred["prev"] = p.Prev
	}
	return map[string]any{
		"_type":         receipts.StatementType,
		"subject":       []any{map[string]any{"name": s.Name, "digest": map[string]any{"salted-sha256": s.Digest}}},
		"predicateType": receipts.PredicateType,
		"predicate":     pred,
	}
}

// An entry is one receipt of a journal before its place in its chain is known.
type entry struct {
	sub  receipts.Subject
	pred receipts.Predicate
}

// journal signs the entries in order as med, each chained in the pair of its from and to
// (receipt-5), with med as the mediator named.
func journal(t testing.TB, med signing.Key, entries []entry) [][]byte {
	t.Helper()
	type link struct {
		seq int64
		id  string
	}
	last := map[[2]string]link{}
	var out [][]byte
	for _, e := range entries {
		p := e.pred
		pair := [2]string{p.From, p.To}
		l := last[pair]
		p.Mediator, p.Seq, p.Prev = med.Reference(), l.seq+1, l.id
		r, id := signed(t, med, statementOf(e.sub, p))
		last[pair] = link{p.Seq, id}
		out = append(out, r)
	}
	return out
}

func statementsOf(entries []entry) []receipts.Statement {
	out := make([]receipts.Statement, len(entries))
	for i, e := range entries {
		out[i] = receipts.Statement{Subject: e.sub, Predicate: e.pred}
	}
	return out
}

// digestOf is a distinct salted digest for each n.
func digestOf(n byte) string {
	return receipts.Digest([]byte("salt-of-sixteen!"), []byte{n})
}

// questionParties are the references the question tests use: the asker, the receiver, the answerer
// and a stranger.
func questionParties(t testing.TB) (asker, receiver, answerer, stranger string) {
	return key(t, "proposer").Reference(), key(t, "applier").Reference(), key(t, "decider").Reference(),
		key(t, "stranger").Reference()
}

// predicateOf is a predicate of one kind, pair, verdict and reason, echoing env.
func predicateOf(kind, from, to, verdict, reason string, env receipts.Envelope) receipts.Predicate {
	return receipts.Predicate{
		Accord: accord, Card: card, Kind: kind, From: from, To: to, At: "2026-11-12T09:00:00Z",
		Envelope: env, Verdict: verdict, Reason: reason,
	}
}

// TestChainsPerPair holds receipt-5 and section 8 row 6: the first receipt of a pair has seq 1 and
// no prev, and each later one seq one above the previous receipt of its pair and prev that receipt's
// id; a receipt that breaks either fails chain at its index, and pairs chain apart.
func TestChainsPerPair(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	from, to, other, _ := questionParties(t)
	link := func(n byte, from, to string, seq int64, prev string) []byte {
		p := predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: seq, Hash: mhash})
		p.Mediator, p.Seq, p.Prev = med.Reference(), seq, prev
		r, _ := signed(t, med, statementOf(receipts.Subject{Name: "note", Digest: digestOf(n)}, p))
		return r
	}
	first, firstID := signed(t, med, statementOf(receipts.Subject{Name: "note", Digest: digestOf(1)}, func() receipts.Predicate {
		p := predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: 1, Hash: mhash})
		p.Mediator, p.Seq = med.Reference(), 1
		return p
	}()))
	for _, c := range []struct {
		name  string
		list  [][]byte
		index int // -1 when the list verifies
	}{
		{"two links and the reverse pair", [][]byte{first, link(2, to, from, 1, ""), link(3, from, to, 2, firstID)}, -1},
		{"another pair's first after a link", [][]byte{first, link(2, from, other, 1, "")}, -1},
		{"seq 3 after seq 1 with the right prev", [][]byte{first, link(2, from, to, 3, firstID)}, 1},
		{"seq 1 again with the right prev", [][]byte{first, link(2, from, to, 1, firstID)}, 1},
		{"seq 2 with another prev", [][]byte{first, link(2, from, to, 2, mhash)}, 1},
		{"seq 2 with no prev", [][]byte{first, link(2, from, to, 2, "")}, 1},
		{"a first receipt at seq 2 with a prev", [][]byte{first, link(2, from, other, 2, firstID)}, 1},
		{"a first receipt at seq 2 with no prev", [][]byte{first, link(2, from, other, 2, "")}, 1},
		{"a first receipt at seq 1 with a prev", [][]byte{first, link(2, from, other, 1, mhash)}, 1},
		{"the very first receipt at seq 2 with no prev", [][]byte{link(2, from, to, 2, "")}, 0},
		{"the very first receipt at seq 1 with a prev", [][]byte{link(2, from, to, 1, firstID)}, 0},
		{"the reverse pair continuing this one", [][]byte{first, link(2, to, from, 2, firstID)}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st, err := receipts.Verify(med.Reference(), c.list)
			if c.index < 0 {
				if err != nil || len(st) != len(c.list) {
					t.Fatalf("got %d statements, %v", len(st), err)
				}
				return
			}
			if !isCode(err, receipts.CodeChain, c.index) {
				t.Fatalf("got %v, want chain at index %d", err, c.index)
			}
		})
	}
}

// TestReleaseForm holds receipt-6's rule 4 and section 8 row 7: after an accept, the release is a
// crossing receipt from the question's from to its to, with the question's subject, name and
// digest; after a change, a resolution receipt from the answerer to the question's to, with the
// changed version's subject, and the question's to is neither the asker nor the answerer. Each
// receipt that breaks one of these alone fails release at its index; the lists that keep them
// verify, each receipt in its place.
func TestReleaseForm(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	asker, receiver, answerer, stranger := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	answer := receipts.Subject{Name: receipts.NameAnswer, Digest: digestOf(2)}
	changed := receipts.Subject{Name: receipts.NameChanged, Digest: digestOf(3)}
	ask := func(to string) entry {
		return entry{asked, predicateOf(receipts.KindCrossing, asker, to, "escalate", "maybe", env)}
	}
	accept := entry{answer, predicateOf(receipts.KindResolution, answerer, asker, "pass", "ok", env)}
	change := entry{changed, predicateOf(receipts.KindResolution, answerer, asker, "pass", "ok", env)}
	acceptRelease := func(change func(e *entry)) entry {
		e := entry{asked, predicateOf(receipts.KindCrossing, asker, receiver, "pass", "ok", env)}
		change(&e)
		return e
	}
	changeRelease := func(to string, change func(e *entry)) entry {
		e := entry{changed, predicateOf(receipts.KindResolution, answerer, to, "pass", "ok", env)}
		change(&e)
		return e
	}
	same := func(*entry) {}

	wantPlaces := []receipts.Place{
		{Role: receipts.RoleQuestion, Question: 0}, {Role: receipts.RoleClosing, Question: 0}, {Role: receipts.RoleRelease, Question: 0},
	}
	for name, entries := range map[string][]entry{
		"an accept and its release": {ask(receiver), accept, acceptRelease(same)},
		"an accept and its blocked release": {ask(receiver), accept, acceptRelease(func(e *entry) {
			e.pred.Verdict, e.pred.Reason = "block", "no-match"
		})},
		"a change and its release": {ask(receiver), change, changeRelease(receiver, same)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			st, err := receipts.Verify(med.Reference(), journal(t, med, entries))
			if err != nil || len(st) != 3 {
				t.Fatalf("got %d statements, %v", len(st), err)
			}
			places, err := receipts.Releases(st)
			if err != nil || !reflect.DeepEqual(places, wantPlaces) {
				t.Fatalf("places %+v, %v", places, err)
			}
		})
	}

	for _, c := range []struct {
		name    string
		entries []entry
		// signable is false when row 4 refuses the release before row 7 reads it, so that only
		// Releases, given the statements, can read it.
		signable bool
	}{
		{"an accept's release as a resolution", []entry{ask(receiver), accept, acceptRelease(func(e *entry) {
			e.pred.Kind = receipts.KindResolution
		})}, true},
		{"an accept's release to another party", []entry{ask(receiver), accept, acceptRelease(func(e *entry) {
			e.pred.To = stranger
		})}, true},
		{"an accept's release of another subject name", []entry{ask(receiver), accept, acceptRelease(func(e *entry) {
			e.sub.Name = "note"
		})}, true},
		{"an accept's release of another subject digest", []entry{ask(receiver), accept, acceptRelease(func(e *entry) {
			e.sub.Digest = digestOf(4)
		})}, true},
		{"an accept's release from another party", []entry{ask(receiver), accept, acceptRelease(func(e *entry) {
			e.pred.From = stranger
		})}, false},
		{"a change's release as a crossing", []entry{ask(receiver), change, changeRelease(receiver, func(e *entry) {
			e.pred.Kind = receipts.KindCrossing
		})}, false},
		{"a change's release from another party", []entry{ask(receiver), change, changeRelease(receiver, func(e *entry) {
			e.pred.From = stranger
		})}, true},
		{"a change's release to another party", []entry{ask(receiver), change, changeRelease(stranger, same)}, true},
		{"a change's release of another subject name", []entry{ask(receiver), change, changeRelease(receiver, func(e *entry) {
			e.sub.Name = receipts.NameAnswer
		})}, true},
		{"a change's release of another subject digest", []entry{ask(receiver), change, changeRelease(receiver, func(e *entry) {
			e.sub.Digest = digestOf(4)
		})}, true},
		{"a change's release to an asker who asked itself", []entry{ask(asker), change, changeRelease(asker, same)}, true},
		{"a change's release to an answerer who was asked", []entry{ask(answerer), change, changeRelease(answerer, same)}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := receipts.ReleasesOf(statementsOf(c.entries)); !isCode(err, receipts.CodeRelease, 2) {
				t.Fatalf("Releases: got %v, want release at index 2", err)
			}
			if !c.signable {
				return
			}
			if _, err := receipts.Verify(med.Reference(), journal(t, med, c.entries)); !isCode(err, receipts.CodeRelease, 2) {
				t.Fatalf("Verify: got %v, want release at index 2", err)
			}
		})
	}
}

// TestAQuestionKeyAfterItsQuestionClosed holds receipt-6's rules 1 and 2: a second ask that echoes a
// closed question's envelope opens a question of its own, and the decline after it closes the
// second question, not the first.
func TestAQuestionKeyAfterItsQuestionClosed(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	first := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	second := receipts.Subject{Name: "proposal", Digest: digestOf(2)}
	list := journal(t, med, []entry{
		{first, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)},
		{first, predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonDeclined, env)},
		{second, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)},
		{second, predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonDeclined, env)},
	})
	st, err := receipts.Verify(med.Reference(), list)
	if err != nil || len(st) != 4 {
		t.Fatalf("got %d statements, %v", len(st), err)
	}
	places, err := receipts.Releases(st)
	want := []receipts.Place{
		{Role: receipts.RoleQuestion, Question: 0}, {Role: receipts.RoleClosing, Question: 0},
		{Role: receipts.RoleQuestion, Question: 2}, {Role: receipts.RoleClosing, Question: 2},
	}
	if err != nil || !reflect.DeepEqual(places, want) {
		t.Fatalf("places %+v, %v; want %+v", places, err, want)
	}
}

// withPaddingBit sets the lowest padding bit of standard base64 text that ends in padding.
func withPaddingBit(t testing.TB, s string) string {
	t.Helper()
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	at := strings.IndexByte(s, '=') - 1
	if at < 0 {
		t.Fatalf("no padding in %d characters", len(s))
	}
	return s[:at] + string(alphabet[strings.IndexByte(alphabet, s[at])|1]) + s[at+1:]
}

// TestBase64InItsOneEncoding holds section 8 row 3 and receipt-1: payload and sig are standard
// base64 in the one encoding that encodes back to the same text, and the signature entry has
// exactly keyid and sig. A newline inside either, nonzero padding bits, or a third member of the
// entry fails signature at that receipt's index, though a lenient decoder reads the same bytes.
func TestBase64InItsOneEncoding(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	from, to, _, _ := questionParties(t)
	// The second receipt's payload must end in padding; the subject's name sets its length.
	var list [][]byte
	for _, name := range []string{"note", "notes", "notess"} {
		sub := receipts.Subject{Name: name, Digest: digestOf(1)}
		list = journal(t, med, []entry{
			{sub, predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: 1, Hash: mhash})},
			{sub, predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: 2, Hash: mhash})},
		})
		var r map[string]any
		if err := json.Unmarshal(list[1], &r); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(r["payload"].(string), "=") {
			break
		}
	}
	if _, err := receipts.Verify(med.Reference(), list); err != nil {
		t.Fatalf("the list does not verify: %v", err)
	}
	newline := func(s string) string { return s[:4] + "\n" + s[4:] }
	for _, c := range []struct {
		name    string
		edit    func(r map[string]any)
		lenient string // the member a lenient decoder reads as the same bytes, if any
	}{
		{"a newline in the payload", func(r map[string]any) { r["payload"] = newline(r["payload"].(string)) }, "payload"},
		{"a padding bit in the payload", func(r map[string]any) { r["payload"] = withPaddingBit(t, r["payload"].(string)) }, "payload"},
		{"a newline in the sig", func(r map[string]any) {
			e := r["signatures"].([]any)[0].(map[string]any)
			e["sig"] = newline(e["sig"].(string))
		}, "sig"},
		{"a padding bit in the sig", func(r map[string]any) {
			e := r["signatures"].([]any)[0].(map[string]any)
			e["sig"] = withPaddingBit(t, e["sig"].(string))
		}, "sig"},
		{"a third member of the signature entry", func(r map[string]any) {
			r["signatures"].([]any)[0].(map[string]any)["note"] = "x"
		}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var before, after map[string]any
			if err := json.Unmarshal(list[1], &before); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(list[1], &after); err != nil {
				t.Fatal(err)
			}
			c.edit(after)
			edited := canonicalJSON(t, after)
			if bytes.Equal(edited, list[1]) {
				t.Fatal("the edit changed nothing")
			}
			if c.lenient != "" {
				text := func(r map[string]any) string {
					if c.lenient == "payload" {
						return r["payload"].(string)
					}
					return r["signatures"].([]any)[0].(map[string]any)["sig"].(string)
				}
				was, err1 := base64.StdEncoding.DecodeString(text(before))
				now, err2 := base64.StdEncoding.DecodeString(text(after))
				if err1 != nil || err2 != nil || !bytes.Equal(was, now) {
					t.Fatalf("a lenient decoder reads other bytes: %v %v", err1, err2)
				}
			}
			if _, err := receipts.Verify(med.Reference(), [][]byte{list[0], edited}); !isCode(err, receipts.CodeSignature, 1) {
				t.Fatalf("got %v, want signature at index 1", err)
			}
		})
	}
}

// TestStatementShape holds section 8 row 4 (receipt-1, receipt-2, receipt-3, receipt-9,
// receipt-10): a receipt the mediator signs whose statement breaks one rule of its members or
// values fails statement-type at its index, the statement written as given.
func TestStatementShape(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	from, to, answerer, other := questionParties(t)
	prior := journal(t, med, []entry{{receipts.Subject{Name: "note", Digest: digestOf(1)},
		predicateOf(receipts.KindCrossing, other, to, "pass", "ok", receipts.Envelope{Sender: other, Seq: 1, Hash: mhash})}})
	// base is a crossing pass from from to to, the first of its pair.
	base := func() map[string]any {
		p := predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: 1, Hash: mhash})
		p.Mediator, p.Seq = med.Reference(), 1
		return statementOf(receipts.Subject{Name: "note", Digest: digestOf(2)}, p)
	}
	subject := func(s map[string]any) map[string]any { return s["subject"].([]any)[0].(map[string]any) }
	digest := func(s map[string]any) map[string]any { return subject(s)["digest"].(map[string]any) }
	pred := func(s map[string]any) map[string]any { return s["predicate"].(map[string]any) }
	envelope := func(s map[string]any) map[string]any { return pred(s)["envelope"].(map[string]any) }
	// resolution makes the receipt a resolution from the answerer, so that the envelope's sender may
	// differ from its from; with no question before it, it fails release once row 4 passes.
	resolution := func(s map[string]any) {
		pred(s)["kind"], pred(s)["from"] = receipts.KindResolution, answerer
	}

	t.Run("the base statement", func(t *testing.T) {
		t.Parallel()
		r, _ := signed(t, med, base())
		if _, err := receipts.Verify(med.Reference(), [][]byte{prior[0], r}); err != nil {
			t.Fatalf("the base statement does not verify: %v", err)
		}
		s := base()
		resolution(s)
		r, _ = signed(t, med, s)
		if _, err := receipts.Verify(med.Reference(), [][]byte{prior[0], r}); !isCode(err, receipts.CodeRelease, 1) {
			t.Fatalf("the base resolution: got %v, want release at index 1", err)
		}
	})

	for name, change := range map[string]func(s map[string]any){
		"a fifth statement member":          func(s map[string]any) { s["note"] = "x" },
		"no _type":                          func(s map[string]any) { delete(s, "_type") },
		"another _type":                     func(s map[string]any) { s["_type"] = "https://in-toto.io/Statement/v0.1" },
		"another predicateType":             func(s map[string]any) { s["predicateType"] = "https://placket.dev/receipt/0.2" },
		"no predicateType":                  func(s map[string]any) { delete(s, "predicateType") },
		"a subject that is no list":         func(s map[string]any) { s["subject"] = subject(s) },
		"no subject entry":                  func(s map[string]any) { s["subject"] = []any{} },
		"two subject entries":               func(s map[string]any) { s["subject"] = []any{subject(s), subject(s)} },
		"a third subject member":            func(s map[string]any) { subject(s)["uri"] = "x" },
		"a subject with no name":            func(s map[string]any) { delete(subject(s), "name") },
		"a name that is no label":           func(s map[string]any) { subject(s)["name"] = "Note" },
		"a name that is no string":          func(s map[string]any) { subject(s)["name"] = 1 },
		"a digest that is no object":        func(s map[string]any) { subject(s)["digest"] = digestOf(2) },
		"a second digest member":            func(s map[string]any) { digest(s)["sha256"] = digestOf(2) },
		"a digest of another name":          func(s map[string]any) { subject(s)["digest"] = map[string]any{"sha256": digestOf(2)} },
		"a digest of 66 hex digits":         func(s map[string]any) { digest(s)["salted-sha256"] = digestOf(2) + "ab" },
		"a digest of 62 hex digits":         func(s map[string]any) { digest(s)["salted-sha256"] = digestOf(2)[:62] },
		"a digest in uppercase hex":         func(s map[string]any) { digest(s)["salted-sha256"] = strings.Repeat("AB", 32) },
		"a digest with a prefix":            func(s map[string]any) { digest(s)["salted-sha256"] = "sha256:" + digestOf(2)[7:] },
		"a predicate member result":         func(s map[string]any) { pred(s)["result"] = "pass" },
		"no accord":                         func(s map[string]any) { delete(pred(s), "accord") },
		"no card":                           func(s map[string]any) { delete(pred(s), "card") },
		"no kind":                           func(s map[string]any) { delete(pred(s), "kind") },
		"no from":                           func(s map[string]any) { delete(pred(s), "from") },
		"no to":                             func(s map[string]any) { delete(pred(s), "to") },
		"no at":                             func(s map[string]any) { delete(pred(s), "at") },
		"no envelope":                       func(s map[string]any) { delete(pred(s), "envelope") },
		"no verdict":                        func(s map[string]any) { delete(pred(s), "verdict") },
		"no reason":                         func(s map[string]any) { delete(pred(s), "reason") },
		"an accord that is no hash":         func(s map[string]any) { pred(s)["accord"] = strings.TrimPrefix(accord, "sha256:") },
		"a card that is no hash":            func(s map[string]any) { pred(s)["card"] = strings.ToUpper(card) },
		"a from that is no reference":       func(s map[string]any) { resolution(s); pred(s)["from"] = "decider" },
		"a to that is no reference":         func(s map[string]any) { pred(s)["to"] = "applier" },
		"an at that is no timestamp":        func(s map[string]any) { pred(s)["at"] = "2026-11-12T09:00:00+00:00" },
		"an at with a leap second":          func(s map[string]any) { pred(s)["at"] = "2026-11-12T23:59:60Z" },
		"an envelope that is no object":     func(s map[string]any) { pred(s)["envelope"] = mhash },
		"a fourth envelope member":          func(s map[string]any) { envelope(s)["at"] = "2026-11-12T09:00:00Z" },
		"an envelope with no hash":          func(s map[string]any) { delete(envelope(s), "hash") },
		"a sender that is no reference":     func(s map[string]any) { resolution(s); envelope(s)["sender"] = "proposer" },
		"an envelope seq 0":                 func(s map[string]any) { envelope(s)["seq"] = 0 },
		"an envelope seq as a string":       func(s map[string]any) { envelope(s)["seq"] = "1" },
		"an envelope seq -1":                func(s map[string]any) { envelope(s)["seq"] = -1 },
		"an envelope seq true":              func(s map[string]any) { envelope(s)["seq"] = true },
		"an envelope hash that is no hash":  func(s map[string]any) { envelope(s)["hash"] = strings.TrimPrefix(mhash, "sha256:") },
		"a crossing echoing another sender": func(s map[string]any) { envelope(s)["sender"] = answerer },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := base()
			change(s)
			r, _ := signed(t, med, s)
			if _, err := receipts.Verify(med.Reference(), [][]byte{prior[0], r}); !isCode(err, receipts.CodeStatementType, 1) {
				t.Fatalf("got %v, want statement-type at index 1", err)
			}
		})
	}
}

// TestVerdictsAndReasons holds receipt-4 through section 8 row 4: the closed list of reasons, pass
// with ok, escalate with maybe on crossing receipts only, block with any other reason, and declined,
// cancelled, timeout and undeliverable on resolution receipts only, of the two kinds alone. Each
// receipt follows an ask: a crossing one is a give of its own, a resolution one the ask's closing.
func TestVerdictsAndReasons(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	ask := entry{asked, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)}
	for _, c := range []struct {
		kind, verdict, reason string
		ok                    bool
	}{
		{receipts.KindCrossing, "pass", "ok", true},
		{receipts.KindCrossing, "pass", "no-match", false},
		{receipts.KindCrossing, "pass", "maybe", false},
		{receipts.KindCrossing, "pass", "check:size", false},
		{receipts.KindCrossing, "block", "ok", false},
		{receipts.KindCrossing, "block", "no-match", true},
		{receipts.KindCrossing, "block", "maybe", true},
		{receipts.KindCrossing, "block", "check:size", true},
		{receipts.KindCrossing, "block", "check:max-size", true},
		{receipts.KindCrossing, "block", "check:", false},
		{receipts.KindCrossing, "block", "check:Size", false},
		{receipts.KindCrossing, "block", "size", false},
		{receipts.KindCrossing, "block", "", false},
		{receipts.KindCrossing, "block", receipts.ReasonDeclined, false},
		{receipts.KindCrossing, "block", receipts.ReasonCancelled, false},
		{receipts.KindCrossing, "block", receipts.ReasonTimeout, false},
		{receipts.KindCrossing, "block", receipts.ReasonUndeliverable, false},
		{receipts.KindCrossing, "escalate", "maybe", true},
		{receipts.KindCrossing, "escalate", "no-match", false},
		{receipts.KindCrossing, "escalate", "ok", false},
		{receipts.KindCrossing, "allow", "ok", false},
		{receipts.KindCrossing, "", "ok", false},
		{"other", "pass", "ok", false},
		{"", "pass", "ok", false},
		{receipts.KindResolution, "pass", "ok", true},
		{receipts.KindResolution, "pass", "no-match", false},
		{receipts.KindResolution, "pass", receipts.ReasonDeclined, false},
		{receipts.KindResolution, "block", "ok", false},
		{receipts.KindResolution, "block", "no-match", true},
		{receipts.KindResolution, "block", "maybe", true},
		{receipts.KindResolution, "block", "check:size", true},
		{receipts.KindResolution, "block", "size", false},
		{receipts.KindResolution, "block", receipts.ReasonDeclined, true},
		{receipts.KindResolution, "block", receipts.ReasonCancelled, true},
		{receipts.KindResolution, "block", receipts.ReasonTimeout, true},
		{receipts.KindResolution, "block", receipts.ReasonUndeliverable, true},
		{receipts.KindResolution, "escalate", "maybe", false},
		{receipts.KindResolution, "escalate", "no-match", false},
	} {
		t.Run(c.kind+" "+c.verdict+" "+c.reason, func(t *testing.T) {
			t.Parallel()
			var e entry
			switch c.kind {
			case receipts.KindResolution:
				sub := receipts.Subject{Name: receipts.NameAnswer, Digest: digestOf(2)}
				switch c.reason {
				case receipts.ReasonDeclined, receipts.ReasonCancelled, receipts.ReasonTimeout, receipts.ReasonUndeliverable:
					sub = asked
				}
				e = entry{sub, predicateOf(c.kind, answerer, asker, c.verdict, c.reason, env)}
			default:
				e = entry{receipts.Subject{Name: "note", Digest: digestOf(2)},
					predicateOf(c.kind, asker, receiver, c.verdict, c.reason, receipts.Envelope{Sender: asker, Seq: 2, Hash: mhash})}
			}
			_, err := receipts.Verify(med.Reference(), journal(t, med, []entry{ask, e}))
			if c.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !c.ok && !isCode(err, receipts.CodeStatementType, 1) {
				t.Fatalf("got %v, want statement-type at index 1", err)
			}
		})
	}
}

// TestReconcileReadsAccordCardAndKept holds section 9 rows 2 and 3 (receipt-7): a presented receipt of the
// expected Accord and another card fails presented at its index; a kept delivered receipt of
// another card, one naming another mediator, one that blocked and one addressed to another party
// each fail kept with the party and the receipt's index in the party's list.
func TestReconcileReadsAccordCardAndKept(t *testing.T) {
	t.Parallel()
	med := key(t, "mediator")
	from, to, _, _ := questionParties(t)
	const otherCard = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	give := func(seq int64, crd string) entry {
		p := predicateOf(receipts.KindCrossing, from, to, "pass", "ok", receipts.Envelope{Sender: from, Seq: seq, Hash: mhash})
		p.Card = crd
		return entry{receipts.Subject{Name: "note", Digest: digestOf(byte(seq))}, p}
	}
	good := journal(t, med, []entry{give(1, card), give(2, card)})
	if _, err := receipts.Reconcile(med.Reference(), accord, card, good,
		[]receipts.Kept{{Actor: to, Delivered: good}}); err != nil {
		t.Fatalf("the good record does not reconcile: %v", err)
	}

	t.Run("a presented receipt of another card", func(t *testing.T) {
		t.Parallel()
		list := journal(t, med, []entry{give(1, card), give(2, otherCard)})
		if _, err := receipts.Reconcile(med.Reference(), accord, card, list, nil); !isCode(err, receipts.CodePresented, 1) {
			t.Fatalf("got %v, want presented at index 1", err)
		}
	})

	// Row 3 reads rows 1 to 5 of a kept receipt, not its chain.
	otherMediator := func() []byte {
		e := give(2, card)
		e.pred.Mediator, e.pred.Seq, e.pred.Prev = key(t, "other-mediator").Reference(), 2, mhash
		r, _ := signed(t, med, statementOf(e.sub, e.pred))
		return r
	}
	for name, kept := range map[string][]byte{
		"a kept receipt of another card":     journal(t, med, []entry{give(1, card), give(2, otherCard)})[1],
		"a kept receipt of another mediator": otherMediator(),
		"a kept receipt that blocked": journal(t, med, []entry{give(1, card), func() entry {
			e := give(2, card)
			e.pred.Verdict, e.pred.Reason = "block", "no-match"
			return e
		}()})[1],
		"a kept receipt to another party": journal(t, med, []entry{give(1, card), func() entry {
			e := give(2, card)
			e.pred.To = key(t, "stranger").Reference()
			return e
		}()})[1],
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := receipts.Reconcile(med.Reference(), accord, card, good[:1],
				[]receipts.Kept{{Actor: to, Delivered: [][]byte{good[0], kept}}})
			var re *receipts.Error
			if !errors.As(err, &re) || *re != (receipts.Error{Code: receipts.CodeKept, Index: 1, Actor: to}) {
				t.Fatalf("got %v, want kept at index 1 of the receiver", err)
			}
		})
	}
}
