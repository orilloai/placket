package conformance

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// The repository's own spec/ and vectors/, two levels up from this package.
const (
	repoSpec    = "../../spec"
	repoVectors = "../../vectors"
)

// tree reads the files under dir into a map file system, adding them over base when it is not nil.
func tree(t *testing.T, dir string, base fstest.MapFS) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	for k, v := range base {
		out[k] = v
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return out
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = &fstest.MapFile{Data: b}
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	return out
}

// fixture runs the runner over the fixture testdata/<name>: its vectors/, and the repository's
// spec/ with the short requirement text of testdata/spec-small/ and then the fixture's own spec/
// laid over it.
func fixture(t *testing.T, name string, h Handlers) Report {
	t.Helper()
	spec := tree(t, filepath.Join("testdata", name, "spec"), tree(t, "testdata/spec-small", tree(t, repoSpec, nil)))
	vectors := tree(t, filepath.Join("testdata", name, "vectors"), nil)
	return Run(spec, vectors, h)
}

// expect compares the whole report, line by line, and its verdict.
func expect(t *testing.T, r Report, passed bool, lines ...string) {
	t.Helper()
	if got := r.Lines(); !reflect.DeepEqual(got, lines) {
		t.Fatalf("report lines:\n  got  %q\n  want %q", got, lines)
	}
	if r.Passed() != passed {
		t.Fatalf("Passed() = %v, want %v", r.Passed(), passed)
	}
}

// The open entries of testdata/spec-small/, as every fixture whose coverage list leaves both its
// requirements open prints them.
var smallOpen = []string{
	`open: canonical-1: M1`,
	`open: canonical-1: scenario "members sorted": M1`,
}

// passing is a handler whose every part matched.
func passing(parts ...string) Handler {
	return func(Vector) []Part {
		var out []Part
		for _, p := range parts {
			out = append(out, Part{Name: p})
		}
		return out
	}
}

// pendingAfterPass is the handler of the pending-part fixture: its first part matched and its
// second needs a package that does not exist yet.
func pendingAfterPass(Vector) []Part {
	return []Part{{Name: "verified"}, {Name: "index", Pending: "needs the receipts package"}}
}

func TestPendingPartFails(t *testing.T) {
	t.Parallel()
	r := fixture(t, "pending-part", Handlers{"verify": pendingAfterPass})
	expect(t, r, false, append([]string{
		`fail: vectors/receipts/a-chain.json: results: part index is pending (needs the receipts package), and a pending part fails`,
	}, append(smallOpen, `1 vector: 0 passed, 1 failed; 0 other failures; 2 entries open`)...)...)
}

func TestOperationWithoutHandlerFails(t *testing.T) {
	t.Parallel()
	for name, h := range map[string]Handlers{
		"the default handlers":            Default(),
		"a handler for another operation": {"verify": passing("verified")},
		"a nil handler":                   {"found": nil},
	} {
		t.Run(name, func(t *testing.T) {
			r := fixture(t, "no-handler", h)
			expect(t, r, false, append([]string{
				`fail: vectors/founding/a-stranger.json: operation: no handler runs the operation "found", so no part of the vector can run`,
			}, append(smallOpen, `1 vector: 0 passed, 1 failed; 0 other failures; 2 entries open`)...)...)
		})
	}
}

func TestInputGivenTwiceFails(t *testing.T) {
	t.Parallel()
	r := fixture(t, "two-inputs", Handlers{"canonical": passing("canonical")})
	expect(t, r, false, append([]string{
		`fail: vectors/canonical/two-forms.json: schema: /: holds 2 of the 2 schemas of vector.json#/allOf/0/then/oneOf, not exactly one`,
	}, `open: receipt-6: M1`, `open: receipt-6: scenario "a chain that verifies": M1`,
		`1 vector: 0 passed, 1 failed; 0 other failures; 2 entries open`)...)
}

func TestVectorAgainstSchemaFails(t *testing.T) {
	t.Parallel()
	r := fixture(t, "invalid", Handlers{"canonical": passing("canonical")})
	expect(t, r, false,
		`fail: vectors/canonical/deep.json: schema: /expect/canonicalHex: does not match ^(?:[0-9a-f]{2})*$`,
		`fail: vectors/canonical/unlisted.json: schema: /: has a member "Vector" the format does not list`,
		`open: receipt-6: M1`, `open: receipt-6: scenario "a chain that verifies": M1`,
		`2 vectors: 0 passed, 2 failed; 0 other failures; 2 entries open`)
}

func TestHeaderFirst(t *testing.T) {
	t.Parallel()
	r := fixture(t, "header", Handlers{"canonical": passing("canonical")})
	expect(t, r, false,
		`fail: vectors/canonical/elsewhere.json: header: its "vector" is "canonical/somewhere", not its own path "canonical/elsewhere"`,
		`fail: vectors/canonical/no-tests.json: header: no member "tests"`,
		`fail: vectors/canonical/spec-twice.json: header: the name "spec" repeated in one object, at byte 57`,
		`fail: vectors/canonical/tests-a-number.json: header: its "tests" is not a string`,
		`open: receipt-6: M1`, `open: receipt-6: scenario "a chain that verifies": M1`,
		`4 vectors: 0 passed, 4 failed; 0 other failures; 2 entries open`)
}

