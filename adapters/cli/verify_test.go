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

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/canonical"
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
// the accept and its release witnessed, the count of the terms' decisions, the complete, claim and
// scope lines, and in --json the same decisions (verify-2, commands-8, U28).
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
		l[n-3] != cli.LineNotComplete || l[n-2] != cli.ClaimVerified || l[n-1] != cli.Scope ||
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
