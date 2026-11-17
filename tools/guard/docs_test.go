package main

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The planted faults of the two checks that read the rules' documents: agents-md-short and
// no-hidden-text. They plant their faults in a copy of the live documents, so the lines they expect
// are computed from them.

const (
	agentsMD   = "AGENTS.md"
	rulesIndex = "docs/rules/README.md"
)

// lineOf returns the number of the first line of a live file that is exactly the text.
func lineOf(t *testing.T, rel, text string) int {
	t.Helper()
	for i, l := range liveLines(t, rel) {
		if l == text {
			return i + 1
		}
	}
	t.Fatalf("no line of %s is %q", rel, text)
	return 0
}

// section is a rule's section as the topic files hold it.
func section(name string) string {
	return "\n## `" + name + "`\n\nThe rule.\n\nWhy: a reason.\n\nChecked by: the reviewer.\n"
}

// dropLine removes the first line of a file of the copy that is exactly the text.
func dropLine(text string, rel string) func(*testing.T, string) {
	return func(t *testing.T, dir string) {
		edit(t, dir, rel, func(s string) string {
			lines := strings.Split(s, "\n")
			for i, l := range lines {
				if l == text {
					return strings.Join(append(lines[:i:i], lines[i+1:]...), "\n")
				}
			}
			t.Fatalf("setup: no line of %s is %q", rel, text)
			return s
		})
	}
}

