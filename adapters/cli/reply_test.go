package cli_test

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// A fakeWindow listens on a socket and answers every call with the reply it was last given,
// whatever the call asked. It keeps the last call it read.
type fakeWindow struct {
	socket string
	mu     sync.Mutex
	reply  []byte
	last   []byte
}

func newFakeWindow(t testing.TB, dir string) *fakeWindow {
	t.Helper()
	f := &fakeWindow{socket: filepath.Join(dir, "f")}
	l, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			call, _ := bufio.NewReader(c).ReadBytes('\n')
			f.mu.Lock()
			f.last = call
			b := append(append([]byte(nil), f.reply...), '\n')
			f.mu.Unlock()
			_, _ = c.Write(b)
			_ = c.Close()
		}
	}()
	return f
}

// set makes the reply to every call o's canonical bytes.
func (f *fakeWindow) set(t testing.TB, o any) {
	t.Helper()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.reply = c
	f.mu.Unlock()
}

// lastCall is the last call the fake window read, as sent.
func (f *fakeWindow) lastCall() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.last...)
}

// as is the socket flags of a command run by name against the fake window.
func (f *fakeWindow) as(a *accord, name string, args ...string) []string {
	return append(append([]string(nil), args...), "--key", a.key(name), "--socket", f.socket, "--accord", a.id)
}

