package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/mediator"
)

// open gives a proposal without waiting and returns its question.
func (a *accord) openOne(t testing.TB, text string) string {
	t.Helper()
	r := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "proposal", "--text", text, "--keep")...)
	if r.code != 4 {
		t.Fatalf("give: %v", r)
	}
	return last(r.stdout)
}

func (a *accord) receipts(t testing.TB) []byte {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(a.journal(), "receipts.jsonl"))
	return b
}

// TestAnAnswerNeedsItsText refuses a decline with no comment, or an empty one, with tool error
// comment, and a change with no version, an empty one, or a file that is not UTF-8, with tool error
// changed: each exits 3, sends no call, and leaves the question open. A change with a comment gives
// the comment to the asker with re the question; a decline of a closed question is no-open-question,
// exit 2, and gives no comment (RCP-01, RCP-02, RCP-05, RCP-12).
func TestAnAnswerNeedsItsText(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Drop notes.")
	before := a.receipts(t)
	bad := filepath.Join(a.dir, "bad")
	if err := os.WriteFile(bad, []byte{0xff, 0xfe}, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		code string
	}{
		{[]string{"decline", "--question", q}, "comment"},
		{[]string{"decline", "--question", q, "--comment", ""}, "comment"},
		{[]string{"change", "--question", q}, "changed"},
		{[]string{"change", "--question", q, "--text", ""}, "changed"},
		{[]string{"change", "--question", q, "--file", bad}, "changed"},
	} {
		r := run(t, env(), a.as("decider", c.args...)...)
		if r.code != 3 || r.stderr != "error "+c.code+"\n" || r.stdout != "" {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
	if !bytes.Equal(a.receipts(t), before) || a.question(t, "decider") != q {
		t.Fatal("a refused answer sent a call")
	}
	r := must(t, a.as("decider", "change", "--question", q, "--text", "Hide notes.", "--comment", "Someone reads it.")...)
	if l := r.lines(); len(l) != 3 || !strings.HasPrefix(l[0], "pass ok ") || !strings.HasPrefix(l[1], "released ok ") || !strings.HasPrefix(l[2], "pass ok ") {
		t.Fatalf("change: %v", r)
	}
	got := a.drain(t, "proposer")
	comment := regexp.MustCompile(`^delivery ` + receiptID + ` kind comment messageId [0-9a-f-]{36} salt [0-9a-f]{64} re ` + q + `\n$`)
	if len(got) != 2 || !comment.MatchString(got[1].stderr) || got[1].stdout != "Someone reads it.\n" {
		t.Fatalf("the proposer fetched %v", got)
	}
	r = run(t, env(), a.as("decider", "decline", "--question", q, "--comment", "Too late.")...)
	if r.code != 2 || r.stderr != "rejected no-open-question\n" {
		t.Fatalf("a decline of a closed question: %v", r)
	}
	if got := a.drain(t, "proposer"); len(got) != 0 {
		t.Fatalf("a refused decline gave a comment: %v", got)
	}
}

// keptLines reads a state directory's kept.jsonl.
func keptLines(t testing.TB, dir string) []map[string]json.RawMessage {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "kept.jsonl"))
	var out []map[string]json.RawMessage
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		var o map[string]json.RawMessage
		if err := json.Unmarshal([]byte(l), &o); err != nil {
			t.Fatal(err)
		}
		out = append(out, o)
	}
	return out
}

func str(r json.RawMessage) string {
	var s string
	_ = json.Unmarshal(r, &s)
	return s
}

