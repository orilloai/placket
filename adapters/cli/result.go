package cli

import (
	"encoding/json"
	"fmt"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["result"] = resultCommand
}

// closing is how give --wait and result learn how a question closed (wait-and-result-shared). It asks
// the window for the question's result, holding no lock: with wait, the door holds the call up to
// that many seconds while the question is open; with untilClosed, it asks again, each time with the
// longest wait, until the question closes. When the question has closed it prints the log's word for
// the closing and the closing receipt's id, with "release blocked <reason>" when the answer passed and
// its release did not, and the receipts on standard output; and after an accept or a change that
// passed, it fetches the answer or the version, under the lock, and prints its text. It exits 0 only
// for an accept whose release passed. Open, it prints open and exits 4; a wait that is interrupted
// prints the question's id and exits 4.
func (s *session) closing(question string, wait int, untilClosed bool) int {
	s.r.put(memberQuestion, question)
	if untilClosed {
		wait = local.MaxWait
	}
	var rep local.Reply
	for {
		var err error
		rep, err = s.c.Result(s.r.env.Context, question, wait)
		if err != nil && untilClosed && s.r.env.Context.Err() != nil {
			s.r.put(memberClosing, map[string]any{memberWord: WordOpen})
			fmt.Fprintln(s.r.stdout, question)
			return ExitOpen
		}
		if err != nil {
			return s.r.failed(err)
		}
		if code, refused := s.r.refusal(rep); refused {
			return code
		}
		if rep.Outcome != nil {
			break
		}
		if !untilClosed {
			s.r.put(memberClosing, map[string]any{memberWord: WordOpen})
			fmt.Fprintln(s.r.stderr, WordOpen)
			fmt.Fprintln(s.r.stdout, question)
			return ExitOpen
		}
	}
	st, err := receipts.Decode(rep.Outcome.Receipt)
	if err != nil {
		return s.r.tool(string(client.CodeReply))
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
// queued (client.FetchOwn): first among the parts kept, then by fetching, every delivery fetched
// recorded and kept, and each that is not its own printed as get prints it. It prints the text found,
// or not fetched. A delivery whose record could not be written is printed all the same, before the
// state directory's error; one whose body cannot be written either is kept in kept.jsonl, as get
// keeps it, and the error is error file kept. The state lock is held from the fetch to that keep.
func (s *session) fetchOwn(question, closing string) int {
	if err := s.c.Lock(); err != nil {
		return s.r.failed(err)
	}
	defer s.c.Unlock()
	own, err := s.c.FetchOwn(s.r.env.Context, question, closing)
	written := true
	for _, d := range own.Others {
		written = s.r.show(d)
		s.r.add(memberDeliveries, deliveryObject(d, true))
	}
	if err != nil {
		// The delivery the window handed over before a record failed is no longer queued: it is
		// printed, and then the error. Its body not written, it is kept.
		if own.Delivery != nil {
			written = writeLine(s.r.stdout, s.r.screen(own.Delivery.Bytes()))
			s.r.put(memberFetched, true)
			s.r.put(memberAnswer, deliveryObject(own.Delivery, true))
		}
		if own.Unkept != nil && !written {
			if kerr := s.c.Dir().Keep(own.Unkept); kerr != nil {
				return s.r.failed(kerr)
			}
			return s.r.tool(codeFile, detailKept)
		}
		return s.r.failed(err)
	}
	if code, refused := s.r.refusal(own.Reply); refused {
		return code
	}
	if own.Delivery == nil {
		fmt.Fprintln(s.r.stderr, LineNotFetched)
		s.r.put(memberFetched, false)
		return 0
	}
	line(s.r.stdout, s.r.screen(own.Delivery.Bytes()))
	s.r.put(memberFetched, true)
	s.r.put(memberAnswer, deliveryObject(own.Delivery, true))
	return 0
}

// changeMessage is the message id the mediator gives the version it forms, before the question's
// id.
const changeMessage = "change:"

// resultCommand is `result --question ID [--wait N]`, with the socket flags: how a question the key
// asked closed, as give --wait says it. With --wait the door holds the call, at most N seconds and
// with no lock held, while the question is open.
func resultCommand(r *run, args []string) int {
	fs := newFlags("result")
	so := socketFlags(fs)
	question := fs.text("question")
	wait := fs.number("wait")
	// A question id that is not UTF-8 is refused, never repaired (parse-strict).
	if !r.parseSocket(fs, args, so) || !allUTF8(*question) || *wait < 0 || *wait > local.MaxWait {
		return r.tool(codeUsage)
	}
	s, code := r.open(so)
	if s == nil {
		return code
	}
	return s.closing(*question, *wait, false)
}
