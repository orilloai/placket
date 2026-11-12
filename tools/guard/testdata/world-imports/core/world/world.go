// Package world imports the packages that reach the process, files, logs and memory.
package world

import (
	"io/ioutil"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"syscall"
	"unsafe"
)

var (
	_ = ioutil.Discard
	_ = log.Println
	_ = slog.Info
	_ = os.Args
	_ = exec.Command
	_ = runtime.NumCPU
	_ = debug.Stack
	_ = syscall.Getpid
	_ = unsafe.Sizeof(0)
)
