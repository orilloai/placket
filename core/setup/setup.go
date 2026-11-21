// Package setup founds an Accord from a founding set, or refuses the set with the first code of
// founding's check table, and holds what the founded Accord says: its id and card hash, its members,
// the roles their bindings give them, the principals that signed those bindings, and their terms.
//
// It implements setup-contract-1 to setup-contract-13 and setup-contract-16: declarations and terms, a maybe term's judges
// among them (setup-contract-1), bindings and roles (setup-contract-2, setup-contract-3), the card
// and its judges (setup-contract-4), the Accord id
// (setup-contract-5), a principal per party and the roles the policy lets it grant
// (setup-contract-2, setup-contract-3, setup-contract-9), membership (setup-contract-6), the mediator and its operator
// (setup-contract-7), the refusal order (setup-contract-8), the shape of a set (setup-contract-9) and
// the order of its lists (setup-contract-10), the earlier Accord a policy supersedes
// (setup-contract-11), the seal of each declaration and its opening (setup-contract-12,
// setup-contract-13), and founding without the openings (setup-contract-16). It runs rows 1 to 17 of
// founding's table; rows 18 to 21, which read the acceptances and the views, are core/founding's,
// since a view is composition's (docs/adr/0025-sealed-terms.md).
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
	// OpeningMissing and SealMismatch are row 4a's: no opening is held for a seal entry, or the
	// opening does not open it (setup-contract-13).
	OpeningMissing Code = "opening-missing"
	SealMismatch   Code = "seal-mismatch"
	// The codes of rows 18 to 21, which core/founding returns: no acceptance from a member, one not
	// signed by it, one that names another Accord, and one whose view is not the member's
	// (setup-contract-15).
	AcceptanceMissing   Code = "acceptance-missing"
	AcceptanceSignature Code = "acceptance-signature"
	AcceptanceAccord    Code = "acceptance-accord"
	AcceptanceView      Code = "acceptance-view"
)

// SaltBytes is the length of a seal's salt (setup-contract-12).
const SaltBytes = 32

// Openings are the openings a founder holds, each an opening's bytes under its actor's reference
// (setup-contract-13, setup-contract-16). With ByActor nil and Some false the founder holds none,
// as any party or verifier does: rows 4a to 8 are not run and the terms are not checked. With
// ByActor set and Some false the founder holds every member's, as the window's operator does: a
// member with none is opening-missing. With Some set it holds some members' only, as a verifier of a
// dispute does: a member with none is not opened.
type Openings struct {
	ByActor map[string][]byte
	Some    bool
}

// CodeAt is not a founding code: the founding time its caller gave does not keep the timestamp rule.
const CodeAt Code = "at"

// CodeViewer is not a founding code: a view was asked of an actor that is not a member, or of an
// Accord whose seals were not all opened, so its view cannot be built (setup-contract-14).
const CodeViewer Code = "viewer"

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
	// Resolve is the rule of a maybe term, and nil on any other.
	Resolve *Rule
}

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
	member                       map[string]bool
	roles                        map[string][]string
	principals                   map[string]string
	terms                        map[string][]Term
	// salts holds each opened member's opening salt; a member not opened has none.
	salts map[string][]byte
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

// Member reports whether actor has a declaration in the set, by its seal (setup-contract-6).
func (a *Accord) Member(actor string) bool {
	if a == nil {
		return false
	}
	return a.member[actor]
}

// Opened reports whether actor's seal was opened at founding, so its terms were read
// (setup-contract-13, setup-contract-16).
func (a *Accord) Opened(actor string) bool {
	if a == nil {
		return false
	}
	_, ok := a.salts[actor]
	return ok
}

// TermsChecked reports whether every member's seal was opened at founding, so rows 4a to 8 ran for
// every declaration (setup-contract-16).
func (a *Accord) TermsChecked() bool {
	if a == nil || len(a.members) == 0 {
		return false
	}
	return len(a.salts) == len(a.members)
}

