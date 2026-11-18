package composition_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// A judges builds a term's resolve.judges: name and use, in order.
func judges(pairs ...string) []any {
	var out []any
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"name": pairs[i], "use": pairs[i+1]})
	}
	return out
}

// maybeTerm is a maybe term of dir for kind, with the other side's role and the ask-and-log rule,
// and judges when given.
func maybeTerm(dir, with, kind string, js []any) map[string]any {
	resolve := map[string]any{"to": []any{"role:decider"}, "timeout": json.Number("3600")}
	if js != nil {
		resolve["judges"] = js
	}
	return map[string]any{"stance": "maybe", "dir": dir, "with": []any{"role:" + with},
		"select": map[string]any{"kind": []any{kind}}, "resolve": resolve}
}

// judged founds the ask-and-log set with a card of two judges, jev and lint, and the proposer's and
// the applier's terms for kind replaced by those given, each declaration signed again. The bindings
// sign the policy, not the card or the terms, so they stand.
func judged(t *testing.T, sender, receiver []map[string]any) *setup.Accord {
	t.Helper()
	raw, at := ptesting.Founding(t, "ask-and-log")
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var set map[string]any
	if err := d.Decode(&set); err != nil {
		t.Fatal(err)
	}
	h := "sha256:" + strings.Repeat("ab", 32)
	set["card"].(map[string]any)["judges"] = []any{
		map[string]any{"name": "jev", "version": "1.0.0", "hash": h},
		map[string]any{"name": "lint", "version": "1.0.0", "hash": h},
	}
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
		sig, err := signing.Sign(encodeJSON(t, decl), signing.TypeDeclaration, ptesting.Key(t, name))
		if err != nil {
			t.Fatal(err)
		}
		decl["sig"] = sig
	}
	a, err := setup.Found(encodeJSON(t, set), at)
	if err != nil {
		t.Fatalf("the judged set does not found: %v", err)
	}
	return a
}

