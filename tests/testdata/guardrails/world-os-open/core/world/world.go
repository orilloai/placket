// Package world reads a file.
package world

import "os"

// Read opens a file.
func Read() (*os.File, error) { return os.Open("seed") }
