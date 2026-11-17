package cli

import (
	"github.com/orilloai/placket/adapters/local"
)

func init() {
	commands["questions"] = questionsCommand
}

// questionsCommand is `questions`, with the socket flags: the open questions this key may answer,
// one JSON object per line, {"asker","deadline","form","id","kind"} and the part: a text part's
// text or a data part's canonical JSON in message, a raw part's size in bytes in size. A text or data
// part the window's reply has no room for is shown by its size too (local.MaxReply). On a terminal
// the line is escaped, which leaves it the same JSON.
func questionsCommand(r *run, args []string) int {
	fs := flags("questions")
	keyPath, socket, accord, stateDir := socketFlags(fs)
	if !r.parse(fs, args, "key", "socket", "accord") {
		return r.tool(codeUsage)
	}
	s, code := r.open(*keyPath, *socket, *accord, *stateDir)
	if s == nil {
		return code
	}
	if err := s.dir.Lock(); err != nil {
		return r.failed(err)
	}
	defer s.dir.Unlock()
	rep, err := s.client.Call(local.OpQuestions, nil)
	if err != nil {
		return r.failed(err)
	}
	if code, refused := r.refusal(rep); refused {
		return code
	}
	list := []any{}
	defer func() { r.put("questions", list) }()
	for _, q := range rep.Questions {
		o := map[string]any{"id": q.ID, "asker": q.Asker, "kind": q.Kind, "deadline": q.Deadline, "form": q.Form}
		if q.Message != nil {
			o["message"] = *q.Message
		}
		if q.Size != nil {
			o["size"] = *q.Size
		}
		line(r.stdout, r.screenJSON(canonicalJSON(o)))
		list = append(list, o)
	}
	return ExitOK
}
