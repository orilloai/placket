package cli_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
)

// seeds is every spelling of every seed of the Accord's key files.
func (a *accord) seeds(t testing.TB) []string {
	t.Helper()
	var out []string
	for name := range a.refs {
		b, err := os.ReadFile(a.key(name))
		if err != nil {
			t.Fatal(err)
		}
		h := strings.TrimSpace(string(b))
		raw, _ := hex.DecodeString(h)
		out = append(out, h[:32], h[32:], strings.ToUpper(h[:32]), string(raw[:12]))
	}
	return out
}

// TestNoCommandPrintsASeed runs every command, with every flag that takes a key file, and finds no
// seed in anything printed; the windows' lines hold no body, no salt and no question's part; and
// questions shows a part only to a key among the question's answerers (secrets-never-print).
func TestNoCommandPrintsASeed(t *testing.T) {
	t.Parallel()
	s := six(t)
	q := s.openOne(t, "Rename the table orders.")
	seeds := s.seeds(t)
	var outs []result
	add := func(r result) { outs = append(outs, r) }
	for name := range s.refs {
		add(must(t, "key", "ref", "--key", s.key(name)))
	}
	add(must(t, "key", "new", "--out", filepath.Join(s.dir, "new.key")))
	add(must(t, "declare", "--key", s.key("proposer"), "--terms", filepath.Join(s.dir, "proposer.terms")))
	add(must(t, "bind", "--key", s.key("principal"), "--actor", s.refs["proposer"], "--role", "proposer", "--mediator", s.refs["mediator"], "--until", "2099-01-01T00:00:00Z", "--policy", filepath.Join(s.dir, "policy.json")))
	decider := must(t, s.as("decider", "questions")...)
	add(decider)
	for _, who := range []string{"proposer", "applier"} {
		r := must(t, s.as(who, "questions")...)
		if r.stdout != "" {
			t.Fatalf("%s's questions: %v", who, r)
		}
		add(r)
	}
	if r := run(t, env(), s.as("stranger", "questions")...); r.code != 2 || r.stderr != "rejected not-a-member\n" {
		t.Fatalf("a stranger's questions: %v", r)
	}
	var shown struct{ ID, Message, Form string }
	if err := json.Unmarshal([]byte(decider.stdout), &shown); err != nil || shown.ID != q || shown.Message != "Rename the table orders." || shown.Form != "text" {
		t.Fatalf("the decider's questions: %v", decider)
	}
	// A raw part is shown by its form and size alone, and a data part as its canonical JSON.
	parts := map[string]string{"raw": "\x00\x01\x02 a raw part", "data": `{"b": [1, 2], "a": "x\u00e9"}`}
	asked := map[string]string{}
	for _, form := range []string{"raw", "data"} {
		f := filepath.Join(s.dir, form+".part")
		if err := os.WriteFile(f, []byte(parts[form]), 0o600); err != nil {
			t.Fatal(err)
		}
		r := run(t, env(), s.as("proposer", "give", "--to", s.refs["applier"], "--kind", "proposal", "--file", f, "--as", form)...)
		if r.code != 4 {
			t.Fatalf("give --as %s: %v", form, r)
		}
		asked[last(r.stdout)] = form
	}
	r := must(t, s.as("decider", "questions")...)
	add(r)
	seen := 0
	for _, l := range strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n") {
		var o struct{ ID, Deadline string }
		if err := json.Unmarshal([]byte(l), &o); err != nil {
			t.Fatal(err)
		}
		head := `{"asker":"` + s.refs["proposer"] + `","deadline":"` + o.Deadline + `","form":"`
		var want string
		switch asked[o.ID] {
		case "raw":
			want = head + `raw","id":"` + o.ID + `","kind":"proposal","size":14}`
		case "data":
			want = head + `data","id":"` + o.ID + `","kind":"proposal","message":"{\"a\":\"xé\",\"b\":[1,2]}"}`
		default:
			continue
		}
		seen++
		if l != want {
			t.Fatalf("the decider's question of a %s part:\n got %s\nwant %s", asked[o.ID], l, want)
		}
	}
	if seen != 2 {
		t.Fatalf("the decider's questions: %v", r)
	}
	for q := range asked {
		must(t, s.as("proposer", "cancel", "--question", q)...)
	}
	add(must(t, s.as("proposer", "result", "--question", s.accepted)...))
	add(run(t, env(), s.as("proposer", "give", "--to", s.refs["applier"], "--kind", "proposal", "--text", "x")...))
	add(must(t, s.as("proposer", "cancel", "--question", q)...))
	add(must(t, s.as("applier", "get", "--keep")...))
	add(must(t, "verify", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--founding", filepath.Join(s.dir, "founding.json")))
	add(must(t, "log", "--mediator", s.refs["mediator"], "--receipts", s.journal(), "--words", "--kept", s.state("proposer")))
	for _, r := range append(outs, s.steps["answer"], s.steps["change"], s.steps["decline"], s.steps["accepted"], s.steps["changed"]) {
		for _, seed := range seeds {
			if strings.Contains(r.stdout+r.stderr, seed) {
				t.Fatalf("a seed is printed: %v", r)
			}
		}
	}
	// The windows: no body, salt or part.
	lines := s.window.stderr.String()
	var secrets []string
	for _, who := range []string{"proposer", "applier", "decider"} {
		for _, k := range keptLines(t, s.state(who)) {
			if v := str(k["salt"]); v != "" {
				secrets = append(secrets, v)
			}
			if v := str(k["text"]); v != "" {
				secrets = append(secrets, v)
			}
		}
	}
	secrets = append(secrets, "Rename", "Drop the column", "Delete the table", "Someone reads it.", "Kept seven years.", "Keep notes", "a raw part", `"b":[1,2]`)
	for _, x := range secrets {
		if strings.Contains(lines, x) {
			t.Fatalf("the window printed %q:\n%s", x, lines)
		}
	}
	if !strings.Contains(lines, "give ask maybe "+q) || !strings.Contains(lines, "questions\n") {
		t.Fatalf("the window's lines:\n%s", lines)
	}
}

// TestFoundingReproducesAVector splits each founding vector that founds into the policy, mediator,
// card, declaration and binding files, and runs founding on them at the vector's founding time: it
// prints the vector's Accord id and card hash (setup-contract-5).
func TestFoundingReproducesAVector(t *testing.T) {
	t.Parallel()
	files, _ := filepath.Glob("../../vectors/founding/*.json")
	n := 0
	for _, f := range files {
		b, _ := os.ReadFile(f)
		var v struct {
			FoundedAt string `json:"foundedAt"`
			InputHex  string `json:"inputHex"`
			Expect    struct{ Accord, Card string }
		}
		if json.Unmarshal(b, &v) != nil || v.Expect.Accord == "" {
			continue
		}
		set, _ := hex.DecodeString(v.InputHex)
		var parts map[string]json.RawMessage
		if err := json.Unmarshal(set, &parts); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		write := func(name string, b []byte) string {
			p := filepath.Join(dir, name)
			_ = os.WriteFile(p, b, 0o600)
			return p
		}
		args := []string{"founding", "--policy", write("policy", parts["policy"]), "--mediator", write("mediator", parts["mediator"]), "--card", write("card", parts["card"])}
		for _, list := range []string{"declarations", "bindings"} {
			var l []json.RawMessage
			_ = json.Unmarshal(parts[list], &l)
			// The files are given in reverse: founding puts them in order.
			for i := len(l) - 1; i >= 0; i-- {
				args = append(args, "--"+map[string]string{"declarations": "seal", "bindings": "binding"}[list], write(list+string(rune('a'+i)), l[i]))
			}
		}
		at, _ := time.Parse(time.RFC3339, v.FoundedAt)
		e := env()
		e.Now = func() time.Time { return at }
		r := run(t, e, args...)
		if r.code != 0 || r.stderr != "accord "+v.Expect.Accord+"\ncard "+v.Expect.Card+"\n"+vectorModule(t, parts["card"])+"\n" || strings.TrimSpace(r.stdout) != string(set) {
			t.Fatalf("%s: %v", f, r)
		}
		n++
	}
	if n < 3 {
		t.Fatalf("%d founding vectors found", n)
	}
}

// failingDisk fails every append to receipts.jsonl.
type failingDisk struct{}

func (failingDisk) Append(path string, b []byte) error {
	if filepath.Base(path) == journal.FileReceipts {
		return errors.New("injected")
	}
	return journal.OS.Append(path, b)
}
func (failingDisk) Create(path string, b []byte) error { return journal.OS.Create(path, b) }
func (failingDisk) Remove(path string) error           { return journal.OS.Remove(path) }

// TestAWindowWhoseJournalFails runs a window whose journal cannot take a receipt: the give gets no
// reply, tool error socket, exit 3, and keeps its envelope line; the window exits 3, naming the file
// (journal-synced).
func TestAWindowWhoseJournalFails(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	ready := make(chan struct{})
	e := env()
	e.Disk, e.Ready = failingDisk{}, ready
	done := make(chan result, 1)
	go func() {
		done <- run(t, e, "window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--openings", a.openings(), "--socket", a.socket(), "--journal", a.journal())
	}()
	<-ready
	r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "proposal", "--text", "x")...)
	if r.code != 3 || r.stderr != "error socket\n" {
		t.Fatalf("the give: %v", r)
	}
	envs, _ := os.ReadFile(filepath.Join(a.state("proposer"), "envelopes.jsonl"))
	if strings.Count(string(envs), "\n") != 1 {
		t.Fatalf("envelopes.jsonl: %q", envs)
	}
	w := waiting(done).end(t)
	if w.code != 3 || !strings.HasSuffix(w.stderr, "error journal receipts.jsonl\n") {
		t.Fatalf("the window: %v", w)
	}
}

// vectorModule is the line founding prints after a vector's card, which names the hash the vectors
// were written with, another module than this build's (docs/adr/0007-the-card-and-the-kernel-hash.md).
func vectorModule(t *testing.T, card []byte) string {
	t.Helper()
	var c struct {
		Kernel struct{ Version, Hash string }
	}
	if err := json.Unmarshal(card, &c); err != nil {
		t.Fatal(err)
	}
	return otherLine(c.Kernel.Version, c.Kernel.Hash)
}
