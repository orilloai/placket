package cli

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
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

// about reports whether a delivery is what its receipt says it is, before anything of it is
// recorded, printed or written: a receipt that decodes (receipts.Decode, which verifies nothing),
// that passed, to this key under this Accord, of the id and kind the delivery names, whose digest
// is that of the delivery's salt and its one body. So a delivery's kind is a label and its receipt
// id a typed hash, whatever window sent it; reconcile and verify, under the mediator's key, are
// what show the receipt is the mediator's.
func (s *session) about(d delivery) bool {
	st, err := receipts.Decode(d.Receipt)
	if err != nil {
		return false
	}
	bodies := 0
	for _, has := range []bool{d.Text != nil, d.Raw != nil, len(d.Data) > 0} {
		if has {
			bodies++
		}
	}
	if d.Raw != nil {
		if _, err := base64.StdEncoding.Strict().DecodeString(*d.Raw); err != nil {
			return false
		}
	}
	salt, err := hex.DecodeString(d.Salt)
	p := st.Predicate
	return err == nil && bodies == 1 && st.ID == d.ReceiptID && st.Subject.Name == d.Kind &&
		p.Verdict == string(composition.Pass) && p.To == s.key.Reference() && p.Accord == s.client.Accord &&
		receipts.Digest(salt, d.bytes()) == st.Subject.Digest
}

// fetch gets the next delivery, under the state lock the caller holds, and records it: its receipt
// in delivered.jsonl, and the delivery, with its answer call beside it, in kept.jsonl when keep is
// set. It returns nil when nothing is queued; a delivery that is not what its receipt says is tool
// error reply, and nothing of it is recorded.
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
	if err := json.Unmarshal(rep.Delivery, &d); err != nil || !s.about(d) {
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

// show prints a fetched delivery as get does: one line on standard error, each field escaped, and
// its body on standard output, escaped on a terminal, or in a new file of the directory out, named
// for the receipt id's hex and the kind, which fetch has read for their form. The file is created
// only if nothing is at its path, so it is never written through a link or over another file.
func (r *run) show(d *delivery, out string) int {
	head := "delivery " + field(d.ReceiptID) + " kind " + field(d.Kind) + " messageId " + field(d.MessageID) + " salt " + field(d.Salt)
	if d.Re != "" {
		head += " re " + field(d.Re)
	}
	fmt.Fprintln(r.stderr, head)
	b := d.bytes()
	if out == "" {
		line(r.stdout, r.screen(b))
		return ExitOK
	}
	path := filepath.Join(out, strings.TrimPrefix(d.ReceiptID, canonical.HashPrefix)+"."+d.Kind)
	if filepath.Dir(path) != filepath.Clean(out) {
		return r.tool(codeFile)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return r.tool(codeFile)
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err != nil || cerr != nil {
		_ = os.Remove(path)
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
