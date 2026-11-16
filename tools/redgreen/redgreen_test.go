package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixtures is the directory of the fixture trees. A case makes a git repository whose first commit,
// on main, is the tree base/, then lays other trees over it, commit by commit.
var fixtures, _ = filepath.Abs("testdata")

// repo is a git repository in a temporary directory, made for one case.
type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.lay("base")
	r.commit("Start the fixture")
	return r
}

// git runs git in the repository and returns its output, trimmed; it stops the case on a failure.
func (r *repo) git(args ...string) string {
	r.t.Helper()
	full := append([]string{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// lay copies each fixture tree named over the repository's working tree.
func (r *repo) lay(trees ...string) {
	r.t.Helper()
	for _, name := range trees {
		src := filepath.Join(fixtures, name)
		err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			dst := filepath.Join(r.dir, rel)
			if d.IsDir() {
				return os.MkdirAll(dst, 0o755)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(dst, b, 0o644)
		})
		if err != nil {
			r.t.Fatalf("lay %s: %v", name, err)
		}
	}
}

// remove deletes files from the working tree.
func (r *repo) remove(paths ...string) {
	r.t.Helper()
	for _, p := range paths {
		if err := os.Remove(filepath.Join(r.dir, p)); err != nil {
			r.t.Fatalf("remove %s: %v", p, err)
		}
	}
}

// commit commits every change in the working tree with the message given and returns its hash.
func (r *repo) commit(msg string) string {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

// judge runs the program on the repository from the merge base of main and HEAD to HEAD, and returns
// its output and exit status.
func (r *repo) judge() (string, int) {
	r.t.Helper()
	base := r.git("merge-base", "main", "HEAD")
	return r.run(base, "HEAD")
}

func (r *repo) run(args ...string) (string, int) {
	r.t.Helper()
	var out bytes.Buffer
	code := run(append([]string{"-C", r.dir}, args...), &out)
	return out.String(), code
}

// branch starts a branch named work from main's tip.
func (r *repo) branch() {
	r.t.Helper()
	r.git("checkout", "-q", "-b", "work")
}

func lines(s ...string) string { return strings.Join(s, "\n") + "\n" }

// expect compares the program's output and status with those wanted, exactly.
func expect(t *testing.T, got string, code int, want string, wantCode int) {
	t.Helper()
	if got != want || code != wantCode {
		t.Fatalf("exit %d, want %d\ngot:\n%s\nwant:\n%s", code, wantCode, got, want)
	}
}

const refused = "red-green-tests: refused: every test the change adds or changes fails without its code and passes with it, and none is removed or skipped unexplained (AGENTS.md, red-green-tests)"

// T1: an added test that fails at the base and passes at the head is red (fails), and the change is ok.
func TestAddedTestRedThenGreen(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("sub-test", "sub-fix")
	r.commit("Fix Sub")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/sub_test.go:5: TestSub: red (fails)",
		"red-green-tests: ok, 1 red on base, 1 green at head, 0 reported",
	), 0)
}

// T2: an added test that passes at the base is refused, unless Green-On-Base names it.
func TestAddedTestGreenOnBase(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("add-more")
	r.commit("Cover Add with a negative")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/more_test.go:5: TestAddMore: green on base",
		refused,
	), 1)

	r.commit("Explain the green test\n\nGreen-On-Base: TestAddMore: covers existing behaviour")
	out, code = r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/more_test.go:5: TestAddMore: green on base, excused (Green-On-Base: covers existing behaviour)",
		"red-green-tests: ok, 0 red on base, 1 green at head, 1 reported",
	), 0)
}

// T3: an added test whose package does not build at the base, since it calls a function the head adds,
// is red (build).
func TestAddedTestRedByBuild(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("mul-test", "sub-fix")
	r.commit("Add Mul")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/mul_test.go:5: TestMul: red (build)",
		"red-green-tests: ok, 1 red on base, 1 green at head, 0 reported",
	), 0)
}

// T4: an added test that skips at the head is not green at the head.
func TestAddedTestSkipsAtHead(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("sub-skip", "sub-fix")
	r.commit("Fix Sub, test it later")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/sub_test.go:5: TestSub: red (skips)",
		"red-green-tests: core/calc/sub_test.go:5: TestSub: not green at head (skips)",
		refused,
	), 1)
}

// T5: a test removed between the base and the head is refused, unless Test-Weakened names it.
func TestRemovedTest(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("old-removed")
	r.commit("Drop TestOld")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc_test.go:11: TestOld: removed",
		refused,
	), 1)

	r.commit("Explain\n\nTest-Weakened: TestOld: TestAdd covers it")
	out, code = r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc_test.go:11: TestOld: removed, excused (Test-Weakened: TestAdd covers it)",
		"red-green-tests: ok, 0 red on base, 0 green at head, 1 reported",
	), 0)
}