// Salt returns a copy of the salt of actor's opening, or nil when its seal was not opened
// (setup-contract-12, setup-contract-14).
func (a *Accord) Salt(actor string) []byte {
	if a == nil {
		return nil
	}
	s, ok := a.salts[actor]
	if !ok {
		return nil
	}
	return append([]byte(nil), s...)
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

// Terms returns actor's terms, in its declaration's order, or nil when its seal was not opened
// (setup-contract-1, setup-contract-16).
func (a *Accord) Terms(actor string) []Term {
	if a == nil {
		return nil
	}
	var out []Term
	for _, t := range a.terms[actor] {
		c := t
		c.With = append([]string(nil), t.With...)
		c.Kinds = append([]string(nil), t.Kinds...)
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

// An entry is a seal entry of the set's declarations: its actor, as read, and its seal
// (setup-contract-12).
type entry struct {
	actor any
	seal  string
}

// Found runs rows 1 to 17 of founding's check table over the set at the founding time at, with the
// openings given, and returns the Accord they found or the first failing row's code (section 3,
// setup-contract-8). Rows 4a to 8 run for each member whose opening is given; with none, its terms are
// not read (setup-contract-16). The id is the typed hash of the set. The set names its mediator; a
// mediator that founds only its own sets checks the Accord's Mediator. Rows 18 to 21, the
// acceptances, are core/founding's.
func Found(set []byte, openings Openings, at string) (*Accord, error) {
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

	a := &Accord{mediator: mediator, operator: operator, supersedes: supersedes, judges: judges, member: map[string]bool{},
		roles: map[string][]string{}, principals: map[string]string{}, terms: map[string][]Term{}, salts: map[string][]byte{}}
	every := openings.ByActor != nil && !openings.Some
	// Rows 4 to 15, each declaration's in actor-reference order.
	for _, d := range decls {
		actor, ok := d.actor.(string)
		if _, isRef := signing.ParseReference(actor); !ok || !isRef {
			return fail(ActorReference)
		}
		// Row 4a: the opening opens the seal (setup-contract-13). Rows 5 to 8 then read the opened
		// declaration; a member whose opening the founder does not hold is not opened.
		opening, held := openings.ByActor[actor]
		if !held && every {
			return fail(OpeningMissing)
		}
		var ts []Term
		var salt []byte
		if held {
			o, s, code := open(opening, actor, d.seal)
			if code != "" {
				return fail(code)
			}
			salt = s
			if !signing.Verify(shape.Encode(o), signing.TypeDeclaration, actor) {
				return fail(DeclarationSignature)
			}
			if ts, ok = terms(o["terms"]); !ok {
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
		a.member[actor] = true
		if held {
			a.terms[actor] = ts
			a.salts[actor] = salt
		}
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

// open is row 4a for one seal entry (setup-contract-13): the opening's bytes, accepted by the
// canonical form, hold exactly declaration and salt, the salt 32 bytes in lowercase hex, and the
// declaration an object of no member but actor, terms and sig, its actor a string; else
// invalid-shape. Its actor is the entry's, and the salted digest of its canonical bytes under the salt
// is the seal; else seal-mismatch. It returns the declaration and the salt.
func open(opening []byte, actor, seal string) (shape.Object, []byte, Code) {
	v, _, err := shape.Decode(opening)
	o, ok := v.(shape.Object)
	if err != nil || !ok || !shape.Only(o, "declaration", "salt") {
		return nil, nil, InvalidShape
	}
	hexSalt, ok := o["salt"].(string)
	if !ok || len(hexSalt) != 2*SaltBytes || !shape.LowerHex(hexSalt) {
		return nil, nil, InvalidShape
	}
	salt, err := hex.DecodeString(hexSalt)
	if err != nil {
		return nil, nil, InvalidShape
	}
	d, ok := o["declaration"].(shape.Object)
	if !ok || !shape.Only(d, "actor", "", "terms", "sig") {
		return nil, nil, InvalidShape
	}
	da, ok := d["actor"].(string)
	if !ok {
		return nil, nil, InvalidShape
	}
	if da != actor || sealOf(shape.Encode(d), salt) != seal {
		return nil, nil, SealMismatch
	}
	return d, salt, ""
}

// sealOf is the seal of a declaration's canonical bytes under salt: sha256: and the hex of the
// SHA-256 of the salt and then the bytes, crossing-3's salted digest (setup-contract-12). It is ""
// for bytes that are nil, which shape.Encode returns only for a value it cannot write.
func sealOf(declaration, salt []byte) string {
	if declaration == nil {
		return ""
	}
	h := sha256.New()
	h.Write(salt)
	h.Write(declaration)
	return canonical.HashPrefix + hex.EncodeToString(h.Sum(nil))
}

// Seal seals a signed declaration under a salt its actor minted (setup-contract-12): it returns the
// canonical bytes of the seal entry, {"actor","seal"}, and of the opening, {"declaration","salt"}
// (setup-contract-13). The declaration must be accepted by the canonical form and be an object whose
// actor is a string; the salt must be SaltBytes long. It checks no signature and no term: founding
// does. A refusal is a *Error of InvalidShape and carries nothing of the declaration.
func Seal(declaration, salt []byte) (entry, opening []byte, err error) {
	v, _, derr := shape.Decode(declaration)
	d, ok := v.(shape.Object)
	if derr != nil || !ok || len(salt) != SaltBytes {
		return nil, nil, &Error{Code: InvalidShape}
	}
	actor, ok := d["actor"].(string)
	if !ok {
		return nil, nil, &Error{Code: InvalidShape}
	}
	e := shape.Encode(shape.Object{"actor": actor, "seal": sealOf(shape.Encode(d), salt)})
	o := shape.Encode(shape.Object{"declaration": d, "salt": hex.EncodeToString(salt)})
	if e == nil || o == nil {
		return nil, nil, &Error{Code: InvalidShape}
	}
	return e, o, nil
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

// declarations reads the declarations' seal entries: a non-empty list of objects of exactly actor, a
// string, and seal, a typed hash, in strictly ascending actor-reference order (setup-contract-9,
// setup-contract-10, setup-contract-12). Whether actor is a reference is row 4's.
func declarations(v any) ([]entry, bool) {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return nil, false
	}
	var out []entry
	prev := ""
	for i, x := range l {
		o, ok := x.(shape.Object)
		if !ok || !shape.Only(o, "actor", "seal") {
			return nil, false
		}
		actor, ok := o["actor"].(string)
		if !ok || (i > 0 && !canonical.Less(prev, actor)) {
			return nil, false
		}
		seal, ok := o["seal"].(string)
		if !ok || !shape.Hash(seal) {
			return nil, false
		}
		prev = actor
		out = append(out, entry{actor: o["actor"], seal: seal})
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
