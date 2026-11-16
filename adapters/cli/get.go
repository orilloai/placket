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
// in delivered.jsonl, its salt in salts.jsonl, and the delivery, with its answer call beside it, in kept.jsonl when keep is
// set. It returns the delivery and its kept.jsonl line, or nil when nothing is queued; a delivery
// that is not what its receipt says is tool error reply, and nothing of it is recorded.
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
	if err := s.dir.AddSalt(d.ReceiptID, d.Salt); err != nil {
		return nil, nil, s.r.failed(err)
	}
	kl := keptLine(rep.Delivery, rep.AnswerCall)
	if keep {
		if err := s.dir.Keep(kl); err != nil {
			return nil, nil, s.r.failed(err)
		}
	}
	return &d, kl, 0
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
// its body on standard output, escaped on a terminal.
func (r *run) show(d *delivery) {
	r.head(d)
	line(r.stdout, r.screen(d.bytes()))
}

// head is a fetched delivery's line on standard error.
func (r *run) head(d *delivery) {
	head := "delivery " + field(d.ReceiptID) + " kind " + field(d.Kind) + " messageId " + field(d.MessageID) + " salt " + field(d.Salt)
	if d.Re != "" {
		head += " re " + field(d.Re)
	}
	fmt.Fprintln(r.stderr, head)
}

// writable reports whether a file can be made in the directory dir, by making one and removing it.
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".placket-")
	if err != nil {
		return false
	}
	name := f.Name()
	cerr := f.Close()
	return os.Remove(name) == nil && cerr == nil
}

// writeOut writes a fetched delivery's body to a new file of the directory out, named for the
// receipt id's hex and the kind, which fetch has read for their form, and returns its path. The
// file is made only if nothing is at its path, so it is never written through a link or over
// another file.
func writeOut(d *delivery, out string) (string, bool) {
	path := filepath.Join(out, strings.TrimPrefix(d.ReceiptID, canonical.HashPrefix)+"."+d.Kind)
	if filepath.Dir(path) != filepath.Clean(out) {
		return "", false
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", false
	}
	_, err = f.Write(d.bytes())
	serr := f.Sync()
	if cerr := f.Close(); err != nil || serr != nil || cerr != nil {
		_ = os.Remove(path)
		return "", false
	}
	return path, true
}

// getCommand is `get [--all] [--out DIR] [--keep]`, with the socket flags: it fetches the next
// delivery. With --out, a directory where no file can be made is tool error file before anything is
// fetched. A body that is fetched and cannot then be written is kept in kept.jsonl, so it is not
// lost, and the error says so: error file kept.
//
// With --all it fetches every delivery waiting, one after another under the one state lock, each
// written as get writes it and each kept in kept.jsonl, as give --wait keeps what it fetches; it
// stops at the first that fails. On an empty queue it prints no delivery and exits 0. Its object
// holds every delivery it fetched, in the order fetched, under deliveries, a list that is empty on an
// empty queue; with --out each is without its body and with the path of its file, once written
// (commands-1, commands-3).
func getCommand(r *run, args []string) int {
	fs := flags("get")
	keyPath, socket, accord, stateDir := socketFlags(fs)
	out, keep := fs.String("out", "", "directory for the body"), fs.Bool("keep", false, "keep the part")
	all := fs.Bool("all", false, "fetch every delivery")
	if !r.parse(fs, args, "key", "socket", "accord") {
		return r.tool(codeUsage)
	}
	s, code := r.open(*keyPath, *socket, *accord, *stateDir)
	if s == nil {
		return code
	}
	if *out != "" && !writable(*out) {
		return r.tool(codeFile)
	}
	if err := s.dir.Lock(); err != nil {
		return r.failed(err)
	}
	defer s.dir.Unlock()
	if *all {
		r.put(memberDeliveries, []any{})
	}
	for n := 0; ; n++ {
		d, kept, code := s.fetch(*keep || *all)
		if d == nil {
			if code == 0 && n == 0 {
				fmt.Fprintln(r.stderr, LineNoDelivery)
			}
			return code
		}
		if code := s.write(d, kept, *out, *keep || *all, *all); code != ExitOK || !*all {
			return code
		}
	}
}

// write writes a fetched delivery as get does: its body on standard output, or to a file of the
// directory out with its path on standard output. A body that cannot be written is kept, unless
// keep has kept it already, and is tool error file kept. The object holds the delivery, with its
// body without out, and with out without it and the path once written; with all it is added to the
// list of every delivery fetched, the path inside it.
func (s *session) write(d *delivery, kept []byte, out string, keep, all bool) int {
	r := s.r
	o := deliveryObject(d, out == "")
	hold := func() {
		if all {
			r.add(memberDeliveries, o)
		} else {
			r.put(memberDelivery, o)
		}
	}
	if out == "" {
		r.show(d)
		hold()
		return ExitOK
	}
	r.head(d)
	path, ok := writeOut(d, out)
	if !ok {
		hold()
		if !keep {
			if err := s.dir.Keep(kept); err != nil {
				return r.failed(err)
			}
		}
		return r.tool(codeFile, detailKept)
	}
	fmt.Fprintln(r.stdout, path)
	if all {
		o[memberPath] = path
	} else {
		r.put(memberPath, path)
	}
	hold()
	return ExitOK
}
