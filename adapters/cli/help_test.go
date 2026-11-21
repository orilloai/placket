package cli_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// helpObject is what --help --json prints.
type helpObject struct {
	Command string `json:"command"`
	Exit    int    `json:"exit"`
	Flags   []struct {
		Default *string `json:"default"`
		Name    string  `json:"name"`
		Usage   *string `json:"usage"`
	} `json:"flags"`
	Subcommands []string `json:"subcommands"`
}

// subcommands are the commands whose help names more than their flags: the subcommands of key and
// window, and init's examples.
var subcommands = map[string][]string{
	"key":    {"key new", "key ref"},
	"window": {"window close"},
	"init":   {"init approve", "init merge", "init handoff"},
}

// flagsOf is the names of the flags the command words name reads, as the command line lists them.
func flagsOf(words ...string) []string {
	if len(words) == 1 && words[0] == "init" {
		return cli.Flags("init", "approve")
	}
	return cli.Flags(words...)
}

// checkHelp checks what a command's --help printed without --json: its usage line, one line per
// flag it reads, each starting with the flag's name, and one line per subcommand; nothing on
// standard error, exit 0.
func checkHelp(t *testing.T, r result, name string, flags, subs []string) {
	t.Helper()
	l := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	if r.code != 0 || r.stderr != "" || len(l) != 1+len(flags)+len(subs) || l[0] != "usage: placket "+name+" [flags]" {
		t.Fatalf("%s --help: %v", name, r)
	}
	for i, f := range flags {
		if !strings.HasPrefix(l[1+i], "--"+f+" ") {
			t.Fatalf("%s --help line %d is %q, want the flag --%s", name, 2+i, l[1+i], f)
		}
	}
	for i, s := range subs {
		if got := l[1+len(flags)+i]; got != "placket "+s+" [flags]" {
			t.Fatalf("%s --help line %d is %q, want the subcommand %s", name, 2+len(flags)+i, got, s)
		}
	}
}

// checkHelpJSON checks what a command's --help --json printed: one object of the command, exit 0,
// its flags in name order, each with its default and usage, and its subcommands when it has any.
func checkHelpJSON(t *testing.T, r result, name string, flags, subs []string) {
	t.Helper()
	var o helpObject
	if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || r.code != 0 || r.stderr != "" || o.Command != name || o.Exit != 0 ||
		len(o.Flags) != len(flags) || !slices.Equal(o.Subcommands, subs) || strings.Count(r.stdout, "\n") != 1 {
		t.Fatalf("%s --help --json: %v", name, r)
	}
	for i, f := range o.Flags {
		if f.Name != flags[i] || f.Default == nil || f.Usage == nil || *f.Usage == "" {
			t.Fatalf("%s --help --json flag %d: %+v, want --%s", name, i, f, flags[i])
		}
	}
}

// TestHelpPrintsUsage runs every command with --help, -help and -h, with no other flag, so every
// flag it requires is left out, and with --json: each prints its usage on standard output, its
// flags one a line, and exits 0, and with --json prints one object of its flags. key and window name
// their subcommands, init its examples. mcp, which takes no --json, refuses --help --json as it
// refuses --json (commands-1).
func TestHelpPrintsUsage(t *testing.T) {
	t.Parallel()
	for _, c := range cli.Commands() {
		flags, subs := flagsOf(c), subcommands[c]
		if len(flags) == 0 {
			t.Fatalf("%s lists no flags", c)
		}
		for _, h := range []string{"--help", "-help", "-h"} {
			checkHelp(t, run(t, env(), c, h), c, flags, subs)
		}
		if c == "mcp" {
			if r := run(t, env(), c, "--help", "--json"); r.code != 3 || r.stdout != `{"error":"usage","exit":3}`+"\n" {
				t.Fatalf("mcp --help --json: %v", r)
			}
			continue
		}
		checkHelpJSON(t, run(t, env(), c, "--help", "--json"), c, flags, subs)
		checkHelpJSON(t, run(t, env(), c, "--json", "-h"), c, flags, subs)
	}
}

