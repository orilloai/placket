package main

import (
	"strconv"
	"strings"
	"testing"
)

// The planted faults of docs-tree-holds. The tree of the live copy gains entries at its end, in its
// last section, and the fixture docs-tree-faults gains a page the tree lists and one it does not.
const docsTree = "docs/README.md"

// treeFaults are the entries the faults case adds to the tree: a page listed, then one fault of each
// kind a tree entry can hold.
const treeFaults = `- [Faults](guides/faults.md)
- [Missing](guides/missing.md)
- [Away](../patterns/README.md)
- [Web](https://placket.tech/docs)
- [Anchor](guides/approve.md#no-such-heading)
- Not a link
`

func docsTreeCases(t *testing.T) []tcase {
	n := len(liveLines(t, docsTree))
	tree := func(i int, text string) string { return docsTree + ":" + strconv.Itoa(n+i) + ": " + text }
	concepts := lineOf(t, docsTree, "## Concepts")
	page := "docs/guides/faults.md"
	return []tcase{
		{name: "docs-tree-faults", fixture: "docs-tree-faults", setup: func(t *testing.T, dir string) {
			edit(t, dir, docsTree, func(s string) string { return s + treeFaults })
		}, refuse: []string{"docs-tree-holds"}, lines: []string{
			tree(2, "guides/missing.md: no such page"),
			tree(3, "../patterns/README.md: a tree link leaves docs/ only for spec/ and vectors/"),
			tree(4, "https://placket.tech/docs: a tree link is relative and names a file"),
			tree(5, "guides/approve.md#no-such-heading: #no-such-heading is no heading of docs/guides/approve.md"),
			tree(6, `not a page of a section: a tree entry is "- [<title>](<path>)" under a section`),
			page + ":3: approve.md#no-such-heading: #no-such-heading is no heading of docs/guides/approve.md",
			page + ":3: nofile.md: no such file",
			page + ":6: `placket nosuchcommand --json`: nosuchcommand is no command of docs/reference/cli.md",
			page + ":7: `placket give --no-such-flag x`: --no-such-flag is no flag of placket give in docs/reference/cli.md",
			page + ":8: `placket init nosuchdoor`: nosuchdoor is none of the values of placket init's argument in docs/reference/cli.md: approve, merge, handoff",
			page + ":11: `placket verify --receipts journal --accord ID --founding founding.json --no-such-flag`: --no-such-flag is no flag of placket verify in docs/reference/cli.md",
			page + ":15: nofile2.md: no such file",
			"docs/guides/unlisted.md: not in the docs tree, docs/README.md",
		}},
		// A section out of its place, and the tree with none.
		{name: "docs-tree-sections", setup: func(t *testing.T, dir string) {
			edit(t, dir, docsTree, func(s string) string { return strings.Replace(s, "\n## Concepts\n", "\n## Ideas\n", 1) })
		}, refuse: []string{"docs-tree-holds"}, lines: []string{
			docsTree + ":" + strconv.Itoa(concepts) + `: the section "Ideas" is not the tree's next; its sections are Quickstarts, Concepts, Guides, Reference, Verify, Security, in order`,
		}},
		{name: "docs-tree-missing", setup: func(t *testing.T, dir string) { remove(t, dir, docsTree) },
			refuse: []string{"docs-tree-holds"}, lines: []string{docsTree + ": missing; it is the docs tree"}},
		// A page of the tree whose command lines name what the CLI reference no longer holds.
		{name: "docs-tree-stale-reference", setup: func(t *testing.T, dir string) {
			edit(t, dir, "docs/reference/cli.md", func(s string) string {
				return strings.Replace(s, "`approve`, `merge`, `handoff`", "`approve`, `handoff`", 1)
			})
		}, refuse: []string{"docs-tree-holds"}, at: initMergeLines(t)},
	}
}

// initMergeLines are the places of every placket init merge line of the pages the live tree lists.
func initMergeLines(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, rel := range []string{"docs/guides/merge.md", "docs/quickstarts/merge.md"} {
		for i, l := range liveLines(t, rel) {
			if strings.HasPrefix(strings.TrimSpace(l), "placket init merge") {
				out = append(out, rel+":"+strconv.Itoa(i+1))
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no page the tree lists runs placket init merge")
	}
	return out
}

// TestDocsTreeHolds plants one fault of each kind docs-tree-holds refuses, each named by its file
// and line.
func TestDocsTreeHolds(t *testing.T) {
	t.Parallel()
	runCases(t, docsTreeCases(t))
}

// A shell's words, up to the first operator, quotes read and substitutions kept as written.
func TestShellWords(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`placket give --text "a b" --to 'x y'`:                       "placket|give|--text|a b|--to|x y",
		`placket bind --actor "$(cat "$a.ref")" --role "$a"`:         `placket|bind|--actor|$(cat "$a.ref")|--role|$a`,
		`placket key new --out k 2>k.err`:                            "placket|key|new|--out|k",
		`placket key new --out k >k.ref`:                             "placket|key|new|--out|k",
		`placket version | jq .`:                                     "placket|version",
		`placket version && placket card --default`:                  "placket|version",
		`placket verify $(for o in *; do printf ' --open %s'; done)`: `placket|verify|$(for o in *; do printf ' --open %s'; done)`,
		`placket window # a comment`:                                 "placket|window",
	} {
		if got := strings.Join(shellWords(in), "|"); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