// TestTheAnswerCallTravelsWithTheAnswer accepts one proposal and changes another, and the asker and
// the receiver fetch with --keep: every delivery whose body is the answer or the version carries the
// decider's call beside it, which verifies under the decider's key and says the question, the action
// and the text; the released proposal carries none. No line of receipts.jsonl holds a call, after
// the fetch no file of the journal holds one or the version, and log marks both rows answer_signed
// (answer-call-attached, RCP-17).
func TestTheAnswerCallTravelsWithTheAnswer(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	accepted := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", accepted, "--text", "Yes.")...)
	changed := a.openOne(t, "Drop notes.")
	must(t, a.as("decider", "change", "--question", changed, "--text", "Hide notes.")...)
	held, _ := os.ReadDir(filepath.Join(a.journal(), "held"))
	if len(held) != 4 {
		t.Fatalf("%d held files before the fetch", len(held))
	}
	a.drain(t, "proposer")
	a.drain(t, "applier")
	calls := map[string]string{}
	for _, who := range []string{"proposer", "applier"} {
		for _, k := range keptLines(t, a.state(who)) {
			kind := str(k["kind"])
			if kind == "" {
				continue
			}
			ac, has := k["answerCall"]
			if kind == "proposal" {
				if has {
					t.Fatal("the released proposal carries an answer call")
				}
				continue
			}
			sa, ok := local.VerifyAnswerCall(ac, a.refs["decider"])
			if !has || !ok || sa.Accord != a.id || sa.Question != str(k["re"]) || sa.Answer != str(k["text"]) {
				t.Fatalf("%s's %s: call %s, %+v, %v", who, kind, ac, sa, ok)
			}
			calls[who+" "+kind] = sa.Action
		}
	}
	want := map[string]string{"proposer answer": "accepted", "proposer changed-version": "changed", "applier changed-version": "changed"}
	if len(calls) != len(want) {
		t.Fatalf("calls %v", calls)
	}
	for k, v := range want {
		if calls[k] != v {
			t.Fatalf("calls %v", calls)
		}
	}
	// Each receipt line is a DSSE envelope whose payload is base64: the statement is read decoded.
	var sigs []string
	for _, who := range []string{"proposer", "applier"} {
		for _, k := range keptLines(t, a.state(who)) {
			var c struct{ Sig string }
			if ac, has := k["answerCall"]; has && json.Unmarshal(ac, &c) == nil && c.Sig != "" {
				sigs = append(sigs, c.Sig)
			}
		}
	}
	if len(sigs) != 3 {
		t.Fatalf("%d answer calls kept", len(sigs))
	}
	for i, l := range strings.Split(strings.TrimSuffix(string(a.receipts(t)), "\n"), "\n") {
		var env struct{ Payload string }
		if err := json.Unmarshal([]byte(l), &env); err != nil {
			t.Fatal(err)
		}
		st, err := base64.StdEncoding.DecodeString(env.Payload)
		if err != nil || len(st) == 0 {
			t.Fatalf("receipts.jsonl line %d: payload %v", i+1, err)
		}
		for _, b := range [][]byte{st, []byte(l)} {
			if bytes.Contains(b, []byte("answerCall")) || bytes.Contains(b, []byte(`"call"`)) {
				t.Fatalf("receipts.jsonl line %d holds a call", i+1)
			}
			for _, sig := range sigs {
				if bytes.Contains(b, []byte(sig)) {
					t.Fatalf("receipts.jsonl line %d holds an answer call's signature", i+1)
				}
			}
		}
	}
	_ = filepath.Walk(a.journal(), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			if bytes.Contains(b, []byte("answerCall")) || bytes.Contains(b, []byte("Hide notes.")) {
				t.Fatalf("%s holds the call or the version after the fetch", p)
			}
		}
		return nil
	})
	r := must(t, "log", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--format", "json", "--kept", a.state("proposer"), "--kept", a.state("applier"))
	var lg struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &lg); err != nil || len(lg.Rows) != 2 {
		t.Fatalf("log: %v", r)
	}
	for _, row := range lg.Rows {
		if row["answer_signed"] != "yes" {
			t.Fatalf("a row is not answer_signed: %v", row)
		}
	}
	if r.stderr != "" {
		t.Fatalf("log noted: %s", r.stderr)
	}

	// The proposer's kept answer, its call replaced by one that does not verify against the row: the
	// row is not answer_signed, and log says which call it left out; the changed row still is.
	call := func(who, accord, question string, action mediator.Action, answer string) []byte {
		t.Helper()
		k, err := keyfile.Read(a.key(who))
		if err != nil {
			t.Fatal(err)
		}
		line, err := local.SignCall(k, local.OpResolve, accord, time.Now(), bytes.Repeat([]byte{9}, 16), local.ResolveArgs(question, action, answer))
		if err != nil {
			t.Fatal(err)
		}
		c, code := local.ParseCall(line)
		if code != "" {
			t.Fatalf("parse: %s", code)
		}
		return local.AnswerCall(c)
	}
	good := call("decider", a.id, accepted, mediator.Accepted, "Yes.")
	var sig struct{ Call, Sig string }
	if err := json.Unmarshal(good, &sig); err != nil {
		t.Fatal(err)
	}
	flipped := []byte(sig.Sig)
	flipped[0] = map[bool]byte{true: 'B', false: 'A'}[flipped[0] == 'A']
	badSig, _ := json.Marshal(map[string]string{"call": sig.Call, "sig": string(flipped)})
	other := "sha256:" + strings.Repeat("0", 64)
	for _, c := range []struct {
		name string
		call []byte
	}{
		{"a signature changed", badSig},
		{"signed by the asker", call("proposer", a.id, accepted, mediator.Accepted, "Yes.")},
		{"another Accord", call("decider", other, accepted, mediator.Accepted, "Yes.")},
		{"another question", call("decider", a.id, changed, mediator.Accepted, "Yes.")},
		{"another action", call("decider", a.id, accepted, mediator.Changed, "Yes.")},
		{"another text", call("decider", a.id, accepted, mediator.Accepted, "No.")},
	} {
		dir := copyDir(t, a.state("proposer"))
		p := filepath.Join(dir, "kept.jsonl")
		b, _ := os.ReadFile(p)
		var out []string
		id := ""
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			var o map[string]json.RawMessage
			if err := json.Unmarshal([]byte(l), &o); err != nil {
				t.Fatal(err)
			}
			if str(o["kind"]) == "answer" {
				o["answerCall"] = c.call
				id = str(o["receiptId"])
				nb, _ := json.Marshal(o)
				l = string(nb)
			}
			out = append(out, l)
		}
		if id == "" {
			t.Fatalf("%s: no kept answer", c.name)
		}
		if err := os.WriteFile(p, []byte(strings.Join(out, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		r := must(t, "log", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--format", "json", "--kept", dir)
		var lg struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &lg); err != nil || len(lg.Rows) != 2 {
			t.Fatalf("%s: log: %v", c.name, r)
		}
		if lg.Rows[0]["answer_signed"] != "" || lg.Rows[1]["answer_signed"] != "yes" ||
			r.stderr != "the answer call kept with receipt "+id+" does not verify; left out\n" {
			t.Fatalf("%s: rows %v; stderr %q", c.name, lg.Rows, r.stderr)
		}
	}
}
