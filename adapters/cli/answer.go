package cli

import (
	"fmt"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["answer"] = answerCommand
}

// An answering is what one answer signs and sends: the question, the action, for an accept or a
// change the text and whether it asks for release, and whether the key keeps the call it signed.
type answering = client.Answering

// resolve answers the question (maybe-11) through the client (client.Resolve): for an accept, a
// change or a decline it first checks the question the window shows, and signs nothing for one it
// does not show as its ask's receipt names it (tool error asked) or on a closed Accord (the rejection
// closed). It prints the outcome, then the release's, and returns the reply and what it signed, or
// nil and the exit code of a refusal.
func (s *session) resolve(a answering) (*local.Reply, client.Signed, int) {
	rep, signed, err := s.c.Resolve(s.r.env.Context, a)
	if err != nil {
		return nil, client.Signed{}, s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		return nil, client.Signed{}, code
	}
	s.r.put(memberQuestion, a.Question)
	s.r.outcome(rep.Outcome, false, memberOutcome)
	if rep.Release != nil {
		s.r.outcome(rep.Release, true, memberRelease)
	}
	return &rep, signed, 0
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
	defer s.r.nest(memberComment)()
	s.r.put(memberCrossed, false)
	to, ok := asker(rep)
	if !ok {
		return s.r.tool(string(client.CodeReply))
	}
	out, code := s.send(client.Message{To: to, Kind: "comment", Body: client.Body{Form: client.FormText, Text: text}, Re: question}, true)
	if out == nil {
		fmt.Fprintln(s.r.stderr, LineCommentNotCross)
		return code
	}
	if out.Verdict != string(composition.Pass) {
		fmt.Fprintln(s.r.stderr, LineCommentNotCross)
		return ExitBlock
	}
	s.r.put(memberCrossed, true)
	return ExitOK
}

// answerFlags are the flags of answer, change, decline and cancel. It opens the session and takes
// the state lock, which the command keeps until it returns (state-lock): a version it keeps and a
// comment it gives are written under it, after any line a stop left unterminated is removed.
func answerFlags(r *run, name string, args []string, more func(fs *flagSet)) (*session, string, int) {
	fs := newFlags(name)
	so := socketFlags(fs)
	question := fs.text("question")
	if more != nil {
		more(fs)
	}
	// A question id that is not UTF-8 is refused, never repaired (parse-strict).
	if !r.parseSocket(fs, args, so) || !allUTF8(*question) {
		return nil, "", r.tool(codeUsage)
	}
	s, code := r.open(so)
	if s == nil {
		return nil, "", code
	}
	if err := s.c.Lock(); err != nil {
		return nil, "", r.failed(err)
	}
	return s, *question, 0
}

// answerCommand is `answer --question ID [--text S] [--no-release] [--keep]`: an accept, the text
// accepted unless given, asking for release unless --no-release. With --keep the key keeps the call
// it signed and the salt it minted. It prints the answer's outcome, then the release's.
func answerCommand(r *run, args []string) int {
	var text *string
	var noRelease, keep *bool
	s, question, code := answerFlags(r, "answer", args, func(fs *flagSet) {
		text = fs.text("text")
		noRelease, keep = fs.switchFlag("no-release"), fs.switchFlag("keep")
	})
	if s == nil {
		return code
	}
	defer s.c.Unlock()
	if !utf8.ValidString(*text) {
		return r.tool(codeText)
	}
	rep, _, code := s.resolve(answering{Question: question, Action: answer.Accepted, Text: *text, Release: !*noRelease, Keep: *keep})
	if rep == nil {
		return code
	}
	if passed(rep) {
		return ExitOK
	}
	return ExitBlock
}
