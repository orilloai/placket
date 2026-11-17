package conformance

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestAVectorWithNoStringOperationFails runs a vector whose schema does not require an operation and
// that has none, or a number: the runner fails it as a part it cannot run, and never panics (core-no-panic).
func TestAVectorWithNoStringOperationFails(t *testing.T) {
	t.Parallel()
	spec := tree(t, "testdata/spec-small", tree(t, repoSpec, nil))
	for k := range spec {
		if strings.HasPrefix(k, "schema/testdata/vector/") {
			delete(spec, k)
		}
	}
	spec["schema/vector.json"] = &fstest.MapFile{Data: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema",` +
		`"$id":"https://placket.dev/schema/0.1/vector.json","type":"object"}`)}
	for _, op := range []string{``, `,"operation":1`} {
		vectors := fstest.MapFS{
			"coverage.json": {Data: []byte(`{"canonical-1":{"vectors":["founding/no-operation"],` +
				`"scenarios":{"members sorted":{"vectors":["founding/no-operation"]}}},` +
				`"receipt-6":{"open":"M1","scenarios":{"a chain that verifies":{"open":"M1"}}}}`)},
			"founding/no-operation.json": {Data: []byte(`{"vector":"founding/no-operation","spec":"0.1",` +
				`"tests":"a vector with no operation","inputText":"{}"` + op + `}`)},
		}
		r := Run(spec, vectors, Default())
		expect(t, r, false,
			`fail: vectors/founding/no-operation.json: operation: no member "operation" that is a string, so no part of the vector can run`,
			`open: receipt-6: M1`, `open: receipt-6: scenario "a chain that verifies": M1`,
			`1 vector: 0 passed, 1 failed; 0 other failures; 2 entries open`)
	}
}
