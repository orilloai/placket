package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// A tree is the repository the rules read, rooted at a directory. Paths are relative to the root and
// written with "/", as the rules print them. Each Go file is parsed once, however many rules read it.
type tree struct {
	root    string
	parsed  map[string]*goFile
	reads   map[string][]byte
	listed  bool
	entries []entry

	trackedDone bool
	trackedSet  map[string]bool
}

// A goFile is a parsed Go file and its lines. When it cannot be parsed, err says why at errLine.
type goFile struct {
	fset    *token.FileSet
	f       *ast.File
	lines   []string
	err     string
	errLine int
}

func newTree(root string) *tree {
	return &tree{root: root, parsed: map[string]*goFile{}, reads: map[string][]byte{}}
}

func (t *tree) abs(rel string) string {
	return filepath.Join(t.root, filepath.FromSlash(rel))
}

// read returns the file's bytes, or nil and false when it cannot be read. A file is read once.
func (t *tree) read(rel string) ([]byte, bool) {
	if b, ok := t.reads[rel]; ok {
		return b, b != nil
	}
	b, err := os.ReadFile(t.abs(rel))
	if err != nil {
		b = nil
	} else if b == nil {
		b = []byte{}
	}
	t.reads[rel] = b
	return b, b != nil
}

// tracked is the set of files git tracks in the tree, by path, and true; or nil and false when the
// root is not the top of a git work tree or git cannot list its files. It is listed once.
func (t *tree) tracked() (map[string]bool, bool) {
	if t.trackedDone {
		return t.trackedSet, t.trackedSet != nil
	}
	t.trackedDone = true
	if _, err := os.Lstat(filepath.Join(t.root, ".git")); err != nil {
		return nil, false
	}
	top, err := exec.Command("git", "-C", t.root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, false
	}
	abs, err := filepath.Abs(t.root)
	if err != nil {
		return nil, false
	}
	want, err1 := filepath.EvalSymlinks(abs)
	got, err2 := filepath.EvalSymlinks(strings.TrimRight(string(top), "\n"))
	if err1 != nil || err2 != nil || want != got {
		return nil, false
	}
	out, err := exec.Command("git", "-C", t.root, "ls-files", "-z", "--cached").Output()
	if err != nil {
		return nil, false
	}
	set := map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			set[p] = true
		}
	}
	t.trackedSet = set
	return set, true
}

// isDir and isFile follow a symbolic link, as the shell's -d and -f do.
func (t *tree) isDir(rel string) bool {
	st, err := os.Stat(t.abs(rel))
	return err == nil && st.IsDir()
}

func (t *tree) isFile(rel string) bool {
	st, err := os.Stat(t.abs(rel))
	return err == nil && st.Mode().IsRegular()
}

// isLink reports whether the path itself is a symbolic link.
func (t *tree) isLink(rel string) bool {
	st, err := os.Lstat(t.abs(rel))
	return err == nil && st.Mode()&fs.ModeSymlink != 0
}

// An entry is a path found by walk: a regular file, a directory or a symbolic link, which walk never
// follows.
type entry struct {
	path string
	dir  bool
	link bool
}

// list is every path of the tree, in lexical order, found once by walking it without following a
// symbolic link; the repository's own .git is left out, the directory with all below it, or the one
// file a worktree or a submodule has there in its place. walk reads its subtrees from it.
func (t *tree) list() []entry {
	if t.listed {
		return t.entries
	}
	t.listed = true
	_ = filepath.WalkDir(t.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == t.root {
			return nil
		}
		rel, rerr := filepath.Rel(t.root, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" {
			// SkipDir on a file would skip the rest of the root: only a directory is skipped whole.
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		t.entries = append(t.entries, entry{path: rel, dir: d.IsDir(), link: d.Type()&fs.ModeSymlink != 0})
		return nil
	})
	return t.entries
}

// walk lists everything under dir, in lexical order, without following a symbolic link: dir itself
// when it is a link, and nothing below it. A directory below dir for which prune returns true is
// neither listed nor entered. "." is the whole tree. A missing dir lists nothing.
func (t *tree) walk(dir string, prune func(rel string, name string) bool) []entry {
	var out []entry
	pruned := map[string]bool{}
	for _, e := range t.list() {
		if e.path == dir {
			if e.link {
				return []entry{e}
			}
			continue
		}
		if dir != "." && !strings.HasPrefix(e.path, dir+"/") {
			continue
		}
		parent := path.Dir(e.path)
		if pruned[parent] {
			if e.dir {
				pruned[e.path] = true
			}
			continue
		}
		if e.dir && prune != nil && prune(e.path, path.Base(e.path)) {
			pruned[e.path] = true
			continue
		}
		out = append(out, e)
	}
	return out
}

// files lists the regular files under dir that keep returns true for, in lexical order; a directory
// named in prune is skipped.
func (t *tree) files(dir string, prune []string, keep func(rel string) bool) []string {
	var out []string
	for _, e := range t.walk(dir, func(_, name string) bool { return contains(prune, name) }) {
		if e.dir || e.link {
			continue
		}
		if keep == nil || keep(e.path) {
			out = append(out, e.path)
		}
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// inTestdata reports whether a path has a directory named testdata among its parts.
func inTestdata(rel string) bool {
	parts := strings.Split(rel, "/")
	return contains(parts[:len(parts)-1], "testdata")
}

func isTestGo(rel string) bool { return strings.HasSuffix(rel, "_test.go") }

// parsedFiles keeps every Go file the checker parsed, by its path and bytes, so that trees holding the
// same file, as the checker's tests make, parse it once. A goFile is never changed once made.
var parsedFiles sync.Map

// parse returns the Go file at rel, parsed with its comments, whatever its build constraints.
func (t *tree) parse(rel string) *goFile {
	if g, ok := t.parsed[rel]; ok {
		return g
	}
	src, ok := t.read(rel)
	if !ok {
		g := &goFile{fset: token.NewFileSet(), err: "cannot be read"}
		t.parsed[rel] = g
		return g
	}
	key := rel + "\x00" + string(src)
	if g, ok := parsedFiles.Load(key); ok {
		t.parsed[rel] = g.(*goFile)
		return g.(*goFile)
	}
	g := &goFile{fset: token.NewFileSet()}
	g.lines = splitLines(string(src))
	f, err := parser.ParseFile(g.fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		var list scanner.ErrorList
		if errors.As(err, &list) && len(list) > 0 {
			g.err, g.errLine = list[0].Msg, list[0].Pos.Line
		} else {
			g.err, g.errLine = err.Error(), 1
		}
	} else {
		g.f = f
	}
	parsedFiles.Store(key, g)
	t.parsed[rel] = g
	return g
}

// splitLines splits text into its lines as awk reads them: a last line without a newline still counts.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// at is the finding for a position of the file: the line, its text without leading blanks.
func (g *goFile) at(file string, pos token.Pos) finding {
	n := g.fset.Position(pos).Line
	return finding{file: file, line: n, text: g.text(n)}
}

func (g *goFile) text(n int) string {
	if n < 1 || n > len(g.lines) {
		return ""
	}
	return strings.TrimLeft(g.lines[n-1], " \t")
}

// unparsed is the finding for a file that cannot be parsed.
func (g *goFile) unparsed(file string) finding {
	return finding{file: file, line: g.errLine, text: "does not parse: " + g.err}
}

func dir(rel string) string { return path.Dir(rel) }
