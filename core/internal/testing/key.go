// Package ptesting holds the helpers the core's tests share (test-helpers-placed): the vectors' keys
// and founding sets. Only tests import it.
package ptesting

import (
	"crypto/sha256"
	"testing"

	"github.com/orilloai/placket/core/signing"
)

// Key is the vectors' key of name: its seed is the SHA-256 of "placket vectors: key " and the name
// (vectors/README.md).
func Key(t testing.TB, name string) signing.Key {
	t.Helper()
	seed := sha256.Sum256([]byte("placket vectors: key " + name))
	k, err := signing.NewKey(seed[:])
	if err != nil {
		t.Fatal(err)
	}
	return k
}
