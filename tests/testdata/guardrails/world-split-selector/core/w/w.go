// Package w reads the clock and prints with a selector split after its dot.
package w

import (
	"fmt"
	"time"
)

// Now breaks the line after the dot.
func Now() time.Time {
	return time.
		Now()
}

// Later puts a comment after the dot.
func Later() time.Time { return time./**/Now() }

// Say puts a line comment between the dot and the name.
func Say() {
	fmt.
		// the name follows
		Println("hello")
}
