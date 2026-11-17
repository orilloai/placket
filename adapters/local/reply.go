package local

import (
	"bufio"
	"bytes"
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
// queued it), the questions (Questions, with IsQuestions, and More when the window left some out to
// keep the reply under MaxReply), a refusal (Rejection, Error or Door), or none of them, a plain ok.
type Reply struct {
	Outcome, Release *mediator.Outcome
	// Salt is never set: the answerer mints its answer's salt, and the reply carries none.
	//
	// Deprecated: the answerer keeps the salt it minted (docs/adr/0021-who-decided-and-the-answer-call.md).
	// It is removed at the release.
	Salt     []byte
	Delivery []byte
	// AnswerCall is never set: the answer call is inside the delivery, its answer member.
	//
	// Deprecated: read the delivery's answer (docs/adr/0021-who-decided-and-the-answer-call.md). It
	// is removed at the release.
	AnswerCall  []byte
	Questions   []Asked
	IsQuestions bool
	More        bool
	Rejection   mediator.RejectionCode
	Error       mediator.ErrorCode
	Door        DoorCode
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
	Questions *[]wireAsked    `json:"questions,omitempty"`
	More      *bool           `json:"more,omitempty"`
	Rejection *string         `json:"rejection,omitempty"`
	Error     *string         `json:"error,omitempty"`
	Door      *string         `json:"door,omitempty"`
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
)

func refusal(by int, code string) []byte {
	f := false
	w := wireReply{OK: &f}
	switch by {
	case byRejection:
		w.Rejection = &code
	case byError:
		w.Error = &code
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
// of its receipt, which must decode (receipts.Decode, which verifies nothing); an open question's
// id is a typed hash, its asker an actor reference, its kind a label, its form text, raw or data with
// that body, and its receipt and salt of their forms.
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
	if *w.OK != (set == 0) || set > 1 || (set == 1 && !label(string(r.Rejection)+string(r.Error)+string(r.Door))) {
		return Reply{}, false
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
	if w.More != nil && (!*w.More || w.Questions == nil) {
		return Reply{}, false
	}
	if w.Questions != nil {
		if r.Outcome != nil || r.Delivery != nil {
			return Reply{}, false
		}
		r.IsQuestions, r.More = true, w.More != nil
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
	// Timeout bounds one call, connect to reply; zero is a minute.
	Timeout time.Duration
}

// Call signs a call of op with args and sends it.
func (c *Client) Call(op string, args map[string]any) (Reply, error) {
	nonce := make([]byte, 16)
	if _, err := io.ReadFull(c.Rand, nonce); err != nil {
		return Reply{}, &Error{Code: CodeSocket}
	}
	line, err := SignCall(c.Key, op, c.Accord, c.Now(), nonce, args)
	if err != nil {
		return Reply{}, &Error{Code: CodeReply}
	}
	return c.Send(line)
}

// Send sends the bytes of one call, as given, and reads the reply. A socket path too long is
// CodeSocketPath, and one that is not a socket of this user's is CodeSocket; nothing is sent. A reply
// longer than MaxReply is CodeReply, read no further, and so is a reply whose outcome or release has
// a receipt under another Accord than the client's.
func (c *Client) Send(line []byte) (Reply, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = time.Minute
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
	return r, nil
}
