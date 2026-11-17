package export

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestExportErrorsQuoteNothing plants a secret in the payload, the payload type and the signature of
// a receipt the log refuses: the error's text is the package, the code, the verifier's code and the
// index, never the secret (error-text-no-content).
func TestExportErrorsQuoteNothing(t *testing.T) {
	t.Parallel()
	secret := "SEKRET-5e4b"
	m := "ed25519:" + strings.Repeat("1", 64)
	p := base64.StdEncoding.EncodeToString([]byte(`{"body":"` + secret + `"}`))
	for _, c := range []struct {
		receipt, want string
	}{
		{`{"payload":"` + p + `","payloadType":"application/vnd.in-toto+json","signatures":[{"keyid":"` + secret + `","sig":"` + secret + `"}]}`, "export: verify: signature, index 0"},
		{`{"payload":"` + p + `","payloadType":"` + secret + `","signatures":[]}`, "export: verify: payload-type, index 0"},
		{secret, "export: verify: not-canonical, index 0"},
	} {
		_, err := Build(m, [][]byte{[]byte(c.receipt)}, nil, func(string) {})
		if err == nil || strings.Contains(err.Error(), secret) || err.Error() != c.want {
			t.Fatalf("%s: got %v, want %q", c.receipt, err, c.want)
		}
	}
}
