// Package cli is placket's command line: one function per command, each run with its arguments, its
// standard streams and an Env, so that every command is tested in process (main-is-thin). Every
// command reads a key only from a file its flags name, writes JSON or a body on standard output and
// prose on standard error, and returns its exit code:
//
//	0  a pass, or a command that did what it was asked
//	1  a block on give, answer or change; a release that blocked; a give --wait whose question closed
//	   any way but an accept that passed and was released; a comment that did not cross; a file, or a
//	   value inside it, no release matches
//	2  a rejection, its code on standard error
//	3  a tool or door error, its code on standard error
//	4  an ask still open, the question's id on standard output
//
// What another party wrote is escaped on its way to a terminal (screen.go). With --json, every
// command prints its result as one JSON object on standard output instead (json.go).
//
// Every socket command takes --key, --socket and --accord, or --label, naming an entry of the labels
// config (client.ReadConfig) in their place, and --state for the actor's state directory, by default
// <key file>.state/<the Accord id's hex>/. Each calls the mediator through adapters/client. The words
// a person reads are in words.go. Each command reads its flags from its row of the command table
// (table.go), from which docs/reference/cli.md is generated.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/internal/lines"
	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
)

// The exit codes.
const (
	ExitOK       = 0
	ExitBlock    = 1
	ExitRejected = 2
	ExitTool     = 3
	ExitOpen     = 4
)

// code is the type of the command line's own tool errors, whose sentences adapters/errtext holds
// under its key adapters/cli. It is an alias, so a code passes as the string its error line prints.
// A detail word that follows a code is untyped, and so is a code another package declares.
type code = string

// The tool errors a command prints, besides the codes of the packages it calls.
const (
	codeUsage   code = "usage"
	codeBody    code = "body"
	codeText    code = "text"
	codeData    code = "data"
	codeTerms   code = "terms"
	codeFile    code = "file"
	codeUntil   code = "until"
	codeComment code = "comment"
	codeChanged code = "changed"
	codeFormat  code = "format"
	codeFound   code = "founding"
	codeVerify  code = "verify"
	// codeClosed is a close of a journal already closed (close-3), the mediator's code;
	// detailSuperseded, under verify, a successor whose supersedes does not pin this log's close
	// (close-4).
	codeClosed       = string(mediator.CodeClosed)
	detailSuperseded = "superseded"
	// codeDelivery: verify's --delivery is not a delivery, or its receipt is none of the receipts.
	codeDelivery code = "delivery"
	// detailAccord and detailCard follow error verify for a journal of more than one Accord or card.
	detailAccord = string(export.CodeAccord)
	detailCard   = string(export.CodeCard)
	// codeCard: card could not build a card with the judges given.
	codeCard code = string(export.CodeCard)
	codePath code = "path"
	// codeInternal: a failure no other code names.
	codeInternal code = "internal"
	// detailKept follows error file when a fetched body could not be written and was kept instead.
	detailKept = "kept"
	// detailKernel follows error founding when a window's card names another module than this
	// build's.
	detailKernel = "kernel"
	// detailLimit follows error socket-path, with the longest socket path the system takes.
	detailLimit = "limit"
	// codeJudge: a --judge file is not a judge's config, with ReadConfig's code after it; and,
	// following error founding, the window's judge configs are not the ones its card lists.
	codeJudge   code = "judge"
	detailJudge      = string(codeJudge)
)

// Env is what a command takes from the world.
type Env struct {
	// Now is the clock, and Rand the source of salts, nonces, message ids and seeds.
	Now  func() time.Time
	Rand io.Reader
	// Context is done when the command is interrupted (SIGINT or SIGTERM).
	Context context.Context
	// Poll is how often give --wait asks for its result; zero is a second.
	Poll time.Duration
	// Disk is what a window's journal writes through; nil is the operating system's.
	Disk journal.Disk
	// Ready, when set, is closed once a window listens.
	Ready chan<- struct{}
	// Terminal reports whether a writer is a terminal, so that another party's bytes are escaped on
	// it; nil takes a character device for one.
	Terminal func(w io.Writer) bool
	// Getenv reads the environment: the judges' key, once when a window starts
	// (docs/adr/0009-who-decided.md), and the directories of the labels config and of a window's default
	// socket and journal (HOME and the XDG_ variables, TMPDIR); nil is an empty environment. The
	// command line never reads the environment another way.
	Getenv func(string) string
}

