// Package calc is a fixture: Sub is wrong at the base, and Mul is not there yet.
package calc

// Add returns a plus b.
func Add(a, b int) int { return a + b }

// Sub returns a minus b, but not at the base.
func Sub(a, b int) int { return a + b }
