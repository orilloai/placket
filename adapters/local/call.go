package local

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

// MaxCall is the most bytes a call may have, its line feed aside.
const MaxCall = 1 << 20

// StaleSeconds is how far a call's at may be from the door's clock; ReplaySeconds is how long the
// door remembers a nonce.
const (
	StaleSeconds  = 60
	ReplaySeconds = 120
)

// The ops of a call.
const (
	OpGive      = "give"
	OpGet       = "get"
	OpQuestions = "questions"
	OpResolve   = "resolve"
	OpResult    = "result"
)

// DoorCode is the code of a call the door refused. It is about the call, never the operation inside
// it, and is neither a mediator.Rejection nor a mediator.Error (rejection-vs-error).
type DoorCode string

// The door's codes, in the order of its checks (door-check-order).
const (
	TooLarge     DoorCode = "too-large"
	BadCall      DoorCode = "bad-call"
	BadSignature DoorCode = "bad-signature"
	WrongAccord  DoorCode = "wrong-accord"
	Stale        DoorCode = "stale"
	Replayed     DoorCode = "replayed"
)

// Internal is the door's code for a failure of the door itself, after a call passed its checks: the
// salt source failed, a reply could not be encoded, or an error came back that is neither a
// mediator.Rejection nor a mediator.Error. It is never the mediator's internal code
// (rejection-vs-error; docs/adr/0017-the-doors-own-code.md).
const Internal DoorCode = "door-internal"

// MaxReply is the most bytes of a reply, before its line feed, a client reads. It is enough for one
// delivery, whose body came in a call under MaxCall, with its receipt and answer call beside it
// (docs/adr/0017-the-doors-own-code.md).
const MaxReply = 4 * MaxCall

// A Call is a call as the door read it: its members, its canonical bytes and its signature as
// received.
type Call struct {
	Op, Caller, Accord, At, Nonce string
	// For give, the message's and envelope's bytes, as decoded.
	Message, Envelope []byte
	// For resolve and result.
	Question, Action, Answer string
	// HasAnswer is whether a resolve call carries an answer.
	HasAnswer bool
	// Bytes is the call's canonical bytes, and Sig its sig member as received.
	Bytes []byte
	Sig   string
}

// Format prints the Call as a fixed text, whatever the verb: it may hold a body (secrets-never-print).
func (Call) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("local.Call")) }

// GoString prints the Call as a fixed text.
func (Call) GoString() string { return "local.Call" }

// String prints the Call as a fixed text.
func (Call) String() string { return "local.Call" }

type wireCall struct {
	Call json.RawMessage `json:"call"`
	Sig  *string         `json:"sig"`
}

type callMembers struct {
	Op     *string         `json:"op"`
	Caller *string         `json:"caller"`
	Accord *string         `json:"accord"`
	At     *string         `json:"at"`
	Nonce  *string         `json:"nonce"`
	Args   json.RawMessage `json:"args"`
}

type giveArgs struct {
	Message  *string `json:"message"`
	Envelope *string `json:"envelope"`
}

type resolveArgs struct {
	Question *string `json:"question"`
	Action   *string `json:"action"`
	Answer   *string `json:"answer"`
}

type resultArgs struct {
	Question *string `json:"question"`
}

type noArgs struct{}

var b64 = base64.StdEncoding.Strict()

// ParseCall reads a call's bytes, its line feed removed, by the door's first two checks: its size,
// and its form. It does not check the signature.
func ParseCall(line []byte) (Call, DoorCode) {
	if len(line) > MaxCall {
		return Call{}, TooLarge
	}
	if _, err := canonical.Form(line); err != nil {
		return Call{}, BadCall
	}
	var w wireCall
	if !strict.Decode(line, &w) || w.Sig == nil || len(w.Call) == 0 {
		return Call{}, BadCall
	}
	cb, err := canonical.Form(w.Call)
	if err != nil {
		return Call{}, BadCall
	}
	var m callMembers
	if !strict.Decode(w.Call, &m) || m.Op == nil || m.Caller == nil || m.Accord == nil || m.At == nil || m.Nonce == nil || len(m.Args) == 0 {
		return Call{}, BadCall
	}
	if _, err := time.Parse(time.RFC3339, *m.At); err != nil || !timestamp(*m.At) {
		return Call{}, BadCall
	}
	if len(*m.Nonce) != 32 || !lowerHex(*m.Nonce) {
		return Call{}, BadCall
	}
	if _, err := b64.DecodeString(*w.Sig); err != nil {
		return Call{}, BadCall
	}
	c := Call{Op: *m.Op, Caller: *m.Caller, Accord: *m.Accord, At: *m.At, Nonce: *m.Nonce, Bytes: cb, Sig: *w.Sig}
	switch c.Op {
	case OpGive:
		var a giveArgs
		if !strict.Decode(m.Args, &a) || a.Message == nil || a.Envelope == nil {
			return Call{}, BadCall
		}
		if c.Message, err = b64.DecodeString(*a.Message); err != nil {
			return Call{}, BadCall
		}
		if c.Envelope, err = b64.DecodeString(*a.Envelope); err != nil {
			return Call{}, BadCall
		}
	case OpResolve:
		var a resolveArgs
		if !strict.Decode(m.Args, &a) || a.Question == nil || a.Action == nil {
			return Call{}, BadCall
		}
		c.Question, c.Action = *a.Question, *a.Action
		if a.Answer != nil {
			c.Answer, c.HasAnswer = *a.Answer, true
		}
	case OpResult:
		var a resultArgs
		if !strict.Decode(m.Args, &a) || a.Question == nil {
			return Call{}, BadCall
		}
		c.Question = *a.Question
	case OpGet, OpQuestions:
		if !strict.Decode(m.Args, &noArgs{}) {
			return Call{}, BadCall
		}
	default:
		return Call{}, BadCall
	}
	return c, ""
}

