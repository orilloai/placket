//go:build windows

// Package w reads the clock in a file built only for another system.
package w

import "time"

// At reads the clock.
func At() time.Time { return time.Now() }
