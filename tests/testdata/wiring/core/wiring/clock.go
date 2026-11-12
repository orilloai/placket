// Package wiring holds one fault for each check of the core.
package wiring

import clock "time"

// At reads the clock through time imported under another name.
func At() clock.Time { return clock.Now() }
