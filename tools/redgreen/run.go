package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// An outcome is what one run of a test came to.
type outcome int

const (
	passed  outcome = iota
	failed          // the test ran and failed
	skipped         // the test ran and skipped
	noBuild         // the test's package did not build, or the go tool could not run it
	notRun          // the package ran, and the test did not
)

// base names an outcome at the base, where anything but a pass is red.
func (o outcome) base() string {
	switch o {
	case passed:
		return "green on base"
	case skipped:
		return "red (skips)"
	case noBuild:
		return "red (build)"
	case notRun:
		return "red (not run)"
	}
	return "red (fails)"
}

// head names an outcome at the head that is not a pass.
func (o outcome) head() string {
	switch o {
	case skipped:
		return "not green at head (skips)"
	case noBuild:
		return "not green at head (build)"
	case notRun:
		return "not green at head (not run)"
	}
	return "not green at head (fails)"
}

// A worktree is a scratch checkout of one commit, made with git worktree.
type worktree struct {
	repo git
	dir  string
}

// addWorktree checks out a commit in a new scratch directory.
func addWorktree(repo git, rev string) (*worktree, error) {
	dir, err := os.MkdirTemp("", "redgreen-")
	if err != nil {
		return nil, err
	}
	// The judged repository's hooks are not run: a checkout here is the check's, not the builder's.
	if _, err := repo.run("", "-c", "core.hooksPath=/dev/null", "worktree", "add", "--quiet", "--detach", dir, rev); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &worktree{repo: repo, dir: dir}, nil
}

// remove deletes the worktree and its directory.
func (w *worktree) remove() {
	// A failure here leaves a scratch directory or a stale entry behind, never a wrong verdict.
	_, _ = w.repo.run("", "worktree", "remove", "--force", w.dir)
	_ = os.RemoveAll(w.dir)
	_, _ = w.repo.run("", "worktree", "prune")
}

// layTests lays every test file of the head's tree over the worktree, and removes each test file of
// the base's tree that the head does not have.
func (w *worktree) layTests(head string, baseTree, headTree map[string]entry) error {
	var paths []string
	for p := range headTree {
		if isTestPath(p) {
			paths = append(paths, p)
		}
	}
	if len(paths) > 0 {
		in := strings.Join(sortedStrings(paths), "\x00") + "\x00"
		if _, err := (git{dir: w.dir}).run(in, "checkout", "--pathspec-from-file=-", "--pathspec-file-nul", head, "--"); err != nil {
			return err
		}
	}
	for p := range baseTree {
		if _, ok := headTree[p]; !ok && isTestPath(p) {
			if err := os.Remove(filepath.Join(w.dir, filepath.FromSlash(p))); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

// runGo runs the tests given in the worktree, one go test per package, and returns each one's outcome
// by key.
func (w *worktree) runGo(tests []goTest) (map[string]outcome, error) {
	byDir := map[string][]string{}
	for _, t := range tests {
		byDir[t.dir] = append(byDir[t.dir], t.name)
	}
	res := map[string]outcome{}
	for _, dir := range sortedKeys(byDir) {
		names := sortedStrings(byDir[dir])
		got, err := w.runPackage(dir, names)
		if err != nil {
			return nil, err
		}
		// A test that did not run, since another of the run panicked or exited, runs again alone, so
		// that a test green on base is never counted red for a neighbour's sake.
		for _, n := range names {
			if got[n] == notRun && len(names) > 1 {
				alone, err := w.runPackage(dir, []string{n})
				if err != nil {
					return nil, err
				}
				got[n] = alone[n]
			}
			res[dir+"\x00"+n] = got[n]
		}
	}
	return res, nil
}

// runPackage runs the named tests of one package directory from its module's root.
func (w *worktree) runPackage(dir string, names []string) (map[string]outcome, error) {
	res := map[string]outcome{}
	mod := dir
	for {
		if _, err := os.Stat(filepath.Join(w.dir, filepath.FromSlash(mod), "go.mod")); err == nil {
			break
		}
		if mod == "." {
			for _, n := range names {
				res[n] = noBuild
			}
			return res, nil
		}
		mod = path.Dir(mod)
	}
	rel := "."
	switch {
	case mod == ".":
		rel = "./" + dir
	case mod != dir:
		rel = "./" + strings.TrimPrefix(dir, mod+"/")
	}
	cmd := exec.Command("go", "test", "-count=1", "-json", "-run", "^("+strings.Join(names, "|")+")$", rel)
	cmd.Dir = filepath.Join(w.dir, filepath.FromSlash(mod))
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, fmt.Errorf("go test: %v", err)
		}
	}
	ran := false
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var ev struct{ Action, Test string }
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Test == "" {
			continue
		}
		ran = true
		switch ev.Action {
		case "pass":
			res[ev.Test] = passed
		case "fail":
			res[ev.Test] = failed
		case "skip":
			res[ev.Test] = skipped
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("go test: reading its output: %v", err)
	}
	for _, n := range names {
		if _, ok := res[n]; !ok {
			if ran {
				res[n] = notRun
			} else {
				res[n] = noBuild
			}
		}
	}
	return res, nil
}

// runShell runs a shell test from the worktree's root: it passes when it exits 0.
func (w *worktree) runShell(p string) (outcome, error) {
	cmd := exec.Command("sh", p)
	cmd.Dir = w.dir
	err := cmd.Run()
	if err == nil {
		return passed, nil
	}
	if _, ok := err.(*exec.ExitError); ok {
		return failed, nil
	}
	return 0, fmt.Errorf("sh %s: %v", p, err)
}

func sortedKeys[V any](m map[string]V) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedStrings(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}
