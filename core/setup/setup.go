// Package setup founds an Accord from a founding set, or refuses the set with the first code of
// founding's check table, and holds what the founded Accord says: its id and card hash, its members,
// the roles their bindings give them, the principals that signed those bindings, and their terms.
//
// It implements setup-contract-1 to setup-contract-11: declarations and terms, a maybe term's judges
// among them (setup-contract-1), bindings and roles (setup-contract-2, setup-contract-3), the card
// and its judges (setup-contract-4), the Accord id
// (setup-contract-5), a principal per party and the roles the policy lets it grant
// (setup-contract-2, setup-contract-3, setup-contract-9), membership (setup-contract-6), the mediator and its operator
// (setup-contract-7), the refusal order (setup-contract-8), the shape of a set (setup-contract-9) and
// the order of its lists (setup-contract-10), and the earlier Accord a policy supersedes
// (setup-contract-11).
package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/signing"
)

// Code is a founding refusal code, spelled as the check table of section 3 spells it
// (setup-contract-1 to setup-contract-10, record 0005).
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
	// JudgeUnknown: a term's resolve names a judge the card's judges do not list (setup-contract-1,
	// setup-contract-8).
	JudgeUnknown     Code = "judge-unknown"
	BindingMissing   Code = "binding-missing"
	BindingPrincipal Code = "binding-principal"
	BindingSignature Code = "binding-signature"
	// BindingRole: a binding gives a role its principal's policy entry does not list
	// (setup-contract-3).
	BindingRole Code = "binding-role"
	// BindingPolicy: a binding names the typed hash of a policy other than the set's
	// (setup-contract-3).
	BindingPolicy      Code = "binding-policy"
	BindingMediator    Code = "binding-mediator"
	BindingExpired     Code = "binding-expired"
	DeclarationMissing Code = "declaration-missing"
	RoleMissing        Code = "role-missing"
)

// CheckUnknown was row 8's code, when a term could name a check.
//
// Deprecated: founding never returns it. Row 8 is JudgeUnknown, and a term with checks is
// TermInvalid (docs/adr/0021-who-decided-and-the-answer-call.md). It is removed at the release.
const CheckUnknown Code = "check-unknown"

// CodeAt is not a founding code: the founding time its caller gave does not keep the timestamp rule.
const CodeAt Code = "at"

// Error is a refused founding: the code of the first row that failed. It carries nothing of the set
// (record 0005).
type Error struct{ Code Code }

// Error returns the package and the code, and nothing of the set.
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
	// Checks is always nil.
	//
	// Deprecated: a term names no check; founding refuses a term with checks as TermInvalid
	// (docs/adr/0021-who-decided-and-the-answer-call.md). It is removed at the release.
	Checks []Check
	// Resolve is the rule of a maybe term, and nil on any other.
	Resolve *Rule
}

// A Check was a check a term named.
//
// Deprecated: nothing builds or reads one (docs/adr/0021-who-decided-and-the-answer-call.md). It is
// removed at the release.
type Check struct{ Name, Corpus string }

// Use is what a term grants a judge: to decide where the terms would ask, or to advise only
// (setup-contract-1, composition-7).
type Use string

// The two uses of a judge (setup-contract-1).
const (
	Decide Use = "decide"
	Advise Use = "advise"
)

// A Judge is one entry of a maybe term's resolve.judges: the judge's card name and the use the term
// grants it (setup-contract-1, composition-7).
type Judge struct {
	Name string
	Use  Use
}

// A CardJudge is one entry of the card's judges: a judge's name, and the version and hash that pin
// its build (setup-contract-4). It is distinct from Judge, since a term grants a use and the card
// pins a build.
type CardJudge struct{ Name, Version, Hash string }

// A Rule is a maybe term's resolve: the role labels of who answers, the timeout in seconds, and the
// judges it lists, in order (setup-contract-1, maybe-1, composition-7).
type Rule struct {
	To      []string
	Timeout int64
	Judges  []Judge
}

