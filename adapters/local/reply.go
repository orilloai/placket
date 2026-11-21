package local

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// Code says why a client got no reply it could read.
type Code string

// The codes of Error.
const (
	// CodeSocket: the window could not be reached, or went away before it replied.
	CodeSocket Code = "socket"
	// CodeReply: what the window sent is not a reply.
	CodeReply Code = "reply"
	// CodeSocketPath: the socket path is longer than MaxSocketPath, so it can be neither bound nor
	// connected to.
	CodeSocketPath Code = "socket-path"
	// CodeCursor: the window could not draw the key of its cursors; it started nothing.
	CodeCursor Code = "cursor"
	// CodeJudge: the window's judge configs are not the ones its card lists, at their hashes and this
	// adapter's version; it started nothing (docs/adr/0023-judges.md).
	CodeJudge Code = "judge"
)

// MaxSocketPath is the longest socket path, in bytes, the system takes: its sun_path less the
// closing zero byte, 107 on Linux and 103 on macOS.
const MaxSocketPath = len(syscall.RawSockaddrUnix{}.Path) - 1

// CheckSocket is CodeSocketPath for a socket path longer than MaxSocketPath, and nil otherwise.
func CheckSocket(path string) error {
	if len(path) > MaxSocketPath {
		return &Error{Code: CodeSocketPath}
	}
	return nil
}

// Error is a call that got no reply: its code. It holds nothing of the call or the reply.
type Error struct{ Code Code }

func (e *Error) Error() string { return "local: " + string(e.Code) }

// An Asked is one open question as the questions reply shows it to an answerer (maybe-1): its id,
// asker, kind, deadline and the form of its body; the ask's receipt, in Receipt; the part's salt; and
// its body, as a text's text or a data body's canonical JSON in Message, or a raw body's bytes in
// Raw, with Size the body's length in bytes. So the answerer checks, before it signs, that the body
// under the salt is the part the receipt names.
type Asked struct {
	ID, Asker, Kind, Deadline, Form string
	Message                         *string
	Size                            *int
	Receipt                         []byte
	Salt                            []byte
	Raw                             []byte
}

// Format prints the Asked as a fixed text, whatever the verb: it holds a question's part
// (secrets-never-print).
func (Asked) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("local.Asked")) }

// A Reply is a reply as a client read it. Exactly one of its kinds is set: outcomes (Outcome, with
// Release where the reply has one), a delivery (Delivery, the answer call inside it when an answer
// queued it), the Accord's close receipt (Close, to a get on a closed Accord whose caller has nothing
// queued, close-3, and beside the empty questions of a closed Accord), the questions (Questions, with IsQuestions, and More when the window left some out to
// keep the reply under MaxReply), a refusal (Rejection, Error or Door), or none of them, a plain ok.
type Reply struct {
	Outcome, Release *mediator.Outcome
	Delivery         []byte
	Questions        []Asked
	IsQuestions      bool
	More             bool
	Rejection        mediator.RejectionCode
	Error            mediator.ErrorCode
	Door             DoorCode
	// Close is the close receipt a get on a closed Accord returns once the caller's queue is empty
	// (close-3, docs/adr/0022-the-close.md).
	Close []byte
	// Cursor is the opaque value every questions reply carries: passed back as a questions call's
	// after, it asks only for the questions opened after the last one this reply carries.
	Cursor string
	// Queued is the reply to a get that waited: a delivery is queued for the caller, and an ordinary
	// get takes it. A get that waits never takes one itself.
	Queued bool
	// View is the reply to a view call: the caller's own view of the Accord, its canonical bytes
	// (setup-contract-14).
	View []byte
	// IsAccept is the reply to an accept the window took, and Founded whether the Accord is now
	// founded, every member having accepted (setup-contract-15).
	IsAccept, Founded bool
	// Founding is the founding code an accept was refused with: the row of founding's table its
	// acceptance fails (setup-contract-15).
	Founding string
}

// Format prints the Reply as a fixed text, whatever the verb: it may hold a body or a salt
// (secrets-never-print).
func (Reply) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("local.Reply")) }

type wireOutcome struct {
	Reason    *string         `json:"reason"`
	Receipt   json.RawMessage `json:"receipt"`
	ReceiptID *string         `json:"receiptId"`
	Verdict   *string         `json:"verdict"`
}

type wireAsked struct {
	Asker    *string         `json:"asker"`
	Data     json.RawMessage `json:"data,omitempty"`
	Deadline *string         `json:"deadline"`
	Form     *string         `json:"form"`
	ID       *string         `json:"id"`
	Kind     *string         `json:"kind"`
	Raw      *string         `json:"raw,omitempty"`
	Receipt  json.RawMessage `json:"receipt"`
	Salt     *string         `json:"salt"`
	Size     *int            `json:"size"`
	Text     *string         `json:"text,omitempty"`
}

