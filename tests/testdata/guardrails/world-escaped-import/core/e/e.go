// Package e names os and net with escapes in their import paths.
package e

import o "\x6fs"

// Env reads a variable.
func Env() string { return o.Getenv("HOME") }
