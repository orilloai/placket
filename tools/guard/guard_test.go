package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The checker's tests plant a fault in a copy of the repository and run every rule of the checker on
// it, in-process. A case names the rules that must refuse and what each must print, and every other
// rule must pass. The fixtures are under testdata/<case>/, laid over the copy; a case may also run a
// setup in the copy, for what a fixture cannot hold (a symbolic link, a change to a live file).

// live is the repository under test: the checker lives in tools/guard/.
var live = filepath.Join("..", "..")

// A tcase is one planted fault, or a tree that must pass.
type tcase struct {
	name    string
	fixture string
	setup   func(t *testing.T, dir string)
	// refuse names the rules that must refuse; none, and every rule must pass.
	refuse []string
	// at: each rule refusing prints a line starting "<rule>: <place>: " for each place, and no other
	// refusal line names a place not listed.
	at []string
	// lines: the refusal lines of the one rule refusing are exactly these, in order, each without the
	// "<rule>: " it starts with.
	lines []string
	// set: the refusal lines of the rules refusing are exactly these, in any order.
	set []string
}

// copyRepo returns a copy of the repository, without .git and the fixtures of the tests, for one
// case to change. The copies are kept and handed on: once a case ends, its copy is put back as it
// was (restore) and the next case takes it, so the tree is linked once for each case running at a
// time and not once for each case. Where the file system makes no hard links, each case copies the
// tree afresh, as copyTree does.
func copyRepo(t *testing.T) string {
	t.Helper()
	m, err := liveManifest()
	if err != nil {
		t.Fatalf("listing the repository: %v", err)
	}
	copies.Lock()
	n := len(copies.free)
	var dir string
	if n > 0 {
		dir, copies.free = copies.free[n-1], copies.free[:n-1]
	}
	plain := copies.plain
	copies.Unlock()
	if plain {
		return copyTree(t, live)
	}
	if dir == "" {
		if dir, err = os.MkdirTemp("", "guard-copy"); err != nil {
			t.Fatalf("copying the repository: %v", err)
		}
		copies.Lock()
		copies.made = append(copies.made, dir)
		copies.Unlock()
		if err := m.fill(dir, nil); err != nil {
			// No hard link here: this case and every later one copies the tree.
			copies.Lock()
			copies.plain = true
			copies.Unlock()
			return copyTree(t, live)
		}
	}
	t.Cleanup(func() {
		if err := m.restore(dir); err != nil {
			return // left out of the free copies; TestMain removes it
		}
		copies.Lock()
		copies.free = append(copies.free, dir)
		copies.Unlock()
	})
	return dir
}

// copies are the copies of the repository copyRepo made: those no case holds, every one made, and
// whether the file system refused a hard link, so that each case copies the tree itself.
var copies struct {
	sync.Mutex
	free, made []string
	plain      bool
}

// TestMain removes the copies of the repository once every test has run.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, dir := range copies.made {
		_ = os.RemoveAll(dir)
	}
	os.Exit(code)
}

// A manifest is what a copy of the repository holds: each directory and regular file copyTree
// would copy, a directory before what it holds, each file with the source's file info.
type manifest struct {
	items []item
	index map[string]int
}

type item struct {
	rel  string
	dir  bool
	info fs.FileInfo
}

var liveOnce struct {
	sync.Once
	m   *manifest
	err error
}

