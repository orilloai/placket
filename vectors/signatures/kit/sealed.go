package kit

import (
	"sort"
)

// This file seals founding sets and builds what each member signs to consent: the seal of each
// declaration and its opening (setup-contract-12, setup-contract-13), each member's view
// (setup-contract-14) and its acceptance (setup-contract-15). The view is built here from the
// protocol text alone, with its own reading of terms and its own matching and decision rows for a
// crossing given by a member with no judgment; it never reaches the core. Its outcomes are checked
// by hand in the view group's generator, whose tables state them.

// TypeAcceptance is the type URI of an acceptance (signing-4).
const TypeAcceptance = Root + "acceptance/" + Spec

// SealSalt is the salt an actor seals its declaration under: 32 bytes from a label naming the actor
// as its declaration names it.
func SealSalt(actor any) string {
	s, _ := actor.(string)
	return Salt("seal "+s, 32)
}

// SealOf is the seal of a declaration under a salt in hex: sha256: and the hex SHA-256 of the salt's
// bytes and then the declaration's canonical bytes (setup-contract-12).
func SealOf(declaration Obj, salt string) string {
	return "sha256:" + Sum(FromHex(salt), Canon(declaration))
}

// Opening is a declaration's opening under a salt in hex (setup-contract-13).
func Opening(declaration Obj, salt string) Obj { return Obj{"declaration": declaration, "salt": salt} }

// An Item is one opening as a vector lists it: the actor and the opening as a JSON value.
func Item(actor any, opening any) Obj { return Obj{"actor": actor, "opening": opening} }

// opened is what the kit keeps of each set it sealed, by the sealed set's Accord id: the set as it was
// in the clear, and its openings as a vector lists them.
var opened = map[string]struct {
	open     Obj
	openings []any
}{}

// Seal seals every declaration of a set given in the clear, in the order its list holds them, each
// under its actor's SealSalt, and returns the sealed set and its openings as a vector lists them. The
// sealed set is remembered, so Openings and Acceptances can find what it was sealed from.
func Seal(open Obj) (Obj, []any) {
	sealed := Copy(open).(Obj)
	var entries, openings []any
	for _, x := range open["declarations"].([]any) {
		d := x.(Obj)
		salt := SealSalt(d["actor"])
		entries = append(entries, Obj{"actor": d["actor"], "seal": SealOf(d, salt)})
		// An actor that is not a string names no file to hold an opening.
		if _, ok := d["actor"].(string); ok {
			openings = append(openings, Item(d["actor"], Opening(Copy(d).(Obj), salt)))
		}
	}
	if entries == nil {
		entries = []any{}
	}
	sealed["declarations"] = entries
	opened[HashOf(sealed)] = struct {
		open     Obj
		openings []any
	}{Copy(open).(Obj), openings}
	return sealed, openings
}

// Openings returns every opening of a set Seal sealed, as a vector lists them.
func Openings(sealed Obj) []any {
	o, ok := opened[HashOf(sealed)]
	if !ok {
		panic("kit: a set the kit did not seal")
	}
	return Copy(o.openings).([]any)
}

// Open returns the set a sealed set was sealed from, in the clear.
func Open(sealed Obj) Obj {
	o, ok := opened[HashOf(sealed)]
	if !ok {
		panic("kit: a set the kit did not seal")
	}
	return Copy(o.open).(Obj)
}

// keyByRef is every key NewKey made, by its reference.
var keyByRef = map[string]Key{}

// KeyOf returns the key of a reference NewKey made.
func KeyOf(ref string) Key {
	k, ok := keyByRef[ref]
	if !ok {
		panic("kit: no key for " + ref)
	}
	return k
}

// Acceptance builds the acceptance of a view by a member (setup-contract-15): the Accord id, the
// view's typed hash and the member's reference, signed under TypeAcceptance by its key.
func Acceptance(accord string, view Obj, from Key) Obj {
	return Signed(Obj{"accord": accord, "view": HashOf(view), "from": from.Ref}, TypeAcceptance, from)
}

