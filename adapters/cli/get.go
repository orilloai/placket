package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["get"] = getCommand
}

// A delivery as a command reads it.
type delivery = client.Delivery

// keptDelivery is a line of kept.jsonl for a fetched part, the delivery as it was fetched. An answer
// line of kept.jsonl, {"answer","salt"}, reads as one with no receipt id, which no reader takes for a
// part.
type keptDelivery struct {
	delivery
}

// fetch gets the next delivery, recorded by the client before it returns (client.Get): its receipt
// in delivered.jsonl, its salt in salts.jsonl, and the delivery, the answer call inside it, in
// kept.jsonl when keep is set. It returns the delivery and its kept.jsonl line, or nil when nothing
// is queued, leaving in s.handed the close receipt the reply carried in its place, if any, for get
// to keep; a delivery that is not what its receipt says is tool error reply, and nothing of it is
// recorded. With wait it waits on the door, without the lock, for a delivery queued.
func (s *session) fetch(wait int, keep bool) (*delivery, []byte, int) {
	g, err := s.c.Get(s.r.env.Context, wait, keep)
	if err != nil {
		return nil, nil, s.r.failed(err)
	}
	if code, refused := s.r.refusal(g.Reply); refused {
		return nil, nil, code
	}
	if g.Delivery == nil {
		s.handed = g.Close
		return nil, nil, 0
	}
	return g.Delivery, g.Kept, 0
}

// keepClose keeps the close receipt a fetch was handed on a closed Accord, whose queue is empty
// (close-3): in close.json, once, and in the object as close; it prints `close <id>`. ParseReply has
// read it as a close, and Send under this Accord. A close.json that holds another is the state
// directory's error.
func (s *session) keepClose(receipt []byte) int {
	c, err := receipts.DecodeClose(receipt)
	if err != nil {
		return s.r.tool(string(client.CodeReply))
	}
	if err := s.c.Dir().KeepClose(receipt); err != nil {
		return s.r.failed(err)
	}
	fmt.Fprintln(s.r.stderr, memberClose+" "+c.ID)
	s.r.put(memberClose, map[string]any{memberReceiptID: c.ID, memberReceipt: json.RawMessage(receipt)})
	return 0
}

// show prints a fetched delivery as get does: one line on standard error, each field escaped, and
// its body on standard output, escaped on a terminal.
func (r *run) show(d *delivery) {
	r.head(d)
	line(r.stdout, r.screen(d.Bytes()))
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
	_, err = f.Write(d.Bytes())
	serr := f.Sync()
	if cerr := f.Close(); err != nil || serr != nil || cerr != nil {
		_ = os.Remove(path)
		return "", false
	}
	return path, true
}

// getCommand is `get [--all] [--out DIR] [--keep] [--wait N]`, with the socket flags: it fetches
// the next delivery. With --wait it first waits, at most N seconds and holding no lock, until a
// delivery is queued for the key; with none by then it prints no delivery and exits 0. With --out, a directory where no file can be made is tool error file before anything is
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
	so := socketFlags(fs)
	out, keep := fs.String("out", "", "directory for the body"), fs.Bool("keep", false, "keep the part")
	all := fs.Bool("all", false, "fetch every delivery")
	wait := fs.Int("wait", 0, "seconds to wait for a delivery, at most 60")
	if !r.parseSocket(fs, args, so) || *wait < 0 || *wait > local.MaxWait {
		return r.tool(codeUsage)
	}
	s, code := r.open(so)
	if s == nil {
		return code
	}
	if *out != "" && !writable(*out) {
		return r.tool(codeFile)
	}
	if *all {
		r.put(memberDeliveries, []any{})
	}
	// Every fetch is under the state lock, held across them under --all; a wait holds none, so with
	// --wait the lock is taken once the first fetch is done (state-lock).
	locked := false
	lock := func() bool {
		if err := s.c.Lock(); err != nil {
			r.failed(err)
			return false
		}
		locked = true
		return true
	}
	defer func() {
		if locked {
			s.c.Unlock()
		}
	}()
	if *wait == 0 && !lock() {
		return ExitTool
	}
	for n := 0; ; n++ {
		w := 0
		if n == 0 {
			w = *wait
		}
		d, kept, code := s.fetch(w, *keep || *all)
		if code == 0 && !locked && !lock() {
			return ExitTool
		}
		if d == nil {
			if code == 0 && s.handed != nil {
				return s.keepClose(s.handed)
			}
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
			if err := s.c.Dir().Keep(kept); err != nil {
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
