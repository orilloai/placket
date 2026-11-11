package clock_test

import (
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"log"
	mrand "math/rand"
	rand2 "math/rand/v2"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/orilloai/placket/core/clock"
)

// The calls the core refuses are allowed in a test file. An example, so that go test runs it.
func ExampleLater() {
	now := time.Now()
	fmt.Print(clock.Later(now, now))
	b := make([]byte, 8)
	_, _ = crand.Read(b)
	_ = mrand.Int()
	_ = rand2.Int()
	_ = os.Getenv("HOME")
	if f, err := os.Open(os.DevNull); err == nil {
		_ = f.Close()
	}
	_ = net.ParseIP("::1")
	fmt.Printf("%s", "")
	fmt.Println()
	_ = time.Since(now)
	_ = ioutil.Discard
	log.SetOutput(ioutil.Discard)
	_ = runtime.NumCPU()
	_ = syscall.Getpid()
	_ = unsafe.Sizeof(b)
	print("")
	defer func() { _ = recover() }()
	panic("a test may panic")
	// Output: false
}
