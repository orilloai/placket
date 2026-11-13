package local

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
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
)

// Error is a call that got no reply: its code. It holds nothing of the call or the reply.
type Error struct{ Code Code }

func (e *Error) Error() string { return "local: " + string(e.Code) }

// An Asked is one open question as the questions reply shows it to an answerer. A text part's
// Message is its text and a data part's its canonical JSON; a raw part has no Message, only its
// Size in bytes.
type Asked struct {
	ID, Asker, Kind, Deadline, Form string
	Message                         *string
	Size                            *int
}

// Format prints the Asked as a fixed text, whatever the verb: it holds a question's part
// (secrets-never-print).
func (Asked) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("local.Asked")) }

// A Reply is a reply as a client read it. Exactly one of its kinds is set: outcomes (Outcome, with
// Release and Salt where the reply has them), a delivery (Delivery, with AnswerCall beside it), the
// questions (Questions, with IsQuestions), a refusal (Rejection, Error or Door), or none of them, a
// plain ok.
type Reply struct {
	Outcome, Release *mediator.Outcome
	Salt             []byte
	Delivery         []byte
	AnswerCall       []byte
	Questions        []Asked
	IsQuestions      bool
	Rejection        mediator.RejectionCode
	Error            mediator.ErrorCode
	Door             DoorCode
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
	Asker    *string `json:"asker"`
	Deadline *string `json:"deadline"`
	Form     *string `json:"form"`
	ID       *string `json:"id"`
	Kind     *string `json:"kind"`
	Message  *string `json:"message,omitempty"`
	Size     *int    `json:"size,omitempty"`
}

type wireReply struct {
	OK         *bool           `json:"ok"`
	Outcome    *wireOutcome    `json:"outcome,omitempty"`
	Release    *wireOutcome    `json:"release,omitempty"`
	Salt       *string         `json:"salt,omitempty"`
	Delivery   json.RawMessage `json:"delivery,omitempty"`
	AnswerCall json.RawMessage `json:"answerCall,omitempty"`
	Questions  *[]wireAsked    `json:"questions,omitempty"`
	Rejection  *string         `json:"rejection,omitempty"`
	Error      *string         `json:"error,omitempty"`
	Door       *string         `json:"door,omitempty"`
}

func wire(o mediator.Outcome) *wireOutcome {
	return &wireOutcome{Reason: &o.Reason, Receipt: o.Receipt, ReceiptID: &o.ReceiptID, Verdict: &o.Verdict}
}

// encodeReply writes a reply's canonical bytes.
func encodeReply(w wireReply) []byte {
	t := true
	if w.OK == nil {
		w.OK = &t
	}
	b, err := json.Marshal(w)
	if err != nil {
		return []byte(`{"error":"internal","ok":false}`)
	}
	c, err := canonical.Form(b)
	if err != nil {
		return []byte(`{"error":"internal","ok":false}`)
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

// asked reports whether an open question as a reply names it has the form a window gives it.
func asked(q wireAsked) bool {
	if q.ID == nil || q.Asker == nil || q.Kind == nil || q.Deadline == nil || q.Form == nil {
		return false
	}
	_, ref := signing.ParseReference(*q.Asker)
	switch *q.Form {
	case "text", "raw", "data":
	default:
		return false
	}
	return typedHash(*q.ID) && ref && label(*q.Kind)
}

// ParseReply reads a reply's bytes, its line feed removed. It reads what the reply names for its
// form, since a client cannot tell what window it reached and what a reply names reaches a person's
// terminal: a refusal's code is a label, and an outcome's receipt id, verdict and reason are those
// of its receipt, which must decode (receipts.Decode, which verifies nothing); an open question's
// id is a typed hash, its asker an actor reference, its kind a label and its form text, raw or data.
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
	if w.Salt != nil {
		s, err := hex.DecodeString(*w.Salt)
		if err != nil || r.Outcome == nil || !lowerHex(*w.Salt) {
			return Reply{}, false
		}
		r.Salt = s
	}
	if len(w.Delivery) > 0 {
		d, err := canonical.Form(w.Delivery)
		if err != nil || w.Delivery[0] != '{' || r.Outcome != nil {
			return Reply{}, false
		}
		r.Delivery = d
	}
	if len(w.AnswerCall) > 0 {
		a, err := canonical.Form(w.AnswerCall)
		if err != nil || w.AnswerCall[0] != '{' || r.Delivery == nil {
			return Reply{}, false
		}
		r.AnswerCall = a
	}
	if w.Questions != nil {
		if r.Outcome != nil || r.Delivery != nil {
			return Reply{}, false
		}
		r.IsQuestions = true
		r.Questions = []Asked{}
		for _, q := range *w.Questions {
			if !asked(q) {
				return Reply{}, false
			}
			r.Questions = append(r.Questions, Asked{ID: *q.ID, Asker: *q.Asker, Kind: *q.Kind, Deadline: *q.Deadline,
				Form: *q.Form, Message: q.Message, Size: q.Size})
		}
	}
	return r, true
}

// A Client calls a window as one key, for one Accord.
type Client struct {
	// Socket is the window's socket path.
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

// Send sends the bytes of one call, as given, and reads the reply.
func (c *Client) Send(line []byte) (Reply, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = time.Minute
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
	b, err := bufio.NewReaderSize(conn, 64<<10).ReadBytes('\n')
	if err != nil {
		return Reply{}, &Error{Code: CodeSocket}
	}
	r, ok := ParseReply(bytes.TrimSuffix(b, []byte{'\n'}))
	if !ok {
		return Reply{}, &Error{Code: CodeReply}
	}
	return r, nil
}
