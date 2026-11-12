package cli

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
)

func init() {
	commands["get"] = getCommand
}

// A delivery as a command reads it.
type delivery struct {
	ReceiptID string          `json:"receiptId"`
	Receipt   json.RawMessage `json:"receipt"`
	MessageID string          `json:"messageId"`
	Kind      string          `json:"kind"`
	Salt      string          `json:"salt"`
	Re        string          `json:"re"`
	Text      *string         `json:"text"`
	Raw       *string         `json:"raw"`
	Data      json.RawMessage `json:"data"`
}

// bytes is the delivery's body as get writes it: a text's UTF-8, a raw body's decoded bytes, a data
// body's canonical bytes.
func (d delivery) bytes() []byte {
	switch {
	case d.Text != nil:
		return []byte(*d.Text)
	case d.Raw != nil:
		b, _ := base64.StdEncoding.DecodeString(*d.Raw)
		return b
	}
	c, _ := canonical.Form(d.Data)
	return c
}

// fetch gets the next delivery, under the state lock the caller holds, and records it: its receipt
// in delivered.jsonl, and the delivery, with its answer call beside it, in kept.jsonl when keep is
// set. It returns nil when nothing is queued.
func (s *session) fetch(keep bool) (*delivery, []byte, int) {
	rep, err := s.client.Call(local.OpGet, nil)
	if err != nil {
		return nil, nil, s.r.failed(err)
	}
	if code, refused := s.r.refusal(rep); refused {
		return nil, nil, code
	}
	if rep.Delivery == nil {
		return nil, nil, 0
	}
	var d delivery
	if err := json.Unmarshal(rep.Delivery, &d); err != nil || len(d.Receipt) == 0 {
		return nil, nil, s.r.tool("reply")
	}
	if err := s.dir.AddDelivered(d.Receipt); err != nil {
		return nil, nil, s.r.failed(err)
	}
	if keep {
		if err := s.dir.Keep(keptLine(rep.Delivery, rep.AnswerCall)); err != nil {
			return nil, nil, s.r.failed(err)
		}
	}
	return &d, rep.Delivery, 0
}

// keptLine is a fetched delivery's line of kept.jsonl: the delivery, with its answer call beside it.
func keptLine(delivery, answerCall []byte) []byte {
	var o map[string]json.RawMessage
	if json.Unmarshal(delivery, &o) != nil {
		return nil
	}
	if answerCall != nil {
		o["answerCall"] = answerCall
	}
	return canonicalJSON(o)
}

// show prints a fetched delivery as get does: one line on standard error, and its body on standard
// output, or in a file of the directory out.
func (r *run) show(d *delivery, out string) int {
	head := "delivery " + d.ReceiptID + " kind " + d.Kind + " messageId " + d.MessageID + " salt " + d.Salt
	if d.Re != "" {
		head += " re " + d.Re
	}
	fmt.Fprintln(r.stderr, head)
	b := d.bytes()
	if out == "" {
		line(r.stdout, b)
		return ExitOK
	}
	path := filepath.Join(out, strings.TrimPrefix(d.ReceiptID, canonical.HashPrefix)+"."+d.Kind)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return r.tool(codeFile)
	}
	fmt.Fprintln(r.stdout, path)
	return ExitOK
}

// getCommand is `get [--out DIR] [--keep]`, with the socket flags: it fetches the next delivery.
func getCommand(r *run, args []string) int {
	fs := flags("get")
	keyPath, socket, accord, stateDir := socketFlags(fs)
	out, keep := fs.String("out", "", "directory for the body"), fs.Bool("keep", false, "keep the part")
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
	d, _, code := s.fetch(*keep)
	if d == nil {
		if code == 0 {
			fmt.Fprintln(r.stderr, LineNoDelivery)
		}
		return code
	}
	return r.show(d, *out)
}
