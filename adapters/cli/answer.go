package cli

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["answer"] = answerCommand
}

// A signedAnswer is what resolve signed and minted: the answer call's bytes and the answer's salt,
// nil for a decline or a cancel.
type signedAnswer struct{ call, salt []byte }

// An answering is what one answer signs and sends: the question, the action, for an accept or a
// change the text and whether it asks for release, and whether the key keeps the call it signed.
type answering struct {
	question string
	action   answer.Action
	text     string
	release  bool
	keep     bool
}

// shown reads the question as the window shows it to this key's answerer and checks it before
// anything is signed: the ask's receipt names the question, under this Accord, and its subject
// digest is the salted digest of the body shown under the salt shown (crossing-3, maybe-1). A
// question the window does not show is left to the mediator to refuse, unless the window left some
// out of its reply. It returns 0, or the exit code of a refusal or of tool error asked.
func (s *session) shown(question string) int {
	rep, err := s.client.Call(local.OpQuestions, nil)
	if err != nil {
		return s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		return code
	}
	for _, q := range rep.Questions {
		if q.ID != question {
			continue
		}
		st, err := receipts.Decode(q.Receipt)
		if err != nil || st.ID != question || st.Predicate.Accord != s.client.Accord || st.Subject.Name != q.Kind ||
			receipts.Digest(q.Salt, askedBody(q)) != st.Subject.Digest {
			return s.r.tool(codeAsked)
		}
		return 0
	}
	if rep.More {
		return s.r.tool(codeAsked)
	}
	return 0
}

// askedBody is the bytes of an asked part's body as the digest reads them: a text's UTF-8, a data
// body's canonical bytes, a raw body's bytes.
func askedBody(q local.Asked) []byte {
	if q.Raw != nil {
		return q.Raw
	}
	if q.Message != nil {
		return []byte(*q.Message)
	}
	return nil
}

// resolve answers the question (maybe-11): for an accept, a change or a decline it first checks the
// question the window shows (shown); for an accept or a change it mints the answer's salt, 32
// random bytes; it signs the answer call with the key, the answer's salted digest in it and never
// its text; with keep it keeps the call and the salt; and it sends the call, with the text and the
// salt beside it. It prints the outcome, then the release's, and returns the reply and what it
// signed, or nil and the exit code of a refusal.
func (s *session) resolve(a answering) (*local.Reply, signedAnswer, int) {
	if a.action != answer.Cancelled {
		if code := s.shown(a.question); code != 0 {
			return nil, signedAnswer{}, code
		}
	}
	call := answer.Call{Accord: s.client.Accord, Question: a.question, Action: a.action}
	var salt []byte
	if a.action == answer.Accepted || a.action == answer.Changed {
		salt = make([]byte, mediator.SaltBytes)
		if _, err := io.ReadFull(s.r.env.Rand, salt); err != nil {
			return nil, signedAnswer{}, s.r.tool(codeRand)
		}
		call.Digest, call.Release = receipts.Digest(salt, []byte(a.text)), a.release
	}
	signed, err := answer.Sign(call, s.key)
	if err != nil {
		return nil, signedAnswer{}, s.r.tool(codeInternal)
	}
	if a.keep && salt != nil {
		if err := s.dir.Keep(canonicalJSON(map[string]any{"answer": json.RawMessage(signed), "salt": hex.EncodeToString(salt)})); err != nil {
			return nil, signedAnswer{}, s.r.failed(err)
		}
	}
	rep, err := s.client.Call(local.OpResolve, local.ResolveArgs(signed, a.text, salt))
	if err != nil {
		return nil, signedAnswer{}, s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		return nil, signedAnswer{}, code
	}
	if rep.Outcome == nil {
		return nil, signedAnswer{}, s.r.tool(codeReply)
	}
	s.r.put(memberQuestion, a.question)
	s.r.outcome(rep.Outcome, false, memberOutcome)
	if rep.Release != nil {
		s.r.outcome(rep.Release, true, memberRelease)
	}
	return &rep, signedAnswer{call: signed, salt: salt}, 0
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
		return s.r.tool(codeReply)
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
	s.r.put(memberCrossed, true)
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
	// A question id that is not UTF-8 is refused, never repaired (parse-strict).
	if !r.parse(fs, args, "key", "socket", "accord", "question") || !allUTF8(*question) {
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

// answerCommand is `answer --question ID [--text S] [--no-release] [--keep]`: an accept, the text
// accepted unless given, asking for release unless --no-release. With --keep the key keeps the call
// it signed and the salt it minted. It prints the answer's outcome, then the release's.
func answerCommand(r *run, args []string) int {
	var text *string
	var noRelease, keep *bool
	s, question, code := answerFlags(r, "answer", args, func(fs *flagSet) {
		text = fs.String("text", WordAccepted, "the answer")
		noRelease, keep = fs.Bool("no-release", false, "ask for no release"), fs.Bool("keep", false, "keep the signed call")
	})
	if s == nil {
		return code
	}
	defer s.dir.Unlock()
	if !utf8.ValidString(*text) {
		return r.tool(codeText)
	}
	rep, _, code := s.resolve(answering{question: question, action: answer.Accepted, text: *text, release: !*noRelease, keep: *keep})
	if rep == nil {
		return code
	}
	if passed(rep) {
		return ExitOK
	}
	return ExitBlock
}