// liveManifest lists the repository once, as copyTree walks it.
func liveManifest() (*manifest, error) {
	liveOnce.Do(func() {
		m := &manifest{index: map[string]int{}}
		tracked, ok := trackedIn(live)
		liveOnce.err = filepath.WalkDir(live, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(live, p)
			if copySkip[filepath.ToSlash(rel)] {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if rel == "." || (!d.IsDir() && (!d.Type().IsRegular() || ok && !tracked[filepath.ToSlash(rel)])) {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			m.index[rel] = len(m.items)
			m.items = append(m.items, item{rel: rel, dir: d.IsDir(), info: info})
			return nil
		})
		liveOnce.m = m
	})
	return liveOnce.m, liveOnce.err
}

// fill makes, in dir, each directory of the manifest and a hard link to each file, but for the
// items have marks as there already.
func (m *manifest) fill(dir string, have []bool) error {
	for i, it := range m.items {
		if have != nil && have[i] {
			continue
		}
		to := filepath.Join(dir, it.rel)
		if it.dir {
			if err := os.MkdirAll(to, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.Link(filepath.Join(live, it.rel), to); err != nil {
			return err
		}
	}
	return nil
}

// restore puts a copy a case changed back as fill made it: whatever the manifest does not hold is
// removed, and so is a file that is no longer the source's own, which the helpers that change a
// copy leave behind since they replace a file and never write into it; then what is missing is
// made again.
func (m *manifest) restore(dir string) error {
	have := make([]bool, len(m.items))
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		drop := func() error {
			if err := os.RemoveAll(p); err != nil {
				return err
			}
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		i, ok := m.index[rel]
		if !ok || m.items[i].dir != d.IsDir() {
			return drop()
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			if info.Mode().Perm() != 0o755 {
				if err := os.Chmod(p, 0o755); err != nil {
					return err
				}
			}
		} else if !os.SameFile(info, m.items[i].info) {
			return drop()
		}
		have[i] = true
		return nil
	})
	if err != nil {
		return err
	}
	return m.fill(dir, have)
}

// atOnce bounds the cases running at a time. Each case reads the whole tree, and past a few at once
// the file system, more than the processors, is what they wait on: on a Mac, every processor's worth
// of cases together takes three times as long as four.
var atOnce = make(chan struct{}, 4)

// copySkip are the paths no copy of a tree holds: .git and the fixtures of the tests.
var copySkip = map[string]bool{".git": true, "tests/testdata": true, "tools/guard/testdata": true}

// copyTree copies the tree at src, without .git and the fixtures of the tests, into a new directory.
// .git is left out whether it is a directory or, in a worktree or a submodule, a file. Where src is
// the top of a git work tree, only the files git tracks are copied, so a file the writer's checkout
// holds and git does not, such as a binary built there, is no part of a case. Each file is a
// hard link to the source's where the file system allows it, so the copy is quick; the helpers that
// change the copy replace a file and never write into it, so the source never changes.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	tracked, ok := trackedIn(src)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if copySkip[filepath.ToSlash(rel)] {
			// SkipDir on a file would skip the rest of its directory: only a directory is skipped whole.
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ok && !d.IsDir() && !tracked[filepath.ToSlash(rel)] {
			return nil
		}
		to := filepath.Join(dst, rel)
		if d.Type().IsRegular() && os.Link(p, to) == nil {
			return nil
		}
		return copyEntry(p, to, d)
	})
	if err != nil {
		t.Fatalf("copying the repository: %v", err)
	}
	return dst
}

// trackedSets keeps the files git tracks in each tree copyTree has copied, by its path, listed once
// for every case that copies it, since the tree does not change while the tests run.
var trackedSets sync.Map

type trackedSet struct {
	once sync.Once
	set  map[string]bool
	ok   bool
}

func trackedIn(src string) (map[string]bool, bool) {
	v, _ := trackedSets.LoadOrStore(src, &trackedSet{})
	s := v.(*trackedSet)
	s.once.Do(func() { s.set, s.ok = newTree(src).tracked() })
	return s.set, s.ok
}

// lay copies the fixture testdata/<name>/ over dir.
func lay(t *testing.T, dir, name string) {
	t.Helper()
	src := filepath.Join("testdata", name)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		return copyEntry(p, filepath.Join(dir, rel), d)
	})
	if err != nil {
		t.Fatalf("laying the fixture %s: %v", name, err)
	}
}

