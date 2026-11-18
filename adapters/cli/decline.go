package cli

import (
	"unicode/utf8"

	"github.com/orilloai/placket/core/answer"
)

func init() {
	commands["decline"] = declineCommand
}

// declineCommand is `decline --question ID --comment S`: a decline, then the comment given to the
// asker. With no comment, or one that is not UTF-8, it is tool error comment and sends nothing. When
// the comment does not cross, it says so.
func declineCommand(r *run, args []string) int {
	var comment *string
	s, question, code := answerFlags(r, "decline", args, func(fs *flagSet) { comment = fs.String("comment", "", "the comment") })
	if s == nil {
		return code
	}
	defer s.dir.Unlock()
	// The comment is given only after the decline is sent, so it is checked first (parse-strict).
	if *comment == "" || !utf8.ValidString(*comment) {
		return r.tool(codeComment)
	}
	rep, _, code := s.resolve(answering{question: question, action: answer.Declined})
	if rep == nil {
		return code
	}
	return s.comment(rep, question, *comment)
}