// Verified reports whether the call's sig verifies over its canonical bytes under its caller's key
// (call-signing).
func (c Call) Verified() bool {
	sig, err := b64.DecodeString(c.Sig)
	return err == nil && signing.VerifyPayload(PayloadType, c.Bytes, sig, c.Caller)
}

// timestamp is the timestamp rule: whole seconds, Z, a calendar date, 00:00:00 to 23:59:59.
func timestamp(s string) bool {
	if len(s) != 20 || s[4] != '-' || s[7] != '-' || s[10] != 'T' || s[13] != ':' || s[16] != ':' || s[19] != 'Z' {
		return false
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	return err == nil && t.Format("2006-01-02T15:04:05Z") == s
}

func lowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Stamp is a time as a call and the mediator carry it: UTC, whole seconds, Z.
func Stamp(t time.Time) string { return t.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z") }

// SignCall builds and signs a call of op with args, as caller's key, for the Accord, at the time
// at, with the nonce, and returns its bytes, its line feed not included.
func SignCall(key signing.Key, op, accord string, at time.Time, nonce []byte, args map[string]any) ([]byte, error) {
	if args == nil {
		args = map[string]any{}
	}
	call := map[string]any{"op": op, "caller": key.Reference(), "accord": accord, "at": Stamp(at),
		"nonce": hex.EncodeToString(nonce), "args": args}
	b, err := json.Marshal(call)
	if err != nil {
		return nil, err
	}
	cb, err := canonical.Form(b)
	if err != nil {
		return nil, err
	}
	sig, err := signing.SignPayload(PayloadType, cb, key)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(map[string]any{"call": json.RawMessage(cb), "sig": base64.StdEncoding.EncodeToString(sig)})
	if err != nil {
		return nil, err
	}
	return canonical.Form(out)
}

// GiveArgs is the args of a give call: the message's and the envelope's exact bytes, in base64.
func GiveArgs(message, envelope []byte) map[string]any {
	return map[string]any{"message": base64.StdEncoding.EncodeToString(message), "envelope": base64.StdEncoding.EncodeToString(envelope)}
}

// ResolveArgs is the args of a resolve call; the answer is sent with an accept or a change.
func ResolveArgs(question string, action mediator.Action, answer string) map[string]any {
	a := map[string]any{"question": question, "action": string(action)}
	if action == mediator.Accepted || action == mediator.Changed {
		a["answer"] = answer
	}
	return a
}

// AnswerCall is the answer call as the door attaches it to a delivery: {"call","sig"}.
func AnswerCall(c Call) []byte {
	b, _ := json.Marshal(map[string]string{"call": base64.StdEncoding.EncodeToString(c.Bytes), "sig": c.Sig})
	out, _ := canonical.Form(b)
	return out
}

// A SignedAnswer is what a verified answer call says.
type SignedAnswer struct {
	Accord, Question, Action, Answer string
}

// Format prints the SignedAnswer as a fixed text, whatever the verb: its answer is a body
// (secrets-never-print).
func (SignedAnswer) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("local.SignedAnswer")) }

// GoString is the fixed text, for %#v.
func (SignedAnswer) GoString() string { return "local.SignedAnswer" }

// String is the fixed text.
func (SignedAnswer) String() string { return "local.SignedAnswer" }

// VerifyAnswerCall checks an answer call kept beside a delivery: its form, its signature under the
// answerer's key, and that it is a resolve call that accepts or changes with an answer. It returns
// what the call says; whoever reads it checks the rest against the receipts.
func VerifyAnswerCall(answerCall []byte, answerer string) (SignedAnswer, bool) {
	var a struct {
		Call *string `json:"call"`
		Sig  *string `json:"sig"`
	}
	if !strict.Decode(answerCall, &a) || a.Call == nil || a.Sig == nil {
		return SignedAnswer{}, false
	}
	cb, err := b64.DecodeString(*a.Call)
	if err != nil {
		return SignedAnswer{}, false
	}
	if c, err := canonical.Form(cb); err != nil || subtle.ConstantTimeCompare(c, cb) != 1 {
		return SignedAnswer{}, false
	}
	line, err := json.Marshal(map[string]any{"call": json.RawMessage(cb), "sig": *a.Sig})
	if err != nil {
		return SignedAnswer{}, false
	}
	c, code := ParseCall(line)
	if code != "" || c.Caller != answerer || !c.Verified() || c.Op != OpResolve || !c.HasAnswer ||
		(c.Action != string(mediator.Accepted) && c.Action != string(mediator.Changed)) {
		return SignedAnswer{}, false
	}
	return SignedAnswer{Accord: c.Accord, Question: c.Question, Action: c.Action, Answer: c.Answer}, true
}
