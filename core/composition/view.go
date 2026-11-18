package composition

import (
	"sort"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/setup"
)

// This file is outside the kernel hash (kernel.go reads composition.go alone): it decides nothing.
// It lists the crossings a member's own terms select and decides each with Decide, so a view's
// outcomes are the module's verdicts (setup-contract-14, docs/adr/0025-sealed-terms.md).

// A Side is one crossing a member is a side of, as its own terms select it: the counterpart, the
// direction from the member, the kind, and Decide's decision of that crossing, given by a member with
// no judgment (setup-contract-14).
type Side struct {
	Counterpart, Dir, Kind string
	Decision               Decision
}

// An Asked is a crossing between two other members that Decide asks about, whose agreed rule's to
// names a role of the member: the asker, the receiver, the kind and the rule's timeout
// (setup-contract-14).
type Asked struct {
	Asker, Receiver, Kind string
	Timeout               int64
}

// Sides returns the crossings member is a side of, by its own terms only: for each of its terms,
// each kind the term selects and each member whose binding gives a role one of the term's with
// selectors names, one Side, decided by Decide (composition-1, setup-contract-14). A kind only the
// other side names never appears. They are sorted by counterpart in actor-reference order, then by
// direction and kind in byte order, each crossing once. A member whose terms were not opened has
// none.
func Sides(a *setup.Accord, member string) []Side {
	seen := map[[3]string]bool{}
	var out []Side
	for _, t := range a.Terms(member) {
		for _, c := range a.Members() {
			// The term names a role c holds: matches with a kind the term selects.
			if !matches(t, firstKind(t), a.Roles(c)) {
				continue
			}
			for _, k := range t.Kinds {
				key := [3]string{c, t.Dir, k}
				if seen[key] {
					continue
				}
				seen[key] = true
				x := Crossing{From: member, To: c, Kind: k}
				if t.Dir == setup.DirGet {
					x = Crossing{From: c, To: member, Kind: k}
				}
				out = append(out, Side{Counterpart: c, Dir: t.Dir, Kind: k, Decision: Decide(a, x)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.Counterpart != y.Counterpart {
			return canonical.Less(x.Counterpart, y.Counterpart)
		}
		if x.Dir != y.Dir {
			return x.Dir < y.Dir
		}
		return x.Kind < y.Kind
	})
	return out
}

// firstKind is a kind the term selects, so that matches reads its with selectors against roles.
func firstKind(t setup.Term) string {
	if len(t.Kinds) == 0 {
		return ""
	}
	return t.Kinds[0]
}

// Answers returns every crossing between two members other than member that Decide asks about and
// whose agreed rule's to names a role member's binding gives it, with its asker, receiver, kind and
// timeout (setup-contract-14). The candidates are each kind of each give term of each other member,
// toward each other member; Decide reads both sides. They are sorted by asker and receiver in
// actor-reference order, then by kind in byte order, each once.
func Answers(a *setup.Accord, member string) []Asked {
	roles := a.Roles(member)
	seen := map[[3]string]bool{}
	var out []Asked
	for _, s := range a.Members() {
		if s == member {
			continue
		}
		for _, t := range a.Terms(s) {
			if t.Dir != setup.DirGive {
				continue
			}
			for _, r := range a.Members() {
				if r == member {
					continue
				}
				for _, k := range t.Kinds {
					key := [3]string{s, r, k}
					if seen[key] {
						continue
					}
					seen[key] = true
					d := Decide(a, Crossing{From: s, To: r, Kind: k})
					if d.Verdict != Escalate || !namesAny(d.Rule.To, roles) {
						continue
					}
					out = append(out, Asked{Asker: s, Receiver: r, Kind: k, Timeout: d.Rule.Timeout})
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.Asker != y.Asker {
			return canonical.Less(x.Asker, y.Asker)
		}
		if x.Receiver != y.Receiver {
			return canonical.Less(x.Receiver, y.Receiver)
		}
		return x.Kind < y.Kind
	})
	return out
}

// namesAny reports whether a rule's to, role labels, names one of roles.
func namesAny(to, roles []string) bool {
	for _, r := range to {
		if contains(roles, r) {
			return true
		}
	}
	return false
}
