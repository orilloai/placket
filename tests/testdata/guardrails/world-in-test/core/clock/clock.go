// Package clock takes the time as a value.
package clock

import "time"

// Later reports whether a is after b.
func Later(a, b time.Time) bool { return a.After(b) }