// A run is one command's streams and Env.
type run struct {
	env            Env
	stdin          io.Reader
	stdout, stderr io.Writer
	// realOut is standard output, which stdout is unless --json was given; rep is the object
	// printed with --json.
	realOut io.Writer
	asJSON  bool
	rep     *report
	// fs is the flag set the command read, so that what a command takes can be listed.
	fs *flag.FlagSet
	// helped is set once --help printed the usage it asked for (help.go).
	helped bool
	// printed is set once the object is printed, by Run, or by a command that must know whether it
	// was written, as get does.
	printed bool
}

type command func(r *run, args []string) int

// commands are the commands by name; each command's file adds its own.
var commands = map[string]command{}

// subcommands are the subcommands by name, the command's word and the subcommand's, such as key new;
// each command's file adds its own.
var subcommands = map[string]command{}

// Run runs the command args name, with its flags, and returns its exit code.
func Run(env Env, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if env.Context == nil {
		env.Context = context.Background()
	}
	if env.Poll == 0 {
		env.Poll = time.Second
	}
	if env.Terminal == nil {
		env.Terminal = isTerminal
	}
	r := &run{env: env, stdin: stdin, stdout: stdout, stderr: stderr, realOut: stdout, rep: newReport()}
	r.setJSON(wantsJSON(args))
	code := r.dispatch(args)
	if r.asJSON && !r.printed {
		r.printJSON(code)
	}
	return code
}

func (r *run) dispatch(args []string) int {
	if len(args) == 0 {
		return r.usage()
	}
	if helpFlag(args[0]) {
		return r.commandList()
	}
	c, ok := commands[args[0]]
	if !ok {
		return r.usage()
	}
	if len(args) > 1 {
		if sub, ok := subcommands[args[0]+" "+args[1]]; ok {
			return sub(r, args[2:])
		}
	}
	return c(r, args[1:])
}

func (r *run) usage() int {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintln(r.stderr, "usage: placket <command> [flags]; commands: "+strings.Join(names, ", "))
	return r.tool(codeUsage)
}

// tool prints a tool error's code, and any detail, and returns its exit code. Once --help printed
// the usage it asked for, parse has returned false and the command stops through tool, which then
// prints nothing and returns 0.
func (r *run) tool(code string, detail ...string) int {
	if r.helped {
		return ExitOK
	}
	r.put(memberError, code)
	if len(detail) > 0 {
		r.put(memberDetail, detail)
	}
	fmt.Fprintln(r.stderr, strings.Join(append([]string{LineError, code}, detail...), " "))
	return ExitTool
}

// failed prints what an error from a package says, as a tool error. A journal or state file it was
// handed that is not a regular file is error file, naming the file.
func (r *run) failed(err error) int {
	var ke *keyfile.Error
	var se *state.Error
	var je *journal.Error
	var le *local.Error
	var ce *client.Error
	switch {
	case errors.As(err, &ce):
		d := make([]string, len(ce.Entries))
		for i, n := range ce.Entries {
			d[i] = strconv.Itoa(n)
		}
		return r.tool(string(ce.Code), d...)
	case errors.Is(err, lines.ErrNotRegular) && errors.As(err, &se):
		return r.tool(codeFile, se.File)
	case errors.Is(err, lines.ErrNotRegular) && errors.As(err, &je):
		return r.tool(codeFile, je.File)
	case errors.As(err, &ke):
		return r.tool(string(ke.Code))
	case errors.As(err, &se) && se.Detail != "":
		// A detail, such as too-large, says what is wrong with the file beyond the code.
		return r.tool(string(se.Code), se.File, se.Detail)
	case errors.As(err, &se):
		return r.tool(string(se.Code), se.File)
	case errors.As(err, &je):
		d := []string{}
		if je.File != "" {
			d = append(d, je.File)
		}
		if je.Line > 0 {
			d = append(d, fmt.Sprintf("line %d", je.Line))
		}
		if je.Core != "" {
			d = append(d, je.Core)
		}
		return r.tool(string(je.Code), d...)
	case errors.As(err, &le) && le.Code == local.CodeSocketPath:
		return r.tool(string(le.Code), detailLimit, strconv.Itoa(local.MaxSocketPath))
	case errors.As(err, &le):
		return r.tool(string(le.Code))
	}
	return r.tool(codeInternal)
}

