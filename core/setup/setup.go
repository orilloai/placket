// Package setup founds an Accord from a founding set, or refuses the set with the first code of
// founding's check table, and holds what the founded Accord says: its id and card hash, its members,
// the roles their bindings give them, and their terms.
//
// It implements setup-contract-1 to setup-contract-10: declarations and terms (setup-contract-1),
// bindings and roles (setup-contract-2, setup-contract-3), the card (setup-contract-4), the Accord id
// (setup-contract-5), membership (setup-contract-6), the mediator and its operator
// (setup-contract-7), the refusal order (setup-contract-8), the shape of a set (setup-contract-9) and
// the order of its lists (setup-contract-10).
package setup

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// Code is a founding refusal code, spelled as the check table of section 3 spells it.
type Code string

// The founding codes, in the order of the check table (section 3).
const (
	NotCanonical         Code = "not-canonical"
	InvalidShape         Code = "invalid-shape"
	BindingDuplicate     Code = "binding-duplicate"
	ActorReference       Code = "actor-reference"
	DeclarationSignature Code = "declaration-signature"
	TermInvalid          Code = "term-invalid"
	MaybeWithoutResolve  Code = "maybe-without-resolve"
	CheckUnknown         Code = "check-unknown"
	BindingMissing       Code = "binding-missing"
	BindingPrincipal     Code = "binding-principal"
	BindingSignature     Code = "binding-signature"
	BindingMediator      Code = "binding-mediator"
	BindingExpired       Code = "binding-expired"
	DeclarationMissing   Code = "declaration-missing"
	RoleMissing          Code = "role-missing"
)

// CodeAt is not a founding code: the founding time its caller gave does not keep the timestamp rule.
const CodeAt Code = "at"

// Error is a refused founding: the code of the first row that failed. It carries nothing of the set.
type Error struct{ Code Code }

func (e *Error) Error() string { return "setup: " + string(e.Code) }

// Spec is the card's spec: the version of this protocol (setup-contract-4).
const Spec = "0.1"

// The stances and directions of a term (setup-contract-1).
const (
	StanceNeed  = "need"
	StanceWant  = "want"
	StanceMaybe = "maybe"
	StanceNo    = "no"
	DirGive     = "give"
	DirGet      = "get"
)

// A Term is one term of a declaration, as founding read it (setup-contract-1).
type Term struct {
	Stance string
	Dir    string
	// With holds the role labels of the term's selectors, role: removed.
	With []string
	// Kinds are the kinds its select names.
	Kinds []string
	// Checks are the checks it names. A founded Accord has none in this version.
	Checks []Check
	// Resolve is the rule of a maybe term, and nil on any other.
	Resolve *Rule
}

// A Check is a check a term names (setup-contract-1).
type Check struct{ Name, Corpus string }

// A Rule is a maybe term's resolve: the role labels of who answers, and the timeout in seconds.
type Rule struct {
	To      []string
	Timeout int64
}

// An Accord is a founded founding set.
type Accord struct {
	id, card, mediator string
	members            []string
	roles              map[string][]string
	terms              map[string][]Term
}

// ID returns the Accord id: the typed hash of the set's canonical bytes (setup-contract-5).
func (a *Accord) ID() string { return a.id }

// Card returns the card hash: the typed hash of the card's canonical bytes (setup-contract-4).
func (a *Accord) Card() string { return a.card }

// Mediator returns the reference of the mediator the set names, its mediator.actor
// (setup-contract-7).
func (a *Accord) Mediator() string { return a.mediator }

// Members returns the actors with a declaration in the set, in actor-reference order
// (setup-contract-6, setup-contract-10).
func (a *Accord) Members() []string { return append([]string(nil), a.members...) }

// Member reports whether actor has a declaration in the set (setup-contract-6).
func (a *Accord) Member(actor string) bool {
	_, ok := a.terms[actor]
	return ok
}

// Roles returns the roles actor's binding gives it, in the binding's order (setup-contract-2).
func (a *Accord) Roles(actor string) []string { return append([]string(nil), a.roles[actor]...) }

