package main

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

var groupCellRE = regexp.MustCompile("^ *`[a-z0-9-]+` *$")

// vectorGroups reads the table "Groups and the packages they test" of vectors/README.md: each group,
// by the name in its first column, and the core packages in its last.
func vectorGroups(t *tree) map[string][]string {
	groups := map[string][]string{}
	b, ok := t.read("vectors/README.md")
	if !ok {
		return groups
	}
	on := false
	clean := func(s string) string { return strings.NewReplacer(" ", "", "`", "").Replace(s) }
	for _, l := range splitLines(string(b)) {
		if strings.HasPrefix(l, "## ") {
			on = l == "## Groups and the packages they test"
		}
		cells := strings.Split(l, "|")
		if !on || len(cells) < 4 || !groupCellRE.MatchString(cells[1]) {
			continue
		}
		g, p := clean(cells[1]), clean(cells[len(cells)-2])
		if p == "" {
			continue
		}
		groups[g] = append(groups[g], strings.Split(p, ",")...)
	}
	return groups
}

// vectors-independent: a generator is Go code under vectors/<group>/, outside testdata/, test files
// included. It imports no core package its group tests, as vectors/README.md's table gives them, and
// never the conformance runner, which runs them all; nor does it spell such a package's path as a
// string; nor does it write an import path with an escape. A Go file outside every group's directory,
// or in a group the table does not list, is refused whole; so is a symbolic link under vectors/. A
// package reached through another package is not seen (vectors/README.md, Known limits).
func vectorsIndependent(t *tree) []finding {
	var fs []finding
	for _, e := range t.walk("vectors", nil) {
		if e.link {
			fs = append(fs, finding{file: e.path, text: "a symbolic link"})
		}
	}
	if t.isLink("vectors") {
		return fs
	}
	groups := vectorGroups(t)
	files := t.files("vectors", nil, func(rel string) bool { return strings.HasSuffix(rel, ".go") && !inTestdata(rel) })
	for _, rel := range files {
		g, rest, ok := strings.Cut(strings.TrimPrefix(rel, "vectors/"), "/")
		if !ok || rest == "" {
			fs = append(fs, finding{file: rel, text: "a generator outside every group's directory; generators go under vectors/<group>/"})
			continue
		}
		pkgs := groups[g]
		if len(pkgs) == 0 {
			fs = append(fs, finding{file: rel, text: "the group " + g + " has no row in vectors/README.md's table of the packages each group tests"})
			continue
		}
		tested := regexp.MustCompile(`^github\.com/orilloai/placket/core/(` + alternation(append(pkgs, "conformance")) + `)(/.*)?$`)
		gf := t.parse(rel)
		if gf.f == nil {
			fs = append(fs, gf.unparsed(rel))
			continue
		}
		imports := map[*ast.BasicLit]bool{}
		for _, s := range gf.f.Imports {
			imports[s.Path] = true
			if p, ok := importPath(s); !ok || tested.MatchString(p) {
				fs = append(fs, gf.at(rel, s.Path.Pos()))
			}
		}
		ast.Inspect(gf.f, func(n ast.Node) bool {
			if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING && !imports[l] && tested.MatchString(l.Value[1:len(l.Value)-1]) {
				fs = append(fs, gf.at(rel, l.Pos()))
			}
			return true
		})
	}
	return fs
}

// alternation joins names into a regular expression that matches any one of them, as written.
func alternation(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = regexp.QuoteMeta(n)
	}
	return strings.Join(q, "|")
}