// refusal prints a reply that is a refusal, and returns its exit code; ok is false for any other.
func (r *run) refusal(rep local.Reply) (int, bool) {
	switch {
	case rep.Rejection != "":
		r.put(memberRejection, string(rep.Rejection))
		fmt.Fprintln(r.stderr, LineRejected+" "+string(rep.Rejection))
		return ExitRejected, true
	case rep.Error != "":
		return r.tool(string(rep.Error)), true
	case rep.Door != "":
		r.put(memberDoor, string(rep.Door))
		fmt.Fprintln(r.stderr, LineDoor+" "+string(rep.Door))
		return ExitTool, true
	case rep.Founding != "":
		// An acceptance the window refused: the founding row it fails (setup-contract-15).
		r.put(memberFounding, rep.Founding)
		fmt.Fprintln(r.stderr, LineRejected+" "+memberFounding+" "+rep.Founding)
		return ExitRejected, true
	}
	return 0, false
}

// many is a flag that may be given more than once.
type many []string

func (m *many) String() string     { return strings.Join(*m, ",") }
func (m *many) Set(v string) error { *m = append(*m, v); return nil }

// flagJSON is the flag every command takes to print its result as one JSON object.
const flagJSON = "json"

// parse parses the flags; a flag that is wrong, or a flag its row requires and missing, is a usage
// error. --json, once read, decides whether the result is printed as JSON. --help, -help or -h, as a
// flag and not a flag's value, prints the command's usage from its row instead (help) and checks no
// flag required.
func (r *run) parse(fs *flagSet, args []string) bool {
	return r.parseFlags(fs, args) && r.required(fs, fs.required()...)
}

// parseFlags parses the flags as parse does, without looking for the flags required. After --help
// it prints the usage and returns false, and the command's usage error then prints nothing (tool).
func (r *run) parseFlags(fs *flagSet, args []string) bool {
	r.fs = fs.FlagSet
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) && r.help(fs) {
		return false
	}
	if f := fs.Lookup(flagJSON); err == nil && f != nil {
		r.setJSON(f.Value.String() == "true")
	}
	if err != nil || fs.NArg() != 0 {
		fmt.Fprintln(r.stderr, "usage: placket "+fs.Name()+": bad flags")
		return false
	}
	return true
}

// required reports whether every flag required was given, and prints the first that was not.
func (r *run) required(fs *flagSet, required ...string) bool {
	for _, n := range required {
		if f := fs.Lookup(n); f == nil || f.Value.String() == "" {
			fmt.Fprintln(r.stderr, "usage: placket "+fs.Name()+": --"+n+" is required")
			return false
		}
	}
	return true
}

