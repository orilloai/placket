// Command redgreen is the check red-green-tests (AGENTS.md; docs/adr/0013-red-green-tests.md): every
// test a change adds or changes fails without the change's code and passes with it, and no test is
// removed or skipped unexplained.
//
// It reads two commits of a repository, a base B (the merge base with main) and a head H, and the
// trailers of the commits between them; nothing else of the branch's history. It finds the tests the
// change touches by parsing the Go of both, runs each added and changed test in a scratch worktree of
// B with H's test files laid over it, then in one of H, and prints one line per test and a last line:
//
//	red-green-tests: ok, <n> red on base, <n> green at head, <n> reported
//	red-green-tests: refused: <why> (AGENTS.md, red-green-tests)
//
// It exits 0 when ok, 1 when refused and 3 when it could not judge (a bad argument, no go or git, a
// base that is not an ancestor of the head). A tool error is never a pass.
//
// Usage: redgreen [-C dir] <base> [<head>]; head is HEAD by default.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
)

const (
	name     = "red-green-tests"
	why      = "every test the change adds or changes fails without its code and passes with it, and none is removed or skipped unexplained (AGENTS.md, red-green-tests)"
	usage    = "usage: redgreen [-C dir] <base> [<head>]"
	exitOK   = 0
	exitNo   = 1
	exitTool = 3
)

// The three trailers, as a commit spells them; git reads a trailer's key without regard to case.
const (
	greenOnBase  = "Green-On-Base"
	testWeakened = "Test-Weakened"
	noTest       = "No-Test"
)

// codeDirs are the directories whose non-test Go code a change may not alter with no test added or
// changed, unless a No-Test trailer gives the reason.
var codeDirs = []string{"core/", "adapters/", "cmd/", "recipes/", "tools/"}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

