package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/errtext"
	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// The arguments of each tool, as strict reads them: every member the tool's input schema names.
type (
	giveArgs struct {
		Mediator *string         `json:"mediator"`
		To       *string         `json:"to"`
		Kind     *string         `json:"kind"`
		Text     *string         `json:"text"`
		Data     json.RawMessage `json:"data"`
		Re       *string         `json:"re"`
		Topic    *string         `json:"topic"`
	}
	getArgs struct {
		Mediator *string `json:"mediator"`
		Wait     *int64  `json:"wait"`
	}
	questionsArgs struct {
		Mediator *string `json:"mediator"`
		After    *string `json:"after"`
		Wait     *int64  `json:"wait"`
	}
	resultArgs struct {
		Mediator *string `json:"mediator"`
		Question *string `json:"question"`
		Wait     *int64  `json:"wait"`
	}
	cancelArgs struct {
		Mediator *string `json:"mediator"`
		Question *string `json:"question"`
	}
	noArgs struct{}
)

var (
	actorRE  = regexp.MustCompile(`^ed25519:[0-9a-f]{64}$`)
	kindRE   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	hashRE   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	cursorRE = regexp.MustCompile(`^[a-z0-9.-]{1,128}$`)
)

// badArguments is the tool error of arguments that fail the tool's input schema or the wait cap.
func badArguments() map[string]any { return toolError(errtext.FromMCP, string(CodeBadArguments)) }

// toolError is a tool's result that is an error: its text is the code and its sentence, from the row
// it arrived under (errtext.Text).
func toolError(from, code string) map[string]any {
	return map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": errtext.Text(from, code)}}}
}

// toolResult is a tool's result: the object as text, and as structured content.
func toolResult(v map[string]any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError(errtext.FromMCP, string(CodeInternal))
	}
	if c, err := canonical.Form(b); err == nil {
		b = c
	}
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}, "structuredContent": v}
}

// failed is the tool error of an error a client returned.
func failed(err error) map[string]any { return toolError(from(err)) }

// failedWith is the tool error of an error a client returned beside what the call still produced,
// such as a delivery the window handed over whose record could not be written: the error's text,
// then the object as text, and the object as structured content.
func failedWith(err error, v map[string]any) map[string]any {
	r := failed(err)
	b, merr := json.Marshal(v)
	if merr != nil {
		return r
	}
	if c, cerr := canonical.Form(b); cerr == nil {
		b = c
	}
	r["content"] = append(r["content"].([]any), map[string]any{"type": "text", "text": string(b)})
	r["structuredContent"] = v
	return r
}

// refusal is the tool error of a reply that is a refusal, under its member's row; nil for any other.
func refusal(rep local.Reply) map[string]any {
	if member, code, ok := client.Refused(rep); ok {
		return toolError(member, code)
	}
	return nil
}

// waitOf reads a wait: absent is 0; from 0 to local.MaxWait, the cap, is itself.
func waitOf(w *int64) (int, bool) {
	if w == nil {
		return 0, true
	}
	return int(*w), *w >= 0 && *w <= local.MaxWait
}

