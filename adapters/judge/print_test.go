package judge_test

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/judge"
)

// The planted strings of the print tests: a body, a model's output and a key. Each is plain words,
// so the Makefile's secrets step takes none of them for a secret.
const (
	plantedBody   = "planted body words"
	plantedOutput = "planted model output words"
)

// TestNothingPrintsContent prints a Request, an Answer and a Record that hold a planted body and
// output every way: each prints its fixed text, never the planted words (secrets-never-print).
func TestNothingPrintsContent(t *testing.T) {
	t.Parallel()
	n := 7
	values := map[string]any{
		"judge.Request": judge.Request{Kind: "report", Instruction: "an instruction", Body: []byte(plantedBody)},
		"judge.Answer":  judge.Answer{Verdict: "pass", Score: &n, Output: []byte(plantedOutput)},
		"judge.Record":  judge.Record{Judge: "a", Use: "decide", Outcome: "pass", Output: []byte(plantedOutput)},
	}
	for want, v := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
			for _, got := range []string{fmt.Sprintf(verb, v), fmt.Sprintf(verb, []any{v}), fmt.Sprintf(verb, struct{ V any }{v})} {
				if planted(got) {
					t.Fatalf("%s under %s prints %s", want, verb, got)
				}
			}
		}
		if got := fmt.Sprint(v); got != want {
			t.Fatalf("%T prints %q, want %q", v, got, want)
		}
	}
	if got := (&judge.Error{Code: judge.CodeUnparsable}).Error(); got != "judge: unparsable" {
		t.Fatalf("the error reads %q", got)
	}
}

// planted reports whether s holds a planted string, as it is or in hex.
func planted(s string) bool {
	for _, p := range []string{plantedBody, plantedOutput, "planted"} {
		if strings.Contains(s, p) || strings.Contains(s, hex.EncodeToString([]byte(p))) {
			return true
		}
	}
	return false
}
