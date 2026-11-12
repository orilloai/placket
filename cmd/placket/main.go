// Command placket is the command line of placket: a window that mediates one Accord, and every
// party's commands. Package cli holds every command; main passes in the world.
package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/orilloai/placket/adapters/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	env := cli.Env{Now: time.Now, Rand: rand.Reader, Context: ctx}
	code := cli.Run(env, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
