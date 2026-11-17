package cli

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"unicode/utf8"

	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["change"] = changeCommand
}

// changeCommand is `change --question ID (--text S | --file F) [--comment S] [--keep]`: a change,
// the version given as text. With no version, an empty one, or a file that is not UTF-8, it is tool
// error changed and sends nothing. It prints the outcome of the version returned to the asker, then
// of the version released; with --keep it keeps the version with the salt the reply carries; with
// --comment it then gives the asker a comment; a comment that is not UTF-8 is tool error comment and
// sends nothing.
func changeCommand(r *run, args []string) int {
	var text, file, comment *string
	var keep *bool
	s, question, code := answerFlags(r, "change", args, func(fs *flagSet) {
		text, file = fs.String("text", "", "the version"), fs.String("file", "", "a file holding the version")
		comment, keep = fs.String("comment", "", "a comment"), fs.Bool("keep", false, "keep the version")
	})
	if s == nil {
		return code
	}
	defer s.dir.Unlock()
	if !utf8.ValidString(*text) {
		return r.tool(codeText)
	}
	if !utf8.ValidString(*comment) {
		return r.tool(codeComment)
	}
	version := *text
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil || !utf8.Valid(b) || *text != "" {
			return r.tool(codeChanged)
		}
		version = string(b)
	}
	if version == "" {
		return r.tool(codeChanged)
	}
	rep, code := s.resolve(question, mediator.Changed, version)
	if rep == nil {
		return code
	}
	if *keep && rep.Outcome.Verdict == string(composition.Pass) && rep.Salt != nil {
		k := canonicalJSON(map[string]any{
			"receipt": json.RawMessage(rep.Outcome.Receipt), "receiptId": rep.Outcome.ReceiptID,
			"messageId": changeMessage + question, "kind": receipts.NameChanged, "salt": hex.EncodeToString(rep.Salt),
			"re": question, "text": version,
		})
		if err := s.dir.Keep(k); err != nil {
			return r.failed(err)
		}
	}
	code = ExitOK
	if !passed(rep) {
		code = ExitBlock
	}
	if *comment != "" {
		if c := s.comment(rep, question, *comment); c != ExitOK {
			return c
		}
	}
	return code
}
