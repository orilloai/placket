package cli_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// usageFlagRE is a flag as a row's usage writes it.
var usageFlagRE = regexp.MustCompile(`--([a-z][a-z-]*)`)

// specIDRE is a requirement id a row's --json object follows.
var specIDRE = regexp.MustCompile(`^commands-[1-8]$`)

// socketFlagNames are the flags every socket command takes, which a usage writes as <socket flags>.
var socketFlagNames = []string{"accord", "key", "label", "socket", "state"}

// TestTheCommandTable holds the command table to the commands. Every command and subcommand has a
// row, and every row names one. Each command reads exactly the flags its row lists, each of the kind
// and with the default its row gives, so a flag added to a command with no row fails here, naming
// both. No summary, usage or sentence is empty, the usage names exactly the row's flags, and the
// --json object of a command that takes --json cites the requirements it follows.
func TestTheCommandTable(t *testing.T) {
	t.Parallel()
	rows := map[string]cli.Row{}
	for _, row := range cli.Table() {
		name := strings.Join(row.Words, " ")
		if _, ok := rows[name]; ok {
			t.Errorf("%s has two rows", name)
		}
		rows[name] = row
	}
	commands := map[string]bool{}
	for _, n := range append(cli.Commands(), cli.Subcommands()...) {
		commands[n] = true
		if _, ok := rows[n]; !ok {
			t.Errorf("the command %s has no row", n)
		}
	}
	for name, row := range rows {
		if !commands[name] {
			t.Errorf("the row %s names no command", name)
			continue
		}
		checkRow(t, name, row, rows)
	}
}

// checkRow checks one row against its command.
func checkRow(t *testing.T, name string, row cli.Row, rows map[string]cli.Row) {
	t.Helper()
	if row.Summary == "" || row.Usage == "" {
		t.Errorf("%s: a summary %q and a usage %q, and neither may be empty", name, row.Summary, row.Usage)
	}
	if len(row.Exits) == 0 {
		t.Errorf("%s: no exit code", name)
	}
	for _, e := range row.Exits {
		if e < cli.ExitOK || e > cli.ExitOpen {
			t.Errorf("%s: the exit code %d is none of the command line's", name, e)
		}
	}
	words := append([]string(nil), row.Words...)
	for _, a := range row.Args {
		if a.Name == "" || a.Sentence == "" || len(a.Values) == 0 {
			t.Errorf("%s: the argument %q has no name, sentence or values", name, a.Name)
			continue
		}
		if !strings.Contains(row.Usage, "<"+a.Name+">") {
			t.Errorf("%s: the usage %q does not name <%s>", name, row.Usage, a.Name)
		}
		// An argument whose value is a subcommand names that subcommand's row; the command itself
		// reads no flag before it.
		if _, sub := rows[name+" "+a.Values[0]]; !sub {
			words = append(words, a.Values[0])
		}
	}
	listed := map[string]cli.Flag{}
	for _, f := range row.Flags {
		if _, ok := listed[f.Name]; ok {
			t.Errorf("%s: --%s is listed twice", name, f.Name)
		}
		listed[f.Name] = f
		if f.Sentence == "" {
			t.Errorf("%s: --%s has no sentence", name, f.Name)
		}
		if (f.Value == "") != (f.Kind == cli.KindSwitch) {
			t.Errorf("%s: --%s: a switch takes no value, and only it: %q", name, f.Name, f.Value)
		}
	}
	read := cli.ReadFlags(words...)
	for n, rf := range read {
		f, ok := listed[n]
		if !ok {
			t.Errorf("%s reads --%s, which its row does not list", name, n)
			continue
		}
		if rf.Kind != f.Kind || rf.Default != defaultOf(f) {
			t.Errorf("%s: --%s is read as %v with the default %q, and its row gives %v and %q", name, n, rf.Kind, rf.Default, f.Kind, defaultOf(f))
		}
	}
	for n := range listed {
		if _, ok := read[n]; !ok {
			t.Errorf("%s's row lists --%s, which it does not read", name, n)
		}
	}
	_, hasJSON := listed["json"]
	if hasJSON != (len(row.JSON) > 0) {
		t.Errorf("%s: takes --json %v, and cites %v for its object", name, hasJSON, row.JSON)
	}
	for _, id := range row.JSON {
		if !specIDRE.MatchString(id) {
			t.Errorf("%s: its object cites %q, which is no requirement of section 11", name, id)
		}
	}
	// The usage names the row's flags, but --json, which every command takes but mcp, and the socket
	// flags, written as one placeholder.
	want := map[string]bool{}
	for n := range listed {
		want[n] = n != "json"
	}
	if strings.Contains(row.Usage, "<socket flags>") {
		for _, n := range socketFlagNames {
			if _, ok := listed[n]; !ok {
				t.Errorf("%s: the usage names the socket flags, and the row does not list --%s", name, n)
			}
			want[n] = false
		}
	}
	named := map[string]bool{}
	for _, m := range usageFlagRE.FindAllStringSubmatch(row.Usage, -1) {
		named[m[1]] = true
		if !want[m[1]] {
			t.Errorf("%s: the usage %q names --%s, which is not a flag of its own in its row", name, row.Usage, m[1])
		}
	}
	for n, w := range want {
		if w && !named[n] {
			t.Errorf("%s: the usage %q does not name --%s", name, row.Usage, n)
		}
	}
}

// defaultOf is a flag's default as the flag package prints it: its row's, or the zero value of its
// kind.
func defaultOf(f cli.Flag) string {
	if f.Default != "" {
		return f.Default
	}
	switch f.Kind {
	case cli.KindSwitch:
		return "false"
	case cli.KindNumber:
		return "0"
	}
	return ""
}
