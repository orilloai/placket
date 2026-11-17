package main

import "testing"

var coreRules = []string{"core-no-deps", "core-imports", "core-no-world", "core-no-panic"}

// The planted faults of the core's checks. Each names the place of every line its rules must print,
// and nothing else; every other rule passes.
var coreCases = []tcase{
	// core-imports: nothing under core/, test files included, imports adapters, cmd or recipes.
	{name: "imports-adapters", fixture: "imports-adapters", refuse: []string{"core-imports"}, at: []string{"core/bad/bad.go:5"}},
	{name: "imports-cmd", fixture: "imports-cmd", refuse: []string{"core-imports"}, at: []string{"core/bad/bad.go:4"}},
	{name: "imports-recipes", fixture: "imports-recipes", refuse: []string{"core-imports"}, at: []string{"core/bad/bad.go:4"}},
	{name: "imports-in-test", fixture: "imports-in-test", refuse: []string{"core-imports"}, at: []string{"core/bad/bad_test.go:6"}},

	// core-no-deps: core/go.mod requires nothing, in either form.
	{name: "deps-require", fixture: "deps-require", refuse: []string{"core-no-deps"}, at: []string{"core/go.mod:7"}},
	{name: "deps-require-block", fixture: "deps-require-block", refuse: []string{"core-no-deps"}, at: []string{"core/go.mod:7"}},

	// core-no-world: each refused name, one case each, in a non-test file.
	{name: "world-time-now", fixture: "world-time-now", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:7"}},
	{name: "world-crypto-rand", fixture: "world-crypto-rand", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4"}},
	{name: "world-math-rand", fixture: "world-math-rand", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4"}},
	{name: "world-math-rand-v2", fixture: "world-math-rand-v2", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4"}},
	{name: "world-os-getenv", fixture: "world-os-getenv", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4", "core/world/world.go:7"}},
	{name: "world-os-open", fixture: "world-os-open", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4", "core/world/world.go:7"}},
	{name: "world-net", fixture: "world-net", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4"}},
	{name: "world-net-http", fixture: "world-net-http", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:4"}},
	{name: "world-fmt-print", fixture: "world-fmt-print", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:7"}},
	{name: "world-fmt-printf", fixture: "world-fmt-printf", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:7"}},
	{name: "world-fmt-println", fixture: "world-fmt-println", refuse: []string{"core-no-world"}, at: []string{"core/world/world.go:7"}},

	// core-no-world: the packages that reach the process, files, logs and memory, under any name, and
	// the other ways to read the clock or print.
	{name: "world-imports", fixture: "world-imports", refuse: []string{"core-no-world"}, at: []string{
		"core/world/world.go:5", "core/world/world.go:6", "core/world/world.go:7", "core/world/world.go:8",
		"core/world/world.go:9", "core/world/world.go:10", "core/world/world.go:11", "core/world/world.go:12",
		"core/world/world.go:13"}},
	{name: "world-calls", fixture: "world-calls", refuse: []string{"core-no-world"}, at: []string{
		"core/world/world.go:8", "core/world/world.go:9", "core/world/world.go:10", "core/world/world.go:11",
		"core/world/world.go:12", "core/world/world.go:13", "core/world/world.go:14", "core/world/world.go:15",
		"core/world/world.go:16", "core/world/world.go:17"}},

	// core-no-panic: a call in code; a call after a string that holds "//", which is not a comment.
	{name: "panic-call", fixture: "panic-call", refuse: []string{"core-no-panic"}, at: []string{"core/p/p.go:7"}},
	{name: "panic-after-slashes", fixture: "panic-after-slashes", refuse: []string{"core-no-panic"}, at: []string{"core/p/p.go:5"}},

	// core-no-world: cgo, and source files Go builds that are not Go (assembly, C, C++, headers,
	// objects), outside testdata/; a selector split after its dot, by a line break or a comment.
	{name: "world-cgo", fixture: "world-cgo", refuse: []string{"core-no-world"}, at: []string{"core/c/c.go:5", "core/c/d.go:4"}},
	{name: "world-non-go", fixture: "world-non-go", refuse: []string{"core-no-world"}, at: []string{
		"core/a/a_amd64.s", "core/a/b_arm64.S", "core/a/x.c", "core/a/x.h", "core/a/y.cc", "core/a/z.syso"}},
	{name: "world-split-selector", fixture: "world-split-selector", refuse: []string{"core-no-world"}, at: []string{
		"core/w/w.go:12", "core/w/w.go:16", "core/w/w.go:22"}},

	// An import path written with an escape is refused by both checks that read import paths,
	// whatever it spells.
	{name: "world-escaped-import", fixture: "world-escaped-import", refuse: []string{"core-imports", "core-no-world"}, at: []string{
		"core/e/e.go:4", "core/e/f.go:4"}},
	{name: "imports-escaped", fixture: "imports-escaped", refuse: []string{"core-imports", "core-no-world"}, at: []string{"core/bad/bad.go:5"}},

	// A symbolic link under core/, to a file, a directory or core/go.mod itself, is refused by every
	// check of the core, and by product-words-once, which reads the core too: the Go tool follows a
	// link, and a check that skipped it would let anything in. no-hidden-text refuses every link
	// outside testdata/.
	{name: "symlink-file", fixture: "symlink-file", setup: func(t *testing.T, dir string) {
		symlink(t, dir, "../../elsewhere/s.go", "core/s/s.go")
	}, refuse: append(coreRules, "product-words-once", "no-hidden-text"), at: []string{"core/s/s.go"}},
	{name: "symlink-dir", fixture: "symlink-dir", setup: func(t *testing.T, dir string) {
		symlink(t, dir, "../elsewhere", "core/d")
	}, refuse: append(coreRules, "product-words-once", "no-hidden-text"), at: []string{"core/d"}},
	{name: "symlink-gomod", fixture: "symlink-gomod", setup: func(t *testing.T, dir string) {
		remove(t, dir, "core/go.mod")
		symlink(t, dir, "../elsewhere/go.mod", "core/go.mod")
	}, refuse: append(coreRules, "product-words-once", "no-hidden-text"), at: []string{"core/go.mod"}},

	// A /* */ comment is not code, even when a line inside it starts with //; a raw string is not a
	// comment, even when a line inside it starts with //. Code after either is checked.
	{name: "block-comment-import", fixture: "block-comment-import", refuse: []string{"core-imports"}, at: []string{"core/bad/bad.go:6"}},
	{name: "block-comment-world", fixture: "block-comment-world", refuse: []string{"core-no-world"}, at: []string{"core/w/w.go:6", "core/w/w.go:15"}},
	{name: "block-comment-panic", fixture: "block-comment-panic", refuse: []string{"core-no-panic"}, at: []string{"core/p/p.go:7"}},
	{name: "raw-string-slashes", fixture: "raw-string-slashes", refuse: []string{"core-no-world"}, at: []string{"core/w/w.go:8"}},

	// The same names in a _test.go file, or only in // or /* */ comments, pass.
	{name: "world-in-test", fixture: "world-in-test"},
	// The test helpers under core/internal/testing/ may read the world; a file built into the core
	// may not import them.
	{name: "world-in-test-helpers", fixture: "world-in-test-helpers"},
	{name: "imports-test-helpers", fixture: "imports-test-helpers", refuse: []string{"core-no-world"}, at: []string{"core/x/x.go:4"}},
	{name: "comments-only", fixture: "comments-only"},
	{name: "block-comments-only", fixture: "block-comments-only"},

	// What core-no-world allows in a non-test file: fmt.Fprint* and fmt.Sprint*, a call of a field
	// named print, "C" and escapes in a string that is not an import path, assembly under testdata/,
	// and a type of package time whose name starts like a refused call (time.Ticker).
	{name: "world-allowed", fixture: "world-allowed"},
}

// What the parser sees that the line scans did not.
var parserCases = []tcase{
	// time and fmt imported under other names: each reference to a refused name is refused at its
	// line, called or not; their other names pass.
	{name: "world-renamed", fixture: "world-renamed", refuse: []string{"core-no-world"}, lines: []string{
		"core/r/r.go:10: func At() clock.Time { return clock.Now() }",
		"core/r/r.go:13: var Later = clock.Since",
		`core/r/r.go:16: func Hello() { say.Println("hello") }`,
	}},
	// A dot import of time or fmt is refused at its line.
	{name: "world-dot", fixture: "world-dot", refuse: []string{"core-no-world"}, lines: []string{
		`core/d/d.go:5: . "fmt"`,
		`core/d/d.go:6: . "time"`,
		`core/d/e.go:3: import . "time"`,
	}},
	// "panic(" inside a string, the one string the line scan refused before, passes now.
	{name: "panic-in-string", fixture: "panic-in-string"},
	// "os", "net/http", an adapters path and the refused calls, inside strings that are not imports,
	// pass.
	{name: "strings-not-imports", fixture: "strings-not-imports"},
	// A core function of its own named print, and its call, pass: the call is not the built-in.
	{name: "own-print", fixture: "own-print"},
	// A file built only for another system is read all the same.
	{name: "world-other-system", fixture: "world-other-system", refuse: []string{"core-no-world"}, lines: []string{
		"core/w/w_windows.go:9: func At() time.Time { return time.Now() }",
	}},
	// A Go file that does not parse is refused by every rule that reads it, at the line of the error.
	{name: "unparsed", fixture: "unparsed", refuse: []string{"core-imports", "core-no-world", "core-no-panic", "product-words-once", "constants-once"}, set: []string{
		"core-imports: core/u/u.go:4: does not parse: expected ')', found '{'",
		"core-no-world: core/u/u.go:4: does not parse: expected ')', found '{'",
		"core-no-panic: core/u/u.go:4: does not parse: expected ')', found '{'",
		"product-words-once: core/u/u.go:4: does not parse: expected ')', found '{'",
		"constants-once: core/u/u.go:4: does not parse: expected ')', found '{'",
	}},
}

// A call of print, println or panic reaches a name of the package's own only when a file of the same
// package clause that every build includes declares it. Otherwise the call is the built-in's, in some
// build, and is refused.
var shadowCases = []tcase{
	// A name declared only in a file some build leaves out does not stand in for the built-in: the
	// call is refused. Here the file is under a build line no build sets.
	{name: "shadow-panic-constrained", fixture: "shadow-panic-constrained", refuse: []string{"core-no-panic"}, lines: []string{
		`core/zz/zz.go:5: func Must() { panic("empty") }`,
	}},
	// Here the file is built only on one system, named by its suffix.
	{name: "shadow-println-constrained", fixture: "shadow-println-constrained", refuse: []string{"core-no-world"}, lines: []string{
		`core/zz/zz.go:5: func Hello() { println("hi") }`,
	}},
	// And a file the Go tool ignores by its name, one built for one architecture, one under an old
	// build line, and one of another package clause.
	{name: "shadow-elsewhere", fixture: "shadow-elsewhere", refuse: []string{"core-no-world", "core-no-panic"}, set: []string{
		`core-no-world: core/a/a.go:5: func Say() { print("a") }`,
		`core-no-panic: core/b/b.go:5: func Must() { panic("b") }`,
		`core-no-world: core/c/c.go:5: func Hello() { println("c") }`,
		`core-no-world: core/e/e.go:5: func Say() { print("e") }`,
	}},
}

func TestCoreChecks(t *testing.T) { runCases(t, coreCases) }

func TestParserSees(t *testing.T) { runCases(t, parserCases) }

func TestShadowedBuiltins(t *testing.T) { runCases(t, shadowCases) }
