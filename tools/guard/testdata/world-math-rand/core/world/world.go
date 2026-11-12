// Package world draws randomness.
package world

import "math/rand"

// N draws a number.
func N() int { return rand.Int() }
