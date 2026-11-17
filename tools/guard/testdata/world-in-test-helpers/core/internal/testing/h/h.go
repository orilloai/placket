// Package h is a test helper: only tests import it, so it may read files.
package h

import "os"

// Read reads a vector for a test.
func Read(name string) ([]byte, error) { return os.ReadFile(name) }
