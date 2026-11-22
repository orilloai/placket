package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/conform"
	"github.com/orilloai/placket/core/receipts"
)

// accepted plays an accept with release on the worked example's Accord, and returns the Accord and
// the question.
func accepted(t *testing.T) (*accord, string) {
	t.Helper()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", q)...)
	return a, q
}

// verifyAccord runs verify --accord with the Accord's founding set and every member's opening, the
// full check, and extra flags.
func verifyAccord(t *testing.T, a *accord, dir string, extra ...string) result {
	t.Helper()
	args := append([]string{"verify", "--receipts", dir, "--accord", a.id, "--founding", filepath.Join(a.dir, "founding.json")}, a.opens()...)
	return run(t, env(), append(args, extra...)...)
}

// journalIDs are the ids of the journal's receipts, in order.
func journalIDs(t *testing.T, a *accord, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "receipts.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, l := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		st, err := receipts.Decode(l)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, st.ID)
	}
	return out
}

// founded is what r printed up to and including its terms line: the lines of the founding set, which
// the journal's decisions do not change.
func founded(t *testing.T, r result) []string {
	t.Helper()
	l := r.lines()
	for i, s := range l {
		if strings.HasPrefix(s, "terms: ") {
			return l[:i+1]
		}
	}
	t.Fatalf("no terms line: %v", r)
	return nil
}

// verifyOutput is every line verify prints of the Accord's journal: head, the founding set as
// founded reads it, then the lines after the terms line, then the claim of a run that checked every
// decision against the terms and the scope line.
func verifyOutput(head []string, after ...string) []string {
	return verifyOutputClaiming(head, cli.ClaimVerified, after...)
}

// verifyOutputClaiming is verifyOutput with the claim line given.
func verifyOutputClaiming(head []string, claim string, after ...string) []string {
	return append(append(slices.Clone(head), after...), claim, cli.Scope)
}

// TestVerifyByTheAccordID verifies a journal given the Accord id and its founding set alone: it exits
// 0 with the valid, conforms and complete lines, in order before the claim and the scope line; with
// the id one hex digit off it is tool error founding and prints none of them (verify-1, U20, U23).
func TestVerifyByTheAccordID(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	r := verifyAccord(t, a, a.journal())
	l := r.lines()
	n := len(l)
	if r.code != 0 || n < 6 || l[0] != "mediator "+a.refs["mediator"]+" accord "+a.id+" card "+a.card ||
		l[n-5] != fmt.Sprintf(cli.LineValid, 3, a.refs["mediator"]) || l[n-4] != cli.LineConforms || l[n-3] != cli.LineNotComplete ||
		l[n-2] != cli.ClaimVerified || l[n-1] != cli.Scope {
		t.Fatalf("verify --accord: %v", r)
	}
	off := []byte(a.id)
	off[len(off)-1] = map[bool]byte{true: '1', false: '0'}[off[len(off)-1] == '0']
	r = run(t, env(), "verify", "--receipts", a.journal(), "--accord", string(off), "--founding", filepath.Join(a.dir, "founding.json"))
	if r.code != 3 || r.stdout != "" || r.stderr != "error founding\n" {
		t.Fatalf("verify --accord one digit off: %v", r)
	}
	for _, args := range [][]string{
		{"verify", "--receipts", a.journal(), "--accord", a.id},
		{"verify", "--receipts", a.journal(), "--accord", a.id, "--mediator", a.refs["mediator"], "--founding", filepath.Join(a.dir, "founding.json")},
		{"verify", "--receipts", a.journal()},
	} {
		if r := run(t, env(), args...); r.code != 3 || r.stderr != "error usage\n" {
			t.Fatalf("%q: %v", args, r)
		}
	}
}

// TestVerifyWitnessesTheAnswer verifies the journal with its answers.jsonl and with it removed: the
// accept and its release are witnessed by the decider's key, and then unwitnessed, the roles the
// decider's binding gives named for both, the release's sender being the asker; both exit 0
// (verify-3, U22).
func TestVerifyWitnessesTheAnswer(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	ids := journalIDs(t, a, a.journal())
	r := verifyAccord(t, a, a.journal())
	witnessed := func(id string) string {
		return fmt.Sprintf(cli.LineWitnessed, id, a.refs["decider"], a.refs["principal"], "decider")
	}
	if r.code != 0 || !slices.Contains(r.lines(), witnessed(ids[1])) || !slices.Contains(r.lines(), witnessed(ids[2])) ||
		!slices.Contains(r.lines(), fmt.Sprintf(cli.LineTermsDecided, 1)) {
		t.Fatalf("verify with answers.jsonl: %v", r)
	}
	bare := copyDir(t, a.journal())
	if err := os.Remove(filepath.Join(bare, "answers.jsonl")); err != nil {
		t.Fatal(err)
	}
	r = verifyAccord(t, a, bare)
	if r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineUnwitnessed, ids[1], "decider")) ||
		!slices.Contains(r.lines(), fmt.Sprintf(cli.LineUnwitnessed, ids[2], "decider")) || !slices.Contains(r.lines(), cli.LineConforms) {
		t.Fatalf("verify without answers.jsonl: %v", r)
	}
	// The calls given in a file of their own witness the answer again.
	calls := filepath.Join(a.dir, "calls")
	b, _ := os.ReadFile(filepath.Join(a.journal(), "answers.jsonl"))
	if err := os.WriteFile(calls, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := verifyAccord(t, a, bare, "--answers", calls); r.code != 0 || !slices.Contains(r.lines(), witnessed(ids[1])) {
		t.Fatalf("verify --answers: %v", r)
	}
	// A call that matches no receipt is counted and fails nothing (U31).
	stray := filepath.Join(a.dir, "stray")
	line := strings.Replace(string(b), `"release":true`, `"release":false`, 1)
	if err := os.WriteFile(stray, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := verifyAccord(t, a, a.journal(), "--answers", stray); r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineUnmatched, 1)) {
		t.Fatalf("verify with a call no receipt names: %v", r)
	}
}

