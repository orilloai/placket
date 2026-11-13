package cli

import (
	"bytes"
	"fmt"
	"os"

	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["show"] = showCommand
}

// showCommand is `show --receipt R`: the one receipt in the file R, decoded without verifying it.
// It prints the core's reading on standard output, and on standard error, in order: not verified;
// <word> <reason> <kind> <at>; from <ref> to <ref>; the module line; the receipt's claim line; the
// scope line.
func showCommand(r *run, args []string) int {
	fs := flags("show")
	path := fs.String("receipt", "", "a file holding one receipt")
	if !r.parse(fs, args, "receipt") {
		return r.tool(codeUsage)
	}
	b, err := os.ReadFile(*path)
	if err != nil {
		return r.tool(codeFile)
	}
	b = bytes.TrimSuffix(b, []byte{'\n'})
	lines, err := receipts.Show(b)
	if err != nil {
		return r.notVerified(err)
	}
	st, _ := receipts.Decode(b)
	p := st.Predicate
	fmt.Fprintln(r.stderr, LineNotVerified)
	fmt.Fprintln(r.stderr, word(p.Verdict)+" "+p.Reason+" "+st.Subject.Name+" "+p.At)
	fmt.Fprintln(r.stderr, "from "+p.From+" to "+p.To)
	fmt.Fprintf(r.stderr, ModuleLine+"\n", p.Card)
	fmt.Fprintln(r.stderr, claim(p))
	fmt.Fprintln(r.stderr, Scope)
	for _, l := range lines {
		fmt.Fprintln(r.stdout, l)
	}
	return ExitOK
}
