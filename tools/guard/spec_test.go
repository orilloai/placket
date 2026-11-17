package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	protocolMD = "spec/protocol.md"
	draft      = "https://json-schema.org/draft/2020-12/schema"
	uri        = "https://placket.tech/schema/0.1"
)

// move renames a file of the copy into another directory.
func move(t *testing.T, dir, rel, toDir string) {
	t.Helper()
	to := filepath.Join(dir, filepath.FromSlash(toDir), filepath.Base(rel))
	if err := os.Rename(filepath.Join(dir, filepath.FromSlash(rel)), to); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func appendTo(t *testing.T, dir, rel, text string) {
	t.Helper()
	edit(t, dir, rel, func(s string) string { return s + text })
}

// specCases are the planted faults of specs-name-behaviour and schemas-hold. They plant their faults
// in a copy of the live spec/, so they keep reading the real text and schemas, and their expected
// lines are computed from it.
func specCases(t *testing.T) []tcase {
	text := liveLines(t, protocolMD)
	ids := liveLines(t, "spec/ids.txt")
	reserved := liveLines(t, "spec/reserved.txt")
	if len(reserved) == 0 {
		t.Fatalf("spec/reserved.txt lists no id")
	}
	firstLine, firstHeading := 0, ""
	for i, l := range text {
		if strings.HasPrefix(l, "### ") {
			firstLine, firstHeading = i+1, l
			break
		}
	}
	if firstLine == 0 {
		t.Fatalf("%s has no requirement heading", protocolMD)
	}
	firstID := strings.Fields(firstHeading)[1]
	firstTitle := strings.TrimPrefix(firstHeading, "### "+firstID+" ")
	idLine := 0
	for i, l := range ids {
		if l == firstID {
			idLine = i + 1
		}
	}
	at := func(n int) string { return strconv.Itoa(n) }
	fl, next, nextID := at(firstLine), at(len(text)+1), at(len(ids)+1)
	const S, H = "specs-name-behaviour", "schemas-hold"
	P := protocolMD
	spec := []string{S}
	schemas := []string{H}

	return []tcase{
		// specs-name-behaviour: the requirement headings and the two id lists.
		{name: "heading without its id", setup: func(t *testing.T, dir string) {
			edit(t, dir, P, func(s string) string {
				lines := strings.Split(s, "\n")
				lines[firstLine-1] = "### " + firstTitle
				return strings.Join(lines, "\n")
			})
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + fl + `: a level-3 heading that is not "### <id> <title>": ### ` + firstTitle,
			S + ": spec/ids.txt:" + at(idLine) + ": " + firstID + ": no requirement heading in " + P,
		}},
		{name: "an id twice", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, P, firstHeading+"\n\n#### Scenario: again\n")
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + next + ": " + firstID + ": a second heading for this id (the first is at line " + fl + ")",
		}},
		{name: "a requirement with no scenario", setup: func(t *testing.T, dir string) {
			edit(t, dir, P, func(s string) string {
				var out []string
				past := false
				for i, l := range strings.Split(s, "\n") {
					if i+1 > firstLine && regexp.MustCompile(`^(#|##|###) `).MatchString(l) {
						past = true
					}
					if i+1 > firstLine && !past && strings.HasPrefix(l, "#### ") {
						continue
					}
					out = append(out, l)
				}
				return strings.Join(out, "\n")
			})
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + fl + ": " + firstID + `: no scenario (a "#### " heading under it)`,
		}},
		{name: "a listed id with no requirement", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "spec/ids.txt", "canonical-99\n")
		}, refuse: spec, set: []string{
			S + ": spec/ids.txt:" + nextID + ": canonical-99: no requirement heading in " + P,
		}},
		{name: "a requirement not listed", setup: func(t *testing.T, dir string) {
			edit(t, dir, "spec/ids.txt", func(s string) string {
				var out []string
				for _, l := range strings.Split(s, "\n") {
					if l != firstID {
						out = append(out, l)
					}
				}
				return strings.Join(out, "\n")
			})
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + fl + ": " + firstID + ": not listed in spec/ids.txt",
		}},
		{name: "an id listed twice", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "spec/ids.txt", firstID+"\n")
		}, refuse: spec, set: []string{
			S + ": spec/ids.txt:" + nextID + ": " + firstID + ": listed twice",
		}},
		{name: "an id both listed and reserved", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "spec/reserved.txt", firstID+"\n")
		}, refuse: spec, set: []string{
			S + ": spec/ids.txt:" + at(idLine) + ": " + firstID + ": also in spec/reserved.txt",
			S + ": " + P + ":" + fl + ": " + firstID + ": a reserved id (spec/reserved.txt)",
		}},
		{name: "a reserved id used", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, P, "### "+reserved[0]+" A reserved requirement\n\n#### Scenario: used\n")
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + next + ": " + reserved[0] + ": a reserved id (spec/reserved.txt)",
		}},
		{name: "a line naming a Go path", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, P, "The question is held in core/mediator until it closes.\n")
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + next + ": names Go code: The question is held in core/mediator until it closes.",
		}},
		{name: "a line naming a package-qualified name", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, P, "A release is found by receipts.Releases alone.\n")
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + next + ": names Go code: A release is found by receipts.Releases alone.",
		}},
		{name: "a line naming a function", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, P, "It is written as func Resolve.\n")
		}, refuse: spec, set: []string{
			S + ": " + P + ":" + next + ": names Go code: It is written as func Resolve.",
		}},

		// schemas-hold: every schema, its examples and the register.
		{name: "a refused example moved into accepted", setup: func(t *testing.T, dir string) {
			move(t, dir, "spec/schema/testdata/predicate/refused/block-with-reason-ok.json", "spec/schema/testdata/predicate/accepted")
		}, refuse: schemas, set: []string{
			H + ": spec/schema/testdata/predicate/accepted/block-with-reason-ok.json: refused by spec/schema/predicate.json",
		}},
		{name: "an accepted example moved into refused", setup: func(t *testing.T, dir string) {
			move(t, dir, "spec/schema/testdata/predicate/accepted/crossing-block-maybe.json", "spec/schema/testdata/predicate/refused")
		}, refuse: schemas, set: []string{
			H + ": spec/schema/testdata/predicate/refused/crossing-block-maybe.json: accepted by spec/schema/predicate.json",
		}},
		{name: "a schema no register row names", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/extra.json", `{"$schema":"`+draft+`","$id":"`+uri+`/extra.json","type":"string"}`+"\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/extra.json: no row of spec/frozen-formats.md names it"}},
		{name: "a schema that is not JSON", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/broken.json", "{\"type\": \"string\",\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/broken.json: not JSON"}},
		{name: "a schema of another draft", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/old.json", `{"$schema":"http://json-schema.org/draft-07/schema#","$id":"`+uri+`/old.json","type":"string"}`+"\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/old.json: not a JSON Schema draft 2020-12 schema: its $schema is not " + draft}},
		{name: "JSON that names no draft", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/list.json", `["not", "a", "schema"]`+"\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/list.json: not a JSON Schema draft 2020-12 schema: its $schema is not " + draft}},
		{name: "a schema whose id is not its file", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/named.json", `{"$schema":"`+draft+`","$id":"`+uri+`/other.json","type":"string"}`+"\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/named.json: its $id is not " + uri + "/named.json"}},
		{name: "a schema that would fetch", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/remote.json", `{"$schema":"`+draft+`","$id":"`+uri+`/remote.json","$ref":"https://example.com/x.json"}`+"\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/remote.json: names a schema outside " + uri + "/: https://example.com/x.json"}},
		{name: "a schema that does not compile", setup: func(t *testing.T, dir string) {
			plantSchema(t, dir, "typo", `{"$schema":"`+draft+`","$id":"`+uri+`/typo.json","type":5}`)
		}, refuse: schemas, set: []string{H + ": spec/schema/typo.json: does not compile as JSON Schema draft 2020-12 with nothing fetched"}},
		{name: "a schema with no refused example", setup: func(t *testing.T, dir string) {
			if err := os.RemoveAll(filepath.Join(dir, "spec", "schema", "testdata", "hash", "refused")); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}, refuse: schemas, set: []string{H + ": spec/schema/hash.json: no refused example in spec/schema/testdata/hash/refused/"}},
		{name: "examples of no schema", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/testdata/ghost/accepted/a.json", "{}\n")
		}, refuse: schemas, set: []string{H + ": spec/schema/testdata/ghost/: examples of no schema; there is no spec/schema/ghost.json"}},
		{name: "an example that is not JSON", setup: func(t *testing.T, dir string) {
			write(t, dir, "spec/schema/testdata/envelope/refused/cut-short.json", `{"seq":`)
		}, refuse: schemas, set: []string{H + ": spec/schema/testdata/envelope/refused/cut-short.json: not JSON"}},
	}
}

// plantSchema adds a schema to the copy with a row of the register and one example of each kind.
func plantSchema(t *testing.T, dir, name, schema string) {
	t.Helper()
	write(t, dir, "spec/schema/"+name+".json", schema+"\n")
	appendTo(t, dir, "spec/frozen-formats.md", "| "+name+" | none | `spec/schema/"+name+".json` | 0.1, unreleased | nothing | nothing |\n")
	write(t, dir, "spec/schema/testdata/"+name+"/accepted/a.json", "\"a\"\n")
	write(t, dir, "spec/schema/testdata/"+name+"/refused/one.json", "1\n")
}

func TestSpecChecks(t *testing.T) { runCases(t, specCases(t)) }

// schemaFetchCases: a schema that refers, under the schemas' own prefix, to a file spec/schema/ does
// not hold, and one that refers outside the prefix through a relative reference, which the check of
// its members cannot see, are each refused: the loader serves the tree and fetches nothing.
func schemaFetchCases() []tcase {
	const H = "schemas-hold"
	return []tcase{
		{name: "a reference to a schema the tree does not hold", setup: func(t *testing.T, dir string) {
			plantSchema(t, dir, "absent", `{"$schema":"`+draft+`","$id":"`+uri+`/absent.json","$ref":"`+uri+`/missing.json"}`)
		}, refuse: []string{H}, set: []string{H + ": spec/schema/absent.json: does not compile as JSON Schema draft 2020-12 with nothing fetched"}},
		{name: "a relative reference outside the prefix", setup: func(t *testing.T, dir string) {
			plantSchema(t, dir, "away", `{"$schema":"`+draft+`","$id":"`+uri+`/away.json","$ref":"../0.2/away.json"}`)
		}, refuse: []string{H}, set: []string{H + ": spec/schema/away.json: does not compile as JSON Schema draft 2020-12 with nothing fetched"}},
	}
}

func TestSchemasFetchNothing(t *testing.T) {
	runCases(t, schemaFetchCases())

	// The loader itself: it serves the tree's schemas, and refuses a URL outside the prefix, or one
	// that climbs out of spec/schema/, without a request.
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"type":"string"}`))
	}))
	defer srv.Close()
	l := treeLoader{newTree(live)}
	if _, err := l.Load(schemaURL + "hash.json"); err != nil {
		t.Fatalf("the loader did not serve hash.json from the tree: %v", err)
	}
	for _, u := range []string{srv.URL + "/x.json", "https://example.com/x.json", schemaURL + "../0.2/x.json", schemaURL + "missing.json", "file:///etc/hostname"} {
		if _, err := l.Load(u); err == nil {
			t.Fatalf("the loader loaded %s", u)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the loader made %d requests", n)
	}
}