// TestVerifyASubset verifies the last two receipts of a chain of three as a subset: it exits 0, says
// the chain was not checked before seq 2 and that completeness is not claimed of a subset; without
// --subset it fails chain at line 1 (receipt-6, U24, U23). A whole journal given as a subset has
// no decision outside it.
func TestVerifyASubset(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	for _, text := range []string{"one", "two", "three"} {
		must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", text)...)
	}
	dir := t.TempDir()
	b, _ := os.ReadFile(filepath.Join(a.journal(), "receipts.jsonl"))
	l := bytes.SplitAfter(b, []byte("\n"))
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), bytes.Join(l[1:], nil), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), "verify", "--receipts", dir, "--mediator", a.refs["mediator"], "--subset")
	if r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineChainStart, a.refs["p1"], a.refs["p2"], 2)) ||
		!slices.Contains(r.lines(), cli.LineSubsetNotComplete) || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineOutsideSubset, 0)) {
		t.Fatalf("verify --subset: %v", r)
	}
	if r := run(t, env(), "verify", "--receipts", dir, "--mediator", a.refs["mediator"]); r.code != 3 || r.stderr != "error verify chain receipts.jsonl line 1\n" {
		t.Fatalf("verify of the subset as a whole: %v", r)
	}
	// A whole journal given as a subset with no founding set leaves no question out: its answer's
	// decisions are not counted as outside the subset.
	acc, _ := accepted(t)
	if r := run(t, env(), "verify", "--receipts", acc.journal(), "--mediator", acc.refs["mediator"], "--subset"); r.code != 0 ||
		!slices.Contains(r.lines(), fmt.Sprintf(cli.LineOutsideSubset, 0)) {
		t.Fatalf("verify --subset of a whole journal: %v", r)
	}
}

// TestVerifyOneBody checks a delivery the receiver saved with get --json, under the mediator's
// reference with no founding set: its body is the part its receipt names, exit 0; with one byte of
// its text changed it is not, exit 1, and conformance is still not checked (verify-2 row 5, U25). A
// file that is no delivery, and a delivery whose receipt is none of the journal's, are tool error
// delivery, with no line printed.
func TestVerifyOneBody(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "The planted note.")...)
	got := must(t, a.as("p2", "get", "--json")...)
	saved := filepath.Join(a.dir, "delivery.json")
	if err := os.WriteFile(saved, []byte(got.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	id := journalIDs(t, a, a.journal())[0]
	r := run(t, env(), "verify", "--receipts", a.journal(), "--mediator", a.refs["mediator"], "--delivery", saved)
	if r.code != 0 || !slices.Contains(r.lines(), cli.LineConformNoFounding) || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineBody, id)) {
		t.Fatalf("verify --delivery: %v", r)
	}
	changed := strings.Replace(got.stdout, "The planted note.", "The planted notE.", 1)
	if err := os.WriteFile(saved, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	r = run(t, env(), "verify", "--receipts", a.journal(), "--mediator", a.refs["mediator"], "--delivery", saved)
	if r.code != 1 || !slices.Contains(r.lines(), cli.LineConformNoFounding) || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineBodyNot, 1)) {
		t.Fatalf("verify --delivery of a changed body: %v", r)
	}
	r = run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", filepath.Join(a.dir, "founding.json"), "--delivery", saved)
	if r.code != 1 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineNotConform, "body", 1)) || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineBodyNot, 1)) {
		t.Fatalf("verify --accord --delivery of a changed body: %v", r)
	}
	other := strings.Replace(got.stdout, id, "sha256:"+strings.Repeat("ab", 32), 1)
	for name, b := range map[string]string{"no delivery": "[]\n", "a delivery of another receipt": other} {
		if err := os.WriteFile(saved, []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, form := range [][]string{{"--mediator", a.refs["mediator"]}, {"--accord", a.id, "--founding", filepath.Join(a.dir, "founding.json")}} {
			args := append([]string{"verify", "--receipts", a.journal(), "--delivery", saved}, form...)
			if r := run(t, env(), args...); r.code != 3 || r.stderr != "error delivery\n" {
				t.Fatalf("verify %s of %s: %v", form[0], name, r)
			}
		}
	}
}

// TestVerifyJSONHoldsTheLines verifies the journal with --json: the object's valid, conforms,
// complete and custody are the lines it prints, and decisions holds each decision line by its
// receipt (commands-8, U26).
func TestVerifyJSONHoldsTheLines(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	plain := verifyAccord(t, a, a.journal())
	o := object(t, verifyAccord(t, a, a.journal(), "--json"))
	l := plain.lines()
	for _, name := range []string{"valid", "conforms", "complete", "custody"} {
		var s string
		if err := json.Unmarshal(o[name], &s); err != nil || !slices.Contains(l, s) {
			t.Fatalf("%s is %s, not a line of %q", name, o[name], l)
		}
	}
	var decisions []struct{ ReceiptID, Line string }
	if err := json.Unmarshal(o["decisions"], &decisions); err != nil || len(decisions) != 2 {
		t.Fatalf("decisions %s", o["decisions"])
	}
	for _, d := range decisions {
		if !slices.Contains(l, d.Line) || !strings.HasPrefix(d.Line, d.ReceiptID+" ") {
			t.Fatalf("decision %+v is not a line of %q", d, l)
		}
	}
	is(t, o, "unmatched", "0")
	is(t, o, "starts", "[]")
}

// TestVerifyCustody verifies a journal whose founding set has the operator principal of two of
// three members: custody says so (commands-8, U27).
func TestVerifyCustody(t *testing.T) {
	t.Parallel()
	a := newAccordBy(t, map[string]string{"p1": "operator", "p2": "operator"}, "p1", "p2", "p3")
	a.start(t).stop(t)
	r := verifyAccord(t, a, a.journal())
	if r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineCustody, a.refs["operator"], 2, 3)) {
		t.Fatalf("verify: %v", r)
	}
}

