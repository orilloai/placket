package cli

import "github.com/orilloai/placket/core/answer"

func init() {
	commands["cancel"] = cancelCommand
}

// cancelCommand is `cancel --question ID`: the asker's cancel of its own question, a call the asker
// signs (maybe-6). It prints the outcome.
func cancelCommand(r *run, args []string) int {
	s, question, code := answerFlags(r, "cancel", args, nil)
	if s == nil {
		return code
	}
	defer s.dir.Unlock()
	if rep, _, code := s.resolve(answering{question: question, action: answer.Cancelled}); rep == nil {
		return code
	}
	return ExitOK
}
