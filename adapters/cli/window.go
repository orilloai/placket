package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/setup"
)

func init() {
	commands["window"] = windowCommand
}

// windowCommand is `window --key F --founding S --socket P --journal DIR`: it runs the mediator of
// the Accord S founds, behind its door on the socket, with its journal in DIR, until it is
// interrupted. It prints the Accord id, the card hash and the operator, then one line per event:
// ids, words and codes, never a body, a salt, a seed, a key or a question's part. A founding set
// whose card names another module than this build's is tool error founding kernel, after a line
// that names both (docs/adr/0012-the-kernel-hash.md).
//
// On a closed journal it prints `accord <id> closed <close id>` and serves the closed Accord. A
// journal whose close was cut short is tool error journal, receipts.jsonl, close, after a line that
// says to run `placket window close`. `window close` is closeCommand.
func windowCommand(r *run, args []string) int {
	if len(args) > 0 && args[0] == "close" {
		return closeCommand(r, args[1:])
	}
	fs := flags("window")
	keyPath, founding := fs.String("key", "", "the mediator's key file"), fs.String("founding", "", "the founding set")
	socket, dir := fs.String("socket", "", "socket"), fs.String("journal", "", "journal directory")
	if !r.parse(fs, args, "key", "founding", "socket", "journal") {
		return r.tool(codeUsage)
	}
	k, err := keyfile.Read(*keyPath)
	if err != nil {
		return r.failed(err)
	}
	set, err := os.ReadFile(*founding)
	if err != nil {
		return r.tool(codeFile, "founding")
	}
	set = bytes.TrimSuffix(set, []byte{'\n'})
	// A card of another module is refused before the set is founded; one that does not read is left to
	// founding (docs/adr/0012-the-kernel-hash.md).
	if k, ok := kernelOf(cardOf(set)); ok {
		if l := otherModule(k); l != "" {
			fmt.Fprintln(r.stderr, l)
			return r.tool(codeFound, detailKernel)
		}
	}
	operator := memberString(set, "mediator", "operator")
	err = local.Run(r.env.Context, local.Window{
		Key: k, Founding: set, Socket: *socket, Journal: *dir, Disk: r.env.Disk,
		Now: r.env.Now, Stderr: r.stderr, Ready: r.env.Ready,
		Event: func(e local.Event) { r.event(e, operator) },
	})
	return r.windowError(err)
}

// windowError is the exit of a window that stopped: 0 when it stopped as asked, and otherwise the
// tool error of what stopped it.
func (r *run) windowError(err error) int {
	if err == nil {
		return ExitOK
	}
	var se *setup.Error
	var me *mediator.Error
	var je *journal.Error
	switch {
	case errors.As(err, &se):
		return r.tool(codeFound, string(se.Code))
	case errors.Is(err, mediator.ErrOtherMediator):
		return r.tool(codeFound, "mediator")
	case errors.As(err, &me) && me.Code == mediator.CodeClosed:
		return r.tool(codeClosed)
	case errors.As(err, &je) && je.Core == string(mediator.CodeClose):
		fmt.Fprintln(r.stderr, LineTornClose)
	case errors.As(err, &je) && je.Core == local.CoreCloseSize:
		fmt.Fprintln(r.stderr, LineCloseSize)
	}
	return r.failed(err)
}

