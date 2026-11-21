package cli

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/orilloai/placket/core/mediator"
)

// With --json a command prints on standard output exactly one JSON object, once it has done its
// work, and nothing else there; standard error and the exit code stay as they are without it
// (spec/protocol.md, section 11, commands-1 to commands-4). A command puts each member as it goes,
// whether or not --json was given, and Run prints the object with its exit code.

// The members of the object.
const (
	memberExit       = "exit"
	memberRejection  = "rejection"
	memberFounding   = "founding"
	memberDoor       = LineDoor
	memberError      = LineError
	memberDetail     = "detail"
	memberWord       = "word"
	memberReason     = "reason"
	memberReceiptID  = "receiptId"
	memberReceipt    = "receipt"
	memberOutcome    = "outcome"
	memberRelease    = "release"
	memberQuestion   = "question"
	memberClosing    = "closing"
	memberFetched    = wordFetched
	memberAnswer     = "answer"
	memberDeliveries = "deliveries"
	memberDelivery   = "delivery"
	memberPath       = "path"
	memberComment    = "comment"
	memberCrossed    = "crossed"
	memberClaim      = "claim"
	memberNotClaimed = "notClaimed"
	memberAccord     = "accord"
	memberCard       = "card"
	memberOperator   = "operator"
	memberMatches    = "matches"
	memberClose      = "close"
	memberClosings   = "closings"
)

// A report is the object a command builds: the members put so far, and the object members go into
// now, the top one or a nested one.
type report struct {
	mu       sync.Mutex
	top, cur map[string]any
}

func newReport() *report {
	top := map[string]any{}
	return &report{top: top, cur: top}
}

// put sets a member of the object members go into now.
func (r *run) put(name string, v any) {
	r.rep.mu.Lock()
	defer r.rep.mu.Unlock()
	r.rep.cur[name] = v
}

// add appends v to the list under name.
func (r *run) add(name string, v any) {
	r.rep.mu.Lock()
	defer r.rep.mu.Unlock()
	l, _ := r.rep.cur[name].([]any)
	r.rep.cur[name] = append(l, v)
}

// nest makes the members put until done is called go into an object under name.
func (r *run) nest(name string) (done func()) {
	r.rep.mu.Lock()
	defer r.rep.mu.Unlock()
	o, outer := map[string]any{}, r.rep.cur
	outer[name] = o
	r.rep.cur = o
	return func() {
		r.rep.mu.Lock()
		defer r.rep.mu.Unlock()
		r.rep.cur = outer
	}
}

// setJSON turns --json on or off: on, standard output is the object's alone.
func (r *run) setJSON(on bool) {
	r.asJSON = on
	if on {
		r.stdout = io.Discard
	} else {
		r.stdout = r.realOut
	}
}

// wantsJSON reports whether --json is among the arguments, for a command whose flags were not read.
func wantsJSON(args []string) bool {
	for _, a := range args {
		switch a {
		case "--":
			return false
		case "-json", "--json", "-json=true", "--json=true":
			return true
		}
	}
	return false
}

// printJSON prints the object with the exit code, escaped on a terminal as any JSON line is, and
// reports whether it was written. Once it has run, Run prints no object again.
func (r *run) printJSON(code int) bool {
	r.rep.mu.Lock()
	r.rep.top[memberExit] = code
	b := canonicalJSON(r.rep.top)
	if b == nil {
		b, _ = json.Marshal(r.rep.top)
	}
	r.rep.mu.Unlock()
	r.stdout = r.realOut
	r.printed = true
	_, err := r.stdout.Write(append(append([]byte(nil), r.screenJSON(b)...), '\n'))
	return err == nil
}

// outcomeObject is an outcome as the object holds it, under the word its line prints.
func outcomeObject(o *mediator.Outcome, w string) map[string]any {
	return map[string]any{memberWord: w, memberReason: o.Reason, memberReceiptID: o.ReceiptID, memberReceipt: json.RawMessage(o.Receipt)}
}

// deliveryObject is a fetched delivery as the object holds it, with its body unless it was written
// to a file.
func deliveryObject(d *delivery, body bool) map[string]any {
	o := map[string]any{memberReceiptID: d.ReceiptID, memberReceipt: d.Receipt, "messageId": d.MessageID, "kind": d.Kind, "salt": d.Salt}
	if d.Re != "" {
		o["re"] = d.Re
	}
	if d.MediaType != nil {
		o["mediaType"] = *d.MediaType
	}
	if len(d.Answer) > 0 {
		o["answer"] = d.Answer
	}
	switch {
	case !body:
	case d.Text != nil:
		o["text"] = *d.Text
	case d.Raw != nil:
		o["raw"] = *d.Raw
	default:
		o["data"] = d.Data
	}
	return o
}
