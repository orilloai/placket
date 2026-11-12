package main

import (
	"sort"
	"strconv"
	"strings"
)

// A finding is one line a rule prints about the tree: "<file>:<line>: <text>", or "<file>: <text>"
// when line is 0, with each detail printed under it, indented. A note is printed but refuses nothing
// (schemas-hold's count of each schema's examples).
type finding struct {
	file    string
	line    int
	text    string
	details []string
	note    bool
}

func (f finding) place() string {
	if f.line > 0 {
		return f.file + ":" + strconv.Itoa(f.line)
	}
	return f.file
}

// A rule is one check of the tree, named as AGENTS.md names it. why is the reason its closing
// "refused:" line gives. ordered rules print their findings in the order found; the others print them
// by file, then line, then text, and print a line found twice once.
type rule struct {
	name    string
	why     string
	ordered bool
	check   func(*tree) []finding
}

// rules is the checker's rule table: every check of make ci that reads the tree. machine-rules-in-ci
// refuses a Makefile whose ci target does not name each of them. It is set in init, since that rule
// reads the table itself.
var rules []rule

func init() {
	rules = []rule{
		{name: "core-no-deps", why: "core/go.mod requires no module", check: coreNoDeps},
		{name: "core-imports", why: "nothing under core/ imports adapters/, cmd/ or recipes/, and no import path is written with an escape", check: coreImports},
		{name: "core-no-world", why: "no non-test code in core/ reads the clock, randomness, the environment or files, or touches the network, the process or memory, or prints; the core is Go alone, without cgo or other sources, and no import path is written with an escape", check: coreNoWorld},
		{name: "core-no-panic", why: "no non-test code in core/ calls panic", check: coreNoPanic},
		{name: "product-words-once", why: "the core never spells the outcome words the command line owns", check: productWordsOnce},
	}
}

// toolchainSteps are the steps of make ci that the checker does not own, each a target of the
// Makefile that ci must name, with modules, the checker's build (guard), the shared test run (tests),
// the conformance runner and the shell tests.
var toolchainSteps = []string{"vet", "lint", "vulnerabilities", "secrets", "race", "fuzz-seeds", "coverage-reported"}

var otherSteps = []string{"modules", "guard", "tests", "runner-no-silent-pass", "scripts"}

func ruleByName(n string) (rule, bool) {
	for _, r := range rules {
		if r.name == n {
			return r, true
		}
	}
	return rule{}, false
}

func ruleNames() string {
	var s []string
	for _, r := range rules {
		s = append(s, r.name)
	}
	return strings.Join(s, " ")
}

// run checks the tree and returns the rule's lines and whether it refused.
func (r rule) run(t *tree) ([]string, bool) {
	fs := r.check(t)
	type out struct {
		f    finding
		line string
	}
	var outs []out
	refused := false
	for _, f := range fs {
		outs = append(outs, out{f, r.name + ": " + f.place() + ": " + f.text})
		if !f.note {
			refused = true
		}
	}
	if !r.ordered {
		sort.SliceStable(outs, func(i, j int) bool {
			a, b := outs[i], outs[j]
			if a.f.file != b.f.file {
				return a.f.file < b.f.file
			}
			if a.f.line != b.f.line {
				return a.f.line < b.f.line
			}
			return a.line < b.line
		})
	}
	var lines []string
	seen := map[string]bool{}
	for _, o := range outs {
		if !r.ordered && seen[o.line] && len(o.f.details) == 0 {
			continue
		}
		seen[o.line] = true
		lines = append(lines, o.line)
		for _, d := range o.f.details {
			lines = append(lines, "    "+d)
		}
	}
	if refused {
		lines = append(lines, r.name+": refused: "+r.why+" (AGENTS.md, "+r.name+")")
	} else {
		lines = append(lines, r.name+": ok")
	}
	return lines, refused
}
