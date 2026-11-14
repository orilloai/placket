package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/core/composition"
)

// What the commands print with --json (spec/protocol.md, section 11: commands-1 to commands-4).

// object reads what a command printed with --json: exactly one JSON object and a line feed on
// standard output, whose exit is the command's exit code.
func object(t testing.TB, r result) map[string]json.RawMessage {
	t.Helper()
	if !strings.HasSuffix(r.stdout, "\n") || strings.Count(r.stdout, "\n") != 1 {
		t.Fatalf("not one line on standard output: %v", r)
	}
	d := json.NewDecoder(strings.NewReader(r.stdout))
	var o map[string]json.RawMessage
	if err := d.Decode(&o); err != nil || o == nil || d.More() {
		t.Fatalf("not one JSON object: %v", r)
	}
	if got := string(o["exit"]); got != strconv.Itoa(r.code) {
		t.Fatalf("exit %s, and the command exited %d: %v", got, r.code, r)
	}
	return o
}

// sub reads a member that is an object.
func sub(t testing.TB, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var o map[string]json.RawMessage
	if err := json.Unmarshal(raw, &o); err != nil || o == nil {
		t.Fatalf("not an object: %s", raw)
	}
	return o
}

// members checks that the object has exactly the members named.
func members(t testing.TB, o map[string]json.RawMessage, want ...string) {
	t.Helper()
	var got []string
	for k := range o {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("members %v, want %v", got, want)
	}
}

// is checks that a member holds the JSON value want.
func is(t testing.TB, o map[string]json.RawMessage, name, want string) {
	t.Helper()
	if !sameJSON(t, string(o[name]), want) {
		t.Fatalf("%s is %s, want %s", name, o[name], want)
	}
}

// quote is s as a JSON string.
func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

// outcomeIs checks an outcome object: its word, its reason, and a receipt whose id it names.
func outcomeIs(t testing.TB, raw json.RawMessage, word, reason string) map[string]json.RawMessage {
	t.Helper()
	o := sub(t, raw)
	members(t, o, "word", "reason", "receiptId", "receipt")
	is(t, o, "word", quote(word))
	is(t, o, "reason", quote(reason))
	if !strings.HasPrefix(str(o["receiptId"]), "sha256:") || len(o["receipt"]) == 0 || o["receipt"][0] != '{' {
		t.Fatalf("outcome %s", raw)
	}
	return o
}

// journalLines are the lines of the journal's receipts.jsonl.
func journalLines(t testing.TB, a *accord) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(string(a.receipts(t)), "\n"), "\n")
}

// TestJSONAGiveThatPasses gives a note that passes with --json: standard output is one object, the
// outcome pass ok with the receipt the journal holds, and standard error the line give prints
// without --json (commands-1).
func TestJSONAGiveThatPasses(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	r := run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello", "--json")...)
	if r.code != 0 {
		t.Fatalf("give: %v", r)
	}
	o := object(t, r)
	members(t, o, "exit", "outcome")
	out := outcomeIs(t, o["outcome"], "pass", "ok")
	j := journalLines(t, a)
	if len(j) != 1 || !sameJSON(t, string(out["receipt"]), j[0]) {
		t.Fatalf("receipt %s, journal %v", out["receipt"], j)
	}
	if r.stderr != "pass ok "+str(out["receiptId"])+"\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
	plain := run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello")...)
	if plain.code != 0 || plain.stdout != journalLines(t, a)[1]+"\n" {
		t.Fatalf("without --json: %v", plain)
	}
}

