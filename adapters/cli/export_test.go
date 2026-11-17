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

// Flags are the names of the flags the command words name takes, sorted: the command runs with a
// flag no command has, which it refuses as it reads its flags, before it does anything else, and
// the flag set it read is listed.
func Flags(words ...string) []string {
	r := &run{stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard, realOut: io.Discard, rep: newReport()}
	r.dispatch(append(append([]string(nil), words...), "--placket-no-such-flag"))
	if r.fs == nil {
		return nil
	}
	var names []string
	r.fs.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	return names
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
