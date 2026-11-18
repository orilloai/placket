package founding_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/internal/shape"
	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

const at = "2026-11-18T08:00:00Z"

var (
	term  = ptesting.Term
	maybe = ptesting.Maybe
)

func l(s ...string) []string { return s }

// keys are the vectors' keys of names.
func keys(t testing.TB, names ...string) []signing.Key {
	var out []signing.Key
	for _, n := range names {
		out = append(out, ptesting.Key(t, n))
	}
	return out
}

func ref(t testing.TB, name string) string { return ptesting.Key(t, name).Reference() }

// twoParty is an Accord of a proposer that gives the applier a note freely and a proposal asked
// about by the applier, and an applier that gets both.
func twoParty(t testing.TB) []ptesting.Party {
	return []ptesting.Party{
		{Name: "proposer", Roles: l("proposer"), Terms: []shape.Object{
			term("want", "give", l("applier"), l("note")),
			maybe("give", l("applier"), l("proposal"), l("applier"), 60),
		}},
		{Name: "applier", Roles: l("applier"), Terms: []shape.Object{
			term("want", "get", l("proposer"), l("note")),
			maybe("get", l("proposer"), l("proposal"), l("applier"), 60),
		}},
	}
}

// sealed seals the parties' set, every member accepting.
func sealed(t testing.TB, parties []ptesting.Party, judges ...string) founding.Inputs {
	t.Helper()
	var names []string
	for _, p := range parties {
		names = append(names, p.Name)
	}
	return ptesting.Sealed(t, ptesting.ClearSet(t, parties, judges...), at, keys(t, names...)...)
}

func codeOf(t testing.TB, err error) setup.Code {
	t.Helper()
	if err == nil {
		return ""
	}
	var se *setup.Error
	if !errors.As(err, &se) {
		t.Fatalf("not a founding refusal: %v", err)
	}
	if err.Error() != "setup: "+string(se.Code) {
		t.Fatalf("error text %q", err.Error())
	}
	return se.Code
}

// TestASealedSetFounds founds a sealed set with every opening and every acceptance: its id is the
// typed hash of the sealed set, its terms are checked, and the same inputs found the same Accord at
// another time (setup-contract-5, setup-contract-15).
func TestASealedSetFounds(t *testing.T) {
	t.Parallel()
	in := sealed(t, twoParty(t))
	a, err := founding.Found(in, at)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != ptesting.ID(t, in) || !a.TermsChecked() {
		t.Fatalf("id %s, terms checked %v", a.ID(), a.TermsChecked())
	}
	b, err := founding.Found(in, "2026-12-01T00:00:00Z")
	if err != nil || b.ID() != a.ID() {
		t.Fatalf("founded later: %v", err)
	}
}

// TestWithoutTheOpenings founds the same inputs without the openings: every row but 4a, 5 to 8 and
// 21 runs, so the id is the same, the terms are not checked, an acceptance signed by another key is
// still refused, and an acceptance of a view that is not the member's is not seen
// (setup-contract-16).
func TestWithoutTheOpenings(t *testing.T) {
	t.Parallel()
	in := sealed(t, twoParty(t))
	none := in
	none.Openings = setup.Openings{}
	a, err := founding.Found(none, at)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID() != ptesting.ID(t, in) || a.TermsChecked() {
		t.Fatalf("id %s, terms checked %v", a.ID(), a.TermsChecked())
	}
	if _, err := founding.View(a, ref(t, "proposer")); codeOf(t, err) != setup.CodeViewer {
		t.Fatalf("a view without the openings: %v", err)
	}
	falseView := withAcceptance(t, in, "proposer", func(o shape.Object) { o["view"] = canonical.HashPrefix + strings.Repeat("0", 64) }, true)
	falseView.Openings = setup.Openings{}
	if _, err := founding.Found(falseView, at); err != nil {
		t.Fatalf("a false view, without the openings: %v", err)
	}
	otherKey := withAcceptance(t, in, "proposer", func(o shape.Object) {}, false)
	otherKey.Openings = setup.Openings{}
	if c := codeOf(t, mustFail(founding.Found(otherKey, at))); c != setup.AcceptanceSignature {
		t.Fatalf("signed by another key, without the openings: %s", c)
	}
}