// copyEntry copies a directory or a regular file to dst, replacing a file already there rather than
// writing into it.
func copyEntry(src, dst string, d fs.DirEntry) error {
	if d.IsDir() {
		return os.MkdirAll(dst, 0o755)
	}
	if !d.Type().IsRegular() {
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	return replace(dst, b, st.Mode().Perm())
}

// replace writes a new file at p, in place of any file there.
func replace(p string, b []byte, mode fs.FileMode) error {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(p, b, mode)
}

// edit rewrites a file of the copy.
func edit(t *testing.T, dir, rel string, fn func(string) string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := replace(p, []byte(fn(string(b))), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// write writes a file of the copy, making its directory.
func write(t *testing.T, dir, rel, text string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := replace(p, []byte(text), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func remove(t *testing.T, dir, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func symlink(t *testing.T, dir, target, rel string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// runAll runs every rule on the tree at dir, as the checker's command does, and returns each rule's
// lines and whether it refused.
func runAll(dir string) (map[string][]string, map[string]bool) {
	out, refused := map[string][]string{}, map[string]bool{}
	tr := newTree(dir)
	for _, r := range rules {
		out[r.name], refused[r.name] = r.run(tr)
	}
	return out, refused
}

var countLineRE = regexp.MustCompile(`^schemas-hold: spec/schema/[^ ]*[.]json: ok, [0-9]+ accepted, [0-9]+ refused$`)

// refusalLines are a rule's lines that are neither its "ok", its closing "refused:" line, a detail
// line nor schemas-hold's count of a schema that holds.
func refusalLines(name string, lines []string) []string {
	var out []string
	for _, l := range lines {
		if !strings.HasPrefix(l, name+": ") || l == name+": ok" || strings.HasPrefix(l, name+": refused: ") || countLineRE.MatchString(l) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// ruleDocs is the topic file of docs/rules/ that holds each rule's heading, which its closing
// "refused:" line links.
var ruleDocs = map[string]string{
	"core-no-deps": "modules.md", "core-imports": "modules.md", "root-module": "modules.md", "recipes-core-only": "modules.md",
	"core-no-world": "core.md", "core-no-panic": "core.md", "specs-name-behaviour": "core.md", "constants-once": "core.md",
	"schemas-hold": "tests.md", "vectors-independent": "tests.md",
	"product-words-once": "adapters.md", "claims-stop-at-crossing": "adapters.md", "main-is-thin": "adapters.md",
	"machine-rules-in-ci": "changes.md", "ci-stays-fast": "changes.md", "no-hidden-text": "changes.md", "agents-md-short": "changes.md",
	"docs-tree-holds": "changes.md",
}

var placeRE = regexp.MustCompile(`^[^ ]+: [^ :]+(:[0-9]+)?: `)

// checkContract holds a rule's lines to the output contract: each line starts with the rule's name or
// is an indented detail line under one; a refusal line names a file, with its line where it has one;
// and the last line is "<rule>: ok", or "<rule>: refused: <why> (docs/rules/<file>.md#<rule>)" when it
// refused, its file the one that holds the rule's heading.
func checkContract(t *testing.T, name string, lines []string, refused bool) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatalf("%s printed nothing", name)
	}
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "    ") && i > 0:
		case !strings.HasPrefix(l, name+": "):
			t.Fatalf("%s printed a line that does not start with its name: %q", name, l)
		}
	}
	for _, l := range refusalLines(name, lines) {
		if !placeRE.MatchString(l) {
			t.Fatalf("%s printed a refusal that names no file: %q", name, l)
		}
	}
	last := lines[len(lines)-1]
	if refused {
		if !strings.HasPrefix(last, name+": refused: ") || !strings.HasSuffix(last, " (docs/rules/"+ruleDocs[name]+"#"+name+")") {
			t.Fatalf("%s refused, and its last line is %q", name, last)
		}
	} else if last != name+": ok" {
		t.Fatalf("%s passed, and its last line is %q", name, last)
	}
}

func runCases(t *testing.T, cases []tcase) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			atOnce <- struct{}{}
			defer func() { <-atOnce }()
			dir := copyRepo(t)
			if c.fixture != "" {
				lay(t, dir, c.fixture)
			}
			if c.setup != nil {
				c.setup(t, dir)
			}
			out, refused := runAll(dir)
			show := func() string {
				var b strings.Builder
				for _, r := range rules {
					for _, l := range out[r.name] {
						b.WriteString("    | " + l + "\n")
					}
				}
				return b.String()
			}
			for _, r := range rules {
				checkContract(t, r.name, out[r.name], refused[r.name])
				if contains(c.refuse, r.name) {
					continue
				}
				if refused[r.name] || len(refusalLines(r.name, out[r.name])) > 0 {
					t.Fatalf("%s refused, and should pass:\n%s", r.name, show())
				}
			}
			var all []string
			for _, name := range c.refuse {
				if !refused[name] {
					t.Fatalf("%s passed, and should refuse:\n%s", name, show())
				}
				got := refusalLines(name, out[name])
				all = append(all, got...)
				for _, p := range c.at {
					if !hasPrefix(got, name+": "+p+": ") {
						t.Fatalf("no line starting %q:\n%s", name+": "+p+": ", show())
					}
				}
				if c.at != nil {
					for _, l := range got {
						if !hasPlace(l, name, c.at) {
							t.Fatalf("%s also refused: %q\n%s", name, l, show())
						}
					}
				}
				if c.lines != nil {
					var want []string
					for _, l := range c.lines {
						want = append(want, name+": "+l)
					}
					if strings.Join(got, "\n") != strings.Join(want, "\n") {
						t.Fatalf("%s's lines differ; want:\n%s\ngot:\n%s", name, strings.Join(want, "\n"), strings.Join(got, "\n"))
					}
				}
			}
			if c.set != nil {
				want := append([]string(nil), c.set...)
				sort.Strings(want)
				sort.Strings(all)
				if strings.Join(all, "\n") != strings.Join(want, "\n") {
					t.Fatalf("the refusal lines differ; want:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(all, "\n"))
				}
			}
		})
	}
}

func hasPrefix(lines []string, p string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

func hasPlace(l, name string, places []string) bool {
	for _, p := range places {
		if strings.HasPrefix(l, name+": "+p+": ") {
			return true
		}
	}
	return false
}

// allCases is every case of the checker's tests, for the test that each rule has one that refuses.
func allCases(t *testing.T) []tcase {
	var all []tcase
	for _, cs := range [][]tcase{coreCases, parserCases, shadowCases, checksCases, specCases(t), schemaFetchCases(), ciCases(t), markerCases(t), gitCases, agentsCases(t), hiddenCases, docsTreeCases(t)} {
		all = append(all, cs...)
	}
	return all
}

// Every rule of the checker has a case that plants a fault it refuses.
func TestEveryRuleHasARefusedCase(t *testing.T) {
	refusing := map[string]bool{}
	for _, c := range allCases(t) {
		for _, r := range c.refuse {
			refusing[r] = true
		}
	}
	for _, r := range rules {
		if !refusing[r.name] {
			t.Errorf("%s: no case plants a fault it refuses", r.name)
		}
	}
}

// The repository as it stands passes every rule, and schemas-hold counts the examples of every schema,
// with at least one refused.
func TestCleanTree(t *testing.T) {
	out, refused := runAll(live)
	for _, r := range rules {
		checkContract(t, r.name, out[r.name], refused[r.name])
		if refused[r.name] {
			t.Fatalf("%s refused the tree:\n%s", r.name, strings.Join(out[r.name], "\n"))
		}
	}
	schemas, err := filepath.Glob(filepath.Join(live, "spec", "schema", "*.json"))
	if err != nil || len(schemas) == 0 {
		t.Fatalf("no schema in spec/schema/: %v", err)
	}
	for _, s := range schemas {
		re := regexp.MustCompile(`^schemas-hold: spec/schema/` + regexp.QuoteMeta(filepath.Base(s)) + `: ok, [0-9]+ accepted, [1-9][0-9]* refused$`)
		found := false
		for _, l := range out["schemas-hold"] {
			found = found || re.MatchString(l)
		}
		if !found {
			t.Fatalf("no count line for spec/schema/%s:\n%s", filepath.Base(s), strings.Join(out["schemas-hold"], "\n"))
		}
	}
}

// The command runs the rules named, in order, and exits 1 when one refuses and 2 for a name that is
// not a rule.
func TestCommand(t *testing.T) {
	var b bytes.Buffer
	if rc := run(live, []string{"core-no-panic", "main-is-thin"}, &b); rc != 0 || b.String() != "core-no-panic: ok\nmain-is-thin: ok\n" {
		t.Fatalf("got %d and %q", rc, b.String())
	}
	dir := copyRepo(t)
	lay(t, dir, "panic-call")
	b.Reset()
	if rc := run(dir, []string{"core-no-panic", "main-is-thin"}, &b); rc != 1 {
		t.Fatalf("got %d and %q; want 1", rc, b.String())
	}
	want := "core-no-panic: core/p/p.go:7: panic(\"empty\")\ncore-no-panic: refused: no non-test code in core/ calls panic (docs/rules/core.md#core-no-panic)\nmain-is-thin: ok\n"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
	b.Reset()
	if want := "guard: no-such-rule: no such rule; the rules are: " + ruleNames() + "\n"; run(live, []string{"no-such-rule"}, &b) != 2 || b.String() != want ||
		!strings.HasPrefix(ruleNames(), "core-no-deps ") {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
	b.Reset()
	if want := "guard: name the rules to check: " + ruleNames() + "\n"; run(live, nil, &b) != 2 || b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

// gitFile is what a worktree or a submodule has at .git in place of the directory.
const gitFile = "gitdir: /elsewhere/.git/worktrees/placket\n"

// The tree's .git is left out, whatever it is: a directory with all below it, or the one file of a
// worktree or a submodule, and then only that file, so every rule still reads the rest of the tree.
var gitCases = []tcase{
	{name: "git-file-panic", fixture: "panic-call", setup: func(t *testing.T, dir string) { write(t, dir, ".git", gitFile) },
		refuse: []string{"core-no-panic"}, at: []string{"core/p/p.go:7"}},
	{name: "git-file-clean", setup: func(t *testing.T, dir string) { write(t, dir, ".git", gitFile) }},
	{name: "git-dir-unread", setup: func(t *testing.T, dir string) {
		write(t, dir, ".git/HEAD", "ref: refs/heads/main\n")
		write(t, dir, ".git/core/p/p.go", "package p\n\nfunc F( {\n\tpanic(\"x\")\n}\n")
	}},
}

func TestGitEntry(t *testing.T) {
	// The copy the cases start from leaves out a .git file and nothing after it.
	src := t.TempDir()
	write(t, src, ".git", gitFile)
	write(t, src, "core/p/p.go", "package p\n")
	write(t, src, "zz.txt", "z\n")
	dst := copyTree(t, src)
	for _, rel := range []string{"core/p/p.go", "zz.txt"} {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("the copy of a tree with a .git file lacks %s: %v", rel, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dst, ".git")); !os.IsNotExist(err) {
		t.Fatalf("the copy holds the .git file: %v", err)
	}
	runCases(t, gitCases)
}

// TestACopyHandedOnIsAsNew changes a copy of the repository in each way a case does, and in ways
// none does yet, and restores it: the copy then holds exactly what the manifest lists, each file
// the source's own, as a copy made afresh does.
func TestACopyHandedOnIsAsNew(t *testing.T) {
	t.Parallel()
	m, err := liveManifest()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := m.fill(dir, nil); err != nil {
		t.Skipf("no hard link here, so no copy is handed on: %v", err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	edit(t, dir, "Makefile", func(s string) string { return s + "\n# changed\n" })
	remove(t, dir, "AGENTS.md")
	write(t, dir, "docs/new/added.md", "added\n")
	symlink(t, dir, "README.md", "docs/link.md")
	must(os.RemoveAll(filepath.Join(dir, "spec")))
	must(os.Remove(filepath.Join(dir, "LICENSE")))
	must(os.MkdirAll(filepath.Join(dir, "LICENSE", "inner"), 0o755))
	must(os.RemoveAll(filepath.Join(dir, "cmd")))
	must(os.WriteFile(filepath.Join(dir, "cmd"), []byte("a file where a directory was\n"), 0o644))
	must(os.Chmod(filepath.Join(dir, "docs"), 0o700))
	must(m.restore(dir))

	seen := map[string]bool{}
	must(filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		i, ok := m.index[rel]
		if !ok {
			t.Errorf("%s is left in the copy", rel)
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch it := m.items[i]; {
		case it.dir != d.IsDir():
			t.Errorf("%s: a directory for a file, or a file for a directory", rel)
		case it.dir && info.Mode().Perm() != 0o755:
			t.Errorf("%s: mode %v", rel, info.Mode().Perm())
		case !it.dir && !os.SameFile(info, it.info):
			t.Errorf("%s is not the source's file", rel)
		}
		seen[rel] = true
		return nil
	}))
	for _, it := range m.items {
		if !seen[it.rel] {
			t.Errorf("%s is missing from the copy", it.rel)
		}
	}
	if len(m.items) < 100 || !seen["Makefile"] || !seen[filepath.Join("spec", "protocol.md")] {
		t.Fatalf("the manifest lists %d entries, and not the Makefile and spec/protocol.md", len(m.items))
	}
}
