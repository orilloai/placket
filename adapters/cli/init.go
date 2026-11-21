package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

func init() {
	commands["init"] = initCommand
}

// codeDirNotEmpty: init's --dir holds something already, and init writes nothing.
const codeDirNotEmpty code = "dir-not-empty"

// The files init writes in each Accord's directory, beside the keys and openings in its own.
const (
	FileFounding = "founding.json"
	fileTerms    = ".terms"
)

// applierDir is the directory of D the fetch line puts the example's applier in.
const applierDir = "applier"

// applierBase is where the fetch line init prints takes an example's applier from: the repository's
// files at a ref, <base><ref>/patterns/<pattern>/apply.sh.
const applierBase = "https://raw.githubusercontent.com/orilloai/placket/"

// bindUntil is the until of every binding init signs: an example Accord is for trying placket, and
// no binding of it should stop it first.
const bindUntil = "2099-01-01T00:00:00Z"

// An example is one Accord, or two, that init founds for one person (docs/adr/0027-placket-init.md).
type example struct {
	accords []exampleAccord
	// pattern is the applier's pattern, and target what its run line names after the label; none
	// for an example with no applier.
	pattern, target, applyLine string
}

// An exampleAccord is one Accord of an example: the suffix of its label, and its parties.
type exampleAccord struct {
	suffix  string
	parties []exampleParty
}

// An exampleParty is a party of an example Accord: its name, its role and its terms. An agent's
// label is served by the claude mcp add line; the person's is watched.
type exampleParty struct {
	name, role, terms string
	agent, person     bool
}

// The parties of approve and merge: an agent that proposes, a person in the deciding role, and an
// applier that is handed what was released.
func askedExample(kind string, answers []string, applierGets []string, personGives string) []exampleParty {
	q := func(l []string) string { return `"` + strings.Join(l, `","`) + `"` }
	return []exampleParty{
		{name: "agent", role: "agent", agent: true, terms: `{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["` + kind + `"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":[` + q(answers) + `]}}]}
`},
		{name: "person", role: "decider", person: true, terms: `{"terms":[
 {"stance":"want","dir":"give","with":["role:agent"],"select":{"kind":[` + q(answers) + `]}}` + personGives + `]}
`},
		{name: "applier", role: "applier", terms: `{"terms":[
 {"stance":"want","dir":"get","with":["role:agent"],"select":{"kind":["` + kind + `"]}}` + strings.Join(applierGets, "") + `]}
`},
	}
}

var examples = map[string]example{
	"approve": {
		accords: []exampleAccord{{parties: askedExample("proposal", []string{"answer", "changed-version", "comment"},
			[]string{`,
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["changed-version"]}}`},
			`,
 {"stance":"want","dir":"give","with":["role:applier"],"select":{"kind":["changed-version"]}}`)}},
		pattern: "local-apply", target: "FILE", applyLine: LineInitApplyFile,
	},
	"merge": {
		accords: []exampleAccord{{parties: askedExample("merge", []string{"answer", "comment"}, nil, "")}},
		pattern: "github-merge", target: "OWNER/NAME", applyLine: LineInitApplyMerge,
	},
	"handoff": {
		accords: []exampleAccord{
			{suffix: "-plan", parties: []exampleParty{
				{name: "planner", role: "planner", agent: true, terms: `{"terms":[
 {"stance":"want","dir":"give","with":["role:coder"],"select":{"kind":["plan"]}}]}
`},
				{name: "coder", role: "coder", agent: true, terms: `{"terms":[
 {"stance":"want","dir":"get","with":["role:planner"],"select":{"kind":["plan"]}}]}
`}}},
			{suffix: "-review", parties: []exampleParty{
				{name: "coder", role: "coder", agent: true, terms: `{"terms":[
 {"stance":"want","dir":"give","with":["role:reviewer"],"select":{"kind":["work"]}}]}
`},
				{name: "reviewer", role: "reviewer", agent: true, terms: `{"terms":[
 {"stance":"want","dir":"get","with":["role:coder"],"select":{"kind":["work"]}}]}
`}}},
		},
	},
}

