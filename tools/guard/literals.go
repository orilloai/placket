package main

import (
	"go/ast"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// A literal is a string literal of a Go file: its text between the quotes, with its escapes as
// written, a raw string's line breaks written as \n, at the line it starts on.
type literal struct {
	file string
	line int
	text string
}

// literals returns the string literals of the files built into a package under the directories, no
// _test.go file and nothing under testdata/, import paths included, in file and line order.
func literals(t *tree, dirs ...string) ([]literal, []finding) {
	var files []string
	for _, d := range dirs {
		if t.isDir(d) && !t.isLink(d) {
			files = append(files, t.files(d, nil, func(rel string) bool {
				return strings.HasSuffix(rel, ".go") && !isTestGo(rel) && !inTestdata(rel)
			})...)
		}
	}
	var lits []literal
	var fs []finding
	for _, rel := range files {
		g := t.parse(rel)
		if g.f == nil {
			fs = append(fs, g.unparsed(rel))
			continue
		}
		ast.Inspect(g.f, func(n ast.Node) bool {
			if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING {
				text := l.Value[1 : len(l.Value)-1]
				if l.Value[0] == '`' {
					text = strings.ReplaceAll(text, "\n", `\n`)
				}
				lits = append(lits, literal{file: rel, line: g.fset.Position(l.Pos()).Line, text: text})
			}
			return true
		})
	}
	return lits, fs
}

// constants-once: every string literal holding "placket.tech/" or "vnd.placket" in a file built into
// core/ or adapters/ is in exactly one file. Each file holding a literal that another also holds is
// named at its first line with it. Two literals are the same when they are written the same, escapes
// included.
func constantsOnce(t *tree) []finding {
	lits, fs := literals(t, "core", "adapters")
	first := map[string]map[string]int{}
	for _, l := range lits {
		if !strings.Contains(l.text, "placket.tech/") && !strings.Contains(l.text, "vnd.placket") {
			continue
		}
		if first[l.text] == nil {
			first[l.text] = map[string]int{}
		}
		if _, ok := first[l.text][l.file]; !ok {
			first[l.text][l.file] = l.line
		}
	}
	for s, byFile := range first {
		if len(byFile) < 2 {
			continue
		}
		var files []string
		for f := range byFile {
			files = append(files, f)
		}
		sort.Strings(files)
		for _, f := range files {
			var others []string
			for _, o := range files {
				if o != f {
					others = append(others, o)
				}
			}
			fs = append(fs, finding{file: f, line: byFile[f], text: `"` + s + `" is also in ` + strings.Join(others, ", ")})
		}
	}
	return fs
}

var (
	escapeRE    = regexp.MustCompile(`\\.`)
	claimWordRE = regexp.MustCompile(`(^|[^a-z])(train|trains|trained|training|retain|retains|retained|retaining|retention|share|shares|shared|sharing|purpose|purposes|consent|consents|consented|consenting)([^a-z]|$)`)
)

// claims-stop-at-crossing: no string literal in a file built into adapters/ or cmd/ holds a word that
// speaks of what a receiver does after receipt: train, retain, retention, share, purpose or consent,
// in any case and with their usual endings, each as a whole word (so "retainer" passes). An escape
// such as \n, and the line break in a raw string, separate words.
func claimsStopAtCrossing(t *tree) []finding {
	lits, fs := literals(t, "adapters", "cmd")
	for _, l := range lits {
		if claimWordRE.MatchString(escapeRE.ReplaceAllString(strings.ToLower(l.text), " ")) {
			fs = append(fs, finding{file: l.file, line: l.line, text: `"` + l.text + `"`})
		}
	}
	return fs
}
