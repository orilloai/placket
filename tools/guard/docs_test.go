package main

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	indexLines := len(liveLines(t, rulesIndex))
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
		// One byte over 10 KiB, within 150 lines, is refused.
		{name: "agents-byte-over", setup: func(t *testing.T, dir string) {
			write(t, dir, agentsMD, strings.Repeat(strings.Repeat("z", 67)+"\n", 149)+strings.Repeat("z", 10241-68*149-1)+"\n")
		}, refuse: []string{"agents-md-short"}, lines: []string{"AGENTS.md: 10241 bytes; the limit is 10240"}},
		// An entry file that is a symbolic link is no entry file; no-hidden-text refuses the link too.
		{name: "agents-link", setup: func(t *testing.T, dir string) {
			remove(t, dir, agentsMD)
			symlink(t, dir, "README.md", agentsMD)
		}, refuse: []string{"agents-md-short", "no-hidden-text"}, set: []string{
			"agents-md-short: AGENTS.md: missing; it is the entry file",
			"no-hidden-text: AGENTS.md: a symbolic link",
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
		// A rule with no Why: and no Checked by: line in the middle of a file: the next heading closes it.
		{name: "rule-no-why-mid-file", setup: func(t *testing.T, dir string) {
			edit(t, dir, "docs/rules/core.md", func(s string) string {
				return strings.Replace(s, "## `core-no-world`\n", "## `bare-rule`\n\nThe rule.\n\n## `core-no-world`\n", 1)
			})
			edit(t, dir, rulesIndex, func(s string) string {
				return strings.Replace(s, "- [`core-no-world`]", "- [`bare-rule`](core.md#bare-rule)\n- [`core-no-world`]", 1)
			})
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/core.md:" + n(world) + ": `bare-rule` has no Checked by: line",
			"docs/rules/core.md:" + n(world) + ": `bare-rule` has no Why: line",
		}},
		// A name listed twice in the index; an index line whose anchor is not its name.
		{name: "index-twice", setup: func(t *testing.T, dir string) {
			edit(t, dir, rulesIndex, func(s string) string {
				l := "- [`core-no-world`](core.md#core-no-world)\n"
				return strings.Replace(s, l, l+l, 1)
			})
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/README.md:" + n(worldIndex+1) + ": `core-no-world` is listed twice",
		}},
		{name: "index-wrong-anchor", setup: func(t *testing.T, dir string) {
			edit(t, dir, rulesIndex, func(s string) string {
				return strings.Replace(s, "(core.md#core-no-world)", "(core.md#core-imports)", 1)
			})
		}, refuse: []string{"agents-md-short"}, lines: []string{
			"docs/rules/README.md:" + n(worldIndex) + ": `core-no-world` links the anchor core-imports, not core-no-world",
		}},
		// A link from one topic file to a heading of another that is not there; a G-number in the
		// index.
		{name: "topic-link-and-index-g", setup: func(t *testing.T, dir string) {
			appendTo(t, dir, "docs/rules/errors.md", "\nSee [gone](core.md#gone-rule).\n")
			appendTo(t, dir, rulesIndex, "\nOnce G-7.\n")
		}, refuse: []string{"agents-md-short"}, set: []string{
			"agents-md-short: docs/rules/README.md:" + n(indexLines+2) + ": names G-7; a rule is named by its public name",
			"agents-md-short: docs/rules/errors.md:" + n(errorsLines+2) + ": docs/rules/core.md#gone-rule is no heading",
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
			write(t, dir, ".github/workflows/w.yml", "# docs/rules/core.md#gone-yml\n")
			write(t, dir, ".github/w.yaml", "# docs/rules/core.md#gone-yaml\n")
			write(t, dir, "scripts/s.sh", "# docs/rules/core.md#gone-sh\n")
			write(t, dir, "scripts/s.mk", "# docs/rules/core.md#gone-mk\n")
		}, refuse: []string{"agents-md-short"}, lines: []string{
			".github/w.yaml:1: docs/rules/core.md#gone-yaml is no heading",
			".github/workflows/w.yml:1: docs/rules/core.md#gone-yml is no heading",
			"docs/notes.md:3: docs/rules/core.md#no-such-rule is no heading",
			"docs/notes.md:4: docs/rules/gone.md#core-no-world: no such file",
			"docs/rules/tests.md:" + n(testsLines+2) + ": names G-12; a rule is named by its public name",
			"scripts/s.mk:1: docs/rules/core.md#gone-mk is no heading",
			"scripts/s.sh:1: docs/rules/core.md#gone-sh is no heading",
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
// characters outside ASCII and a fixture under testdata/ pass. A NUL byte is refused wherever it is
// in a file, and the rest of the file is still read.
var hiddenCases = []tcase{
	{name: "hidden-text", fixture: "hidden-text", refuse: []string{"no-hidden-text"}, lines: []string{
		"core/h/h.go:3: holds U+202E (bidi control), U+202C (bidi control)",
		"docs/blob.bin:1: holds U+0000 (NUL), so it is not text; the rest of the file is not read",
		"docs/hidden.md:3: holds U+200B (invisible)",
		"docs/hidden.md:5: holds U+2066 (bidi control), U+2069 (bidi control), U+FEFF (invisible)",
		"docs/hidden.md:6: holds U+E0041 (invisible)",
	}},
	// A byte-order mark at the very start of a file is refused too: it is invisible.
	{name: "hidden-bom", setup: func(t *testing.T, dir string) {
		write(t, dir, "scripts/bom.sh", "\ufeffecho hi\n")
	}, refuse: []string{"no-hidden-text"}, lines: []string{"scripts/bom.sh:1: holds U+FEFF (invisible)"}},
	// A byte that is not valid UTF-8 does not hide the rest of the file.
	{name: "hidden-invalid-utf8", setup: func(t *testing.T, dir string) {
		write(t, dir, "docs/stray.md", "ok \xff\n\u202e here\n")
	}, refuse: []string{"no-hidden-text"}, lines: []string{"docs/stray.md:2: holds U+202E (bidi control)"}},
	// Variation selectors, and format characters beyond the named ones, are refused.
	{name: "hidden-selectors-and-format", setup: func(t *testing.T, dir string) {
		write(t, dir, "docs/vs.md", "a\ufe0f b\U000e0100\n\ufff9c\U0001d173\n")
	}, refuse: []string{"no-hidden-text"}, lines: []string{
		"docs/vs.md:1: holds U+FE0F (variation selector), U+E0100 (variation selector)",
		"docs/vs.md:2: holds U+FFF9 (invisible), U+1D173 (invisible)",
	}},
	// A NUL byte far into a file is refused, and so is what the file holds before it. The file is
	// then not text, and nothing after its first NUL is named, so a binary gives one line, however
	// many NUL bytes and hidden characters follow.
	{name: "hidden-nul-late", setup: func(t *testing.T, dir string) {
		write(t, dir, "docs/late.md", "\U000e0068\U000e0069\n"+strings.Repeat("x", 8100)+"\x00\u202e\n\x00\n\u200b\n")
	}, refuse: []string{"no-hidden-text"}, lines: []string{
		"docs/late.md:1: holds U+E0068 (invisible), U+E0069 (invisible)",
		"docs/late.md:2: holds U+0000 (NUL), so it is not text; the rest of the file is not read",
	}},
	// The invisible characters outside category Cf are refused too.
	{name: "hidden-fillers", setup: func(t *testing.T, dir string) {
		write(t, dir, "docs/fill.md", "a\u034fb\u115f\u1160\u17b4\u17b5\u3164\uffa0\n")
	}, refuse: []string{"no-hidden-text"}, lines: []string{
		"docs/fill.md:1: holds U+034F (invisible), U+115F (invisible), U+1160 (invisible), U+17B4 (invisible), U+17B5 (invisible), U+3164 (invisible), U+FFA0 (invisible)",
	}},
	// A symbolic link under testdata/ passes.
	{name: "hidden-link-in-testdata", setup: func(t *testing.T, dir string) {
		write(t, dir, "docs/testdata/note.md", "plain\n")
		symlink(t, dir, "note.md", "docs/testdata/alias.md")
	}},
	// A symbolic link outside testdata/ is refused, whatever it leads to: here, a file under testdata/.
	{name: "hidden-link", setup: func(t *testing.T, dir string) {
		write(t, dir, "docs/testdata/note.md", "\u202e\n")
		symlink(t, dir, "testdata/note.md", "docs/note.md")
	}, refuse: []string{"no-hidden-text"}, lines: []string{"docs/note.md: a symbolic link"}},
}

func TestNoHiddenText(t *testing.T) { runCases(t, hiddenCases) }

// no-hidden-text reads only the files git tracks, where the root is the top of a git work tree: a
// tracked file holding a hidden character or a NUL byte, and a tracked symbolic link, are refused,
// while an untracked file, ignored or not, passes whatever it holds, as a binary built in a checkout
// or an editor's scratch file would. Where the root is not the top of a work tree, every file is read.
func TestNoHiddenTextReadsTrackedFiles(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "-q")
	write(t, dir, ".gitignore", "/placket\n")
	write(t, dir, "docs/a.md", "plain\n\u202e here\n")
	write(t, dir, "docs/blob.bin", "\x00\x01\u202e\x00\n\x00\n")
	symlink(t, dir, "a.md", "docs/link.md")
	git("add", ".gitignore", "docs/a.md", "docs/blob.bin", "docs/link.md")
	write(t, dir, ".DS_Store", "\x00\x00\u202e\x00")
	write(t, dir, "placket", "\x7fELF\x00\x00\u2066")
	write(t, dir, "docs/.a.md.swp", "b0VIM\x00\u200b")
	write(t, dir, "docs/new.md", "\u202e\n")
	symlink(t, dir, "a.md", "docs/alias.md")

	place := func(fs []finding) string {
		var got []string
		for _, f := range fs {
			got = append(got, f.place()+": "+f.text)
		}
		sort.Strings(got)
		return strings.Join(got, "\n")
	}
	want := strings.Join([]string{
		"docs/a.md:2: holds U+202E (bidi control)",
		"docs/blob.bin:1: holds U+0000 (NUL), so it is not text; the rest of the file is not read",
		"docs/link.md: a symbolic link",
	}, "\n")
	if got := place(noHiddenText(newTree(dir))); got != want {
		t.Fatalf("in a work tree; want:\n%s\ngot:\n%s", want, got)
	}

	// Rooted below the top of the work tree, the check reads every file.
	wantAll := strings.Join([]string{
		".a.md.swp:1: holds U+0000 (NUL), so it is not text; the rest of the file is not read",
		"a.md:2: holds U+202E (bidi control)",
		"alias.md: a symbolic link",
		"blob.bin:1: holds U+0000 (NUL), so it is not text; the rest of the file is not read",
		"link.md: a symbolic link",
		"new.md:1: holds U+202E (bidi control)",
	}, "\n")
	if got := place(noHiddenText(newTree(filepath.Join(dir, "docs")))); got != wantAll {
		t.Fatalf("below the top of a work tree; want:\n%s\ngot:\n%s", wantAll, got)
	}
}
