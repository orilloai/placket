package errtext_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/errtext"
)

// root is the repository's root, from this package's directory.
const root = "../.."

// A source is a package directory and the Code types read there, each with the key its codes are
// rowed under.
type source struct {
	dir   string
	types map[string]string
}

var sources = []source{
	{"adapters/local", map[string]string{"DoorCode": errtext.FromDoor, "Code": errtext.FromLocal}},
	{"core/mediator", map[string]string{"RejectionCode": errtext.FromRejection, "ErrorCode": errtext.FromError}},
	{"adapters/client", map[string]string{"Code": errtext.FromClient}},
	{"adapters/mcp", map[string]string{"Code": errtext.FromMCP}},
	{"adapters/state", map[string]string{"Code": errtext.FromState}},
	{"adapters/keyfile", map[string]string{"Code": errtext.FromKeyfile}},
}

// imports maps the packages a constant's value may name, by their import path's last element, to
// their directories.
var imports = map[string]string{"answer": "core/answer"}

// consts reads every constant of a package's non-test files: its type's name, or "" when it is
// untyped, and the expression that gives its value.
type constant struct {
	typ  string
	expr ast.Expr
}

func consts(t *testing.T, dir string) map[string]constant {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(root, dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("%s: no Go files: %v", dir, err)
	}
	out := map[string]constant{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				v := spec.(*ast.ValueSpec)
				typ := ""
				if id, ok := v.Type.(*ast.Ident); ok {
					typ = id.Name
				}
				for i, n := range v.Names {
					if i < len(v.Values) {
						out[n.Name] = constant{typ, v.Values[i]}
					}
				}
			}
		}
	}
	return out
}

// value is the string a constant's expression gives: a string literal, a conversion of one, or a
// constant of the same package or of an imported one, named or converted.
func value(t *testing.T, dir string, e ast.Expr) string {
	t.Helper()
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind == token.STRING {
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
	case *ast.CallExpr:
		if len(x.Args) == 1 {
			return value(t, dir, x.Args[0])
		}
	case *ast.Ident:
		if c, ok := consts(t, dir)[x.Name]; ok {
			return value(t, dir, c.expr)
		}
	case *ast.SelectorExpr:
		if p, ok := x.X.(*ast.Ident); ok {
			if d, ok := imports[p.Name]; ok {
				if c, ok := consts(t, d)[x.Sel.Name]; ok {
					return value(t, d, c.expr)
				}
			}
		}
	}
	t.Fatalf("%s: a constant whose value is not read: %T", dir, e)
	return ""
}

// TestEveryCodeHasItsSentence reads, with go/parser, every constant of each Code type placket mcp can
// return a code of, keyed by where it comes from: each has a row, and every row is one such
// constant's. A constant converted from another package's is read through that package's constant.
func TestEveryCodeHasItsSentence(t *testing.T) {
	t.Parallel()
	want := map[[2]string]bool{}
	for _, s := range sources {
		for _, c := range consts(t, s.dir) {
			from, ok := s.types[c.typ]
			if !ok {
				continue
			}
			want[[2]string{from, value(t, s.dir, c.expr)}] = true
		}
	}
	if !want[[2]string{errtext.FromRejection, "answer-call"}] {
		t.Fatal("core/mediator.AnswerCall was not read through core/answer's constant")
	}
	var missing []string
	for k := range want {
		if s, ok := errtext.Sentence(k[0], k[1]); !ok || s == "" {
			missing = append(missing, k[0]+" "+k[1])
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("codes with no row: %v", missing)
	}
	for _, r := range errtext.Rows() {
		if !want[[2]string{r.From, r.Code}] {
			t.Errorf("a row with no constant: %s %s", r.From, r.Code)
		}
	}
}

// TestOneSpellingTwoSentences reads reply under adapters/local, not a reply, and under
// adapters/client, not what the call asked for: two rows, two sentences.
func TestOneSpellingTwoSentences(t *testing.T) {
	t.Parallel()
	l, lok := errtext.Sentence(errtext.FromLocal, "reply")
	c, cok := errtext.Sentence(errtext.FromClient, "reply")
	if !lok || !cok || l == c {
		t.Fatalf("reply: %q, %q", l, c)
	}
	if got := errtext.Text(errtext.FromClient, "reply"); got != "reply: "+c {
		t.Fatalf("the text of a row: %q", got)
	}
	if got := errtext.Text(errtext.FromDoor, "no-such-code"); got != "no-such-code" {
		t.Fatalf("the text of a code with no row: %q", got)
	}
}

// TestSentencesStopAtTheCrossing holds every sentence to one sentence of cause and fix: it ends in a
// full stop, holds a semicolon between cause and fix, and speaks no word of what a receiver does
// after receipt (claims-stop-at-crossing).
func TestSentencesStopAtTheCrossing(t *testing.T) {
	t.Parallel()
	words := []string{"share", "shared", "retain", "consent", "purpose", "train"}
	for _, r := range errtext.Rows() {
		if !strings.HasSuffix(r.Sentence, ".") || strings.Count(r.Sentence, ". ") > 0 || !strings.Contains(r.Sentence, "; ") {
			t.Errorf("%s %s: %q is not one sentence of cause and fix", r.From, r.Code, r.Sentence)
		}
		for _, w := range strings.Fields(strings.ToLower(r.Sentence)) {
			for _, bad := range words {
				if strings.Trim(w, ".,;") == bad {
					t.Errorf("%s %s: %q", r.From, r.Code, r.Sentence)
				}
			}
		}
	}
}