// An Accord is a founded founding set (setup-contract-5, record 0005). Its
// read methods take a nil *Accord as an Accord with no members and no terms, and return zero values,
// so a caller handed none fails closed and never panics (core-no-panic).
type Accord struct {
	id, card, mediator, operator string
	supersedes                   [2]string
	judges                       []CardJudge
	members                      []string
	roles                        map[string][]string
	principals                   map[string]string
	terms                        map[string][]Term
}

// ID returns the Accord id: the typed hash of the set's canonical bytes (setup-contract-5).
func (a *Accord) ID() string {
	if a == nil {
		return ""
	}
	return a.id
}

// Card returns the card hash: the typed hash of the card's canonical bytes (setup-contract-4).
func (a *Accord) Card() string {
	if a == nil {
		return ""
	}
	return a.card
}

// Mediator returns the reference of the mediator the set names, its mediator.actor
// (setup-contract-7).
func (a *Accord) Mediator() string {
	if a == nil {
		return ""
	}
	return a.mediator
}

// Operator returns the reference of the party that runs the mediator, the set's mediator.operator
// (setup-contract-7).
func (a *Accord) Operator() string {
	if a == nil {
		return ""
	}
	return a.operator
}

// CardJudges returns the card's judges, in card order (setup-contract-4).
func (a *Accord) CardJudges() []CardJudge {
	if a == nil {
		return nil
	}
	return append([]CardJudge(nil), a.judges...)
}