// exampleNames are the examples init takes, in the order they are offered.
var exampleNames = []string{"approve", "merge", "handoff"}

// longest is the length of the longest entry label of the example, under the label l.
func (x example) longest(l string) int {
	n := 0
	for _, a := range x.accords {
		for _, p := range a.parties {
			n = max(n, len(l+a.suffix+"-"+p.name))
		}
	}
	return n
}

// initCommand is `init <example> [--dir D] [--label L]` (docs/adr/0027-placket-init.md): it founds
// the example's Accords for one person, who holds every key, and prints what to run next. The
// example is approve, merge or handoff; the label is the example's name unless --label gives one.
//
// Into D, by default <placket config dir>/<label>/, made mode 0700, it writes the mediator's, the
// operator's and the principal's keys, one key per party, <entry>.key, each mode 0600, and each
// party's opening, so that D is the window's --openings; and for each Accord, in D/<accord label>/,
// each party's terms, the policy, the mediator, the card, founding.json and acceptances.json, each
// member's acceptance signed with its key of the view the core builds for it. Each party's view and
// acceptance are kept in its state directory, as view and accept keep them. It adds one entry per
// party to the labels config, <accord label>-<party>, with its Accord's default socket.
//
// Before it writes anything it refuses: an example it does not know, a label that is not one or
// whose longest entry would be longer than a label may be, a D or a socket path that is not
// printable, error usage; a socket path too long, error socket-path limit N; a label the config holds
// already, error label-taken; a D that holds anything, error dir-not-empty. A failure after it has
// made D removes what it wrote there.
//
// On standard error it prints, per Accord, its id, each party's label, key reference and role, and
// the window's line; then the claude mcp add line of the agents' labels, the watch line of the
// person's, and the lines that fetch and run the example's applier, each command on a line of its
// own after the line that says what it is for. Its --json object holds dir, accords (label, accord,
// window and parties), and mcp, watch, fetch and apply, the commands.
func initCommand(r *run, args []string) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(r.stderr, "usage: placket init <example> [--dir D] [--label L]; examples: "+strings.Join(exampleNames, ", "))
		return r.tool(codeUsage)
	}
	x, ok := examples[args[0]]
	if !ok {
		fmt.Fprintln(r.stderr, "usage: placket init: the examples are "+strings.Join(exampleNames, ", "))
		return r.tool(codeUsage)
	}
	set := newFlags("init")
	dirFlag, labelFlag := set.text("dir"), set.text("label")
	if !r.parse(set, args[1:]) {
		return r.tool(codeUsage)
	}
	// The label is the example's name unless --label is given, even given empty.
	label := args[0]
	set.Visit(func(f *flag.Flag) {
		if f.Name == "label" {
			label = *labelFlag
		}
	})
	if !client.Label(label) || x.longest(label) > 63 {
		fmt.Fprintln(r.stderr, "usage: placket init: --label is 1 to 63 of a-z, 0-9 and -, and with its parties' names no longer than 63")
		return r.tool(codeUsage)
	}
	// The applier is fetched into D/applier/, so no Accord's directory may be that one.
	for _, a := range x.accords {
		if x.pattern != "" && label+a.suffix == applierDir {
			fmt.Fprintln(r.stderr, "usage: placket init: --label "+applierDir+" names the directory the applier is fetched into")
			return r.tool(codeUsage)
		}
	}
	cfgDir := client.ConfigDir(r.getenv)
	if cfgDir == "" {
		fmt.Fprintln(r.stderr, "usage: placket init: there is no config directory: set HOME")
		return r.tool(codeUsage)
	}
	d := *dirFlag
	if d == "" {
		d = filepath.Join(cfgDir, label)
	}
	d, err := filepath.Abs(d)
	if err != nil || !printable(d) || !printable(cfgDir) {
		fmt.Fprintln(r.stderr, "usage: placket init: the directory's path holds a character a terminal does not print")
		return r.tool(codeUsage)
	}
	// Every Accord's default socket is as long as any other's under this environment.
	probe := local.DefaultSocketWith(r.getenv, canonical.HashPrefix+strings.Repeat("0", 64))
	if !printable(probe) {
		fmt.Fprintln(r.stderr, "usage: placket init: the socket's path holds a character a terminal does not print")
		return r.tool(codeUsage)
	}
	if err := local.CheckSocket(probe); err != nil {
		return r.failed(err)
	}
	cfg, err := r.config()
	if err != nil {
		return r.failed(err)
	}
	for _, m := range cfg.Mediators {
		for _, a := range x.accords {
			for _, p := range a.parties {
				if m.Label == label+a.suffix+"-"+p.name {
					return r.tool(string(client.CodeLabelTaken))
				}
			}
		}
	}
	if fi, err := os.Lstat(d); err == nil {
		if !fi.IsDir() {
			return r.tool(codeDirNotEmpty)
		}
		if l, err := os.ReadDir(d); err != nil || len(l) > 0 {
			return r.tool(codeDirNotEmpty)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return r.tool(codeFile, "dir")
	}

	w := &initWriter{r: r, d: d}
	if err := w.make(); err != nil {
		return r.tool(codeFile, "dir")
	}
	done, code := w.found(x, label, cfgDir)
	if code != 0 {
		w.undo()
		return code
	}
	w.print(x, done)
	return ExitOK
}

