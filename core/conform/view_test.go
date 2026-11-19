package conform

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/founding"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/internal/testing/seeds"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
)

// viewed is the worked journal's evidence as a member gives it, with the member's own view as the
// window built it and no opening (verify-1 rows 0a and 4a).
func (w worked) viewed(t testing.TB, member string) Evidence {
	t.Helper()
	a, err := founding.Found(w.set, "2026-11-12T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	v, err := founding.View(a, ptesting.Key(t, member).Reference())
	if err != nil {
		t.Fatal(err)
	}
	e := w.evidence()
	e.Openings, e.View = setup.Openings{}, v
	return e
}

// lines are a report's viewed receipts, each its index, row and whether it matches, for comparing.
func lines(r Report) string {
	var out []string
	for _, v := range r.Viewed {
		m := "no"
		if v.Matches {
			m = "yes"
		}
		out = append(out, strings.Join([]string{string(rune('0' + v.Index)), v.Counterpart[:12], v.Dir, v.Kind, v.Outcome, m}, " "))
	}
	return strings.Join(out, "; ")
}

// TestAViewReadsTheViewersReceipts is verify-2's view rows over the worked journal, a note, an ask,
// the decider's accept and its release, with no opening: the applier's view reads all four against
// its rows of notes and proposals from the proposer; the proposer's reads them too, the accept's
// answer from the decider crossing as its row says; the decider's reads none, since its accept
// closes a question it neither asked nor received. Without a view nothing is read. A pass of a kind
// no row of the applier's names is view-conformance at its index, and a stranger's pass, which fails
// party and the view at once, is party, the earlier row (check-order); and a note asked about, and
// its closing, each fail the view.
func TestAViewReadsTheViewersReceipts(t *testing.T) {
	t.Parallel()
	w := journal(t)
	p, ap, d := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference(), ptesting.Key(t, "decider").Reference()
	r, err := Check(w.viewed(t, "applier"))
	if err != nil || r.Viewer != ap || r.Terms {
		t.Fatalf("the applier's view: %+v, %v", r, err)
	}
	q := p[:12]
	if got, want := lines(r), "0 "+q+" get note crosses yes; 1 "+q+" get proposal asked yes; 2 "+q+" get proposal asked yes; 3 "+q+" get proposal asked yes"; got != want {
		t.Fatalf("the applier's view read %q, want %q", got, want)
	}
	r, err = Check(w.viewed(t, "proposer"))
	a := ap[:12]
	if got, want := lines(r), "0 "+a+" give note crosses yes; 1 "+a+" give proposal asked yes; 2 "+a+" give proposal asked yes; 3 "+a+" give proposal asked yes"; err != nil || r.Viewer != p || got != want {
		t.Fatalf("the proposer's view read %q, %v; want %q", got, err, want)
	}
	r, err = Check(w.viewed(t, "decider"))
	if err != nil || r.Viewer != d || r.Viewed == nil || len(r.Viewed) != 0 {
		t.Fatalf("the decider's view: %+v, %v", r, err)
	}
	e := w.evidence()
	e.Openings = setup.Openings{}
	if r, err := Check(e); err != nil || r.Viewer != "" || r.Viewed != nil {
		t.Fatalf("no view: %+v, %v", r, err)
	}

	st, err := receipts.Decode(w.list[0])
	if err != nil {
		t.Fatal(err)
	}
	forged := func(from string) [][]byte {
		s := newSigner(t)
		s.add(t, receipts.Subject{Name: "tests", Digest: strings.Repeat("cd", 32)}, receipts.Predicate{Accord: w.accord,
			Card: st.Predicate.Card, Kind: receipts.KindCrossing, From: from, To: ap, At: "2026-11-12T09:00:00Z",
			Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: st.Predicate.Envelope.Hash},
			Verdict:  "pass", Reason: "ok", By: receipts.By{Decider: receipts.ByTerms}})
		return s.list
	}
	e = w.viewed(t, "applier")
	e.Receipts = forged(p)
	r, err = Check(e)
	if !isCode(err, CodeViewConformance, 0) || lines(r) != "0 "+q+" get tests  no" {
		t.Fatalf("a pass with no row: %q, %v", lines(r), err)
	}
	e.Receipts = forged(ptesting.Key(t, "stranger").Reference())
	if _, err := Check(e); !isCode(err, CodeParty, 0) {
		t.Fatalf("a stranger's pass with no row: %v", err)
	}

	// A note asked about, which the applier's view shows crossing, and a timeout from the mediator at
	// the ask's own time: the ask does not match, and neither does the closing, since its question's
	// row is not asked and so names no rule.
	sg := newSigner(t)
	env := receipts.Envelope{Sender: p, Seq: 1, Hash: st.Predicate.Envelope.Hash}
	sub := receipts.Subject{Name: "note", Digest: strings.Repeat("cd", 32)}
	sg.add(t, sub, receipts.Predicate{Accord: w.accord, Card: st.Predicate.Card, Kind: receipts.KindCrossing, From: p, To: ap,
		At: "2026-11-12T09:00:00Z", Envelope: env, Verdict: "escalate", Reason: "maybe", By: receipts.By{Decider: receipts.ByTerms}})
	sg.add(t, sub, receipts.Predicate{Accord: w.accord, Card: st.Predicate.Card, Kind: receipts.KindResolution, From: ptesting.Key(t, "mediator").Reference(), To: p,
		At: "2026-11-12T09:00:00Z", Envelope: env, Verdict: "block", Reason: "timeout", By: receipts.By{Decider: receipts.ByMediator, Event: receipts.EventDeadline}})
	e.Receipts = sg.list
	r, err = Check(e)
	if !isCode(err, CodeViewConformance, 0) || lines(r) != "0 "+q+" get note crosses no; 1 "+q+" get note crosses no" {
		t.Fatalf("a note asked about and closed: %q, %v", lines(r), err)
	}
}

