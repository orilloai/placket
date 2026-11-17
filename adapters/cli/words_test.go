package cli_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// literal is a string literal of a Go file: its value, and where it is.
type literal struct {
	value, at string
}

// literals reads every string literal of the non-test Go files under the directories, struct tags
// aside.
func literals(t *testing.T, dirs ...string) []literal {
	t.Helper()
	var out []literal
	for _, d := range dirs {
		files, _ := filepath.Glob(filepath.Join(d, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			tags := map[*ast.BasicLit]bool{}
			ast.Inspect(file, func(n ast.Node) bool {
				if fl, ok := n.(*ast.Field); ok && fl.Tag != nil {
					tags[fl.Tag] = true
				}
				return true
			})
			ast.Inspect(file, func(n ast.Node) bool {
				if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING && !tags[l] {
					v, err := strconv.Unquote(l.Value)
					if err != nil {
						t.Fatal(err)
					}
					out = append(out, literal{v, fs.Position(l.Pos()).String()})
				}
				return true
			})
		}
	}
	return out
}

var notClaimed = regexp.MustCompile(`(?i)(^|[^a-z])(train|trains|trained|training|retain|retains|retained|retaining|retention|share|shares|shared|sharing|purpose|purposes|consent|consents|consented|consenting)([^a-z]|$)`)

var printing = []string{".", "../export", "../local", "../journal", "../state", "../keyfile", "../../cmd/placket"}

// TestEveryPrintedStringStopsAtTheCrossing reads every string the command line, the export and the
// adapters under them can print, and every line the worked example's show, verify and log printed:
// none speaks of what a receiver does after receipt; every claim line is followed by the scope line;
// show prints not verified first (claims-stop-at-crossing).
func TestEveryPrintedStringStopsAtTheCrossing(t *testing.T) {
	t.Parallel()
	for _, l := range literals(t, printing...) {
		if notClaimed.MatchString(l.value) {
			t.Fatalf("%s: %q", l.at, l.value)
		}
	}
	s := six(t)
	var printed []result
	for _, line := range strings.Split(strings.TrimSpace(string(s.receipts(t))), "\n") {
		p := filepath.Join(s.dir, "one.receipt")
		if err := os.WriteFile(p, []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		r := must(t, "show", "--receipt", p)
		if r.lines()[0] != "not verified" {
			t.Fatalf("show: %v", r)
		}
		printed = append(printed, r)
	}
	printed = append(printed,
		must(t, "verify", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--founding", filepath.Join(s.dir, "founding.json")),
		must(t, "log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--format", "csv", "--words", "--kept", s.state("proposer")),
		must(t, "log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--format", "json", "--words", "--kept", s.state("proposer")))
	claims := 0
	for _, r := range printed {
		if notClaimed.MatchString(r.stdout + r.stderr) {
			t.Fatalf("printed: %v", r)
		}
		l := r.lines()
		for i, x := range l {
			if strings.HasPrefix(x, "claim: ") {
				claims++
				if i+1 >= len(l) || !strings.HasPrefix(l[i+1], "not claimed: ") {
					t.Fatalf("a claim line with no scope line after it: %v", r)
				}
			}
		}
	}
	if claims != len(printed)-1 {
		t.Fatalf("%d claim lines in %d outputs", claims, len(printed))
	}
	var lg struct{ Claim, NotClaimed string }
	if err := json.Unmarshal([]byte(printed[len(printed)-1].stdout), &lg); err != nil || lg.Claim != cli.ClaimVerified || lg.NotClaimed != cli.Scope {
		t.Fatalf("log --format json: claim %q, notClaimed %q, %v", lg.Claim, lg.NotClaimed, err)
	}
}

// TestEachWordIsDefinedOnce reads the words file: every product word and claim line is a constant
// of it, defined once, and its value is a literal of no other file of the adapters or the binary;
// the words the core spells come from the core's constants (product-words-once).
func TestEachWordIsDefinedOnce(t *testing.T) {
	t.Parallel()
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "words.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	core := map[string]string{
		"WordPass": "composition", "WordBlock": "composition", "WordOK": "composition", "WordAccepted": "answer",
		"WordChanged": "answer", "WordDeclined": "answer", "WordCancelled": "answer", "WordTimeout": "receipts",
		"WordUndeliverable": "receipts", "WordBlocked": "export", "WordOpen": "export",
	}
	defined := map[string]string{}
	for _, d := range file.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			v := sp.(*ast.ValueSpec)
			for i, n := range v.Names {
				var pkg string
				ast.Inspect(v.Values[i], func(x ast.Node) bool {
					if sel, ok := x.(*ast.SelectorExpr); ok {
						if id, ok := sel.X.(*ast.Ident); ok {
							pkg = id.Name
						}
					}
					return true
				})
				if want, ok := core[n.Name]; ok && pkg != want {
					t.Fatalf("%s is not the %s package's constant", n.Name, want)
				}
				if lit, ok := v.Values[i].(*ast.BasicLit); ok {
					s, _ := strconv.Unquote(lit.Value)
					defined[n.Name] = s
				}
			}
		}
	}
	for name, pkg := range core {
		if _, ok := defined[name]; ok {
			t.Fatalf("%s spells a word the %s package spells", name, pkg)
		}
	}
	if len(defined) < 15 {
		t.Fatalf("%d words defined", len(defined))
	}
	count := map[string][]string{}
	for _, l := range literals(t, printing...) {
		count[l.value] = append(count[l.value], l.at)
	}
	for name, v := range defined {
		if len(count[v]) != 1 || !strings.HasPrefix(count[v][0], "words.go:") {
			t.Fatalf("%s, %q, is written at %v", name, v, count[v])
		}
	}
	for _, v := range []string{"blocked", "open", "yes"} {
		if len(count[v]) != 1 || !strings.Contains(count[v][0], "export.go:") {
			t.Fatalf("%q is written at %v", v, count[v])
		}
	}
}