func mustFail(_ *setup.Accord, err error) error { return err }

// withAcceptance returns the inputs with name's acceptance changed by edit and signed again, by its
// own key when own is set and by the stranger's otherwise.
func withAcceptance(t testing.TB, in founding.Inputs, name string, edit func(shape.Object), own bool) founding.Inputs {
	t.Helper()
	v, _, err := shape.Decode(in.Acceptances)
	if err != nil {
		t.Fatal(err)
	}
	list := v.([]any)
	for i, x := range list {
		o := x.(shape.Object)
		if o["from"] != ref(t, name) {
			continue
		}
		delete(o, "sig")
		edit(o)
		k := ptesting.Key(t, name)
		if !own {
			k = ptesting.Key(t, "stranger")
		}
		sig, err := signing.Sign(shape.Encode(o), signing.TypeAcceptance, k)
		if err != nil {
			t.Fatal(err)
		}
		o["sig"] = sig
		list[i] = o
	}
	out := in
	out.Acceptances = shape.Encode(list)
	return out
}

// TestAcceptanceRefusals holds rows 18 to 21 and the reading of the list before them, in order
// (setup-contract-15, setup-contract-8): a missing acceptance, one signed by another key, one of
// another Accord and one of a view other than the member's; a list out of order, with a repeat, with
// a stranger or with an extra member is invalid-shape, and bytes the canonical form refuses are
// not-canonical. With two members failing, the first in actor-reference order is reported.
func TestAcceptanceRefusals(t *testing.T) {
	t.Parallel()
	in := sealed(t, twoParty(t))
	members := []string{ref(t, "proposer"), ref(t, "applier")}
	first, second := "proposer", "applier"
	if canonical.Less(members[1], members[0]) {
		first, second = "applier", "proposer"
	}
	v, _, _ := shape.Decode(in.Acceptances)
	list := v.([]any)
	dropped := func(name string) []byte {
		var out []any
		for _, x := range list {
			if x.(shape.Object)["from"] != ref(t, name) {
				out = append(out, x)
			}
		}
		return shape.Encode(out)
	}
	reversed := shape.Encode([]any{list[1], list[0]})
	repeated := shape.Encode([]any{list[0], list[0], list[1]})
	extra := map[string]any{"note": "x"}
	for k, x := range list[0].(shape.Object) {
		extra[k] = x
	}
	stranger, err := founding.Accept(ptesting.ID(t, in), []byte(`{}`), ptesting.Key(t, "stranger"))
	if err != nil {
		t.Fatal(err)
	}
	sv, _, _ := shape.Decode(stranger)
	withStranger := append(append([]any(nil), list...), sv)
	strangerList, _ := founding.Acceptances(func() [][]byte {
		var out [][]byte
		for _, x := range withStranger {
			out = append(out, shape.Encode(x))
		}
		return out
	}())
	otherAccord := func(o shape.Object) { o["accord"] = canonical.HashPrefix + strings.Repeat("1", 64) }
	otherView := func(o shape.Object) { o["view"] = canonical.HashPrefix + strings.Repeat("2", 64) }
	both := withAcceptance(t, withAcceptance(t, in, first, otherView, true), second, func(shape.Object) {}, false)
	bothRev := withAcceptance(t, withAcceptance(t, in, first, func(shape.Object) {}, false), second, otherView, true)

	for name, c := range map[string]struct {
		acc  []byte
		want setup.Code
	}{
		"every acceptance":         {in.Acceptances, ""},
		"none":                     {[]byte(`[]`), setup.AcceptanceMissing},
		"the second missing":       {dropped(second), setup.AcceptanceMissing},
		"signed by another key":    {withAcceptance(t, in, second, func(shape.Object) {}, false).Acceptances, setup.AcceptanceSignature},
		"another Accord":           {withAcceptance(t, in, second, otherAccord, true).Acceptances, setup.AcceptanceAccord},
		"another view":             {withAcceptance(t, in, second, otherView, true).Acceptances, setup.AcceptanceView},
		"out of order":             {reversed, setup.InvalidShape},
		"repeated":                 {repeated, setup.InvalidShape},
		"a stranger":               {strangerList, setup.InvalidShape},
		"an extra member":          {shape.Encode([]any{extra, list[1]}), setup.InvalidShape},
		"an object, not a list":    {[]byte(`{}`), setup.InvalidShape},
		"an accord not a hash":     {withAcceptance(t, in, second, func(o shape.Object) { o["accord"] = "x" }, true).Acceptances, setup.InvalidShape},
		"a from not a string":      {shape.Encode([]any{shape.Object{"accord": canonical.HashPrefix + strings.Repeat("1", 64), "view": canonical.HashPrefix + strings.Repeat("1", 64), "from": 7, "sig": "00"}}), setup.InvalidShape},
		"not canonical":            {[]byte(`[{"from":"a","from":"b"}]`), setup.NotCanonical},
		"no bytes":                 {nil, setup.NotCanonical},
		"two failing, view first":  {both.Acceptances, setup.AcceptanceView},
		"two failing, the first's": {bothRev.Acceptances, setup.AcceptanceSignature},
	} {
		got := in
		got.Acceptances = c.acc
		if code := codeOf(t, mustFail(founding.Found(got, at))); code != c.want {
			t.Errorf("%s: %q, want %q", name, code, c.want)
		}
	}
	// A sig of the wrong form is row 19's.
	bad := withAcceptance(t, in, second, func(shape.Object) {}, true)
	bv, _, _ := shape.Decode(bad.Acceptances)
	for _, x := range bv.([]any) {
		if o := x.(shape.Object); o["from"] == ref(t, second) {
			o["sig"] = "XYZ"
		}
	}
	bad.Acceptances = shape.Encode(bv)
	if c := codeOf(t, mustFail(founding.Found(bad, at))); c != setup.AcceptanceSignature {
		t.Fatalf("a sig of the wrong form: %s", c)
	}
	// Rows 1 to 17 come first: a set refused there is refused whatever the acceptances.
	noOpening := in
	noOpening.Openings = setup.Openings{ByActor: map[string][]byte{}}
	noOpening.Acceptances = []byte(`{`)
	if c := codeOf(t, mustFail(founding.Found(noOpening, at))); c != setup.OpeningMissing {
		t.Fatalf("rows 1 to 17 first: %s", c)
	}
}