// TestVerifyANonconformingLog verifies the accept journal with one more receipt the mediator's key
// signed, whose decision breaks the terms, a pass of a kind no term names: it exits 1, prints the
// valid line and does not conform: decision at its line, and still prints every line of the log:
// the accept and its release witnessed, the count of the terms' decisions, the complete line, the
// narrower claim that each receipt is signed by the mediator, and the scope line, and in --json the
// same decisions (verify-2, commands-8, U28).
func TestVerifyANonconformingLog(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	dir := copyDir(t, a.journal())
	ids := journalIDs(t, a, dir)
	b, _ := os.ReadFile(filepath.Join(dir, "receipts.jsonl"))
	release, err := receipts.Decode(bytes.Split(b, []byte("\n"))[2])
	if err != nil {
		t.Fatal(err)
	}
	mk, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	p := release.Predicate
	p.Seq, p.Prev, p.By = p.Seq+1, release.ID, receipts.By{Decider: receipts.ByTerms}
	p.Envelope = receipts.Envelope{Sender: p.From, Seq: p.Envelope.Seq + 1, Hash: "sha256:" + strings.Repeat("ef", 32)}
	bad, _, err := receipts.Build(receipts.Subject{Name: "secret", Digest: strings.Repeat("ab", 32)}, p, mk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), append(append(b, bad...), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	witnessed := func(id string) string {
		return fmt.Sprintf(cli.LineWitnessed, id, a.refs["decider"], a.refs["principal"], "decider")
	}
	r := verifyAccord(t, a, dir)
	l := r.lines()
	n := len(l)
	if r.code != 1 || l[n-5] != fmt.Sprintf(cli.LineValid, 4, a.refs["mediator"]) || l[n-4] != fmt.Sprintf(cli.LineNotConform, "decision", 4) ||
		l[n-3] != cli.LineNotComplete || l[n-2] != cli.ClaimSigned || l[n-1] != cli.Scope ||
		!slices.Contains(l, witnessed(ids[1])) || !slices.Contains(l, witnessed(ids[2])) || !slices.Contains(l, fmt.Sprintf(cli.LineTermsDecided, 2)) {
		t.Fatalf("verify of a nonconforming log: %v", r)
	}
	o := object(t, verifyAccord(t, a, dir, "--json"))
	var decisions []struct{ ReceiptID, Line string }
	if err := json.Unmarshal(o["decisions"], &decisions); err != nil || len(decisions) != 2 || decisions[0].Line != witnessed(ids[1]) {
		t.Fatalf("decisions %s", o["decisions"])
	}
	is(t, o, "unmatched", "0")
}

// TestVerifyAnotherModule verifies a founding set whose card names another kernel hash: conformance
// is not checked, and it exits 0 (verify-2, U29).
func TestVerifyAnotherModule(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	_, set := a.refound(t, otherCard)
	b, err := os.ReadFile(set)
	if err != nil {
		t.Fatal(err)
	}
	id, err := canonical.TypedHash(bytes.TrimSuffix(b, []byte("\n")))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), "verify", "--receipts", dir, "--accord", id, "--founding", set, "--acceptances", a.acceptancesOf(t, set))
	if r.code != 0 || !slices.Contains(r.lines(), cli.LineConformOtherModule) {
		t.Fatalf("verify: %v", r)
	}
}

// TestShowNamesTheDecider shows the accept's receipt: the line after its verdict names the answer's
// call by its hash, and --json holds by as the predicate does (receipt-8, U32).
func TestShowNamesTheDecider(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	b, _ := os.ReadFile(filepath.Join(a.journal(), "receipts.jsonl"))
	accept := bytes.Split(b, []byte("\n"))[1]
	st, err := receipts.Decode(accept)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(a.dir, "accept.receipt")
	if err := os.WriteFile(p, accept, 0o600); err != nil {
		t.Fatal(err)
	}
	r := must(t, "show", "--receipt", p)
	l := strings.Split(r.stdout, "\n")
	if l[3] != "verdict pass, reason ok" || l[4] != "by answer "+st.Predicate.By.Answer {
		t.Fatalf("show: %q", l)
	}
	is(t, object(t, run(t, env(), "show", "--receipt", p, "--json")), "by", `{"answer":"`+st.Predicate.By.Answer+`"}`)
}