// T6: a skip added to an existing test is refused, unless Test-Weakened names the test.
func TestSkipAdded(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("keep-skipped")
	r.commit("Skip TestKeep")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc_test.go:17: TestKeep: skipped",
		refused,
	), 1)

	r.commit("Explain\n\nTest-Weakened: TestKeep: flaky on the runner")
	out, code = r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc_test.go:17: TestKeep: skipped, excused (Test-Weakened: flaky on the runner)",
		"red-green-tests: ok, 0 red on base, 0 green at head, 1 reported",
	), 0)
}

// T7: a branch with a merge commit from main, and code and tests in mixed commits in any order, is
// judged on its base and head alone: the same as the same change squashed onto main.
func TestHistoryIgnored(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("sub-fix")
	r.commit("Fix Sub first")
	r.git("checkout", "-q", "main")
	r.lay("readme")
	r.commit("Say more on main")
	r.git("checkout", "-q", "work")
	r.lay("sub-test", "readme")
	r.commit("Test Sub and say more")
	r.git("merge", "-q", "--no-ff", "--no-edit", "main")
	r.lay("mul-test")
	r.commit("Test Mul")
	want := lines(
		"red-green-tests: core/calc/mul_test.go:5: TestMul: red (build)",
		"red-green-tests: core/calc/sub_test.go:5: TestSub: red (build)",
		"red-green-tests: ok, 2 red on base, 2 green at head, 0 reported",
	)
	out, code := r.judge()
	expect(t, out, code, want, 0)

	tree := r.git("rev-parse", "HEAD^{tree}")
	squashed := r.git("commit-tree", tree, "-p", "main", "-m", "The change, squashed")
	out, code = r.run(r.git("rev-parse", "main"), squashed)
	expect(t, out, code, want, 0)
}

// T8: code changed under core/ with no test added or changed is refused, unless No-Test gives a
// reason; a change to Markdown alone runs no test and is ok.
func TestCodeWithoutTest(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("sub-fix")
	r.commit("Fix Sub")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc.go: code changed, and no test added or changed",
		refused,
	), 1)

	r.commit("Explain\n\nNo-Test: the vectors cover Sub")
	out, code = r.judge()
	expect(t, out, code, lines(
		"red-green-tests: no test added or changed, excused (No-Test: the vectors cover Sub)",
		"red-green-tests: ok, 0 red on base, 0 green at head, 1 reported",
	), 0)

	m := newRepo(t)
	m.branch()
	m.lay("readme")
	m.commit("Say more")
	out, code = m.judge()
	expect(t, out, code, lines("red-green-tests: ok, 0 red on base, 0 green at head, 0 reported"), 0)
}

// T9: an added shell test that fails at the base and passes at the head is ok; one that passes at the
// base is refused, unless Green-On-Base names its file.
func TestShellTests(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("shell-red", "feature")
	r.commit("Add the feature")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: tests/test_x.sh: red (fails)",
		"red-green-tests: ok, 1 red on base, 1 green at head, 0 reported",
	), 0)

	g := newRepo(t)
	g.branch()
	g.lay("shell-green")
	g.commit("Add a shell test")
	out, code = g.judge()
	expect(t, out, code, lines(
		"red-green-tests: tests/test_x.sh: green on base",
		refused,
	), 1)

	g.commit("Explain\n\nGreen-On-Base: tests/test_x.sh: covers what already works")
	out, code = g.judge()
	expect(t, out, code, lines(
		"red-green-tests: tests/test_x.sh: green on base, excused (Green-On-Base: covers what already works)",
		"red-green-tests: ok, 0 red on base, 1 green at head, 1 reported",
	), 0)
}

// T10: a changed test that passes at the base is reported, not refused.
func TestChangedTestGreenOnBase(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("add-changed")
	r.commit("Check Add both ways")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc_test.go:5: TestAdd: green on base, changed: reported",
		"red-green-tests: ok, 0 red on base, 1 green at head, 1 reported",
	), 0)
}

// T11: a trailer that names a test the change does not touch is refused.
func TestTrailerNamesUntouchedTest(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("sub-test", "sub-fix")
	c := r.commit("Fix Sub\n\nGreen-On-Base: TestKeep: it was always green")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/sub_test.go:5: TestSub: red (fails)",
		"red-green-tests: "+c[:7]+": Green-On-Base: TestKeep: trailer names an untouched test",
		refused,
	), 1)
}

