package conform

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestErrorTextQuotesNoInput plants a body, a salt, a key and a call in the evidence and a delivery,
// and checks that no *Error's text holds them under any verb: the package, the code and an index
// alone (error-text-no-content).
func TestErrorTextQuotesNoInput(t *testing.T) {
	t.Parallel()
	w := journal(t)
	salt := strings.Repeat("09", 32)
	planted := []string{body, salt, string(w.call), "ed25519:" + strings.Repeat("7", 64)}
	e := w.evidence()
	e.Accord = "sha256:" + strings.Repeat("ab", 32)
	e.Answers = [][]byte{[]byte(body), []byte(salt)}
	_, founding := Check(e)
	r, err := Check(w.evidence())
	if err != nil {
		t.Fatal(err)
	}
	bodyErr := Body(r, []byte(`{"receiptId":"`+body+`","salt":"`+salt+`","text":"`+body+`"}`))
	for _, err := range []error{founding, bodyErr, &Error{Code: CodeWitness, Index: 3}} {
		var ce *Error
		if !errors.As(err, &ce) {
			t.Fatalf("got %v, want *Error", err)
		}
		for _, verb := range []string{"%v", "%+v", "%s", "%q", "%#v"} {
			text := fmt.Sprintf(verb, err)
			for _, p := range planted {
				if strings.Contains(text, p) {
					t.Fatalf("%s quotes its input: %s", verb, text)
				}
			}
		}
	}
	if founding.Error() != "conform: founding" || bodyErr.Error() != "conform: body" || (&Error{Code: CodeWitness, Index: 3}).Error() != "conform: witness at index 3" {
		t.Fatalf("texts %q, %q", founding, bodyErr)
	}
}
