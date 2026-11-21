package cli_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/receipts"
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
// the comment to the asker with re the question; a decline of a closed question, which the window
// no longer shows, reaches the window, which rejects it with no-open-question, exit 2, with --json
// too, and gives no comment (RCP-01, RCP-02, RCP-05, RCP-12, maybe-7).
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
	if r.code != 2 || r.stderr != "rejected no-open-question\n" || r.stdout != "" {
		t.Fatalf("a decline of a closed question: %v", r)
	}
	r = run(t, env(), a.as("decider", "decline", "--question", q, "--comment", "Too late.", "--json")...)
	if r.code != 2 || r.stderr != "rejected no-open-question\n" || r.stdout != `{"exit":2,"rejection":"no-open-question"}`+"\n" {
		t.Fatalf("a decline of a closed question with --json: %v", r)
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

// TestTheAnswerCallTravelsWithTheAnswer accepts one proposal and changes another, each asking for
// release, and the asker and the receiver fetch with --keep: every delivery an answer queued, the
// released proposal included, carries the decider's signed call inside it, which reads under the
// decider's key, names the question and is the one the receipt's by names. The journal's
// answers.jsonl holds both calls; no line of receipts.jsonl holds a call; after the fetch no file of
// the journal holds the version; and log marks both rows answer_signed, from the journal or from the
// kept deliveries alone, and neither once both are gone (answer-call-attached, maybe-12, U8, U30).
func TestTheAnswerCallTravelsWithTheAnswer(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	accepted := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", accepted, "--text", "Yes.")...)
	changed := a.openOne(t, "Drop notes.")
	must(t, a.as("decider", "change", "--question", changed, "--text", "Hide notes.")...)
	a.drain(t, "proposer")
	a.drain(t, "applier")
	calls := map[string]bool{}
	carried := 0
	for _, who := range []string{"proposer", "applier"} {
		for _, k := range keptLines(t, a.state(who)) {
			if str(k["kind"]) == "" || str(k["kind"]) == "note" {
				continue
			}
			st, err := receipts.Decode(k["receipt"])
			if err != nil || st.Predicate.By.Decider != receipts.ByAnswer {
				t.Fatalf("%s's %s: its receipt is by %+v, %v", who, str(k["kind"]), st.Predicate.By, err)
			}
			c, err := answer.Read(k["answer"])
			if err != nil || c.From != a.refs["decider"] || c.Accord != a.id || answer.ID(k["answer"]) != st.Predicate.By.Answer ||
				(c.Question != accepted && c.Question != changed) {
				t.Fatalf("%s's %s: call %s, %v", who, str(k["kind"]), k["answer"], err)
			}
			calls[string(k["answer"])] = true
			carried++
		}
	}
	if carried != 4 || len(calls) != 2 {
		t.Fatalf("%d deliveries carry %d calls, want 4 and 2", carried, len(calls))
	}
	jb, err := os.ReadFile(filepath.Join(a.journal(), "answers.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for c := range calls {
		if !bytes.Contains(jb, []byte(c+"\n")) {
			t.Fatalf("answers.jsonl does not hold %s", c)
		}
	}
	for i, l := range strings.Split(strings.TrimSuffix(string(a.receipts(t)), "\n"), "\n") {
		for c := range calls {
			var sig struct{ Sig string }
			_ = json.Unmarshal([]byte(c), &sig)
			if strings.Contains(l, sig.Sig) || strings.Contains(l, "answerCall") {
				t.Fatalf("receipts.jsonl line %d holds a call", i+1)
			}
		}
	}
	_ = filepath.Walk(a.journal(), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			if bytes.Contains(b, []byte("Hide notes.")) {
				t.Fatalf("%s holds the version after the fetch", p)
			}
		}
		return nil
	})
	signed := func(journal string, kept ...string) []string {
		t.Helper()
		args := []string{"log", "--mediator", a.refs["mediator"], "--receipts", journal, "--format", "json"}
		for _, k := range kept {
			args = append(args, "--kept", k)
		}
		r := must(t, args...)
		var lg struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &lg); err != nil || len(lg.Rows) != 2 || r.stderr != "" {
			t.Fatalf("log: %v", r)
		}
		return []string{fmt.Sprint(lg.Rows[0]["answer_signed"]), fmt.Sprint(lg.Rows[1]["answer_signed"])}
	}
	if got := signed(a.journal()); got[0] != "yes" || got[1] != "yes" {
		t.Fatalf("from the journal: %v", got)
	}
	bare := copyDir(t, a.journal())
	if err := os.Remove(filepath.Join(bare, "answers.jsonl")); err != nil {
		t.Fatal(err)
	}
	if got := signed(bare); got[0] != "" || got[1] != "" {
		t.Fatalf("with no answers.jsonl and nothing kept: %v", got)
	}
	if got := signed(bare, a.state("proposer")); got[0] != "yes" || got[1] != "yes" {
		t.Fatalf("from the proposer's kept deliveries: %v", got)
	}
}

