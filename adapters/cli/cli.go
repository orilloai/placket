// Package cli is placket's command line: one function per command, each run with its arguments, its
// standard streams and an Env, so that every command is tested in process (main-is-thin). Every
// command reads a key only from a file its flags name, writes JSON or a body on standard output and
// prose on standard error, and returns its exit code:
//
//	0  a pass, or a command that did what it was asked
//	1  a block on give, answer or change; a release that blocked; a give --wait whose question closed
//	   any way but an accept that passed and was released; a comment that did not cross; a file no
//	   release matches
//	2  a rejection, its code on standard error
//	3  a tool or door error, its code on standard error
//	4  an ask still open, the question's id on standard output
//
// What another party wrote is escaped on its way to a terminal (screen.go). With --json, every
// command prints its result as one JSON object on standard output instead (json.go).
//
// Every socket command takes --key, --socket and --accord, and --state for the actor's state
// directory, by default <key file>.state/<the Accord id's hex>/. The words a person reads are in
// words.go.
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
	"strings"
	"time"

	"github.com/orilloai/placket/adapters/export"
	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

// The exit codes.
const (
	ExitOK       = 0
	ExitBlock    = 1
	ExitRejected = 2
	ExitTool     = 3
	ExitOpen     = 4
)

// The tool errors a command prints, besides the codes of the packages it calls.
const (
	codeUsage   = "usage"
	codeBody    = "body"
	codeText    = "text"
	codeData    = "data"
	codeTerms   = "terms"
	codeFile    = "file"
	codeUntil   = "until"
	codeComment = "comment"
	codeChanged = "changed"
	codeFormat  = "format"
	codeFound   = "founding"
	codeVerify  = "verify"
	codeAccord  = string(export.CodeAccord)
	codeCard    = string(export.CodeCard)
	codeRand    = "rand"
	// detailKept follows error file when a fetched body could not be written and was kept instead.
	detailKept = "kept"
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
}

type command func(r *run, args []string) int

// commands are the commands by name; each command's file adds its own.
var commands = map[string]command{}

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
	if r.asJSON {
		r.printJSON(code)
	}
	return code
}

func (r *run) dispatch(args []string) int {
	if len(args) == 0 {
		return r.usage()
	}
	c, ok := commands[args[0]]
	if !ok {
		return r.usage()
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

// tool prints a tool error's code, and any detail, and returns its exit code.
func (r *run) tool(code string, detail ...string) int {
	r.put(memberError, code)
	if len(detail) > 0 {
		r.put(memberDetail, detail)
	}
	fmt.Fprintln(r.stderr, strings.Join(append([]string{LineError, code}, detail...), " "))
	return ExitTool
}

// failed prints what an error from a package says, as a tool error.
func (r *run) failed(err error) int {
	var ke *keyfile.Error
	var se *state.Error
	var je *journal.Error
	var le *local.Error
	switch {
	case errors.As(err, &ke):
		return r.tool(string(ke.Code))
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
	case errors.As(err, &le):
		return r.tool(string(le.Code))
	}
	return r.tool("internal")
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
	}
	return 0, false
}

type flagSet = flag.FlagSet

// many is a flag that may be given more than once.
type many []string

func (m *many) String() string     { return strings.Join(*m, ",") }
func (m *many) Set(v string) error { *m = append(*m, v); return nil }

// flagJSON is the flag every command takes to print its result as one JSON object.
const flagJSON = "json"

// flags makes a flag set that prints nothing of its own, with --json.
func flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Bool(flagJSON, false, "print the result as one JSON object")
	return fs
}

// parse parses the flags; a flag that is wrong, or a flag required and missing, is a usage error.
// --json, once read, decides whether the result is printed as JSON.
func (r *run) parse(fs *flag.FlagSet, args []string, required ...string) bool {
	err := fs.Parse(args)
	if err == nil {
		r.setJSON(fs.Lookup(flagJSON).Value.String() == "true")
	}
	if err != nil || fs.NArg() != 0 {
		fmt.Fprintln(r.stderr, "usage: placket "+fs.Name()+": bad flags")
		return false
	}
	for _, n := range required {
		if f := fs.Lookup(n); f == nil || f.Value.String() == "" {
			fmt.Fprintln(r.stderr, "usage: placket "+fs.Name()+": --"+n+" is required")
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
func line(w io.Writer, b []byte) { _, _ = w.Write(append(append([]byte(nil), b...), '\n')) }

// canonicalJSON is the canonical bytes of v.
func canonicalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	c, _ := canonical.Form(b)
	return c
}

// A session is a socket command's key, client and state directory.
type session struct {
	r      *run
	key    signing.Key
	client *local.Client
	dir    *state.Dir
}

// socketFlags adds the flags every socket command takes.
func socketFlags(fs *flag.FlagSet) (key, socket, accord, stateDir *string) {
	return fs.String("key", "", "key file"), fs.String("socket", "", "the window's socket"),
		fs.String("accord", "", "the Accord id"), fs.String("state", "", "state directory")
}

// open reads the key and opens the state directory; it prints the failure and returns its code.
func (r *run) open(keyPath, socket, accord, stateDir string) (*session, int) {
	k, err := keyfile.Read(keyPath)
	if err != nil {
		return nil, r.failed(err)
	}
	if stateDir == "" {
		stateDir = state.DefaultPath(keyPath, accord)
	}
	d, err := state.Open(stateDir, k.Reference(), r.stderr)
	if err != nil {
		return nil, r.failed(err)
	}
	return &session{r: r, key: k, dir: d, client: &local.Client{Socket: socket, Key: k, Accord: accord, Now: r.env.Now, Rand: r.env.Rand}}, 0
}

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
