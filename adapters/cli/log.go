package cli

import (
	"errors"
	"fmt"

	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/state"
)

func init() {
	commands["log"] = logCommand
}

// logCommand is `log --mediator M --receipts DIR [--kept DIR...] [--format json|csv] [--words |
// --no-words]`: the export, one row per question. JSON carries the claim and the scope line; with
// CSV they are printed on standard error. A journal that does not verify prints no row and exits 3.
func logCommand(r *run, args []string) int {
	fs := flags("log")
	med, dir, format := fs.String("mediator", "", "mediator"), fs.String("receipts", "", "journal directory"), fs.String("format", "json", "json or csv")
	withWords, noWords := fs.Bool("words", false, "add the texts"), fs.Bool("no-words", false, "empty the comments")
	var kept many
	fs.Var(&kept, "kept", "a state directory")
	if !r.parse(fs, args, "mediator", "receipts") {
		return r.tool(codeUsage)
	}
	if (*format != "json" && *format != "csv") || (*withWords && *noWords) {
		return r.tool(codeFormat)
	}
	list, err := journal.ReadReceipts(*dir)
	if err != nil {
		return r.failed(err)
	}
	var lines [][]byte
	for _, path := range kept {
		d, err := state.Read(path)
		if err != nil {
			return r.failed(err)
		}
		k, err := d.Kept()
		if err != nil {
			return r.failed(err)
		}
		lines = append(lines, k...)
	}
	lg, err := export.Build(*med, list, lines, func(s string) { fmt.Fprintln(r.stderr, s) })
	if err != nil {
		var ee *export.Error
		if errors.As(err, &ee) && ee.Index >= 0 {
			return r.tool(codeVerify, ee.Core, journal.FileReceipts, fmt.Sprintf("line %d", ee.Index+1))
		}
		return r.tool(codeVerify)
	}
	if *format == "json" {
		line(r.stdout, lg.JSON(ClaimVerified, Scope, *withWords, *noWords))
		return ExitOK
	}
	if err := lg.CSV(r.stdout, *withWords, *noWords); err != nil {
		return r.tool(codeFile)
	}
	fmt.Fprintln(r.stderr, ClaimVerified)
	fmt.Fprintln(r.stderr, Scope)
	return ExitOK
}
