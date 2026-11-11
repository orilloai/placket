package c

import (
	"C"
)

// Pid asks C for the process.
func Pid() int { return int(C.getpid()) }