// T12: the old flow, a commit marked Loom-Lock: tests and then the code, is ok.
func TestLockCommitFlow(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("sub-test")
	r.commit("Add tests for Sub\n\nLoom-Lock: tests")
	r.lay("sub-fix")
	r.commit("Fix Sub")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/sub_test.go:5: TestSub: red (fails)",
		"red-green-tests: ok, 1 red on base, 1 green at head, 0 reported",
	), 0)
}

// T13: with no go on PATH, or a base that is not an ancestor of the head, the program fails as a tool
// (exit 3), never passes. So does a call without a base.
func TestToolErrors(t *testing.T) {
	r := newRepo(t)
	r.branch()
	r.lay("sub-test", "sub-fix")
	r.commit("Fix Sub")
	r.git("checkout", "-q", "-b", "side", "main")
	side := r.commit("A side commit")
	r.git("checkout", "-q", "work")

	out, code := r.run(side, "HEAD")
	expect(t, out, code, lines("red-green-tests: error: "+side+" is not an ancestor of HEAD"), 3)

	out, code = r.run()
	expect(t, out, code, lines("red-green-tests: error: usage: redgreen [-C dir] <base> [<head>]"), 3)

	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("no git: %v", err)
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatalf("link git: %v", err)
	}
	t.Setenv("PATH", bin)
	out, code = r.run(r.git("merge-base", "main", "HEAD"), "HEAD")
	expect(t, out, code, lines("red-green-tests: error: no go on PATH"), 3)
}

// T14: a test whose body differs only in layout or comments is not a changed test.
func TestLayoutIsNoChange(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("keep-reformatted")
	r.commit("Reformat the tests")
	out, code := r.judge()
	expect(t, out, code, lines("red-green-tests: ok, 0 red on base, 0 green at head, 0 reported"), 0)
}

// A helper changed in a test file the base has is reported, and no test is run on its account.
func TestHelperChanged(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("helper-changed")
	r.commit("Add a helper")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: helpers changed: core/calc/calc_test.go",
		"red-green-tests: ok, 0 red on base, 0 green at head, 0 reported",
	), 0)
}

// A test behind a build tag the default build leaves out is listed as not run, and reported.
func TestBuildTagNotRun(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("tagged")
	r.commit("Add a tagged test")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/tagged_test.go:7: TestTagged: not run: build tag",
		"red-green-tests: ok, 0 red on base, 0 green at head, 1 reported",
	), 0)
}

// A shell test removed is refused, unless Test-Weakened names its file; a trailer with no reason is
// refused.
func TestShellTestRemoved(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.lay("shell-green")
	r.commit("Add a shell test on main")
	r.branch()
	r.remove("tests/test_x.sh")
	c := r.commit("Remove it\n\nTest-Weakened: tests/test_x.sh")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: tests/test_x.sh: removed",
		"red-green-tests: "+c[:7]+": Test-Weakened: tests/test_x.sh: trailer gives no test and reason",
		refused,
	), 1)

	r.commit("Explain\n\nTest-Weakened: tests/test_x.sh: nothing left to test")
	out, code = r.judge()
	expect(t, out, code, lines(
		"red-green-tests: tests/test_x.sh: removed, excused (Test-Weakened: nothing left to test)",
		"red-green-tests: "+c[:7]+": Test-Weakened: tests/test_x.sh: trailer gives no test and reason",
		refused,
	), 1)
}

// A build tag added to a test file the default build read before leaves its tests out: each is
// skipped, as if it called Skip.
func TestBuildTagAddedSkips(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("tagged-existing")
	r.commit("Tag the tests")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/calc_test.go:7: TestAdd: skipped",
		"red-green-tests: core/calc/calc_test.go:13: TestOld: skipped",
		"red-green-tests: core/calc/calc_test.go:19: TestKeep: skipped",
		refused,
	), 1)
}

// An added test green on base is found green even when another test of its run panics at the base
// and ends the run before it: it runs again alone.
func TestNeighbourPanics(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("panics", "sub-fix")
	r.commit("Fix Sub")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/ab_test.go:5: TestA: red (fails)",
		"red-green-tests: core/calc/ab_test.go:11: TestB: green on base",
		refused,
	), 1)
}

// A test that is not run, behind a build tag, does not count as the test of a code change.
func TestTaggedTestIsNoTest(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.branch()
	r.lay("tagged", "sub-fix")
	r.commit("Fix Sub, with a test that never runs")
	out, code := r.judge()
	expect(t, out, code, lines(
		"red-green-tests: core/calc/tagged_test.go:7: TestTagged: not run: build tag",
		"red-green-tests: core/calc/calc.go: code changed, and no test added or changed",
		refused,
	), 1)
}