// Terms returns actor's terms, in its declaration's order (setup-contract-1).
func (a *Accord) Terms(actor string) []Term {
	var out []Term
	for _, t := range a.terms[actor] {
		c := t
		c.With = append([]string(nil), t.With...)
		c.Kinds = append([]string(nil), t.Kinds...)
		c.Checks = append([]Check(nil), t.Checks...)
		if t.Resolve != nil {
			r := Rule{To: append([]string(nil), t.Resolve.To...), Timeout: t.Resolve.Timeout}
			c.Resolve = &r
		}
		out = append(out, c)
	}
	return out
}

type binding struct {
	actor, principal, mediator, until string
	roles                             []string
	bytes                             []byte
}

type declaration struct {
	actor any
	o     shape.Object
	bytes []byte
}

// Found founds the set at the founding time at, or refuses it with the first failing row of
// founding's check table (section 3, setup-contract-8). The set names its mediator; a mediator that
// founds only its own sets checks the Accord's Mediator.
func Found(set []byte, at string) (*Accord, error) {
	if !shape.Timestamp(at) {
		return nil, &Error{Code: CodeAt}
	}
	fail := func(c Code) (*Accord, error) { return nil, &Error{Code: c} }

	// Row 1: the canonical form.
	v, canon, err := shape.Decode(set)
	if err != nil {
		return fail(NotCanonical)
	}
	// Row 2: the whole shape.
	top, ok := v.(shape.Object)
	if !ok || !shape.Only(top, "policy", "mediator", "card", "declarations", "bindings") {
		return fail(InvalidShape)
	}
	principals, roles, ok := policy(top["policy"])
	if !ok {
		return fail(InvalidShape)
	}
	mediator, ok := mediatorOf(top["mediator"])
	if !ok || !card(top["card"]) {
		return fail(InvalidShape)
	}
	decls, ok := declarations(top["declarations"])
	if !ok {
		return fail(InvalidShape)
	}
	binds, ok := bindings(top["bindings"])
	if !ok {
		return fail(InvalidShape)
	}
	// Row 3: one binding per actor. The list is in order, so two for one actor are neighbours.
	for i := 1; i < len(binds); i++ {
		if binds[i].actor == binds[i-1].actor {
			return fail(BindingDuplicate)
		}
	}
	byActor := map[string]binding{}
	for _, b := range binds {
		byActor[b.actor] = b
	}

	a := &Accord{mediator: mediator, roles: map[string][]string{}, terms: map[string][]Term{}}
	// Rows 4 to 13, each declaration's in actor-reference order.
	for _, d := range decls {
		actor, ok := d.actor.(string)
		if _, isRef := signing.ParseReference(actor); !ok || !isRef {
			return fail(ActorReference)
		}
		if !signing.Verify(d.bytes, signing.TypeDeclaration, actor) {
			return fail(DeclarationSignature)
		}
		ts, ok := terms(d.o["terms"])
		if !ok {
			return fail(TermInvalid)
		}
		for _, t := range ts {
			if t.Stance == StanceMaybe && t.Resolve == nil {
				return fail(MaybeWithoutResolve)
			}
		}
		for _, t := range ts {
			if len(t.Checks) > 0 {
				// The card loads no check in this version (setup-contract-4), so every named check is
				// unknown.
				return fail(CheckUnknown)
			}
		}
		b, ok := byActor[actor]
		if !ok {
			return fail(BindingMissing)
		}
		if !principals[b.principal] {
			return fail(BindingPrincipal)
		}
		if !signing.Verify(b.bytes, signing.TypeBinding, b.principal) {
			return fail(BindingSignature)
		}
		if b.mediator != mediator {
			return fail(BindingMediator)
		}
		if expired(b.until, at) {
			return fail(BindingExpired)
		}
		a.members = append(a.members, actor)
		a.terms[actor] = ts
		a.roles[actor] = b.roles
	}
	// Row 14: a binding with no declaration.
	for _, b := range binds {
		if !a.Member(b.actor) {
			return fail(DeclarationMissing)
		}
	}
	// Row 15: every policy role held by a declaring actor.
	for _, r := range roles {
		held := false
		for _, m := range a.members {
			for _, mr := range a.roles[m] {
				held = held || mr == r
			}
		}
		if !held {
			return fail(RoleMissing)
		}
	}

	sum := sha256.Sum256(canon)
	a.id = canonical.HashPrefix + hex.EncodeToString(sum[:])
	cardBytes := shape.Encode(top["card"])
	cs := sha256.Sum256(cardBytes)
	a.card = canonical.HashPrefix + hex.EncodeToString(cs[:])
	return a, nil
}

