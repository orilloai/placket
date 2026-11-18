// Package founding founds an Accord from its sealed founding set, the openings its founder holds and
// every member's acceptance, by all 21 rows of founding's check table: setup's rows 1 to 17, then the
// acceptances' rows 18 to 21. It builds each member's view, the hashed object a member signs to
// consent, and the acceptance it signs.
//
// It implements setup-contract-14 (the view), setup-contract-15 (the acceptance and the founding
// record) and setup-contract-16 (founding without the openings), and with setup, setup-contract-8's
// order across all its rows. It is a package of its own because a view's outcomes are composition's,
// and composition imports setup (docs/adr/0025-sealed-terms.md).
package founding

import (
	"encoding/hex"
	"sort"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// Inputs is what founding reads (setup-contract-15, setup-contract-16): the sealed set's bytes, the
// openings its founder holds, and acceptances.json's bytes.
type Inputs struct {
	Set         []byte
	Openings    setup.Openings
	Acceptances []byte
}

// The outcomes of a view's crossing (setup-contract-14).
const (
	Crosses = "crosses"
	Asked   = "asked"
	Blocked = "blocked"
)

// Found runs every row of founding's check table over the inputs at the founding time at, and
// returns the founded Accord or a *setup.Error with the first failing row's code (section 3,
// setup-contract-8). Rows 1 to 17 are setup.Found's. Then the acceptance list is read whole
// (not-canonical, invalid-shape), and rows 18 to 21 run for each member in actor-reference order, all
// of one member's before the next's. Row 21 runs only when every member's seal was opened, since a
// view reads every member's terms; the Accord says whether its terms were checked
// (setup.Accord.TermsChecked).
func Found(in Inputs, at string) (*setup.Accord, error) {
	a, err := setup.Found(in.Set, in.Openings, at)
	if err != nil {
		return nil, err
	}
	byFrom, code := list(in.Acceptances, a)
	if code != "" {
		return nil, &setup.Error{Code: code}
	}
	for _, m := range a.Members() {
		// Row 18: an acceptance from the member.
		e, ok := byFrom[m]
		if !ok {
			return nil, &setup.Error{Code: setup.AcceptanceMissing}
		}
		if code := rows(a, e); code != "" {
			return nil, &setup.Error{Code: code}
		}
	}
	return a, nil
}

// An acceptance is one entry of the list as read: its members and its bytes.
type acceptance struct {
	o     shape.Object
	bytes []byte
}

// list reads acceptances.json before row 18 (setup-contract-15): bytes the canonical form refuses are
// not-canonical; a list of objects of exactly accord, view, from and sig, accord and view typed
// hashes and from a string, in strict from order, each from a member, or it is invalid-shape. The sig
// form is row 19's.
func list(b []byte, a *setup.Accord) (map[string]acceptance, setup.Code) {
	v, _, err := shape.Decode(b)
	if err != nil {
		return nil, setup.NotCanonical
	}
	l, ok := v.([]any)
	if !ok {
		return nil, setup.InvalidShape
	}
	out := map[string]acceptance{}
	prev := ""
	for i, x := range l {
		e, ok := entry(x)
		if !ok {
			return nil, setup.InvalidShape
		}
		from := e.o["from"].(string)
		if (i > 0 && !canonical.Less(prev, from)) || !a.Member(from) {
			return nil, setup.InvalidShape
		}
		prev = from
		out[from] = e
	}
	return out, ""
}

// entry reads one acceptance's members: exactly accord, view, from and sig, accord and view typed
// hashes, from a string.
func entry(x any) (acceptance, bool) {
	o, ok := x.(shape.Object)
	if !ok || !shape.Only(o, "accord", "view", "from", "sig") {
		return acceptance{}, false
	}
	for _, n := range []string{"accord", "view"} {
		if s, ok := o[n].(string); !ok || !shape.Hash(s) {
			return acceptance{}, false
		}
	}
	if _, ok := o["from"].(string); !ok {
		return acceptance{}, false
	}
	b := shape.Encode(o)
	return acceptance{o: o, bytes: b}, b != nil
}

// rows runs rows 19 to 21 over one member's acceptance: its signature by its from under the
// acceptance's type URI, its accord the Accord's id, and, when every seal was opened, its view the
// typed hash of the member's view.
func rows(a *setup.Accord, e acceptance) setup.Code {
	from := e.o["from"].(string)
	if !signing.Verify(e.bytes, signing.TypeAcceptance, from) {
		return setup.AcceptanceSignature
	}
	if e.o["accord"] != any(a.ID()) {
		return setup.AcceptanceAccord
	}
	if !a.TermsChecked() {
		return ""
	}
	view, err := View(a, from)
	if err != nil {
		return setup.AcceptanceView
	}
	if h, err := canonical.TypedHash(view); err != nil || e.o["view"] != any(h) {
		return setup.AcceptanceView
	}
	return ""
}

// Accepted checks one acceptance as the window takes it, before the list is whole
// (setup-contract-15): its bytes accepted by the canonical form (not-canonical), its members as the
// list's entries and its from a member (invalid-shape), then rows 19 to 21. It returns the from.
func Accepted(a *setup.Accord, b []byte) (string, error) {
	v, _, err := shape.Decode(b)
	if err != nil {
		return "", &setup.Error{Code: setup.NotCanonical}
	}
	e, ok := entry(v)
	if !ok || !a.Member(e.o["from"].(string)) {
		return "", &setup.Error{Code: setup.InvalidShape}
	}
	if code := rows(a, e); code != "" {
		return "", &setup.Error{Code: code}
	}
	return e.o["from"].(string), nil
}

// Acceptances writes acceptances.json from the members' acceptances, each one's bytes: the list in
// strict from order, in canonical form (setup-contract-15). Each must be accepted by the canonical
// form and have a string from, and no two the same from, or it is a *setup.Error of InvalidShape. It
// checks no signature: Found does.
func Acceptances(list [][]byte) ([]byte, error) {
	type item struct {
		from string
		v    any
	}
	items := make([]item, 0, len(list))
	for _, b := range list {
		v, _, err := shape.Decode(b)
		o, ok := v.(shape.Object)
		if err != nil || !ok {
			return nil, &setup.Error{Code: setup.InvalidShape}
		}
		from, ok := o["from"].(string)
		if !ok {
			return nil, &setup.Error{Code: setup.InvalidShape}
		}
		items = append(items, item{from, o})
	}
	sort.Slice(items, func(i, j int) bool { return canonical.Less(items[i].from, items[j].from) })
	out := make([]any, len(items))
	for i, x := range items {
		if i > 0 && x.from == items[i-1].from {
			return nil, &setup.Error{Code: setup.InvalidShape}
		}
		out[i] = x.v
	}
	b := shape.Encode(out)
	if b == nil {
		return nil, &setup.Error{Code: setup.InvalidShape}
	}
	return b, nil
}

// Accept signs a member's acceptance of a view (setup-contract-15): {accord, view, from, sig}, view
// the typed hash of the view's bytes and from the key's reference, signed under the acceptance's type
// URI. The accord must be a typed hash and the view accepted by the canonical form, or it is a
// *setup.Error of InvalidShape; a signing failure is signing's *Error.
func Accept(accord string, view []byte, key signing.Key) ([]byte, error) {
	h, err := canonical.TypedHash(view)
	if err != nil || !shape.Hash(accord) {
		return nil, &setup.Error{Code: setup.InvalidShape}
	}
	o := shape.Object{"accord": accord, "view": h, "from": key.Reference()}
	b := shape.Encode(o)
	if b == nil {
		return nil, &setup.Error{Code: setup.InvalidShape}
	}
	sig, err := signing.Sign(b, signing.TypeAcceptance, key)
	if err != nil {
		return nil, err
	}
	o["sig"] = sig
	return shape.Encode(o), nil
}

// View builds viewer's view of the Accord, in canonical bytes (setup-contract-14): accord, viewer,
// salt, its opening's salt in hex; crossings, one row per Side composition.Sides gives, with the
// counterpart's roles as its binding gives them and the outcome, crosses, blocked with no reason, or
// asked with the agreed rule's to as role selectors, its timeout and, when any, the judges both sides
// consented to; and answers, one row per composition.Answers. The same openings give the same bytes.
// An actor that is not a member, or an Accord whose seals were not all opened, has no view: a
// *setup.Error of CodeViewer.
func View(a *setup.Accord, viewer string) ([]byte, error) {
	if !a.Member(viewer) || !a.TermsChecked() {
		return nil, &setup.Error{Code: setup.CodeViewer}
	}
	crossings := []any{}
	for _, s := range composition.Sides(a, viewer) {
		row := shape.Object{"counterpart": s.Counterpart, "roles": strs(a.Roles(s.Counterpart)), "dir": s.Dir, "kind": s.Kind}
		switch s.Decision.Verdict {
		case composition.Pass:
			row["outcome"] = Crosses
		case composition.Escalate:
			row["outcome"] = Asked
			to := make([]any, len(s.Decision.Rule.To))
			for i, r := range s.Decision.Rule.To {
				to[i] = "role:" + r
			}
			row["to"], row["timeout"] = to, s.Decision.Rule.Timeout
			if js := s.Decision.Rule.Judges; len(js) > 0 {
				l := make([]any, len(js))
				for i, j := range js {
					l[i] = shape.Object{"name": j.Name, "use": string(j.Use)}
				}
				row["judges"] = l
			}
		default:
			row["outcome"] = Blocked
		}
		crossings = append(crossings, row)
	}
	answers := []any{}
	for _, x := range composition.Answers(a, viewer) {
		answers = append(answers, shape.Object{"asker": x.Asker, "receiver": x.Receiver, "kind": x.Kind, "timeout": x.Timeout})
	}
	b := shape.Encode(shape.Object{"accord": a.ID(), "viewer": viewer, "salt": hex.EncodeToString(a.Salt(viewer)),
		"crossings": crossings, "answers": answers})
	if b == nil {
		return nil, &setup.Error{Code: setup.CodeViewer}
	}
	return b, nil
}

func strs(l []string) []any {
	out := make([]any, len(l))
	for i, s := range l {
		out[i] = s
	}
	return out
}
