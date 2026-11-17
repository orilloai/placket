// Command guard runs the checks of placket's make ci that read the tree, one rule per name given:
//
//	guard <rule>...
//
// It reads the repository from the directory it runs in and changes nothing. For each rule it prints
// "<rule>: ok", or a line per fault, each starting "<rule>: <file>[:<line>]: ", and then
// "<rule>: refused: <why> (docs/rules/<file>.md#<rule>)", which links the rule's heading. It exits 1 when a rule refuses and 2 when a name is
// not a rule. docs/adr/0007-one-checker.md records why it exists and how a rule is added.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(".", os.Args[1:], os.Stdout))
}

// run checks the tree at root against each rule named, writing every rule's lines to w, and returns
// the exit status.
func run(root string, names []string, w io.Writer) int {
	if len(names) == 0 {
		fmt.Fprintln(w, "guard: name the rules to check:", ruleNames())
		return 2
	}
	t := newTree(root)
	status := 0
	for _, n := range names {
		r, ok := ruleByName(n)
		if !ok {
			fmt.Fprintf(w, "guard: %s: no such rule; the rules are: %s\n", n, ruleNames())
			return 2
		}
		lines, refused := r.run(t)
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		if refused {
			status = 1
		}
	}
	return status
}