// TestAnswerSignsOnlyWhatItChecked answers a question a window shows under a salt that does not make
// the ask's receipt's digest: answer, change and decline are tool error asked and send no resolve
// call, their last call the questions they read; questions prints nothing of the misstated part and
// is tool error asked (U12). A question the window does not show is answered, and the window's
// refusal printed: rejected no-open-question, exit 2, the last call the resolve.
func TestAnswerSignsOnlyWhatItChecked(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	ask := bytes.Split(a.receipts(t), []byte("\n"))[0]
	w := newFakeWindow(t, a.dir)
	entry := map[string]any{"id": q, "asker": a.refs["proposer"], "kind": "proposal", "deadline": "2099-01-01T00:00:00Z",
		"form": "text", "text": "Rename total.", "size": 13, "receipt": json.RawMessage(ask), "salt": strings.Repeat("ab", 32)}
	w.set(t, map[string]any{"ok": true, "questions": []any{entry}})
	for _, args := range [][]string{{"answer", "--question", q}, {"change", "--question", q, "--text", "Rename it."}, {"decline", "--question", q, "--comment", "No."}} {
		r := run(t, env(), w.as(a, "decider", args...)...)
		if r.code != 3 || r.stderr != "error asked\n" || !bytes.Contains(w.lastCall(), []byte(`"op":"questions"`)) {
			t.Fatalf("%s of a part the window misstates: %v, last call %s", args[0], r, w.lastCall())
		}
	}
	if r := run(t, env(), w.as(a, "decider", "questions")...); r.code != 3 || r.stderr != "error asked\n" || r.stdout != "" {
		t.Fatalf("questions of a part the window misstates: %v", r)
	}
	w.set(t, map[string]any{"ok": false, "rejection": "no-open-question"})
	w.setOp(t, "questions", map[string]any{"ok": true, "questions": []any{}})
	for _, args := range [][]string{{"answer", "--question", q}, {"change", "--question", q, "--text", "Rename it."}, {"decline", "--question", q, "--comment", "No."}} {
		r := run(t, env(), w.as(a, "decider", args...)...)
		if r.code != 2 || r.stderr != "rejected no-open-question\n" || !bytes.Contains(w.lastCall(), []byte(`"op":"resolve"`)) {
			t.Fatalf("%s of a question the window does not show: %v, last call %s", args[0], r, w.lastCall())
		}
	}
	w.setOp(t, "questions", map[string]any{"ok": true})
	for _, args := range [][]string{{"answer", "--question", q}, {"change", "--question", q, "--text", "Rename it."}, {"decline", "--question", q, "--comment", "No."}} {
		r := run(t, env(), w.as(a, "decider", args...)...)
		if r.code != 3 || r.stderr != "error reply\n" || !bytes.Contains(w.lastCall(), []byte(`"op":"questions"`)) {
			t.Fatalf("%s after a reply to questions that is no questions: %v, last call %s", args[0], r, w.lastCall())
		}
	}
	if r := run(t, env(), w.as(a, "decider", "questions")...); r.code != 3 || r.stderr != "error reply\n" || r.stdout != "" {
		t.Fatalf("questions after a reply that is no questions: %v", r)
	}
}

