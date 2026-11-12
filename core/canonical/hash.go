package canonical

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashPrefix starts every typed hash (canonical-3).
const HashPrefix = "sha256:"

// TypedHash puts input in canonical form and returns its typed hash: "sha256:" followed by the 64
// lowercase hex digits of the SHA-256 of the canonical bytes (canonical-3). It refuses what Form
// refuses, so a typed hash is computed only from bytes the canonical form accepts.
func TypedHash(input []byte) (string, error) {
	b, err := Form(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return HashPrefix + hex.EncodeToString(sum[:]), nil
}
