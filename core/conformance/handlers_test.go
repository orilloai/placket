package conformance

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

// TestDefaultRunsEveryOperation checks that the runner has a handler for every operation of
// conformance-vectors-4, so no vector of this version fails for want of one.
func TestDefaultRunsEveryOperation(t *testing.T) {
	t.Parallel()
	var ops []string
	for op, h := range Default() {
		if h == nil {
			t.Fatalf("a nil handler for %s", op)
		}
		ops = append(ops, op)
	}
	sort.Strings(ops)
	want := []string{"canonical", "found", "reconcile", "scenario", "sign", "verify", "verify-signature"}
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("handlers for %v, want %v", ops, want)
	}
}

// vectorFile reads one of the repository's vectors as members.
func vectorFile(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(repoVectors + "/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func runParts(t *testing.T, v map[string]any) []Part {
	t.Helper()
	b, _ := json.Marshal(v)
	return Default()[v["operation"].(string)](Vector{Path: "vectors/" + v["vector"].(string) + ".json", Operation: v["operation"].(string), Bytes: b})
}

// TestAReceiptDifferingInOneByteFails is conformance-vectors-5's scenario: a give step whose
// expected receipt differs from the one written in one byte of its signature fails the vector at
// that step, and the vector as written passes.
func TestAReceiptDifferingInOneByteFails(t *testing.T) {
	t.Parallel()
	v := vectorFile(t, "crossing/a-give-then-the-end-state")
	for _, p := range runParts(t, v) {
		if p.Failure != "" || p.Pending != "" {
			t.Fatalf("the vector as written: part %s: %s%s", p.Name, p.Failure, p.Pending)
		}
	}
	step := v["steps"].([]any)[0].(map[string]any)
	entry := step["expect"].(map[string]any)["receipt"].(map[string]any)["signatures"].([]any)[0].(map[string]any)
	sig, _ := base64.StdEncoding.DecodeString(entry["sig"].(string))
	sig[17] ^= 0x80
	entry["sig"] = base64.StdEncoding.EncodeToString(sig)
	parts := runParts(t, v)
	if len(parts) == 0 || parts[0].Name != "step-0" || parts[0].Failure == "" {
		t.Fatalf("parts %+v", parts)
	}
	for _, p := range parts[1:] {
		if p.Failure != "" {
			t.Fatalf("a part after the changed step failed: %+v", p)
		}
	}
}

// TestAStepTheCoreCannotRunIsPending adds to a scenario vector a step whose op no handler runs,
// which the vector schema refuses but the handler, called directly, still reaches: the step is
// pending, and a pending step fails its vector (runner-no-silent-pass).
func TestAStepTheCoreCannotRunIsPending(t *testing.T) {
	t.Parallel()
	v := vectorFile(t, "questions/a-proposal-is-asked-about")
	pending := v["final"].(map[string]any)["pending"].([]any)
	v["steps"] = append(v["steps"].([]any), map[string]any{
		"op": "evaluate", "at": "2026-11-12T09:30:00Z", "question": pending[0],
		"expect": map[string]any{"rejection": "no-open-question"},
	})
	parts := runParts(t, v)
	last := parts[len(parts)-1]
	if last.Name != "step-2" || last.Pending == "" || last.Failure != "" {
		t.Fatalf("parts %+v", parts)
	}
}

// TestResolveAndTickStepsCompareExactly changes one expected value of a resolve step, of its
// release, and of a tick step, and checks that each vector then fails at that step
// (conformance-vectors-5, conformance-vectors-6, conformance-vectors-7).
func TestResolveAndTickStepsCompareExactly(t *testing.T) {
	t.Parallel()
	expectOf := func(v map[string]any, i int) map[string]any {
		return v["steps"].([]any)[i].(map[string]any)["expect"].(map[string]any)
	}
	for _, c := range []struct {
		name   string
		step   int
		change func(v map[string]any)
	}{
		{"questions/an-accept-releases-the-asked-part", 1, func(v map[string]any) { expectOf(v, 1)["reason"] = "maybe"; expectOf(v, 1)["verdict"] = "block" }},
		{"questions/an-accept-releases-the-asked-part", 1, func(v map[string]any) {
			expectOf(v, 1)["release"].(map[string]any)["receiptId"] = "sha256:" + strings.Repeat("0", 64)
		}},
		{"questions/an-accept-releases-the-asked-part", 1, func(v map[string]any) { delete(expectOf(v, 1), "release") }},
		{"questions/a-change-released-to-the-receiver", 1, func(v map[string]any) {
			v["steps"].([]any)[1].(map[string]any)["salt"] = strings.Repeat("ab", 16)
		}},
		{"questions/one-receipt-at-the-deadline", 1, func(v map[string]any) { expectOf(v, 1)["receipts"] = []any{} }},
		{"questions/before-the-deadline", 1, func(v map[string]any) {
			expectOf(v, 1)["receipts"] = vectorFile(t, "questions/one-receipt-at-the-deadline")["steps"].([]any)[1].(map[string]any)["expect"].(map[string]any)["receipts"]
		}},
	} {
		v := vectorFile(t, c.name)
		for _, p := range runParts(t, v) {
			if p.Failure != "" || p.Pending != "" {
				t.Fatalf("%s as written: part %s: %s%s", c.name, p.Name, p.Failure, p.Pending)
			}
		}
		c.change(v)
		parts := runParts(t, v)
		if len(parts) <= c.step || parts[c.step].Name != "step-"+strconv.Itoa(c.step) || parts[c.step].Failure == "" {
			t.Fatalf("%s: a changed expectation passed: %+v", c.name, parts)
		}
	}
}

// TestEveryVectorRunsAndNoEntryIsOpen runs the repository's vectors: every vector passes, so no part
// is pending, and the coverage list leaves no entry open, in whole or in part (conformance-vectors-6,
// conformance-vectors-7, runner-no-silent-pass).
func TestEveryVectorRunsAndNoEntryIsOpen(t *testing.T) {
	t.Parallel()
	r := Run(os.DirFS(repoSpec), os.DirFS(repoVectors), Default())
	if !r.Passed() || len(r.Results) == 0 {
		t.Fatalf("the run failed: %q", r.Problems)
	}
	if len(r.Open) != 0 {
		t.Fatalf("%d coverage entries are open, the first %+v", len(r.Open), r.Open[0])
	}
}

// TestHeaderRefusesWhatTheCanonicalFormRefuses runs a vector file holding a fraction: the runner's
// own reader takes it, and the canonical form refuses it, before its group runs
// (conformance-vectors-11).
func TestHeaderRefusesWhatTheCanonicalFormRefuses(t *testing.T) {
	t.Parallel()
	r := fixture(t, "header-canonical", Default())
	expect(t, r, false,
		`fail: vectors/canonical/a-fraction.json: header: the canonical form refuses the file: canonical: fractional-number at offset 198`,
		`open: receipt-6: M1`, `open: receipt-6: scenario "a chain that verifies": M1`,
		`1 vector: 0 passed, 1 failed; 0 other failures; 2 entries open`)
}

// TestARefusedVectorExampleThatValidatesFails is conformance-vectors-10's first scenario: a refused
// example of vector.json that the schema accepts fails the run, naming the example.
func TestARefusedVectorExampleThatValidatesFails(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("testdata/coverage-tests/vectors/canonical/sorted.json")
	if err != nil {
		t.Fatal(err)
	}
	spec := tree(t, "testdata/spec-small", tree(t, repoSpec, nil))
	spec["schema/testdata/vector/refused/a-valid-vector.json"] = &fstest.MapFile{Data: b}
	r := Run(spec, tree(t, "testdata/coverage-tests/vectors", nil), Default())
	want := []string{"spec/schema/testdata/vector/refused/a-valid-vector.json: a refused example the schema accepts"}
	if !reflect.DeepEqual(r.Problems, want) || r.Passed() {
		t.Fatalf("problems %q", r.Problems)
	}
}

// TestCoverageNamesGoTestsAndWhatStaysOpen reads a coverage list whose entries name Go tests beside
// vectors, and say what stays open of an entry covered in part: the run passes and prints each such
// entry as open, with what stays open.
func TestCoverageNamesGoTestsAndWhatStaysOpen(t *testing.T) {
	t.Parallel()
	r := fixture(t, "coverage-tests", Default())
	expect(t, r, true,
		`pass: vectors/canonical/sorted.json: 1 part`,
		`open: canonical-1: M2, in part: the order of names outside the Basic Multilingual Plane`,
		`open: receipt-6: scenario "a chain that verifies": M3, in part: a longer chain`,
		`1 vector: 1 passed, 0 failed; 0 other failures; 2 entries open`)
}

// TestCoverageTestEntryShapes refuses entries that name Go tests or what stays open wrongly.
func TestCoverageTestEntryShapes(t *testing.T) {
	t.Parallel()
	spec := tree(t, "testdata/spec-small", tree(t, repoSpec, nil))
	const receipt6 = `"receipt-6": {"open": "M1", "scenarios": {"a chain that verifies": {"open": "M1"}}}`
	const sorted = `"members sorted": {"open": "M1"}`
	for _, c := range []struct {
		entry string
		want  []string
	}{
		{`{"tests": [], "scenarios": {` + sorted + `}}`, []string{`vectors/coverage.json: canonical-1: "tests" is not a list of Go tests`}},
		{`{"tests": ["TestX"], "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: "TestX" is not a Go test's name, its package's directory and its name joined by :`}},
		{`{"tests": ["core/x:Testx"], "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: "core/x:Testx" is not a Go test's name, its package's directory and its name joined by :`}},
		{`{"tests": ["core/X:TestX"], "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: "core/X:TestX" is not a Go test's name, its package's directory and its name joined by :`}},
		{`{"tests": ["core/x:TestX"], "open": "M1", "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: an entry has either "tests" or "open", and this has both`}},
		{`{"open": "M1", "rest": {"open": "M2", "what": "x"}, "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: "rest" says what stays open of an entry covered in part, and this entry is open as a whole`}},
		{`{"tests": ["core/x:TestX"], "rest": {"open": "M4", "what": "x"}, "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: "rest" is not an object of "open", the milestone that writes the rest, and "what", what stays open`}},
		{`{"tests": ["core/x:TestX"], "rest": {"open": "M2"}, "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: "rest" is not an object of "open", the milestone that writes the rest, and "what", what stays open`}},
		{`{"rest": {"open": "M2", "what": "x"}, "scenarios": {` + sorted + `}}`,
			[]string{`vectors/coverage.json: canonical-1: an entry has either "vectors" or "open", and this has neither`}},
	} {
		list := `{"canonical-1": ` + c.entry + `, ` + receipt6 + `}`
		r := Run(spec, fstest.MapFS{"coverage.json": {Data: []byte(list)}}, Default())
		if r.Passed() || !reflect.DeepEqual(r.Problems, c.want) {
			t.Errorf("%s:\n  got  %q\n  want %q", c.entry, r.Problems, c.want)
		}
	}
}