// linksTo returns the line agents-md-short prints for each link in the live tree to a heading of
// docs/rules/ that is gone: a path ending rules/<file>#<anchor> in a Markdown, Go, shell, make or
// workflow file outside testdata/, and a link "](<file>#<anchor>)" from another topic file.
func linksTo(t *testing.T, file, anchor string) []string {
	t.Helper()
	ref := "docs/rules/" + file + "#" + anchor
	path := regexp.MustCompile(`(^|[^A-Za-z0-9_-])rules/` + regexp.QuoteMeta(file+"#"+anchor) + `($|[^A-Za-z0-9_-])`)
	local := "](" + file + "#" + anchor + ")"
	var out []string
	err := filepath.WalkDir(live, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		name := d.Name()
		if d.IsDir() || strings.HasSuffix(name, "_test.go") || !(name == "Makefile" || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, ".sh") || strings.HasSuffix(name, ".yml")) {
			return nil
		}
		rel, _ := filepath.Rel(live, p)
		rel = filepath.ToSlash(rel)
		inRules := filepath.ToSlash(filepath.Dir(rel)) == "docs/rules" && rel != rulesIndex
		for i, l := range liveLines(t, rel) {
			if path.MatchString(l) || (inRules && strings.Contains(l, local)) {
				out = append(out, "agents-md-short: "+rel+":"+strconv.Itoa(i+1)+": "+ref+" is no heading")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("nothing in the tree links %s", ref)
	}
	return out
}

func agentsCases(t *testing.T) []tcase {
	coreLines := len(liveLines(t, "docs/rules/core.md"))
	errorsLines := len(liveLines(t, "docs/rules/errors.md"))
	testsLines := len(liveLines(t, "docs/rules/tests.md"))
	n := func(i int) string { return strconv.Itoa(i) }
	world := lineOf(t, "docs/rules/core.md", "## `core-no-world`")
	worldIndex := lineOf(t, rulesIndex, "- [`core-no-world`](core.md#core-no-world)")
	evaluate := lineOf(t, rulesIndex, "- [`reserved-evaluate`](core.md#reserved-evaluate)")
	stays := lineOf(t, rulesIndex, "- [`ci-stays-fast`](changes.md#ci-stays-fast)")
	redGreen := lineOf(t, rulesIndex, "- [`red-green-tests`](tests.md#red-green-tests)")
	return []tcase{
		// The entry file over 150 lines is refused at its 151st; over 10 KiB, as a whole.
		{name: "agents-long", setup: func(t *testing.T, dir string) {
			write(t, dir, agentsMD, strings.Repeat("x\n", 151))
		}, refuse: []string{"agents-md-short"}, lines: []string{"AGENTS.md:151: 151 lines; the limit is 150"}},
		{name: "agents-big", setup: func(t *testing.T, dir string) {
			write(t, dir, agentsMD, strings.Repeat(strings.Repeat("y", 120)+"\n", 100))
		}, refuse: []string{"agents-md-short"}, lines: []string{"AGENTS.md: 12100 bytes; the limit is 10240"}},
		// 150 lines of 10240 bytes in all pass.
		{name: "agents-at-limits", setup: func(t *testing.T, dir string) {
			write(t, dir, agentsMD, strings.Repeat(strings.Repeat("z", 67)+"\n", 149)+strings.Repeat("z", 10240-68*149-1)+"\n")
		}},
		{name: "agents-missing", setup: func(t *testing.T, dir string) { remove(t, dir, agentsMD) },
			refuse: []string{"agents-md-short"}, lines: []string{"AGENTS.md: missing; it is the entry file"}},
		// A heading missing from the index; a name with two headings; an index line with no heading.
		{name: "rule-not-in-index", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "docs/rules/core.md", section("extra-rule"))
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/core.md:" + n(coreLines+2) + ": `extra-rule` is not in the index, docs/rules/README.md",
		}},
		{name: "rule-twice", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "docs/rules/errors.md", section("core-no-world"))
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/errors.md:" + n(errorsLines+2) + ": `core-no-world` has a second heading; the first is docs/rules/core.md:" + n(world),
		}},
		{name: "rule-no-heading", setup: dropLine("## `reserved-evaluate`", "docs/rules/core.md"),
			refuse: []string{"agents-md-short"}, lines: []string{
				"docs/rules/README.md:" + n(evaluate) + ": `reserved-evaluate` has no heading in docs/rules/",
			}},
		// The index links a rule to a file that does not hold its heading.
		{name: "index-wrong-file", setup: func(t *testing.T, dir string) {
			edit(t, dir, rulesIndex, func(s string) string {
				return strings.Replace(s, "(core.md#core-no-world)", "(errors.md#core-no-world)", 1)
			})
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/README.md:" + n(worldIndex) + ": `core-no-world` links errors.md; its heading is in core.md",
		}},
		// A rule's heading with no Why: line, and one with no Checked by: line.
		{name: "rule-no-why", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "docs/rules/tests.md", "\n## `bare-rule`\n\nThe rule.\n")
			edit(t, dir, rulesIndex, func(s string) string {
				return strings.Replace(s, "- [`red-green-tests`]", "- [`bare-rule`](tests.md#bare-rule)\n- [`red-green-tests`]", 1)
			})
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/tests.md:" + n(testsLines+2) + ": `bare-rule` has no Checked by: line",
			"docs/rules/tests.md:" + n(testsLines+2) + ": `bare-rule` has no Why: line",
		}},
		// A rule the checker prints, and red-green-tests, each with no heading: the index names
		// them, the checker's refusal links a heading that is not there, and so does every other
		// link to them.
		{name: "checker-rule-no-heading", setup: func(t *testing.T, dir string) {
			dropLine("## `ci-stays-fast`", "docs/rules/changes.md")(t, dir)
			dropLine("## `red-green-tests`", "docs/rules/tests.md")(t, dir)
		}, refuse: []string{"agents-md-short"}, set: append(append([]string{
			"agents-md-short: docs/rules/README.md:" + n(stays) + ": `ci-stays-fast` has no heading in docs/rules/",
			"agents-md-short: docs/rules/README.md:" + n(redGreen) + ": `red-green-tests` has no heading in docs/rules/",
			"agents-md-short: tools/guard/rules.go: the rule ci-stays-fast links docs/rules/changes.md#ci-stays-fast, which is no heading",
		}, linksTo(t, "changes.md", "ci-stays-fast")...), linksTo(t, "tests.md", "red-green-tests")...)},
		// A link to a rule's heading that does not resolve, anywhere in the tree; a G-number in
		// the rules.
		{name: "broken-links", setup: func(t *testing.T, dir string) {
			write(t, dir, "docs/notes.md", "# Notes\n\nSee docs/rules/core.md#no-such-rule.\nAnd [this](../docs/rules/gone.md#core-no-world).\n")
			appendTo(t, dir, "docs/rules/tests.md", "\nOnce G-12.\n")
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/notes.md:3: docs/rules/core.md#no-such-rule is no heading",
			"docs/notes.md:4: docs/rules/gone.md#core-no-world: no such file",
			"docs/rules/tests.md:" + n(testsLines+2) + ": names G-12; a rule is named by its public name",
		}},
		// A link to a heading that is not a rule's resolves too.
		{name: "link-to-section", setup: func(t *testing.T, dir string) {
			write(t, dir, "docs/notes.md", "See docs/rules/checks.md#what-make-ci-prints and docs/rules/adapters.md#product-words-once.\n")
		}},
		// A rule's heading is a whole backquoted name; a heading that only mentions one is not.
		{name: "rule-heading-shape", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "docs/rules/adapters.md", "\n## About `product-words-once`\n\nMore.\n")
		}},
	}
}

func TestAgentsMDShort(t *testing.T) { runCases(t, agentsCases(t)) }

// no-hidden-text: a bidi control character or an invisible one in a text file outside testdata/ is
// refused at its line, each named by its code point, a Go file and a Markdown file alike. Other
// characters outside ASCII, a fixture under testdata/ and a binary file pass.
var hiddenCases = []tcase{
	{name: "hidden-text", fixture: "hidden-text", refuse: []string{"no-hidden-text"}, lines: []string{
		"core/h/h.go:3: holds U+202E (bidi control), U+202C (bidi control)",
		"docs/hidden.md:3: holds U+200B (invisible)",
		"docs/hidden.md:5: holds U+2066 (bidi control), U+2069 (bidi control), U+FEFF (invisible)",
		"docs/hidden.md:6: holds U+E0041 (invisible)",
	}},
	// A byte-order mark at the very start of a file is refused too: it is invisible.
	{name: "hidden-bom", setup: func(t *testing.T, dir string) {
		write(t, dir, "scripts/bom.sh", "\ufeffecho hi\n")
	}, refuse: []string{"no-hidden-text"}, lines: []string{"scripts/bom.sh:1: holds U+FEFF (invisible)"}},
}

func TestNoHiddenText(t *testing.T) { runCases(t, hiddenCases) }
