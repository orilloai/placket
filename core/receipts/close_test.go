package receipts_test

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// closeAt is the time of the closes these tests write.
const closeAt = "2026-11-12T09:10:00Z"

// closedLog is a list the close tests share, in the order a close writes it: a note from the asker
// to the receiver, which passed and was never fetched; an ask from the asker to the receiver; a
// second note, which passed; and the ask's closed closing from the answerer, at closeAt. It returns
// the receipts and their ids.
func closedLog(t testing.TB, med signing.Key) ([][]byte, []string) {
	t.Helper()
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 2, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	closing := predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonClosed, env)
	closing.At = closeAt
	entries := []entry{
		{receipts.Subject{Name: "note", Digest: digestOf(2)}, predicateOf(receipts.KindCrossing, asker, receiver, "pass", "ok", receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash})},
		{asked, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)},
		{receipts.Subject{Name: "note", Digest: digestOf(3)}, predicateOf(receipts.KindCrossing, asker, receiver, "pass", "ok", receipts.Envelope{Sender: asker, Seq: 3, Hash: mhash})},
		{asked, closing},
	}
	list := journal(t, med, entries)
	ids := make([]string, len(list))
	for i, r := range list {
		s, err := receipts.Decode(r)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = s.ID
	}
	return list, ids
}

// closeOfLog is the close of closedLog's list, as close-2 gives it: its two chains, ordered by from,
// each at its last receipt, and the first note, never fetched.
func closeOfLog(t testing.TB, ids []string) receipts.Close {
	t.Helper()
	asker, receiver, answerer, _ := questionParties(t)
	chains := []receipts.Final{{From: asker, To: receiver, Seq: 3, Receipt: ids[2]}, {From: answerer, To: asker, Seq: 1, Receipt: ids[3]}}
	if canonical.Less(answerer, asker) {
		chains[0], chains[1] = chains[1], chains[0]
	}
	return receipts.Close{Accord: accord, Card: card, At: closeAt, Chains: chains,
		Unfetched: []receipts.Unfetched{{To: receiver, Receipt: ids[0]}}}
}

func buildClose(t testing.TB, c receipts.Close, med signing.Key) []byte {
	t.Helper()
	r, _, err := receipts.BuildClose(c, med)
	if err != nil {
		t.Fatalf("BuildClose: %v", err)
	}
	return r
}

