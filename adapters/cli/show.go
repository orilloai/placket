package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["show"] = showCommand
}

// showCommand is `show --receipt R`: the one receipt in the file R, decoded without verifying it.
// It prints the core's reading on standard output, its decider on the line after its verdict
// (receipt-8, receipt-11), and on standard error, in order: not verified;
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
	module := fmt.Sprintf(ModuleLine, p.Card)
	fmt.Fprintln(r.stderr, LineNotVerified)
	fmt.Fprintln(r.stderr, word(p.Verdict)+" "+p.Reason+" "+st.Subject.Name+" "+p.At)
	fmt.Fprintln(r.stderr, "from "+p.From+" to "+p.To)
	fmt.Fprintln(r.stderr, module)
	fmt.Fprintln(r.stderr, claim(p))
	fmt.Fprintln(r.stderr, Scope)
	for _, l := range lines {
		fmt.Fprintln(r.stdout, l)
	}
	by, _ := member(predicateOf(b), "by")
	r.put("by", json.RawMessage(by))
	for name, v := range map[string]any{
		"verified": false, memberReceiptID: st.ID, memberWord: word(p.Verdict), memberReason: p.Reason, "kind": st.Subject.Name,
		"at": p.At, "from": p.From, "to": p.To, LabelModule: module, memberClaim: claim(p), memberNotClaimed: Scope, "shown": lines,
	} {
		r.put(name, v)
	}
	return ExitOK
}
