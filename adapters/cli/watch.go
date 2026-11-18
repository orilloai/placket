package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
)

func init() {
	commands["watch"] = watchCommand
}

// A sighting is a question one watched mediator showed, or the failure that ended its loop.
type sighting struct {
	label string
	q     local.Asked
	rep   local.Reply
	err   error
}

// watchCommand is `watch --label L... [--count N] [--json]`: it waits on the questions of each
// mediator named, at once, and prints each new question once, as it opens: a block of its id, the
// label, its kind, asker and deadline, then the placket answer, change and decline lines for it, each
// naming the label as given, ready to run once its text is filled in. It never answers, signs
// nothing and writes nothing to a state directory. Each label's loop calls questions with the longest
// wait, after the cursor of its last reply, so it waits instead of asking again, and holds no lock
// (state-lock). A question already printed is not printed again; it keeps no state between runs, so
// after a restart it prints the questions still open again. With --count it exits 0 once it has
// printed N; it runs until interrupted otherwise, and exits 0 then. The first failure of any label's
// loop, a refusal or an error, ends it with that refusal's or error's exit.
//
// With --json each question is one object on a line of its own, as it opens: label, id, kind, asker,
// deadline, form, the part as questions shows it, and commands, the three lines; the command's
// object follows when it exits.
func watchCommand(r *run, args []string) int {
	fs := flags("watch")
	var labels many
	fs.Var(&labels, "label", "a mediator of the labels config to watch, once per mediator")
	count := fs.Int("count", 0, "exit once this many questions are printed")
	if !r.parse(fs, args) || len(labels) == 0 || *count < 0 {
		return r.tool(codeUsage)
	}
	cfg, err := r.config()
	if err != nil {
		return r.failed(err)
	}
	var clients []*client.Client
	for _, l := range labels {
		m, err := cfg.Find(l)
		if err != nil {
			return r.failed(err)
		}
		c, err := client.Open(m, client.Options{Now: r.env.Now, Rand: r.env.Rand, Stderr: r.stderr})
		if err != nil {
			return r.failed(err)
		}
		clients = append(clients, c)
	}
	ctx, stop := context.WithCancel(r.env.Context)
	var wg sync.WaitGroup
	// Every loop is this command's: it stops them and waits for each before it returns
	// (goroutines-owned).
	defer func() {
		stop()
		wg.Wait()
	}()
	seen := make(chan sighting)
	for i, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			watchOne(ctx, labels[i], c, seen)
		}()
	}
	printed := 0
	for {
		select {
		case <-r.env.Context.Done():
			return ExitOK
		case s := <-seen:
			if s.err != nil {
				return r.failed(s.err)
			}
			if code, refused := r.refusal(s.rep); refused {
				return code
			}
			r.printQuestion(s.label, s.q)
			if printed++; *count > 0 && printed == *count {
				return ExitOK
			}
		}
	}
}

// watchOne is one label's loop: questions with the longest wait, after the last reply's cursor, each
// new question sent on seen once. It ends when ctx does, on a closed Accord, which opens no question,
// and after sending a failure.
func watchOne(ctx context.Context, label string, c *client.Client, seen chan<- sighting) {
	send := func(s sighting) bool {
		select {
		case seen <- s:
			return true
		case <-ctx.Done():
			return false
		}
	}
	printed := map[string]bool{}
	cursor := ""
	for {
		shown, err := c.Questions(ctx, local.MaxWait, cursor)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			send(sighting{label: label, err: err})
			return
		}
		if _, _, refused := client.Refused(shown.Reply); refused {
			send(sighting{label: label, rep: shown.Reply})
			return
		}
		for _, q := range shown.Questions {
			if printed[q.ID] {
				continue
			}
			printed[q.ID] = true
			if !send(sighting{label: label, q: q}) {
				return
			}
		}
		if shown.Close != nil {
			return
		}
		cursor = shown.Cursor
	}
}

// answerLines are the commands that answer a question of the mediator named label.
func answerLines(label, id string) []string {
	at := "--label " + label + " --question " + id
	return []string{"placket answer " + at, "placket change " + at + " --text TEXT", "placket decline " + at + " --comment TEXT"}
}

// printQuestion prints one question watch shows: as a block on standard output, each field another
// party wrote escaped, or with --json as one object on a line of its own.
func (r *run) printQuestion(label string, q local.Asked) {
	lines := answerLines(label, q.ID)
	if r.asJSON {
		o := map[string]any{"label": label, "id": q.ID, "kind": q.Kind, "asker": q.Asker, "deadline": q.Deadline, "form": q.Form,
			"commands": map[string]any{"answer": lines[0], "change": lines[1], "decline": lines[2]}}
		switch {
		case q.Message != nil:
			o["message"] = *q.Message
		case q.Size != nil:
			o["size"] = *q.Size
		}
		line(r.realOut, r.screenJSON(canonicalJSON(o)))
		return
	}
	fmt.Fprintln(r.stdout, "question "+q.ID+"\n"+"label "+label+"\n"+"kind "+field(q.Kind)+"\n"+"asker "+field(q.Asker)+"\n"+
		"deadline "+field(q.Deadline)+"\n"+strings.Join(lines, "\n")+"\n")
}
