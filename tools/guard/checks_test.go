package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The planted faults of the checks that read the vectors' generators, the format constants, the
// product words, the claims, the binary's main, the recipes' imports, the root module and the tools' versions. Each names
// the exact lines its rule prints, in order.
var checksCases = []tcase{
	// vectors-independent: a generator under vectors/<group>/ imports no core package its group
	// tests, as vectors/README.md's table gives them, and never the runner; its test files are
	// generators too.
	{name: "independent-imports", fixture: "independent-imports", refuse: []string{"vectors-independent"}, lines: []string{
		`vectors/canonical/gen/main.go:5: "github.com/orilloai/placket/core/canonical"`,
	}},
	{name: "independent-block", fixture: "independent-block", refuse: []string{"vectors-independent"}, lines: []string{
		`vectors/questions/gen/gen.go:8: m "github.com/orilloai/placket/core/mediator"`,
		`vectors/questions/gen/gen.go:9: "github.com/orilloai/placket/core/conformance"`,
		`vectors/questions/gen/gen.go:11: "github.com/orilloai/placket/core/receipts/internal/chain"`,
		`vectors/questions/gen/gen_test.go:3: import "github.com/orilloai/placket/core/mediator"`,
	}},
	// A generator outside every group's directory, or in a group the table does not list, is refused
	// whole.
	{name: "independent-placement", fixture: "independent-placement", refuse: []string{"vectors-independent"}, lines: []string{
		"vectors/elsewhere/gen.go: the group elsewhere has no row in vectors/README.md's table of the packages each group tests",
		"vectors/gen.go: a generator outside every group's directory; generators go under vectors/<group>/",
	}},
	// A symbolic link under vectors/ is refused: the check does not follow it. no-hidden-text refuses
	// every link outside testdata/.
	{name: "independent-symlink", setup: func(t *testing.T, dir string) {
		symlink(t, dir, "../core", "vectors/core")
	}, refuse: []string{"vectors-independent", "no-hidden-text"}, lines: []string{"vectors/core: a symbolic link"}},
	// A lower package, an import named only in a comment, and code under testdata/ pass.
	{name: "independent-allowed", fixture: "independent-allowed"},

	// constants-once: each placket.dev/ or vnd.placket literal is in one non-test file of core/ and
	// adapters/, named at its first line in each file that holds it; a second use in the same file, a
	// test file and a comment pass.
	{name: "constants-twice", fixture: "constants-twice", refuse: []string{"constants-once"}, lines: []string{
		`adapters/local/local.go:5: "application/vnd.placket.call+json" is also in core/b/b.go`,
		`core/a/a.go:5: "https://placket.dev/fixture/0.1" is also in core/b/b.go`,
		`core/b/b.go:5: "https://placket.dev/fixture/0.1" is also in core/a/a.go`,
		`core/b/b.go:8: "application/vnd.placket.call+json" is also in adapters/local/local.go`,
	}},

	// product-words-once: the core spells neither "ask" nor "released" as a whole literal, in either
	// quote form; "asks", a test file, a comment and the command line's own words file pass.
	{name: "words-in-core", fixture: "words-in-core", refuse: []string{"product-words-once"}, lines: []string{
		`core/w/w.go:5: released = "released"`,
		"core/w/w.go:7: ask      = `ask`",
	}},

	// claims-stop-at-crossing: no literal under adapters/ or cmd/ holds a word about use after
	// receipt, in any case or common ending, a raw string over two lines included; "retainer", an
	// identifier, a comment, a test file and the core pass.
	{name: "claims", fixture: "claims", refuse: []string{"claims-stop-at-crossing"}, lines: []string{
		`adapters/cli/words.go:6: "the receiver will not retain the part"`,
		`adapters/cli/words.go:8: "a line\nTraining on it is not prevented"`,
		`cmd/placket/main.go:3: "SHARED with consent"`,
	}},

	// main-is-thin: 40 lines is refused at the 40th; 39 pass; with no main.go the check passes (the
	// clean tree).
	{name: "main-40", fixture: "main-40", refuse: []string{"main-is-thin"}, lines: []string{"cmd/placket/main.go:40: main.go has 40 lines"}},
	{name: "main-39", fixture: "main-39"},

	// recipes-core-only: nothing under recipes/, test files included, imports adapters/ or cmd/, or
	// writes an import path with an escape; a core import, a longer name that only starts like
	// adapters, a comment, a string and a file under testdata/ pass. With no recipes/ the check passes
	// (the clean tree).
	{name: "recipes-adapters", fixture: "recipes-adapters", refuse: []string{"recipes-core-only"}, lines: []string{
		`recipes/job/job.go:5: "github.com/orilloai/placket/adapters/local"`,
		`recipes/job/job.go:6: cli "github.com/orilloai/placket/cmd/placket"`,
		`recipes/job/job.go:7: _ "github.com/orilloai/placket/adapters"`,
		`recipes/job/job_test.go:3: import "github.com/orilloai/placket/adapters/state"`,
	}},
	{name: "recipes-escaped", fixture: "recipes-escaped", refuse: []string{"recipes-core-only"}, lines: []string{
		`recipes/job/job.go:3: import _ "github.com/orilloai/placket/\x61dapters/cli"`,
	}},
	{name: "recipes-ok", fixture: "recipes-ok"},

	// root-module: a root go.mod requires the core and replaces it with ./core, and is the root
	// module; no go.work or go.work.sum is committed, one under testdata/ aside. With no root go.mod
	// the check passes (the clean tree).
	{name: "root-no-replace", fixture: "root-no-replace", refuse: []string{"root-module"}, lines: []string{
		"go.mod: no replace of github.com/orilloai/placket/core with ./core",
	}},
	{name: "root-other", fixture: "root-other", refuse: []string{"root-module"}, lines: []string{
		"go.mod:1: the module is github.com/orilloai/placket/adapters, not github.com/orilloai/placket",
		"go.mod:8: github.com/orilloai/placket/core is replaced with ../placket-core, not ./core",
	}},
	{name: "root-workspace", setup: func(t *testing.T, dir string) {
		write(t, dir, "go.work", "go 1.27\n\nuse ./core\n")
		write(t, dir, "go.work.sum", "")
	}, refuse: []string{"root-module"}, lines: []string{
		"go.work: a workspace file; none is committed",
		"go.work.sum: a workspace file; none is committed",
	}},
	{name: "root-ok", fixture: "root-ok", setup: func(t *testing.T, dir string) {
		write(t, dir, "tests/testdata/w/go.work", "go 1.27\n")
	}},

	// machine-rules-in-ci: every go run or go install of a tool names an exact version. In a script,
	// no version, a partial one and a branch are refused, and a local path, an exact version inside
	// parentheses, a pseudo-version, a comment and another command's "run" pass.
	{name: "tool-unpinned", fixture: "tool-unpinned", refuse: []string{"machine-rules-in-ci"}, lines: []string{
		"scripts/fetch.sh:3: golang.org/x/tools/cmd/stringer: not an exact version",
		"scripts/fetch.sh:4: golang.org/x/tools/cmd/stringer@v0.30: not an exact version",
		"scripts/fetch.sh:7: golang.org/x/vuln/cmd/govulncheck@master: not an exact version",
	}},
	// Every action a workflow uses is pinned to 40 lowercase hex: a tag, a short hash, a branch, an
	// image and upper-case hex are refused, under a quoted key or in a flow mapping too; a commit with
	// a comment after it, a local action, a comment and a reusable workflow at a commit pass.
	{name: "action-unpinned", fixture: "action-unpinned", refuse: []string{"machine-rules-in-ci"}, lines: []string{
		".github/workflows/extra.yml:8: actions/checkout@v4: not pinned to a commit",
		".github/workflows/extra.yml:10: actions/cache@0c45773: not pinned to a commit",
		".github/workflows/extra.yml:13: actions/upload-artifact@main: not pinned to a commit",
		".github/workflows/extra.yml:14: docker://alpine:3.20: not pinned to a commit",
		".github/workflows/extra.yml:15: Actions/Setup-Go@924AE3A1CDED613372AB5595356FB5720E22BA16: not pinned to a commit",
		".github/workflows/extra.yml:21: actions/checkout@v4: not pinned to a commit",
		".github/workflows/extra.yml:22: actions/cache@v4: not pinned to a commit",
		".github/workflows/extra.yml:23: actions/upload-artifact@v4: not pinned to a commit",
		".github/workflows/extra.yml:24: actions/setup-go@v5: not pinned to a commit",
	}},
}

