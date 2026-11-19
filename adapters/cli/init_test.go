package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
)

// An inited is what init printed with --json: its directory, each Accord with its parties, and the
// lines it prints for the person to run.
type inited struct {
	Dir     string `json:"dir"`
	Accords []struct {
		Label   string `json:"label"`
		Accord  string `json:"accord"`
		Window  string `json:"window"`
		Parties []struct {
			Label string `json:"label"`
			Actor string `json:"actor"`
			Role  string `json:"role"`
		} `json:"parties"`
	} `json:"accords"`
	MCP   string `json:"mcp"`
	Watch string `json:"watch"`
	Fetch string `json:"fetch"`
	Apply string `json:"apply"`
}

// initRun runs init with --json, which must exit 0, and returns its object and what it printed on
// standard error, one line each, as the person reads it.
func initRun(t *testing.T, e cli.Env, args ...string) (inited, []string) {
	t.Helper()
	r := run(t, e, append([]string{"init"}, append(args, "--json")...)...)
	if r.code != 0 {
		t.Fatalf("init %v: %v", args, r)
	}
	var got inited
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("init's object: %v: %s", err, r.stdout)
	}
	return got, r.lines()
}

// commandLines are the lines of a command to run, each printed indented by two spaces, without the
// indent.
func commandLines(lines []string) []string {
	var out []string
	for _, l := range lines {
		if c, ok := strings.CutPrefix(l, "  "); ok {
			out = append(out, c)
		}
	}
	return out
}

