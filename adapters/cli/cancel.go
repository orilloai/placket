package cli

import "github.com/orilloai/placket/core/mediator"

func init() {
	commands["cancel"] = cancelCommand
}

// cancelCommand is `cancel --question ID`: a cancel. It prints the outcome.
func cancelCommand(r *run, args []string) int {
	s, question, code := answerFlags(r, "cancel", args, nil)
	if s == nil {
		return code
	}
	if rep, code := s.resolve(question, mediator.Cancelled, ""); rep == nil {
		return code
	}
	return ExitOK
}