// TestHelpOfASubcommand prints the usage of key new, key ref, window close and init approve, each
// with its own flags.
func TestHelpOfASubcommand(t *testing.T) {
	t.Parallel()
	for _, words := range [][]string{{"key", "new"}, {"key", "ref"}, {"window", "close"}, {"init", "approve"}} {
		flags := cli.Flags(words...)
		name := strings.Join(words, " ")
		checkHelp(t, run(t, env(), append(words, "--help")...), name, flags, nil)
		checkHelpJSON(t, run(t, env(), append(words, "-h", "--json")...), name, flags, nil)
	}
}

// TestHelpReadsNothing runs commands with --help beside flags naming files and sockets that do not
// exist: each prints its usage and exits 0, reading none of them.
func TestHelpReadsNothing(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"verify", "--receipts", "/nonexistent/j", "--help"},
		{"get", "--key", "/nonexistent/k", "--socket", "/nonexistent/s", "--accord", "x", "--help"},
		{"window", "--key", "/nonexistent/k", "-h"},
	} {
		checkHelp(t, run(t, env(), args...), args[0], flagsOf(args[0]), subcommands[args[0]])
	}
}

// TestHelpOfPlacket lists the commands with placket --help, -help and -h, exit 0, and with --json
// in one object; placket alone and an unknown command stay error usage, exit 3; and --help given as
// a flag's value is that value.
func TestHelpOfPlacket(t *testing.T) {
	t.Parallel()
	want := []string{"usage: placket <command> [flags]"}
	for _, c := range cli.Commands() {
		want = append(want, "placket "+c+" [flags]")
	}
	for _, h := range []string{"--help", "-help", "-h"} {
		r := run(t, env(), h)
		if r.code != 0 || r.stderr != "" || r.stdout != strings.Join(want, "\n")+"\n" {
			t.Fatalf("placket %s: %v", h, r)
		}
	}
	r := run(t, env(), "--help", "--json")
	var o struct {
		Commands []string `json:"commands"`
		Exit     int      `json:"exit"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || r.code != 0 || r.stderr != "" || !slices.Equal(o.Commands, cli.Commands()) {
		t.Fatalf("placket --help --json: %v", r)
	}
	for _, args := range [][]string{nil, {"no-such-command", "--help"}} {
		if r := run(t, env(), args...); r.code != 3 || r.stdout != "" || !strings.HasSuffix(r.stderr, "\nerror usage\n") {
			t.Fatalf("placket %v: %v", args, r)
		}
	}
	if r := run(t, env(), "give", "--text", "--help"); r.code != 3 || r.stdout != "" || r.stderr != "usage: placket give: --key is required\nerror usage\n" {
		t.Fatalf("give --text --help: %v", r)
	}
}

// TestInitHelpGivesTheExampleAsTheLabel: init <example> --help names the example as --label's
// default, the label init takes without --label, in its line and, with --json, as the flag's
// default; init --help, before any example, gives none.
func TestInitHelpGivesTheExampleAsTheLabel(t *testing.T) {
	t.Parallel()
	labelOf := func(r result) *string {
		var o helpObject
		if err := json.Unmarshal([]byte(r.stdout), &o); err != nil {
			t.Fatalf("%v: %v", r, err)
		}
		for _, f := range o.Flags {
			if f.Name == "label" {
				return f.Default
			}
		}
		return nil
	}
	for _, x := range []string{"approve", "merge", "handoff"} {
		if d := labelOf(run(t, env(), "init", x, "--help", "--json")); d == nil || *d != x {
			t.Errorf("init %s --help --json gives --label the default %v, want %q", x, d, x)
		}
		r := run(t, env(), "init", x, "--help")
		i := strings.Index(r.stdout, "\n--label <label>  ")
		if i < 0 || !strings.HasSuffix(strings.SplitN(r.stdout[i+1:], "\n", 2)[0], " (default "+x+")") {
			t.Errorf("init %s --help does not give --label the default %s: %v", x, x, r)
		}
	}
	if d := labelOf(run(t, env(), "init", "--help", "--json")); d == nil || *d != "" {
		t.Errorf("init --help --json gives --label the default %v, want none", d)
	}
}
