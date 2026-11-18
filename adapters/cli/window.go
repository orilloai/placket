package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
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
func windowCommand(r *run, args []string) int {
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
	if err == nil {
		return ExitOK
	}
	var se *setup.Error
	switch {
	case errors.As(err, &se):
		return r.tool(codeFound, string(se.Code))
	case errors.Is(err, mediator.ErrOtherMediator):
		return r.tool(codeFound, "mediator")
	}
	return r.failed(err)
}

// event prints one line per event of the window.
func (r *run) event(e local.Event, operator string) {
	switch e.Kind {
	case local.EventStarted:
		how := wordReloaded
		if e.Founded {
			how = wordFounded
		}
		fmt.Fprintln(r.stderr, "accord "+e.Accord+" "+how)
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
