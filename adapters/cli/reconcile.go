package cli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands["reconcile"] = reconcileCommand
}

// reconcileCommand is `reconcile --mediator M --accord A --card C --receipts DIR --kept DIR...`:
// receipts.Reconcile of the journal's receipts with what each state directory kept. It prints
// {"missing","removed","unrecorded"} on standard output and one line per entry on standard error,
// or that there is nothing to reconcile.
func reconcileCommand(r *run, args []string) int {
	fs := flags("reconcile")
	med, accord, card := fs.String("mediator", "", "mediator"), fs.String("accord", "", "accord"), fs.String("card", "", "card")
	dir := fs.String("receipts", "", "journal directory")
	var kept many
	fs.Var(&kept, "kept", "a state directory")
	if !r.parse(fs, args, "mediator", "accord", "card", "receipts", "kept") {
		return r.tool(codeUsage)
	}
	list, err := journal.ReadReceipts(*dir, r.stderr)
	if err != nil {
		return r.failed(err)
	}
	var ks []receipts.Kept
	for _, path := range kept {
		d, err := state.Read(path)
		if err != nil {
			return r.failed(err)
		}
		envs, err := d.Envelopes()
		if err != nil {
			return r.failed(err)
		}
		del, err := d.Delivered()
		if err != nil {
			return r.failed(err)
		}
		ks = append(ks, receipts.Kept{Actor: d.Actor(), Envelopes: envs, Delivered: del})
	}
	rep, err := receipts.Reconcile(*med, *accord, *card, list, ks)
	if err != nil {
		// A state directory whose actor file holds no actor reference: the error names the --kept
		// it came from, in order, and its actor file, never what the file holds.
		var re *receipts.Error
		if errors.As(err, &re) && re.Code == receipts.CodeKept && re.Actor == "" {
			return r.tool(codeVerify, string(re.Code), "--kept", strconv.Itoa(re.Index+1), state.FileActor)
		}
		return r.notVerified(err)
	}
	items := func(l []receipts.Item) []map[string]string {
		out := []map[string]string{}
		for _, i := range l {
			out = append(out, map[string]string{"actor": i.Actor, "receiptId": i.ReceiptID})
		}
		return out
	}
	missing := []map[string]any{}
	for _, m := range rep.Missing {
		missing = append(missing, map[string]any{"actor": m.Actor, "seq": m.Seq})
	}
	// An actor is read from a state directory's actor file, which whoever handed the directory over
	// wrote: on its way to a terminal it is escaped, as any field and any JSON of another's is.
	lists := map[string]any{"removed": items(rep.Removed), "unrecorded": items(rep.Unrecorded), "missing": missing}
	line(r.stdout, r.screenJSON(canonicalJSON(lists)))
	for name, v := range lists {
		r.put(name, v)
	}
	for _, i := range rep.Removed {
		fmt.Fprintln(r.stderr, "removed "+field(i.Actor)+" "+field(i.ReceiptID))
	}
	for _, i := range rep.Unrecorded {
		fmt.Fprintln(r.stderr, "unrecorded "+field(i.Actor)+" "+field(i.ReceiptID))
	}
	for _, m := range rep.Missing {
		fmt.Fprintln(r.stderr, "missing "+field(m.Actor)+" seq "+strconv.FormatInt(m.Seq, 10))
	}
	if len(rep.Removed)+len(rep.Unrecorded)+len(rep.Missing) == 0 {
		fmt.Fprintln(r.stderr, LineNothing)
	}
	return ExitOK
}
