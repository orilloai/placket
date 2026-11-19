package conform

import (
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/internal/shape"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

// This file is a member's check of its own receipts against the view it signed: verify-1's rows 0a
// and 4a, which verify-2 describes (docs/adr/0026-verify-against-a-view.md). It reads the view's
// rows and never the terms: it compares no stance, matches no term and compares no two terms'
// judges (one-decision-module).

// The view codes (verify-1 rows 0a and 4a). A view that is not the one its viewer accepted is the
// code founding's row 21 returns for the same fact.
const (
	CodeAcceptanceView  Code = Code(setup.AcceptanceView)
	CodeViewConformance Code = "view-conformance"
)

// Viewed is one receipt of the viewer's read against its view (verify-2): its index, the row it was
// read against, its counterpart, its direction from the viewer, its kind and its outcome, with
// Outcome empty when the view has no row for it, and whether it matches.
type Viewed struct {
	Index                           int
	Counterpart, Dir, Kind, Outcome string
	Matches                         bool
}

// A viewRow is one crossing row of a view as row 4a reads it: its outcome, and for an asked row its
// rule's roles, as labels, its timeout, and the judges it lists with use decide.
type viewRow struct {
	outcome string
	to      []string
	timeout int64
	decide  []string
}

// A view is a view row 0a has read: its viewer and its crossing rows, by counterpart, direction and
// kind.
type view struct {
	viewer string
	rows   map[[3]string]viewRow
}

// readView is row 0a: the bytes are accepted by the canonical form and have the closed shape of
// view.json, the view's accord is the Accord id, its viewer is a member, and its typed hash is the
// view of the viewer's acceptance in acceptances (setup-contract-14, setup-contract-15). Any other
// view is none.
func readView(b []byte, a *setup.Accord, accord string, acceptances []byte) (view, bool) {
	h, err := canonical.TypedHash(b)
	if err != nil {
		return view{}, false
	}
	v, _, err := shape.Decode(b)
	o, ok := v.(shape.Object)
	if err != nil || !ok || !shape.Only(o, "accord", "viewer", "salt", "crossings", "answers") {
		return view{}, false
	}
	viewer, _ := o["viewer"].(string)
	salt, _ := o["salt"].(string)
	if o["accord"] != any(accord) || !a.Member(viewer) || len(salt) != 64 || !shape.LowerHex(salt) || acceptedView(acceptances, viewer) != h {
		return view{}, false
	}
	if _, ok := o["answers"].([]any); !ok {
		return view{}, false
	}
	crossings, ok := o["crossings"].([]any)
	if !ok {
		return view{}, false
	}
	out := view{viewer: viewer, rows: map[[3]string]viewRow{}}
	for _, x := range crossings {
		key, r, ok := crossingRow(x)
		if _, twice := out.rows[key]; !ok || twice {
			return view{}, false
		}
		out.rows[key] = r
	}
	return out, true
}

// crossingRow reads one row of a view's crossings: exactly counterpart, roles, dir, kind and outcome,
// and for an asked row to, timeout and judges when it has any.
func crossingRow(x any) ([3]string, viewRow, bool) {
	o, ok := x.(shape.Object)
	if !ok {
		return [3]string{}, viewRow{}, false
	}
	c, _ := o["counterpart"].(string)
	dir, _ := o["dir"].(string)
	kind, _ := o["kind"].(string)
	r := viewRow{}
	r.outcome, _ = o["outcome"].(string)
	if _, ok := o["roles"].([]any); !ok || c == "" || (dir != setup.DirGive && dir != setup.DirGet) || !shape.Label(kind) {
		return [3]string{}, viewRow{}, false
	}
	key := [3]string{c, dir, kind}
	switch r.outcome {
	case founding.Crosses, founding.Blocked:
		return key, r, shape.Only(o, "counterpart", "roles", "dir", "kind", "outcome")
	case founding.Asked:
	default:
		return [3]string{}, viewRow{}, false
	}
	if !shape.Only(o, "counterpart", "roles", "dir", "kind", "outcome", "to", "timeout", "", "judges") {
		return [3]string{}, viewRow{}, false
	}
	to, ok := o["to"].([]any)
	if !ok || len(to) == 0 {
		return [3]string{}, viewRow{}, false
	}
	for _, s := range to {
		sel, _ := s.(string)
		label, ok := shape.Selector(sel)
		if !ok {
			return [3]string{}, viewRow{}, false
		}
		r.to = append(r.to, label)
	}
	t, ok := shape.Int(o["timeout"])
	if !ok {
		return [3]string{}, viewRow{}, false
	}
	r.timeout = t
	if js, has := o["judges"]; has {
		l, ok := js.([]any)
		if !ok || len(l) == 0 {
			return [3]string{}, viewRow{}, false
		}
		for _, j := range l {
			jo, ok := j.(shape.Object)
			name, _ := jo["name"].(string)
			if !ok || !shape.Only(jo, "name", "use") || !shape.Label(name) {
				return [3]string{}, viewRow{}, false
			}
			if jo["use"] == any(string(setup.Decide)) {
				r.decide = append(r.decide, name)
			}
		}
	}
	return key, r, true
}

// acceptedView is the view of the viewer's acceptance in acceptances.json, which founding has read,
// or empty.
func acceptedView(acceptances []byte, viewer string) string {
	v, _, err := shape.Decode(acceptances)
	l, _ := v.([]any)
	if err != nil {
		return ""
	}
	for _, x := range l {
		o, _ := x.(shape.Object)
		if o["from"] == any(viewer) {
			h, _ := o["view"].(string)
			return h
		}
	}
	return ""
}

// row is the view's row of a crossing from from to to of kind, as the viewer sees it: the give row
// toward to when the viewer is from, the get row from from when the viewer is to, and for a part the
// viewer sends to itself the give row, or the get row when there is no give row. ok is whether the
// viewer is a side of the crossing.
func (v view) row(from, to, kind string) (Viewed, viewRow, bool) {
	var keys [][3]string
	if from == v.viewer {
		keys = append(keys, [3]string{to, setup.DirGive, kind})
	}
	if to == v.viewer {
		keys = append(keys, [3]string{from, setup.DirGet, kind})
	}
	if len(keys) == 0 {
		return Viewed{}, viewRow{}, false
	}
	for _, k := range keys {
		if r, has := v.rows[k]; has {
			return Viewed{Counterpart: k[0], Dir: k[1], Kind: kind, Outcome: r.outcome}, r, true
		}
	}
	return Viewed{Counterpart: keys[0][0], Dir: keys[0][1], Kind: kind}, viewRow{}, true
}

// A viewQuestion is what row 4a keeps of a question of the viewer's: the row of its ask, and the
// index of its closing.
type viewQuestion struct {
	seen    Viewed
	row     viewRow
	at      string
	closing int
}

// viewed is row 4a for the receipt at index i, when it is one of the viewer's: a give's receipt whose
// sender or receiver is the viewer, or the closing or release of a question the viewer asked or
// received (verify-2). It records the receipt, and reports whether it was read and whether it
// matches.
func (c *checker) viewed(i int, s receipts.Statement, place receipts.Place) (read, matches bool) {
	v, p := c.view, s.Predicate
	var seen Viewed
	switch place.Role {
	case receipts.RoleGive, receipts.RoleQuestion:
		var r viewRow
		var side bool
		seen, r, side = v.row(p.From, p.To, s.Subject.Name)
		if !side {
			return false, false
		}
		seen.Matches = given(r, p)
		if place.Role == receipts.RoleQuestion {
			c.viewQuestions[i] = &viewQuestion{seen: seen, row: r, at: p.At, closing: -1}
		}
	case receipts.RoleClosing, receipts.RoleRelease:
		q, ok := c.viewQuestions[place.Question]
		if !ok {
			return false, false
		}
		seen = q.seen
		var formed *Viewed
		if place.Role == receipts.RoleClosing {
			q.closing = i
			seen.Matches, formed = c.closing(q, s)
		} else {
			seen.Matches, formed = c.release(q, s)
		}
		if formed != nil && !formed.Matches {
			seen = *formed
		}
	default:
		return false, false
	}
	seen.Index = i
	c.r.Viewed = append(c.r.Viewed, seen)
	return true, seen.Matches
}

// given reads a give's receipt against its row (verify-2): crosses is a pass ok by the terms; blocked
// a block by the terms, no-match or maybe; asked an escalate maybe by the terms, or a pass ok or a
// block judge:<name> by a judge the row lists to decide; and no row a block no-match by the terms.
func given(r viewRow, p receipts.Predicate) bool {
	terms := p.By.Decider == receipts.ByTerms
	switch r.outcome {
	case founding.Crosses:
		return terms && p.Verdict == string(composition.Pass) && p.Reason == composition.ReasonOK
	case founding.Blocked:
		return terms && p.Verdict == string(composition.Block) && (p.Reason == composition.ReasonNoMatch || p.Reason == composition.ReasonMaybe)
	case founding.Asked:
		if terms {
			return p.Verdict == string(composition.Escalate) && p.Reason == composition.ReasonMaybe
		}
		if p.By.Decider != receipts.ByJudge || !contains(r.decide, p.By.Judge) {
			return false
		}
		return (p.Verdict == string(composition.Pass) && p.Reason == composition.ReasonOK) ||
			(p.Verdict == string(composition.Block) && p.Reason == composition.ReasonJudge(p.By.Judge))
	}
	return terms && p.Verdict == string(composition.Block) && p.Reason == composition.ReasonNoMatch
}

// closing reads a question's closing against the asked row of its question, as verify-1's row 3
// reads it with the row for the agreed rule: the answerers are the members holding a role the row's
// to names, and the deadline the ask's at plus the row's timeout (maybe-1). An accept's or a
// change's formed part is then read against the row of the formed crossing when the viewer is a side
// of it, and returned.
func (c *checker) closing(q *viewQuestion, s receipts.Statement) (bool, *Viewed) {
	if q.row.outcome != founding.Asked {
		return false, nil
	}
	p := s.Predicate
	deadline := shape.AddSeconds(q.at, q.row.timeout)
	answerers := c.answerers(q.row.to)
	first := c.r.Mediator
	if len(answerers) > 0 {
		first = answerers[0]
	}
	switch p.Reason {
	case receipts.ReasonTimeout:
		return p.At == deadline && p.From == first, nil
	case receipts.ReasonUndeliverable, receipts.ReasonClosed:
		return p.At < deadline && p.From == first, nil
	case receipts.ReasonDeclined, receipts.ReasonCancelled:
		return p.At < deadline && len(answerers) > 0 && p.From == first, nil
	}
	if p.At >= deadline || !contains(answerers, p.From) {
		return false, nil
	}
	return c.formed(s)
}

// release reads a question's release at its closing's at: after an accept a pass ok, since the asked
// part decided again with no judgment is asked again (maybe-10); after a change its version, a formed
// part.
func (c *checker) release(q *viewQuestion, s receipts.Statement) (bool, *Viewed) {
	p := s.Predicate
	if q.closing < 0 || p.At != c.statement(q.closing).Predicate.At {
		return false, nil
	}
	if p.Kind == receipts.KindCrossing {
		return p.Verdict == string(composition.Pass) && p.Reason == composition.ReasonOK, nil
	}
	return c.formed(s)
}

// formed reads a part the mediator formed, as composition-5 decides it, against the row of the
// formed crossing when the viewer is one of its sides: crosses is a pass ok; blocked, and no row, a
// block, no-match or maybe; and asked a block maybe. When the viewer is no side of it, its verdict is
// not read, and it matches.
func (c *checker) formed(s receipts.Statement) (bool, *Viewed) {
	p := s.Predicate
	seen, r, side := c.view.row(p.From, p.To, s.Subject.Name)
	if !side {
		return true, nil
	}
	block := p.Verdict == string(composition.Block)
	switch r.outcome {
	case founding.Crosses:
		seen.Matches = p.Verdict == string(composition.Pass) && p.Reason == composition.ReasonOK
	case founding.Asked:
		seen.Matches = block && p.Reason == composition.ReasonMaybe
	default:
		seen.Matches = block && (p.Reason == composition.ReasonNoMatch || p.Reason == composition.ReasonMaybe)
	}
	return seen.Matches, &seen
}
