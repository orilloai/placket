package mediator

import (
	"fmt"

	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/internal/crossing"
	"github.com/orilloai/placket/core/setup"
)

// A Judging is what a give would show the judges its agreed rule lists, as Judges reads it before
// the give (composition-7, docs/adr/0023-judges.md): the part's kind; the form of its body (text,
// raw or data); in Body the body as Question reads it, a text body's bytes, a raw body's decoded
// bytes or a data body's canonical bytes; and in Judges the agreed rule's judges, in its order. It
// holds a body, so it prints as a fixed text under every format verb (secrets-never-print).
type Judging struct {
	Kind, Form string
	Body       []byte
	Judges     []setup.Judge
}

// Format prints the Judging as a fixed text, whatever the verb.
func (Judging) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("mediator.Judging")) }

// GoString prints the Judging as a fixed text.
func (Judging) GoString() string { return "mediator.Judging" }

// String prints the Judging as a fixed text.
func (Judging) String() string { return "mediator.Judging" }

// Judges reads which judges the give of message under envelope from caller would reach, and writes
// nothing (composition-7, docs/adr/0023-judges.md). It runs every row Give runs before composition,
// in Give's order: membership, a closed Accord (close-3), and the door's rows (section 4); a give
// one of them rejects reaches no judge, and Give returns its rejection. It then decides the give with
// no judgment. It returns true only when that decision is escalate and the agreed rule lists judges:
// composition alone says which judges both sides consented to (one-decision-module). What it returns
// is a copy (results-are-copies).
func (m *Mediator) Judges(caller string, message, envelope []byte) (Judging, bool) {
	if !m.founded() {
		return Judging{}, false
	}
	if m.closed != nil && m.accord.Member(caller) {
		return Judging{}, false
	}
	g, code := m.admit(caller, message, envelope)
	if code != "" {
		return Judging{}, false
	}
	d := composition.Decide(m.accord, composition.Crossing{From: caller, To: g.to, Kind: g.kind})
	if d.Verdict != composition.Escalate || len(d.Rule.Judges) == 0 {
		return Judging{}, false
	}
	j := Judging{Kind: g.kind, Judges: append([]setup.Judge(nil), d.Rule.Judges...)}
	for _, form := range crossing.Forms {
		if v, has := g.delivery[form]; has {
			j.Form = form
			j.Body, _ = crossing.Body(form, v)
		}
	}
	return j, true
}