func TestChecks(t *testing.T) { runCases(t, checksCases) }

// liveLines returns the lines of a live file.
func liveLines(t *testing.T, rel string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(live, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return splitLines(string(b))
}

// liveLine returns the number of the first line of a live file that holds the text.
func liveLine(t *testing.T, rel, text string) int {
	t.Helper()
	for i, l := range liveLines(t, rel) {
		if strings.Contains(l, text) {
			return i + 1
		}
	}
	t.Fatalf("no line of %s holds %q", rel, text)
	return 0
}

// ciLine returns the number of the line of the live Makefile where ci's prerequisites start.
func ciLine(t *testing.T) int {
	t.Helper()
	for i, l := range liveLines(t, "Makefile") {
		if strings.HasPrefix(l, "ci:") && !strings.Contains(l, "=") {
			return i + 1
		}
	}
	t.Fatalf("the Makefile has no ci target")
	return 0
}

// dropFromCI removes a step from ci's prerequisites in the copy's Makefile.
func dropFromCI(step string) func(*testing.T, string) {
	return func(t *testing.T, dir string) {
		edit(t, dir, "Makefile", func(s string) string {
			lines := strings.Split(s, "\n")
			in := false
			for i, l := range lines {
				if strings.HasPrefix(l, "ci:") && !strings.Contains(l, "=") {
					in = true
				}
				if !in {
					continue
				}
				words := strings.Fields(l)
				for j, w := range words {
					if w == step {
						words = append(words[:j], words[j+1:]...)
						break
					}
				}
				lead := ""
				if !strings.HasPrefix(l, "ci:") {
					lead = "\t"
				}
				lines[i] = lead + strings.Join(words, " ")
				if !strings.HasSuffix(l, `\`) {
					break
				}
			}
			return strings.Join(lines, "\n")
		})
	}
}

// The cases of machine-rules-in-ci that plant their faults in a copy of the live Makefile.
func ciCases(t *testing.T) []tcase {
	lint := liveLine(t, "Makefile", "go run $(STATICCHECK) ./...")
	ci := strconv.Itoa(ciLine(t))
	return []tcase{
		// A tool of the Makefile set to @latest is refused where it runs, its variable read.
		{name: "tool-latest", setup: func(t *testing.T, dir string) {
			edit(t, dir, "Makefile", func(s string) string {
				return strings.Replace(s, "staticcheck@v0.8.2", "staticcheck@latest", 1)
			})
		}, refuse: []string{"machine-rules-in-ci"}, lines: []string{
			"Makefile:" + strconv.Itoa(lint) + ": honnef.co/go/tools/cmd/staticcheck@latest: not an exact version",
		}},
		// A step, a rule of the checker or the shared test run left out of ci is refused, named.
		{name: "ci-without-secrets", setup: dropFromCI("secrets"), refuse: []string{"machine-rules-in-ci"}, lines: []string{
			"Makefile:" + ci + ": ci does not run secrets",
		}},
		{name: "ci-without-a-rule", setup: dropFromCI("ci-stays-fast"), refuse: []string{"machine-rules-in-ci"}, lines: []string{
			"Makefile:" + ci + ": ci does not run ci-stays-fast",
		}},
		{name: "ci-without-tests", setup: dropFromCI("tests"), refuse: []string{"machine-rules-in-ci"}, lines: []string{
			"Makefile:" + ci + ": ci does not run tests",
		}},
	}
}

func TestMakeCIRunsEverything(t *testing.T) { runCases(t, ciCases(t)) }

// ci-stays-fast: a script or a Go test outside testdata/ that clears the marker is refused at each
// line that clears it, in every form; one that sets it to a value or only reads it passes, and so
// does a comment and a fixture under testdata/. The lines the first case expects are in
// testdata/marker-cleared.want, since this file must not hold them itself.
func markerCases(t *testing.T) []tcase {
	return []tcase{
		{name: "marker-cleared", fixture: "marker-cleared", refuse: []string{"ci-stays-fast"}, lines: wantLines(t, "marker-cleared.want")},
		{name: "marker-kept", fixture: "marker-kept"},
	}
}

// wantLines reads the lines a case expects from a file under testdata/.
func wantLines(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading testdata/%s: %v", name, err)
	}
	return splitLines(string(b))
}

func TestMarkerCleared(t *testing.T) { runCases(t, markerCases(t)) }
