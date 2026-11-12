// Package d imports time and fmt into its own scope.
package d

import (
	. "fmt"
	. "time"
)

// At reads the clock with no package name in front.
func At() Time { Println("now"); return Now() }
