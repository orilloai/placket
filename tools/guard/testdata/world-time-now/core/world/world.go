// Package world reads the world, which the core never does.
package world

import "time"

// At reads the clock.
func At() time.Time { return time.Now() }