// TestJSONARefusedCommand runs a stranger's give, a give with no flags, a command that does not
// exist, key with no subcommand, a get into a directory that is not there and a give under another
// Accord, each with --json: each prints its refusal's object, and standard error as without --json
// (commands-1).
func TestJSONARefusedCommand(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	for _, c := range []struct {
		args           []string
		stdout, stderr string
		code           int
	}{
		{a.as("stranger", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello", "--json"),
			`{"exit":2,"rejection":"not-a-member"}`, "rejected not-a-member\n", 2},
		{[]string{"give", "--json"}, `{"error":"usage","exit":3}`, "usage: placket give: --key is required\nerror usage\n", 3},
		{[]string{"nosuch", "--json"}, `{"error":"usage","exit":3}`, "", 3},
		{[]string{"key", "--json"}, `{"error":"usage","exit":3}`, "", 3},
		{a.as("bob", "get", "--out", filepath.Join(a.dir, "nowhere"), "--json"), `{"error":"file","exit":3}`, "error file\n", 3},
		{append(a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello", "--json"), "--accord", "sha256:"+strings.Repeat("a", 64)),
			`{"door":"wrong-accord","exit":3}`, "door wrong-accord\n", 3},
	} {
		r := run(t, env(), c.args...)
		if r.code != c.code || r.stdout != c.stdout+"\n" || (c.stderr != "" && r.stderr != c.stderr) {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
}

// TestJSONAnAskLeftOpen gives a proposal the terms ask about, with --json and no wait: the object
// holds the ask and the question's id, exit 4, and result prints the open closing (commands-2).
func TestJSONAnAskLeftOpen(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Add an index.", "--json")...)
	if r.code != 4 {
		t.Fatalf("give: %v", r)
	}
	o := object(t, r)
	members(t, o, "exit", "outcome", "question")
	out := outcomeIs(t, o["outcome"], "ask", "maybe")
	q := str(out["receiptId"])
	is(t, o, "question", quote(q))
	res := run(t, env(), a.as("proposer", "result", "--question", q, "--json")...)
	if res.code != 4 || res.stdout != `{"closing":{"word":"open"},"exit":4,"question":`+quote(q)+"}\n" || res.stderr != "open\n" {
		t.Fatalf("result: %v", res)
	}
}

// TestJSONWaitsThatEndWithAnAcceptAndAChange waits with --json on two proposals: the first accepted
// and the second changed, each answer with --json (commands-2).
func TestJSONWaitsThatEndWithAnAcceptAndAChange(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q, w := a.ask(t, "Rename the column total to amount.", "billing", "--json")
	ans := run(t, env(), a.as("decider", "answer", "--question", q, "--json")...)
	if ans.code != 0 {
		t.Fatalf("answer: %v", ans)
	}
	ao := object(t, ans)
	members(t, ao, "exit", "question", "outcome", "release")
	is(t, ao, "question", quote(q))
	answered := outcomeIs(t, ao["outcome"], "pass", "ok")
	released := outcomeIs(t, ao["release"], "released", "ok")
	r := w.end(t)
	if r.code != 0 {
		t.Fatalf("the wait: %v", r)
	}
	o := object(t, r)
	members(t, o, "exit", "outcome", "question", "closing", "release", "fetched", "answer")
	outcomeIs(t, o["outcome"], "ask", "maybe")
	is(t, o, "question", quote(q))
	c := sub(t, o["closing"])
	members(t, c, "word", "receiptId", "receipt")
	is(t, c, "word", `"accepted"`)
	is(t, c, "receiptId", string(answered["receiptId"]))
	is(t, c, "receipt", string(answered["receipt"]))
	is(t, sub(t, o["release"]), "receiptId", string(released["receiptId"]))
	is(t, o, "fetched", "true")
	d := sub(t, o["answer"])
	members(t, d, "receiptId", "receipt", "messageId", "kind", "salt", "re", "text")
	is(t, d, "kind", `"answer"`)
	is(t, d, "re", quote(q))
	is(t, d, "text", `"accepted"`)
	is(t, d, "receiptId", string(answered["receiptId"]))

	q, w = a.ask(t, "Drop the column notes.", "billing", "--json")
	ch := run(t, env(), a.as("decider", "change", "--question", q, "--text", "Keep notes, hidden.", "--json")...)
	if ch.code != 0 {
		t.Fatalf("change: %v", ch)
	}
	co := object(t, ch)
	members(t, co, "exit", "question", "outcome", "release")
	outcomeIs(t, co["outcome"], "pass", "ok")
	outcomeIs(t, co["release"], "released", "ok")
	r = w.end(t)
	if r.code != 1 {
		t.Fatalf("the wait: %v", r)
	}
	o = object(t, r)
	is(t, sub(t, o["closing"]), "word", `"changed"`)
	is(t, o, "fetched", "true")
	d = sub(t, o["answer"])
	is(t, d, "kind", `"changed-version"`)
	is(t, d, "text", `"Keep notes, hidden."`)
}

// TestJSONADeclineAndItsComment declines an open question with a comment and --json: the object
// holds the decline's block and the comment that crossed; result then prints the closing declined,
// exit 1 (commands-2).
func TestJSONADeclineAndItsComment(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Delete the table invoices.")
	r := run(t, env(), a.as("decider", "decline", "--question", q, "--comment", "Kept seven years.", "--json")...)
	if r.code != 0 {
		t.Fatalf("decline: %v", r)
	}
	o := object(t, r)
	members(t, o, "exit", "question", "outcome", "comment")
	is(t, o, "question", quote(q))
	declined := outcomeIs(t, o["outcome"], "block", "declined")
	c := sub(t, o["comment"])
	members(t, c, "crossed", "outcome")
	is(t, c, "crossed", "true")
	outcomeIs(t, c["outcome"], "pass", "ok")
	res := run(t, env(), a.as("proposer", "result", "--question", q, "--json")...)
	if res.code != 1 {
		t.Fatalf("result: %v", res)
	}
	ro := object(t, res)
	members(t, ro, "exit", "question", "closing")
	cl := sub(t, ro["closing"])
	is(t, cl, "word", `"declined"`)
	is(t, cl, "receiptId", string(declined["receiptId"]))
}

// TestJSONAFounding runs key, declare, bind and founding with --json: each object holds what the
// command prints without --json, and founding's the Accord id and the card hash it prints on
// standard error (commands-3).
func TestJSONAFounding(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	k := filepath.Join(a.dir, "new.key")
	r := run(t, env(), "key", "new", "--out", k, "--json")
	ref := strings.TrimSpace(must(t, "key", "ref", "--key", k).stdout)
	if r.code != 0 || r.stdout != `{"actor":`+quote(ref)+`,"exit":0}`+"\n" {
		t.Fatalf("key new: %v", r)
	}
	if r := run(t, env(), "key", "ref", "--key", k, "--json"); r.stdout != `{"actor":`+quote(ref)+`,"exit":0}`+"\n" {
		t.Fatalf("key ref: %v", r)
	}
	declare := []string{"declare", "--key", a.key("ann"), "--terms", filepath.Join(a.dir, "ann.terms")}
	o := object(t, run(t, env(), append(declare, "--json")...))
	members(t, o, "exit", "declaration")
	is(t, o, "declaration", must(t, declare...).stdout)
	bind := []string{"bind", "--key", a.key("principal"), "--actor", a.refs["ann"], "--role", "peer", "--mediator", a.refs["mediator"], "--until", "2099-01-01T00:00:00Z"}
	o = object(t, run(t, env(), append(bind, "--json")...))
	members(t, o, "exit", "binding")
	is(t, o, "binding", must(t, bind...).stdout)
	founding := []string{"founding", "--policy", filepath.Join(a.dir, "policy.json"), "--mediator", filepath.Join(a.dir, "mediator.json"),
		"--card", filepath.Join(a.dir, "card.json"), "--declaration", filepath.Join(a.dir, "ann.declaration"), "--binding", filepath.Join(a.dir, "ann.binding"),
		"--declaration", filepath.Join(a.dir, "bob.declaration"), "--binding", filepath.Join(a.dir, "bob.binding")}
	plain := must(t, founding...)
	r = run(t, env(), append(founding, "--json")...)
	o = object(t, r)
	members(t, o, "exit", "founding", "accord", "card")
	is(t, o, "founding", plain.stdout)
	is(t, o, "accord", quote(a.id))
	is(t, o, "card", quote(a.card))
	if r.stderr != plain.stderr || r.code != 0 {
		t.Fatalf("founding: %v", r)
	}
}

// TestJSONAnEmptyQueueAndADelivery fetches with --json from an empty queue, then a note, then a
// note into a directory; and an answerer lists the open questions with --json (commands-3).
func TestJSONAnEmptyQueueAndADelivery(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	r := run(t, env(), a.as("bob", "get", "--json")...)
	if r.code != 0 || r.stdout != `{"exit":0}`+"\n" || r.stderr != "no delivery\n" {
		t.Fatalf("an empty queue: %v", r)
	}
	gave := object(t, run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello", "--json")...))
	id := str(sub(t, gave["outcome"])["receiptId"])
	r = run(t, env(), a.as("bob", "get", "--json")...)
	o := object(t, r)
	members(t, o, "exit", "delivery")
	d := sub(t, o["delivery"])
	members(t, d, "receiptId", "receipt", "messageId", "kind", "salt", "text")
	is(t, d, "receiptId", quote(id))
	is(t, d, "kind", `"note"`)
	is(t, d, "text", `"hello"`)
	if !strings.HasPrefix(r.stderr, "delivery "+id+" kind note messageId "+str(d["messageId"])+" salt "+str(d["salt"])+"\n") {
		t.Fatalf("stderr %q", r.stderr)
	}

	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "again")...)
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	r = run(t, env(), a.as("bob", "get", "--out", out, "--json")...)
	o = object(t, r)
	members(t, o, "exit", "delivery", "path")
	d = sub(t, o["delivery"])
	members(t, d, "receiptId", "receipt", "messageId", "kind", "salt")
	path := filepath.Join(out, strings.TrimPrefix(str(d["receiptId"]), "sha256:")+".note")
	is(t, o, "path", quote(path))
	if b, err := os.ReadFile(path); err != nil || string(b) != "again" {
		t.Fatalf("the body: %q %v", b, err)
	}

	p := newAccord(t, "proposer", "applier", "decider")
	p.start(t)
	p.openOne(t, "Add an index.")
	plain := must(t, p.as("decider", "questions")...)
	o = object(t, run(t, env(), p.as("decider", "questions", "--json")...))
	members(t, o, "exit", "questions")
	is(t, o, "questions", "["+strings.TrimSuffix(plain.stdout, "\n")+"]")
}

// TestJSONAWindowStops runs a window with --json, stops it and starts it again: each object holds
// the Accord id, the card hash, the operator and how it started (commands-3).
func TestJSONAWindowStops(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	for _, how := range []string{"founded", "reloaded"} {
		ctx, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		e := env()
		e.Context, e.Ready = ctx, ready
		var stdout bytes.Buffer
		stderr := &syncBuffer{}
		done := make(chan int, 1)
		go func() {
			done <- cli.Run(e, []string{"window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"),
				"--socket", a.socket(), "--journal", a.journal(), "--json"}, strings.NewReader(""), &stdout, stderr)
		}()
		select {
		case <-ready:
		case code := <-done:
			cancel()
			t.Fatalf("the window exited %d: %s", code, stderr.String())
		case <-time.After(10 * time.Second):
			cancel()
			t.Fatal("the window did not start")
		}
		cancel()
		var code int
		select {
		case code = <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the window did not stop")
		}
		want := `{"accord":` + quote(a.id) + `,"card":` + quote(a.card) + `,"exit":0,"operator":` + quote(a.refs["operator"]) + `,"started":"` + how + `"}` + "\n"
		if code != 0 || stdout.String() != want {
			t.Fatalf("%s: exit %d, stdout %q, want %q", how, code, stdout.String(), want)
		}
	}
}

// TestJSONAShownReceiptAndAVerifiedJournal shows and verifies a journal of one pass with --json:
// each object holds its claim line and the scope line, and what the command prints on standard
// output without --json; standard error is as without --json (commands-4).
func TestJSONAShownReceiptAndAVerifiedJournal(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	gave := object(t, run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello", "--json")...))
	id := str(sub(t, gave["outcome"])["receiptId"])
	p := filepath.Join(a.dir, "one.receipt")
	if err := os.WriteFile(p, a.receipts(t), 0o600); err != nil {
		t.Fatal(err)
	}
	plain := must(t, "show", "--receipt", p)
	r := run(t, env(), "show", "--receipt", p, "--json")
	o := object(t, r)
	members(t, o, "exit", "verified", "receiptId", "word", "reason", "kind", "at", "from", "to", "module", "claim", "notClaimed", "shown")
	l := plain.lines()
	at := strings.Fields(l[1])[3]
	for name, want := range map[string]string{
		"verified": "false", "receiptId": quote(id), "word": `"pass"`, "reason": `"ok"`, "kind": `"note"`, "at": quote(at),
		"from": quote(a.refs["ann"]), "to": quote(a.refs["bob"]), "module": quote("module: named by card " + a.card + "."),
		"claim": quote(cli.ClaimPass), "notClaimed": quote(cli.Scope),
	} {
		is(t, o, name, want)
	}
	shown, _ := json.Marshal(strings.Split(strings.TrimSuffix(plain.stdout, "\n"), "\n"))
	is(t, o, "shown", string(shown))
	if r.stderr != plain.stderr {
		t.Fatalf("stderr %q, want %q", r.stderr, plain.stderr)
	}

	verify := []string{"verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--founding", filepath.Join(a.dir, "founding.json")}
	plain = must(t, verify...)
	r = run(t, env(), append(verify, "--json")...)
	o = object(t, r)
	members(t, o, "exit", "verified", "mediator", "accord", "card", "operator", "runs", "module", "checks", "counts",
		"questionsClosed", "questionsOpen", "releases", "blockedReleases", "predicates", "claim", "notClaimed")
	for name, want := range map[string]string{
		"verified": "1", "mediator": quote(a.refs["mediator"]), "accord": quote(a.id), "card": quote(a.card),
		"operator": quote(a.refs["operator"]), "runs": `"process"`,
		"module": `{"hash":"` + composition.KernelHash() + `","version":"` + composition.KernelVersion + `"}`, "checks": "[]",
		"counts": `{"ask":0,"block":0,"pass":1}`, "questionsClosed": "0", "questionsOpen": "0", "releases": "0", "blockedReleases": "0",
		"predicates": "[" + strings.TrimSuffix(plain.stdout, "\n") + "]", "claim": quote(cli.ClaimVerified), "notClaimed": quote(cli.Scope),
	} {
		is(t, o, name, want)
	}
	if r.stderr != plain.stderr {
		t.Fatalf("stderr %q, want %q", r.stderr, plain.stderr)
	}
	o = object(t, run(t, env(), "verify", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--json"))
	members(t, o, "exit", "verified", "mediator", "accord", "card", "counts",
		"questionsClosed", "questionsOpen", "releases", "blockedReleases", "predicates", "claim", "notClaimed")

	bad := []string{"verify", "--mediator", a.refs["ann"], "--receipts", a.journal()}
	plain = run(t, env(), bad...)
	r = run(t, env(), append(bad, "--json")...)
	f := strings.Fields(plain.stderr)
	if len(f) != 6 || f[4] != "line" {
		t.Fatalf("verify under another key, without --json: %v", plain)
	}
	detail, _ := json.Marshal([]string{f[2], f[3], f[4] + " " + f[5]})
	if plain.code != 3 || r.code != 3 || r.stderr != plain.stderr || r.stdout != `{"detail":`+string(detail)+`,"error":"verify","exit":3}`+"\n" {
		t.Fatalf("verify under another key: %v, without --json %v", r, plain)
	}
}

// TestJSONTheLogAndReconciliation runs log and reconcile with --json over a journal of one pass: each
// object holds what the command prints without --json; log as CSV with --json is error format
// (commands-4).
func TestJSONTheLogAndReconciliation(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "hello", "--keep")...)
	log := []string{"log", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--kept", a.state("ann")}
	plain := must(t, log...)
	r := run(t, env(), append(log, "--json")...)
	if r.code != 0 || r.stdout != `{"exit":0,"log":`+strings.TrimSuffix(plain.stdout, "\n")+"}\n" || r.stderr != plain.stderr {
		t.Fatalf("log: %v, without --json %v", r, plain)
	}
	var lg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(plain.stdout), &lg); err != nil {
		t.Fatal(err)
	}
	is(t, lg, "claim", quote(cli.ClaimVerified))
	is(t, lg, "notClaimed", quote(cli.Scope))
	if r := run(t, env(), append(log, "--format", "csv", "--json")...); r.code != 3 || r.stdout != `{"error":"format","exit":3}`+"\n" {
		t.Fatalf("log as CSV: %v", r)
	}

	rec := []string{"reconcile", "--mediator", a.refs["mediator"], "--accord", a.id, "--card", a.card, "--receipts", a.journal(), "--kept", a.state("ann")}
	plain = must(t, rec...)
	r = run(t, env(), append(rec, "--json")...)
	o := object(t, r)
	members(t, o, "exit", "removed", "unrecorded", "missing")
	var want map[string]json.RawMessage
	if err := json.Unmarshal([]byte(plain.stdout), &want); err != nil {
		t.Fatal(err)
	}
	delete(o, "exit")
	if !reflect.DeepEqual(o, want) || r.stderr != plain.stderr {
		t.Fatalf("reconcile: %v, without --json %v", r, plain)
	}
}
