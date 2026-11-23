// Command judge-eval runs a judge's basic eval by hand: `judge-eval JUDGE CASES` asks the judge of
// the config JUDGE about every case of the file CASES, with the user's own key from PLACKET_JEV_KEY,
// and prints one line per case and the counts (docs/adr/0009-who-decided.md). make judge-eval runs it; it
// is no step of make ci, and not in the release.
package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/adapters/judge/eval"
	"github.com/orilloai/placket/adapters/judge/jev"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: judge-eval JUDGE CASES")
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail("error file judge")
	}
	cfg, _, err := judge.ReadConfig(bytes.TrimSuffix(b, []byte{'\n'}))
	if err != nil {
		fail(err.Error())
	}
	cb, err := os.ReadFile(os.Args[2])
	if err != nil {
		fail("error file cases")
	}
	cases, err := eval.ReadCases(cb)
	if err != nil {
		fail(err.Error())
	}
	if err := eval.Run(cfg, cases, jev.New(cfg, os.Getenv(jev.KeyEnv), nil), os.Stdout); err != nil {
		fail("error output")
	}
}

func fail(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(3)
}
