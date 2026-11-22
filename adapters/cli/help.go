package cli

import (
	"fmt"
	"sort"
	"strings"
)

// --help, -help and -h, given as a flag and not as a flag's value, ask for a command's usage, which
// it prints on standard output, exiting 0, before it reads any file, socket or state, and without
// checking the flags it requires (commands-1, docs/adr/0029-help-and-answers-the-window-does-not-show.md).
// The usage is the line `usage: placket <command> [flags]`, one line per flag of the command's row
// of the command table, in name order, each its flag and value as the row writes them, its sentence
// and any default, and one line per subcommand, `placket <subcommand> [flags]`: the rows of the
// table one word longer, and the values of the row's first argument, such as init's examples. With
// --json it is one object instead: command, flags, each its name, sentence and default, and
// subcommands when there are any. mcp, which takes no --json, refuses --help with --json as it
// refuses --json. placket --help lists the commands.

// The members of the usage object.
const (
	memberCommand     = "command"
	memberCommands    = "commands"
	memberFlags       = "flags"
	memberSubcommands = "subcommands"
)

// helpFlag reports whether a is a request for usage, as the flag package reads one.
func helpFlag(a string) bool { return a == "-h" || a == "-help" || a == "--help" }

// help prints the usage of the command whose flag set is fs, from its row, and reports whether it
// did. A command that takes no --json, given --json, prints nothing here: its flags cannot be read.
func (r *run) help(fs *flagSet) bool {
	if r.asJSON && fs.Lookup(flagJSON) == nil {
		return false
	}
	r.helped = true
	var row Row
	if fs.row != nil {
		row = *fs.row
	}
	rowFlags := append([]Flag(nil), row.Flags...)
	sort.Slice(rowFlags, func(i, j int) bool { return rowFlags[i].Name < rowFlags[j].Name })
	lines := make([]string, 0, len(rowFlags))
	list := make([]any, 0, len(rowFlags))
	for _, f := range rowFlags {
		def := f.Default
		if g := fs.Lookup(f.Name); g != nil {
			def = g.DefValue
		}
		l := f.String() + "  " + f.Sentence
		if def != "" && def != "false" && def != "0" {
			l += " (default " + def + ")"
		}
		lines = append(lines, l)
		list = append(list, map[string]any{"name": f.Name, "usage": f.Sentence, "default": def})
	}
	var subs []string
	if strings.Join(row.Words, " ") == fs.Name() {
		subs = subcommandsOf(row)
	}
	r.put(memberCommand, fs.Name())
	r.put(memberFlags, list)
	if len(subs) > 0 {
		r.put(memberSubcommands, subs)
	}
	if r.asJSON {
		return true
	}
	fmt.Fprintln(r.realOut, "usage: placket "+fs.Name()+" [flags]")
	for _, l := range lines {
		fmt.Fprintln(r.realOut, l)
	}
	for _, s := range subs {
		fmt.Fprintln(r.realOut, "placket "+s+" [flags]")
	}
	return true
}

// subcommandsOf are the subcommands a row's help names: each value of its first argument, such as
// key's new and ref or init's examples, then each row of the table one word longer, such as window
// close, once each and in that order.
func subcommandsOf(row Row) []string {
	name := strings.Join(row.Words, " ")
	var out []string
	add := func(s string) {
		if !contains(out, s) {
			out = append(out, s)
		}
	}
	if len(row.Args) > 0 {
		for _, v := range row.Args[0].Values {
			add(name + " " + v)
		}
	}
	for _, t := range table {
		if len(t.Words) == len(row.Words)+1 && strings.Join(t.Words[:len(row.Words)], " ") == name {
			add(strings.Join(t.Words, " "))
		}
	}
	return out
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