// TestHandlersCompareExactly changes one expected value of a vector of each operation and checks the
// vector then fails (conformance-vectors-5).
func TestHandlersCompareExactly(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		change func(v map[string]any)
	}{
		{"canonical/members-sorted", func(v map[string]any) { v["expect"].(map[string]any)["hash"] = "sha256:" + strings.Repeat("0", 64) }},
		{"canonical/duplicate-key", func(v map[string]any) { v["expect"].(map[string]any)["offset"] = 8 }},
		{"signatures/sign-declaration", func(v map[string]any) { v["expect"].(map[string]any)["sig"] = strings.Repeat("0", 128) }},
		{"signatures/reference-forms", func(v map[string]any) { v["expect"].(map[string]any)["valid"] = false }},
		{"founding/ask-and-log", func(v map[string]any) { v["expect"].(map[string]any)["card"] = "sha256:" + strings.Repeat("0", 64) }},
		{"founding/binding-unlisted-principal", func(v map[string]any) { v["expect"].(map[string]any)["code"] = "binding-signature" }},
		{"receipts/a-chain-verifies", func(v map[string]any) { v["expect"].(map[string]any)["verified"] = 2 }},
		{"receipts/an-unfetched-delivery", func(v map[string]any) { v["expect"].(map[string]any)["unrecorded"] = []any{} }},
		{"crossing/a-delivery-is-fetched-once", func(v map[string]any) {
			v["steps"].([]any)[2].(map[string]any)["expect"] = v["steps"].([]any)[1].(map[string]any)["expect"]
		}},
		{"questions/a-proposal-is-asked-about", func(v map[string]any) { v["final"].(map[string]any)["pending"] = []any{} }},
		{"crossing/a-give-then-the-end-state", func(v map[string]any) { v["final"].(map[string]any)["delivered"] = map[string]any{} }},
	} {
		v := vectorFile(t, c.name)
		for _, p := range runParts(t, v) {
			if p.Failure != "" || p.Pending != "" {
				t.Fatalf("%s as written: part %s: %s", c.name, p.Name, p.Failure)
			}
		}
		c.change(v)
		failed := false
		for _, p := range runParts(t, v) {
			failed = failed || p.Failure != ""
		}
		if !failed {
			t.Fatalf("%s: a changed expectation passed", c.name)
		}
	}
}
