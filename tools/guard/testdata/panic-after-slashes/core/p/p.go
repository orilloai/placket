// Package p holds two slashes in a string before a call.
package p

// Pair holds two functions.
var Pair = [2]func() string{func() string { return "//" }, func() string { panic("no") }}
