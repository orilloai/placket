package signing_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/orilloai/placket/core/signing"
)

// holder is a struct that holds a Key in an unexported field: fmt does not call Key's Format there,
// so the Key must print nothing of itself by its shape alone (secrets-never-print).
type holder struct {
	name string
	key  signing.Key
}

// TestAKeyInAnUnexportedFieldNeverPrints prints a struct holding a Key in an unexported field, and
// a pointer to it, under every verb, and finds neither the seed nor the private key in any form.
func TestAKeyInAnUnexportedFieldNeverPrints(t *testing.T) {
	t.Parallel()
	seed := sha256.Sum256([]byte("placket vectors: key proposer"))
	k, err := signing.NewKey(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	h := holder{name: "proposer", key: k}
	secrets := []string{hex.EncodeToString(seed[:]), strings.ToUpper(hex.EncodeToString(seed[:])),
		strings.TrimSuffix(fmt.Sprint(seed[:8]), "]"), string(seed[:8])}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q", "%X", "%d"} {
		out := fmt.Sprintf(verb, h) + fmt.Sprintf(verb, &h) + fmt.Sprintf(verb, []holder{h})
		for _, secret := range secrets {
			if strings.Contains(out, secret) {
				t.Fatalf("%s prints the key: %q", verb, out)
			}
		}
	}
}