// viewOf builds name's view in the parties' Accord.
func viewOf(t testing.TB, parties []ptesting.Party, name string, judges ...string) map[string]any {
	t.Helper()
	b := viewBytes(t, parties, name, judges...)
	v, _, err := shape.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return v.(shape.Object)
}

func viewBytes(t testing.TB, parties []ptesting.Party, name string, judges ...string) []byte {
	t.Helper()
	in := sealed(t, parties, judges...)
	a, err := setup.Found(in.Set, in.Openings, at)
	if err != nil {
		t.Fatal(err)
	}
	b, err := founding.View(a, ref(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rows returns a view's crossing rows of kind, each as canonical JSON.
func rows(v map[string]any, kind string) []string {
	var out []string
	for _, x := range v["crossings"].([]any) {
		if o := x.(shape.Object); o["kind"] == kind {
			out = append(out, string(shape.Encode(o)))
		}
	}
	return out
}

// TestAViewIsBuiltFromTheViewersTermsOnly holds setup-contract-14: A refuses to give B a secret and
// B says nothing of it, so B's view has no row for secret and A's has one, blocked, with no reason;
// and B's row for a kind A refuses is byte-identical to its row where A says nothing of it.
func TestAViewIsBuiltFromTheViewersTermsOnly(t *testing.T) {
	t.Parallel()
	parties := func(aTerms ...shape.Object) []ptesting.Party {
		return []ptesting.Party{
			{Name: "proposer", Roles: l("proposer"), Terms: append([]shape.Object{term("want", "give", l("applier"), l("note"))}, aTerms...)},
			{Name: "applier", Roles: l("applier"), Terms: []shape.Object{term("want", "get", l("proposer"), l("note", "k"))}},
		}
	}
	secret := parties(term("no", "give", l("applier"), l("secret")))
	b := viewOf(t, secret, "applier")
	if r := rows(b, "secret"); r != nil {
		t.Fatalf("the applier's view shows secret: %v", r)
	}
	a := viewOf(t, secret, "proposer")
	want := `{"counterpart":"` + ref(t, "applier") + `","dir":"give","kind":"secret","outcome":"blocked","roles":["applier"]}`
	if r := rows(a, "secret"); len(r) != 1 || r[0] != want {
		t.Fatalf("the proposer's rows for secret: %v, want %s", r, want)
	}
	refused := rows(viewOf(t, parties(term("no", "give", l("applier"), l("k"))), "applier"), "k")
	silent := rows(viewOf(t, parties(), "applier"), "k")
	if len(refused) != 1 || len(silent) != 1 || refused[0] != silent[0] {
		t.Fatalf("a refused kind %v, a kind not named %v", refused, silent)
	}
	if !strings.Contains(refused[0], `"outcome":"blocked"`) || strings.Contains(refused[0], "reason") {
		t.Fatalf("the blocked row %s", refused[0])
	}
}

// TestAViewsOutcomes holds the outcomes of setup-contract-14: two maybe rules that differ are blocked
// on both sides (composition-5); two that agree are asked, with to, timeout and the judges both
// consented to (composition-7), and with judges that differ asked with none; a crossing both want
// crosses.
func TestAViewsOutcomes(t *testing.T) {
	t.Parallel()
	pair := func(give, get shape.Object) []ptesting.Party {
		return []ptesting.Party{
			{Name: "proposer", Roles: l("proposer"), Terms: []shape.Object{give}},
			{Name: "applier", Roles: l("applier"), Terms: []shape.Object{get}},
			{Name: "decider", Roles: l("decider"), Terms: []shape.Object{term("want", "give", l("proposer"), l("answer"))}},
		}
	}
	app := ref(t, "applier")
	prop := ref(t, "proposer")
	differ := pair(maybe("give", l("applier"), l("k"), l("decider"), 60), maybe("get", l("proposer"), l("k"), l("decider"), 120))
	for _, name := range []string{"proposer", "applier"} {
		r := rows(viewOf(t, differ, name), "k")
		if len(r) != 1 || !strings.Contains(r[0], `"outcome":"blocked"`) || strings.Contains(r[0], "timeout") {
			t.Fatalf("%s's row for maybe rules that differ: %v", name, r)
		}
	}
	judged := pair(maybe("give", l("applier"), l("k"), l("decider"), 60, "jev", "decide"), maybe("get", l("proposer"), l("k"), l("decider"), 60, "jev", "decide"))
	want := `{"counterpart":"` + app + `","dir":"give","judges":[{"name":"jev","use":"decide"}],"kind":"k","outcome":"asked","roles":["applier"],"timeout":60,"to":["role:decider"]}`
	if r := rows(viewOf(t, judged, "proposer", "jev"), "k"); len(r) != 1 || r[0] != want {
		t.Fatalf("an asked row with judges: %v, want %s", r, want)
	}
	unjudged := pair(maybe("give", l("applier"), l("k"), l("decider"), 60, "jev", "decide"), maybe("get", l("proposer"), l("k"), l("decider"), 60, "jev", "advise"))
	want = `{"counterpart":"` + prop + `","dir":"get","kind":"k","outcome":"asked","roles":["proposer"],"timeout":60,"to":["role:decider"]}`
	if r := rows(viewOf(t, unjudged, "applier", "jev"), "k"); len(r) != 1 || r[0] != want {
		t.Fatalf("an asked row whose judges differ: %v, want %s", r, want)
	}
	crosses := pair(term("want", "give", l("applier"), l("k")), term("need", "get", l("proposer"), l("k")))
	if r := rows(viewOf(t, crosses, "applier"), "k"); len(r) != 1 || !strings.Contains(r[0], `"outcome":"crosses"`) {
		t.Fatalf("a crossing both want: %v", r)
	}
}

// TestAViewIsDeterministic builds one view twice, and from inputs sealed again, and holds the bytes,
// the typed hash and the order of rows and answers (setup-contract-14).
func TestAViewIsDeterministic(t *testing.T) {
	t.Parallel()
	parties := answerers(t)
	one, two := viewBytes(t, parties, "proposer"), viewBytes(t, parties, "proposer")
	h1, _ := canonical.TypedHash(one)
	h2, _ := canonical.TypedHash(two)
	if !bytes.Equal(one, two) || h1 != h2 {
		t.Fatal("one view built twice differs")
	}
	if c, err := canonical.Form(one); err != nil || !bytes.Equal(c, one) {
		t.Fatal("the view is not canonical")
	}
	v := viewOf(t, parties, "proposer")
	var prev []string
	for _, x := range v["crossings"].([]any) {
		o := x.(shape.Object)
		cur := []string{o["counterpart"].(string), o["dir"].(string), o["kind"].(string)}
		if prev != nil && !(canonical.Less(prev[0], cur[0]) || (prev[0] == cur[0] && (prev[1] < cur[1] || (prev[1] == cur[1] && prev[2] < cur[2])))) {
			t.Fatalf("rows out of order: %v then %v", prev, cur)
		}
		prev = cur
	}
}

// answerers is an Accord in which the proposer and the applier each ask the other about k, answered
// by the approver role the lead holds, and the lead is a side of no crossing on k.
func answerers(t testing.TB) []ptesting.Party {
	toward := func(other string) []shape.Object {
		return []shape.Object{
			maybe("give", l(other), l("k"), l("approver"), 300),
			maybe("get", l(other), l("k"), l("approver"), 300),
			term("want", "give", l(other), l("note")),
			term("want", "get", l(other), l("note")),
		}
	}
	return []ptesting.Party{
		{Name: "proposer", Roles: l("proposer"), Terms: toward("applier")},
		{Name: "applier", Roles: l("applier"), Terms: toward("proposer")},
		{Name: "lead", Roles: l("approver"), Terms: []shape.Object{term("want", "give", l("proposer"), l("answer"))}},
	}
}

// TestAnAnswerersViewListsWhatItAnswers holds setup-contract-14's answers: the lead's view lists the
// proposer's and the applier's asked crossings to each other on k, with kind and timeout, and holds
// no term of theirs; the proposer's view, which answers nothing, has none.
func TestAnAnswerersViewListsWhatItAnswers(t *testing.T) {
	t.Parallel()
	parties := answerers(t)
	b := viewBytes(t, parties, "lead")
	v := viewOf(t, parties, "lead")
	p, a := ref(t, "proposer"), ref(t, "applier")
	first, second := p, a
	if canonical.Less(a, p) {
		first, second = a, p
	}
	want := `[{"asker":"` + first + `","kind":"k","receiver":"` + second + `","timeout":300},{"asker":"` + second + `","kind":"k","receiver":"` + first + `","timeout":300}]`
	if got := string(shape.Encode(v["answers"])); got != want {
		t.Fatalf("answers %s, want %s", got, want)
	}
	if rows(v, "k") != nil || bytes.Contains(b, []byte("note")) || bytes.Contains(b, []byte("approver")) {
		t.Fatalf("the lead's view shows the others' terms: %s", b)
	}
	if got := string(shape.Encode(viewOf(t, parties, "proposer")["answers"])); got != `[]` {
		t.Fatalf("the proposer answers %s", got)
	}
}

// TestAViewHoldsNothingOfAnotherParty holds what a party learns (setup-contract-14): the applier's
// view holds no byte of the proposer's salt, opening or signature, and names its own salt; a view
// built from the clear parts without that salt never has the hash of the view the applier accepts.
func TestAViewHoldsNothingOfAnotherParty(t *testing.T) {
	t.Parallel()
	in := sealed(t, twoParty(t))
	a, err := founding.Found(in, at)
	if err != nil {
		t.Fatal(err)
	}
	prop, app := ref(t, "proposer"), ref(t, "applier")
	view, err := founding.View(a, app)
	if err != nil {
		t.Fatal(err)
	}
	ov, _, _ := shape.Decode(in.Openings.ByActor[prop])
	sig := ov.(shape.Object)["declaration"].(shape.Object)["sig"].(string)
	for name, secret := range map[string]string{"salt": hex.EncodeToString(a.Salt(prop)), "sig": sig, "opening": string(in.Openings.ByActor[prop])} {
		if bytes.Contains(view, []byte(secret)) {
			t.Fatalf("the applier's view holds the proposer's %s", name)
		}
	}
	vv, _, _ := shape.Decode(view)
	if vv.(shape.Object)["salt"] != hex.EncodeToString(a.Salt(app)) {
		t.Fatal("the view does not name the viewer's own salt")
	}
	accepted, _ := canonical.TypedHash(view)
	for _, salt := range []string{"", strings.Repeat("0", 64), hex.EncodeToString(ptesting.SaltOf(prop))} {
		o := vv.(shape.Object)
		guess := shape.Object{}
		for k, x := range o {
			guess[k] = x
		}
		guess["salt"] = salt
		if h, _ := canonical.TypedHash(shape.Encode(guess)); h == accepted {
			t.Fatalf("a view guessed with salt %q has the accepted hash", salt)
		}
	}
}

// TestAcceptAndAccepted signs an acceptance of a member's view and checks it as the window does
// (setup-contract-15): it names the Accord, the view's typed hash and the signer, verifies under
// the acceptance's type URI, and Accepted returns its from; another member's view, another key and
// a stranger are refused with their rows' codes.
func TestAcceptAndAccepted(t *testing.T) {
	t.Parallel()
	in := sealed(t, twoParty(t))
	a, err := setup.Found(in.Set, in.Openings, at)
	if err != nil {
		t.Fatal(err)
	}
	app := ref(t, "applier")
	view, _ := founding.View(a, app)
	acc, err := founding.Accept(a.ID(), view, ptesting.Key(t, "applier"))
	if err != nil {
		t.Fatal(err)
	}
	h, _ := canonical.TypedHash(view)
	v, _, _ := shape.Decode(acc)
	o := v.(shape.Object)
	if o["accord"] != a.ID() || o["view"] != h || o["from"] != app || !signing.Verify(acc, signing.TypeAcceptance, app) {
		t.Fatalf("the acceptance %s", acc)
	}
	if from, err := founding.Accepted(a, acc); err != nil || from != app {
		t.Fatalf("Accepted: %s, %v", from, err)
	}
	other, _ := founding.View(a, ref(t, "proposer"))
	wrongView, _ := founding.Accept(a.ID(), other, ptesting.Key(t, "applier"))
	strangers, _ := founding.Accept(a.ID(), view, ptesting.Key(t, "stranger"))
	wrongAccord, _ := founding.Accept(canonical.HashPrefix+strings.Repeat("3", 64), view, ptesting.Key(t, "applier"))
	forged := bytes.Replace(acc, []byte(o["sig"].(string)), []byte(strings.Repeat("0", 128)), 1)
	for name, c := range map[string]struct {
		b    []byte
		want setup.Code
	}{
		"another member's view": {wrongView, setup.AcceptanceView},
		"a stranger":            {strangers, setup.InvalidShape},
		"another Accord":        {wrongAccord, setup.AcceptanceAccord},
		"a forged signature":    {forged, setup.AcceptanceSignature},
		"not canonical":         {[]byte(`{"a":1,"a":2}`), setup.NotCanonical},
		"a list":                {[]byte(`[]`), setup.InvalidShape},
	} {
		if _, err := founding.Accepted(a, c.b); codeOf(t, err) != c.want {
			t.Errorf("%s: %v, want %s", name, err, c.want)
		}
	}
	if _, err := founding.Accept(a.ID(), []byte(`{`), ptesting.Key(t, "applier")); codeOf(t, err) != setup.InvalidShape {
		t.Fatalf("a view the canonical form refuses: %v", err)
	}
	// Changing the Accord id or the view changes the acceptance (inputs-as-parameters).
	again, _ := founding.Accept(a.ID(), view, ptesting.Key(t, "applier"))
	if !bytes.Equal(again, acc) || bytes.Equal(wrongAccord, acc) || bytes.Equal(wrongView, acc) {
		t.Fatal("an acceptance does not follow its inputs")
	}
}

// TestAcceptancesAreInFromOrder writes the list in strict from order whatever order it is given in,
// and refuses two of one from and one with no from (setup-contract-15).
func TestAcceptancesAreInFromOrder(t *testing.T) {
	t.Parallel()
	a := []byte(`{"from":"b","x":1}`)
	b := []byte(`{"from":"a"}`)
	got, err := founding.Acceptances([][]byte{a, b})
	if err != nil || string(got) != `[{"from":"a"},{"from":"b","x":1}]` {
		t.Fatalf("%s, %v", got, err)
	}
	for name, list := range map[string][][]byte{"a repeat": {a, a}, "no from": {[]byte(`{}`)}, "not JSON": {[]byte(`{`)}} {
		if _, err := founding.Acceptances(list); codeOf(t, err) != setup.InvalidShape {
			t.Errorf("%s: %v", name, err)
		}
	}
	if got, err := founding.Acceptances(nil); err != nil || string(got) != `[]` {
		t.Fatalf("none: %s, %v", got, err)
	}
}

// TestViewsResultsAreCopies changes a view's bytes and builds it again (results-are-copies).
func TestViewsResultsAreCopies(t *testing.T) {
	t.Parallel()
	in := sealed(t, twoParty(t))
	a, _ := setup.Found(in.Set, in.Openings, at)
	v1, _ := founding.View(a, ref(t, "applier"))
	keep := append([]byte(nil), v1...)
	for i := range v1 {
		v1[i] = 'x'
	}
	if v2, _ := founding.View(a, ref(t, "applier")); !bytes.Equal(v2, keep) {
		t.Fatal("a view shares memory with the Accord")
	}
	if _, err := founding.View(a, ref(t, "stranger")); codeOf(t, err) != setup.CodeViewer {
		t.Fatal("a stranger has a view")
	}
	if _, err := founding.View(nil, ref(t, "applier")); codeOf(t, err) != setup.CodeViewer {
		t.Fatal("a nil Accord has a view")
	}
}

// FuzzFound founds the two-party inputs with any acceptance bytes. It never panics; a refusal is a
// founding code, and a founded Accord is the set's.
func FuzzFound(f *testing.F) {
	in := sealed(f, twoParty(f))
	f.Add(in.Acceptances)
	f.Add([]byte(`[]`))
	f.Add([]byte(`[{"accord":"x"}]`))
	f.Fuzz(func(t *testing.T, acc []byte) {
		got := in
		got.Acceptances = acc
		a, err := founding.Found(got, at)
		if err != nil {
			var se *setup.Error
			if !errors.As(err, &se) || a != nil {
				t.Fatalf("got %v, %v", a, err)
			}
			return
		}
		if a.ID() != ptesting.ID(t, in) {
			t.Fatal("founded another Accord")
		}
	})
}

// FuzzAccepted checks any bytes as an acceptance, and builds any actor's view. Neither panics.
func FuzzAccepted(f *testing.F) {
	in := sealed(f, twoParty(f))
	a, err := setup.Found(in.Set, in.Openings, at)
	if err != nil {
		f.Fatal(err)
	}
	v, _, _ := shape.Decode(in.Acceptances)
	for _, x := range v.([]any) {
		f.Add(shape.Encode(x), x.(shape.Object)["from"].(string))
	}
	f.Add([]byte(`{}`), "")
	f.Fuzz(func(t *testing.T, b []byte, actor string) {
		from, err := founding.Accepted(a, b)
		if err == nil && !a.Member(from) {
			t.Fatalf("accepted from %q", from)
		}
		view, err := founding.View(a, actor)
		if (err == nil) != a.Member(actor) {
			t.Fatalf("a view of %q: %v", actor, err)
		}
		if err == nil && !bytes.Contains(view, []byte(actor)) {
			t.Fatalf("the view of %q does not name it", actor)
		}
	})
}
