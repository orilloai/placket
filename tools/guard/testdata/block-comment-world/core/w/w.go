// Package w hides world access behind block comments whose second line starts with //.
package w

import (
	/*
		// */"net"
	"time"
)

var _ = net.IPv4len

// Now reads the clock.
func Now() time.Time {
	/*
		// */return time.Now()
}
