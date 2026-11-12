// Package zz calls the built-in panic: the panic it declares is in a file no build includes.
package zz

// Must calls the built-in.
func Must() { panic("empty") }
