package cli_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// TestHelpComesFromTheRow runs every command and subcommand of the command table with --help: after
// the usage line, each line is a flag of its row, in name order, written as the row writes it, with
// its sentence and its default, so that the help a person reads and docs/reference/cli.md say the
// same. key new prints --out <file> and its sentence.
func TestHelpComesFromTheRow(t *testing.T) {
	t.Parallel()
	for _, row := range cli.Table() {
		name := strings.Join(row.Words, " ")
		flags := append([]cli.Flag(nil), row.Flags...)
		sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
		want := []string{"usage: placket " + name + " [flags]"}
		for _, f := range flags {
			l := f.String() + "  " + f.Sentence
			if d := defaultOf(f); d != "" && d != "false" && d != "0" {
				l += " (default " + d + ")"
			}
			want = append(want, l)
		}
		r := run(t, env(), append(append([]string(nil), row.Words...), "--help")...)
		got := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
		if r.code != 0 || len(got) < len(want) {
			t.Fatalf("%s --help: %v", name, r)
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("%s --help line %d is %q, and its row gives %q", name, i+1, got[i], w)
			}
		}
	}
	r := run(t, env(), "key", "new", "--help")
	if !strings.Contains(r.stdout, "\n--out <file>  The key file to write; a file already there is never replaced.\n") {
		t.Fatalf("key new --help does not print its row's --out: %v", r)
	}
}