// TestBuildCloseStatement is U3 (close-2): a close's payload is an in-toto statement of the close's
// predicate type, whose one subject is the Accord by the 64 hex of its id, with the predicate's six
// members, every one present; its id is the typed hash of the statement's canonical bytes, and
// BuildClose yields it as a close and never as a receipt's statement. The time and the key each
// change the receipt (inputs-as-parameters).
func TestBuildCloseStatement(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	_, ids := closedLog(t, med)
	c := closeOfLog(t, ids)
	r, v, err := receipts.BuildClose(c, med)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		PayloadType string `json:"payloadType"`
		Payload     string `json:"payload"`
	}
	if err := json.Unmarshal(r, &env); err != nil || env.PayloadType != receipts.PayloadType {
		t.Fatalf("envelope %s: %v", r, err)
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatal(err)
	}
	asker, receiver, answerer, _ := questionParties(t)
	first, second := []string{asker, receiver, ids[2], "3"}, []string{answerer, asker, ids[3], "1"}
	if canonical.Less(answerer, asker) {
		first, second = second, first
	}
	want := `{"_type":"https://in-toto.io/Statement/v1","predicate":{"accord":"` + accord + `","at":"` + closeAt + `","card":"` + card +
		`","chains":[{"from":"` + first[0] + `","receipt":"` + first[2] + `","seq":` + first[3] + `,"to":"` + first[1] + `"},` +
		`{"from":"` + second[0] + `","receipt":"` + second[2] + `","seq":` + second[3] + `,"to":"` + second[1] + `"}],` +
		`"mediator":"` + med.Reference() + `","unfetched":[{"receipt":"` + ids[0] + `","to":"` + receiver + `"}]},` +
		`"predicateType":"https://placket.tech/close/0.1","subject":[{"digest":{"sha256":"` + strings.TrimPrefix(accord, "sha256:") + `"},"name":"accord"}]}`
	if string(payload) != want {
		t.Fatalf("statement\n%s\nwant\n%s", payload, want)
	}
	id, _ := canonical.TypedHash(payload)
	got, ok := v.Close()
	if !ok || got.ID != id || got.Mediator != med.Reference() || v.Statement().ID != "" {
		t.Fatalf("BuildClose yields %+v, %v, statement %+v; want the close %s", got, ok, v.Statement(), id)
	}
	if !signing.VerifyPayload(receipts.PayloadType, payload, sigOf(t, r), med.Reference()) {
		t.Fatal("the close's signature does not verify under the mediator's key")
	}
	if d, err := receipts.DecodeClose(r); err != nil || !reflect.DeepEqual(d, got) {
		t.Fatalf("DecodeClose: %+v, %v; want %+v", d, err, got)
	}
	if _, err := receipts.Decode(r); !isCode(err, receipts.CodeStatementType, 0) {
		t.Fatalf("Decode of a close: %v, want statement-type", err)
	}
	empty := receipts.Close{Accord: accord, Card: card, At: closeAt}
	er, _, err := receipts.BuildClose(empty, med)
	if err != nil || !strings.Contains(string(mustPayload(t, er)), `"chains":[],`) || !strings.Contains(string(mustPayload(t, er)), `"unfetched":[]}`) {
		t.Fatalf("an empty close: %s, %v; want both lists, empty", mustPayload(t, er), err)
	}
	later := c
	later.At = "2026-11-12T09:10:01Z"
	if string(buildClose(t, later, med)) == string(r) || string(buildClose(t, c, ptesting.Key(t, "other"))) == string(r) {
		t.Fatal("the close does not depend on its time and its key")
	}
	for name, bad := range map[string]func(*receipts.Close){
		"an accord that is no typed hash":   func(c *receipts.Close) { c.Accord = "accord" },
		"a time the timestamp rule refuses": func(c *receipts.Close) { c.At = "2026-11-12T24:00:00Z" },
		"a chain at seq 0":                  func(c *receipts.Close) { c.Chains[0].Seq = 0 },
		"an unfetched of no reference":      func(c *receipts.Close) { c.Unfetched[0].To = "applier" },
	} {
		cc := closeOfLog(t, ids)
		bad(&cc)
		if _, _, err := receipts.BuildClose(cc, med); !isCode(err, receipts.CodeStatement, -1) {
			t.Fatalf("%s: %v, want statement", name, err)
		}
	}
	// A close statement is refused by the receipt statement's reader, and a receipt's by the close's.
	if _, err := receipts.Verify(med.Reference(), [][]byte{signedAs(t, med, payload, func(o map[string]any) {
		o["predicateType"] = receipts.PredicateType
	})}); !isCode(err, receipts.CodeStatementType, 0) {
		t.Fatalf("a close statement under the receipt's type: %v, want statement-type", err)
	}
	for name, change := range map[string]func(map[string]any){
		"a subject of the id's text": func(o map[string]any) {
			o["subject"] = []any{map[string]any{"name": "accord", "digest": map[string]any{"sha256": strings.Repeat("0", 64)}}}
		},
		"a seq":            func(o map[string]any) { o["predicate"].(map[string]any)["seq"] = 1 },
		"no unfetched":     func(o map[string]any) { delete(o["predicate"].(map[string]any), "unfetched") },
		"a second subject": func(o map[string]any) { o["subject"] = append(o["subject"].([]any), o["subject"].([]any)[0]) },
	} {
		if _, err := receipts.Verify(med.Reference(), [][]byte{signedAs(t, med, payload, change)}); !isCode(err, receipts.CodeStatementType, 0) {
			t.Fatalf("%s: %v, want statement-type", name, err)
		}
	}
}