// expired reports whether a binding's until is not later than the founding time. Both keep the
// timestamp rule, whose fixed form orders as text.
func expired(until, at string) bool { return until <= at }

func reference(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	_, ok = signing.ParseReference(s)
	return s, ok
}

func isReference(s string) bool {
	_, ok := signing.ParseReference(s)
	return ok
}

// policy reads the policy: exactly roles, distinct labels, and principals, a non-empty list of
// distinct actor references (setup-contract-9).
func policy(v any) (map[string]bool, []string, bool) {
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "roles", "principals") {
		return nil, nil, false
	}
	var roles []string
	if l, ok := o["roles"].([]any); ok && len(l) == 0 {
		roles = nil
	} else if roles, ok = shape.Strings(o["roles"], shape.Label); !ok {
		return nil, nil, false
	}
	ps, ok := shape.Strings(o["principals"], isReference)
	if !ok {
		return nil, nil, false
	}
	set := map[string]bool{}
	for _, p := range ps {
		set[p] = true
	}
	return set, roles, true
}

// mediatorOf reads the mediator: exactly actor and operator, actor references, and runs, process
// (setup-contract-7).
func mediatorOf(v any) (string, bool) {
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "actor", "operator", "runs") {
		return "", false
	}
	actor, ok := reference(o["actor"])
	if !ok {
		return "", false
	}
	if _, ok := reference(o["operator"]); !ok || o["runs"] != "process" {
		return "", false
	}
	return actor, true
}

// card reads the card for shape: spec 0.1, the kernel's version and hash, the reserved judges, and
// an empty checks (setup-contract-4).
func card(v any) bool {
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "spec", "kernel", "judges", "checks") || o["spec"] != Spec {
		return false
	}
	k, ok := o["kernel"].(shape.Object)
	if !ok || !shape.Only(k, "version", "hash") || !version(k["version"]) || !hash(k["hash"]) {
		return false
	}
	judges, ok := o["judges"].([]any)
	if !ok {
		return false
	}
	for _, j := range judges {
		c, ok := j.(shape.Object)
		if !ok || !shape.Only(c, "name", "version", "hash") || !label(c["name"]) || !version(c["version"]) || !hash(c["hash"]) {
			return false
		}
	}
	checks, ok := o["checks"].([]any)
	return ok && len(checks) == 0
}

func label(v any) bool {
	s, ok := v.(string)
	return ok && shape.Label(s)
}

func hash(v any) bool {
	s, ok := v.(string)
	return ok && shape.Hash(s)
}

// version reads a component's version: lowercase letters and digits in words joined by ".", "+" or
// "-", at most 64 characters (card.json).
func version(v any) bool {
	s, ok := v.(string)
	if !ok || s == "" || len(s) > 64 {
		return false
	}
	join := true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			join = false
		case c == '.' || c == '+' || c == '-':
			if join {
				return false
			}
			join = true
		default:
			return false
		}
	}
	return !join
}

// declarations reads the declarations for shape only: a non-empty list of objects with no member but
// actor, terms and sig, each actor present and a string, in strictly ascending actor-reference order
// (setup-contract-9, setup-contract-10).
func declarations(v any) ([]declaration, bool) {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return nil, false
	}
	var out []declaration
	prev := ""
	for i, x := range l {
		o, ok := x.(shape.Object)
		if !ok || !shape.Only(o, "actor", "", "terms", "sig") {
			return nil, false
		}
		actor, ok := o["actor"].(string)
		if !ok || (i > 0 && !canonical.Less(prev, actor)) {
			return nil, false
		}
		prev = actor
		out = append(out, declaration{actor: o["actor"], o: o, bytes: shape.Encode(o)})
	}
	return out, true
}