// TestVerifyNamesWhatThePolicySupersedes founds the Accord again under a policy that supersedes an
// earlier Accord, each binding signed again for it: verify --founding prints the earlier Accord's id
// and its close receipt's (setup-contract-11, U19).
func TestVerifyNamesWhatThePolicySupersedes(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	earlier, closed := "sha256:"+strings.Repeat("ab", 32), "sha256:"+strings.Repeat("cd", 32)
	b, err := os.ReadFile(filepath.Join(a.dir, "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err := json.Unmarshal(b, &policy); err != nil {
		t.Fatal(err)
	}
	policy["supersedes"] = map[string]any{"accord": earlier, "close": closed}
	pb, _ := json.Marshal(policy)
	pp := filepath.Join(a.dir, "superseding.json")
	if err := os.WriteFile(pp, pb, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"founding", "--policy", pp, "--mediator", filepath.Join(a.dir, "mediator.json"), "--card", filepath.Join(a.dir, "card.json")}
	for _, p := range []string{"p1", "p2"} {
		bind := must(t, "bind", "--key", a.key("principal"), "--actor", a.refs[p], "--role", "peer", "--mediator", a.refs["mediator"],
			"--until", "2099-01-01T00:00:00Z", "--policy", pp).stdout
		bp := filepath.Join(a.dir, p+".superseding.binding")
		if err := os.WriteFile(bp, []byte(bind), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--seal", filepath.Join(a.dir, p+".seal"), "--binding", bp)
	}
	r := must(t, args...)
	set := filepath.Join(a.dir, "superseding-founding.json")
	if err := os.WriteFile(set, []byte(r.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := canonical.TypedHash([]byte(strings.TrimSuffix(r.stdout, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	v := run(t, env(), "verify", "--receipts", dir, "--accord", id, "--founding", set, "--acceptances", a.acceptancesOf(t, set))
	if v.code != 0 || !slices.Contains(v.lines(), fmt.Sprintf(cli.LineSupersedes, earlier, closed)) {
		t.Fatalf("verify: %v", v)
	}
}

// TestVerifyWithoutTheOpenings is U4 and U18 at the command line (setup-contract-16, verify-1):
// verify with the founding set and its acceptances and no opening founds as any party can, prints
// that the terms were not checked and that the receipts conform as far as the sealed terms let it
// say, and exits 0; it prints no seal and no view line. With no acceptances.json beside the set and
// none given it is error file acceptances.
func TestVerifyWithoutTheOpenings(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	r := run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--acceptances", a.acceptances())
	l := r.lines()
	if r.code != 0 || !slices.Contains(l, cli.LineTermsNotChecked) || !slices.Contains(l, fmt.Sprintf(cli.LineValid, 3, a.refs["mediator"])) ||
		!slices.Contains(l, cli.LineConformSealed) || slices.ContainsFunc(l, func(s string) bool { return strings.HasPrefix(s, "seal ") || strings.HasPrefix(s, "view ") }) {
		t.Fatalf("verify without openings: %v", r)
	}
	if o := object(t, run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--json")); string(o["terms"]) != `"`+cli.LineTermsNotChecked+`"` ||
		string(o["seals"]) != "[]" || string(o["views"]) != "[]" {
		t.Fatalf("verify --json: %v", o)
	}
	moved := filepath.Join(t.TempDir(), "founding.json")
	b, _ := os.ReadFile(a.founding())
	if err := os.WriteFile(moved, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", moved); r.code != cli.ExitTool || r.stderr != "error file acceptances\n" {
		t.Fatalf("verify with no acceptances: %v", r)
	}
}

// TestVerifyOpensASeal is U15 (setup-contract-13, setup-contract-16): with one member's opening,
// verify prints that its seal opens and that its view was not checked, since a view reads every
// member's terms; with every opening, that each seal opens and each view matches its acceptance,
// that the terms were checked, and that the log conforms. An opening whose salt is another is error
// founding seal-mismatch; acceptances in which a member accepted a view other than its own pass
// without the openings and are error founding acceptance-view with every opening.
func TestVerifyOpensASeal(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	base := []string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding()}
	one := filepath.Join(a.openings(), cli.OpeningName(a.refs["decider"]))
	r := must(t, append(base, "--open", one)...)
	l := r.lines()
	if !slices.Contains(l, fmt.Sprintf(cli.LineSealOpens, a.refs["decider"])) || !slices.Contains(l, fmt.Sprintf(cli.LineViewNotChecked, a.refs["decider"])) ||
		!slices.Contains(l, cli.LineTermsNotChecked) || slices.Contains(l, fmt.Sprintf(cli.LineSealOpens, a.refs["proposer"])) {
		t.Fatalf("verify with one opening: %v", r)
	}
	r = must(t, append(base, a.opens()...)...)
	l = r.lines()
	for _, p := range a.parties {
		if !slices.Contains(l, fmt.Sprintf(cli.LineSealOpens, a.refs[p])) || !slices.Contains(l, fmt.Sprintf(cli.LineViewMatches, a.refs[p])) {
			t.Fatalf("verify with every opening, %s: %v", p, r)
		}
	}
	if !slices.Contains(l, cli.LineTermsChecked) || !slices.Contains(l, cli.LineConforms) {
		t.Fatalf("verify with every opening: %v", r)
	}

	// An opening whose salt is another opens nothing.
	b, _ := os.ReadFile(one)
	var op map[string]json.RawMessage
	if err := json.Unmarshal(b, &op); err != nil {
		t.Fatal(err)
	}
	op["salt"] = json.RawMessage(`"` + strings.Repeat("00", 32) + `"`)
	bad := filepath.Join(t.TempDir(), "bad.opening.json")
	nb, _ := json.Marshal(op)
	if err := os.WriteFile(bad, nb, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), append(base, "--open", bad)...); r.code != cli.ExitTool || r.stderr != "error founding seal-mismatch\n" {
		t.Fatalf("an opening of another salt: %v", r)
	}

	// The applier accepted the decider's view: only the openings show it.
	false_ := a.falseAcceptances(t, "applier", "decider")
	if r := run(t, env(), append(base, "--acceptances", false_)...); r.code != 0 {
		t.Fatalf("a false view without the openings: %v", r)
	}
	if r := run(t, env(), append(append(base, "--acceptances", false_), a.opens()...)...); r.code != cli.ExitTool || r.stderr != "error founding acceptance-view\n" {
		t.Fatalf("a false view with every opening: %v", r)
	}
}

// TestVerifyRefusesAnOpeningOfNoSeal gives --open a second opening of one actor, and an opening
// whose actor is no member: each opens no seal of the set, so verify says why, as a usage error.
func TestVerifyRefusesAnOpeningOfNoSeal(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	base := []string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding()}
	one := filepath.Join(a.openings(), cli.OpeningName(a.refs["decider"]))
	if r := run(t, env(), append(base, "--open", one, "--open", one)...); r.code != cli.ExitTool || r.stderr != cli.LineOpenTwice+"\nerror usage\n" {
		t.Fatalf("two openings of one actor: %v", r)
	}
	b, _ := os.ReadFile(one)
	stranger := "ed25519:" + strings.Repeat("ab", 32)
	other := filepath.Join(t.TempDir(), "stranger.opening.json")
	if err := os.WriteFile(other, bytes.Replace(b, []byte(a.refs["decider"]), []byte(stranger), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), append(base, "--open", other)...); r.code != cli.ExitTool || r.stderr != cli.LineOpenNotMember+"\nerror usage\n" {
		t.Fatalf("an opening of no member: %v", r)
	}
}

// TestVerifyAgainstAView is a member's view check at the command line (verify-1 rows 0a and 4a,
// verify-2, commands-8): with the applier's view as it kept it, and no opening, verify prints after the
// conforms line one line per receipt of the applier's, each matching its row of proposals from the
// proposer, and the view line, and exits 0; --json holds the same lines. A view with one byte
// changed is error founding acceptance-view and prints nothing else. A pass of a kind the applier's
// view has no row for exits 1, its line and the view line naming it. --view beside the applier's own
// --open, and --view with no founding set, are usage errors, and a view file not there is error file
// view.
func TestVerifyAgainstAView(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	view := filepath.Join(a.state("applier"), "view.json")
	base := []string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--view", view}
	ids := journalIDs(t, a, a.journal())
	ap, row := a.refs["applier"], fmt.Sprintf(cli.LineViewRowGet, "proposal", a.refs["proposer"], cli.WordAsk)
	r := run(t, env(), base...)
	l := r.lines()
	i := slices.Index(l, cli.LineConformSealed)
	want := []string{cli.LineConformSealed}
	for n, id := range ids {
		want = append(want, fmt.Sprintf(cli.LineViewOfMatches, ap, n+1, id, row))
	}
	want = append(want, fmt.Sprintf(cli.LineViewAll, ap, 3), cli.LineNotComplete)
	if r.code != 0 || i < 0 || len(l) < i+len(want) || !slices.Equal(l[i:i+len(want)], want) {
		t.Fatalf("verify --view: %v", r)
	}
	o := object(t, run(t, env(), append(base, "--json")...))
	var lines []struct{ ReceiptID, Line string }
	if err := json.Unmarshal(o["viewReceipts"], &lines); err != nil || len(lines) != 3 || lines[2].ReceiptID != ids[2] || lines[2].Line != want[3] {
		t.Fatalf("viewReceipts %s", o["viewReceipts"])
	}
	is(t, o, "viewCheck", `"`+fmt.Sprintf(cli.LineViewAll, ap, 3)+`"`)
	is(t, o, "viewer", `"`+ap+`"`)

	// One byte of the view changed: it is not the view the applier accepted.
	b, err := os.ReadFile(view)
	if err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(t.TempDir(), "view.json")
	if err := os.WriteFile(changed, bytes.Replace(b, []byte(`"timeout":3600`), []byte(`"timeout":3601`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--view", changed); r.code != cli.ExitTool ||
		r.stderr != "error founding acceptance-view\n" || r.stdout != "" {
		t.Fatalf("a changed view: %v", r)
	}

	// A pass of a kind the applier's terms do not select toward the proposer.
	dir := copyDir(t, a.journal())
	jb, _ := os.ReadFile(filepath.Join(dir, "receipts.jsonl"))
	release, err := receipts.Decode(bytes.Split(jb, []byte("\n"))[2])
	if err != nil {
		t.Fatal(err)
	}
	mk, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	p := release.Predicate
	p.Seq, p.Prev, p.By = p.Seq+1, release.ID, receipts.By{Decider: receipts.ByTerms}
	p.Envelope = receipts.Envelope{Sender: p.From, Seq: p.Envelope.Seq + 1, Hash: "sha256:" + strings.Repeat("ef", 32)}
	bad, v, err := receipts.Build(receipts.Subject{Name: "secret", Digest: strings.Repeat("ab", 32)}, p, mk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), append(append(jb, bad...), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	r = run(t, env(), "verify", "--receipts", dir, "--accord", a.id, "--founding", a.founding(), "--view", view)
	l = r.lines()
	noRow := fmt.Sprintf(cli.LineViewRowGet, "secret", a.refs["proposer"], cli.LineViewNoRow)
	if r.code != 1 || !slices.Contains(l, fmt.Sprintf(cli.LineNotConform, "view-conformance", 4)) ||
		!slices.Contains(l, fmt.Sprintf(cli.LineViewOfNotMatch, ap, 4, v.Statement().ID, noRow)) ||
		!slices.Contains(l, fmt.Sprintf(cli.LineViewNotAll, ap, 4)) || l[len(l)-1] != cli.Scope {
		t.Fatalf("a pass the view does not allow: %v", r)
	}

	own := filepath.Join(a.openings(), cli.OpeningName(ap))
	if r := run(t, env(), append(base, "--open", own)...); r.code != cli.ExitTool || r.stderr != cli.LineViewOwnOpening+"\nerror usage\n" {
		t.Fatalf("--view with the viewer's own opening: %v", r)
	}
	if r := run(t, env(), "verify", "--receipts", a.journal(), "--mediator", a.refs["mediator"], "--view", view); r.code != cli.ExitTool || r.stderr != "error usage\n" {
		t.Fatalf("--view with no founding set: %v", r)
	}
	if r := run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--view", filepath.Join(t.TempDir(), "none.json")); r.code != cli.ExitTool || r.stderr != "error file view\n" {
		t.Fatalf("--view of no file: %v", r)
	}
}

// reanswered is a copy of the accepted journal with no answers.jsonl, whose accept and release name,
// in place of the decider's call, the call change makes of it signed by the key of name; and a file
// that holds that call alone.
func reanswered(t *testing.T, a *accord, name string, change func(*answer.Call)) (dir, calls string) {
	t.Helper()
	dir = copyDir(t, a.journal())
	b, err := os.ReadFile(filepath.Join(dir, "answers.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "answers.jsonl")); err != nil {
		t.Fatal(err)
	}
	c, err := answer.Read(bytes.Split(b, []byte("\n"))[0])
	if err != nil {
		t.Fatal(err)
	}
	change(&c)
	k, err := keyfile.Read(a.key(name))
	if err != nil {
		t.Fatal(err)
	}
	call, err := answer.Sign(c, k)
	if err != nil {
		t.Fatal(err)
	}
	mk, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	rb, _ := os.ReadFile(filepath.Join(dir, "receipts.jsonl"))
	lines := bytes.Split(bytes.TrimSuffix(rb, []byte("\n")), []byte("\n"))
	for i := 1; i < len(lines); i++ {
		st, err := receipts.Decode(lines[i])
		if err != nil {
			t.Fatal(err)
		}
		p := st.Predicate
		p.By.Answer = answer.ID(call)
		if lines[i], _, err = receipts.Build(st.Subject, p, mk); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), append(bytes.Join(lines, []byte("\n")), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	calls = filepath.Join(t.TempDir(), "calls")
	if err := os.WriteFile(calls, append(call, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, calls
}

// TestVerifyContradictedCalls gives the accepted journal a call that reads but that the proposer
// signed, not the decider whose accept it is: the accept and its release are each contradicted, the
// log does not conform with witness at the accept's line, and it exits 1. With the decider's call
// asking for no release, the accept is witnessed and only the release is contradicted. No receipt
// that fails row 4 is printed witnessed or unwitnessed (verify-1 row 4, verify-3, commands-8).
func TestVerifyContradictedCalls(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	head := founded(t, verifyAccord(t, a, a.journal()))
	witnessed := func(id string) string {
		return fmt.Sprintf(cli.LineWitnessed, id, a.refs["decider"], a.refs["principal"], "decider")
	}
	for name, c := range map[string]struct {
		key        string
		change     func(*answer.Call)
		lines      func(ids []string) []string
		failedLine int
	}{
		"signed by another member": {"proposer", func(*answer.Call) {}, func(ids []string) []string {
			return []string{fmt.Sprintf(cli.LineContradicted, ids[1]), fmt.Sprintf(cli.LineContradicted, ids[2])}
		}, 2},
		"asking for no release": {"decider", func(c *answer.Call) { c.Release = false }, func(ids []string) []string {
			return []string{witnessed(ids[1]), fmt.Sprintf(cli.LineContradicted, ids[2])}
		}, 3},
	} {
		dir, calls := reanswered(t, a, c.key, c.change)
		ids := journalIDs(t, a, dir)
		r := verifyAccord(t, a, dir, "--answers", calls)
		after := append([]string{"pass 2 block 0 ask 1", "questions closed 1 open 0", "releases 1 blocked releases 0"}, c.lines(ids)...)
		want := verifyOutput(head, append(after, fmt.Sprintf(cli.LineTermsDecided, 1), fmt.Sprintf(cli.LineValid, 3, a.refs["mediator"]),
			fmt.Sprintf(cli.LineNotConform, "witness", c.failedLine), cli.LineNotComplete)...)
		if r.code != 1 || !slices.Equal(r.lines(), want) {
			t.Fatalf("%s: %v\nwant:\n%s", name, r, strings.Join(want, "\n"))
		}
	}
}

// TestVerifyAViewWithASubset gives --view with --subset: the view check reads every receipt of the
// viewer's, which a subset may leave out, so it is error usage, in an object with --json (commands-8).
func TestVerifyAViewWithASubset(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	base := []string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--view", filepath.Join(a.state("applier"), "view.json"), "--subset"}
	if r := run(t, env(), base...); r.code != cli.ExitTool || r.stderr != "error usage\n" || r.stdout != "" {
		t.Fatalf("--view --subset: %v", r)
	}
	if r := run(t, env(), append(base, "--json")...); r.code != cli.ExitTool || r.stdout != `{"error":"usage","exit":3}`+"\n" {
		t.Fatalf("--view --subset --json: %v", r)
	}
}

// TestVerifyAViewWithNoReceiptOfItsOwn checks the decider's view against the accepted journal: the
// decider gave, got and asked nothing, its accept closing a question it neither asked nor received,
// so the view line says no receipt was checked, viewCheck holds it, and it exits 0 (verify-2,
// commands-8).
func TestVerifyAViewWithNoReceiptOfItsOwn(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	args := []string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding(), "--view", filepath.Join(a.state("decider"), "view.json")}
	want := fmt.Sprintf(cli.LineViewNone, a.refs["decider"])
	head := founded(t, run(t, env(), args[:len(args)-2]...))
	witnessed := func(id string) string {
		return fmt.Sprintf(cli.LineWitnessed, id, a.refs["decider"], a.refs["principal"], "decider")
	}
	ids := journalIDs(t, a, a.journal())
	lines := verifyOutput(head, "pass 2 block 0 ask 1", "questions closed 1 open 0", "releases 1 blocked releases 0",
		witnessed(ids[1]), witnessed(ids[2]), fmt.Sprintf(cli.LineTermsDecided, 1), fmt.Sprintf(cli.LineValid, 3, a.refs["mediator"]),
		cli.LineConformSealed, want, cli.LineNotComplete)
	if r := run(t, env(), args...); r.code != 0 || !slices.Equal(r.lines(), lines) {
		t.Fatalf("a view with no receipt of its own: %v\nwant:\n%s", r, strings.Join(lines, "\n"))
	}
	o := object(t, run(t, env(), append(args, "--json")...))
	is(t, o, "viewCheck", quote(want))
	is(t, o, "viewReceipts", "[]")
}

// TestVerifyReadsTheViewBeforeValidity gives a view with one row changed and a journal whose second
// receipt's signature is broken: the view is read first, so it is error founding acceptance-view and
// no validity code is printed (verify-1 row 0a).
func TestVerifyReadsTheViewBeforeValidity(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	b, err := os.ReadFile(filepath.Join(a.state("applier"), "view.json"))
	if err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(t.TempDir(), "view.json")
	if err := os.WriteFile(changed, bytes.Replace(b, []byte(`"timeout":3600`), []byte(`"timeout":3601`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := copyDir(t, a.journal())
	lines := a.journalLines(t)
	i := bytes.Index(lines[1], []byte(`"sig":"`)) + len(`"sig":"`)
	lines[1][i] = map[bool]byte{true: 'B', false: 'A'}[lines[1][i] == 'A']
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), append(bytes.Join(lines, []byte("\n")), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), "verify", "--receipts", dir, "--mediator", a.refs["mediator"]); r.code != cli.ExitTool || r.stderr != "error verify signature receipts.jsonl line 2\n" {
		t.Fatalf("the broken journal alone: %v", r)
	}
	r := run(t, env(), "verify", "--receipts", dir, "--accord", a.id, "--founding", a.founding(), "--view", changed)
	if r.code != cli.ExitTool || r.stderr != "error founding acceptance-view\n" || r.stdout != "" {
		t.Fatalf("a changed view and a broken journal: %v", r)
	}
}

// TestVerifyABodyOfALogThatDoesNotConform checks the applier's delivery of the released proposal
// against the accepted journal with one more receipt whose decision breaks the terms: the log does
// not conform with decision at that line, and the body line still says the part is its receipt's,
// with the narrower claim line, exit 1; with one byte of the body changed the body line says so, and the conforms line keeps the
// decision code (verify-1 row 5, commands-8).
func TestVerifyABodyOfALogThatDoesNotConform(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	got := must(t, a.as("applier", "get", "--json")...)
	saved := filepath.Join(a.dir, "delivery.json")
	if err := os.WriteFile(saved, []byte(got.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := copyDir(t, a.journal())
	ids := journalIDs(t, a, dir)
	b, _ := os.ReadFile(filepath.Join(dir, "receipts.jsonl"))
	release, err := receipts.Decode(bytes.Split(b, []byte("\n"))[2])
	if err != nil {
		t.Fatal(err)
	}
	mk, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	p := release.Predicate
	p.Seq, p.Prev, p.By = p.Seq+1, release.ID, receipts.By{Decider: receipts.ByTerms}
	p.Envelope = receipts.Envelope{Sender: p.From, Seq: p.Envelope.Seq + 1, Hash: "sha256:" + strings.Repeat("ef", 32)}
	bad, _, err := receipts.Build(receipts.Subject{Name: "secret", Digest: strings.Repeat("ab", 32)}, p, mk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), append(append(b, bad...), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	witnessed := func(id string) string {
		return fmt.Sprintf(cli.LineWitnessed, id, a.refs["decider"], a.refs["principal"], "decider")
	}
	head := founded(t, verifyAccord(t, a, a.journal()))
	output := func(body string) []string {
		return verifyOutputClaiming(head, cli.ClaimSigned, "pass 3 block 0 ask 1", "questions closed 1 open 0", "releases 1 blocked releases 0",
			witnessed(ids[1]), witnessed(ids[2]), fmt.Sprintf(cli.LineTermsDecided, 2), fmt.Sprintf(cli.LineValid, 4, a.refs["mediator"]),
			fmt.Sprintf(cli.LineNotConform, "decision", 4), cli.LineNotComplete, body)
	}
	r := verifyAccord(t, a, dir, "--delivery", saved)
	if want := output(fmt.Sprintf(cli.LineBody, ids[2])); r.code != 1 || !slices.Equal(r.lines(), want) {
		t.Fatalf("the body of a log that does not conform: %v\nwant:\n%s", r, strings.Join(want, "\n"))
	}
	changed := strings.Replace(got.stdout, "Rename total.", "Rename totaL.", 1)
	if changed == got.stdout {
		t.Fatal("no byte changed")
	}
	if err := os.WriteFile(saved, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	r = verifyAccord(t, a, dir, "--delivery", saved)
	if want := output(fmt.Sprintf(cli.LineBodyNot, 3)); r.code != 1 || !slices.Equal(r.lines(), want) {
		t.Fatalf("a changed body of a log that does not conform: %v\nwant:\n%s", r, strings.Join(want, "\n"))
	}
}

// TestVerifyADeliveryOfNoReceiptAfterAClose gives a closed journal a delivery whose receiptId is
// empty, and one with none: a close is no receipt a delivery names, so each is error delivery, exit
// 3 (verify-1 row 5, commands-8).
func TestVerifyADeliveryOfNoReceiptAfterAClose(t *testing.T) {
	t.Parallel()
	a := notedAccord(t)
	a.closeNow(t)
	saved := filepath.Join(a.dir, "delivery.json")
	salt := strings.Repeat("00", 32)
	for name, d := range map[string]string{
		"an empty receiptId": `{"delivery":{"receiptId":"","kind":"note","salt":"` + salt + `","text":"One."}}`,
		"no receiptId":       `{"delivery":{"kind":"note","salt":"` + salt + `","text":"One."}}`,
	} {
		if err := os.WriteFile(saved, []byte(d+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if r := run(t, env(), "verify", "--receipts", a.journal(), "--mediator", a.refs["mediator"], "--delivery", saved); r.code != cli.ExitTool || r.stderr != "error delivery\n" {
			t.Fatalf("%s: %v", name, r)
		}
	}
}

// TestVerifyUnwitnessedNamesTheBindingsRoles verifies, with no call, an accept and its release by a
// decider whose binding gives it the roles decider and reviewer, while the question's rule names
// decider alone: both lines name the two roles, as the binding gives them, since the rule is not
// known when the terms are sealed (commands-8).
func TestVerifyUnwitnessedNamesTheBindingsRoles(t *testing.T) {
	t.Parallel()
	a := newAccordWith(t, nil, map[string][]string{"decider": {"reviewer"}}, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", q)...)
	bare := copyDir(t, a.journal())
	if err := os.Remove(filepath.Join(bare, "answers.jsonl")); err != nil {
		t.Fatal(err)
	}
	ids := journalIDs(t, a, bare)
	head := founded(t, run(t, env(), "verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding()))
	want := verifyOutput(head, "pass 2 block 0 ask 1", "questions closed 1 open 0", "releases 1 blocked releases 0",
		fmt.Sprintf(cli.LineUnwitnessed, ids[1], "decider,reviewer"), fmt.Sprintf(cli.LineUnwitnessed, ids[2], "decider,reviewer"),
		fmt.Sprintf(cli.LineTermsDecided, 1), fmt.Sprintf(cli.LineValid, 3, a.refs["mediator"]), cli.LineConformSealed, cli.LineNotComplete)
	if r := run(t, env(), "verify", "--receipts", bare, "--accord", a.id, "--founding", a.founding()); r.code != 0 || !slices.Equal(r.lines(), want) {
		t.Fatalf("an unwitnessed answer of two roles: %v\nwant:\n%s", r, strings.Join(want, "\n"))
	}
}

// TestVerifyReadsADeliveryAsOneFirst checks the applier's delivery of the released proposal, as get
// --json saved it, with the answer call inside it as maybe-12 lets it: its body line says the part is
// its receipt's. The same delivery with messageId removed, and separately with a member topic added,
// is no delivery: error delivery, exit 3, and nothing else printed (verify-1 row 5, commands-8).
func TestVerifyReadsADeliveryAsOneFirst(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	got := must(t, a.as("applier", "get", "--json")...)
	ids := journalIDs(t, a, a.journal())
	saved := filepath.Join(a.dir, "delivery.json")
	write := func(b []byte) {
		t.Helper()
		if err := os.WriteFile(saved, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write([]byte(got.stdout))
	if r := verifyAccord(t, a, a.journal(), "--delivery", saved); r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineBody, ids[2])) {
		t.Fatalf("the delivery: %v", r)
	}
	var o struct {
		Delivery map[string]json.RawMessage `json:"delivery"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &o); err != nil {
		t.Fatal(err)
	}
	if _, has := o.Delivery["answer"]; !has {
		t.Fatal("the released proposal's delivery holds no answer call")
	}
	for name, change := range map[string]func(map[string]json.RawMessage){
		"messageId removed": func(d map[string]json.RawMessage) { delete(d, "messageId") },
		"topic added":       func(d map[string]json.RawMessage) { d["topic"] = json.RawMessage(`"merges"`) },
	} {
		d := map[string]json.RawMessage{}
		for k, v := range o.Delivery {
			d[k] = v
		}
		change(d)
		b, err := json.Marshal(map[string]any{"delivery": d, "exit": 0})
		if err != nil {
			t.Fatal(err)
		}
		write(append(b, '\n'))
		if r := verifyAccord(t, a, a.journal(), "--delivery", saved); r.code != cli.ExitTool || r.stdout != "" || r.stderr != "error delivery\n" {
			t.Fatalf("%s: %v", name, r)
		}
	}
}

// TestVerifyAnEmptyJournalChecksNoUntil verifies a journal with no receipt against a founding set
// whose bindings' until, 2099-01-01, passed a year before the clock, and with the clock a year
// earlier and a year later: there is no time to found at, so no until is checked, as conform.Check
// says for the same evidence; each run founds and prints the same lines (verify-1 row 0).
func TestVerifyAnEmptyJournalChecksNoUntil(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := os.ReadFile(a.founding())
	if err != nil {
		t.Fatal(err)
	}
	acc, err := os.ReadFile(a.acceptances())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conform.Check(conform.Evidence{Accord: a.id, Founding: bytes.TrimSuffix(set, []byte("\n")),
		Acceptances: bytes.TrimSuffix(acc, []byte("\n")), Kernel: composition.KernelHash()}); err != nil {
		t.Fatalf("conform.Check: %v", err)
	}
	var first result
	for i, now := range []string{"2100-01-01T00:00:00Z", "2099-01-01T00:00:00Z", "2101-01-01T00:00:00Z"} {
		at, err := time.Parse(time.RFC3339, now)
		if err != nil {
			t.Fatal(err)
		}
		e := env()
		e.Now = func() time.Time { return at }
		r := run(t, e, "verify", "--receipts", dir, "--accord", a.id, "--founding", a.founding(), "--acceptances", a.acceptances())
		if r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LineValid, 0, a.refs["mediator"])) || !slices.Contains(r.lines(), cli.LineConformSealed) {
			t.Fatalf("at %s: %v", now, r)
		}
		if i == 0 {
			first = r
		} else if r != first {
			t.Fatalf("at %s: %v\nwant the run of %s:\n%v", now, r, "2100-01-01T00:00:00Z", first)
		}
	}
}

// TestVerifyClaimsWhatItChecked verifies the accepted journal with every member's opening, with
// --mediator alone and with no opening: the first prints the claim that each receipt is the
// mediator's signed decision under the Accord's terms, the others the narrower claim that each is
// signed by the mediator, and each --json object's claim is the line it printed (commands-8).
func TestVerifyClaimsWhatItChecked(t *testing.T) {
	t.Parallel()
	a, _ := accepted(t)
	every := append([]string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding()}, a.opens()...)
	for name, c := range map[string]struct {
		args  []string
		claim string
	}{
		"every opening":   {every, cli.ClaimVerified},
		"--mediator only": {[]string{"verify", "--receipts", a.journal(), "--mediator", a.refs["mediator"]}, cli.ClaimSigned},
		"no opening":      {[]string{"verify", "--receipts", a.journal(), "--accord", a.id, "--founding", a.founding()}, cli.ClaimSigned},
	} {
		r := run(t, env(), c.args...)
		l := r.lines()
		if n := len(l); r.code != 0 || n < 2 || l[n-2] != c.claim || l[n-1] != cli.Scope {
			t.Fatalf("%s: %v\nwant the claim %q", name, r, c.claim)
		}
		var claim string
		if o := object(t, run(t, env(), append(c.args, "--json")...)); json.Unmarshal(o["claim"], &claim) != nil || claim != c.claim {
			t.Fatalf("%s --json: claim %s, want %q", name, o["claim"], c.claim)
		}
	}
}
