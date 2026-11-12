//go:build windows

// Package u is built only on Windows, so go list on any other host finds no package here; its
// format is still checked.
package u

func  F() int { return 1 }
