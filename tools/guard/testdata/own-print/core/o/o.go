// Package o has a function of its own named print, which is not the built-in.
package o

// Line prints nothing: it calls the package's own print.
func Line(s string) string { return print(s) }
