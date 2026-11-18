package mediator_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// judgesOf builds a term's resolve.judges: name and use, in order.
func judgesOf(pairs ...string) []any {
	var out []any
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"name": pairs[i], "use": pairs[i+1]})
	}
	return out
}

// maybeOn is a maybe term of dir for kind, with the other side's role and the ask-and-log rule, and
// judges when given.
func maybeOn(dir, with, kind string, js []any) map[string]any {
	resolve := map[string]any{"to": []any{"role:decider"}, "timeout": json.Number("3600")}
	if js != nil {
		resolve["judges"] = js
	}
	return map[string]any{"stance": "maybe", "dir": dir, "with": []any{"role:" + with},
		"select": map[string]any{"kind": []any{kind}}, "resolve": resolve}
}

// judgedSet is the ask-and-log set with a card of the judges a, b and c, and the proposer's and the
// applier's terms extended by those given, each declaration signed again. The bindings sign the
// policy, not the card or the terms, so they stand.
func judgedSet(t testing.TB, sender, receiver []map[string]any) founding.Inputs {
	t.Helper()
	sealed, at := ptesting.Founding(t, "ask-and-log")
	raw := ptesting.Clear(t, sealed)
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var set map[string]any
	if err := d.Decode(&set); err != nil {
		t.Fatal(err)
	}
	h := "sha256:" + strings.Repeat("ab", 32)
	var card []any
	for _, n := range []string{"a", "b", "c"} {
		card = append(card, map[string]any{"name": n, "version": "0.1.0", "hash": h})
	}
	set["card"].(map[string]any)["judges"] = card
	for _, x := range set["declarations"].([]any) {
		decl := x.(map[string]any)
		var name string
		var extra []map[string]any
		switch decl["actor"] {
		case ptesting.Key(t, "proposer").Reference():
			name, extra = "proposer", sender
		case ptesting.Key(t, "applier").Reference():
			name, extra = "applier", receiver
		default:
			continue
		}
		terms := decl["terms"].([]any)
		for _, e := range extra {
			terms = append(terms, e)
		}
		decl["terms"] = terms
		delete(decl, "sig")
		sig, err := signing.Sign(canonicalOf(t, decl), signing.TypeDeclaration, ptesting.Key(t, name))
		if err != nil {
			t.Fatal(err)
		}
		decl["sig"] = sig
	}
	return ptesting.SealedByAll(t, canonicalOf(t, set), at)
}