// closeCommand is `window close --key F --founding S --journal DIR --accord ID [--socket P]`
// (close-1, close-3): it closes the Accord of a stopped window's journal. It rebuilds the mediator as
// a restart does, closes each open question, a timeout at its deadline where that has passed and
// closed otherwise, writes the close receipt last, and prints `close <id>` and one line per closing.
// --accord must be the founding set's Accord id: the close cannot be undone. A running window holds
// the journal's lock, so the command is journal-locked; an empty journal is tool error journal,
// unfounded; a closed one is tool error closed; a close that would not fit in one reply is tool
// error journal, receipts.jsonl, close-size, after LineCloseSize, and writes nothing. With --socket it then serves the closed Accord until
// interrupted, as `window` does; without it, it exits 0. Its --json object, written at exit, holds
// accord, close (closeObject) and closings, in both modes.
func closeCommand(r *run, args []string) int {
	fs := flags("window close")
	keyPath, founding := fs.String("key", "", "the mediator's key file"), fs.String("founding", "", "the founding set")
	socket, dir := fs.String("socket", "", "socket"), fs.String("journal", "", "journal directory")
	accord := fs.String("accord", "", "the Accord id, the founding set's")
	if !r.parse(fs, args, "key", "founding", "journal", "accord") || !accordID(*accord) {
		return r.tool(codeUsage)
	}
	k, err := keyfile.Read(*keyPath)
	if err != nil {
		return r.failed(err)
	}
	set, err := os.ReadFile(*founding)
	if err != nil {
		return r.tool(codeFile, "founding")
	}
	set = bytes.TrimSuffix(set, []byte{'\n'})
	if id, err := canonical.TypedHash(set); err != nil || id != *accord {
		fmt.Fprintln(r.stderr, "usage: placket window close: --accord is not the founding set's Accord id")
		return r.tool(codeUsage)
	}
	if k, ok := kernelOf(cardOf(set)); ok {
		if l := otherModule(k); l != "" {
			fmt.Fprintln(r.stderr, l)
			return r.tool(codeFound, detailKernel)
		}
	}
	operator := memberString(set, "mediator", "operator")
	err = local.Run(r.env.Context, local.Window{
		Key: k, Founding: set, Socket: *socket, Journal: *dir, Disk: r.env.Disk, Close: true,
		Now: r.env.Now, Stderr: r.stderr, Ready: r.env.Ready,
		Event: func(e local.Event) { r.event(e, operator) },
	})
	return r.windowError(err)
}

// closeObject is the close as the --json object of window and window close holds it, in both modes:
// its id and its receipt, as an outcome is.
func closeObject(e local.Event) map[string]any {
	return map[string]any{memberReceiptID: e.Close, "receipt": json.RawMessage(e.CloseReceipt)}
}

// event prints one line per event of the window.
func (r *run) event(e local.Event, operator string) {
	switch e.Kind {
	case local.EventStarted:
		how := wordReloaded
		if e.Founded {
			how = wordFounded
		}
		if e.Close != "" {
			how = wordClosed
			fmt.Fprintln(r.stderr, "accord "+e.Accord+" "+how+" "+e.Close)
			r.put(memberClose, closeObject(e))
		} else {
			fmt.Fprintln(r.stderr, "accord "+e.Accord+" "+how)
		}
		fmt.Fprintln(r.stderr, "card "+e.Card)
		fmt.Fprintln(r.stderr, "operator "+operator)
		r.put(memberAccord, e.Accord)
		r.put(memberCard, e.Card)
		r.put(memberOperator, operator)
		r.put("started", how)
		return
	case local.EventRefused:
		fmt.Fprintln(r.stderr, LineDoor+" "+string(e.Door))
		return
	case local.EventClosed:
		// The close's own line, then one per closing, as the window prints a reload's.
		fmt.Fprintln(r.stderr, memberClose+" "+e.Close)
		closings := []any{}
		for i := range e.Outcomes {
			closings = append(closings, outcomeObject(&e.Outcomes[i], word(e.Outcomes[i].Verdict)))
		}
		r.put(memberAccord, e.Accord)
		r.put(memberClosings, closings)
		r.put(memberClose, closeObject(e))
		if len(e.Outcomes) == 0 {
			return
		}
	}
	op := e.Op
	if op == "" {
		op = e.Kind
	}
	switch {
	case e.Rejection != "":
		fmt.Fprintln(r.stderr, op+" "+LineRejected+" "+string(e.Rejection))
	case e.Error != "":
		fmt.Fprintln(r.stderr, op+" "+LineError+" "+string(e.Error))
	case e.Door != "":
		fmt.Fprintln(r.stderr, op+" "+LineDoor+" "+string(e.Door))
	case e.Fetched != "":
		fmt.Fprintln(r.stderr, op+" "+wordFetched+" "+e.Fetched)
	case len(e.Outcomes) == 0:
		fmt.Fprintln(r.stderr, op)
	}
	for i, o := range e.Outcomes {
		w := word(o.Verdict)
		if i == e.Release && w == WordPass {
			w = WordReleased
		}
		fmt.Fprintln(r.stderr, op+" "+w+" "+o.Reason+" "+o.ReceiptID)
	}
}
