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

// A rule is one check of the tree, named by its public name. doc is the topic file of docs/rules/
// that holds the rule's heading, which its closing "refused:" line links; why is the reason that line
// gives. ordered rules print their findings in the order found; the others print them
// by file, then line, then text, and print a line found twice once.
type rule struct {
	name    string
	doc     string
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
		{name: "core-no-deps", doc: "modules.md", why: "core/go.mod requires no module", check: coreNoDeps},
		{name: "core-imports", doc: "modules.md", why: "nothing under core/ imports adapters/, cmd/ or recipes/, and no import path is written with an escape", check: coreImports},
		{name: "core-no-world", doc: "core.md", why: "no non-test code in core/ reads the clock, randomness, the environment or files, or touches the network, the process or memory, or prints; the core is Go alone, without cgo or other sources, and no import path is written with an escape", check: coreNoWorld},
		{name: "core-no-panic", doc: "core.md", why: "no non-test code in core/ calls panic", check: coreNoPanic},
		{name: "specs-name-behaviour", doc: "core.md", why: "the requirement text names behaviour under the ids spec/ids.txt lists, each once with a scenario, and never Go code", ordered: true, check: specsNameBehaviour},
		{name: "schemas-hold", doc: "tests.md", why: "every schema compiles from the tree with nothing fetched, holds against its examples and has a row in the register", ordered: true, check: schemasHold},
		{name: "vectors-independent", doc: "tests.md", why: "no generator imports a core package its group tests, or the runner, and every generator is under its group's directory", check: vectorsIndependent},
		{name: "constants-once", doc: "core.md", why: "a format constant is defined in one file and imported everywhere else", check: constantsOnce},
		{name: "product-words-once", doc: "adapters.md", why: "the core never spells the outcome words the command line owns", check: productWordsOnce},
		{name: "claims-stop-at-crossing", doc: "adapters.md", why: "nothing placket prints speaks of what a receiver does with content after receipt", check: claimsStopAtCrossing},
		{name: "main-is-thin", doc: "adapters.md", why: "cmd/placket/main.go stays under 40 lines and every command is tested from adapters/cli", check: mainIsThin},
		{name: "root-module", doc: "modules.md", why: "the root module requires the core and replaces it with ./core, and no go.work is committed", check: rootModule},
		{name: "machine-rules-in-ci", doc: "changes.md", why: "every tool make ci runs is pinned to an exact version, and make ci runs every check and step", check: machineRulesInCI},
		{name: "ci-stays-fast", doc: "changes.md", why: "no test or script clears PLACKET_IN_CI, the marker that stops a make ci inside make ci", check: ciStaysFast},
		{name: "no-hidden-text", doc: "changes.md", why: "no text file outside testdata/ holds a bidi control character or an invisible one", check: noHiddenText},
		{name: "agents-md-short", doc: "changes.md", why: "AGENTS.md stays within 150 lines and 10 KiB, every public name has exactly one heading in docs/rules/ and a line in its index, and every link to a rule's heading resolves", check: agentsMDShort},
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
		lines = append(lines, r.name+": refused: "+r.why+" (docs/rules/"+r.doc+"#"+r.name+")")
	} else {
		lines = append(lines, r.name+": ok")
	}
	return lines, refused
}
