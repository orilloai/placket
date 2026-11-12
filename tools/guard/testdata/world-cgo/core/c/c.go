// Package c reads the environment through cgo.
package c

// #include <stdlib.h>
import "C"

// Env reads a variable.
func Env() string { return C.GoString(C.getenv(C.CString("HOME"))) }
