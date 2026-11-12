// Package elsewhere is built into the core through a linked directory.
package elsewhere

import "time"

// Now reads the clock.
func Now() time.Time { return time.Now() }