// TestAViewNotTheOneAccepted is verify-1's row 0a: the applier's view with one row changed, bytes the
// canonical form refuses, a view with a member besides its five, the proposer's view named as the
// applier's, the applier's named as the proposer's, and an empty view are each acceptance-view at index -1; and so is a changed view beside
// a list that validity refuses, since row 0a comes first (check-order). Its text is the package and
// the code.
func TestAViewNotTheOneAccepted(t *testing.T) {
	t.Parallel()
	w := journal(t)
	good := w.viewed(t, "applier")
	p, ap := ptesting.Key(t, "proposer").Reference(), ptesting.Key(t, "applier").Reference()
	changed := bytes.Replace(good.View, []byte(`"outcome":"crosses"`), []byte(`"outcome":"blocked"`), 1)
	if bytes.Equal(changed, good.View) {
		t.Fatal("no row of the view crosses")
	}
	proposers := w.viewed(t, "proposer").View
	for name, v := range map[string][]byte{
		"one row changed":  changed,
		"not canonical":    append(append([]byte(nil), good.View...), '}'),
		"a member besides": bytes.Replace(good.View, []byte(`{"accord"`), []byte(`{"a":1,"accord"`), 1),
		"another's rows":   bytes.Replace(proposers, []byte(`"viewer":"`+p+`"`), []byte(`"viewer":"`+ap+`"`), 1),
		"empty":            {},
		"another viewer":   bytes.Replace(good.View, []byte(`"viewer":"`+ap+`"`), []byte(`"viewer":"`+p+`"`), 1),
	} {
		e := good
		e.View = v
		r, err := Check(e)
		if !isCode(err, CodeAcceptanceView, -1) || r.Viewed != nil {
			t.Fatalf("%s: %+v, %v", name, r, err)
		}
		if err.Error() != "conform: acceptance-view" {
			t.Fatalf("%s: text %q", name, err)
		}
	}
	e := good
	e.View = changed
	e.Receipts = w.list[1:]
	if _, err := Check(e); !isCode(err, CodeAcceptanceView, -1) {
		t.Fatalf("a changed view and a list cut at its start: %v", err)
	}
	e.View = good.View
	if _, err := Check(e); !isReceiptCode(err, receipts.CodeChain, 0) {
		t.Fatalf("the view and a list cut at its start: %v", err)
	}
}

func isReceiptCode(err error, code receipts.Code, index int) bool {
	var e *receipts.Error
	return errors.As(err, &e) && e.Code == code && e.Index == index
}

// FuzzView checks the worked journal with any view as the applier's. It never panics (fuzz-parsers,
// core-no-panic). Its seeds are the views of the conform-view vectors and the journal's members'.
func FuzzView(f *testing.F) {
	w := journal(f)
	for _, b := range seeds.Inputs(f, "conform-view", "view") {
		f.Add(b)
	}
	for _, m := range []string{"proposer", "applier", "decider"} {
		f.Add(w.viewed(f, m).View)
	}
	e := w.viewed(f, "applier")
	f.Fuzz(func(t *testing.T, view []byte) {
		e := e
		e.View = view
		_, _ = Check(e)
	})
}