// An initWriter is one run of init writing into its directory, and what it made, to undo.
type initWriter struct {
	r       *run
	d       string
	created bool
}

// make makes the directory, mode 0700, or sets an empty one to that mode.
func (w *initWriter) make() error {
	if err := os.MkdirAll(filepath.Dir(w.d), 0o700); err != nil {
		return err
	}
	err := os.Mkdir(w.d, 0o700)
	if err == nil {
		w.created = true
		return nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	return os.Chmod(w.d, 0o700)
}

// undo removes what init wrote: the directory it made, or, in one that was empty, what it holds.
func (w *initWriter) undo() {
	if w.created {
		_ = os.RemoveAll(w.d)
		return
	}
	l, _ := os.ReadDir(w.d)
	for _, e := range l {
		_ = os.RemoveAll(filepath.Join(w.d, e.Name()))
	}
}

// A foundParty is a party init made: its entry's label, its key's path and reference, and its role.
type foundParty struct {
	label, key, actor, role string
	person, agent           bool
}

// A foundAccord is an Accord init founded: its label, its id, its window's line, and its parties.
type foundAccord struct {
	label, id, window string
	parties           []foundParty
}

// key makes a key file in the directory, and returns its path and key.
func (w *initWriter) key(name string) (string, signing.Key, error) {
	p := filepath.Join(w.d, name+".key")
	k, err := keyfile.New(p, w.r.env.Rand)
	return p, k, err
}

// found makes every key, founds each of the example's Accords, keeps each member's view and
// acceptance, and adds the entries to the labels config. It returns the Accords, or the exit code of
// what failed, which it has printed.
func (w *initWriter) found(x example, label, cfgDir string) ([]foundAccord, int) {
	r := w.r
	keys := map[string]signing.Key{}
	paths := map[string]string{}
	for _, n := range []string{"mediator", "operator", "principal"} {
		p, k, err := w.key(n)
		if err != nil {
			return nil, r.failed(err)
		}
		keys[n], paths[n] = k, p
	}
	med := canonicalJSON(map[string]any{"actor": keys["mediator"].Reference(), "operator": keys["operator"].Reference(), "runs": "process"})
	card := composition.Card()
	at := local.Stamp(r.env.Now())
	var out []foundAccord
	var entries []json.RawMessage
	for _, xa := range x.accords {
		al := label + xa.suffix
		ad := filepath.Join(w.d, al)
		if err := os.Mkdir(ad, 0o700); err != nil {
			return nil, r.tool(codeFile, "dir")
		}
		var roles []string
		for _, p := range xa.parties {
			roles = append(roles, p.role)
		}
		policy := canonicalJSON(map[string]any{"roles": roles,
			"principals": []any{map[string]any{"principal": keys["principal"].Reference(), "roles": roles}}})
		policyHash, err := canonical.TypedHash(policy)
		if err != nil {
			return nil, r.tool(codeInternal)
		}
		fa := foundAccord{label: al}
		openings := map[string][]byte{}
		partyKeys := map[string]signing.Key{}
		var seals, binds []json.RawMessage
		for _, p := range xa.parties {
			entry := al + "-" + p.name
			kp, k, err := w.key(entry)
			if err != nil {
				return nil, r.failed(err)
			}
			partyKeys[entry] = k
			fa.parties = append(fa.parties, foundParty{label: entry, key: kp, actor: k.Reference(), role: p.role, person: p.person, agent: p.agent})
			if err := writeNew(filepath.Join(ad, p.name+fileTerms), []byte(p.terms)); err != nil {
				return nil, r.tool(codeFile, "terms")
			}
			seal, opening, code := w.seal(k, p.terms)
			if code != 0 {
				return nil, code
			}
			if err := writeNew(filepath.Join(w.d, OpeningName(k.Reference())), append(opening, '\n')); err != nil {
				return nil, r.tool(codeFile, "opening")
			}
			openings[k.Reference()] = opening
			seals = append(seals, seal)
			b := map[string]any{"actor": k.Reference(), "principal": keys["principal"].Reference(), "roles": []string{p.role},
				"policy": policyHash, "mediator": keys["mediator"].Reference(), "until": bindUntil}
			sig, err := signing.Sign(canonicalJSON(b), signing.TypeBinding, keys["principal"])
			if err != nil {
				return nil, r.tool(codeInternal)
			}
			b["sig"] = sig
			binds = append(binds, canonicalJSON(b))
		}
		set := canonicalJSON(map[string]any{"policy": json.RawMessage(policy), "mediator": json.RawMessage(med), "card": json.RawMessage(card),
			"declarations": sortByActor(seals), "bindings": sortByActor(binds)})
		a, err := setup.Found(set, setup.Openings{ByActor: openings}, at)
		if err != nil {
			return nil, r.foundingError(err)
		}
		fa.id = a.ID()
		// Every member's acceptance of its own view, signed with its key, as accept signs it.
		views := map[string][]byte{}
		var accs [][]byte
		for _, p := range fa.parties {
			v, err := founding.View(a, p.actor)
			if err != nil {
				return nil, r.foundingError(err)
			}
			acc, err := founding.Accept(a.ID(), v, partyKeys[p.label])
			if err != nil {
				return nil, r.foundingError(err)
			}
			views[p.label] = v
			accs = append(accs, acc)
		}
		list, err := founding.Acceptances(accs)
		if err != nil {
			return nil, r.foundingError(err)
		}
		if _, err := founding.Found(founding.Inputs{Set: set, Openings: setup.Openings{ByActor: openings}, Acceptances: list}, at); err != nil {
			return nil, r.foundingError(err)
		}
		for name, b := range map[string][]byte{"policy.json": policy, "mediator.json": med, "card.json": card, FileFounding: set} {
			if err := writeNew(filepath.Join(ad, name), append(append([]byte(nil), b...), '\n')); err != nil {
				return nil, r.tool(codeFile, name)
			}
		}
		if err := writeAcceptances(filepath.Join(ad, FileAcceptances), list); err != nil {
			return nil, r.tool(codeFile, "acceptances")
		}
		for i, p := range fa.parties {
			if code := w.keep(p, a.ID(), views[p.label], accs[i]); code != 0 {
				return nil, code
			}
		}
		socket := local.DefaultSocketWith(r.getenv, a.ID())
		if err := local.MakeSocketDir(socket); err != nil {
			return nil, r.failed(err)
		}
		fa.window = shellLine("placket", "window", "--key", paths["mediator"], "--founding", filepath.Join(ad, FileFounding),
			"--openings", w.d, "--acceptances", filepath.Join(ad, FileAcceptances), "--journal", filepath.Join(ad, "journal"), "--socket", socket)
		for _, p := range fa.parties {
			entries = append(entries, canonicalJSON(map[string]any{"label": p.label, "accord": a.ID(), "key": p.key,
				"founding": filepath.Join(ad, FileFounding), "socket": socket}))
		}
		out = append(out, fa)
	}
	if code := w.addEntries(cfgDir, entries); code != 0 {
		return nil, code
	}
	return out, 0
}

// seal signs the key's declaration of the terms and seals it under a salt init mints, as declare
// and seal do; it returns the seal entry and the opening.
func (w *initWriter) seal(k signing.Key, termsFile string) ([]byte, []byte, int) {
	terms, ok := readTerms([]byte(termsFile))
	if !ok {
		return nil, nil, w.r.tool(codeTerms)
	}
	decl := canonicalJSON(map[string]any{"actor": k.Reference(), "terms": terms})
	sig, err := signing.Sign(decl, signing.TypeDeclaration, k)
	if err != nil {
		return nil, nil, w.r.tool(codeTerms)
	}
	decl = canonicalJSON(map[string]any{"actor": k.Reference(), "terms": terms, "sig": sig})
	salt := make([]byte, setup.SaltBytes)
	if _, err := io.ReadFull(w.r.env.Rand, salt); err != nil {
		return nil, nil, w.r.tool(codeInternal)
	}
	entry, opening, err := setup.Seal(decl, salt)
	if err != nil {
		return nil, nil, w.r.tool(codeSeal)
	}
	return entry, opening, 0
}

// keep keeps the party's view and its acceptance in its state directory, under its lock, as view
// and accept keep them.
func (w *initWriter) keep(p foundParty, accord string, view, acc []byte) int {
	dir, err := state.Open(state.DefaultPath(p.key, accord), p.actor, w.r.stderr)
	if err != nil {
		return w.r.failed(err)
	}
	if err := dir.Lock(); err != nil {
		return w.r.failed(err)
	}
	defer dir.Unlock()
	if err := dir.KeepView(view); err != nil {
		return w.r.failed(err)
	}
	if err := dir.KeepAcceptance(acc); err != nil {
		return w.r.failed(err)
	}
	return 0
}

// addEntries adds the entries to the labels config, after those it holds, which it keeps as they
// are: it checks the whole config as a reader would, then writes it through a file of its own in
// the config directory, mode 0600, synced, renamed over it.
func (w *initWriter) addEntries(cfgDir string, entries []json.RawMessage) int {
	path := filepath.Join(cfgDir, client.ConfigFile)
	var f struct {
		Mediators []json.RawMessage `json:"mediators"`
	}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil || json.Unmarshal(b, &f) != nil:
		return w.r.tool(string(client.CodeBadConfig))
	}
	list := append(f.Mediators, entries...)
	lines := make([]string, len(list))
	for i, e := range list {
		lines[i] = " " + string(canonicalJSON(e))
	}
	nb := []byte("{\"mediators\":[\n" + strings.Join(lines, ",\n") + "]}\n")
	if _, err := client.DecodeConfig(nb, cfgDir, w.r.getenv); err != nil {
		return w.r.failed(err)
	}
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return w.r.tool(codeFile, "config")
	}
	tmp, err := os.CreateTemp(cfgDir, "."+client.ConfigFile+".*")
	if err != nil {
		return w.r.tool(codeFile, "config")
	}
	_, werr := tmp.Write(nb)
	serr := tmp.Sync()
	cerr := tmp.Close()
	if werr != nil || serr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
		return w.r.tool(codeFile, "config")
	}
	return 0
}

