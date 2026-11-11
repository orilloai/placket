// Package world reads the clock and prints, in the forms beyond time.Now and fmt.Print.
package world

import "time"

// Calls reaches the world.
func Calls(t time.Time) {
	_ = time.Since(t)
	_ = time.Until(t)
	_ = time.After(time.Second)
	_ = time.AfterFunc(time.Second, func() {})
	_ = time.Tick(time.Second)
	_ = time.NewTimer(time.Second)
	_ = time.NewTicker(time.Second)
	time.Sleep(time.Second)
	print("x")
	println("x")
}