func TestHandlerWithoutPartsFails(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		h    Handler
		line string
	}{
		"no part": {func(Vector) []Part { return nil },
			`fail: vectors/receipts/a-chain.json: results: the handler reported no part, and a vector with no part compared does not pass`},
		"a part that differs": {func(Vector) []Part { return []Part{{Name: "verified", Failure: "verified 2, want 0"}} },
			`fail: vectors/receipts/a-chain.json: results: part verified: verified 2, want 0`},
		"a panic": {func(Vector) []Part { var p []Part; return p[:1] },
			`fail: vectors/receipts/a-chain.json: results: the handler stopped: it panicked`},
	} {
		t.Run(name, func(t *testing.T) {
			r := fixture(t, "pending-part", Handlers{"verify": c.h})
			expect(t, r, false, append([]string{c.line},
				append(smallOpen, `1 vector: 0 passed, 1 failed; 0 other failures; 2 entries open`)...)...)
		})
	}
}

func TestPassingVectorPasses(t *testing.T) {
	t.Parallel()
	var got Vector
	h := func(v Vector) []Part { got = v; return passing("verified", "index")(v) }
	r := fixture(t, "pending-part", Handlers{"verify": h})
	expect(t, r, true, append([]string{`pass: vectors/receipts/a-chain.json: 2 parts`},
		append(smallOpen, `1 vector: 1 passed, 0 failed; 0 other failures; 2 entries open`)...)...)
	b, err := os.ReadFile("testdata/pending-part/vectors/receipts/a-chain.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "vectors/receipts/a-chain.json" || got.Operation != "verify" || string(got.Bytes) != string(b) {
		t.Fatalf("the handler got %q, %q and %d bytes", got.Path, got.Operation, len(got.Bytes))
	}
}

func TestEmptyVectorsPass(t *testing.T) {
	t.Parallel()
	r := fixture(t, "empty", Default())
	expect(t, r, true,
		`open: canonical-1: M1`,
		`open: canonical-1: scenario "members sorted": M1`,
		`open: receipt-6: M2`,
		`open: receipt-6: scenario "a chain that verifies": M2`,
		`0 vectors: 0 passed, 0 failed; 0 other failures; 4 entries open`)
}

func TestInputsOnce(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ vector, want string }{
		{`{"operation":"canonical","inputText":"{}","inputHex":"7b7d"}`,
			`/: the input is given in 2 forms, inputText and inputHex; it is given exactly once`},
		{`{"operation":"found","input":{},"inputText":"{}","inputHex":"7b7d"}`,
			`/: the input is given in 3 forms, input and inputText and inputHex; it is given exactly once`},
		{`{"operation":"found"}`, `/: the input is missing; it is given as one of input, inputText, inputHex`},
		{`{"operation":"canonical","input":{}}`, `/: a canonical vector takes its input as inputText or inputHex only`},
		{`{"operation":"sign","inputText":"{}"}`, `/: a sign vector takes its input as input, an object`},
		{`{"operation":"canonical","inputHex":"7B7D"}`,
			`/: inputHex does not decode: it is not an even number of lowercase hex digits`},
		{`{"operation":"canonical","inputHex":"7b7"}`,
			`/: inputHex does not decode: it is not an even number of lowercase hex digits`},
		{`{"operation":"verify","inputText":"{}"}`, ``},
		{`{"operation":"scenario","steps":[{"op":"get"},{"op":"give","message":{},"messageHex":"7b7d","envelope":{}}]}`,
			`/steps/1: the message is given in 2 forms, message and messageHex; it is given exactly once`},
		{`{"operation":"scenario","steps":[{"op":"give","message":{}}]}`,
			`/steps/0: the envelope is missing; it is given as one of envelope, envelopeText, envelopeHex`},
		{`{"operation":"verify","receipts":[{"receipt":{}},{"receiptText":"{}","receiptHex":"7b7d"}]}`,
			`/receipts/1: the receipt is given in 2 forms, receiptText and receiptHex; it is given exactly once`},
		{`{"operation":"reconcile","receipts":[],"kept":{"a":{"envelopes":[],"delivered":[{}]}}}`,
			`/kept/a/delivered/0: the receipt is missing; it is given as one of receipt, receiptText, receiptHex`},
		{`{"operation":"verify","receipts":[{"receiptHex":"zz"}]}`,
			`/receipts/0: receiptHex does not decode: it is not an even number of lowercase hex digits`},
		{`{"operation":"verify","receipts":[1]}`, `/receipts/0: not an object holding a receipt`},
		{`{"operation":"canonical","inputHex":""}`, ``},
	} {
		v, err := read([]byte(c.vector))
		if err != nil {
			t.Fatalf("%s: %v", c.vector, err)
		}
		if got := checkInputs(v.(*object)); got != c.want {
			t.Errorf("%s:\n  got  %q\n  want %q", c.vector, got, c.want)
		}
	}
}

// TestVectors is the conformance run over the repository's own vectors/. It prints the report, the
// open entries of the coverage list included, and fails unless every vector passed and nothing else
// failed. `make ci` runs it as its runner-no-silent-pass step.
func TestVectors(t *testing.T) {
	r := Run(os.DirFS(repoSpec), os.DirFS(repoVectors), Default())
	for _, l := range r.Lines() {
		fmt.Println("runner-no-silent-pass: " + l)
	}
	if !r.Passed() {
		var failed []string
		for _, l := range r.Lines() {
			if strings.HasPrefix(l, "fail: ") {
				failed = append(failed, l)
			}
		}
		t.Fatalf("the conformance run failed:\n%s", strings.Join(failed, "\n"))
	}
}
