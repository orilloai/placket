package cli

import (
	"bytes"
	"encoding/json"
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
// --no-words] [--all]`: the export, one row per question, and with --all one for every crossing
// besides, in journal order (log-1, log-2). The journal's answers.jsonl, when it has one, is read
// beside its receipts, for answer_signed. JSON carries the claim and the scope line; with
// CSV they are printed on standard error. On a terminal the cells' control characters are escaped.
// A journal that does not verify prints no row and exits 3.
func logCommand(r *run, args []string) int {
	fs := newFlags("log")
	med, dir, format := fs.text("mediator"), fs.text("receipts"), fs.text("format")
	withWords, noWords := fs.switchFlag("words"), fs.switchFlag("no-words")
	all := fs.switchFlag("all")
	var kept many
	fs.list(&kept, "kept")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	if (*format != "json" && *format != "csv") || (*withWords && *noWords) || (r.asJSON && *format == "csv") {
		return r.tool(codeFormat)
	}
	list, err := journal.ReadReceipts(*dir, r.stderr)
	if err != nil {
		return r.failed(err)
	}
	answers, err := journal.ReadAnswers(*dir, r.stderr)
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
	build := export.Build
	if *all {
		build = export.BuildAll
	}
	lg, err := build(*med, list, lines, answers, func(s string) { fmt.Fprintln(r.stderr, s) })
	if err != nil {
		var ee *export.Error
		if errors.As(err, &ee) && ee.Index >= 0 {
			// The verifier's code, or the export's own when the receipts verified but hold more
			// than one Accord or card.
			code := ee.Core
			if ee.Code != export.CodeVerify {
				code = string(ee.Code)
			}
			return r.tool(codeVerify, code, journal.FileReceipts, fmt.Sprintf("line %d", ee.Index+1))
		}
		return r.tool(codeVerify)
	}
	if *format == "json" {
		b := lg.JSON(ClaimVerified, Scope, *withWords, *noWords)
		line(r.stdout, r.screenJSON(b))
		r.put("log", json.RawMessage(b))
		return ExitOK
	}
	var b bytes.Buffer
	if err := lg.CSV(&b, *withWords, *noWords); err != nil {
		return r.tool(codeFile)
	}
	_, _ = r.stdout.Write(r.screen(b.Bytes()))
	fmt.Fprintln(r.stderr, ClaimVerified)
	fmt.Fprintln(r.stderr, Scope)
	return ExitOK
}
