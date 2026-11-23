package local

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
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
	// OpView and OpAccept are a member's own view of the Accord, and its signed acceptance of it, the
	// two ops a proposed Accord serves (setup-contract-14, setup-contract-15,
	// docs/adr/0011-the-door-and-the-journal.md).
	OpView   = "view"
	OpAccept = "accept"
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
// (rejection-vs-error; docs/adr/0011-the-door-and-the-journal.md).
const Internal DoorCode = "door-internal"

// JudgeBusy is the door's code for a caller's give while its last give is with a judge. It is checked
// after the nonce, so the refused call's nonce is spent, and before the tick, so the refused give
// writes nothing. It is about the order of the caller's calls: a later give never reaches the core
// before an earlier one (crossing-4, docs/adr/0011-the-door-and-the-journal.md).
const JudgeBusy DoorCode = "judge-busy"

// NotFounded is the door's code for a call of any op but view and accept to a proposed Accord, one
// whose members have not all accepted their views: it is not founded, so nothing crosses yet
// (setup-contract-15, docs/adr/0011-the-door-and-the-journal.md). It is checked after the nonce, as JudgeBusy is.
const NotFounded DoorCode = "not-founded"

// MaxReply is the most bytes of a reply, before its line feed, a client reads. It is enough for one
// delivery, whose body came in a call under MaxCall, with its receipt and the answer call inside it
// (docs/adr/0011-the-door-and-the-journal.md).
const MaxReply = 4 * MaxCall

// A Call is a call as the door read it: its members, its canonical bytes and its signature as
// received.
type Call struct {
	Op, Caller, Accord, At, Nonce string
	// For give, the message's and envelope's bytes, as decoded.
	Message, Envelope []byte
	// For result, the question's id.
	Question string
	// For questions, get and result: how many whole seconds the caller lets the door hold the call
	// until it has something to return, 0 when the call does not wait (at most MaxWait).
	Wait int
	// For questions: the cursor of an earlier questions reply, and whether the call carries one.
	After    string
	HasAfter bool
	// For resolve: the answerer's signed answer call, its canonical bytes as the call carried them;
	// the answer's text, in Answer, and whether the call carries one; and the answerer's salt, nil
	// when the call carries none.
	AnswerCall []byte
	Answer     string
	HasAnswer  bool
	Salt       []byte
	// For accept: the member's signed acceptance, its canonical bytes as the call carried them.
	Acceptance []byte
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
	Answer json.RawMessage `json:"answer"`
	Salt   *string         `json:"salt"`
	Text   *string         `json:"text"`
}

type resultArgs struct {
	Question *string `json:"question"`
	Wait     *int64  `json:"wait"`
}

type acceptArgs struct {
	Acceptance json.RawMessage `json:"acceptance"`
}

type getArgs struct {
	Wait *int64 `json:"wait"`
}

type questionsArgs struct {
	After *string `json:"after"`
	Wait  *int64  `json:"wait"`
}

// MaxWait is the most whole seconds a call may ask the door to hold it.
const MaxWait = 60

// wait reads a call's wait: absent is 0, and anything but a whole number from 0 to MaxWait is not one.
func wait(w *int64) (int, bool) {
	if w == nil {
		return 0, true
	}
	if *w < 0 || *w > MaxWait {
		return 0, false
	}
	return int(*w), true
}

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
	ok := true
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
		if !strict.Decode(m.Args, &a) || len(a.Answer) == 0 || a.Answer[0] != '{' {
			return Call{}, BadCall
		}
		if c.AnswerCall, err = canonical.Form(a.Answer); err != nil {
			return Call{}, BadCall
		}
		if a.Text != nil {
			c.Answer, c.HasAnswer = *a.Text, true
		}
		if a.Salt != nil {
			if c.Salt, err = hex.DecodeString(*a.Salt); err != nil || !lowerHex(*a.Salt) {
				return Call{}, BadCall
			}
		}
	case OpResult:
		var a resultArgs
		if !strict.Decode(m.Args, &a) || a.Question == nil {
			return Call{}, BadCall
		}
		c.Question = *a.Question
		if c.Wait, ok = wait(a.Wait); !ok {
			return Call{}, BadCall
		}
	case OpGet:
		var a getArgs
		if !strict.Decode(m.Args, &a) {
			return Call{}, BadCall
		}
		if c.Wait, ok = wait(a.Wait); !ok {
			return Call{}, BadCall
		}
	case OpView:
		var a struct{}
		if !strict.Decode(m.Args, &a) {
			return Call{}, BadCall
		}
	case OpAccept:
		var a acceptArgs
		if !strict.Decode(m.Args, &a) || len(a.Acceptance) == 0 || a.Acceptance[0] != '{' {
			return Call{}, BadCall
		}
		if c.Acceptance, err = canonical.Form(a.Acceptance); err != nil {
			return Call{}, BadCall
		}
	case OpQuestions:
		var a questionsArgs
		if !strict.Decode(m.Args, &a) {
			return Call{}, BadCall
		}
		if c.Wait, ok = wait(a.Wait); !ok {
			return Call{}, BadCall
		}
		if a.After != nil {
			c.After, c.HasAfter = *a.After, true
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

// AcceptArgs is the args of an accept call: the member's signed acceptance, as a JSON object, the
// canonical bytes it was signed as (setup-contract-15).
func AcceptArgs(acceptance []byte) map[string]any {
	return map[string]any{"acceptance": json.RawMessage(acceptance)}
}

// ResolveArgs is the args of a resolve call: the answerer's signed answer call, and for an accept or
// a change, given with a salt, the answer's text and that salt in hex (maybe-11). The call travels as
// a JSON object, the canonical bytes it was signed as.
func ResolveArgs(call []byte, text string, salt []byte) map[string]any {
	a := map[string]any{"answer": json.RawMessage(call)}
	if salt != nil {
		a["text"], a["salt"] = text, hex.EncodeToString(salt)
	}
	return a
}
