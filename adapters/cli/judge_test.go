package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/judge/jev"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/setup"
)

// plantedJudgeKey is the key the tests plant in the environment: plain words, so the Makefile's
// secrets step takes it for no secret.
const plantedJudgeKey = "planted judge key words"

// judgeConfig is the config of a judge at host h.example and model m, with the instruction given.
func judgeConfig(name, instruction string) string {
	return `{"host":"h.example","instruction":"` + instruction + `","model":"m","name":"` + name + `","provider":"jev","threshold":80}`
}

// judgedTerms are the terms of a judged Accord: the proposer's patches and proposals cross to the
// applier only once the decider answers, and both sides list the judge review with the use given.
func judgedTerms(use string) map[string]string {
	return map[string]string{
		"proposer": `{"terms":[
		{"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal","patch"]},"resolve":{"to":["role:decider"],"timeout":3600,"judges":[{"name":"review","use":"` + use + `"}]}},
		{"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","changed-version","comment"]}}]}`,
		"applier": `{"terms":[
		{"stance":"maybe","dir":"get","with":["role:proposer"],"select":{"kind":["proposal","patch"]},"resolve":{"to":["role:decider"],"timeout":3600,"judges":[{"name":"review","use":"` + use + `"}]}},
		{"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["changed-version"]}}]}`,
		"decider": sixTerms["decider"],
	}
}

// A judgedAccord is an Accord whose card lists the judges review and spare, made by the commands,
// with review's config in review.json and spare's in spare.json. Only review is in any term.
type judgedAccord struct {
	*accord
	cards string
}

func (a *judgedAccord) config(name string) string { return filepath.Join(a.dir, name+".json") }

func newJudgedAccord(t *testing.T) *judgedAccord {
	t.Helper()
	return newJudgedAccordOf(t, "decide")
}