// TestInitApproveFoundsTheAccord runs init approve from a clean HOME (U1): it exits 0 and prints the
// Accord's id, its parties, the full window line, the claude mcp add line of the agent, the watch
// line of the person, and the lines that fetch and run the applier, from main since a test binary
// has no version tag. The set, openings and acceptances it wrote found with the core's founding at
// the id printed; each entry's state directory keeps its view and its acceptance; the labels config
// holds approve-agent, approve-person and approve-applier on the Accord's default socket; and the
// window line starts a window that takes a give at once.
func TestInitApproveFoundsTheAccord(t *testing.T) {
	t.Parallel()
	e, home := labelsEnv(t)
	got, lines := initRun(t, e, "approve")
	d := filepath.Join(home, "config", "placket", "approve")
	if len(got.Accords) != 1 || got.Accords[0].Label != "approve" || got.Dir != d {
		t.Fatalf("init approve founded %+v", got.Accords)
	}
	a := got.Accords[0]
	id := a.Accord
	socket := local.DefaultSocketWith(e.Getenv, id)
	wantWindow := "placket window --key " + filepath.Join(d, "mediator.key") + " --founding " + filepath.Join(d, "approve", "founding.json") +
		" --openings " + d + " --acceptances " + filepath.Join(d, "approve", "acceptances.json") +
		" --journal " + filepath.Join(d, "approve", "journal") + " --socket " + socket
	if a.Window != wantWindow {
		t.Fatalf("the window line:\n got %s\nwant %s", a.Window, wantWindow)
	}
	roles := map[string]string{}
	refs := map[string]string{}
	for _, p := range a.Parties {
		roles[p.Label], refs[p.Label] = p.Role, p.Actor
	}
	if len(a.Parties) != 3 || roles["approve-agent"] != "agent" || roles["approve-person"] != "decider" || roles["approve-applier"] != "applier" {
		t.Fatalf("the parties: %+v", a.Parties)
	}
	fetch := "curl -fsSL --create-dirs -o " + filepath.Join(d, "applier", "apply.sh") +
		" https://raw.githubusercontent.com/orilloai/placket/main/patterns/local-apply/apply.sh"
	wantLines := []string{
		"accord approve " + id,
		"party approve-agent " + refs["approve-agent"] + " agent",
		"party approve-person " + refs["approve-person"] + " decider",
		"party approve-applier " + refs["approve-applier"] + " applier",
		fmt.Sprintf(cli.LineInitWindow, "approve"),
		"  " + wantWindow,
		cli.LineInitMCP,
		"  claude mcp add placket -- placket mcp --label approve-agent",
		cli.LineInitWatch,
		"  placket watch --label approve-person",
		cli.LineInitFetchMain,
		"  " + fetch,
		cli.LineInitApplyFile,
		"  sh " + filepath.Join(d, "applier", "apply.sh") + " approve-applier FILE",
	}
	if strings.Join(lines, "\n") != strings.Join(wantLines, "\n") {
		t.Fatalf("init printed:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(wantLines, "\n"))
	}
	if got.MCP != "claude mcp add placket -- placket mcp --label approve-agent" || got.Watch != "placket watch --label approve-person" ||
		got.Fetch != fetch || got.Apply != "sh "+filepath.Join(d, "applier", "apply.sh")+" approve-applier FILE" {
		t.Fatalf("init's object: %+v", got)
	}

	// The set founds, with its openings and acceptances, at the id printed.
	set := readTrimmed(t, filepath.Join(d, "approve", "founding.json"))
	openings := map[string][]byte{}
	for _, ref := range refs {
		openings[ref] = readTrimmed(t, filepath.Join(d, cli.OpeningName(ref)))
	}
	acc, err := founding.Found(founding.Inputs{Set: set, Openings: setup.Openings{ByActor: openings},
		Acceptances: readTrimmed(t, filepath.Join(d, "approve", "acceptances.json"))}, local.Stamp(time.Now()))
	if err != nil {
		t.Fatalf("the set init wrote does not found: %v", err)
	}
	if acc.ID() != id || !acc.TermsChecked() {
		t.Fatalf("the set founds %s, terms checked %v; init printed %s", acc.ID(), acc.TermsChecked(), id)
	}
	for label, ref := range refs {
		key := filepath.Join(d, label+".key")
		view, ok, err := state.Accepted(state.DefaultPath(key, id))
		if err != nil || !ok {
			t.Fatalf("%s keeps no accepted view: %v", label, err)
		}
		want, err := founding.View(acc, ref)
		if err != nil || !bytes.Equal(view, want) {
			t.Fatalf("%s keeps the view %s, want %s", label, view, want)
		}
		if acc.Roles(ref)[0] != roles[label] {
			t.Fatalf("%s's roles: %v", label, acc.Roles(ref))
		}
	}

	// The labels config holds one entry per party, each on the Accord's default socket.
	cfg, err := client.ReadConfig(e.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, m := range cfg.Mediators {
		labels = append(labels, m.Label)
		if m.Accord != id || m.Socket != socket || m.Founding != filepath.Join(d, "approve", "founding.json") || m.Key != filepath.Join(d, m.Label+".key") {
			t.Fatalf("the entry %+v", m)
		}
	}
	if strings.Join(labels, " ") != "approve-agent approve-person approve-applier" {
		t.Fatalf("the labels config holds %v", labels)
	}

	// The window line starts a window that is founded at once, and takes a give.
	w := startLine(t, e, a.Window)
	if !strings.Contains(w.stderr.String(), "accord "+id+" founded\n") {
		t.Fatalf("the window started: %s", w.stderr.String())
	}
	g := run(t, e, "give", "--label", "approve-agent", "--to", refs["approve-applier"], "--kind", "proposal", "--text", "Add a line.")
	if g.code != 4 || !strings.HasPrefix(g.stderr, "ask maybe sha256:") {
		t.Fatalf("the agent's give: %v", g)
	}
}

// readTrimmed reads a file, its one trailing line feed removed.
func readTrimmed(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(b, []byte("\n"))
}

// startLine runs a printed window line in this process, under e, until the test ends.
func startLine(t *testing.T, e cli.Env, line string) *window {
	t.Helper()
	f := strings.Fields(line)
	if len(f) < 2 || f[0] != "placket" || f[1] != "window" {
		t.Fatalf("not a window line: %s", line)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	e.Context, e.Ready = ctx, ready
	w := &window{cancel: cancel, done: make(chan int, 1), stderr: &syncBuffer{}}
	go func() { w.done <- cli.Run(e, f[1:], strings.NewReader(""), &bytes.Buffer{}, w.stderr) }()
	select {
	case <-ready:
	case code := <-w.done:
		t.Fatalf("the window exited %d: %s", code, w.stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	t.Cleanup(func() { w.stop(t) })
	return w
}

// TestInitKeysArePrivate checks every key init approve wrote (U2): mode 0600 and the user's own,
// in a directory of mode 0700, itself the user's.
func TestInitKeysArePrivate(t *testing.T) {
	t.Parallel()
	e, home := labelsEnv(t)
	// Without --json, init prints on standard error alone.
	if r := run(t, e, "init", "approve"); r.code != 0 || r.stdout != "" {
		t.Fatalf("init approve: %v", r)
	}
	d := filepath.Join(home, "config", "placket", "approve")
	keys, err := filepath.Glob(filepath.Join(d, "*.key"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, k := range keys {
		names = append(names, filepath.Base(k))
	}
	if strings.Join(names, " ") != "approve-agent.key approve-applier.key approve-person.key mediator.key operator.key principal.key" {
		t.Fatalf("the keys: %v", names)
	}
	for _, p := range append(keys, d) {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0o600)
		if p == d {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Fatalf("%s: mode %o, want %o", p, fi.Mode().Perm(), want)
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
			t.Fatalf("%s is not the user's own", p)
		}
	}
}

// TestInitNeverOverwrites checks what init refuses before it writes anything (U3): a --dir that is
// not empty is exit 3, error dir-not-empty, and the directory and the config stay as they were; a
// label the config holds already is error label-taken; a --label of 60 characters, whose entries
// would be longer than a label may be, is error usage; and so is an example init does not know.
func TestInitNeverOverwrites(t *testing.T) {
	t.Parallel()
	e, home := labelsEnv(t)
	full := filepath.Join(home, "full")
	if err := os.MkdirAll(full, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "notes"), []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, e, "init", "approve", "--dir", full)
	if r.code != 3 || r.stderr != "error dir-not-empty\n" || r.stdout != "" {
		t.Fatalf("init into a directory that is not empty: %v", r)
	}
	if l, _ := os.ReadDir(full); len(l) != 1 {
		t.Fatalf("init wrote into %s: %v", full, l)
	}
	nothing(t, home)

	// A label already in the config.
	cfgDir := filepath.Join(home, "config", "placket")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	taken := `{"mediators":[{"label":"approve-person","accord":"sha256:` + strings.Repeat("ab", 32) + `","key":"/k","founding":"/f"}]}`
	if err := os.WriteFile(filepath.Join(cfgDir, client.ConfigFile), []byte(taken), 0o600); err != nil {
		t.Fatal(err)
	}
	r = run(t, e, "init", "approve")
	if r.code != 3 || r.stderr != "error label-taken\n" {
		t.Fatalf("init with its label taken: %v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(cfgDir, client.ConfigFile)); string(b) != taken {
		t.Fatalf("init changed the config: %s", b)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "approve")); !os.IsNotExist(err) {
		t.Fatalf("init made its directory: %v", err)
	}

	// A label too long for its entries, and an example init does not know.
	for _, args := range [][]string{{"approve", "--label", strings.Repeat("a", 60)}, {"handoff", "--label", strings.Repeat("a", 48)},
		{"deploy"}, {}, {"approve", "extra"}, {"approve", "--label", "Big"}} {
		r = run(t, e, append([]string{"init"}, args...)...)
		if r.code != 3 || last(r.stderr) != "error usage" {
			t.Fatalf("init %v: %v", args, r)
		}
		if l, _ := os.ReadDir(cfgDir); len(l) != 1 {
			t.Fatalf("init %v wrote into %s: %v", args, cfgDir, l)
		}
	}
	// The longest label that fits: its longest entry, <label>-review-reviewer, has 63 characters.
	r = run(t, e, "init", "handoff", "--label", strings.Repeat("a", 47))
	if r.code != 0 {
		t.Fatalf("init handoff with a label of 47: %v", r)
	}
}

// nothing checks that init wrote no config and no directory under home's config directory.
func nothing(t *testing.T, home string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, "config")); !os.IsNotExist(err) {
		t.Fatalf("init wrote under the config directory: %v", err)
	}
}

// TestInitHandoffFoundsTwoAccords runs init handoff (U4): it prints two Accord ids and two window
// lines, the plan Accord's and the review Accord's, each with its own journal and socket; the config
// holds the four entries, and mediators lists them with no error; the claude mcp add line serves all
// four, and nothing is printed to watch or to fetch. Each window line starts its window, and the
// planner's plan and the coder's work each pass to their receiver.
func TestInitHandoffFoundsTwoAccords(t *testing.T) {
	t.Parallel()
	e, home := labelsEnv(t)
	got, lines := initRun(t, e, "handoff")
	d := filepath.Join(home, "config", "placket", "handoff")
	if len(got.Accords) != 2 || got.Accords[0].Label != "handoff-plan" || got.Accords[1].Label != "handoff-review" ||
		got.Accords[0].Accord == got.Accords[1].Accord {
		t.Fatalf("init handoff founded %+v", got.Accords)
	}
	var ids, windows []string
	for _, l := range lines {
		if f := strings.Fields(l); len(f) == 3 && f[0] == "accord" {
			ids = append(ids, f[2])
		}
	}
	for _, c := range commandLines(lines) {
		if strings.HasPrefix(c, "placket window ") {
			windows = append(windows, c)
		}
	}
	if len(ids) != 2 || ids[0] != got.Accords[0].Accord || ids[1] != got.Accords[1].Accord || len(windows) != 2 ||
		windows[0] != got.Accords[0].Window || windows[1] != got.Accords[1].Window {
		t.Fatalf("init handoff printed:\n%s", strings.Join(lines, "\n"))
	}
	for i, a := range got.Accords {
		if !strings.Contains(a.Window, " --journal "+filepath.Join(d, a.Label, "journal")+" --socket "+local.DefaultSocketWith(e.Getenv, a.Accord)) {
			t.Fatalf("window %d: %s", i, a.Window)
		}
	}
	if got.MCP != "claude mcp add placket -- placket mcp --label handoff-plan-planner --label handoff-plan-coder --label handoff-review-coder --label handoff-review-reviewer" ||
		got.Watch != "" || got.Fetch != "" || got.Apply != "" {
		t.Fatalf("init's object: %+v", got)
	}
	m := run(t, e, "mediators", "--json")
	var list struct {
		Mediators []struct{ Label string }
	}
	if m.code != 0 || json.Unmarshal([]byte(m.stdout), &list) != nil {
		t.Fatalf("mediators: %v", m)
	}
	var labels []string
	for _, x := range list.Mediators {
		labels = append(labels, x.Label)
	}
	if strings.Join(labels, " ") != "handoff-plan-planner handoff-plan-coder handoff-review-coder handoff-review-reviewer" {
		t.Fatalf("mediators lists %v", labels)
	}
	refs := map[string]string{}
	for _, a := range got.Accords {
		for _, p := range a.Parties {
			refs[p.Label] = p.Actor
		}
		startLine(t, e, a.Window)
	}
	if refs["handoff-plan-coder"] == refs["handoff-review-coder"] {
		t.Fatal("the coder has one key in both Accords")
	}
	for _, h := range []struct{ from, to, kind string }{
		{"handoff-plan-planner", "handoff-plan-coder", "plan"}, {"handoff-review-coder", "handoff-review-reviewer", "work"}} {
		g := run(t, e, "give", "--label", h.from, "--to", refs[h.to], "--kind", h.kind, "--text", "Done.")
		if g.code != 0 || !strings.HasPrefix(g.stderr, "pass ok sha256:") {
			t.Fatalf("%s gives %s: %v", h.from, h.kind, g)
		}
		f := run(t, e, "get", "--label", h.to)
		if f.code != 0 || f.stdout != "Done.\n" {
			t.Fatalf("%s gets: %v", h.to, f)
		}
	}
}

// TestInitMergeNamesItsApplier runs init merge --label shop --dir D (U1's example B): the parties
// are shop-agent, shop-person and shop-applier, in D, and the applier fetched is github-merge's,
// run on OWNER/NAME.
func TestInitMergeNamesItsApplier(t *testing.T) {
	t.Parallel()
	e, home := labelsEnv(t)
	d := filepath.Join(home, "mine")
	got, lines := initRun(t, e, "merge", "--label", "shop", "--dir", d)
	if len(got.Accords) != 1 || got.Accords[0].Label != "shop" || got.Dir != d {
		t.Fatalf("init merge: %+v", got)
	}
	cmds := commandLines(lines)
	want := []string{
		got.Accords[0].Window,
		"claude mcp add placket -- placket mcp --label shop-agent",
		"placket watch --label shop-person",
		"curl -fsSL --create-dirs -o " + filepath.Join(d, "applier", "apply.sh") +
			" https://raw.githubusercontent.com/orilloai/placket/main/patterns/github-merge/apply.sh",
		"sh " + filepath.Join(d, "applier", "apply.sh") + " shop-applier OWNER/NAME",
	}
	if strings.Join(cmds, "\n") != strings.Join(want, "\n") {
		t.Fatalf("init merge printed:\n%s", strings.Join(lines, "\n"))
	}
}

// TestInitFetchesAtTheVersionTag: a build whose version is a release tag fetches its applier at that
// tag; any other, a pseudo-version, a dirty build or none, from main.
func TestInitFetchesAtTheVersionTag(t *testing.T) {
	t.Parallel()
	for v, want := range map[string]string{
		"v0.1.0": "v0.1.0", "v1.12.3": "v1.12.3", "v0.2.0-rc.1": "v0.2.0-rc.1",
		"(devel)": "main", "": "main", "v0.1.0+dirty": "main", "v0.0.0-20261118120000-abcdefabcdef": "main",
		"v0.1.1-0.20261118120000-abcdefabcdef": "main", "v0.1": "main",
	} {
		if got := cli.ApplierRef(v); got != want {
			t.Errorf("version %q fetches at %q, want %q", v, got, want)
		}
	}
}

// TestInitQuotesItsPaths: a --dir with a space in it is printed quoted for the shell in every line
// that names it, and a --dir with a control character is error usage.
func TestInitQuotesItsPaths(t *testing.T) {
	t.Parallel()
	e, home := labelsEnv(t)
	d := filepath.Join(home, "my dir")
	got, _ := initRun(t, e, "approve", "--dir", d)
	if !strings.Contains(got.Accords[0].Window, " --key '"+filepath.Join(d, "mediator.key")+"' ") ||
		got.Apply != "sh '"+filepath.Join(d, "applier", "apply.sh")+"' approve-applier FILE" {
		t.Fatalf("init's lines: %+v", got)
	}
	e2, home2 := labelsEnv(t)
	r := run(t, e2, "init", "approve", "--dir", filepath.Join(home2, "a\x1bb"))
	if r.code != 3 || last(r.stderr) != "error usage" {
		t.Fatalf("init --dir with an escape: %v", r)
	}
}

// TestWindowMakesTheDefaultSocketsDirectory: a window given --socket that is its Accord's default
// socket makes the directory the socket is in, as it does with no --socket, so the line init prints
// starts after the runtime directory is gone.
func TestWindowMakesTheDefaultSocketsDirectory(t *testing.T) {
	t.Parallel()
	e, _ := labelsEnv(t)
	e.Getenv = withVar(e.Getenv, "XDG_RUNTIME_DIR", filepath.Join(shortRuntime(t), "r"))
	got, _ := initRun(t, e, "approve")
	socket := local.DefaultSocketWith(e.Getenv, got.Accords[0].Accord)
	if err := os.RemoveAll(filepath.Dir(socket)); err != nil {
		t.Fatal(err)
	}
	startLine(t, e, got.Accords[0].Window)
	fi, err := os.Stat(filepath.Dir(socket))
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("the socket's directory: %v, %v", fi, err)
	}
}

// withVar is getenv with name set to value.
func withVar(getenv func(string) string, name, value string) func(string) string {
	return func(n string) string {
		if n == name {
			return value
		}
		return getenv(n)
	}
}
