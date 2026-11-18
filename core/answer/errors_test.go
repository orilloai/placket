package answer

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/internal/shape"
)

// TestErrorTextQuotesNoInput plants a body, a salt, a key and a call in Read's input and checks that
// the error's text holds none of them under any verb (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	k := key(t, "decider")
	body := "the answer's secret text"
	salt := hex.EncodeToString([]byte("a salt of the answerer, 32 bytes"))
	call, err := Sign(Call{Accord: accord, Question: question, Action: Accepted, Digest: digest, Release: true}, k)
	if err != nil {
		t.Fatal(err)
	}
	planted := map[string][]byte{
		"a body":  edit(t, call, func(o shape.Object) { o["text"] = body }),
		"a salt":  edit(t, call, func(o shape.Object) { o["salt"] = salt }),
		"a key":   edit(t, call, func(o shape.Object) { o["from"] = "ed25519:" + strings.Repeat("9", 64) }),
		"a call":  append(append([]byte(nil), call...), '!'),
		"garbage": []byte(body + salt),
	}
	for name, b := range planted {
		_, err := Read(b)
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("Read(%s) = %v, want *Error", name, err)
		}
		for _, verb := range []string{"%v", "%+v", "%s", "%q", "%#v"} {
			text := fmt.Sprintf(verb, err)
			for _, secret := range []string{body, salt, strings.Repeat("9", 64), string(call[:40]), k.Reference()} {
				if strings.Contains(text, secret) {
					t.Fatalf("Read(%s)'s error under %s quotes its input: %s", name, verb, text)
				}
			}
		}
		if err.Error() != "answer: answer-call" {
			t.Fatalf("Read(%s)'s error = %q", name, err.Error())
		}
	}
}
