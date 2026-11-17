package cli

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

func init() {
	commands["give"] = giveCommand
}

// A body is a part's body as give sends it: its form and its value, a string for text, standard
// base64 for raw, or canonical JSON for data.
type body struct {
	form  string
	value any
}

// readBody reads the body of a give: --text, or --file read as text, raw or data.
func (r *run) readBody(text, file, as string, textSet bool) (body, int) {
	if textSet == (file != "") {
		return body{}, r.tool(codeBody)
	}
	if textSet {
		// --as names how a file is read; text given on the command line is only text.
		if as != "text" {
			return body{}, r.tool(codeUsage)
		}
		// Text that is not UTF-8 is refused, never repaired (parse-strict).
		if !utf8.ValidString(text) {
			return body{}, r.tool(codeText)
		}
		return body{form: "text", value: text}, 0
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return body{}, r.tool(codeFile)
	}
	switch as {
	case "text":
		if !utf8.Valid(b) {
			return body{}, r.tool(codeText)
		}
		return body{form: "text", value: string(b)}, 0
	case "raw":
		return body{form: "raw", value: base64.StdEncoding.EncodeToString(b)}, 0
	case "data":
		c, err := canonical.Form(b)
		if err != nil {
			return body{}, r.tool(codeData)
		}
		return body{form: "data", value: json.RawMessage(c)}, 0
	}
	return body{}, r.tool(codeUsage)
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

// uuid is a fresh lowercase UUID version 4.
func uuid(rand io.Reader) (string, bool) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand, b); err != nil {
		return "", false
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], true
}

// message builds a crossing message of one part (crossing-1, crossing-2).
func (s *session) message(to, kind string, b body, re, topic string) ([]byte, bool) {
	id, ok := uuid(s.r.env.Rand)
	if !ok {
		return nil, false
	}
	salt := make([]byte, mediator.MinSaltBytes)
	if _, err := io.ReadFull(s.r.env.Rand, salt); err != nil {
		return nil, false
	}
	ext := map[string]any{"accord": s.client.Accord, "to": to}
	if re != "" {
		ext["re"] = re
	}
	if topic != "" {
		ext["topic"] = topic
	}
	part := map[string]any{b.form: b.value, "metadata": map[string]any{mediator.Extension: map[string]any{"kind": kind, "salt": hex.EncodeToString(salt)}}}
	m := canonicalJSON(map[string]any{
		"messageId": id, "role": mediator.RoleAgent, "extensions": []string{mediator.Extension},
		"metadata": map[string]any{mediator.Extension: ext}, "parts": []any{part},
	})
	return m, m != nil
}

// send gives one part as the session's key, under the state directory's lock, which the caller
// holds (state-lock): at the next seq, above the seq file and every envelope recorded, so a give
// whose reply was lost never has its seq used again; its envelope recorded before the call is sent
// and removed again only on a refusal. It prints the outcome, keeps the part when asked, and returns
// the outcome, or nil and the exit code.
func (s *session) send(to, kind string, b body, re, topic string, keep bool) (*mediator.Outcome, int) {
	seq, err := s.dir.Next()
	if err != nil {
		return nil, s.r.failed(err)
	}
	msg, ok := s.message(to, kind, b, re, topic)
	if !ok {
		return nil, s.r.tool(codeRand)
	}
	hash, _ := canonical.TypedHash(msg)
	env := canonicalJSON(map[string]any{"seq": seq, "hash": hash})
	sig, err := signing.Sign(env, signing.TypeEnvelope, s.key)
	if err != nil {
		return nil, s.r.tool(codeEnvelope)
	}
	env = canonicalJSON(map[string]any{"seq": seq, "hash": hash, "sig": sig})
	if err := s.dir.AddEnvelope(seq, hash); err != nil {
		return nil, s.r.failed(err)
	}
	rep, err := s.client.Call(local.OpGive, local.GiveArgs(msg, env))
	if err != nil {
		// No reply: the envelope stays, so a submission that crossed unseen shows as missing.
		return nil, s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		if err := s.dir.DropEnvelope(seq, hash); err != nil {
			return nil, s.r.failed(err)
		}
		return nil, code
	}
	if rep.Outcome == nil {
		return nil, s.r.tool(codeReply)
	}
	if err := s.dir.SetSeq(seq); err != nil {
		return nil, s.r.failed(err)
	}
	if keep {
		k := canonicalJSON(map[string]any{"message": base64.StdEncoding.EncodeToString(msg), "receipt": json.RawMessage(rep.Outcome.Receipt)})
		if err := s.dir.Keep(k); err != nil {
			return nil, s.r.failed(err)
		}
	}
	s.r.outcome(rep.Outcome, false, memberOutcome)
	return rep.Outcome, 0
}

// giveCommand is `give --to A --kind K (--text S | --file B --as text|raw|data) [--re R]
// [--topic T] [--keep] [--wait]`, with the socket flags.
func giveCommand(r *run, args []string) int {
	fs := flags("give")
	keyPath, socket, accord, stateDir := socketFlags(fs)
	to, kind := fs.String("to", "", "receiver"), fs.String("kind", "", "kind")
	text, file, as := fs.String("text", "", "text"), fs.String("file", "", "file"), fs.String("as", "text", "text, raw or data")
	re, topic := fs.String("re", "", "re"), fs.String("topic", "", "topic")
	keep, wait := fs.Bool("keep", false, "keep the part"), fs.Bool("wait", false, "wait for the answer")
	// A receiver, kind, re or topic that is not UTF-8 is refused, never repaired (parse-strict).
	if !r.parse(fs, args, "key", "socket", "accord", "to", "kind") || !allUTF8(*to, *kind, *re, *topic) {
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
	s, code := r.open(*keyPath, *socket, *accord, *stateDir)
	if s == nil {
		return code
	}
	if err := s.dir.Lock(); err != nil {
		return r.failed(err)
	}
	out, code := s.send(*to, *kind, b, *re, *topic, *keep)
	s.dir.Unlock()
	if out == nil {
		return code
	}
	switch out.Verdict {
	case string(composition.Pass):
		return ExitOK
	case string(composition.Escalate):
		r.put(memberQuestion, out.ReceiptID)
		if *wait {
			return s.closing(out.ReceiptID, true)
		}
		fmt.Fprintln(r.stdout, out.ReceiptID)
		return ExitOpen
	}
	return ExitBlock
}