// fileOf is a file's bytes, or the empty string when there is none.
func fileOf(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// TestTheCommandsReadTheReplyForItsForm answers the commands with replies a window does not send.
// What a reply names is read for its form before it reaches a terminal, a state file or a file: a
// refusal's code, an outcome's reason and receipt id, a release's reason, and a delivery, whose
// receipt must be a receipt to this key under this Accord about this very part, of the kind and id
// the delivery names. A reply that fails is tool error reply, exit 3, and nothing of it is printed,
// recorded or written. A receipt on a terminal is escaped as any JSON of another's is. get --out
// writes only a new file in its directory: never through a link, never over a file there.
func TestTheCommandsReadTheReplyForItsForm(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	gave := must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "a note")...)
	receipt := json.RawMessage(strings.TrimSuffix(gave.stdout, "\n"))
	f := strings.Fields(gave.stderr)
	if len(f) != 3 || f[0] != "pass" || f[1] != "ok" {
		t.Fatalf("give: %v", gave)
	}
	id := f[2]
	must(t, a.as("p2", "get", "--keep")...)
	var delivery map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSuffix(fileOf(t, filepath.Join(a.state("p2"), "kept.jsonl")), "\n")), &delivery); err != nil {
		t.Fatal(err)
	}
	outcome := map[string]any{"verdict": "pass", "reason": "ok", "receiptId": id, "receipt": receipt}
	w := newFakeWindow(t, a.dir)

	// A refusal and an outcome, each naming what no window sends.
	refused := []struct {
		name  string
		args  []string
		reply map[string]any
	}{
		{"a rejection holding an OSC 52 sequence", w.as(a, "p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "x"),
			map[string]any{"ok": false, "rejection": "\x1b]52;c;aGk=\x07"}},
		{"an error holding a line", w.as(a, "p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "x"),
			map[string]any{"ok": false, "error": "journal\nreleased ok"}},
		{"a door code holding a C1 control", w.as(a, "p1", "questions"), map[string]any{"ok": false, "door": "stale\u009b2J"}},
		{"a reason holding an escape", w.as(a, "p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "x"),
			map[string]any{"ok": true, "outcome": map[string]any{"verdict": "pass", "reason": "ok\x1b[2J", "receiptId": id, "receipt": receipt}}},
		{"a receipt id holding an escape", w.as(a, "p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "x"),
			map[string]any{"ok": true, "outcome": map[string]any{"verdict": "pass", "reason": "ok", "receiptId": "sha256:\x1b[2J", "receipt": receipt}}},
		{"a release whose reason holds an escape", w.as(a, "p1", "result", "--question", id),
			map[string]any{"ok": true, "outcome": outcome, "release": map[string]any{"verdict": "block", "reason": "no-match\x1b]52;c;aGk=\x07", "receiptId": id, "receipt": receipt}}},
	}
	for _, c := range refused {
		w.set(t, c.reply)
		r := run(t, terminal(), c.args...)
		if r.code != 3 || r.stderr != "error reply\n" || r.stdout != "" {
			t.Fatalf("%s: %v", c.name, r)
		}
	}

	// A receipt is printed on a terminal as any other party's JSON is.
	shown := map[string]any{}
	if err := json.Unmarshal(receipt, &shown); err != nil {
		t.Fatal(err)
	}
	shown["z"] = "\u009b2J\u202e"
	w.set(t, map[string]any{"ok": true, "outcome": map[string]any{"verdict": "pass", "reason": "ok", "receiptId": id, "receipt": shown}})
	r := run(t, terminal(), w.as(a, "p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "x")...)
	if r.code != 0 || r.stderr != "pass ok "+id+"\n" || !strings.HasSuffix(r.stdout, `,"z":"\u009b2J\u202e"}`+"\n") {
		t.Fatalf("a receipt with a member that holds a C1 control: %v", r)
	}

	// Receipts the mediator's key signs, of this part, as a window could send them.
	st, err := receipts.Decode(receipt)
	if err != nil {
		t.Fatal(err)
	}
	mk, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	sign := func(s receipts.Subject, p receipts.Predicate) (json.RawMessage, string) {
		t.Helper()
		b, v, err := receipts.Build(s, p, mk)
		if err != nil {
			t.Fatal(err)
		}
		return json.RawMessage(b), v.Statement().ID
	}

	// An answer that passed and whose release blocked: answer exits 1, and result's line says the
	// release blocked and exits 1.
	answered := st.Predicate
	answered.Kind = receipts.KindResolution
	answered.By = receipts.By{Decider: receipts.ByAnswer, Answer: "sha256:" + strings.Repeat("a1", 32)}
	resReceipt, resID := sign(receipts.Subject{Name: receipts.NameAnswer, Digest: st.Subject.Digest}, answered)
	blocked := st.Predicate
	blocked.Verdict, blocked.Reason, blocked.By = "block", "no-match", answered.By
	relReceipt, relID := sign(st.Subject, blocked)
	w.set(t, map[string]any{"ok": true,
		"outcome": map[string]any{"verdict": "pass", "reason": "ok", "receiptId": resID, "receipt": resReceipt},
		"release": map[string]any{"verdict": "block", "reason": "no-match", "receiptId": relID, "receipt": relReceipt}})
	r = run(t, env(), w.as(a, "p1", "answer", "--question", id)...)
	if r.code != 1 || r.stderr != "pass ok "+resID+"\nblock no-match "+relID+"\n" {
		t.Fatalf("an answer whose release blocked: %v", r)
	}
	r = run(t, env(), w.as(a, "p1", "result", "--question", id)...)
	if r.code != 1 || r.stderr != "accepted "+resID+" release blocked no-match\nnot fetched\n" {
		t.Fatalf("the result of an answer whose release blocked: %v", r)
	}

	// A delivery that is not what its receipt says.
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	with := func(k string, v any) map[string]any {
		d := map[string]any{}
		for n, x := range delivery {
			d[n] = x
		}
		if k != "" {
			d[k] = v
		}
		return map[string]any{"ok": true, "delivery": d}
	}
	type lie struct {
		name string
		who  string
		rep  map[string]any
	}
	var lies []lie
	add := func(name, who string, k string, v any) {
		lies = append(lies, lie{name, who, with(k, v)})
	}
	add("a kind that is a path", "p2", "kind", "/../../x")
	add("a kind not its receipt's", "p2", "kind", "comment")
	add("a receipt id that is a path", "p2", "receiptId", "sha256:../../x")
	add("a receipt id not its receipt's", "p2", "receiptId", "sha256:"+strings.Repeat("0", 64))
	add("a body not its receipt's", "p2", "text", "another note")
	add("a receipt that is no receipt", "p2", "receipt", map[string]any{"a": 1})
	add("a receipt to another key", "p1", "", nil)
	add("an answer call its receipt names not", "p2", "answer", map[string]any{"action": "accepted"})
	// A receipt of this very part, to this key, but under another Accord.
	elsewhere := st.Predicate
	elsewhere.Accord = "sha256:" + strings.Repeat("0", 64)
	otherReceipt, otherID := sign(st.Subject, elsewhere)
	l := with("receipt", otherReceipt)
	l["delivery"].(map[string]any)["receiptId"] = otherID
	lies = append(lies, lie{"a receipt under another Accord", "p2", l})
	for _, l := range lies {
		before := fileOf(t, filepath.Join(a.state(l.who), "delivered.jsonl"))
		w.set(t, l.rep)
		r := run(t, env(), w.as(a, l.who, "get", "--out", out, "--keep")...)
		if r.code != 3 || r.stderr != "error reply\n" || r.stdout != "" {
			t.Fatalf("%s: %v", l.name, r)
		}
		if got := fileOf(t, filepath.Join(a.state(l.who), "delivered.jsonl")); got != before {
			t.Fatalf("%s: delivered.jsonl %q, was %q", l.name, got, before)
		}
		if es, _ := os.ReadDir(out); len(es) != 0 {
			t.Fatalf("%s: %d files in --out", l.name, len(es))
		}
		if fileOf(t, filepath.Join(a.dir, "x")) != "" {
			t.Fatalf("%s: a file outside --out", l.name)
		}
	}

	// The delivery as it was sent: written to a new file of --out, never through a link or over a file.
	w.set(t, with("", nil))
	path := filepath.Join(out, strings.TrimPrefix(id, "sha256:")+".note")
	victim := filepath.Join(a.dir, "victim")
	if err := os.WriteFile(victim, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	keptBefore := len(keptLines(t, a.state("p2")))
	r = run(t, env(), w.as(a, "p2", "get", "--out", out)...)
	if r.code != 3 || r.stdout != "" || last(r.stderr) != "error file kept" || fileOf(t, victim) != "mine" {
		t.Fatalf("get --out onto a link: %v", r)
	}
	_ = os.Remove(path)
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = run(t, env(), w.as(a, "p2", "get", "--out", out)...)
	if r.code != 3 || r.stdout != "" || last(r.stderr) != "error file kept" || fileOf(t, path) != "mine" {
		t.Fatalf("get --out onto a file: %v", r)
	}
	if k := keptLines(t, a.state("p2")); len(k) != keptBefore+2 || str(k[len(k)-1]["text"]) != "a note" {
		t.Fatalf("get --out that could not write kept %d parts, was %d", len(k), keptBefore)
	}
	_ = os.Remove(path)
	r = run(t, env(), w.as(a, "p2", "get", "--out", out)...)
	if r.code != 0 || r.stdout != path+"\n" || fileOf(t, path) != "a note" {
		t.Fatalf("get --out: %v", r)
	}
}