// newJudgedAccordOf is newJudgedAccord with review's use in both terms given.
func newJudgedAccordOf(t *testing.T, use string) *judgedAccord {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "plk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	parties := []string{"proposer", "applier", "decider"}
	a := &judgedAccord{accord: &accord{dir: d, refs: map[string]string{}, parties: parties}}
	if err := os.Mkdir(a.openings(), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name, s string) string {
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, k := range append([]string{"mediator", "operator", "principal", "stranger"}, parties...) {
		a.refs[k] = strings.TrimSpace(must(t, "key", "new", "--out", a.key(k)).stdout)
	}
	write("review.json", judgeConfig("review", "Block a patch that deletes data.")+"\n")
	write("spare.json", judgeConfig("spare", "Block nothing."))
	a.cards = must(t, "card", "--default", "--judge", a.config("review"), "--judge", a.config("spare")).stdout
	pb, _ := json.Marshal(map[string]any{"roles": parties, "principals": []any{map[string]any{"principal": a.refs["principal"], "roles": parties}}})
	policy := write("policy.json", string(pb))
	args := []string{"founding"}
	for _, p := range parties {
		decl := must(t, "declare", "--key", a.key(p), "--terms", write(p+".terms", judgedTerms(use)[p])).stdout
		entry := must(t, "seal", "--key", a.key(p), "--declaration", write(p+".declaration", decl), "--out", a.openings()).stdout
		bind := must(t, "bind", "--key", a.key("principal"), "--actor", a.refs[p], "--role", p, "--mediator", a.refs["mediator"],
			"--until", "2099-01-01T00:00:00Z", "--policy", policy).stdout
		args = append(args, "--seal", write(p+".seal", entry), "--binding", write(p+".binding", bind))
	}
	args = append(args, "--policy", policy,
		"--mediator", write("mediator.json", `{"actor":"`+a.refs["mediator"]+`","operator":"`+a.refs["operator"]+`","runs":"process"}`),
		"--card", write("card.json", a.cards))
	r := must(t, args...)
	write("founding.json", r.stdout)
	for _, l := range r.lines() {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "accord" {
			a.id = f[1]
		}
	}
	// Every party accepts its view beforehand, so each window starts founded.
	if err := os.Rename(a.acceptancesOf(t, a.founding()), a.acceptances()); err != nil {
		t.Fatal(err)
	}
	return a
}

// A keyEnv is a Getenv that holds one key, and counts each read of jev.KeyEnv.
type keyEnv struct {
	mu    sync.Mutex
	key   string
	reads int
	other []string
}

func (k *keyEnv) Getenv(name string) string {
	k.mu.Lock()
	defer k.mu.Unlock()
	if name != jev.KeyEnv {
		k.other = append(k.other, name)
		return ""
	}
	k.reads++
	return k.key
}

// startJudged runs the window with Getenv and the --judge files of the names, until the test ends or
// it is stopped.
func (a *judgedAccord) startJudged(t *testing.T, getenv func(string) string, judges ...string) *window {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	e := env()
	e.Context, e.Ready, e.Getenv = ctx, ready, getenv
	w := &window{cancel: cancel, done: make(chan int, 1), stderr: &syncBuffer{}}
	args := []string{"window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--openings", a.openings(),
		"--openings", a.openings(), "--socket", a.socket(), "--journal", a.journal()}
	for _, n := range judges {
		args = append(args, "--judge", a.config(n))
	}
	go func() { w.done <- cli.Run(e, args, strings.NewReader(""), &bytes.Buffer{}, w.stderr) }()
	select {
	case <-ready:
	case code := <-w.done:
		t.Fatalf("the window exited %d: %s", code, w.stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	a.window = w
	t.Cleanup(func() { w.stop(t) })
	return w
}

// TestCardListsTheJudges builds a card with two judges' configs: its judges are each config's name,
// jev.Version and the typed hash of the config's canonical bytes, in flag order, as the founded
// Accord reads them (setup-contract-4, docs/adr/0009-who-decided.md). A config that does not hold is
// tool error judge with its code.
func TestCardListsTheJudges(t *testing.T) {
	t.Parallel()
	a := newJudgedAccord(t)
	review, _ := canonical.TypedHash([]byte(judgeConfig("review", "Block a patch that deletes data.")))
	spare, _ := canonical.TypedHash([]byte(judgeConfig("spare", "Block nothing.")))
	set, err := os.ReadFile(filepath.Join(a.dir, "founding.json"))
	if err != nil {
		t.Fatal(err)
	}
	acc, err := setup.Found(bytes.TrimSuffix(set, []byte("\n")), setup.Openings{}, "2026-11-17T09:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	want := []setup.CardJudge{{Name: "review", Version: jev.Version, Hash: review}, {Name: "spare", Version: jev.Version, Hash: spare}}
	got := acc.CardJudges()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("CardJudges = %+v, want %+v", got, want)
	}
	bad := filepath.Join(a.dir, "bad.json")
	if err := os.WriteFile(bad, []byte(strings.Replace(judgeConfig("bad", "x"), "h.example", "https://h.example", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), "card", "--default", "--judge", bad); r.code != cli.ExitTool || r.stderr != "error judge config-host\n" {
		t.Fatalf("a config that does not hold: %v", r)
	}
	if r := run(t, env(), "card", "--default", "--judge", filepath.Join(a.dir, "none.json")); r.code != cli.ExitTool || r.stderr != "error file judge\n" {
		t.Fatalf("no config: %v", r)
	}
}

// TestTheWindowTakesOnlyTheCardsConfigs starts the window with a config whose instruction differs by
// a byte, with no --judge, with one config the card lists too few, with a config the card does not
// name, and on a card whose version for review is not this adapter's: each prints error founding
// judge, starts nothing and leaves the journal unfounded.
func TestTheWindowTakesOnlyTheCardsConfigs(t *testing.T) {
	t.Parallel()
	a := newJudgedAccord(t)
	other := filepath.Join(a.dir, "other.json")
	if err := os.WriteFile(other, []byte(judgeConfig("review", "Block a patch that deletes data!")), 0o600); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(a.dir, "extra.json")
	if err := os.WriteFile(extra, []byte(judgeConfig("extra", "Block nothing.")), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--openings", a.openings(),
		"--socket", a.socket(), "--journal", a.journal()}
	for name, judges := range map[string][]string{
		"another instruction": {"--judge", other, "--judge", a.config("spare")},
		"no --judge":          nil,
		"one config":          {"--judge", a.config("review")},
		"a config not named":  {"--judge", a.config("review"), "--judge", a.config("spare"), "--judge", extra},
	} {
		r := run(t, env(), append(append([]string(nil), base...), judges...)...)
		if r.code != cli.ExitTool || r.stderr != "error founding judge\n" {
			t.Fatalf("%s: %v", name, r)
		}
		jn, err := journal.Open(a.journal(), &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		rec, err := jn.Read()
		_ = jn.Close()
		if err != nil || !rec.Empty {
			t.Fatalf("%s: the journal: %+v, %v", name, rec, err)
		}
		if b, err := os.ReadFile(filepath.Join(a.journal(), journal.FileAccord)); err != nil || len(b) != 0 {
			t.Fatalf("%s: the accord file: %q, %v", name, b, err)
		}
		if _, err := os.Stat(filepath.Join(a.journal(), journal.FileReceipts)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: receipts.jsonl: %v", name, err)
		}
	}
	// A card whose version for review is not jev.Version, founded anew.
	card := strings.Replace(a.cards, `"version":"`+jev.Version+`"`, `"version":"9.9.9"`, 1)
	_, founding := refoundJudged(t, a, card)
	r := run(t, env(), "window", "--key", a.key("mediator"), "--founding", founding, "--openings", a.openings(), "--socket", a.socket(),
		"--journal", filepath.Join(a.dir, "j2"), "--judge", a.config("review"), "--judge", a.config("spare"))
	if r.code != cli.ExitTool || r.stderr != "error founding judge\n" {
		t.Fatalf("another version: %v", r)
	}
}

// refoundJudged founds the judged Accord's declarations and bindings again under card, and returns
// the new Accord id and the founding file's path.
func refoundJudged(t *testing.T, a *judgedAccord, card string) (string, string) {
	t.Helper()
	cardPath := filepath.Join(a.dir, "card2.json")
	if err := os.WriteFile(cardPath, []byte(card), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"founding"}
	for _, p := range []string{"proposer", "applier", "decider"} {
		args = append(args, "--seal", filepath.Join(a.dir, p+".seal"), "--binding", filepath.Join(a.dir, p+".binding"))
	}
	args = append(args, "--policy", filepath.Join(a.dir, "policy.json"), "--mediator", filepath.Join(a.dir, "mediator.json"), "--card", cardPath)
	r := must(t, args...)
	path := filepath.Join(a.dir, "founding2.json")
	if err := os.WriteFile(path, []byte(r.stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	id, _ := canonical.TypedHash([]byte(strings.TrimSuffix(r.stdout, "\n")))
	return id, path
}

// TestTheWindowSaysWhereContentMayGo starts the window with the judges' key and without: after its
// operator line it prints review's kinds, host and model, or that review has no key, and that spare,
// which no term lists, sends nothing. With no key a patch is asked and review's line is none, no-key
// (docs/adr/0009-who-decided.md).
func TestTheWindowSaysWhereContentMayGo(t *testing.T) {
	t.Parallel()
	a := newJudgedAccord(t)
	keyed := &keyEnv{key: plantedJudgeKey}
	w := a.startJudged(t, keyed.Getenv, "review", "spare")
	lines := strings.Split(strings.TrimSuffix(w.stderr.String(), "\n"), "\n")
	want := []string{"operator " + a.refs["operator"], "judge review may send kinds patch, proposal to h.example, model m",
		"judge spare sends nothing: no term lists it"}
	if len(lines) != 5 || strings.Join(lines[2:], "\n") != strings.Join(want, "\n") {
		t.Fatalf("the start-up lines:\n%s", w.stderr.String())
	}
	w.stop(t)

	a.startJudged(t, (&keyEnv{}).Getenv, "review", "spare")
	lines = strings.Split(strings.TrimSuffix(a.window.stderr.String(), "\n"), "\n")
	want[1] = "judge review has no key in PLACKET_JEV_KEY: it sends nothing, and says none"
	if len(lines) < 5 || strings.Join(lines[2:5], "\n") != strings.Join(want, "\n") {
		t.Fatalf("the start-up lines with no key:\n%s", a.window.stderr.String())
	}
	r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "patch", "--text", "Drop the table.")...)
	if r.code != cli.ExitOpen {
		t.Fatalf("the patch: %v", r)
	}
	id := judgedReceipt(t, a)
	b, err := os.ReadFile(filepath.Join(a.journal(), journal.DirJudged, strings.TrimPrefix(id, "sha256:")+".jsonl"))
	if err != nil || string(b) != `{"error":"no-key","judge":"review","outcome":"none","use":"decide"}`+"\n" {
		t.Fatalf("the judged file: %q, %v", b, err)
	}
}

// judgedReceipt is the id of the one judged receipt of the Accord's journal.
func judgedReceipt(t *testing.T, a *judgedAccord) string {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(a.journal(), journal.DirJudged))
	if err != nil || len(es) != 1 {
		t.Fatalf("judged/: %v, %v", es, err)
	}
	return "sha256:" + strings.TrimSuffix(es[0].Name(), ".jsonl")
}

// TestJudgedIsDroppable gives a judged patch, then removes judged/: verify --founding and log print
// what they printed before, and the window restarts as before (docs/adr/0009-who-decided.md).
func TestJudgedIsDroppable(t *testing.T) {
	t.Parallel()
	a := newJudgedAccord(t)
	w := a.startJudged(t, nil, "review", "spare")
	if r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "patch", "--text", "Drop the table.")...); r.code != cli.ExitOpen {
		t.Fatalf("the patch: %v", r)
	}
	id := judgedReceipt(t, a)
	info, err := os.Stat(filepath.Join(a.journal(), journal.DirJudged, strings.TrimPrefix(id, "sha256:")+".jsonl"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the judged file: %v", err)
	}
	w.stop(t)
	read := func() (string, string) {
		v := must(t, "verify", "--accord", a.id, "--founding", filepath.Join(a.dir, "founding.json"), "--receipts", a.journal())
		l := must(t, "log", "--mediator", a.refs["mediator"], "--receipts", a.journal())
		return v.stdout + v.stderr, l.stdout + l.stderr
	}
	verified, logged := read()
	if err := os.RemoveAll(filepath.Join(a.journal(), journal.DirJudged)); err != nil {
		t.Fatal(err)
	}
	if v, l := read(); v != verified || l != logged {
		t.Fatalf("without judged/:\n%s\n%s\nwant\n%s\n%s", v, l, verified, logged)
	}
	before := a.receipts(t)
	a.startJudged(t, nil, "review", "spare")
	if got := a.receipts(t); !bytes.HasPrefix(got, before) {
		t.Fatal("the restart changed the receipts")
	}
}

// TestTheKeyIsReadOnceAndWrittenNowhere plants the judges' key in the environment and starts the
// window twice, around a give and a fetch: each start reads jev.KeyEnv once and nothing else of the
// environment, and no file of the journal or of a member's state directory holds the key.
func TestTheKeyIsReadOnceAndWrittenNowhere(t *testing.T) {
	t.Parallel()
	a := newJudgedAccord(t)
	k := &keyEnv{key: plantedJudgeKey}
	w := a.startJudged(t, k.Getenv, "review", "spare")
	// A note no judge reads, so that nothing reaches h.example.
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "comment", "--text", "A comment.")...)
	must(t, a.as("proposer", "get", "--keep")...)
	w.stop(t)
	a.startJudged(t, k.Getenv, "review", "spare")
	a.window.stop(t)
	k.mu.Lock()
	reads, other := k.reads, k.other
	k.mu.Unlock()
	if reads != 2 || len(other) != 0 {
		t.Fatalf("the key was read %d times over two starts, and %v besides", reads, other)
	}
	for _, dir := range []string{a.journal(), filepath.Join(a.dir, "proposer.key.state"), filepath.Join(a.dir, "decider.key.state")} {
		_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
			if err == nil && fi.Mode().IsRegular() {
				if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("planted")) {
					t.Errorf("%s holds the key", p)
				}
			}
			return nil
		})
	}
}