// sigOf is the signature bytes of a receipt's one entry.
func sigOf(t testing.TB, r []byte) []byte {
	t.Helper()
	var env struct {
		Signatures []struct{ Sig string } `json:"signatures"`
	}
	if err := json.Unmarshal(r, &env); err != nil || len(env.Signatures) != 1 {
		t.Fatalf("signatures of %s: %v", r, err)
	}
	b, err := base64.StdEncoding.DecodeString(env.Signatures[0].Sig)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// signedAs signs, as med, the statement payload holds after change.
func signedAs(t testing.TB, med signing.Key, payload []byte, change func(map[string]any)) []byte {
	t.Helper()
	var o map[string]any
	if err := json.Unmarshal(payload, &o); err != nil {
		t.Fatal(err)
	}
	change(o)
	r, _ := signed(t, med, o)
	return r
}

// TestVerifyAfterClose is U9 (close-4, section 8 row 9): a receipt the mediator signed after its
// close fails after-close at its index, and so does a second close; the list up to the close
// verifies, and yields the close as one.
func TestVerifyAfterClose(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := closedLog(t, med)
	c := buildClose(t, closeOfLog(t, ids), med)
	closed := append(append([][]byte{}, list...), c)
	v, err := receipts.Verify(med.Reference(), closed)
	if err != nil {
		t.Fatalf("a closed list: %v", err)
	}
	if got, ok := v[4].Close(); !ok || got.At != closeAt {
		t.Fatalf("the close verified as %+v, %v", got, ok)
	}
	asker, receiver, _, _ := questionParties(t)
	note := receipts.Predicate{Accord: accord, Card: card, Kind: receipts.KindCrossing, From: asker, To: receiver,
		Seq: 4, Prev: ids[2], At: closeAt, Envelope: receipts.Envelope{Sender: asker, Seq: 4, Hash: mhash},
		Verdict: "pass", Reason: "ok", By: receipts.By{Decider: receipts.ByTerms}}
	after, _, err := receipts.Build(receipts.Subject{Name: "note", Digest: digestOf(9)}, note, med)
	if err != nil {
		t.Fatal(err)
	}
	for name, l := range map[string][][]byte{
		"a give after the close": append(append([][]byte{}, closed...), after),
		"a second close":         append(append([][]byte{}, closed...), c),
	} {
		if _, err := receipts.Verify(med.Reference(), l); !isCode(err, receipts.CodeAfterClose, 5) {
			t.Fatalf("%s: %v, want after-close at index 5", name, err)
		}
		if _, err := receipts.VerifySubset(med.Reference(), l); !isCode(err, receipts.CodeAfterClose, 5) {
			t.Fatalf("%s as a subset: %v, want after-close at index 5", name, err)
		}
	}
	// An Accord with no receipts can be closed: its close has empty lists.
	empty := buildClose(t, receipts.Close{Accord: accord, Card: card, At: closeAt}, med)
	if _, err := receipts.Verify(med.Reference(), [][]byte{empty}); err != nil {
		t.Fatalf("the close of an empty list: %v", err)
	}
	// A close signed by another key fails signature, as any receipt does.
	if _, err := receipts.Verify(med.Reference(), [][]byte{buildClose(t, receipts.Close{Accord: accord, Card: card, At: closeAt}, ptesting.Key(t, "other"))}); !isCode(err, receipts.CodeSignature, 0) {
		t.Fatalf("a close of another key: %v, want signature", err)
	}
}

// TestVerifyCloseMismatch is U10 (close-4, section 8 row 8): a close whose chains are not the list's
// last receipts, pair by pair and in order, whose unfetched names a receipt that queued nothing to
// its to, or one receipt twice, or whose at is not its closed closings', fails close at its index;
// and a closed closing followed by anything but another closed closing or a close fails close at
// that receipt's index.
func TestVerifyCloseMismatch(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := closedLog(t, med)
	asker, receiver, answerer, _ := questionParties(t)
	for name, change := range map[string]func(*receipts.Close){
		"a seq that is not its chain's last": func(c *receipts.Close) {
			for i := range c.Chains {
				if c.Chains[i].From == asker {
					c.Chains[i].Seq, c.Chains[i].Receipt = 2, ids[1]
				}
			}
		},
		"a chain left out": func(c *receipts.Close) { c.Chains = c.Chains[:1] },
		"a chain the list does not hold": func(c *receipts.Close) {
			c.Chains = append(c.Chains, receipts.Final{From: receiver, To: asker, Seq: 1, Receipt: ids[0]})
		},
		"chains out of order":              func(c *receipts.Close) { c.Chains[0], c.Chains[1] = c.Chains[1], c.Chains[0] },
		"an unfetched that queued nothing": func(c *receipts.Close) { c.Unfetched[0].Receipt = ids[1] },
		"an unfetched to another member":   func(c *receipts.Close) { c.Unfetched[0].To = answerer },
		"an unfetched named twice":         func(c *receipts.Close) { c.Unfetched = append(c.Unfetched, c.Unfetched[0]) },
		"an at other than its closings'":   func(c *receipts.Close) { c.At = "2026-11-12T09:10:01Z" },
	} {
		c := closeOfLog(t, ids)
		change(&c)
		l := append(append([][]byte{}, list...), buildClose(t, c, med))
		if _, err := receipts.Verify(med.Reference(), l); !isCode(err, receipts.CodeClose, 4) {
			t.Fatalf("%s: %v, want close at index 4", name, err)
		}
	}
	// A closed closing that is not in the run right before the close: a give follows it.
	note := receipts.Predicate{Accord: accord, Card: card, Kind: receipts.KindCrossing, From: asker, To: receiver,
		Seq: 4, Prev: ids[2], At: closeAt, Envelope: receipts.Envelope{Sender: asker, Seq: 4, Hash: mhash},
		Verdict: "pass", Reason: "ok", By: receipts.By{Decider: receipts.ByTerms}}
	after, _, err := receipts.Build(receipts.Subject{Name: "note", Digest: digestOf(9)}, note, med)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := receipts.Verify(med.Reference(), append(append([][]byte{}, list...), after)); !isCode(err, receipts.CodeClose, 4) {
		t.Fatalf("a give after a closed closing: %v, want close at index 4", err)
	}
	// A torn close, its closings with no close after them, verifies: a list cut short still does.
	if _, err := receipts.Verify(med.Reference(), list); err != nil {
		t.Fatalf("a torn close: %v", err)
	}
}

// TestVerifySubsetWithClose is U12's core (close-4): of row 8 a subset checks only that each chain it
// holds ends where the close says; its chains may start late, and its unfetched is not read.
func TestVerifySubsetWithClose(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := closedLog(t, med)
	c := closeOfLog(t, ids)
	c.Unfetched = []receipts.Unfetched{{To: questionPartiesTo(t), Receipt: ids[1]}}
	closed := append(append([][]byte{}, list...), buildClose(t, c, med))
	sub, err := receipts.VerifySubset(med.Reference(), closed[2:])
	if err != nil {
		t.Fatalf("the last three as a subset: %v", err)
	}
	asker, receiver, _, _ := questionParties(t)
	if want := []receipts.Start{{From: asker, To: receiver, Seq: 3}}; !reflect.DeepEqual(sub.Starts, want) {
		t.Fatalf("starts %+v, want %+v", sub.Starts, want)
	}
	if sub.Places[2].Role != receipts.RoleClose {
		t.Fatalf("places %+v", sub.Places)
	}
	early := closeOfLog(t, ids)
	for i := range early.Chains {
		if early.Chains[i].From == asker {
			early.Chains[i].Seq, early.Chains[i].Receipt = 1, ids[0]
		}
	}
	l := [][]byte{list[0], list[3], buildClose(t, early, med)}
	if _, err := receipts.VerifySubset(med.Reference(), l); err != nil {
		t.Fatalf("a subset whose chains end where the close says: %v", err)
	}
	if _, err := receipts.VerifySubset(med.Reference(), closed[2:4]); err != nil {
		t.Fatalf("a subset with no close: %v", err)
	}
	if _, err := receipts.VerifySubset(med.Reference(), [][]byte{list[0], closed[4]}); !isCode(err, receipts.CodeClose, 1) {
		t.Fatalf("a chain that does not end where the close says: %v, want close at index 1", err)
	}
	if _, err := receipts.Verify(med.Reference(), closed[2:]); !isCode(err, receipts.CodeChain, 0) {
		t.Fatalf("the subset as a whole list: %v, want chain at index 0", err)
	}
}

// questionPartiesTo is a member no delivery of closedLog was queued for: the answerer.
func questionPartiesTo(t testing.TB) string {
	_, _, answerer, _ := questionParties(t)
	return answerer
}

// TestReleasesPlacesClose holds receipt-6's reading of a list with a close (close-2): the close
// belongs to no question, RoleClose with no question; a closed closing is its question's closing,
// of the asked part; and one of another subject fails release.
func TestReleasesPlacesClose(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	list, ids := closedLog(t, med)
	v, err := receipts.Verify(med.Reference(), append(append([][]byte{}, list...), buildClose(t, closeOfLog(t, ids), med)))
	if err != nil {
		t.Fatal(err)
	}
	places, err := receipts.Releases(v)
	if err != nil {
		t.Fatal(err)
	}
	want := []receipts.Place{{Role: receipts.RoleGive, Question: -1}, {Role: receipts.RoleQuestion, Question: 1},
		{Role: receipts.RoleGive, Question: -1}, {Role: receipts.RoleClosing, Question: 1}, {Role: receipts.RoleClose, Question: -1}}
	if !reflect.DeepEqual(places, want) {
		t.Fatalf("places %+v, want %+v", places, want)
	}
	asker, _, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 2, Hash: mhash}
	other := receipts.Subject{Name: "proposal", Digest: digestOf(7)}
	closing := predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonClosed, env)
	if _, err := receipts.ReleasesOf([]receipts.Statement{
		{Subject: receipts.Subject{Name: "proposal", Digest: digestOf(1)}, Predicate: predicateOf(receipts.KindCrossing, asker, questionPartiesTo(t), "escalate", "maybe", env)},
		{Subject: other, Predicate: closing},
	}); !isCode(err, receipts.CodeRelease, 1) {
		t.Fatalf("a closed closing of another part: %v, want release at index 1", err)
	}
}

