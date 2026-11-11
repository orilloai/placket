// Package world draws randomness under another name.
package world

import cr "crypto/rand"

// Salt fills b.
func Salt(b []byte) { _, _ = cr.Read(b) }
