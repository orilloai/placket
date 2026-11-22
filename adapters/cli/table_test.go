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
// both. No summary or sentence is empty, the usage names exactly the row's flags, and the --json
// object of a command that takes --json cites the requirements it follows.
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
	if row.Summary == "" {
		t.Errorf("%s: no summary", name)
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
		if f.Own && !f.Required {
			t.Errorf("%s: --%s is checked by the command itself and not required", name, f.Name)
		}
		if f.DefaultArg && (len(row.Args) == 0 || f.Default != "" || f.Kind != cli.KindText) {
			t.Errorf("%s: --%s takes its default from an argument, and its row has none, or gives a default too", name, f.Name)
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
		want := defaultOf(f)
		if f.DefaultArg {
			want = words[len(words)-1]
		}
		if rf.Kind != f.Kind || rf.Default != want {
			t.Errorf("%s: --%s is read as %v with the default %q, and its row gives %v and %q", name, n, rf.Kind, rf.Default, f.Kind, want)
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
	// A flag that may be given more than once is the one the usage writes with ... after it.
	for n, f := range listed {
		if want[n] && repeatedInUsage(row.Usage, n) != (f.Kind == cli.KindList) {
			t.Errorf("%s: the usage %q and the row's kind, %v, disagree on whether --%s may be given more than once", name, row.Usage, f.Kind, n)
		}
	}
	// A flag the usage writes outside square brackets and outside a group of alternatives is one
	// the command needs, and a flag the row requires is written so at least once.
	needed := neededInUsage(row.Usage)
	for n, f := range listed {
		if !want[n] {
			continue
		}
		if needed[n] && !f.Required {
			t.Errorf("%s: the usage %q writes --%s as needed, and its row does not require it", name, row.Usage, n)
		}
		if f.Required && !needed[n] {
			t.Errorf("%s: its row requires --%s, and the usage %q writes it as optional", name, n, row.Usage)
		}
	}
}

// repeatedInUsage reports whether a usage writes the flag with ... after it, and its value, and any
// closing bracket.
func repeatedInUsage(usage, name string) bool {
	return regexp.MustCompile(`--` + regexp.QuoteMeta(name) + `( <[^>]*>)?\]?\.\.\.`).MatchString(usage)
}

// neededInUsage are the flags a usage writes outside every [...] and (... | ...).
func neededInUsage(usage string) map[string]bool {
	out := map[string]bool{}
	depth := 0
	for i := 0; i < len(usage); i++ {
		switch usage[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '-':
			if m := usageFlagRE.FindStringSubmatch(usage[i:]); m != nil && strings.HasPrefix(usage[i:], m[0]) {
				if depth == 0 {
					out[m[1]] = true
				}
				i += len(m[0]) - 1
			}
		}
	}
	return out
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