// call runs one tool with its arguments, as the input schema reads them: a member the schema does
// not name, one of another type or form, or a wait above 60, is CodeBadArguments, and nothing is
// called. Then the mediator is named: one not served is CodeUnknownMediator, and a served one that
// was refused at start is its code.
func (s *Server) call(ctx context.Context, name string, raw json.RawMessage, list []*served) map[string]any {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		raw = json.RawMessage(`{}`)
	}
	if name == ToolMediators {
		if !strict.Decode(raw, &noArgs{}) {
			return badArguments()
		}
		return s.mediators(list)
	}
	open := func(mediator *string) (*client.Client, map[string]any) {
		if mediator == nil || *mediator == "" {
			return nil, badArguments()
		}
		for _, v := range list {
			if v.name != *mediator {
				continue
			}
			if !v.accepted {
				return nil, toolError(v.from, v.code)
			}
			c, err := client.Open(v.m, s.Options)
			if err != nil {
				return nil, failed(err)
			}
			return c, nil
		}
		return nil, toolError(errtext.FromClient, string(client.CodeUnknownMediator))
	}
	switch name {
	case ToolGive:
		var a giveArgs
		if !strict.Decode(raw, &a) || a.To == nil || !actorRE.MatchString(*a.To) || a.Kind == nil || len(*a.Kind) > 64 ||
			!kindRE.MatchString(*a.Kind) || (a.Text != nil) == (len(a.Data) > 0) || (a.Re != nil && *a.Re == "") ||
			(a.Topic != nil && *a.Topic == "") {
			return badArguments()
		}
		body := client.Body{Form: client.FormText}
		if a.Text != nil {
			body.Text = *a.Text
		} else {
			d, err := canonical.Form(a.Data)
			if err != nil {
				return badArguments()
			}
			body = client.Body{Form: client.FormData, Data: d}
		}
		c, e := open(a.Mediator)
		if e != nil {
			return e
		}
		m := client.Message{To: *a.To, Kind: *a.Kind, Body: body}
		if a.Re != nil {
			m.Re = *a.Re
		}
		if a.Topic != nil {
			m.Topic = *a.Topic
		}
		return s.give(ctx, c, m)
	case ToolGet:
		var a getArgs
		w, ok := 0, strict.Decode(raw, &a)
		if ok {
			w, ok = waitOf(a.Wait)
		}
		if !ok {
			return badArguments()
		}
		c, e := open(a.Mediator)
		if e != nil {
			return e
		}
		return s.get(ctx, c, w)
	case ToolQuestions:
		var a questionsArgs
		w, ok := 0, strict.Decode(raw, &a)
		if ok {
			w, ok = waitOf(a.Wait)
		}
		if !ok || (a.After != nil && !cursorRE.MatchString(*a.After)) {
			return badArguments()
		}
		c, e := open(a.Mediator)
		if e != nil {
			return e
		}
		after := ""
		if a.After != nil {
			after = *a.After
		}
		return s.questions(ctx, c, w, after)
	case ToolResult:
		var a resultArgs
		w, ok := 0, strict.Decode(raw, &a)
		if ok {
			w, ok = waitOf(a.Wait)
		}
		if !ok || a.Question == nil || !hashRE.MatchString(*a.Question) {
			return badArguments()
		}
		c, e := open(a.Mediator)
		if e != nil {
			return e
		}
		return s.result(ctx, c, *a.Question, w)
	default:
		var a cancelArgs
		if !strict.Decode(raw, &a) || a.Question == nil || !hashRE.MatchString(*a.Question) {
			return badArguments()
		}
		c, e := open(a.Mediator)
		if e != nil {
			return e
		}
		rep, err := c.Cancel(ctx, *a.Question)
		if err != nil {
			return failed(err)
		}
		if r := refusal(rep); r != nil {
			return r
		}
		return toolResult(map[string]any{"outcome": outcomeObject(rep.Outcome)})
	}
}

// outcomeObject is an outcome as a tool returns it.
func outcomeObject(o *mediator.Outcome) map[string]any {
	return map[string]any{"verdict": o.Verdict, "reason": o.Reason, "receiptId": o.ReceiptID, "receipt": json.RawMessage(o.Receipt)}
}

// deliveryObject is a delivery as a tool returns it, its body as the delivery carries it.
func deliveryObject(d *client.Delivery) map[string]any {
	o := map[string]any{"receiptId": d.ReceiptID, "receipt": d.Receipt, "messageId": d.MessageID, "kind": d.Kind, "salt": d.Salt}
	if d.Re != "" {
		o["re"] = d.Re
	}
	if d.MediaType != nil {
		o["mediaType"] = *d.MediaType
	}
	if len(d.Answer) > 0 {
		o["answer"] = d.Answer
	}
	switch {
	case d.Text != nil:
		o["text"] = *d.Text
	case d.Raw != nil:
		o["raw"] = *d.Raw
	default:
		o["data"] = d.Data
	}
	return o
}

// give gives the part and returns its outcome, and, when the part is asked about, the question's id
// at once: the agent follows with result and its wait.
func (s *Server) give(ctx context.Context, c *client.Client, m client.Message) map[string]any {
	rep, err := c.Give(ctx, m, false)
	if err != nil {
		return failed(err)
	}
	if r := refusal(rep); r != nil {
		return r
	}
	o := map[string]any{"outcome": outcomeObject(rep.Outcome)}
	if rep.Outcome.Verdict == string(composition.Escalate) {
		o["question"] = rep.Outcome.ReceiptID
	}
	return toolResult(o)
}