// TestClosedFitsOnlyTheClose holds receipt-4 and receipt-11 for the reason closed: a resolution
// block by the mediator with the event close, and nothing else.
func TestClosedFitsOnlyTheClose(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	asker, receiver, answerer, _ := questionParties(t)
	env := receipts.Envelope{Sender: asker, Seq: 1, Hash: mhash}
	asked := receipts.Subject{Name: "proposal", Digest: digestOf(1)}
	ask := entry{asked, predicateOf(receipts.KindCrossing, asker, receiver, "escalate", "maybe", env)}
	closeBy := receipts.By{Decider: receipts.ByMediator, Event: receipts.EventClose}
	for name, e := range map[string]entry{
		"closed on a crossing receipt": {asked, func() receipts.Predicate {
			p := predicateOf(receipts.KindCrossing, asker, receiver, "block", receipts.ReasonClosed, env)
			return p
		}()},
		"closed by the deadline": {asked, func() receipts.Predicate {
			p := predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonClosed, env)
			p.By = receipts.By{Decider: receipts.ByMediator, Event: receipts.EventDeadline}
			return p
		}()},
		"a timeout by the close": {asked, func() receipts.Predicate {
			p := predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonTimeout, env)
			p.By = closeBy
			return p
		}()},
		"a decline by the close": {asked, func() receipts.Predicate {
			p := predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonDeclined, env)
			p.By = closeBy
			return p
		}()},
	} {
		if _, err := receipts.Verify(med.Reference(), journal(t, med, []entry{ask, e})); !isCode(err, receipts.CodeStatementType, 1) {
			t.Fatalf("%s: %v, want statement-type at index 1", name, err)
		}
	}
	ok := entry{asked, predicateOf(receipts.KindResolution, answerer, asker, "block", receipts.ReasonClosed, env)}
	r := journal(t, med, []entry{ask, ok})
	if _, err := receipts.Verify(med.Reference(), r); err != nil {
		t.Fatalf("a closed closing: %v", err)
	}
	lines, err := receipts.Show(r[1])
	if err != nil || lines[3] != "verdict block, reason closed" || lines[4] != "by mediator close" {
		t.Fatalf("Show: %q, %v", lines, err)
	}
}

