// Package composition is the decision module of this version: it decides a crossing from the two
// parties' terms and the roles their bindings give them, and returns a verdict and a reason. It
// refuses nothing: every crossing that reaches it is decided.
//
// It implements composition-1 to composition-6: matching terms (composition-1), no match
// (composition-2), a refusal wins (composition-3), checks (composition-4), maybe (composition-5) and
// need (composition-6), run in the order of section 5's table. Decide is the one entry point
// (one-decision-module): no other code compares a stance or matches a term.
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

// The reasons composition returns (receipt-4). A failing check's reason, check:<name>, cannot arise
// in this version: no founded Accord has a term that names a check (composition-4).
const (
	ReasonOK      = "ok"
	ReasonNoMatch = "no-match"
	ReasonMaybe   = "maybe"
)

// A Crossing is what composition decides: a part of a kind from a sender to a receiver, given by a
// member or formed by the mediator from an answer (composition-1, maybe-8, record 0005).
type Crossing struct {
	From, To string
	Kind     string
	Formed   bool
}

// A Decision is composition's verdict and reason, and for an escalate the rule every matching maybe
// term agreed on, with its roles in sorted order (composition-5, record 0005).
type Decision struct {
	Verdict Verdict
	Reason  string
	Rule    setup.Rule
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
	// Row 3, the checks (composition-4), never applies in this version: the card loads no check and
	// founding refuses a term that names one, so no matching term has a check to run.
	var rules []setup.Rule
	for _, t := range matching {
		if t.Stance == setup.StanceMaybe && t.Resolve != nil {
			rules = append(rules, *t.Resolve)
		}
	}
	if len(rules) == 0 {
		// Row 7 (composition-6: need composes as want).
		return Decision{Verdict: Pass, Reason: ReasonOK}
	}
	// Row 4: a part the mediator formed meets a maybe term.
	if c.Formed {
		return block(ReasonMaybe)
	}
	// Row 5: the maybe rules must agree.
	agreed := normal(rules[0])
	for _, r := range rules[1:] {
		if !same(agreed, normal(r)) {
			return block(ReasonMaybe)
		}
	}
	// Row 6: ask (maybe-1).
	return Decision{Verdict: Escalate, Reason: ReasonMaybe, Rule: agreed}
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
