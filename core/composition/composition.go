// Package composition is the decision module of this version: it decides a crossing from the two
// parties' terms and the roles their bindings give them, and returns a verdict and a reason. It
// refuses nothing: every crossing that reaches it is decided.
//
// It implements composition-1 to composition-3 and composition-5 to composition-7: matching terms
// (composition-1), no match (composition-2), a refusal wins (composition-3), maybe (composition-5),
// need (composition-6) and a judge's judgment where the terms would ask (composition-7), run in the
// order of section 5's table. Decide is the one entry point (one-decision-module): no other code
// compares a stance, matches a term or compares two terms' judges. It runs no judge: a judgment is an
// input, settled before Decide is called.
package composition

import (
	"sort"

	"github.com/orilloai/placket/core/setup"
)

// Verdict is a crossing's verdict (receipt-4).
type Verdict string

// The verdicts composition returns (receipt-4, record 0005).
const (
	Pass     Verdict = "pass"
	Block    Verdict = "block"
	Escalate Verdict = "escalate"
)

// The reasons composition returns (receipt-4). A judge's block has the reason ReasonJudge writes.
const (
	ReasonOK      = "ok"
	ReasonNoMatch = "no-match"
	ReasonMaybe   = "maybe"
)

// judgePrefix opens the reason of a judge's block (receipt-4, composition-7).
const judgePrefix = "judge:"

// ReasonJudge is the reason of a block the judge named name decided: judge: and its card name
// (receipt-4, composition-7).
func ReasonJudge(name string) string { return judgePrefix + name }

// A Judgment is a judge's verdict on a crossing, Pass or Block, by the judge's card name, as the
// runner settled it before Decide is called (composition-7). A nil *Judgment is none.
type Judgment struct {
	Name    string
	Verdict Verdict
}

// A Crossing is what composition decides: a part of a kind from a sender to a receiver, given by a
// member or formed by the mediator from an answer, and the judgment a judge gave it, or nil
// (composition-1, composition-7, maybe-8, record 0005).
type Crossing struct {
	From, To string
	Kind     string
	Formed   bool
	Judge    *Judgment
}

// A Decision is composition's verdict and reason; for an escalate, and for a judged pass or block,
// the rule every matching maybe term agreed on, with its roles in sorted order and its Judges the
// judges both sides consented to, or none (composition-5, composition-7, record 0005); and the name
// of the judge that decided, empty when the terms decided.
type Decision struct {
	Verdict Verdict
	Reason  string
	Rule    setup.Rule
	Judge   string
}

// Decide decides c under the Accord a by the rows of section 5, in order; the first that applies
// decides (composition-1 to composition-6, record 0005). It is the decision module's one entry point
// (one-decision-module).
func Decide(a *setup.Accord, c Crossing) Decision {
	var matching []setup.Term
	sent, got := 0, 0
	for _, t := range a.Terms(c.From) {
		if t.Dir == setup.DirGive && matches(t, c.Kind, a.Roles(c.To)) {
			matching = append(matching, t)
			sent++
		}
	}
	for _, t := range a.Terms(c.To) {
		if t.Dir == setup.DirGet && matches(t, c.Kind, a.Roles(c.From)) {
			matching = append(matching, t)
			got++
		}
	}
	block := func(reason string) Decision { return Decision{Verdict: Block, Reason: reason} }

	// Row 1: a refusal wins (composition-3).
	for _, t := range matching {
		if t.Stance == setup.StanceNo {
			return block(ReasonNoMatch)
		}
	}
	// Row 2: both sides (composition-2).
	if sent == 0 || got == 0 {
		return block(ReasonNoMatch)
	}
	var rules []setup.Rule
	allMaybe := true
	for _, t := range matching {
		if t.Stance == setup.StanceMaybe && t.Resolve != nil {
			rules = append(rules, *t.Resolve)
		} else {
			allMaybe = false
		}
	}
	if len(rules) == 0 {
		// Row 7 (composition-6: need composes as want).
		return Decision{Verdict: Pass, Reason: ReasonOK}
	}
	// Row 3: a part the mediator formed meets a maybe term.
	if c.Formed {
		return block(ReasonMaybe)
	}
	// Row 4: the maybe rules must agree, compared without their judges.
	agreed := normal(rules[0])
	for _, r := range rules[1:] {
		if !same(agreed, normal(r)) {
			return block(ReasonMaybe)
		}
	}
	// Consent: the judges hold only when every matching term, on both sides, is a maybe and all of
	// them list the same judges, in the same order with the same uses (composition-7).
	agreed.Judges = nil
	if allMaybe && len(rules[0].Judges) > 0 {
		agreed.Judges = append([]setup.Judge(nil), rules[0].Judges...)
		for _, r := range rules[1:] {
			if !sameJudges(agreed.Judges, r.Judges) {
				agreed.Judges = nil
				break
			}
		}
	}
	// Row 5: a judge the terms consented to, with use decide, decides (composition-7). Any other
	// judgment is none.
	if j := c.Judge; j != nil && (j.Verdict == Pass || j.Verdict == Block) {
		for _, g := range agreed.Judges {
			if g.Name != j.Name || g.Use != setup.Decide {
				continue
			}
			if j.Verdict == Pass {
				return Decision{Verdict: Pass, Reason: ReasonOK, Rule: agreed, Judge: j.Name}
			}
			return Decision{Verdict: Block, Reason: ReasonJudge(j.Name), Rule: agreed, Judge: j.Name}
		}
	}
	// Row 6: ask (maybe-1).
	return Decision{Verdict: Escalate, Reason: ReasonMaybe, Rule: agreed}
}

// sameJudges reports whether two lists of judges hold the same names in the same order with the
// same uses.
func sameJudges(a, b []setup.Judge) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// matches reports whether t selects kind and names a role the other party holds (composition-1).
func matches(t setup.Term, kind string, otherRoles []string) bool {
	if !contains(t.Kinds, kind) {
		return false
	}
	for _, w := range t.With {
		if contains(otherRoles, w) {
			return true
		}
	}
	return false
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// normal returns a rule with its roles sorted, so that rules naming one set of roles compare equal.
func normal(r setup.Rule) setup.Rule {
	to := append([]string(nil), r.To...)
	sort.Strings(to)
	return setup.Rule{To: to, Timeout: r.Timeout}
}

func same(a, b setup.Rule) bool {
	if a.Timeout != b.Timeout || len(a.To) != len(b.To) {
		return false
	}
	for i := range a.To {
		if a.To[i] != b.To[i] {
			return false
		}
	}
	return true
}
