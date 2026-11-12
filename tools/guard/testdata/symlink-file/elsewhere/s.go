// Package s is built into the core through a symbolic link.
package s

import "os"

// Env reads a variable.
func Env() string { return os.Getenv("HOME") }
