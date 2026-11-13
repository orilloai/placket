package cli

import (
	"fmt"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["answer"] = answerCommand
}

// resolve sends an answer to the question and prints its outcome, then its release's. It returns the
// reply, or nil and the exit code of a refusal.
func (s *session) resolve(question string, action mediator.Action, answer string) (*local.Reply, int) {
	rep, err := s.client.Call(local.OpResolve, local.ResolveArgs(question, action, answer))
	if err != nil {
		return nil, s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		return nil, code
	}
	if rep.Outcome == nil {
		return nil, s.r.tool("reply")
	}
	s.r.outcome(rep.Outcome, false)
	if rep.Release != nil {
		s.r.outcome(rep.Release, true)
	}
	return &rep, 0
}

// passed reports whether an answer and its release, when it has one, both passed.
func passed(rep *local.Reply) bool {
	return rep.Outcome.Verdict == string(composition.Pass) && (rep.Release == nil || rep.Release.Verdict == string(composition.Pass))
}

// asker is the question's asker, the closing receipt's to.
func asker(rep *local.Reply) (string, bool) {
	st, err := receipts.Decode(rep.Outcome.Receipt)
	if err != nil {
		return "", false
	}
	return st.Predicate.To, true
}

// comment gives the asker a comment on the question, of kind comment with re the question, kept by
// the giver so that the log can show it, under the state lock the command holds. It returns the exit code: 1 when the comment did not
// cross, which it says.
func (s *session) comment(rep *local.Reply, question, text string) int {
	to, ok := asker(rep)
	if !ok {
		return s.r.tool("reply")
	}
	out, code := s.send(to, "comment", body{form: "text", value: text}, question, "", true)
	if out == nil {
		fmt.Fprintln(s.r.stderr, LineCommentNotCross)
		return code
	}
	if out.Verdict != string(composition.Pass) {
		fmt.Fprintln(s.r.stderr, LineCommentNotCross)
		return ExitBlock
	}
	return ExitOK
}

// answerFlags are the flags of answer, change, decline and cancel. It opens the session and takes
// the state lock, which the command keeps until it returns (state-lock): a version it keeps and a
// comment it gives are written under it, after any line a stop left unterminated is removed.
func answerFlags(r *run, name string, args []string, more func(fs *flagSet)) (*session, string, int) {
	fs := flags(name)
	keyPath, socket, accord, stateDir := socketFlags(fs)
	question := fs.String("question", "", "the question's id")
	if more != nil {
		more(fs)
	}
	if !r.parse(fs, args, "key", "socket", "accord", "question") {
		return nil, "", r.tool(codeUsage)
	}
	s, code := r.open(*keyPath, *socket, *accord, *stateDir)
	if s == nil {
		return nil, "", code
	}
	if err := s.dir.Lock(); err != nil {
		return nil, "", r.failed(err)
	}
	return s, *question, 0
}

// answerCommand is `answer --question ID [--text S]`: an accept, the text accepted unless given. It
// prints the answer's outcome, then the release's.
func answerCommand(r *run, args []string) int {
	var text *string
	s, question, code := answerFlags(r, "answer", args, func(fs *flagSet) { text = fs.String("text", WordAccepted, "the answer") })
	if s == nil {
		return code
	}
	defer s.dir.Unlock()
	rep, code := s.resolve(question, mediator.Accepted, *text)
	if rep == nil {
		return code
	}
	if passed(rep) {
		return ExitOK
	}
	return ExitBlock
}