func canonicalOf(t testing.TB, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// judgedFound founds judgedSet's set.
func judgedFound(t testing.TB, sender, receiver []map[string]any) *mediator.Mediator {
	t.Helper()
	_, at := ptesting.Founding(t, "ask-and-log")
	m, err := mediator.Found(judgedSet(t, sender, receiver), ptesting.Key(t, "mediator"), at)
	if err != nil {
		t.Fatalf("the judged set does not found: %v", err)
	}
	return m
}

// reportGive is the proposer's give of a text report to the applier at seq: the caller, the message
// and the envelope.
func reportGive(t testing.TB, m *mediator.Mediator, seq int64, kind string) (string, []byte, []byte) {
	t.Helper()
	p := ptesting.Key(t, "proposer")
	msg := message(t, m.Accord(), fmt.Sprintf("report-%d", seq), ptesting.Key(t, "applier").Reference(), kind, "A report.", nil)
	return p.Reference(), msg, envelope(t, seq, msg, p)
}

// TestJudgesOnlyWhereBothSidesAgree reads the judges a give would reach: only a give composition
// would ask, whose matching terms on both sides are all maybe and list the same judges, reaches
// them, in the rule's order (composition-5, composition-7). Every other give reaches none.
func TestJudgesOnlyWhereBothSidesAgree(t *testing.T) {
	t.Parallel()
	ab := judgesOf("a", "decide", "b", "decide")
	want := map[string]any{"stance": "want", "dir": "give", "with": []any{"role:applier"}, "select": map[string]any{"kind": []any{"report"}}}
	for _, c := range []struct {
		name             string
		sender, receiver []map[string]any
		kind             string
		judges           []setup.Judge
	}{
		{"both sides list a and b", []map[string]any{maybeOn("give", "applier", "report", ab)},
			[]map[string]any{maybeOn("get", "proposer", "report", ab)}, "report",
			[]setup.Judge{{Name: "a", Use: setup.Decide}, {Name: "b", Use: setup.Decide}}},
		{"a kind no maybe term selects", []map[string]any{maybeOn("give", "applier", "report", ab)},
			[]map[string]any{maybeOn("get", "proposer", "report", ab)}, "note", nil},
		{"only the receiver's term is maybe", nil,
			[]map[string]any{maybeOn("get", "proposer", "report", judgesOf("a", "decide"))}, "report", nil},
		{"the two sides list other judges", []map[string]any{maybeOn("give", "applier", "report", judgesOf("a", "decide"))},
			[]map[string]any{maybeOn("get", "proposer", "report", judgesOf("b", "decide"))}, "report", nil},
		{"only the sender lists a", []map[string]any{maybeOn("give", "applier", "report", judgesOf("a", "decide"))},
			[]map[string]any{maybeOn("get", "proposer", "report", nil)}, "report", nil},
		{"neither side lists a judge", []map[string]any{maybeOn("give", "applier", "report", nil)},
			[]map[string]any{maybeOn("get", "proposer", "report", nil)}, "report", nil},
		{"a want beside the sender's maybe", []map[string]any{want, maybeOn("give", "applier", "report", judgesOf("a", "decide"))},
			[]map[string]any{maybeOn("get", "proposer", "report", judgesOf("a", "decide"))}, "report", nil},
	} {
		receiver := c.receiver
		if c.sender == nil {
			// The sender's term for report is the ask-and-log proposer's want to give to the applier.
			c.sender = []map[string]any{want}
		}
		m := judgedFound(t, c.sender, receiver)
		caller, msg, env := reportGive(t, m, 1, c.kind)
		got, ok := m.Judges(caller, msg, env)
		if ok != (c.judges != nil) || !reflect.DeepEqual(got.Judges, c.judges) {
			t.Fatalf("%s: Judges = %v, %v; want %v", c.name, got.Judges, ok, c.judges)
		}
		if !ok {
			continue
		}
		if got.Kind != "report" || got.Form != "text" || string(got.Body) != "A report." {
			t.Fatalf("%s: read kind %q, form %q", c.name, got.Kind, got.Form)
		}
	}
}

// TestJudgesReadsAndWritesNothing reads the judge-block vector's give: Judges returns its kind, form,
// body and the rule's judges and changes nothing, and Give with jev's block writes the receipt the
// vector expects (composition-7, inputs-as-parameters). What Judges returns is a copy
// (results-are-copies).
func TestJudgesReadsAndWritesNothing(t *testing.T) {
	t.Parallel()
	v := scenarioOf(t, "judge-block")
	before, pending := v.m.Receipts(), v.m.Pending()
	got, ok := v.m.Judges(v.caller, v.message, v.envelope)
	want := mediator.Judging{Kind: "report", Form: "text", Body: v.body, Judges: []setup.Judge{{Name: "jev", Use: setup.Decide}}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Judges = %v, %v", got.Judges, ok)
	}
	if !reflect.DeepEqual(v.m.Receipts(), before) || !reflect.DeepEqual(v.m.Pending(), pending) {
		t.Fatal("Judges changed the mediator")
	}
	got.Body[0] = 'X'
	got.Judges[0].Name = "lint"
	again, ok := v.m.Judges(v.caller, v.message, v.envelope)
	if !ok || !reflect.DeepEqual(again, want) {
		t.Fatal("Judges shares its result with the mediator")
	}
	out, err := v.m.Give(v.caller, v.message, v.envelope, &composition.Judgment{Name: "jev", Verdict: composition.Block}, v.at)
	if err != nil {
		t.Fatal(err)
	}
	if out.Verdict != "block" || out.Reason != "judge:jev" || !bytes.Equal(out.Receipt, v.receipt) {
		t.Fatalf("Give wrote %s %s, not the vector's receipt", out.Verdict, out.Reason)
	}
}

// TestJudgesOfVectorsThatAsk reads the gives of the vectors whose terms do not agree on a judge: they
// reach none (composition-5, composition-7).
func TestJudgesOfVectorsThatAsk(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"judge-one-side", "judges-differ"} {
		v := scenarioOf(t, name)
		if got, ok := v.m.Judges(v.caller, v.message, v.envelope); ok || got.Judges != nil {
			t.Fatalf("%s: Judges = %v, %v; want none", name, got.Judges, ok)
		}
	}
}

