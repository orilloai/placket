package cli

import (
	"flag"
	"io"
	"sort"
	"strings"
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
