// Package v formats a string as a number.
package v

import "fmt"

// Wrong formats s with the verb for an integer.
func Wrong(s string) string { return fmt.Sprintf("%d", s) }
