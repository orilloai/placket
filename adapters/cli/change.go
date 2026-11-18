package cli

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"unicode/utf8"

	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["change"] = changeCommand
}

// changeCommand is `change --question ID (--text S | --file F) [--comment S] [--keep]
// [--no-release]`: a change, the version given as text, asking for release unless --no-release. With
// no version, an empty one, or a file that is not UTF-8, it is tool error changed and sends nothing.
// It prints the outcome of the version returned to the asker, then of the version released; with
// --keep it keeps the call it signed and the salt it minted, and the version as the asker's delivery
// holds it; with --comment it then gives the asker a comment; a comment that is not UTF-8 is tool
// error comment and sends nothing.
func changeCommand(r *run, args []string) int {
	var text, file, comment *string
	var keep, noRelease *bool
	s, question, code := answerFlags(r, "change", args, func(fs *flagSet) {
		text, file = fs.String("text", "", "the version"), fs.String("file", "", "a file holding the version")
		comment, keep = fs.String("comment", "", "a comment"), fs.Bool("keep", false, "keep the version")
		noRelease = fs.Bool("no-release", false, "ask for no release")
	})
	if s == nil {
		return code
	}
	defer s.c.Unlock()
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
	rep, signed, code := s.resolve(answering{Question: question, Action: answer.Changed, Text: version, Release: !*noRelease, Keep: *keep})
	if rep == nil {
		return code
	}
	if *keep && rep.Outcome.Verdict == string(composition.Pass) {
		k := canonicalJSON(map[string]any{
			"receipt": json.RawMessage(rep.Outcome.Receipt), "receiptId": rep.Outcome.ReceiptID,
			"messageId": changeMessage + question, "kind": receipts.NameChanged, "salt": hex.EncodeToString(signed.Salt),
			"re": question, "text": version, "answer": json.RawMessage(signed.Call),
		})
		if err := s.c.Dir().Keep(k); err != nil {
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