// bindings reads the bindings in full, in ascending actor-reference order, equal neighbours allowed
// (binding.json, setup-contract-10).
func bindings(v any) ([]binding, bool) {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return nil, false
	}
	var out []binding
	for i, x := range l {
		o, ok := x.(shape.Object)
		if !ok || !shape.Only(o, "actor", "principal", "roles", "mediator", "until", "sig") {
			return nil, false
		}
		var b binding
		if b.actor, ok = reference(o["actor"]); !ok {
			return nil, false
		}
		if b.principal, ok = reference(o["principal"]); !ok {
			return nil, false
		}
		if b.mediator, ok = reference(o["mediator"]); !ok {
			return nil, false
		}
		if b.roles, ok = shape.Strings(o["roles"], shape.Label); !ok {
			return nil, false
		}
		until, ok := o["until"].(string)
		if !ok || !shape.Timestamp(until) {
			return nil, false
		}
		b.until = until
		sig, ok := o["sig"].(string)
		if !ok || len(sig) != 128 || !shape.LowerHex(sig) {
			return nil, false
		}
		if i > 0 && canonical.Less(b.actor, out[i-1].actor) {
			return nil, false
		}
		b.bytes = shape.Encode(o)
		out = append(out, b)
	}
	return out, true
}

// terms reads a declaration's terms (setup-contract-1): a non-empty list of terms, each with exactly
// stance, dir, with and select, and checks and resolve where they may be. Whether a maybe term has its
// resolve is the next row's.
func terms(v any) ([]Term, bool) {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return nil, false
	}
	out := make([]Term, 0, len(l))
	for _, x := range l {
		o, ok := x.(shape.Object)
		if !ok || !shape.Only(o, "stance", "dir", "with", "select", "", "checks", "resolve") {
			return nil, false
		}
		var t Term
		t.Stance, _ = o["stance"].(string)
		switch t.Stance {
		case StanceNeed, StanceWant, StanceMaybe, StanceNo:
		default:
			return nil, false
		}
		t.Dir, _ = o["dir"].(string)
		if t.Dir != DirGive && t.Dir != DirGet {
			return nil, false
		}
		if t.With, ok = selectors(o["with"]); !ok {
			return nil, false
		}
		sel, ok := o["select"].(shape.Object)
		if !ok || !shape.Only(sel, "kind") {
			return nil, false
		}
		if t.Kinds, ok = shape.Strings(sel["kind"], shape.Label); !ok {
			return nil, false
		}
		if c, has := o["checks"]; has {
			cl, ok := c.([]any)
			if !ok || len(cl) == 0 {
				return nil, false
			}
			for _, y := range cl {
				co, ok := y.(shape.Object)
				if !ok || !shape.Only(co, "name", "corpus") || !label(co["name"]) || !label(co["corpus"]) {
					return nil, false
				}
				t.Checks = append(t.Checks, Check{Name: co["name"].(string), Corpus: co["corpus"].(string)})
			}
		}
		if r, has := o["resolve"]; has {
			if t.Stance != StanceMaybe {
				return nil, false
			}
			ro, ok := r.(shape.Object)
			if !ok || !shape.Only(ro, "to", "timeout") {
				return nil, false
			}
			to, ok := selectors(ro["to"])
			if !ok {
				return nil, false
			}
			timeout, ok := shape.Int(ro["timeout"])
			if !ok {
				return nil, false
			}
			t.Resolve = &Rule{To: to, Timeout: timeout}
		}
		out = append(out, t)
	}
	return out, true
}

// selectors reads a non-empty list of distinct role selectors, role:<label>, and returns the labels.
func selectors(v any) ([]string, bool) {
	l, ok := shape.Strings(v, func(s string) bool { _, ok := shape.Selector(s); return ok })
	if !ok {
		return nil, false
	}
	out := make([]string, len(l))
	for i, s := range l {
		out[i], _ = shape.Selector(s)
	}
	return out, true
}
