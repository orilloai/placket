package main

import (
	"bytes"
	"go/ast"
	"go/build"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A goTest is one Test or Fuzz function of a package's _test.go files at one commit.
type goTest struct {
	dir, name, file string
	line            int
	// body is the function's tokens, without comments or layout, so that two bodies that gofmt and
	// comments alone set apart are equal.
	body string
	// skips counts the calls of Skip, Skipf and SkipNow in the function.
	skips int
	// built is whether the default build includes the function's file; a test behind a build tag is
	// not run.
	built bool
}

func (g goTest) key() string { return g.dir + "\x00" + g.name }

// A pkgTests is what one commit's _test.go files hold: their tests, by key, the files that do not
// parse, and each file's other declarations (helpers), as tokens.
type pkgTests struct {
	tests    map[string]goTest
	broken   []string
	helpers  map[string]string
	contents map[string][]byte
}

// isTestPath reports whether a path is a test file: a _test.go file, anything under a testdata/
// directory, or anything under tests/.
func isTestPath(p string) bool {
	return strings.HasSuffix(p, "_test.go") || strings.HasPrefix(p, "testdata/") ||
		strings.Contains(p, "/testdata/") || strings.HasPrefix(p, "tests/")
}

// isGoTestFile reports whether a path is a _test.go file the go tool reads: none of its directories
// is testdata, vendor, or starts with "." or "_", and neither does its name.
func isGoTestFile(p string) bool {
	if !strings.HasSuffix(p, "_test.go") {
		return false
	}
	for _, el := range strings.Split(p, "/") {
		if el == "testdata" || el == "vendor" || strings.HasPrefix(el, ".") || strings.HasPrefix(el, "_") {
			return false
		}
	}
	return true
}

// isShellTest reports whether a path is one of the repository's shell tests, tests/test_*.sh.
func isShellTest(p string) bool {
	return strings.HasPrefix(p, "tests/test_") && strings.HasSuffix(p, ".sh") && !strings.Contains(p[len("tests/"):], "/")
}

// isTestName reports whether a name is a test the go tool runs with the prefix given: the prefix
// alone, or followed by a character that is not a lower-case letter.
func isTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}

// findTests reads the _test.go files of the directories given in a commit's tree.
func findTests(g git, t map[string]entry, dirs map[string]bool) (pkgTests, error) {
	p := pkgTests{tests: map[string]goTest{}, helpers: map[string]string{}, contents: map[string][]byte{}}
	var files []string
	for f := range t {
		if isGoTestFile(f) && dirs[path.Dir(f)] {
			files = append(files, f)
		}
	}
	for _, f := range files {
		b, err := g.blob(t[f].hash)
		if err != nil {
			return p, err
		}
		p.contents[filepath.FromSlash(f)] = b
	}
	ctxt := build.Default
	ctxt.OpenFile = func(name string) (io.ReadCloser, error) {
		b, ok := p.contents[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	for _, f := range sortedStrings(files) {
		src := p.contents[filepath.FromSlash(f)]
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, parser.SkipObjectResolution)
		if err != nil {
			p.broken = append(p.broken, f)
			continue
		}
		built, err := ctxt.MatchFile(filepath.FromSlash(path.Dir(f)), path.Base(f))
		if err != nil {
			built = false
		}
		var helper []string
		for _, d := range af.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || !isTestFunc(fn) {
				if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
					continue
				}
				helper = append(helper, tokens(fset, src, d.Pos(), d.End()))
				continue
			}
			gt := goTest{
				dir:   path.Dir(f),
				name:  fn.Name.Name,
				file:  f,
				line:  fset.Position(fn.Pos()).Line,
				body:  tokens(fset, src, fn.Type.Pos(), fn.End()),
				skips: countSkips(fn),
				built: built,
			}
			if _, dup := p.tests[gt.key()]; !dup {
				p.tests[gt.key()] = gt
			}
		}
		p.helpers[f] = strings.Join(helper, "\n")
	}
	return p, nil
}

// isTestFunc reports whether a declaration is a test the go tool runs: a top-level function, with no
// receiver or type parameters and one parameter, named Test... or Fuzz...; TestMain is not a test.
func isTestFunc(fn *ast.FuncDecl) bool {
	if fn.Recv != nil || fn.Body == nil || fn.Type.TypeParams != nil || fn.Type.Params.NumFields() != 1 {
		return false
	}
	n := fn.Name.Name
	return n != "TestMain" && (isTestName(n, "Test") || isTestName(n, "Fuzz"))
}

// countSkips counts the calls x.Skip, x.Skipf and x.SkipNow in a function, x a name.
func countSkips(fn *ast.FuncDecl) int {
	n := 0
	ast.Inspect(fn, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, ok := sel.X.(*ast.Ident); ok {
			switch sel.Sel.Name {
			case "Skip", "Skipf", "SkipNow":
				n++
			}
		}
		return true
	})
	return n
}

// tokens returns the Go tokens of src between two positions, as text, without comments and without
// what layout alone decides: a semicolon or a comma just before a closing bracket, and whether a
// semicolon was written or came from a line's end.
func tokens(fset *token.FileSet, src []byte, from, to token.Pos) string {
	start, end := fset.Position(from).Offset, fset.Position(to).Offset
	if start < 0 || end > len(src) || start > end {
		return ""
	}
	part := src[start:end]
	var s scanner.Scanner
	sf := token.NewFileSet().AddFile("", -1, len(part))
	s.Init(sf, part, nil, 0)
	var toks []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		switch {
		case tok == token.SEMICOLON:
			toks = append(toks, ";")
		case lit != "":
			toks = append(toks, lit)
		default:
			toks = append(toks, tok.String())
		}
	}
	var out []string
	for i, tk := range toks {
		if (tk == ";" || tk == ",") && i+1 < len(toks) && (toks[i+1] == ")" || toks[i+1] == "}" || toks[i+1] == "]") {
			continue
		}
		if tk == ";" && i+1 == len(toks) {
			continue
		}
		out = append(out, tk)
	}
	return strings.Join(out, " ")
}