type wireReply struct {
	OK        *bool           `json:"ok"`
	Outcome   *wireOutcome    `json:"outcome,omitempty"`
	Release   *wireOutcome    `json:"release,omitempty"`
	Delivery  json.RawMessage `json:"delivery,omitempty"`
	Close     json.RawMessage `json:"close,omitempty"`
	Questions *[]wireAsked    `json:"questions,omitempty"`
	More      *bool           `json:"more,omitempty"`
	Cursor    *string         `json:"cursor,omitempty"`
	Queued    *bool           `json:"queued,omitempty"`
	View      json.RawMessage `json:"view,omitempty"`
	Founded   *bool           `json:"founded,omitempty"`
	Rejection *string         `json:"rejection,omitempty"`
	Error     *string         `json:"error,omitempty"`
	Door      *string         `json:"door,omitempty"`
	Founding  *string         `json:"founding,omitempty"`
}

func wire(o mediator.Outcome) *wireOutcome {
	return &wireOutcome{Reason: &o.Reason, Receipt: o.Receipt, ReceiptID: &o.ReceiptID, Verdict: &o.Verdict}
}

// internalReply is the reply when a reply cannot be encoded: a failure of the door itself.
const internalReply = `{"door":"` + string(Internal) + `","ok":false}`

// encodeReply writes a reply's canonical bytes.
func encodeReply(w wireReply) []byte {
	t := true
	if w.OK == nil {
		w.OK = &t
	}
	b, err := json.Marshal(w)
	if err != nil {
		return []byte(internalReply)
	}
	c, err := canonical.Form(b)
	if err != nil {
		return []byte(internalReply)
	}
	return c
}

// refusals: the reply's member that names the code.
const (
	byRejection = iota
	byError
	byDoor
	byFounding
)

func refusal(by int, code string) []byte {
	f := false
	w := wireReply{OK: &f}
	switch by {
	case byRejection:
		w.Rejection = &code
	case byError:
		w.Error = &code
	case byFounding:
		w.Founding = &code
	default:
		w.Door = &code
	}
	return encodeReply(w)
}

func outcome(w *wireOutcome) (*mediator.Outcome, bool) {
	if w == nil {
		return nil, true
	}
	if w.Reason == nil || w.ReceiptID == nil || w.Verdict == nil || len(w.Receipt) == 0 {
		return nil, false
	}
	r, err := canonical.Form(w.Receipt)
	if err != nil {
		return nil, false
	}
	st, err := receipts.Decode(r)
	if err != nil || st.ID != *w.ReceiptID || st.Predicate.Verdict != *w.Verdict || st.Predicate.Reason != *w.Reason {
		return nil, false
	}
	return &mediator.Outcome{Verdict: *w.Verdict, Reason: *w.Reason, ReceiptID: *w.ReceiptID, Receipt: r}, true
}