func encodeJSON(t *testing.T, v any) []byte {
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

func crossing(t *testing.T, kind string, j *composition.Judgment) composition.Crossing {
	return composition.Crossing{From: ptesting.Key(t, "proposer").Reference(), To: ptesting.Key(t, "applier").Reference(), Kind: kind, Judge: j}
}

func jev(v composition.Verdict) *composition.Judgment {
	return &composition.Judgment{Name: "jev", Verdict: v}
}

// TestJudgeDecides gives a judge both sides consented to, with use decide, the crossing the terms
// would ask about: its pass is pass ok and its block block judge:jev, each naming the judge, with
// the consented judges on the rule; with no judgment the part is asked, the judges still on the rule
// (composition-7, composition-5).
func TestJudgeDecides(t *testing.T) {
	t.Parallel()
	a := judged(t, []map[string]any{maybeTerm("give", "applier", "report", judges("jev", "decide"))},
		[]map[string]any{maybeTerm("get", "proposer", "report", judges("jev", "decide"))})
	want := []setup.Judge{{Name: "jev", Use: setup.Decide}}
	for _, c := range []struct {
		judge   *composition.Judgment
		verdict composition.Verdict
		reason  string
		by      string
	}{
		{jev(composition.Pass), composition.Pass, "ok", "jev"},
		{jev(composition.Block), composition.Block, "judge:jev", "jev"},
		{nil, composition.Escalate, "maybe", ""},
		{&composition.Judgment{Name: "jev", Verdict: composition.Escalate}, composition.Escalate, "maybe", ""},
		{&composition.Judgment{Name: "lint", Verdict: composition.Pass}, composition.Escalate, "maybe", ""},
	} {
		got := composition.Decide(a, crossing(t, "report", c.judge))
		if got.Verdict != c.verdict || got.Reason != c.reason || got.Judge != c.by {
			t.Fatalf("judgment %+v: got %+v, want %s %s by %q", c.judge, got, c.verdict, c.reason, c.by)
		}
		if len(got.Rule.Judges) != 1 || got.Rule.Judges[0] != want[0] || got.Rule.Timeout != 3600 {
			t.Fatalf("judgment %+v: rule %+v, want the judges %v", c.judge, got.Rule, want)
		}
	}
	if got := composition.ReasonJudge("jev"); got != "judge:jev" {
		t.Fatalf("ReasonJudge = %q", got)
	}
}

// TestJudgeOneSideIsAsked asks about a part when only one side's maybe term lists judges, or when
// the two sides' lists differ only in order, or in use: no judge is consulted and the rule holds no
// judges (composition-5, composition-7).
func TestJudgeOneSideIsAsked(t *testing.T) {
	t.Parallel()
	for name, terms := range map[string][2][]any{
		"only the sender lists":   {judges("jev", "decide"), nil},
		"only the receiver lists": {nil, judges("jev", "decide")},
		"in another order":        {judges("jev", "decide", "lint", "decide"), judges("lint", "decide", "jev", "decide")},
		"with another use":        {judges("jev", "decide"), judges("jev", "advise")},
	} {
		a := judged(t, []map[string]any{maybeTerm("give", "applier", "report", terms[0])},
			[]map[string]any{maybeTerm("get", "proposer", "report", terms[1])})
		got := composition.Decide(a, crossing(t, "report", jev(composition.Pass)))
		if got.Verdict != composition.Escalate || got.Reason != "maybe" || got.Judge != "" || len(got.Rule.Judges) != 0 {
			t.Fatalf("%s: got %+v, want escalate maybe with no judges", name, got)
		}
	}
}

// TestJudgeWantBesideMaybeIsAsked asks about a part whose sender has a want term beside its maybe
// term: not every matching term is a maybe, so no judge is consulted (composition-7).
func TestJudgeWantBesideMaybeIsAsked(t *testing.T) {
	t.Parallel()
	want := map[string]any{"stance": "want", "dir": "give", "with": []any{"role:applier"}, "select": map[string]any{"kind": []any{"report"}}}
	a := judged(t, []map[string]any{want, maybeTerm("give", "applier", "report", judges("jev", "decide"))},
		[]map[string]any{maybeTerm("get", "proposer", "report", judges("jev", "decide"))})
	got := composition.Decide(a, crossing(t, "report", jev(composition.Pass)))
	if got.Verdict != composition.Escalate || got.Reason != "maybe" || got.Judge != "" || len(got.Rule.Judges) != 0 {
		t.Fatalf("got %+v, want escalate maybe with no judges", got)
	}
}

// TestAdviseNeverDecides asks about a part whose judge both sides list with use advise, whatever its
// judgment (composition-7).
func TestAdviseNeverDecides(t *testing.T) {
	t.Parallel()
	a := judged(t, []map[string]any{maybeTerm("give", "applier", "report", judges("jev", "advise"))},
		[]map[string]any{maybeTerm("get", "proposer", "report", judges("jev", "advise"))})
	for _, v := range []composition.Verdict{composition.Pass, composition.Block} {
		got := composition.Decide(a, crossing(t, "report", jev(v)))
		if got.Verdict != composition.Escalate || got.Judge != "" || len(got.Rule.Judges) != 1 || got.Rule.Judges[0].Use != setup.Advise {
			t.Fatalf("%s: got %+v, want escalate with the advise judge on the rule", v, got)
		}
	}
}

// TestFormedPartNeverJudged blocks a part the mediator formed that meets a judged maybe term, as any
// formed part, whatever the judgment (composition-7, maybe-3).
func TestFormedPartNeverJudged(t *testing.T) {
	t.Parallel()
	a := judged(t, []map[string]any{maybeTerm("give", "applier", "report", judges("jev", "decide"))},
		[]map[string]any{maybeTerm("get", "proposer", "report", judges("jev", "decide"))})
	c := crossing(t, "report", jev(composition.Pass))
	c.Formed = true
	got := composition.Decide(a, c)
	if got.Verdict != composition.Block || got.Reason != "maybe" || got.Judge != "" {
		t.Fatalf("got %+v, want block maybe", got)
	}
}