// print prints what init founded and what to run next, and puts it in the --json object.
func (w *initWriter) print(x example, done []foundAccord) {
	r := w.r
	var agents []string
	person := ""
	applier := ""
	var accords []any
	for _, a := range done {
		fmt.Fprintf(r.stderr, LineInitAccord+"\n", a.label, a.id)
		var parties []any
		for _, p := range a.parties {
			fmt.Fprintf(r.stderr, LineInitParty+"\n", p.label, p.actor, p.role)
			parties = append(parties, map[string]any{"label": p.label, "actor": p.actor, "role": p.role})
			switch {
			case p.agent:
				agents = append(agents, "--label", p.label)
			case p.person:
				person = p.label
			default:
				applier = p.label
			}
		}
		fmt.Fprintf(r.stderr, LineInitWindow+"\n", a.label)
		fmt.Fprintln(r.stderr, "  "+a.window)
		accords = append(accords, map[string]any{"label": a.label, "accord": a.id, "window": a.window, "parties": parties})
	}
	r.put("dir", w.d)
	r.put("accords", accords)
	mcp := shellLine(append([]string{"claude", "mcp", "add", "placket", "--", "placket", "mcp"}, agents...)...)
	fmt.Fprintln(r.stderr, LineInitMCP)
	fmt.Fprintln(r.stderr, "  "+mcp)
	r.put("mcp", mcp)
	if person != "" {
		watch := shellLine("placket", "watch", "--label", person)
		fmt.Fprintln(r.stderr, LineInitWatch)
		fmt.Fprintln(r.stderr, "  "+watch)
		r.put("watch", watch)
	}
	if x.pattern == "" {
		return
	}
	version, revision := "", ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		version = bi.Main.Version
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
		}
	}
	ref := ApplierRef(version, revision)
	switch ref {
	case "main":
		fmt.Fprintln(r.stderr, LineInitFetchMain)
	case version:
		fmt.Fprintf(r.stderr, LineInitFetch+"\n", ref)
	default:
		fmt.Fprintf(r.stderr, LineInitFetchCommit+"\n", version, ref)
	}
	script := filepath.Join(w.d, applierDir, "apply.sh")
	fetch := shellLine("curl", "-fsSL", "--create-dirs", "-o", script, applierBase+ref+"/patterns/"+x.pattern+"/apply.sh")
	fmt.Fprintln(r.stderr, "  "+fetch)
	apply := shellLine("sh", script, applier, x.target)
	fmt.Fprintln(r.stderr, x.applyLine)
	fmt.Fprintln(r.stderr, "  "+apply)
	r.put("fetch", fetch)
	r.put("apply", apply)
}