// get fetches the next delivery, waiting up to wait seconds for one. A delivery the window handed
// over whose record could not be written is no longer queued, so the tool error holds it.
func (s *Server) get(ctx context.Context, c *client.Client, wait int) map[string]any {
	g, err := c.Get(ctx, wait, false)
	if err != nil && g.Delivery != nil {
		return failedWith(err, map[string]any{"delivery": deliveryObject(g.Delivery)})
	}
	if err != nil {
		return failed(err)
	}
	if r := refusal(g.Reply); r != nil {
		return r
	}
	o := map[string]any{}
	if g.Delivery != nil {
		o["delivery"] = deliveryObject(g.Delivery)
	}
	if g.Close != nil {
		o["close"] = json.RawMessage(g.Close)
	}
	return toolResult(o)
}

// questions lists the open questions the key may answer, each as questions prints it with its ask's
// receipt, and the cursor to pass back as after. A question the window shows that is not the part
// its ask's receipt names is left out, and its id listed in misstated, so the agent learns of it and
// still gets the questions after it, and a cursor past them all.
func (s *Server) questions(ctx context.Context, c *client.Client, wait int, after string) map[string]any {
	shown, err := c.Questions(ctx, wait, after)
	if err != nil {
		return failed(err)
	}
	if r := refusal(shown.Reply); r != nil {
		return r
	}
	list := []any{}
	for _, q := range shown.Questions {
		o := map[string]any{"id": q.ID, "asker": q.Asker, "kind": q.Kind, "deadline": q.Deadline, "form": q.Form,
			"receiptId": q.ID, "receipt": json.RawMessage(q.Receipt)}
		switch {
		case q.Message != nil:
			o["message"] = *q.Message
		case q.Size != nil:
			o["size"] = *q.Size
		}
		list = append(list, o)
	}
	o := map[string]any{"questions": list, "cursor": shown.Cursor}
	if shown.More {
		o["more"] = true
	}
	if shown.Close != nil {
		o["close"] = json.RawMessage(shown.Close)
	}
	if len(shown.Misstated) > 0 {
		o["misstated"] = shown.Misstated
	}
	return toolResult(o)
}

// result is how a question the key asked closed: open while it is; once closed, its closing, its
// release when it has one, and, after an accept or a change, the answer or the version, fetched
// under the lock as result fetches it.
func (s *Server) result(ctx context.Context, c *client.Client, question string, wait int) map[string]any {
	rep, err := c.Result(ctx, question, wait)
	if err != nil {
		return failed(err)
	}
	if r := refusal(rep); r != nil {
		return r
	}
	if rep.Outcome == nil {
		return toolResult(map[string]any{"question": question, "open": true})
	}
	closing := outcomeObject(rep.Outcome)
	if st, err := receipts.Decode(rep.Outcome.Receipt); err == nil {
		closing["word"] = export.Closing(st.Predicate, st.Subject)
	}
	o := map[string]any{"question": question, "open": false, "closing": closing}
	if rep.Release != nil {
		o["release"] = outcomeObject(rep.Release)
	}
	if answered(rep.Outcome) {
		own, err := c.FetchOwn(ctx, question, rep.Outcome.ReceiptID)
		if err == nil {
			if r := refusal(own.Reply); r != nil {
				return r
			}
		}
		if own.Delivery != nil {
			o["answer"] = deliveryObject(own.Delivery)
		}
		if len(own.Others) > 0 {
			var others []any
			for _, d := range own.Others {
				others = append(others, deliveryObject(d))
			}
			o["deliveries"] = others
		}
		if err != nil {
			// What was fetched before the error, and a delivery whose record could not be written,
			// is no longer queued, so the tool error holds it.
			if own.Delivery != nil || len(own.Others) > 0 {
				return failedWith(err, o)
			}
			return failed(err)
		}
	}
	return toolResult(o)
}

// answered reports whether a closing is an accept or a change that passed, which queues the answer
// or the version for the asker.
func answered(o *mediator.Outcome) bool { return o.Verdict == string(composition.Pass) }

// mediators lists the labels config's entries, each with its name, label, Accord id and socket, and
// for each label the server was started with whether it is served or the error it is refused with.
func (s *Server) mediators(list []*served) map[string]any {
	by := map[string]*served{}
	for _, v := range list {
		by[v.m.Name()] = v
	}
	out := []any{}
	for _, m := range s.Config.Mediators {
		o := map[string]any{"name": m.Name(), "accord": m.Accord, "socket": m.Socket, "served": false}
		if m.Label != "" {
			o["label"] = m.Label
		}
		if v, ok := by[m.Name()]; ok {
			if v.accepted {
				o["served"] = true
			} else {
				o["refused"] = errtext.Text(v.from, v.code)
			}
		}
		out = append(out, o)
	}
	return toolResult(map[string]any{"mediators": out})
}
