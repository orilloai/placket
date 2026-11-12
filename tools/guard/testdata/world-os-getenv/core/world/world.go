// Package world reads the environment.
package world

import "os"

// Home reads a variable.
func Home() string { return os.Getenv("HOME") }