// TestJudgesRunsGivesRejectionRows reads gives Give would reject: none reaches a judge, and Give
// returns the rejection. A closed Accord is closed, right after membership (close-3), and a stranger
// is not-a-member (setup-contract-6).
func TestJudgesRunsGivesRejectionRows(t *testing.T) {
	t.Parallel()
	ab := judgesOf("a", "decide")
	sender := []map[string]any{maybeOn("give", "applier", "report", ab)}
	receiver := []map[string]any{maybeOn("get", "proposer", "report", ab)}
	set := judgedSet(t, sender, receiver)
	_, foundedAt := ptesting.Founding(t, "ask-and-log")
	med := ptesting.Key(t, "mediator")
	closed, _, _, err := mediator.Close(set, nil, nil, nil, foundedAt, med, foundedAt)
	if err != nil {
		t.Fatal(err)
	}
	caller, msg, env := reportGive(t, closed, 1, "report")
	if got, ok := closed.Judges(caller, msg, env); ok || got.Judges != nil {
		t.Fatalf("a closed Accord: Judges = %v, %v", got.Judges, ok)
	}
	if _, err := closed.Give(caller, msg, env, nil, foundedAt); !rejected(err, mediator.AccordClosed) {
		t.Fatalf("a closed Accord's give: %v, want closed", err)
	}

	m := judgedFound(t, sender, receiver)
	stranger := ptesting.Key(t, "stranger")
	for name, g := range map[string][3][]byte{
		"a stranger":          {[]byte(stranger.Reference()), msg, envelope(t, 1, msg, stranger)},
		"a bad envelope":      {[]byte(caller), msg, []byte("not an envelope")},
		"bytes not a message": {[]byte(caller), []byte("{"), env},
	} {
		if got, ok := m.Judges(string(g[0]), g[1], g[2]); ok || got.Judges != nil {
			t.Fatalf("%s: Judges = %v, %v", name, got.Judges, ok)
		}
	}
	if got, ok := (&mediator.Mediator{}).Judges(caller, msg, env); ok || got.Judges != nil {
		t.Fatal("an unfounded mediator reaches a judge")
	}
	// A seq already taken: the give is refused, and so is reaching a judge for it.
	if _, err := m.Give(caller, msg, env, nil, foundedAt); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Judges(caller, msg, env); ok {
		t.Fatal("a give whose seq was taken reaches a judge")
	}
}

// TestJudgingNeverPrints prints a Judging every way: its body never shows (secrets-never-print).
func TestJudgingNeverPrints(t *testing.T) {
	t.Parallel()
	j := mediator.Judging{Kind: "report", Form: "text", Body: []byte("planted body text"), Judges: []setup.Judge{{Name: "a", Use: setup.Decide}}}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
		got := fmt.Sprintf(verb, j) + fmt.Sprintf(verb, &j)
		if strings.Contains(got, "planted") || strings.Contains(got, hex.EncodeToString([]byte("planted"))) {
			t.Fatalf("%s prints the body: %s", verb, got)
		}
	}
	if got := fmt.Sprint(j); got != "mediator.Judging" {
		t.Fatalf("Judging prints %q", got)
	}
}