// accordID reports whether s is an Accord id: a typed hash, the prefix and 64 lowercase hex digits.
func accordID(s string) bool {
	h, ok := strings.CutPrefix(s, canonical.HashPrefix)
	if !ok || len(h) != 64 {
		return false
	}
	for i := 0; i < len(h); i++ {
		if c := h[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// readJSON reads a file that holds one JSON value, as canonical bytes.
func readJSON(path string) ([]byte, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	c, err := canonical.Form(b)
	if err != nil {
		return nil, false
	}
	return c, true
}

// line writes b and a line feed.
func line(w io.Writer, b []byte) { writeLine(w, b) }

// writeLine writes b and a line feed to w, and reports whether it was written.
func writeLine(w io.Writer, b []byte) bool {
	_, err := w.Write(append(append([]byte(nil), b...), '\n'))
	return err == nil
}

// canonicalJSON is the canonical bytes of v.
func canonicalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	c, _ := canonical.Form(b)
	return c
}

// A session is a socket command's client of one mediator.
type session struct {
	r *run
	c *client.Client
	// handed is the close receipt a fetch was handed in place of a delivery, once the Accord is
	// closed and the queue is empty (close-3). Only get keeps it.
	handed []byte
}

// socketOpts are the flags every socket command takes: --key, --socket and --accord, or --label in
// their place, and --state.
type socketOpts struct{ key, socket, accord, state, label *string }

// socketFlags adds the flags every socket command takes.
func socketFlags(fs *flagSet) *socketOpts {
	return &socketOpts{key: fs.text("key"), socket: fs.text("socket"), accord: fs.text("accord"),
		state: fs.text("state"), label: fs.text("label")}
}

// parseSocket parses a socket command's flags, as parse does: without --label, --key, --socket and
// --accord are required, ahead of the others its row requires; with it, none of the three may be
// given.
func (r *run) parseSocket(fs *flagSet, args []string, so *socketOpts) bool {
	if !r.parseFlags(fs, args) {
		return false
	}
	required := fs.required()
	if *so.label == "" {
		return r.required(fs, append([]string{"key", "socket", "accord"}, required...)...)
	}
	if *so.key != "" || *so.socket != "" || *so.accord != "" {
		fmt.Fprintln(r.stderr, "usage: placket "+fs.Name()+": --label takes the place of --key, --socket and --accord")
		return false
	}
	return r.required(fs, required...)
}

// open opens the session's client; it prints the failure and returns its code. With --label it
// reads the labels config and names its entry, by label or Accord id; a config that is refused, or
// a name no entry has, is its client error. Otherwise an accord that is not an Accord id, a typed
// hash, is error usage before any path is built from it; then a socket path too long is refused,
// error socket-path limit N, before anything is read. --state overrides the state directory.
func (r *run) open(so *socketOpts) (*session, int) {
	var m client.Mediator
	if *so.label != "" {
		cfg, err := r.config()
		if err != nil {
			return nil, r.failed(err)
		}
		if m, err = cfg.Find(*so.label); err != nil {
			return nil, r.failed(err)
		}
	} else {
		if !accordID(*so.accord) {
			fmt.Fprintln(r.stderr, "usage: placket: --accord is not an Accord id")
			return nil, r.tool(codeUsage)
		}
		m = client.Mediator{Accord: *so.accord, Key: *so.key, Socket: *so.socket}
	}
	m.State = *so.state
	c, err := client.Open(m, client.Options{Now: r.env.Now, Rand: r.env.Rand, Stderr: r.stderr})
	if err != nil {
		return nil, r.failed(err)
	}
	return &session{r: r, c: c}, 0
}

// getenv reads the environment as Env.Getenv does, nil being an empty one.
func (r *run) getenv(name string) string {
	if r.env.Getenv == nil {
		return ""
	}
	return r.env.Getenv(name)
}

// config reads the labels config.
func (r *run) config() (client.Config, error) { return client.ReadConfig(r.getenv) }

// outcome prints an outcome's line, <word> <reason> <receipt id>, with released for a release that
// passed, and the receipt on standard output, escaped on a terminal; and puts it under name.
// ParseReply has read the reason and the id for their form.
func (r *run) outcome(o *mediator.Outcome, release bool, name string) {
	w := word(o.Verdict)
	if release && w == WordPass {
		w = WordReleased
	}
	fmt.Fprintln(r.stderr, w+" "+o.Reason+" "+o.ReceiptID)
	line(r.stdout, r.screenJSON(o.Receipt))
	r.put(name, outcomeObject(o, w))
}