// label reports whether s is a label: lowercase letters and digits in words joined by single
// hyphens, at most 64 characters. Every code a refusal names is one.
func label(s string) bool {
	if s == "" || len(s) > 64 || s[0] == '-' || s[len(s)-1] == '-' || strings.Contains(s, "--") {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// cursorForm reports whether s has a cursor's form, as a window mints one: 1 to MaxCursor lowercase
// letters, digits, hyphens and dots. A reply's cursor is passed back as it came, and may be printed.
func cursorForm(s string) bool {
	if s == "" || len(s) > MaxCursor {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// MaxCursor is the most bytes of a cursor a client reads.
const MaxCursor = 128

// typedHash reports whether s has a typed hash's form: the prefix and 64 lowercase hex digits.
func typedHash(s string) bool {
	h, ok := strings.CutPrefix(s, canonical.HashPrefix)
	return ok && len(h) == 64 && lowerHex(h)
}

// asked reads an open question as a reply names it, in the form a window gives it: an id that is a
// typed hash, an asker that is an actor reference, a kind that is a label, a form of text, raw or
// data with exactly that body, its size, the ask's receipt as a JSON object, and the salt in
// lowercase hex.
func asked(q wireAsked) (Asked, bool) {
	if q.ID == nil || q.Asker == nil || q.Kind == nil || q.Deadline == nil || q.Form == nil || q.Salt == nil || q.Size == nil ||
		len(q.Receipt) == 0 || q.Receipt[0] != '{' {
		return Asked{}, false
	}
	if _, ref := signing.ParseReference(*q.Asker); !ref || !typedHash(*q.ID) || !label(*q.Kind) || !lowerHex(*q.Salt) {
		return Asked{}, false
	}
	a := Asked{ID: *q.ID, Asker: *q.Asker, Kind: *q.Kind, Deadline: *q.Deadline, Form: *q.Form, Size: q.Size}
	var err error
	if a.Receipt, err = canonical.Form(q.Receipt); err != nil {
		return Asked{}, false
	}
	a.Salt, _ = hex.DecodeString(*q.Salt)
	bodies := 0
	for _, has := range []bool{q.Text != nil, q.Raw != nil, len(q.Data) > 0} {
		if has {
			bodies++
		}
	}
	switch {
	case bodies != 1:
		return Asked{}, false
	case *q.Form == "text" && q.Text != nil:
		t := *q.Text
		a.Message = &t
	case *q.Form == "raw" && q.Raw != nil:
		if a.Raw, err = b64.DecodeString(*q.Raw); err != nil {
			return Asked{}, false
		}
	case *q.Form == "data" && len(q.Data) > 0:
		d, err := canonical.Form(q.Data)
		if err != nil {
			return Asked{}, false
		}
		m := string(d)
		a.Message = &m
	default:
		return Asked{}, false
	}
	return a, true
}

// ParseReply reads a reply's bytes, its line feed removed. It reads what the reply names for its
// form, since a client cannot tell what window it reached and what a reply names reaches a person's
// terminal: a refusal's code is a label, and an outcome's receipt id, verdict and reason are those
// of its receipt, which must decode (receipts.Decode, which verifies nothing); a close receipt must
// decode as one (receipts.DecodeClose, which verifies nothing either); an open question's
// id is a typed hash, its asker an actor reference, its kind a label, its form text, raw or data with
// that body, and its receipt and salt of their forms; a cursor, of the form cursorForm reads, comes
// only in a questions reply; and a queued reply is {"ok":true,"queued":true} alone.
func ParseReply(b []byte) (Reply, bool) {
	if _, err := canonical.Form(b); err != nil {
		return Reply{}, false
	}
	var w wireReply
	if !strict.Decode(b, &w) || w.OK == nil {
		return Reply{}, false
	}
	var r Reply
	set := 0
	if w.Rejection != nil {
		r.Rejection, set = mediator.RejectionCode(*w.Rejection), set+1
	}
	if w.Error != nil {
		r.Error, set = mediator.ErrorCode(*w.Error), set+1
	}
	if w.Door != nil {
		r.Door, set = DoorCode(*w.Door), set+1
	}
	if w.Founding != nil {
		r.Founding, set = *w.Founding, set+1
	}
	if *w.OK != (set == 0) || set > 1 || (set == 1 && !label(string(r.Rejection)+string(r.Error)+string(r.Door)+r.Founding)) {
		return Reply{}, false
	}
	// A view and an accept's reply each stand alone in their reply (setup-contract-14,
	// setup-contract-15).
	others := w.Outcome != nil || w.Release != nil || len(w.Delivery) > 0 || len(w.Close) > 0 || w.Questions != nil ||
		w.More != nil || w.Cursor != nil || w.Queued != nil
	if len(w.View) > 0 {
		v, err := canonical.Form(w.View)
		if err != nil || w.View[0] != '{' || set != 0 || others || w.Founded != nil {
			return Reply{}, false
		}
		r.View = v
	}
	if w.Founded != nil {
		if set != 0 || others {
			return Reply{}, false
		}
		r.IsAccept, r.Founded = true, *w.Founded
	}
	var ok bool
	if r.Outcome, ok = outcome(w.Outcome); !ok {
		return Reply{}, false
	}
	if r.Release, ok = outcome(w.Release); !ok || (r.Release != nil && r.Outcome == nil) {
		return Reply{}, false
	}
	if len(w.Delivery) > 0 {
		d, err := canonical.Form(w.Delivery)
		if err != nil || w.Delivery[0] != '{' || r.Outcome != nil {
			return Reply{}, false
		}
		r.Delivery = d
	}
	if len(w.Close) > 0 {
		c, err := canonical.Form(w.Close)
		if err != nil || w.Close[0] != '{' || r.Outcome != nil || r.Delivery != nil || (w.Questions != nil && len(*w.Questions) > 0) {
			return Reply{}, false
		}
		if _, err := receipts.DecodeClose(c); err != nil {
			return Reply{}, false
		}
		r.Close = c
	}
	if w.More != nil && (!*w.More || w.Questions == nil) {
		return Reply{}, false
	}
	if w.Cursor != nil && (w.Questions == nil || !cursorForm(*w.Cursor)) {
		return Reply{}, false
	}
	if w.Queued != nil {
		if !*w.Queued || set != 0 || r.Outcome != nil || len(w.Delivery) > 0 || len(w.Close) > 0 || w.Questions != nil {
			return Reply{}, false
		}
		r.Queued = true
	}
	if w.Questions != nil {
		if r.Outcome != nil || r.Delivery != nil {
			return Reply{}, false
		}
		r.IsQuestions, r.More = true, w.More != nil
		if w.Cursor != nil {
			r.Cursor = *w.Cursor
		}
		r.Questions = []Asked{}
		for _, q := range *w.Questions {
			a, ok := asked(q)
			if !ok {
				return Reply{}, false
			}
			r.Questions = append(r.Questions, a)
		}
	}
	return r, true
}

// A Client calls a window as one key, for one Accord.
type Client struct {
	// Socket is the window's socket path. It must be a socket itself, not a link to one, owned by
	// the user the client runs as: a window makes its socket mode 0600, so only its own user can
	// call it, and a socket another user made is not a window's.
	Socket string
	// Key signs every call.
	Key signing.Key
	// Accord is the Accord id the window printed.
	Accord string
	// Now is the caller's clock, and Rand the source of nonces.
	Now  func() time.Time
	Rand io.Reader
	// Timeout bounds one call, connect to reply; zero is the larger of a minute and the call's wait
	// plus WaitMargin.
	Timeout time.Duration
}

// WaitMargin is how much longer than its wait a client's call may take, when its Timeout is zero:
// the door replies by the end of the wait, and a give's judges may take as long as judge.Budget.
const WaitMargin = 15 * time.Second

// Call signs a call of op with args and sends it.
func (c *Client) Call(op string, args map[string]any) (Reply, error) {
	return c.CallContext(context.Background(), op, args)
}

// CallContext signs a call of op with args and sends it, as Call does. When ctx ends before the
// reply is read, the connection is closed, and the call is CodeSocket.
func (c *Client) CallContext(ctx context.Context, op string, args map[string]any) (Reply, error) {
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(c.Rand, nonce); err != nil {
		return Reply{}, &Error{Code: CodeSocket}
	}
	line, err := SignCall(c.Key, op, c.Accord, c.Now(), nonce, args)
	if err != nil {
		return Reply{}, &Error{Code: CodeReply}
	}
	return c.send(ctx, line, argsWait(args))
}

// argsWait is the wait a call's args ask for, in whole seconds, or 0.
func argsWait(args map[string]any) int {
	switch w := args["wait"].(type) {
	case int:
		return w
	case int64:
		return int(w)
	}
	return 0
}

// Send sends the bytes of one call, as given, and reads the reply. A socket path too long is
// CodeSocketPath, and one that is not a socket of this user's is CodeSocket; nothing is sent. A reply
// longer than MaxReply is CodeReply, read no further, and so is a reply whose outcome or release has
// a receipt under another Accord than the client's, or whose close closes another Accord.
func (c *Client) Send(line []byte) (Reply, error) {
	return c.send(context.Background(), line, 0)
}

// send is Send, with ctx, which closes the connection when it ends, and the call's wait, which
// lengthens a zero Timeout.
func (c *Client) send(ctx context.Context, line []byte, wait int) (Reply, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = max(time.Minute, time.Duration(wait)*time.Second+WaitMargin)
	}
	if err := ctx.Err(); err != nil {
		return Reply{}, &Error{Code: CodeSocket}
	}
	if err := CheckSocket(c.Socket); err != nil {
		return Reply{}, err
	}
	fi, err := os.Lstat(c.Socket)
	if err != nil || fi.Mode()&fs.ModeSocket == 0 {
		return Reply{}, &Error{Code: CodeSocket}
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int64(st.Uid) != int64(os.Geteuid()) {
		return Reply{}, &Error{Code: CodeSocket}
	}
	conn, err := net.DialTimeout("unix", c.Socket, timeout)
	if err != nil {
		return Reply{}, &Error{Code: CodeSocket}
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(append(append([]byte(nil), line...), '\n')); err != nil {
		return Reply{}, &Error{Code: CodeSocket}
	}
	b, err := bufio.NewReaderSize(io.LimitReader(conn, MaxReply+1), 64<<10).ReadBytes('\n')
	switch {
	case len(b) > MaxReply && err != nil:
		return Reply{}, &Error{Code: CodeReply}
	case err != nil:
		return Reply{}, &Error{Code: CodeSocket}
	}
	r, ok := ParseReply(bytes.TrimSuffix(b, []byte{'\n'}))
	if !ok {
		return Reply{}, &Error{Code: CodeReply}
	}
	for _, o := range []*mediator.Outcome{r.Outcome, r.Release} {
		if o == nil {
			continue
		}
		if st, err := receipts.Decode(o.Receipt); err != nil || st.Predicate.Accord != c.Accord {
			return Reply{}, &Error{Code: CodeReply}
		}
	}
	if r.Close != nil {
		if cl, err := receipts.DecodeClose(r.Close); err != nil || cl.Accord != c.Accord {
			return Reply{}, &Error{Code: CodeReply}
		}
	}
	return r, nil
}
