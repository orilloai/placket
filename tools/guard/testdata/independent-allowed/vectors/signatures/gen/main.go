// Command gen writes the signature vectors with the canonical form, a lower package whose own
// vectors already pass.
package main

import (
	"crypto/ed25519"

	"github.com/orilloai/placket/core/canonical"
	// "github.com/orilloai/placket/core/signing" in a comment is not an import.
)

func main() { _, _ = canonical.Form, ed25519.Sign }
