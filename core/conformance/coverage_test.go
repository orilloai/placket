package conformance

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestCoverageNamesAbsentVector(t *testing.T) {
	t.Parallel()
	r := fixture(t, "coverage-absent", Default())
	expect(t, r, false,
		`fail: vectors/coverage.json: canonical-1: names canonical/absent, and there is no vectors/canonical/absent.json`,
		`fail: vectors/coverage.json: canonical-1: scenario "members sorted": names canonical/absent, and there is no vectors/canonical/absent.json`,
		`fail: vectors/: no vector found, and vectors/coverage.json names 1`,
		`open: receipt-6: M1`,
		`open: receipt-6: scenario "a chain that verifies": M1`,
		`0 vectors: 0 passed, 0 failed; 3 other failures; 2 entries open`)
}

func TestCoverageLacksListedID(t *testing.T) {
	t.Parallel()
	r := fixture(t, "coverage-lacks-id", Default())
	expect(t, r, false,
		`fail: vectors/coverage.json: receipt-6: no entry, and spec/ids.txt lists it`,
		`open: canonical-1: M1`,
		`open: canonical-1: scenario "members sorted": M1`,
		`0 vectors: 0 passed, 0 failed; 1 other failure; 2 entries open`)
}

func TestCoverageListIsExact(t *testing.T) {
	t.Parallel()
	r := fixture(t, "coverage-exact", Handlers{"verify": passing("verified")})
	expect(t, r, false,
		`pass: vectors/receipts/a-chain.json: 1 part`,
		`fail: vectors/coverage.json: receipt-99: not an id spec/ids.txt lists`,
		`fail: vectors/coverage.json: canonical-1: "open" is not the milestone that writes its vectors, M1, M2 or M3`,
		`fail: vectors/coverage.json: canonical-1: scenario "members sorted": no entry, and spec/protocol.md has it`,
		`fail: vectors/coverage.json: canonical-1: scenario "members shuffled": spec/protocol.md has no such scenario under canonical-1`,
		`fail: vectors/coverage.json: receipt-6: an entry has either "vectors" or "open", and this has both`,
		`fail: vectors/coverage.json: receipt-6: scenario "a chain that verifies": a member "notes" an entry does not have`,
		`fail: vectors/receipts/a-chain.json: no entry of vectors/coverage.json names it`,
		`1 vector: 1 passed, 0 failed; 7 other failures; 0 entries open`)
}

func TestCoverageListMissing(t *testing.T) {
	t.Parallel()
	spec := tree(t, "testdata/spec-small", tree(t, repoSpec, nil))
	r := Run(spec, fstest.MapFS{}, Default())
	expect(t, r, false,
		`fail: vectors/coverage.json: missing or unreadable; it says what covers each requirement`,
		`0 vectors: 0 passed, 0 failed; 1 other failure; 0 entries open`)
}

func TestCoverageEntryShapes(t *testing.T) {
	t.Parallel()
	spec := tree(t, "testdata/spec-small", tree(t, repoSpec, nil))
	const receipt6 = `"receipt-6": {"open": "M1", "scenarios": {"a chain that verifies": {"open": "M1"}}}`
	const noScenario = `vectors/coverage.json: canonical-1: scenario "members sorted": no entry, and spec/protocol.md has it`
	for _, c := range []struct {
		list string
		want []string
	}{
		{`[]`, []string{`vectors/coverage.json: not a JSON object of requirement ids`}},
		{`{"canonical-1": {"open": "M1"}, "canonical-1": {"open": "M1"}}`,
			[]string{`vectors/coverage.json: the name "canonical-1" repeated in one object, at byte 32`}},
		{`{"canonical-1": [], ` + receipt6 + `}`,
			[]string{`vectors/coverage.json: canonical-1: not an object`, noScenario}},
		{`{"canonical-1": {"vectors": [], "scenarios": {"members sorted": {"open": "M1"}}}, ` + receipt6 + `}`,
			[]string{`vectors/coverage.json: canonical-1: "vectors" is not a list of vectors`}},
		{`{"canonical-1": {"vectors": ["Canonical/x"], "scenarios": {"members sorted": {"open": "M1"}}}, ` + receipt6 + `}`,
			[]string{`vectors/coverage.json: canonical-1: "Canonical/x" is not a vector's name, its group and name joined by /`}},
		{`{"canonical-1": {"scenarios": {"members sorted": {"open": "M1"}}}, ` + receipt6 + `}`,
			[]string{`vectors/coverage.json: canonical-1: an entry has either "vectors" or "open", and this has neither`}},
		{`{"canonical-1": {"open": "M1", "scenarios": []}, ` + receipt6 + `}`,
			[]string{`vectors/coverage.json: canonical-1: "scenarios" is not an object`, noScenario}},
		{`{"canonical-1": {"open": "M1", "scenarios": {"members sorted": {"open": "M1", "scenarios": {}}}}, ` + receipt6 + `}`,
			[]string{`vectors/coverage.json: canonical-1: scenario "members sorted": a member "scenarios" an entry does not have`}},
	} {
		r := Run(spec, fstest.MapFS{"coverage.json": {Data: []byte(c.list)}}, Default())
		if r.Passed() {
			t.Errorf("%s: the run passed", c.list)
		}
		if !reflect.DeepEqual(r.Problems, c.want) {
			t.Errorf("%s:\n  got  %q\n  want %q", c.list, r.Problems, c.want)
		}
	}
}
