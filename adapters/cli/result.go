package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["result"] = resultCommand
}

// closing is how give --wait and result learn how a question closed (wait-and-result-shared). It asks
// the window for the question's result, once a second while wait is set and the question is open.
// When the question has closed it prints the log's word for the closing and the closing receipt's
// id, with "release blocked <reason>" when the answer passed and its release did not, and the
// receipts on standard output; and after an accept or a change that passed, it fetches the answer or
// the version and prints its text. It exits 0 only for an accept whose release passed. Open, it
// prints open and exits 4; a wait that is interrupted prints the question's id and exits 4.
func (s *session) closing(question string, wait bool) int {
	s.r.put(memberQuestion, question)
	var rep local.Reply
	for {
		var err error
		rep, err = s.client.Call(local.OpResult, map[string]any{"question": question})
		if err != nil {
			return s.r.failed(err)
		}
		if code, refused := s.r.refusal(rep); refused {
			return code
		}
		if rep.Outcome != nil {
			break
		}
		if !wait {
			s.r.put(memberClosing, map[string]any{memberWord: WordOpen})
			fmt.Fprintln(s.r.stderr, WordOpen)
			fmt.Fprintln(s.r.stdout, question)
			return ExitOpen
		}
		select {
		case <-s.r.env.Context.Done():
			s.r.put(memberClosing, map[string]any{memberWord: WordOpen})
			fmt.Fprintln(s.r.stdout, question)
			return ExitOpen
		case <-time.After(s.r.env.Poll):
		}
	}
	st, err := receipts.Decode(rep.Outcome.Receipt)
	if err != nil {
		return s.r.tool("reply")
	}
	w := export.Closing(st.Predicate, st.Subject)
	text := w + " " + rep.Outcome.ReceiptID
	released := false
	if rep.Release != nil {
		if rep.Release.Verdict == string(composition.Pass) {
			released = true
		} else {
			text += " " + LineReleaseBlocked + " " + rep.Release.Reason
		}
	}
	fmt.Fprintln(s.r.stderr, text)
	line(s.r.stdout, s.r.screenJSON(rep.Outcome.Receipt))
	s.r.put(memberClosing, map[string]any{memberWord: w, memberReceiptID: rep.Outcome.ReceiptID, memberReceipt: json.RawMessage(rep.Outcome.Receipt)})
	if rep.Release != nil {
		line(s.r.stdout, s.r.screenJSON(rep.Release.Receipt))
		rw := WordBlock
		if released {
			rw = WordReleased
		}
		s.r.put(memberRelease, outcomeObject(rep.Release, rw))
	}
	code := ExitBlock
	if w == WordAccepted && released {
		code = ExitOK
	}
	if w == WordAccepted || w == WordChanged {
		if c := s.fetchOwn(question, rep.Outcome.ReceiptID); c != 0 {
			return c
		}
	}
	return code
}

// fetchOwn finds the answer or the version to the question, the delivery the closing receipt
// queued: first among the parts kept, then by fetching, every delivery fetched recorded and kept,
// and each that is not its own printed as get prints it. It prints the text found, or not fetched.
func (s *session) fetchOwn(question, closing string) int {
	if err := s.dir.Lock(); err != nil {
		return s.r.failed(err)
	}
	defer s.dir.Unlock()
	kept, err := s.dir.Kept()
	if err != nil {
		return s.r.failed(err)
	}
	for _, l := range kept {
		var k keptDelivery
		if strict.Decode(l, &k) && own(&k.delivery, question, closing) {
			line(s.r.stdout, s.r.screen(k.bytes()))
			s.r.put(memberFetched, true)
			s.r.put(memberAnswer, deliveryObject(&k.delivery, true))
			return 0
		}
	}
	for {
		d, _, code := s.fetch(true)
		if code != 0 {
			return code
		}
		if d == nil {
			fmt.Fprintln(s.r.stderr, LineNotFetched)
			s.r.put(memberFetched, false)
			return 0
		}
		if own(d, question, closing) {
			line(s.r.stdout, s.r.screen(d.bytes()))
			s.r.put(memberFetched, true)
			s.r.put(memberAnswer, deliveryObject(d, true))
			return 0
		}
		s.r.show(d)
		s.r.add(memberDeliveries, deliveryObject(d, true))
	}
}

// changeMessage is the message id the mediator gives the version it forms, before the question's
// id.
const changeMessage = "change:"

// own reports whether the delivery is the answer or the version to the question: the part queued by
// the closing receipt, whose id it names. A part of that kind and re that a member gave has a
// receipt of its own, and is not.
func own(d *delivery, question, closing string) bool {
	return d.ReceiptID == closing && d.Re == question && d.Text != nil &&
		(d.Kind == receipts.NameAnswer || d.Kind == receipts.NameChanged)
}

// resultCommand is `result --question ID`, with the socket flags: how a question the key asked
// closed, as give --wait says it.
func resultCommand(r *run, args []string) int {
	fs := flags("result")
	keyPath, socket, accord, stateDir := socketFlags(fs)
	question := fs.String("question", "", "the question's id")
	if !r.parse(fs, args, "key", "socket", "accord", "question") {
		return r.tool(codeUsage)
	}
	s, code := r.open(*keyPath, *socket, *accord, *stateDir)
	if s == nil {
		return code
	}
	return s.closing(*question, false)
}