// TestShowAClose shows a close without verifying it (receipt-8, close-2): the line that says so,
// its id, Accord, card, mediator and time, then one line per chain and one per delivery not yet
// fetched.
func TestShowAClose(t *testing.T) {
	t.Parallel()
	med := ptesting.Key(t, "mediator")
	_, ids := closedLog(t, med)
	c := closeOfLog(t, ids)
	r, v, err := receipts.BuildClose(c, med)
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := v.Close()
	lines, err := receipts.Show(r)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{receipts.NotVerified, "close " + cl.ID, "accord " + accord + ", card " + card, "mediator " + med.Reference() + ", at " + closeAt}
	for _, f := range c.Chains {
		want = append(want, "chain from "+f.From+" to "+f.To+", seq "+strconv.FormatInt(f.Seq, 10)+", receipt "+f.Receipt)
	}
	want = append(want, "unfetched to "+c.Unfetched[0].To+", receipt "+c.Unfetched[0].Receipt)
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines\n%q\nwant\n%q", lines, want)
	}
}

// FuzzDecodeClose decodes any bytes as a close. It never panics, and a close it reads builds again
// to the same id under the key it names (fuzz-parsers, core-no-panic).
func FuzzDecodeClose(f *testing.F) {
	med := ptesting.Key(f, "mediator")
	_, ids := closedLog(f, med)
	f.Add(buildClose(f, closeOfLog(f, ids), med))
	f.Add(buildClose(f, receipts.Close{Accord: accord, Card: card, At: closeAt}, med))
	for _, b := range receiptSeeds(f) {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		c, err := receipts.DecodeClose(b)
		if err != nil {
			return
		}
		if c.ID == "" || !strings.HasPrefix(c.ID, "sha256:") {
			t.Fatalf("a decoded close with no id: %+v", c)
		}
		if _, err := receipts.Show(b); err != nil {
			t.Fatalf("a close DecodeClose reads that Show refuses: %v", err)
		}
	})
}

// FuzzBuildClose builds a close of any time and Accord. It never panics, and a close it builds
// verifies as the one entry of a list with no receipts only when its lists are empty (core-no-panic).
func FuzzBuildClose(f *testing.F) {
	med := ptesting.Key(f, "mediator")
	f.Add(accord, card, closeAt)
	f.Add("sha256:", "card", "2026-02-30T00:00:00Z")
	f.Fuzz(func(t *testing.T, a, c, at string) {
		r, v, err := receipts.BuildClose(receipts.Close{Accord: a, Card: c, At: at}, med)
		if err != nil {
			return
		}
		got, ok := v.Close()
		if !ok || got.Accord != a || got.At != at {
			t.Fatalf("BuildClose yields %+v", got)
		}
		if _, err := receipts.Verify(med.Reference(), [][]byte{r}); err != nil {
			t.Fatalf("a close BuildClose wrote does not verify: %v", err)
		}
	})
}