// TestAnswerReadsEveryPage answers a question the window shows only on the second page of its
// questions: the first page shows none and says there are more, so answer asks again after its
// cursor, finds the question as its ask's receipt names it, and sends the resolve, whose reply it
// prints. Had it stopped at the first page it would have sent no resolve.
func TestAnswerReadsEveryPage(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	c, err := client.Open(client.Mediator{Accord: a.id, Key: a.key("decider"), Socket: a.socket()}, client.Options{Now: time.Now, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	shown, err := c.Questions(context.Background(), 0, "")
	if err != nil || len(shown.Questions) != 1 || shown.Questions[0].ID != q {
		t.Fatalf("the window shows %d questions, %v", len(shown.Questions), err)
	}
	ask := bytes.Split(a.receipts(t), []byte("\n"))[0]
	entry := map[string]any{"id": q, "asker": a.refs["proposer"], "kind": "proposal", "deadline": "2099-01-01T00:00:00Z",
		"form": "text", "text": "Rename total.", "size": 13, "receipt": json.RawMessage(ask), "salt": hex.EncodeToString(shown.Questions[0].Salt)}
	w := newFakeWindow(t, a.dir)
	w.set(t, map[string]any{"ok": false, "rejection": "no-open-question"})
	w.setPage(t, "", map[string]any{"ok": true, "questions": []any{}, "more": true, "cursor": "page-2"})
	w.setPage(t, "page-2", map[string]any{"ok": true, "questions": []any{entry}, "cursor": "page-3"})
	r := run(t, env(), w.as(a, "decider", "answer", "--question", q)...)
	if r.code != 2 || r.stderr != "rejected no-open-question\n" || !bytes.Contains(w.lastCall(), []byte(`"op":"resolve"`)) {
		t.Fatalf("an answer to a question on the second page: %v, last call %s", r, w.lastCall())
	}
}

// TestAnswerRefusesPagesThatDoNotAdvance answers a question the window does not show on a page that
// says there are more but gives no cursor, and then on one whose cursor is the one it was asked
// after. Neither lets answer, change or decline read the rest of the questions, so each is tool
// error reply, exit 3, sends no resolve, and its last call is the questions it read.
func TestAnswerRefusesPagesThatDoNotAdvance(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	w := newFakeWindow(t, a.dir)
	w.set(t, map[string]any{"ok": false, "rejection": "no-open-question"})
	for _, c := range []struct {
		name  string
		pages map[string]map[string]any
	}{
		{"no cursor", map[string]map[string]any{
			"": {"ok": true, "questions": []any{}, "more": true},
		}},
		{"the same cursor", map[string]map[string]any{
			"":       {"ok": true, "questions": []any{}, "more": true, "cursor": "page-2"},
			"page-2": {"ok": true, "questions": []any{}, "more": true, "cursor": "page-2"},
		}},
	} {
		for after, page := range c.pages {
			w.setPage(t, after, page)
		}
		for _, args := range [][]string{{"answer", "--question", q}, {"change", "--question", q, "--text", "Rename it."}, {"decline", "--question", q, "--comment", "No."}} {
			r := run(t, env(), w.as(a, "decider", args...)...)
			if r.code != 3 || r.stderr != "error reply\n" || !bytes.Contains(w.lastCall(), []byte(`"op":"questions"`)) {
				t.Fatalf("%s after a page with %s: %v, last call %s", args[0], c.name, r, w.lastCall())
			}
		}
	}
}

// TestOnlyAnAnswererAnswers answers an open question as a member whose binding gives it no role the
// question's rule names: the window does not show it the question, so the answer reaches the window,
// which rejects it with not-an-answerer, exit 2, and the question stays open (maybe-6).
func TestOnlyAnAnswererAnswers(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	before := a.receipts(t)
	r := run(t, env(), a.as("applier", "answer", "--question", q)...)
	if r.code != 2 || r.stderr != "rejected not-an-answerer\n" || r.stdout != "" {
		t.Fatalf("an answer by a member that answers nothing: %v", r)
	}
	if !bytes.Equal(a.receipts(t), before) || a.question(t, "decider") != q {
		t.Fatal("a rejected answer wrote a receipt or closed the question")
	}
}

// TestAnswerWithNoRelease accepts with --no-release and --keep: the answer passes with no release
// receipt, the applier is queued nothing, and the decider keeps the call it signed, release false,
// with the salt its digest was taken under.
func TestAnswerWithNoRelease(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	r := must(t, a.as("decider", "answer", "--question", q, "--no-release", "--keep")...)
	if len(r.lines()) != 1 || !strings.HasPrefix(r.stderr, "pass ok ") {
		t.Fatalf("answer --no-release: %v", r)
	}
	if got := a.drain(t, "applier"); len(got) != 0 {
		t.Fatalf("the applier fetched %v", got)
	}
	k := keptLines(t, a.state("decider"))
	if len(k) != 1 {
		t.Fatalf("the decider kept %d lines", len(k))
	}
	c, err := answer.Read(k[0]["answer"])
	salt := strings.Trim(string(k[0]["salt"]), `"`)
	if err != nil || c.Release || c.Question != q || c.Action != answer.Accepted || len(salt) != 64 {
		t.Fatalf("the kept call %s, %v", k[0]["answer"], err)
	}
}
