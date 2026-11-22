package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/adapters/judge/jev"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/setup"
)

func init() {
	commands["window"] = windowCommand
	subcommands["window close"] = closeCommand
}

// windowCommand is `window --key F --founding S --openings O [--acceptances A] [--socket P]
// [--journal DIR]`: it runs the mediator of the Accord S founds, behind its door on the socket,
// with its journal in DIR, until it is interrupted. O is the directory of every member's opening,
// each <hex of its actor reference>.opening.json; one missing is error founding opening-missing.
// The acceptances are the file --acceptances names or, without it, the acceptances.json beside S,
// which the window writes once every member has accepted (readAcceptances). On an empty journal
// with neither the Accord starts proposed (setup-contract-15): the window serves each member its
// own view and takes its acceptance, refuses every other call with door not-founded, and once every
// member has accepted writes acceptances.json beside S, prints `accord <id> founded` and serves as
// founded. With either it is founded at once, on an empty journal as on a founded one, so
// collecting the members' consent again means moving that file away. With no --socket it listens on
// the Accord's default socket, and with no --journal it writes to the Accord's default journal
// (local.DefaultSocketWith, local.DefaultJournalWith); with neither an absolute XDG_STATE_HOME nor
// an absolute HOME there is none, and no --journal is tool error usage. It prints the Accord id,
// the card hash and the operator, then one line per event: ids, words and codes, never a body, a
// salt, a seed, a key or a question's part. A founding set whose card names another module than
// this build's is tool error founding kernel, after a line that names both
// (docs/adr/0012-the-kernel-hash.md).
//
// On a closed journal it prints `accord <id> closed <close id>` and serves the closed Accord. A
// journal whose close was cut short is tool error journal, receipts.jsonl, close, after a line that
// says to run `placket window close`. `window close` is closeCommand.
//
// Each --judge is the config of a judge the card lists (docs/adr/0023-judges.md): a window that can
// take a give needs exactly the card's, each at the card's hash, or it is tool error founding judge
// and starts nothing. The judges' key is read once, from jev.KeyEnv. After the operator line the
// window prints, for each judge the card lists, which kinds may go to which host and model, or that
// it sends nothing.
func windowCommand(r *run, args []string) int {
	fs := newFlags("window")
	keyPath, founding := fs.text("key"), fs.text("founding")
	socket, dir := fs.text("socket"), fs.text("journal")
	openingsDir, accPath := fs.text("openings"), fs.text("acceptances")
	var judges many
	fs.list(&judges, "judge")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	loaded, code := r.judgeConfigs(judges)
	if code != 0 {
		return code
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
	// With no --socket or --journal the window takes the Accord's defaults: its socket under the
	// user's runtime directory, made mode 0700, and its journal under the user's state directory. A
	// --socket that is the default socket has its directory made too, as the labels config names
	// it and the runtime directory may be gone since (docs/adr/0027-placket-init.md). The environment
	// is read for it only when the socket's directory is missing.
	if _, err := os.Stat(filepath.Dir(*socket)); *socket != "" && errors.Is(err, os.ErrNotExist) && isDefaultSocket(r, set, *socket) {
		if err := local.CheckSocket(*socket); err != nil {
			return r.failed(err)
		}
		if err := local.MakeSocketDir(*socket); err != nil {
			return r.failed(err)
		}
	}
	if *socket == "" || *dir == "" {
		id, err := canonical.TypedHash(set)
		if err != nil {
			return r.tool(codeFound, string(setup.NotCanonical))
		}
		if *dir == "" {
			// With no state directory to put it under, the journal must be named.
			if *dir = local.DefaultJournalWith(r.getenv, id); *dir == "" {
				return r.tool(codeUsage)
			}
		}
		if *socket == "" {
			*socket = local.DefaultSocketWith(r.getenv, id)
			if err := local.CheckSocket(*socket); err != nil {
				return r.failed(err)
			}
			if err := local.MakeSocketDir(*socket); err != nil {
				return r.failed(err)
			}
		}
	}
	openings, err := readOpenings(*openingsDir, sealedActors(set))
	if err != nil {
		return r.tool(codeFile, "opening")
	}
	beside := filepath.Join(filepath.Dir(*founding), FileAcceptances)
	acceptances, code := r.readAcceptances(*accPath, beside)
	if code != 0 {
		return code
	}
	operator := memberString(set, "mediator", "operator")
	// The key is read once, here, and held only by the judges' adapters (docs/adr/0023-judges.md).
	key := ""
	if r.env.Getenv != nil {
		key = r.env.Getenv(jev.KeyEnv)
	}
	err = local.Run(r.env.Context, local.Window{
		Key: k, Founding: set, Openings: openings, Acceptances: acceptances, Socket: *socket, Journal: *dir, Disk: r.env.Disk,
		Accepted: func(b []byte) error { return writeAcceptances(beside, b) },
		Now:      r.env.Now, Stderr: r.stderr, Ready: r.env.Ready,
		Event:    func(e local.Event) { r.event(e, operator) },
		Judges:   loaded,
		NewJudge: func(c judge.Config) judge.Judge { return jev.New(c, key, nil) },
		Keyed:    key != "",
	})
	return r.windowError(err)
}

// isDefaultSocket reports whether socket is the default socket of the Accord the set founds.
func isDefaultSocket(r *run, set []byte, socket string) bool {
	id, err := canonical.TypedHash(set)
	return err == nil && socket == local.DefaultSocketWith(r.getenv, id)
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
	var le *local.Error
	switch {
	case errors.As(err, &le) && le.Code == local.CodeJudge:
		return r.tool(codeFound, detailJudge)
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

// closeCommand is `window close --key F --founding S --journal DIR --accord ID [--acceptances A]
// [--openings O] [--socket P]` (close-1, close-3), the acceptances as window takes them on a founded
// journal; a close decides nothing, so it needs no opening: it closes the Accord of a stopped window's journal. It rebuilds the mediator as
// a restart does, closes each open question, a timeout at its deadline where that has passed and
// closed otherwise, writes the close receipt last, and prints `close <id>` and one line per closing.
// --accord must be the founding set's Accord id: the close cannot be undone. A running window holds
// the journal's lock, so the command is journal-locked; an empty journal is tool error journal,
// unfounded; a closed one is tool error closed; a close that would not fit in one reply is tool
// error journal, receipts.jsonl, close-size, after LineCloseSize, and writes nothing. With --socket it then serves the closed Accord until
// interrupted, as `window` does; without it, it exits 0. Its --json object, written at exit, holds
// accord, close (closeObject) and closings, in both modes.
func closeCommand(r *run, args []string) int {
	fs := newFlags("window close")
	keyPath, founding := fs.text("key"), fs.text("founding")
	socket, dir := fs.text("socket"), fs.text("journal")
	accord := fs.text("accord")
	openingsDir, accPath := fs.text("openings"), fs.text("acceptances")
	if !r.parse(fs, args) || !accordID(*accord) {
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
	var openings map[string][]byte
	if *openingsDir != "" {
		if openings, err = readOpenings(*openingsDir, sealedActors(set)); err != nil {
			return r.tool(codeFile, "opening")
		}
	}
	acceptances, code := r.readAcceptances(*accPath, filepath.Join(filepath.Dir(*founding), FileAcceptances))
	if code != 0 {
		return code
	}
	operator := memberString(set, "mediator", "operator")
	err = local.Run(r.env.Context, local.Window{
		Key: k, Founding: set, Openings: openings, Acceptances: acceptances, Socket: *socket, Journal: *dir, Disk: r.env.Disk, Close: true,
		Now: r.env.Now, Stderr: r.stderr, Ready: r.env.Ready,
		Event: func(e local.Event) { r.event(e, operator) },
	})
	return r.windowError(err)
}

// reachLine is the start-up line of a judge's reach: the kinds it may be sent and to which host and
// model, that it sends nothing since no term lists it, or that it sends nothing for want of a key.
// The name, the kinds, the host and the model come from files the operator was handed, so each is
// escaped as a field.
func reachLine(x local.Reach) string {
	if len(x.Kinds) == 0 {
		return fmt.Sprintf(LineJudgeNoTerm, field(x.Judge))
	}
	if !x.Keyed {
		return fmt.Sprintf(LineJudgeNoKey, field(x.Judge), jev.KeyEnv)
	}
	kinds := make([]string, len(x.Kinds))
	for i, k := range x.Kinds {
		kinds[i] = field(k)
	}
	return fmt.Sprintf(LineJudgeSends, field(x.Judge), strings.Join(kinds, ", "), field(x.Host), field(x.Model))
}

// closeObject is the close as the --json object of window and window close holds it, in both modes:
// its id and its receipt, as an outcome is.
func closeObject(e local.Event) map[string]any {
	return map[string]any{memberReceiptID: e.Close, "receipt": json.RawMessage(e.CloseReceipt)}
}

// event prints one line per event of the window.
func (r *run) event(e local.Event, operator string) {
	switch e.Kind {
	case local.EventFounded:
		// The last member accepted its view: the founding line, as a start that founds prints it.
		fmt.Fprintln(r.stderr, "accord "+e.Accord+" "+wordFounded)
		r.put("started", wordFounded)
		return
	case local.EventStarted:
		how := wordReloaded
		switch {
		case e.Founded:
			how = wordFounded
		case e.Proposed:
			how = wordProposed
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
		for _, x := range e.Reach {
			fmt.Fprintln(r.stderr, reachLine(x))
		}
		r.put(memberAccord, e.Accord)
		r.put(memberCard, e.Card)
		r.put(memberOperator, operator)
		r.put("started", how)
		return
	case local.EventRefused:
		fmt.Fprintln(r.stderr, LineDoor+" "+string(e.Door))
		return
	case local.EventAdvised:
		// One line per advise judge, never its output (docs/adr/0023-judges.md).
		for _, a := range e.Advice {
			fmt.Fprintf(r.stderr, LineAdvised+"\n", field(a.Judge), a.Outcome, a.Receipt)
		}
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
	case e.Founding != "":
		fmt.Fprintln(r.stderr, op+" "+LineRejected+" "+memberFounding+" "+e.Founding)
	case e.Op == local.OpAccept || e.Op == local.OpView:
		fmt.Fprintln(r.stderr, op+" "+e.Caller)
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

// readAcceptances reads acceptances.json for a window (setup-contract-15): the file --acceptances
// names, or, when it is not given, the one beside the founding set, when there is one; with neither
// there are none, and on an empty journal the window starts the Accord proposed. A file that cannot
// be read is tool error file acceptances.
func (r *run) readAcceptances(flag, beside string) ([]byte, int) {
	path := flag
	if path == "" {
		if _, err := os.Stat(beside); err != nil {
			return nil, 0
		}
		path = beside
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, r.tool(codeFile, "acceptances")
	}
	return trimLF(b), 0
}

// writeAcceptances writes acceptances.json beside the founding set once every member has accepted
// (setup-contract-15): through a file of its own in the directory, synced, then renamed over it,
// mode 0644, since every party and verifier holds it.
func writeAcceptances(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+FileAcceptances+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(append(append([]byte(nil), b...), '\n'))
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil || os.Chmod(tmp, 0o644) != nil || os.Rename(tmp, path) != nil {
		_ = os.Remove(tmp)
		return errors.Join(werr, serr, cerr, errors.New("acceptances"))
	}
	return nil
}
