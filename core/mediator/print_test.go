package mediator_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	ptesting "github.com/orilloai/placket/core/internal/testing"
	"github.com/orilloai/placket/core/mediator"
)

// TestMediatorNeverPrints prints a mediator that holds its key, a queued part and a held question,
// under every verb, as a value, a pointer, and inside a slice, a map and an exported field, and finds neither the key's seed nor its private key nor
// a body nor a salt, in any spelling; it prints the same fixed text whatever the verb
// (secrets-never-print).
func TestMediatorNeverPrints(t *testing.T) {
	t.Parallel()
	m := found(t)
	p, ap := ptesting.Key(t, "proposer"), ptesting.Key(t, "applier")
	const body = "the body of a queued part"
	note := message(t, m.Accord(), "n", ap.Reference(), "note", body, nil)
	if _, err := m.Give(p.Reference(), note, envelope(t, 1, note, p), "2026-11-12T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	const asked = "the body of a held proposal"
	prop := message(t, m.Accord(), "q", ap.Reference(), "proposal", asked, nil)
	if out, err := m.Give(p.Reference(), prop, envelope(t, 2, prop, p), "2026-11-12T09:01:00Z"); err != nil || out.Verdict != "escalate" {
		t.Fatalf("got %+v, %v", out, err)
	}
	seed := sha256.Sum256([]byte("placket vectors: key mediator"))
	private := ed25519.NewKeyFromSeed(seed[:])
	salt := hex.EncodeToString([]byte("a salt of 16 b n")[:16])
	var secrets []string
	for _, s := range []string{string(seed[:]), string(private), body, asked, salt} {
		secrets = append(secrets, s[:16], hex.EncodeToString([]byte(s))[:32], strings.ToUpper(hex.EncodeToString([]byte(s)))[:32])
	}
	secrets = append(secrets, fmt.Sprint([]byte(string(seed[:8])))[1:20])
	holders := []any{m, *m, []*mediator.Mediator{m}, struct{ M *mediator.Mediator }{m}, map[string]any{"m": m}}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%T", "%z"} {
		for _, h := range holders {
			out := fmt.Sprintf(verb, h)
			for _, s := range secrets {
				if strings.Contains(out, s) {
					t.Fatalf("%s prints a secret of the mediator: %q", verb, out)
				}
			}
		}
	}
	for _, v := range []any{m, *m} {
		if got := fmt.Sprintf("%v|%+v|%#v|%s|%x|%d|%z", v, v, v, v, v, v, v); got != strings.Repeat("mediator.Mediator|", 6)+"mediator.Mediator" {
			t.Fatalf("got %q", got)
		}
	}
	if m.String() != "mediator.Mediator" || m.GoString() != "mediator.Mediator" {
		t.Fatalf("String %q, GoString %q", m.String(), m.GoString())
	}
}
