package mediator_test

import (
	"testing"

	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

// TestEveryURIUnderTheCardsSpec holds every placket URI to the one protocol number: each is
// https://placket.tech/, its kind, a slash and the card's spec (setup-contract-4, signing-4,
// receipt-1, crossing-1). A later version changes them all together.
func TestEveryURIUnderTheCardsSpec(t *testing.T) {
	t.Parallel()
	for kind, uri := range map[string]string{
		"declaration": signing.TypeDeclaration,
		"binding":     signing.TypeBinding,
		"envelope":    signing.TypeEnvelope,
		"receipt":     receipts.PredicateType,
		"crossing":    mediator.Extension,
	} {
		if want := "https://placket.tech/" + kind + "/" + setup.Spec; uri != want {
			t.Errorf("the %s URI is %s, want %s", kind, uri, want)
		}
	}
}
