package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// git runs git commands in one repository or worktree.
type git struct{ dir string }

// run runs git with the arguments given, the standard input in, and returns its standard output.
func (g git) run(in string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = g.dir, childEnv()
	if in != "" {
		cmd.Stdin = strings.NewReader(in)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return out.String(), nil
}

// commit resolves a revision to a commit's full hash.
func (g git) commit(rev string) (string, error) {
	out, err := g.run("", "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s is not a commit", rev)
	}
	return strings.TrimSpace(out), nil
}

// isAncestor reports whether a is an ancestor of b, or b itself.
func (g git) isAncestor(a, b string) (bool, error) {
	cmd := exec.Command("git", "merge-base", "--is-ancestor", a, b)
	cmd.Dir, cmd.Env = g.dir, childEnv()
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base: %v", err)
}

// An entry is one blob of a commit's tree.
type entry struct {
	mode, hash, path string
}

// tree lists every blob of a commit's tree, by path; a submodule's entry is left out.
func (g git) tree(rev string) (map[string]entry, error) {
	out, err := g.run("", "ls-tree", "-r", "-z", "--full-tree", rev)
	if err != nil {
		return nil, err
	}
	t := map[string]entry{}
	for _, rec := range strings.Split(out, "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		f := strings.Fields(meta)
		if len(f) != 3 || f[1] != "blob" {
			continue
		}
		t[path] = entry{mode: f[0], hash: f[2], path: path}
	}
	return t, nil
}

// blob reads one blob by its hash.
func (g git) blob(hash string) ([]byte, error) {
	out, err := g.run("", "cat-file", "blob", hash)
	return []byte(out), err
}

// A trailer is one trailer of a commit in the change's range.
type trailer struct {
	commit, key, value string
}

// trailers lists the trailers of every commit reachable from head and not from base, newest first.
func (g git) trailers(base, head string) ([]trailer, error) {
	out, err := g.run("", "log", "-z", "--format=%h%n%(trailers:only,unfold)", base+".."+head)
	if err != nil {
		return nil, err
	}
	var ts []trailer
	for _, rec := range strings.Split(out, "\x00") {
		ls := strings.Split(strings.Trim(rec, "\n"), "\n")
		if len(ls) == 0 || ls[0] == "" {
			continue
		}
		for _, l := range ls[1:] {
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			ts = append(ts, trailer{commit: ls[0], key: strings.TrimSpace(k), value: strings.TrimSpace(v)})
		}
	}
	return ts, nil
}

// changeCommits lists every commit reachable from head and not from base, oldest first, with its
// author's email, whether it is a merge, and its Signed-off-by trailers, read as git reads trailers.
// Each field is on a line of its own: no header holds a newline, and no unfolded trailer does. A
// record that does not line up with the commits of the range fails the judgement, never skips it.
func (g git) changeCommits(base, head string) ([]changeCommit, error) {
	list, err := g.run("", "rev-list", "--reverse", base+".."+head)
	if err != nil {
		return nil, err
	}
	hashes := strings.Fields(list)
	out, err := g.run("", "log", "-z", "--reverse",
		"--format=%H%n%h%n%P%n%ae%n%(trailers:key=Signed-off-by,valueonly,unfold)", base+".."+head)
	if err != nil {
		return nil, err
	}
	var recs []string
	if out = strings.TrimSuffix(out, "\x00"); out != "" {
		recs = strings.Split(out, "\x00")
	}
	if len(recs) != len(hashes) {
		return nil, fmt.Errorf("git log: %d commits read, %d in the range", len(recs), len(hashes))
	}
	cs := make([]changeCommit, 0, len(recs))
	for i, rec := range recs {
		f := strings.Split(rec, "\n")
		if len(f) < 5 || f[0] != hashes[i] || f[len(f)-1] != "" {
			return nil, fmt.Errorf("git log: commit %d of the range is unreadable", i+1)
		}
		c := changeCommit{hash: f[1], author: f[3], merge: len(strings.Fields(f[2])) > 1}
		for _, s := range f[4 : len(f)-1] {
			if s = strings.TrimSpace(s); s != "" {
				c.signOffs = append(c.signOffs, s)
			}
		}
		cs = append(cs, c)
	}
	return cs, nil
}
