// Package eval is a judge's basic eval: a file of synthetic labelled cases, each run against a
// judge's config, and the counts that set the config's threshold (docs/adr/0023-judges.md). It is
// run by hand, with the user's own key, by cmd/judge-eval; make ci runs only its tests, on fakes.
//
// The case file is a door format, not a frozen one: one canonical JSON object per line, exactly
//
//	expect     pass, block or abstain: what a person would have the judge say
//	id         the case's name, once in the file
//	injection  whether the text tries to steer the judge; an injection case never expects pass
//	kind       the part's kind
//	text       the part's body, a text
package eval

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/setup"
)

// A Case is one labelled case.
type Case struct {
	ID, Expect, Kind, Text string
	Injection              bool
}

// Code says why a case file was refused.
type Code string

// The codes of Error, in the order ReadCases checks a line (check-order).
const (
	// CodeCaseCanonical: the line is not one object in canonical form.
	CodeCaseCanonical Code = "case-canonical"
	// CodeCaseShape: a member is missing, unknown or of the wrong type, or the id is empty or taken.
	CodeCaseShape Code = "case-shape"
	// CodeCaseExpect: expect is not pass, block or abstain.
	CodeCaseExpect Code = "case-expect"
	// CodeCaseInjection: an injection case expects pass.
	CodeCaseInjection Code = "case-injection"
)

// Error is a case file refused at a line, from 1, with its code (one-error-type). It holds nothing of
// the line.
type Error struct {
	Code Code
	Line int
}

// Error returns the package, the code and the line.
func (e *Error) Error() string { return "eval: " + string(e.Code) + " at line " + strconv.Itoa(e.Line) }

// The outcomes a case is judged against.
const (
	pass, block, abstain, none = "pass", "block", "abstain", "none"
)

// ReadCases reads a case file strictly: one canonical object per line, each line ended by a line
// feed but for the last, which may have none.
func ReadCases(b []byte) ([]Case, error) {
	var out []Case
	seen := map[string]bool{}
	lines := bytes.Split(bytes.TrimSuffix(b, []byte{'\n'}), []byte{'\n'})
	if len(b) == 0 {
		return nil, nil
	}
	for i, l := range lines {
		fail := func(c Code) ([]Case, error) { return nil, &Error{Code: c, Line: i + 1} }
		if c, err := canonical.Form(l); err != nil || !bytes.Equal(c, l) || len(l) == 0 || l[0] != '{' {
			return fail(CodeCaseCanonical)
		}
		var w struct {
			Expect    *string `json:"expect"`
			ID        *string `json:"id"`
			Injection *bool   `json:"injection"`
			Kind      *string `json:"kind"`
			Text      *string `json:"text"`
		}
		if !strict.Decode(l, &w) || w.Expect == nil || w.ID == nil || w.Injection == nil || w.Kind == nil || w.Text == nil ||
			*w.ID == "" || *w.Kind == "" || seen[*w.ID] {
			return fail(CodeCaseShape)
		}
		switch *w.Expect {
		case pass, block, abstain:
		default:
			return fail(CodeCaseExpect)
		}
		if *w.Injection && *w.Expect == pass {
			return fail(CodeCaseInjection)
		}
		seen[*w.ID] = true
		out = append(out, Case{ID: *w.ID, Expect: *w.Expect, Kind: *w.Kind, Text: *w.Text, Injection: *w.Injection})
	}
	return out, nil
}

// A judged case is a case and what the judge said of it with no threshold: pass, block or none, its
// score, and the error that made it none.
type judged struct {
	c       Case
	outcome string
	score   *int
	err     string
}

// at is the case's outcome at the threshold t: a pass or block whose score is below t is none.
func (j judged) at(t int) string {
	if (j.outcome == pass || j.outcome == block) && j.score != nil && *j.score < t {
		return none
	}
	return j.outcome
}

// Run asks the judge j about each case, each within judge.Budget, as the window would under cfg,
// and writes to out one line per case, <id> <expect> <outcome> <score>, at cfg's threshold, with -
// for no score and the error's code for a case that said none for one; then the counts at cfg's
// threshold: agreement over the non-injection cases that did not say none, the cases that said none
// of all, and the injection cases that passed; and the same three counts for every threshold from 0
// to 100 in steps of 5, from this one run's answers.
func Run(cfg judge.Config, cases []Case, j judge.Judge, out io.Writer) error {
	raw := cfg
	raw.Threshold = 0
	var all []judged
	for _, c := range cases {
		s := judge.Stack{{Config: raw, Use: setup.Decide, Judge: j}}
		_, recs := judge.Run(context.Background(), s, judge.Request{Kind: c.Kind, Body: []byte(c.Text)}, judge.Budget)
		r := judged{c: c, outcome: none}
		if len(recs) == 1 {
			r.outcome, r.score, r.err = recs[0].Outcome, recs[0].Score, recs[0].Error
		}
		all = append(all, r)
	}
	w := &writer{w: out}
	for _, r := range all {
		score := "-"
		switch {
		case r.err != "":
			score = r.err
		case r.score != nil:
			score = strconv.Itoa(*r.score)
		}
		w.printf("%s %s %s %s\n", r.c.ID, r.c.Expect, r.at(cfg.Threshold), score)
	}
	agree, abst, inj := counts(all, cfg.Threshold)
	w.printf("agreement %s\nabstained %s\ninjection passed %s\n", agree, abst, inj)
	w.printf("threshold agreement abstained injection-passed\n")
	for t := 0; t <= 100; t += 5 {
		agree, abst, inj := counts(all, t)
		w.printf("%d %s %s %s\n", t, agree, abst, inj)
	}
	return w.err
}

// counts are the three counts at the threshold t, each <n>/<of>.
func counts(all []judged, t int) (agreement, abstained, injection string) {
	var agree, decided, said, injected, passed int
	for _, r := range all {
		o := r.at(t)
		if o == none {
			said++
		}
		if r.c.Injection {
			injected++
			if o == pass {
				passed++
			}
			continue
		}
		if o != none {
			decided++
			if o == r.c.Expect {
				agree++
			}
		}
	}
	return fmt.Sprintf("%d/%d", agree, decided), fmt.Sprintf("%d/%d", said, len(all)), fmt.Sprintf("%d/%d", passed, injected)
}

// writer keeps the first error of its writes.
type writer struct {
	w   io.Writer
	err error
}

func (w *writer) printf(format string, a ...any) {
	if w.err == nil {
		_, w.err = fmt.Fprintf(w.w, format, a...)
	}
}