// TestACloseNeedsNoJudge closes a judged Accord's journal with no --judge, and starts the closed
// window with none: the close exits 0, and the closed window prints its closed start line and no
// judge line, and neither reads the key (close-1, docs/adr/0009-who-decided.md).
func TestACloseNeedsNoJudge(t *testing.T) {
	t.Parallel()
	a := newJudgedAccord(t)
	w := a.startJudged(t, nil, "review", "spare")
	w.stop(t)
	if err := os.Remove(a.socket()); err != nil {
		t.Fatal(err)
	}
	k := &keyEnv{key: plantedJudgeKey}
	e := env()
	e.Getenv = k.Getenv
	c := run(t, e, a.closeArgs()...)
	if c.code != 0 || strings.Contains(c.stderr, "judge") {
		t.Fatalf("the close: %v", c)
	}
	closeID := strings.Fields(c.lines()[0])[1]
	a.startJudged(t, nil)
	lines := strings.Split(strings.TrimSuffix(a.window.stderr.String(), "\n"), "\n")
	if lines[0] != "accord "+a.id+" closed "+closeID || len(lines) != 3 || strings.Contains(a.window.stderr.String(), "judge") {
		t.Fatalf("the closed start:\n%s", a.window.stderr.String())
	}
	if k.reads != 0 {
		t.Fatalf("the close read the key %d times", k.reads)
	}
}