// FuzzJudges reads any give: it never panics, and writes nothing (core-no-panic).
func FuzzJudges(f *testing.F) {
	m := judgedFound(f, []map[string]any{maybeOn("give", "applier", "report", judgesOf("a", "decide"))},
		[]map[string]any{maybeOn("get", "proposer", "report", judgesOf("a", "decide"))})
	caller, msg, env := reportGive(f, m, 1, "report")
	f.Add(caller, msg, env)
	f.Add("", []byte("{}"), []byte("{}"))
	f.Fuzz(func(t *testing.T, caller string, message, envelope []byte) {
		before := m.Receipts()
		j, ok := m.Judges(caller, message, envelope)
		if ok && len(j.Judges) == 0 {
			t.Fatal("a give reaches an empty list of judges")
		}
		if !reflect.DeepEqual(m.Receipts(), before) {
			t.Fatal("Judges wrote a receipt")
		}
	})
}

// A scenario is a crossing vector's mediator, founded, and its first step, a give.
type scenario struct {
	m                       *mediator.Mediator
	caller, at              string
	message, envelope, body []byte
	receipt                 []byte
}

// scenarioOf reads vectors/crossing/<name>.json, founds its set as its mediator, and reads its first
// step, a give of a text part.
func scenarioOf(t testing.TB, name string) scenario {
	t.Helper()
	rel := filepath.Join("vectors", "crossing", name+".json")
	var b []byte
	var err error
	dir := "."
	for range 5 {
		if b, err = os.ReadFile(filepath.Join(dir, rel)); err == nil {
			break
		}
		dir = filepath.Join(dir, "..")
	}
	if err != nil {
		t.Fatalf("no %s: %v", rel, err)
	}
	var v struct {
		Founding json.RawMessage `json:"founding"`
		Openings []struct {
			Actor   string          `json:"actor"`
			Opening json.RawMessage `json:"opening"`
		} `json:"openings"`
		Acceptances json.RawMessage   `json:"acceptances"`
		FoundedAt   string            `json:"foundedAt"`
		Keys        map[string]string `json:"keys"`
		Mediator    string            `json:"mediator"`
		Steps       []struct {
			As, At, Op  string
			MessageHex  string `json:"messageHex"`
			EnvelopeHex string `json:"envelopeHex"`
			Expect      struct {
				Receipt json.RawMessage `json:"receipt"`
			} `json:"expect"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(b, &v); err != nil || len(v.Steps) == 0 || v.Steps[0].Op != "give" {
		t.Fatalf("%s: %v", rel, err)
	}
	keyOf := func(n string) signing.Key {
		seed, err := hex.DecodeString(v.Keys[n])
		if err != nil {
			t.Fatal(err)
		}
		k, err := signing.NewKey(seed)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	set, err := canonical.Form(v.Founding)
	if err != nil {
		t.Fatal(err)
	}
	in := founding.Inputs{Set: set, Openings: setup.Openings{ByActor: map[string][]byte{}}, Acceptances: v.Acceptances}
	for _, o := range v.Openings {
		in.Openings.ByActor[o.Actor] = o.Opening
	}
	m, err := mediator.Found(in, keyOf(v.Mediator), v.FoundedAt)
	if err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	s := v.Steps[0]
	sc := scenario{m: m, caller: keyOf(s.As).Reference(), at: s.At}
	if sc.message, err = hex.DecodeString(s.MessageHex); err != nil {
		t.Fatal(err)
	}
	if sc.envelope, err = hex.DecodeString(s.EnvelopeHex); err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(sc.message, &msg); err != nil || len(msg.Parts) != 1 {
		t.Fatalf("%s: the give's message: %v", rel, err)
	}
	sc.body = []byte(msg.Parts[0].Text)
	if len(s.Expect.Receipt) > 0 {
		if sc.receipt, err = canonical.Form(s.Expect.Receipt); err != nil {
			t.Fatal(err)
		}
	}
	return sc
}
