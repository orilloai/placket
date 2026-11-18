package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
)

// labelsEnv is an environment whose config directory is a directory of the test's own, holding the
// labels config entries, and whose runtime and state directories are the test's too.
func labelsEnv(t testing.TB, entries ...map[string]any) (cli.Env, string) {
	t.Helper()
	home := t.TempDir()
	vars := map[string]string{"XDG_CONFIG_HOME": filepath.Join(home, "config"), "XDG_STATE_HOME": filepath.Join(home, "state"),
		"XDG_RUNTIME_DIR": shortRuntime(t), "HOME": home}
	if entries != nil {
		dir := filepath.Join(vars["XDG_CONFIG_HOME"], "placket")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		list := make([]any, len(entries))
		for i, e := range entries {
			list[i] = e
		}
		b, _ := json.Marshal(map[string]any{"mediators": list})
		if err := os.WriteFile(filepath.Join(dir, "mediators.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e := env()
	e.Getenv = func(n string) string { return vars[n] }
	return e, home
}

// shortRuntime is a runtime directory short enough for a default socket on every system.
func shortRuntime(t testing.TB) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "plr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// entry is an entry of the labels config for the key of name on this Accord's window.
func (a *accord) entry(name, label string) map[string]any {
	e := map[string]any{"accord": a.id, "key": a.key(name), "founding": filepath.Join(a.dir, "founding.json"), "socket": a.socket()}
	if label != "" {
		e["label"] = label
	}
	return e
}

// TestLabelsStandInForTheFlags gives with --label merge, which names the proposer's key on the
// Accord's window: the give reaches that window as that key, its envelope recorded in the key's
// own state directory. mediators lists merge with its Accord id and socket; --label nope is
// unknown-mediator, exit 3; --label with --socket is usage; with no config file, mediators lists
// nothing, and its object holds an empty list.
func TestLabelsStandInForTheFlags(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	e, _ := labelsEnv(t, a.entry("proposer", "merge"))
	g := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Merge it.")
	if g.code != 4 || !strings.HasPrefix(g.stderr, "ask maybe sha256:") {
		t.Fatalf("give --label merge: %v", g)
	}
	if b, err := os.ReadFile(filepath.Join(a.state("proposer"), "envelopes.jsonl")); err != nil || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("the proposer's envelopes: %q, %v", b, err)
	}
	if q := a.question(t, "decider"); q != last(g.stdout) {
		t.Fatalf("the window asks %s, the give printed %s", q, last(g.stdout))
	}
	m := run(t, e, "mediators")
	want := `{"accord":"` + a.id + `","founding":"` + filepath.Join(a.dir, "founding.json") + `","key":"` + a.key("proposer") +
		`","label":"merge","socket":"` + a.socket() + `","state":"` + a.state("proposer") + `"}` + "\n"
	if m.code != 0 || m.stdout != want {
		t.Fatalf("mediators: %v\nwant %s", m, want)
	}
	if r := run(t, e, "give", "--label", "nope", "--to", a.refs["applier"], "--kind", "proposal", "--text", "No."); r.code != 3 || r.stderr != "error unknown-mediator\n" {
		t.Fatalf("--label nope: %v", r)
	}
	if r := run(t, e, "give", "--label", "merge", "--socket", a.socket(), "--to", a.refs["applier"], "--kind", "proposal", "--text", "No."); r.code != 3 ||
		r.stderr != "usage: placket give: --label takes the place of --key, --socket and --accord\nerror usage\n" {
		t.Fatalf("--label with --socket: %v", r)
	}
	none, _ := labelsEnv(t)
	if r := run(t, none, "mediators"); r.code != 0 || r.stdout != "" {
		t.Fatalf("mediators with no config: %v", r)
	}
	if r := run(t, none, "mediators", "--json"); r.code != 0 || r.stdout != `{"exit":0,"mediators":[]}`+"\n" {
		t.Fatalf("mediators --json with no config: %v", r)
	}
}

// TestTheConfigRefusesCollisions runs mediators on configs that collide: two entries of one label
// are label-taken, two Accords on one socket, two entries on one state directory, and two unlabelled
// entries of one Accord are mediator-collision, each naming both entries, exit 3; a label of the
// form sha256: is bad-config; a config with a label twice and a socket of two Accords is label-taken
// alone; of four entries of four Accords where 0 and 3 use socket S1 and 1 and 2 use S2, 0 and 3 are
// named. Two unlabelled entries of different Accords on one key file are both listed, and
// --label sha256:<id> reaches that entry's window.
func TestTheConfigRefusesCollisions(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	other := func(n byte) string { return "sha256:" + strings.Repeat(string(rune(n)), 64) }
	with := func(e map[string]any, k string, v any) map[string]any {
		c := map[string]any{}
		for n, x := range e {
			c[n] = x
		}
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
		return c
	}
	s1, s2 := filepath.Join(a.dir, "s1"), filepath.Join(a.dir, "s2")
	base := a.entry("proposer", "")
	cases := []struct {
		name    string
		entries []map[string]any
		stderr  string
	}{
		{"a label twice", []map[string]any{with(base, "label", "merge"), with(with(base, "label", "merge"), "key", a.key("decider"))}, "error label-taken 0 1\n"},
		{"two Accords on one socket", []map[string]any{with(base, "label", "a"), with(with(base, "label", "b"), "accord", other('1'))}, "error mediator-collision 0 1\n"},
		{"one state directory", []map[string]any{with(base, "label", "a"), with(with(base, "label", "b"), "socket", s1)}, "error mediator-collision 0 1\n"},
		{"one Accord unlabelled twice", []map[string]any{base, with(base, "key", a.key("decider"))}, "error mediator-collision 0 1\n"},
		{"a label like an Accord id", []map[string]any{with(base, "label", a.id)}, "error bad-config\n"},
		{"a label twice and one socket", []map[string]any{with(base, "label", "m"), with(with(with(base, "label", "m"), "accord", other('2')), "key", a.key("decider"))}, "error label-taken 0 1\n"},
		{"the first pair by i, then j", []map[string]any{
			with(with(base, "socket", s1), "accord", other('3')), with(with(base, "socket", s2), "accord", other('4')),
			with(with(base, "socket", s2), "accord", other('5')), with(with(base, "socket", s1), "accord", other('6'))}, "error mediator-collision 0 3\n"},
	}
	for _, c := range cases {
		e, _ := labelsEnv(t, c.entries...)
		if r := run(t, e, "mediators"); r.code != 3 || r.stderr != c.stderr || r.stdout != "" {
			t.Fatalf("%s: %v", c.name, r)
		}
	}
	e, _ := labelsEnv(t, base, with(with(base, "accord", other('7')), "socket", s1))
	if r := run(t, e, "mediators"); r.code != 0 || strings.Count(r.stdout, "\n") != 2 {
		t.Fatalf("two Accords on one key file: %v", r)
	}
	if r := run(t, e, "give", "--label", a.id, "--to", a.refs["applier"], "--kind", "proposal", "--text", "By id."); r.code != 4 {
		t.Fatalf("give --label <the Accord id>: %v", r)
	}
}

// TestAWindowWithNoSocketListensOnTheDefault runs a window with no --socket and no --journal: it
// listens on the Accord's default socket and writes the Accord's default journal, and a give with
// the labels config's entry, which names no socket, reaches it.
func TestAWindowWithNoSocketListensOnTheDefault(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	e, home := labelsEnv(t, map[string]any{"accord": a.id, "key": a.key("proposer"), "founding": filepath.Join(a.dir, "founding.json"), "label": "merge"})
	w := startWith(t, e, "window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"))
	defer w.stop(t)
	if r := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Default."); r.code != 4 {
		t.Fatalf("a give on the default socket: %v", r)
	}
	journal := filepath.Join(home, "state", "placket", strings.TrimPrefix(a.id, "sha256:"))
	if b, err := os.ReadFile(filepath.Join(journal, "receipts.jsonl")); err != nil || strings.Count(string(b), "\n") != 1 {
		t.Fatalf("the default journal: %v", err)
	}
}

// TestAWindowWithNoHomeNeedsItsJournal runs a window with no --journal, no XDG_STATE_HOME and no
// HOME, from a directory of its own: it is tool error usage, starts nothing and writes nothing in
// that directory.
func TestAWindowWithNoHomeNeedsItsJournal(t *testing.T) {
	a := newAccord(t, "proposer", "applier", "decider")
	e := env()
	e.Getenv = func(string) string { return "" }
	t.Chdir(t.TempDir())
	r := run(t, e, "window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--socket", a.socket())
	if r.code != 3 || r.stderr != "error usage\n" {
		t.Fatalf("a window with no journal to default to: %v", r)
	}
	if left, err := os.ReadDir("."); err != nil || len(left) != 0 {
		t.Fatalf("left in the directory it ran in: %v, %v", left, err)
	}
}