// foundingError is the tool error of a founding init could not make: a bug, since it built every
// file itself.
func (r *run) foundingError(err error) int {
	var se *setup.Error
	if errors.As(err, &se) {
		return r.tool(codeFound, string(se.Code))
	}
	return r.tool(codeFound)
}

// sortByActor orders JSON objects by their actor members, as founding orders seals and bindings.
func sortByActor(l []json.RawMessage) []json.RawMessage {
	out := append([]json.RawMessage(nil), l...)
	sort.SliceStable(out, func(i, j int) bool {
		return canonical.Less(memberString(out[i], "actor"), memberString(out[j], "actor"))
	})
	return out
}

// releaseTag is a version that names a release tag: vMAJOR.MINOR.PATCH, with a pre-release of
// alpha, beta or rc at most; a pseudo-version, a dirty build and (devel) are not.
var releaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(alpha|beta|rc)(\.(0|[1-9][0-9]*))?)?$`)

// commitHash is a full git commit hash, as Go stamps it in vcs.revision.
var commitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ApplierRef is the ref init's fetch line takes an applier at, for a build of the version and the
// commit Go stamped: for a release tag, the tag's commit when the build carries it, since a tag can
// be moved and a commit cannot, or else the tag; and main for any other build.
func ApplierRef(version, revision string) string {
	if !releaseTag.MatchString(version) {
		return "main"
	}
	if commitHash.MatchString(revision) {
		return revision
	}
	return version
}

// printable reports whether s is UTF-8 that a terminal prints as it is: no control or format
// character, and no space but the ASCII one.
func printable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if c != ' ' && !unicode.IsPrint(c) {
			return false
		}
	}
	return true
}

// shellLine is the words as a line a POSIX shell reads back as them: a word of only the characters
// a shell takes as they are is written so, and any other in single quotes.
func shellLine(words ...string) string {
	out := make([]string, len(words))
	for i, s := range words {
		out[i] = shellWord(s)
	}
	return strings.Join(out, " ")
}

var plainWord = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellWord(s string) string {
	if plainWord.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