// JudgeKinds returns the kinds of every term, of any member, whose resolve lists the judge named
// name, sorted and each once (setup-contract-1, docs/adr/0023-judges.md). Only a maybe term has a
// resolve, so it compares no stance and decides nothing: it is a superset of the kinds that can
// reach the judge, since both sides must agree (composition-7), and the window prints it at start.
func (a *Accord) JudgeKinds(name string) []string {
	if a == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range a.members {
		for _, t := range a.terms[m] {
			if t.Resolve == nil || !listsJudge(t.Resolve.Judges, name) {
				continue
			}
			for _, k := range t.Kinds {
				if !seen[k] {
					seen[k] = true
					out = append(out, k)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// listsJudge reports whether a resolve's judges list one named name.
func listsJudge(judges []Judge, name string) bool {
	for _, j := range judges {
		if j.Name == name {
			return true
		}
	}
	return false
}

// Supersedes returns the earlier Accord the policy supersedes, by its id and the id of its close
// receipt; ok is false when the policy has no supersedes (setup-contract-11).
func (a *Accord) Supersedes() (accord, close string, ok bool) {
	if a == nil || a.supersedes[0] == "" {
		return "", "", false
	}
	return a.supersedes[0], a.supersedes[1], true
}

// Members returns the actors with a declaration in the set, in actor-reference order
// (setup-contract-6, setup-contract-10).
func (a *Accord) Members() []string {
	if a == nil {
		return nil
	}
	return append([]string(nil), a.members...)
}

// Member reports whether actor has a declaration in the set (setup-contract-6).
func (a *Accord) Member(actor string) bool {
	if a == nil {
		return false
	}
	_, ok := a.terms[actor]
	return ok
}

// Roles returns the roles actor's binding gives it, in the binding's order (setup-contract-2).
func (a *Accord) Roles(actor string) []string {
	if a == nil {
		return nil
	}
	return append([]string(nil), a.roles[actor]...)
}

// Principal returns the reference of the principal that signed actor's binding, or "" when actor is
// not a member (setup-contract-2).
func (a *Accord) Principal(actor string) string {
	if a == nil {
		return ""
	}
	return a.principals[actor]
}

// Terms returns actor's terms, in its declaration's order (setup-contract-1).
func (a *Accord) Terms(actor string) []Term {
	if a == nil {
		return nil
	}
	var out []Term
	for _, t := range a.terms[actor] {
		c := t
		c.With = append([]string(nil), t.With...)
		c.Kinds = append([]string(nil), t.Kinds...)
		c.Checks = nil
		if t.Resolve != nil {
			r := Rule{To: append([]string(nil), t.Resolve.To...), Timeout: t.Resolve.Timeout,
				Judges: append([]Judge(nil), t.Resolve.Judges...)}
			c.Resolve = &r
		}
		out = append(out, c)
	}
	return out
}

type binding struct {
	actor, principal, policy, mediator, until string
	roles                                     []string
	bytes                                     []byte
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
	grants, roles, supersedes, ok := policy(top["policy"])
	if !ok {
		return fail(InvalidShape)
	}
	// The one check that ties a binding to its policy fails closed: a policy that does not encode
	// refuses the set rather than hashing to the hash of nothing.
	policyBytes := shape.Encode(top["policy"])
	if policyBytes == nil {
		return fail(InvalidShape)
	}
	ps := sha256.Sum256(policyBytes)
	policyHash := canonical.HashPrefix + hex.EncodeToString(ps[:])
	mediator, operator, ok := mediatorOf(top["mediator"])
	if !ok {
		return fail(InvalidShape)
	}
	judges, ok := card(top["card"])
	if !ok {
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

	a := &Accord{mediator: mediator, operator: operator, supersedes: supersedes, judges: judges, roles: map[string][]string{}, principals: map[string]string{}, terms: map[string][]Term{}}
	// Rows 4 to 15, each declaration's in actor-reference order.
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
		// Row 8: every judge a resolve names is a judge of the card (setup-contract-1).
		for _, t := range ts {
			if t.Resolve == nil {
				continue
			}
			for _, j := range t.Resolve.Judges {
				if !hasJudge(judges, j.Name) {
					return fail(JudgeUnknown)
				}
			}
		}
		b, ok := byActor[actor]
		if !ok {
			return fail(BindingMissing)
		}
		grant, ok := grants[b.principal]
		if !ok {
			return fail(BindingPrincipal)
		}
		if !signing.Verify(b.bytes, signing.TypeBinding, b.principal) {
			return fail(BindingSignature)
		}
		for _, r := range b.roles {
			if !grant[r] {
				return fail(BindingRole)
			}
		}
		if b.policy != policyHash {
			return fail(BindingPolicy)
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
		a.principals[actor] = b.principal
	}
	// Row 16: a binding with no declaration.
	for _, b := range binds {
		if !a.Member(b.actor) {
			return fail(DeclarationMissing)
		}
	}
	// Row 17: every policy role held by a declaring actor.
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

// hasJudge reports whether the card's judges list one named name.
func hasJudge(judges []CardJudge, name string) bool {
	for _, j := range judges {
		if j.Name == name {
			return true
		}
	}
	return false
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

// policy reads the policy: exactly roles, distinct labels, and principals, a non-empty list of
// entries, each exactly a principal, an actor reference no other entry names, and roles, a non-empty
// list of distinct labels (setup-contract-9); and optionally supersedes, exactly accord and close,
// two typed hashes (setup-contract-11). It returns the roles each principal may grant, and the
// earlier Accord's id and close receipt's id, or two empty strings.
func policy(v any) (map[string]map[string]bool, []string, [2]string, bool) {
	var sup [2]string
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "roles", "principals", "", "supersedes") {
		return nil, nil, sup, false
	}
	if x, has := o["supersedes"]; has {
		so, ok := x.(shape.Object)
		if !ok || !shape.Only(so, "accord", "close") || !hash(so["accord"]) || !hash(so["close"]) {
			return nil, nil, sup, false
		}
		sup = [2]string{so["accord"].(string), so["close"].(string)}
	}
	var roles []string
	if l, ok := o["roles"].([]any); ok && len(l) == 0 {
		roles = nil
	} else if roles, ok = shape.Strings(o["roles"], shape.Label); !ok {
		return nil, nil, sup, false
	}
	l, ok := o["principals"].([]any)
	if !ok || len(l) == 0 {
		return nil, nil, sup, false
	}
	grants := map[string]map[string]bool{}
	for _, x := range l {
		e, ok := x.(shape.Object)
		if !ok || !shape.Only(e, "principal", "roles") {
			return nil, nil, sup, false
		}
		p, ok := reference(e["principal"])
		if _, seen := grants[p]; !ok || seen {
			return nil, nil, sup, false
		}
		rs, ok := shape.Strings(e["roles"], shape.Label)
		if !ok {
			return nil, nil, sup, false
		}
		grants[p] = map[string]bool{}
		for _, r := range rs {
			grants[p][r] = true
		}
	}
	return grants, roles, sup, true
}

// mediatorOf reads the mediator: exactly actor and operator, actor references, and runs, process
// (setup-contract-7). It returns the actor and the operator.
func mediatorOf(v any) (string, string, bool) {
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "actor", "operator", "runs") {
		return "", "", false
	}
	actor, ok := reference(o["actor"])
	if !ok {
		return "", "", false
	}
	operator, ok := reference(o["operator"])
	if !ok || o["runs"] != "process" {
		return "", "", false
	}
	return actor, operator, true
}

// card reads the card for shape: exactly spec 0.1, the kernel's version and hash, and judges, an
// ordered list of a name, a version and a hash each, no two of one name (setup-contract-4). It
// returns the judges.
func card(v any) ([]CardJudge, bool) {
	o, ok := v.(shape.Object)
	if !ok || !shape.Only(o, "spec", "kernel", "judges") || o["spec"] != Spec {
		return nil, false
	}
	k, ok := o["kernel"].(shape.Object)
	if !ok || !shape.Only(k, "version", "hash") || !version(k["version"]) || !hash(k["hash"]) {
		return nil, false
	}
	l, ok := o["judges"].([]any)
	if !ok {
		return nil, false
	}
	judges := []CardJudge{}
	for _, j := range l {
		c, ok := j.(shape.Object)
		if !ok || !shape.Only(c, "name", "version", "hash") || !label(c["name"]) || !version(c["version"]) || !hash(c["hash"]) {
			return nil, false
		}
		name := c["name"].(string)
		if hasJudge(judges, name) {
			return nil, false
		}
		judges = append(judges, CardJudge{Name: name, Version: c["version"].(string), Hash: c["hash"].(string)})
	}
	return judges, true
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
		if !ok || !shape.Only(o, "actor", "principal", "roles", "policy", "mediator", "until", "sig") {
			return nil, false
		}
		var b binding
		if b.actor, ok = reference(o["actor"]); !ok {
			return nil, false
		}
		if b.principal, ok = reference(o["principal"]); !ok {
			return nil, false
		}
		if b.policy, ok = o["policy"].(string); !ok || !shape.Hash(b.policy) {
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
// stance, dir, with and select, and resolve where it may be, with its judges. A term with checks
// holds a member the term does not list. Whether a maybe term has its resolve is the next row's, and
// whether the card has each judge the row after.
func terms(v any) ([]Term, bool) {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return nil, false
	}
	out := make([]Term, 0, len(l))
	for _, x := range l {
		o, ok := x.(shape.Object)
		if !ok || !shape.Only(o, "stance", "dir", "with", "select", "", "resolve") {
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
		if r, has := o["resolve"]; has {
			if t.Stance != StanceMaybe {
				return nil, false
			}
			ro, ok := r.(shape.Object)
			if !ok || !shape.Only(ro, "to", "timeout", "", "judges") {
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
			if j, has := ro["judges"]; has {
				if t.Resolve.Judges, ok = termJudges(j); !ok {
					return nil, false
				}
			}
		}
		out = append(out, t)
	}
	return out, true
}

// termJudges reads a resolve's judges: a non-empty ordered list of objects of exactly a name, a
// label, and a use, decide or advise, no two of one name (setup-contract-1).
func termJudges(v any) ([]Judge, bool) {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return nil, false
	}
	out := make([]Judge, 0, len(l))
	seen := map[string]bool{}
	for _, x := range l {
		o, ok := x.(shape.Object)
		if !ok || !shape.Only(o, "name", "use") || !label(o["name"]) {
			return nil, false
		}
		name := o["name"].(string)
		u, _ := o["use"].(string)
		use := Use(u)
		if (use != Decide && use != Advise) || seen[name] {
			return nil, false
		}
		seen[name] = true
		out = append(out, Judge{Name: name, Use: use})
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
