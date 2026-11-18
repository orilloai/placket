package cli

import (
	"flag"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
)

func init() {
	commands["give"] = giveCommand
}

// readBody reads the body of a give: --text, or --file read as text, raw or data.
func (r *run) readBody(text, file, as string, textSet bool) (client.Body, int) {
	if textSet == (file != "") {
		return client.Body{}, r.tool(codeBody)
	}
	if textSet {
		// --as names how a file is read; text given on the command line is only text.
		if as != "text" {
			return client.Body{}, r.tool(codeUsage)
		}
		// Text that is not UTF-8 is refused, never repaired (parse-strict).
		if !utf8.ValidString(text) {
			return client.Body{}, r.tool(codeText)
		}
		return client.Body{Form: client.FormText, Text: text}, 0
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return client.Body{}, r.tool(codeFile)
	}
	switch as {
	case "text":
		if !utf8.Valid(b) {
			return client.Body{}, r.tool(codeText)
		}
		return client.Body{Form: client.FormText, Text: string(b)}, 0
	case "raw":
		return client.Body{Form: client.FormRaw, Raw: b}, 0
	case "data":
		c, err := canonical.Form(b)
		if err != nil {
			return client.Body{}, r.tool(codeData)
		}
		return client.Body{Form: client.FormData, Data: c}, 0
	}
	return client.Body{}, r.tool(codeUsage)
}

// allUTF8 reports whether every string is UTF-8.
func allUTF8(list ...string) bool {
	for _, s := range list {
		if !utf8.ValidString(s) {
			return false
		}
	}
	return true
}

// send gives one part as the session's key, under the state directory's lock (state-lock), which
// client.Give takes, or holds once more when the caller holds it. It prints the outcome, keeps the
// part when asked, and returns the outcome, or nil and the exit code.
func (s *session) send(m client.Message, keep bool) (*mediator.Outcome, int) {
	rep, err := s.c.Give(s.r.env.Context, m, keep)
	if err != nil {
		return nil, s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		return nil, code
	}
	s.r.outcome(rep.Outcome, false, memberOutcome)
	return rep.Outcome, 0
}

// giveCommand is `give --to A --kind K (--text S | --file B --as text|raw|data) [--re R]
// [--topic T] [--keep] [--wait]`, with the socket flags. With --wait it waits on the door, holding
// no lock, until the question closes.
func giveCommand(r *run, args []string) int {
	fs := flags("give")
	so := socketFlags(fs)
	to, kind := fs.String("to", "", "receiver"), fs.String("kind", "", "kind")
	text, file, as := fs.String("text", "", "text"), fs.String("file", "", "file"), fs.String("as", "text", "text, raw or data")
	re, topic := fs.String("re", "", "re"), fs.String("topic", "", "topic")
	keep, wait := fs.Bool("keep", false, "keep the part"), fs.Bool("wait", false, "wait for the answer")
	// A receiver, kind, re or topic that is not UTF-8 is refused, never repaired (parse-strict).
	if !r.parseSocket(fs, args, so, "to", "kind") || !allUTF8(*to, *kind, *re, *topic) {
		return r.tool(codeUsage)
	}
	textSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "text" {
			textSet = true
		}
	})
	b, code := r.readBody(*text, *file, *as, textSet)
	if code != 0 {
		return code
	}
	s, code := r.open(so)
	if s == nil {
		return code
	}
	// The lock is let go before a wait: the wait holds none (state-lock).
	out, code := s.send(client.Message{To: *to, Kind: *kind, Body: b, Re: *re, Topic: *topic}, *keep)
	if out == nil {
		return code
	}
	switch out.Verdict {
	case string(composition.Pass):
		return ExitOK
	case string(composition.Escalate):
		r.put(memberQuestion, out.ReceiptID)
		if *wait {
			return s.closing(out.ReceiptID, 0, true)
		}
		fmt.Fprintln(r.stdout, out.ReceiptID)
		return ExitOpen
	}
	return ExitBlock
}