// run judges the change its arguments name, prints the verdict to out and returns the exit status.
func run(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("redgreen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("C", ".", "the repository")
	if err := fs.Parse(args); err != nil || fs.NArg() < 1 || fs.NArg() > 2 {
		fmt.Fprintf(out, "%s: error: %s\n", name, usage)
		return exitTool
	}
	head := "HEAD"
	if fs.NArg() == 2 {
		head = fs.Arg(1)
	}
	lines, refused, err := judge(git{dir: *dir}, fs.Arg(0), head)
	if err != nil {
		fmt.Fprintf(out, "%s: error: %v\n", name, err)
		return exitTool
	}
	for _, l := range lines {
		fmt.Fprintf(out, "%s: %s\n", name, l)
	}
	if refused {
		return exitNo
	}
	return exitOK
}

// An item is one line of the verdict, before the last.
type item struct {
	file   string
	line   int
	text   string
	refuse bool
}

func (i item) String() string {
	switch {
	case i.file == "":
		return i.text
	case i.line > 0:
		return fmt.Sprintf("%s:%d: %s", i.file, i.line, i.text)
	}
	return i.file + ": " + i.text
}

// An excuse is a trailer's reason, by the name it gives.
type excuse struct{ commit, why string }

// A touched test is a Go test or a shell test the change adds, changes or removes.
type touched struct {
	file string
	line int
	name string // the Go test's name, or the shell test's path
	key  string // the Go test's key; empty for a shell test
	kind string // "added", "changed", "removed" or "skipped"
	run  bool   // whether it is run at the base and the head
	tag  bool   // a Go test behind a build tag, not run
}

// judge compares the change from base to head and returns the verdict's lines, last line included.
func judge(g git, baseRev, headRev string) ([]string, bool, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, false, errors.New("no git on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		return nil, false, errors.New("no go on PATH")
	}
	base, err := g.commit(baseRev)
	if err != nil {
		return nil, false, err
	}
	head, err := g.commit(headRev)
	if err != nil {
		return nil, false, err
	}
	if ok, err := g.isAncestor(base, head); err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, fmt.Errorf("%s is not an ancestor of %s", baseRev, headRev)
	}
	baseTree, err := g.tree(base)
	if err != nil {
		return nil, false, err
	}
	headTree, err := g.tree(head)
	if err != nil {
		return nil, false, err
	}

	// The paths the change touches, and the package directories whose tests to compare.
	var changed []string
	for p, e := range headTree {
		if b, ok := baseTree[p]; !ok || b != e {
			changed = append(changed, p)
		}
	}
	for p := range baseTree {
		if _, ok := headTree[p]; !ok {
			changed = append(changed, p)
		}
	}
	changed = sortedStrings(changed)
	dirs := map[string]bool{}
	for _, p := range changed {
		if isGoTestFile(p) {
			dirs[path.Dir(p)] = true
		}
	}
	bt, err := findTests(g, baseTree, dirs)
	if err != nil {
		return nil, false, err
	}
	ht, err := findTests(g, headTree, dirs)
	if err != nil {
		return nil, false, err
	}

	var items, tail []item
	for _, f := range bt.broken {
		items = append(items, item{file: f, text: "does not parse at base", refuse: true})
	}
	for _, f := range ht.broken {
		items = append(items, item{file: f, text: "does not parse at head", refuse: true})
	}

	// The tests the change touches.
	var ts []touched
	for _, k := range sortedKeys(ht.tests) {
		h := ht.tests[k]
		b, ok := bt.tests[k]
		t := touched{file: h.file, line: h.line, name: h.name, key: k}
		switch {
		case !ok:
			t.kind = "added"
		case h.skips > b.skips:
			t.kind = "skipped"
		case h.body != b.body:
			t.kind = "changed"
		default:
			continue
		}
		t.tag = !h.built && t.kind != "skipped"
		t.run = h.built && t.kind != "skipped"
		ts = append(ts, t)
	}
	for _, k := range sortedKeys(bt.tests) {
		if _, ok := ht.tests[k]; !ok {
			b := bt.tests[k]
			ts = append(ts, touched{file: b.file, line: b.line, name: b.name, key: k, kind: "removed"})
		}
	}
	for _, p := range changed {
		if !isShellTest(p) {
			continue
		}
		_, inBase := baseTree[p]
		_, inHead := headTree[p]
		t := touched{file: p, name: p, kind: "changed", run: true}
		switch {
		case !inBase:
			t.kind = "added"
		case !inHead:
			t.kind, t.run = "removed", false
		}
		ts = append(ts, t)
	}
	names := map[string]bool{}
	tested := false
	for _, t := range ts {
		names[t.name] = true
		if t.kind != "removed" {
			tested = true
		}
	}

	// The trailers: each names what it excuses and why.
	trailers, err := g.trailers(base, head)
	if err != nil {
		return nil, false, err
	}
	greens, weakened := map[string]excuse{}, map[string]excuse{}
	var noTests []excuse
	for _, tr := range trailers {
		switch {
		case strings.EqualFold(tr.key, greenOnBase), strings.EqualFold(tr.key, testWeakened):
			key, into := greenOnBase, greens
			if strings.EqualFold(tr.key, testWeakened) {
				key, into = testWeakened, weakened
			}
			n, reason, _ := strings.Cut(tr.value, ":")
			n, reason = strings.TrimSpace(n), strings.TrimSpace(reason)
			switch {
			case n == "" || reason == "":
				tail = append(tail, item{text: tr.commit + ": " + key + ": " + tr.value + ": trailer gives no test and reason", refuse: true})
			case !names[n]:
				tail = append(tail, item{text: tr.commit + ": " + key + ": " + n + ": trailer names an untouched test", refuse: true})
			default:
				if _, seen := into[n]; !seen {
					into[n] = excuse{commit: tr.commit, why: reason}
				}
			}
		case strings.EqualFold(tr.key, noTest):
			if tr.value == "" {
				tail = append(tail, item{text: tr.commit + ": " + noTest + ": trailer gives no reason", refuse: true})
			} else {
				noTests = append(noTests, excuse{commit: tr.commit, why: tr.value})
			}
		}
	}

	// Code changed with no test added or changed.
	var code []string
	for _, p := range changed {
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") && !isTestPath(p) && underCode(p) {
			code = append(code, p)
		}
	}
	excusedCode := false
	if len(code) > 0 && !tested {
		if len(noTests) > 0 {
			excusedCode = true
			tail = append(tail, item{text: "no test added or changed, excused (" + noTest + ": " + noTests[0].why + ")"})
		} else {
			for _, p := range code {
				tail = append(tail, item{file: p, text: "code changed, and no test added or changed", refuse: true})
			}
		}
	}

	// Helpers: test files the base has that changed outside any test function, or went, reported for
	// the reviewer. A new helper or fixture serves the new tests, which run anyway.
	var helpers []string
	for _, p := range changed {
		if _, ok := baseTree[p]; !ok {
			continue
		}
		switch {
		case isGoTestFile(p):
			if bt.helpers[p] != ht.helpers[p] {
				helpers = append(helpers, p)
			}
		case isTestPath(p) && !isShellTest(p):
			helpers = append(helpers, p)
		}
	}

	// Run each added and changed test at the base, with the head's tests, and then at the head.
	var goRun []goTest
	var shRun []string
	for _, t := range ts {
		if !t.run {
			continue
		}
		if t.key == "" {
			shRun = append(shRun, t.name)
		} else {
			goRun = append(goRun, ht.tests[t.key])
		}
	}
	var atBase, atHead map[string]outcome
	if len(goRun)+len(shRun) > 0 {
		if atBase, err = runAt(g, base, head, baseTree, headTree, goRun, shRun, true); err != nil {
			return nil, false, err
		}
		if atHead, err = runAt(g, head, head, baseTree, headTree, goRun, shRun, false); err != nil {
			return nil, false, err
		}
	}

	red, green, reported := 0, 0, 0
	if excusedCode {
		reported++
	}
	for _, t := range ts {
		id := t.key
		if id == "" {
			id = t.name
		}
		it := item{file: t.file, line: t.line}
		label := t.name + ": "
		if t.key == "" {
			label = ""
		}
		switch {
		case t.kind == "removed" || t.kind == "skipped":
			it.text = label + t.kind
			if e, ok := weakened[t.name]; ok {
				it.text += ", excused (" + testWeakened + ": " + e.why + ")"
				reported++
			} else {
				it.refuse = true
			}
			items = append(items, it)
		case t.tag:
			it.text = label + "not run: build tag"
			reported++
			items = append(items, it)
		default:
			b := atBase[id]
			it.text = label + b.base()
			switch {
			case b != passed:
				red++
			case t.kind == "changed":
				it.text += ", changed: reported"
				reported++
			default:
				if e, ok := greens[t.name]; ok {
					it.text += ", excused (" + greenOnBase + ": " + e.why + ")"
					reported++
				} else {
					it.refuse = true
				}
			}
			items = append(items, it)
			if h := atHead[id]; h == passed {
				green++
			} else {
				items = append(items, item{file: t.file, line: t.line, text: label + h.head(), refuse: true})
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].file != items[j].file {
			return items[i].file < items[j].file
		}
		return items[i].line < items[j].line
	})
	if len(helpers) > 0 {
		items = append(items, item{text: "helpers changed: " + strings.Join(helpers, ", ")})
	}
	items = append(items, tail...)

	var lines []string
	refused := false
	for _, it := range items {
		lines = append(lines, it.String())
		refused = refused || it.refuse
	}
	if refused {
		lines = append(lines, "refused: "+why)
	} else {
		lines = append(lines, fmt.Sprintf("ok, %d red on base, %d green at head, %d reported", red, green, reported))
	}
	return lines, refused, nil
}

// underCode reports whether a path is under one of codeDirs.
func underCode(p string) bool {
	for _, d := range codeDirs {
		if strings.HasPrefix(p, d) {
			return true
		}
	}
	return false
}

// runAt runs the tests given in a scratch worktree of rev, with the head's test files laid over it
// when overlay is set, and returns each one's outcome, by the Go test's key or the shell test's path.
func runAt(g git, rev, head string, baseTree, headTree map[string]entry, goRun []goTest, shRun []string, overlay bool) (map[string]outcome, error) {
	w, err := addWorktree(g, rev)
	if err != nil {
		return nil, err
	}
	defer w.remove()
	if overlay {
		if err := w.layTests(head, baseTree, headTree); err != nil {
			return nil, err
		}
	}
	res := map[string]outcome{}
	if len(goRun) > 0 {
		if res, err = w.runGo(goRun); err != nil {
			return nil, err
		}
	}
	for _, p := range shRun {
		if res[p], err = w.runShell(p); err != nil {
			return nil, err
		}
	}
	return res, nil
}
