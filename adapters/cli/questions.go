package cli

import (
	"encoding/json"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
)

func init() {
	commands["questions"] = questionsCommand
}

// questionsCommand is `questions [--wait N]`, with the socket flags: the open questions this key may answer,
// one JSON object per line, {"asker","deadline","form","id","kind"} and the part: a text part's
// text or a data part's canonical JSON in message, a raw part's size in bytes in size, never its
// bytes. Under --json each question also has receiptId and receipt, the ask's, and never the part's
// salt or a raw body. When the window left questions out of its reply, to keep it under
// local.MaxReply, the oldest come first and the rest show once those close. Each question is printed
// only when it is the part its ask's receipt names (asked); when one is not, it is left out and the
// command ends in tool error asked after the others. On a terminal the line is escaped, which
// leaves it the same JSON. It reads no seq and writes nothing, so it holds no lock (state-lock); with
// --wait the door holds the call, at most N seconds, until there is a question to show.
func questionsCommand(r *run, args []string) int {
	fs := flags("questions")
	so := socketFlags(fs)
	wait := fs.Int("wait", 0, "seconds to wait for a question, at most 60")
	if !r.parseSocket(fs, args, so) || *wait < 0 || *wait > local.MaxWait {
		return r.tool(codeUsage)
	}
	s, code := r.open(so)
	if s == nil {
		return code
	}
	shown, err := s.c.Questions(r.env.Context, *wait, "")
	if err != nil {
		return r.failed(err)
	}
	if code, refused := r.refusal(shown.Reply); refused {
		return code
	}
	list := []any{}
	defer func() { r.put("questions", list) }()
	for _, q := range shown.Questions {
		o := map[string]any{"id": q.ID, "asker": q.Asker, "kind": q.Kind, "deadline": q.Deadline, "form": q.Form}
		switch {
		case q.Message != nil:
			o["message"] = *q.Message
		case q.Size != nil:
			o["size"] = *q.Size
		}
		line(r.stdout, r.screenJSON(canonicalJSON(o)))
		j := map[string]any{memberReceiptID: q.ID, memberReceipt: json.RawMessage(q.Receipt)}
		for k, v := range o {
			j[k] = v
		}
		list = append(list, j)
	}
	if len(shown.Misstated) > 0 {
		return r.tool(string(client.CodeAsked))
	}
	return ExitOK
}
