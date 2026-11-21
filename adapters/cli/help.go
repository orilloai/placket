package cli

import (
	"flag"
	"fmt"
	"sort"
	"strings"
)

// --help, -help and -h, given as a flag and not as a flag's value, ask for a command's usage, which
// it prints on standard output, exiting 0, before it reads any file, socket or state, and without
// checking the flags it requires (commands-1, docs/adr/0029-help-and-answers-the-window-does-not-show.md).
// The usage is the line `usage: placket <command> [flags]`, one line per flag the command reads, in
// name order, each starting with the flag, and one line per subcommand, `placket <subcommand>
// [flags]`. With --json it is one object instead: command, flags, each its name, usage and default
// as the flag set holds them, and subcommands when there are any. mcp, which takes no --json,
// refuses --help with --json as it refuses --json. placket --help lists the commands.

// The members of the usage object.
const (
	memberCommand     = "command"
	memberCommands    = "commands"
	memberFlags       = "flags"
	memberSubcommands = "subcommands"
)

// helpFlag reports whether a is a request for usage, as the flag package reads one.
func helpFlag(a string) bool { return a == "-h" || a == "-help" || a == "--help" }

// help prints the usage of the command whose flag set is fs, and reports whether it did. A command
// that takes no --json, given --json, prints nothing here: its flags cannot be read.
func (r *run) help(fs *flag.FlagSet) bool {
	if r.asJSON && fs.Lookup(flagJSON) == nil {
		return false
	}
	r.helped = true
	var lines []string
	var list []any
	fs.VisitAll(func(f *flag.Flag) {
		kind, usage := flag.UnquoteUsage(f)
		l := "--" + f.Name
		if kind != "" {
			l += " " + kind
		}
		l += "  " + usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			l += " (default " + f.DefValue + ")"
		}
		lines = append(lines, l)
		list = append(list, map[string]any{"name": f.Name, "usage": f.Usage, "default": f.DefValue})
	})
	r.put(memberCommand, fs.Name())
	r.put(memberFlags, append([]any{}, list...))
	if len(r.subs) > 0 {
		r.put(memberSubcommands, r.subs)
	}
	if r.asJSON {
		return true
	}
	fmt.Fprintln(r.realOut, "usage: placket "+fs.Name()+" [flags]")
	for _, l := range lines {
		fmt.Fprintln(r.realOut, l)
	}
	for _, s := range r.subs {
		fmt.Fprintln(r.realOut, "placket "+s+" [flags]")
	}
	return true
}

// commandList is placket --help: the usage line and one line per command, or with --json the
// commands in one object; exit 0.
func (r *run) commandList() int {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	r.put(memberCommands, names)
	if !r.asJSON {
		lines := []string{"usage: placket <command> [flags]"}
		for _, n := range names {
			lines = append(lines, "placket "+n+" [flags]")
		}
		fmt.Fprintln(r.realOut, strings.Join(lines, "\n"))
	}
	return ExitOK
}
