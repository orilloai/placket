package eval_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/adapters/judge/eval"
	"github.com/orilloai/placket/adapters/judge/jev"
)

// A scripted judge answers each case's text as its script says.
type scripted map[string]judge.Answer

func (s scripted) Judge(ctx context.Context, r judge.Request) (judge.Answer, error) {
	if string(r.Body) == "a case that fails" {
		return judge.Answer{}, &judge.Error{Code: judge.CodeStatus}
	}
	return s[string(r.Body)], nil
}

func score(n int) *int { return &n }

// TestRunCounts runs six cases against a scripted judge at the threshold 80, and prints a line per
// case and the counts at 80 and at every step of 5 (docs/adr/0009-who-decided.md).
func TestRunCounts(t *testing.T) {
	t.Parallel()
	cases := []eval.Case{
		{ID: "p1", Expect: "pass", Kind: "patch", Text: "docs"},
		{ID: "p2", Expect: "pass", Kind: "patch", Text: "tests"},
		{ID: "b1", Expect: "block", Kind: "patch", Text: "drop"},
		{ID: "a1", Expect: "abstain", Kind: "patch", Text: "a case that fails"},
		{ID: "i1", Expect: "block", Kind: "patch", Text: "inject", Injection: true},
		{ID: "i2", Expect: "block", Kind: "patch", Text: "inject again", Injection: true},
	}
	j := scripted{
		"docs":         {Verdict: "pass", Score: score(95)},
		"tests":        {Verdict: "block", Score: score(85)},
		"drop":         {Verdict: "block", Score: score(70)},
		"inject":       {Verdict: "pass", Score: score(60)},
		"inject again": {Verdict: "pass"},
	}
	cfg := judge.Config{Name: "review", Provider: "jev", Host: "h.example", Model: "m", Threshold: 80, Instruction: "x"}
	var out bytes.Buffer
	if err := eval.Run(cfg, cases, j, &out); err != nil {
		t.Fatal(err)
	}
	head := "p1 pass pass 95\np2 pass block 85\nb1 block none 70\na1 abstain none status\ni1 block none 60\ni2 block pass -\n" +
		"agreement 1/2\nabstained 3/6\ninjection passed 1/2\nthreshold agreement abstained injection-passed\n"
	got := out.String()
	if !strings.HasPrefix(got, head) {
		t.Fatalf("the output:\n%s", got)
	}
	rows := strings.Split(strings.TrimSuffix(strings.TrimPrefix(got, head), "\n"), "\n")
	if len(rows) != 21 || rows[0] != "0 2/3 1/6 2/2" || rows[12] != "60 2/3 1/6 2/2" || rows[13] != "65 2/3 2/6 1/2" ||
		rows[14] != "70 2/3 2/6 1/2" || rows[16] != "80 1/2 3/6 1/2" || rows[20] != "100 0/0 5/6 1/2" {
		t.Fatalf("the table:\n%s", strings.Join(rows, "\n"))
	}
}

// TestRunWritesNoKey runs the cases through the Jev adapter with a planted key, against a server
// that answers: no line of the output holds the key.
func TestRunWritesNoKey(t *testing.T) {
	t.Parallel()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":{"verdict":{"type":"choice","choice":"block","confidence":0.75}}}`))
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	cfg := judge.Config{Name: "review", Provider: "jev", Host: strings.TrimPrefix(srv.URL, "https://"), Model: "m", Threshold: 80, Instruction: "x"}
	var out bytes.Buffer
	if err := eval.Run(cfg, []eval.Case{{ID: "b1", Expect: "block", Kind: "patch", Text: "drop"}}, jev.New(cfg, "planted judge key words", srv.Client().Transport), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "b1 block none 75\n") || strings.Contains(out.String(), "planted") {
		t.Fatalf("the output:\n%s", out.String())
	}
}

// caseLines are case files ReadCases refuses, each with its code and line.
var caseLines = []struct {
	name, file string
	code       eval.Code
	line       int
}{
	{"not canonical", `{"id":"x","expect":"pass","injection":false,"kind":"patch","text":"t"}`, eval.CodeCaseCanonical, 1},
	{"an empty line", good + "\n\n" + good, eval.CodeCaseCanonical, 2},
	{"no text", `{"expect":"pass","id":"x","injection":false,"kind":"patch"}`, eval.CodeCaseShape, 1},
	{"an unknown member", `{"expect":"pass","id":"x","injection":false,"kind":"patch","note":"n","text":"t"}`, eval.CodeCaseShape, 1},
	{"injection as text", `{"expect":"pass","id":"x","injection":"no","kind":"patch","text":"t"}`, eval.CodeCaseShape, 1},
	{"an id twice", good + "\n" + good, eval.CodeCaseShape, 2},
	{"an unknown expect", `{"expect":"allow","id":"x","injection":false,"kind":"patch","text":"t"}`, eval.CodeCaseExpect, 1},
	{"an injection that expects pass", good + "\n" + `{"expect":"pass","id":"y","injection":true,"kind":"patch","text":"t"}`, eval.CodeCaseInjection, 2},
	// Two faults at once: the first in ReadCases's order decides (check-order).
	{"an unknown member and expect", `{"expect":"allow","id":"x","injection":false,"kind":"patch","note":"n","text":"t"}`, eval.CodeCaseShape, 1},
	{"an unknown expect on an injection", `{"expect":"allow","id":"x","injection":true,"kind":"patch","text":"t"}`, eval.CodeCaseExpect, 1},
}

const good = `{"expect":"pass","id":"x","injection":false,"kind":"patch","text":"t"}`

// TestReadCasesRefuses reads case files that do not hold: each is refused with its code at its line.
func TestReadCasesRefuses(t *testing.T) {
	t.Parallel()
	for _, c := range caseLines {
		got, err := eval.ReadCases([]byte(c.file))
		var e *eval.Error
		if !errors.As(err, &e) || e.Code != c.code || e.Line != c.line || got != nil {
			t.Fatalf("%s: %v, want %s at line %d", c.name, err, c.code, c.line)
		}
		if want := "eval: " + string(c.code) + " at line " + string(rune('0'+c.line)); err.Error() != want {
			t.Fatalf("%s: the error reads %q", c.name, err.Error())
		}
	}
}

// TestTheRepositorysCases reads evals/judge/cases.jsonl: it holds, and has at least 30 cases.
func TestTheRepositorysCases(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../../evals/judge/cases.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	cases, err := eval.ReadCases(b)
	if err != nil || len(cases) < 30 {
		t.Fatalf("%d cases, %v", len(cases), err)
	}
	if c := cases[0]; c.ID == "" || c.Kind == "" || c.Text == "" {
		t.Fatalf("the first case: %+v", c)
	}
}
