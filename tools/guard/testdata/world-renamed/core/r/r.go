// Package r reaches the clock and prints through time and fmt imported under other names.
package r

import (
	say "fmt"
	clock "time"
)

// At reads the clock.
func At() clock.Time { return clock.Now() }

// Later holds the clock's function without calling it.
var Later = clock.Since

// Hello prints.
func Hello() { say.Println("hello") }

// Format only formats, which is allowed.
func Format() string { return say.Sprint(clock.RFC3339) }
