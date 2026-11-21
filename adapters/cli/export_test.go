package cli

import (
	"bytes"
	"flag"
	"io"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/orilloai/placket/core/composition"
)

// Commands are the names of the commands, sorted.
func Commands() []string {
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Subcommands are the names of the subcommands, such as key new, sorted.
func Subcommands() []string {
	names := make([]string, 0, len(subcommands))
	for n := range subcommands {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// A ReadFlag is a flag as a command read it: its kind and its default, as the flag package prints it.
type ReadFlag struct {
	Kind    Kind
	Default string
}

// ReadFlags are the flags the command words name reads, as Flags runs it, by name.
func ReadFlags(words ...string) map[string]ReadFlag {
	fs := readFlagSet(words)
	if fs == nil {
		return nil
	}
	out := map[string]ReadFlag{}
	fs.VisitAll(func(f *flag.Flag) {
		k := KindText
		if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
			k = KindSwitch
		} else if _, ok := f.Value.(*many); ok {
			k = KindList
		} else if g, ok := f.Value.(flag.Getter); ok {
			if _, ok := g.Get().(int); ok {
				k = KindNumber
			}
		}
		out[f.Name] = ReadFlag{Kind: k, Default: f.DefValue}
	})
	return out
}

// Flags are the names of the flags the command words name takes, sorted: the command runs with a
// flag no command has, which it refuses as it reads its flags, before it does anything else, and
// the flag set it read is listed.
func Flags(words ...string) []string {
	fs := readFlagSet(words)
	if fs == nil {
		return nil
	}
	var names []string
	fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	return names
}

// readFlagSet is the flag set the command words name read, run as Flags says.
func readFlagSet(words []string) *flag.FlagSet {
	r := &run{stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard, realOut: io.Discard, rep: newReport()}
	r.dispatch(append(append([]string(nil), words...), "--placket-no-such-flag"))
	return r.fs
}

// Version is what version prints on standard output for the build info bi, which may be nil, with
// or without --json.
func Version(bi *debug.BuildInfo, asJSON bool) string {
	var out bytes.Buffer
	r := &run{env: Env{Terminal: isTerminal}, stdin: strings.NewReader(""), stdout: &out, stderr: io.Discard, realOut: &out, rep: newReport()}
	r.setJSON(asJSON)
	r.version(bi, composition.KernelVersion, composition.KernelHash())
	if asJSON {
		r.printJSON(ExitOK)
	}
	return out.String()
}

// SetRelocking sets the hook get calls before it takes the state lock again for a delivery whose
// record failed after a wait.
func SetRelocking(f func()) { relocking = f }