// Acceptances is every member's acceptance of its own view of a set Seal sealed, in strict from
// order: acceptances.json (setup-contract-15).
func Acceptances(sealed Obj) []any {
	open, accord := Open(sealed), HashOf(sealed)
	var out []any
	for _, m := range ViewMembers(open) {
		out = append(out, Acceptance(accord, View(open, accord, m), KeyOf(m)))
	}
	return out
}

// A term as the view reads it (setup-contract-1).
type vterm struct {
	stance, dir string
	with, kinds []string
	to          []string
	timeout     int64
	judges      []any
	maybe       bool
}

func strList(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

func labels(selectors []string) []string {
	var out []string
	for _, s := range selectors {
		if len(s) > 5 && s[:5] == "role:" {
			out = append(out, s[5:])
		}
	}
	return out
}

// world is a set in the clear as the view reads it: its members in actor-reference order, each one's
// terms, and the roles each one's binding gives it.
type world struct {
	members []string
	terms   map[string][]vterm
	roles   map[string][]string
}

func read(open Obj) world {
	w := world{terms: map[string][]vterm{}, roles: map[string][]string{}}
	for _, x := range open["declarations"].([]any) {
		d := x.(Obj)
		a := d["actor"].(string)
		w.members = append(w.members, a)
		for _, y := range d["terms"].([]any) {
			t := y.(Obj)
			v := vterm{stance: t["stance"].(string), dir: t["dir"].(string), with: labels(strList(t["with"])),
				kinds: strList(t["select"].(Obj)["kind"])}
			if r, ok := t["resolve"].(Obj); ok {
				v.maybe = true
				v.to = labels(strList(r["to"]))
				switch n := r["timeout"].(type) {
				case int64:
					v.timeout = n
				case int:
					v.timeout = int64(n)
				}
				if j, ok := r["judges"].([]any); ok {
					v.judges = j
				}
			}
			w.terms[a] = append(w.terms[a], v)
		}
	}
	for _, x := range open["bindings"].([]any) {
		b := x.(Obj)
		a := b["actor"].(string)
		if _, seen := w.roles[a]; !seen {
			w.roles[a] = strList(b["roles"])
		}
	}
	sort.Slice(w.members, func(i, j int) bool { return Less16(w.members[i], w.members[j]) })
	return w
}

// ViewMembers is the members of a set in the clear, in actor-reference order.
func ViewMembers(open Obj) []string { return read(open).members }

func has(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func meets(a, b []string) bool {
	for _, x := range a {
		if has(b, x) {
			return true
		}
	}
	return false
}

// A verdictOf is a crossing's decision as a view shows it: crosses, blocked or asked, and for asked the
// agreed rule's roles sorted, its timeout and the judges both sides consented to.
type verdictOf struct {
	word    string
	to      []string
	timeout int64
	judges  []any
}

// decide reads section 5's rows for a part of kind given by a member from one member to another, with
// no judgment: a matching no blocks; no match on either side blocks; no maybe crosses; maybe rules that
// differ in their roles or timeout block; otherwise it is asked, and the judges are consented to only
// when every matching term is a maybe listing the same judges.
func (w world) decide(from, to, kind string) verdictOf {
	var matching []vterm
	sent, got := 0, 0
	for _, t := range w.terms[from] {
		if t.dir == "give" && has(t.kinds, kind) && meets(t.with, w.roles[to]) {
			matching, sent = append(matching, t), sent+1
		}
	}
	for _, t := range w.terms[to] {
		if t.dir == "get" && has(t.kinds, kind) && meets(t.with, w.roles[from]) {
			matching, got = append(matching, t), got+1
		}
	}
	for _, t := range matching {
		if t.stance == "no" {
			return verdictOf{word: "blocked"}
		}
	}
	if sent == 0 || got == 0 {
		return verdictOf{word: "blocked"}
	}
	var maybes []vterm
	all := true
	for _, t := range matching {
		if t.maybe {
			maybes = append(maybes, t)
		} else {
			all = false
		}
	}
	if len(maybes) == 0 {
		return verdictOf{word: "crosses"}
	}
	first := append([]string(nil), maybes[0].to...)
	sort.Strings(first)
	for _, t := range maybes[1:] {
		o := append([]string(nil), t.to...)
		sort.Strings(o)
		if t.timeout != maybes[0].timeout || string(Canon(o)) != string(Canon(first)) {
			return verdictOf{word: "blocked"}
		}
	}
	out := verdictOf{word: "asked", to: first, timeout: maybes[0].timeout}
	if all && len(maybes[0].judges) > 0 {
		same := true
		for _, t := range maybes[1:] {
			if string(Canon(t.judges)) != string(Canon(maybes[0].judges)) {
				same = false
			}
		}
		if same {
			out.judges = maybes[0].judges
		}
	}
	return out
}

// View is a member's view of a set in the clear, under the Accord id (setup-contract-14).
func View(open Obj, accord, viewer string) Obj {
	w := read(open)
	type row struct {
		counterpart, dir, kind string
		o                      verdictOf
	}
	var rows []row
	seen := map[[3]string]bool{}
	for _, t := range w.terms[viewer] {
		for _, c := range w.members {
			if !meets(t.with, w.roles[c]) {
				continue
			}
			for _, k := range t.kinds {
				key := [3]string{c, t.dir, k}
				if seen[key] {
					continue
				}
				seen[key] = true
				o := w.decide(viewer, c, k)
				if t.dir == "get" {
					o = w.decide(c, viewer, k)
				}
				rows = append(rows, row{c, t.dir, k, o})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		x, y := rows[i], rows[j]
		if x.counterpart != y.counterpart {
			return Less16(x.counterpart, y.counterpart)
		}
		if x.dir != y.dir {
			return x.dir < y.dir
		}
		return x.kind < y.kind
	})
	crossings := []any{}
	for _, r := range rows {
		o := Obj{"counterpart": r.counterpart, "roles": strs(w.roles[r.counterpart]), "dir": r.dir, "kind": r.kind, "outcome": r.o.word}
		if r.o.word == "asked" {
			o["to"], o["timeout"] = selectors(r.o.to), r.o.timeout
			if r.o.judges != nil {
				o["judges"] = Copy(r.o.judges)
			}
		}
		crossings = append(crossings, o)
	}
	type ask struct {
		asker, receiver, kind string
		timeout               int64
	}
	var asks []ask
	seenAsk := map[[3]string]bool{}
	for _, s := range w.members {
		if s == viewer {
			continue
		}
		for _, t := range w.terms[s] {
			if t.dir != "give" {
				continue
			}
			for _, r := range w.members {
				if r == viewer {
					continue
				}
				for _, k := range t.kinds {
					key := [3]string{s, r, k}
					if seenAsk[key] {
						continue
					}
					seenAsk[key] = true
					if o := w.decide(s, r, k); o.word == "asked" && meets(o.to, w.roles[viewer]) {
						asks = append(asks, ask{s, r, k, o.timeout})
					}
				}
			}
		}
	}
	sort.Slice(asks, func(i, j int) bool {
		x, y := asks[i], asks[j]
		if x.asker != y.asker {
			return Less16(x.asker, y.asker)
		}
		if x.receiver != y.receiver {
			return Less16(x.receiver, y.receiver)
		}
		return x.kind < y.kind
	})
	answers := []any{}
	for _, a := range asks {
		answers = append(answers, Obj{"asker": a.asker, "receiver": a.receiver, "kind": a.kind, "timeout": a.timeout})
	}
	return Obj{"accord": accord, "viewer": viewer, "salt": SealSalt(viewer), "crossings": crossings, "answers": answers}
}
