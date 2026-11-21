package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// The usage every command and subcommand is to print, written down as expected values:
// testdata/help.txt holds each --help, one after another with an empty line between, and
// testdata/help.jsonl each --help --json object, one a line. mcp, which takes no --json, has no
// object.
const (
	helpText = "testdata/help.txt"
	helpJSON = "testdata/help.jsonl"
)

// subcommands are the subcommands whose usage is printed on their own: those of key and window, and
// init's examples.
var subcommands = []string{"key new", "key ref", "window close", "init approve", "init merge", "init handoff"}

// helpWant is the expected usage, by the command's words: the text --help prints, and the object
// --help --json prints, with its line feed.
func helpWant(t *testing.T) (text, object map[string]string) {
	t.Helper()
	b, err := os.ReadFile(helpText)
	if err != nil {
		t.Fatal(err)
	}
	text = map[string]string{}
	for _, block := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n\n") {
		name, ok := strings.CutPrefix(strings.SplitN(block, "\n", 2)[0], "usage: placket ")
		name, ok2 := strings.CutSuffix(name, " [flags]")
		if !ok || !ok2 || text[name] != "" {
			t.Fatalf("%s: a block that is not one command's usage: %q", helpText, block)
		}
		text[name] = block + "\n"
	}
	b, err = os.ReadFile(helpJSON)
	if err != nil {
		t.Fatal(err)
	}
	object = map[string]string{}
	for _, l := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		var o struct {
			Command string `json:"command"`
			Exit    int    `json:"exit"`
			Flags   []struct {
				Default string `json:"default"`
				Name    string `json:"name"`
				Usage   string `json:"usage"`
			} `json:"flags"`
			Subcommands []string `json:"subcommands"`
		}
		dec := json.NewDecoder(bytes.NewReader(l))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil || o.Exit != 0 || object[o.Command] != "" || !sort.SliceIsSorted(o.Flags, func(i, j int) bool { return o.Flags[i].Name < o.Flags[j].Name }) {
			t.Fatalf("%s: a line that is not one command's usage object, its flags in name order: %s (%v)", helpJSON, l, err)
		}
		object[o.Command] = string(l) + "\n"
	}
	return text, object
}

// sortedKeys is the keys of m, sorted.
func sortedKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestHelpPrintsUsage runs every command with --help, -help and -h, with no other flag, so every
// flag it requires is left out, and with --json: each prints exactly the usage written for it, its
// usage line, one line per flag with its usage and default, and its subcommands, nothing on standard
// error, exit 0, and with --json exactly its object. mcp, which takes no --json, refuses --help
// --json as it refuses --json (commands-1). The usage written covers every command and subcommand,
// and nothing else.
func TestHelpPrintsUsage(t *testing.T) {
	t.Parallel()
	text, object := helpWant(t)
	names := append(cli.Commands(), subcommands...)
	sort.Strings(names)
	if got := sortedKeys(text); !slices.Equal(got, names) {
		t.Fatalf("%s covers %v, want %v", helpText, got, names)
	}
	if got := sortedKeys(object); !slices.Equal(got, slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == "mcp" })) {
		t.Fatalf("%s covers %v, want every command but mcp", helpJSON, got)
	}
	for _, c := range cli.Commands() {
		for _, h := range []string{"--help", "-help", "-h"} {
			if r := run(t, env(), c, h); r.code != 0 || r.stderr != "" || r.stdout != text[c] {
				t.Fatalf("%s %s: %v, want standard output %q", c, h, r, text[c])
			}
		}
		if c == "mcp" {
			if r := run(t, env(), c, "--help", "--json"); r.code != 3 || r.stdout != `{"error":"usage","exit":3}`+"\n" {
				t.Fatalf("mcp --help --json: %v", r)
			}
			continue
		}
		for _, args := range [][]string{{c, "--help", "--json"}, {c, "--json", "-h"}} {
			if r := run(t, env(), args...); r.code != 0 || r.stderr != "" || r.stdout != object[c] {
				t.Fatalf("%v: %v, want standard output %q", args, r, object[c])
			}
		}
	}
}

// TestHelpOfASubcommand prints the usage of key new, key ref, window close and each init example,
// each exactly the usage written for it, with its own flags, and with --json its object.
func TestHelpOfASubcommand(t *testing.T) {
	t.Parallel()
	text, object := helpWant(t)
	for _, name := range subcommands {
		words := strings.Fields(name)
		if r := run(t, env(), append(words, "--help")...); r.code != 0 || r.stderr != "" || r.stdout != text[name] {
			t.Fatalf("%s --help: %v, want standard output %q", name, r, text[name])
		}
		if r := run(t, env(), append(words, "-h", "--json")...); r.code != 0 || r.stderr != "" || r.stdout != object[name] {
			t.Fatalf("%s -h --json: %v, want standard output %q", name, r, object[name])
		}
	}
}

// TestHelpReadsNothing runs commands with --help beside flags naming files and sockets that do not
// exist: each prints exactly its usage and exits 0, reading none of them.
func TestHelpReadsNothing(t *testing.T) {
	t.Parallel()
	text, _ := helpWant(t)
	for _, args := range [][]string{
		{"verify", "--receipts", "/nonexistent/j", "--help"},
		{"get", "--key", "/nonexistent/k", "--socket", "/nonexistent/s", "--accord", "x", "--help"},
		{"window", "--key", "/nonexistent/k", "-h"},
	} {
		if r := run(t, env(), args...); r.code != 0 || r.stderr != "" || r.stdout != text[args[0]] {
			t.Fatalf("%v: %v, want standard output %q", args, r, text[args[0]])
		}
	}
}

// TestHelpOfPlacket lists the commands with placket --help, -help and -h, exit 0, and with --json
// in one object; placket alone and an unknown command stay error usage, exit 3; and --help given as
// a flag's value is that value.
func TestHelpOfPlacket(t *testing.T) {
	t.Parallel()
	commands := []string{"accept", "answer", "bind", "cancel", "card", "change", "declare", "decline", "founding", "get", "give",
		"init", "key", "log", "mcp", "mediators", "questions", "reconcile", "released", "result", "seal", "show", "verify",
		"version", "view", "watch", "window"}
	want := []string{"usage: placket <command> [flags]"}
	for _, c := range commands {
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
	if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || r.code != 0 || r.stderr != "" || !slices.Equal(o.Commands, commands) ||
		r.stdout != `{"commands":["`+strings.Join(commands, `","`)+`"],"exit":0}`+"\n" {
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
