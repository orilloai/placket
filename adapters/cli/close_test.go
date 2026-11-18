package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// closeArgs is `window close` of the Accord's journal, without --socket, and extra flags.
func (a *accord) closeArgs(extra ...string) []string {
	return append([]string{"window", "close", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"),
		"--journal", a.journal(), "--accord", a.id}, extra...)
}

// journalLines are the lines of the journal's receipts.jsonl.
func (a *accord) journalLines(t testing.TB) [][]byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.journal(), journal.FileReceipts))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n"))
}

// closedJournal plays, on the worked example's Accord, a change with release, a comment from the
// decider that the proposer never fetches, and three proposals left open, Q1, Q2 and Q3; stops the
// window; and moves Q2's deadline in questions.jsonl to 10 minutes after the asks, so that a close
// 30 minutes after them reaches only Q2's (deadlines are per rule). It returns the Accord, the three
// questions, the comment's receipt id and the time of the asks.
func closedJournal(t *testing.T) (a *accord, open []string, comment string, at time.Time) {
	t.Helper()
	a = newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q0 := a.openOne(t, "Drop the column notes.")
	must(t, a.as("decider", "change", "--question", q0, "--text", "Keep notes, hidden.")...)
	c := must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "comment", "--text", "Read me later.")...)
	cs, err := receipts.Decode([]byte(last(c.stdout)))
	if err != nil {
		t.Fatalf("the comment's receipt: %v", err)
	}
	comment = cs.ID
	at = time.Now().UTC()
	for _, text := range []string{"One.", "Two.", "Three."} {
		open = append(open, a.openOne(t, text))
	}
	a.window.stop(t)
	// A stopped window leaves its socket file; the close must make none.
	if err := os.Remove(a.socket()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.journal(), journal.FileQuestions)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for _, l := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		var q map[string]any
		if err := json.Unmarshal(l, &q); err != nil {
			t.Fatal(err)
		}
		if q["question"] == open[1] {
			q["deadline"] = at.Add(10 * time.Minute).Truncate(time.Second).Format(time.RFC3339)
		}
		qb, _ := json.Marshal(q)
		cb, err := canonical.Form(qb)
		if err != nil {
			t.Fatal(err)
		}
		out = append(append(out, cb...), '\n')
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return a, open, comment, at
}

// TestWindowCloseCommand is U1's journal lines, U4, U5, U22 without --socket and U23 (close-1,
// close-2, close-3): the guards first, each writing nothing; then the close of a stopped window's
// journal, which appends exactly Q2's timeout, Q1's and Q3's closed closings and the close, last;
// the close's chains are the journal's three, at their last receipts in pair order, and its unfetched
// names the comment no one fetched; no socket is made; and a second close is tool error closed.
func TestWindowCloseCommand(t *testing.T) {
	t.Parallel()
	a, open, comment, at := closedJournal(t)
	before := a.receipts(t)
	later := env()
	later.Now = func() time.Time { return at.Add(30 * time.Minute) }

	off := a.id[:len(a.id)-1] + map[bool]string{true: "0", false: "1"}[a.id[len(a.id)-1] != '0']
	for name, args := range map[string][]string{
		"no --accord": {"window", "close", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"), "--journal", a.journal()},
		"an id one hex digit off": {"window", "close", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"),
			"--journal", a.journal(), "--accord", off},
		"an id that is no hash": a.closeArgs("--accord", "accord"),
	} {
		if r := run(t, later, args...); r.code != cli.ExitTool || !slices.Contains(r.lines(), "error usage") {
			t.Fatalf("%s: %v", name, r)
		}
	}
	empty := newAccord(t, "p1", "p2")
	if r := run(t, later, empty.closeArgs()...); r.code != cli.ExitTool || !slices.Contains(r.lines(), "error journal unfounded") {
		t.Fatalf("an empty journal: %v", r)
	}
	if !bytes.Equal(a.receipts(t), before) {
		t.Fatal("a refused close wrote")
	}

	r := run(t, later, a.closeArgs("--json")...)
	if r.code != 0 {
		t.Fatalf("window close: %v", r)
	}
	lines := a.journalLines(t)
	n := len(bytes.Split(bytes.TrimSuffix(before, []byte("\n")), []byte("\n")))
	if len(lines) != n+4 {
		t.Fatalf("%d lines after the close, want %d", len(lines), n+4)
	}
	want := []struct{ question, reason string }{{open[1], receipts.ReasonTimeout}, {open[0], receipts.ReasonClosed}, {open[2], receipts.ReasonClosed}}
	var got []string
	for i, w := range want {
		st, err := receipts.Decode(lines[n+i])
		if err != nil {
			t.Fatal(err)
		}
		q := statementOf(t, lines, w.question)
		if st.Predicate.Reason != w.reason || st.Predicate.Envelope != q.Predicate.Envelope || st.Subject != q.Subject {
			t.Fatalf("line %d: %+v, want %s of %s", n+i+1, st, w.reason, w.question)
		}
		got = append(got, "closed block "+w.reason+" "+st.ID)
	}
	cl, err := receipts.DecodeClose(lines[n+3])
	if err != nil {
		t.Fatalf("the last line is no close: %v", err)
	}
	wantErr := append([]string{"close " + cl.ID}, got...)
	if !slices.Equal(r.lines(), wantErr) {
		t.Fatalf("stderr %q, want %q", r.lines(), wantErr)
	}
	var obj struct {
		Accord string
		Close  struct {
			ReceiptID string
			Receipt   json.RawMessage
		}
		Closings []struct{ Word, Reason, ReceiptID string }
		Exit     int
	}
	if err := json.Unmarshal([]byte(r.stdout), &obj); err != nil || obj.Accord != a.id || obj.Close.ReceiptID != cl.ID ||
		!bytes.Equal(obj.Close.Receipt, lines[n+3]) || len(obj.Closings) != 3 || obj.Closings[0].Reason != receipts.ReasonTimeout ||
		obj.Closings[0].Word != cli.WordBlock || obj.Exit != 0 {
		t.Fatalf("the object %s: %v", r.stdout, err)
	}

	// U4: one entry per chain, its last receipt, in pair order.
	last := map[[2]string]receipts.Final{}
	for _, l := range lines[:n+3] {
		st, err := receipts.Decode(l)
		if err != nil {
			t.Fatal(err)
		}
		p := st.Predicate
		last[[2]string{p.From, p.To}] = receipts.Final{From: p.From, To: p.To, Seq: p.Seq, Receipt: st.ID}
	}
	if len(cl.Chains) != 3 || len(last) != 3 {
		t.Fatalf("chains %+v, want the journal's three", cl.Chains)
	}
	for i, f := range cl.Chains {
		if last[[2]string{f.From, f.To}] != f {
			t.Fatalf("chain %d %+v, want %+v", i, f, last[[2]string{f.From, f.To}])
		}
		if i > 0 && !(canonical.Less(cl.Chains[i-1].From, f.From) || cl.Chains[i-1].From == f.From && canonical.Less(cl.Chains[i-1].To, f.To)) {
			t.Fatalf("chains out of pair order: %+v", cl.Chains)
		}
	}
	// U5: the comment was never fetched, and the close did not wait for it.
	if !slices.Contains(cl.Unfetched, receipts.Unfetched{To: a.refs["proposer"], Receipt: comment}) {
		t.Fatalf("unfetched %+v holds no comment %s to the proposer", cl.Unfetched, comment)
	}
	if _, err := os.Lstat(a.socket()); !os.IsNotExist(err) {
		t.Fatalf("a socket was made: %v", err)
	}

	after := a.receipts(t)
	if r := run(t, later, a.closeArgs()...); r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{"error closed"}) {
		t.Fatalf("a second close: %v", r)
	}
	if !bytes.Equal(a.receipts(t), after) {
		t.Fatal("a second close wrote")
	}
}

// statementOf is the statement of the line whose receipt id is id.
func statementOf(t *testing.T, lines [][]byte, id string) receipts.Statement {
	t.Helper()
	for _, l := range lines {
		if st, err := receipts.Decode(l); err == nil && st.ID == id {
			return st
		}
	}
	t.Fatalf("no receipt %s", id)
	return receipts.Statement{}
}

// closeNow closes the Accord's stopped journal at the current time, and returns the close's id.
func (a *accord) closeNow(t *testing.T) string {
	t.Helper()
	r := must(t, a.closeArgs()...)
	return strings.TrimPrefix(r.lines()[0], "close ")
}

// TestGetKeepsClose is U7 (close-3): on a closed Accord the applier's three fetches return its
// delivery, then the close twice, kept once as close.json; receipts.jsonl does not change; and get
// --all after the last delivery holds the close. The window's start line names the close (U8), and
// a give is refused closed.
func TestGetKeepsClose(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", q)...)
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "comment", "--text", "Later.")...)
	a.window.stop(t)
	id := a.closeNow(t)
	w := a.start(t)
	if !strings.Contains(w.stderr.String(), "accord "+a.id+" closed "+id+"\n") {
		t.Fatalf("the start line of a closed window: %s", w.stderr.String())
	}
	before := a.receipts(t)
	first := must(t, a.as("applier", "get", "--json")...)
	if !strings.HasPrefix(first.lines()[0], "delivery ") || strings.Contains(first.stdout, `"close"`) {
		t.Fatalf("the first fetch: %v", first)
	}
	for i := 0; i < 2; i++ {
		r := must(t, a.as("applier", "get", "--json")...)
		var o struct {
			Close struct{ ReceiptID string }
			Exit  int
		}
		if err := json.Unmarshal([]byte(r.stdout), &o); err != nil || !slices.Equal(r.lines(), []string{"close " + id}) || o.Close.ReceiptID != id {
			t.Fatalf("fetch %d: %v, %v", i+2, r, err)
		}
	}
	kept, err := os.ReadFile(filepath.Join(a.state("applier"), "close.json"))
	lines := a.journalLines(t)
	if err != nil || !bytes.Equal(kept, append(append([]byte(nil), lines[len(lines)-1]...), '\n')) {
		t.Fatalf("close.json: %q, %v", kept, err)
	}
	all := must(t, a.as("proposer", "get", "--all", "--json")...)
	var o struct {
		Deliveries []json.RawMessage
		Close      struct{ ReceiptID string }
	}
	if err := json.Unmarshal([]byte(all.stdout), &o); err != nil || len(o.Deliveries) != 2 || o.Close.ReceiptID != id {
		t.Fatalf("get --all: %v, %v", all, err)
	}
	if _, err := os.Stat(filepath.Join(a.state("proposer"), "close.json")); err != nil {
		t.Fatalf("the proposer's close.json: %v", err)
	}
	g := run(t, env(), a.as("proposer", "give", "--to", a.refs["applier"], "--kind", "proposal", "--text", "After.")...)
	if g.code != cli.ExitRejected || !slices.Equal(g.lines(), []string{"rejected closed"}) {
		t.Fatalf("a give after the close: %v", g)
	}
	if !bytes.Equal(a.receipts(t), before) {
		t.Fatal("the closed window wrote a receipt")
	}
	// A close.json that holds another receipt is not written over.
	if err := os.WriteFile(filepath.Join(a.state("applier"), "close.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := run(t, env(), a.as("applier", "get")...); r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{"error state close.json"}) {
		t.Fatalf("a close.json of another receipt: %v", r)
	}
}

// TestClosedQuestionsAndResult is U6's read cases (close-3): on a closed Accord questions is an
// empty list, result still returns a closed question's receipts, an answer is refused closed and
// writes nothing, and a stranger's get is not-a-member.
func TestClosedQuestionsAndResult(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", q)...)
	q2 := a.openOne(t, "Rename count.")
	must(t, a.as("decider", "answer", "--question", q2)...)
	must(t, a.as("proposer", "get", "--keep")...)
	must(t, a.as("proposer", "get")...)
	open := a.openOne(t, "Rename amount.")
	a.window.stop(t)
	a.closeNow(t)
	a.start(t)
	before := a.receipts(t)
	answers, _ := os.ReadFile(filepath.Join(a.journal(), journal.FileAnswers))
	if r := must(t, a.as("decider", "questions", "--json")...); r.stdout != `{"exit":0,"questions":[]}`+"\n" {
		t.Fatalf("questions: %v", r)
	}
	if r := run(t, env(), a.as("proposer", "result", "--question", q)...); r.code != 0 || !strings.HasPrefix(r.lines()[0], "accepted") {
		t.Fatalf("result: %v", r)
	}
	if r := run(t, env(), a.as("proposer", "result", "--question", open, "--json")...); !strings.Contains(r.stdout, `"word":"closed"`) {
		t.Fatalf("the result of a question the close closed: %v", r)
	}
	// The answer to q2 was fetched and not kept, so result fetches until the queue is empty and is
	// handed the close, which only get keeps: result prints no close line and holds no close.
	if r := run(t, env(), a.as("proposer", "result", "--question", q2, "--json")...); r.code != 0 || strings.Contains(r.stdout, `"close"`) ||
		strings.Contains(r.stderr, "close ") || !strings.HasSuffix(r.stderr, "\n"+cli.LineNotFetched+"\n") {
		t.Fatalf("the result of an answer fetched and not kept: %v", r)
	}
	if _, err := os.Stat(filepath.Join(a.state("proposer"), "close.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("result kept the close: %v", err)
	}
	for _, args := range [][]string{{"answer", "--question", open}, {"decline", "--question", open, "--comment", "No."}} {
		if r := run(t, env(), a.as("decider", args...)...); r.code != cli.ExitRejected || !slices.Equal(r.lines(), []string{"rejected closed"}) {
			t.Fatalf("%s after the close: %v", args[0], r)
		}
	}
	if r := run(t, env(), a.as("proposer", "cancel", "--question", open)...); r.code != cli.ExitRejected || !slices.Equal(r.lines(), []string{"rejected closed"}) {
		t.Fatalf("a cancel after the close: %v", r)
	}
	if r := run(t, env(), a.as("stranger", "answer", "--question", open)...); r.code != cli.ExitRejected || !slices.Equal(r.lines(), []string{"rejected not-a-member"}) {
		t.Fatalf("a stranger's answer: %v", r)
	}
	if r := run(t, env(), a.as("stranger", "get")...); r.code != cli.ExitRejected || !slices.Equal(r.lines(), []string{"rejected not-a-member"}) {
		t.Fatalf("a stranger's get: %v", r)
	}
	if !bytes.Equal(a.receipts(t), before) {
		t.Fatal("the closed window wrote a receipt")
	}
	if now, _ := os.ReadFile(filepath.Join(a.journal(), journal.FileAnswers)); !bytes.Equal(now, answers) {
		t.Fatal("the closed window wrote an answer call")
	}
}

// TestWindowCloseServes is U22's --socket case (close-1, close-3): `window close --socket` writes
// the close, prints the closed start line, serves the closed Accord, whose get returns the close,
// and exits 0 when interrupted; its --json object holds the close as it does without --socket, its
// id and its receipt, the journal's last line.
func TestWindowCloseServes(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "A note.")...)
	a.window.stop(t)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	e := env()
	e.Context, e.Ready = ctx, ready
	stdout, stderr := &syncBuffer{}, &syncBuffer{}
	done := make(chan int, 1)
	go func() {
		done <- cli.Run(e, a.closeArgs("--socket", a.socket(), "--json"), strings.NewReader(""), stdout, stderr)
	}()
	select {
	case <-ready:
	case code := <-done:
		t.Fatalf("window close exited %d: %s", code, stderr.String())
	case <-time.After(10 * time.Second):
		t.Fatal("window close did not serve")
	}
	must(t, a.as("p2", "get")...)
	r := must(t, a.as("p2", "get")...)
	cancel()
	if code := <-done; code != 0 {
		t.Fatalf("window close exited %d: %s", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	id := strings.TrimPrefix(lines[0], "close ")
	if !slices.Equal(r.lines(), []string{"close " + id}) || lines[1] != "accord "+a.id+" closed "+id {
		t.Fatalf("stderr %q; the get: %v", lines, r)
	}
	journalLines := a.journalLines(t)
	var obj struct {
		Accord string
		Close  struct {
			ReceiptID string
			Receipt   json.RawMessage
		}
		Closings []json.RawMessage
	}
	if err := json.Unmarshal([]byte(stdout.String()), &obj); err != nil || obj.Accord != a.id || obj.Close.ReceiptID != id ||
		!bytes.Equal(obj.Close.Receipt, journalLines[len(journalLines)-1]) || obj.Closings == nil {
		t.Fatalf("the object %s: %v", stdout.String(), err)
	}
}

// successor makes the founding set of a new Accord of the same parties, whose policy supersedes the
// given Accord and close, and returns its path; with accord empty, it supersedes nothing.
func successor(t *testing.T, a *accord, accord, close string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.dir, "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err := json.Unmarshal(b, &policy); err != nil {
		t.Fatal(err)
	}
	if accord != "" {
		policy["supersedes"] = map[string]any{"accord": accord, "close": close}
	}
	pb, _ := json.Marshal(policy)
	name := strings.TrimPrefix(accord+close, "sha256:")
	if len(name) > 12 {
		name = name[len(name)-12:]
	}
	pp := filepath.Join(a.dir, "policy-"+name+".json")
	if err := os.WriteFile(pp, pb, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"founding", "--policy", pp, "--mediator", filepath.Join(a.dir, "mediator.json"), "--card", filepath.Join(a.dir, "card.json")}
	for _, p := range []string{"p1", "p2"} {
		bind := must(t, "bind", "--key", a.key("principal"), "--actor", a.refs[p], "--role", "peer", "--mediator", a.refs["mediator"],
			"--until", "2099-01-01T00:00:00Z", "--policy", pp).stdout
		bp := filepath.Join(a.dir, p+"-"+name+".binding")
		if err := os.WriteFile(bp, []byte(bind), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "--declaration", filepath.Join(a.dir, p+".declaration"), "--binding", bp)
	}
	set := filepath.Join(a.dir, "founding-"+name+".json")
	if err := os.WriteFile(set, []byte(must(t, args...).stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	return set
}

// notedAccord plays two notes from p1 to p2, one back and a third from p1, stops the window and
// returns the Accord.
func notedAccord(t *testing.T) *accord {
	t.Helper()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "One.")...)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "Two.")...)
	must(t, a.as("p2", "give", "--to", a.refs["p1"], "--kind", "note", "--text", "Back.")...)
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "Three.")...)
	a.window.stop(t)
	return a
}

// TestVerifyCompleteLine is U11 and U12 (verify-4, close-4): a whole log with no close is not
// complete; once it ends in its close, the complete line is against that close, in the object as on
// standard error; its last two receipts as a subset claim nothing and name the chain that starts
// late; and the closed log with its first receipt removed, whose first two are one pair, fails chain
// at line 1.
func TestVerifyCompleteLine(t *testing.T) {
	t.Parallel()
	a := notedAccord(t)
	if r := verifyAccord(t, a, a.journal()); r.code != 0 || !slices.Contains(r.lines(), cli.LineNotComplete) {
		t.Fatalf("a log with no close: %v", r)
	}
	id := a.closeNow(t)
	want := fmt.Sprintf(cli.LineComplete, id)
	r := verifyAccord(t, a, a.journal(), "--json")
	var o struct{ Complete string }
	if err := json.Unmarshal([]byte(r.stdout), &o); r.code != 0 || err != nil || !slices.Contains(r.lines(), want) || o.Complete != want {
		t.Fatalf("a closed log: %v", r)
	}
	lines := a.journalLines(t)
	write := func(l [][]byte) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, journal.FileReceipts), append(bytes.Join(l, []byte("\n")), '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	sub := verifyAccord(t, a, write(lines[len(lines)-2:]), "--subset")
	if sub.code != 0 || !slices.Contains(sub.lines(), cli.LineSubsetNotComplete) || slices.Contains(sub.lines(), want) ||
		!slices.Contains(sub.lines(), fmt.Sprintf(cli.LineChainStart, a.refs["p1"], a.refs["p2"], 3)) {
		t.Fatalf("the last two as a subset: %v", sub)
	}
	cut := verifyAccord(t, a, write(lines[1:]))
	if cut.code != cli.ExitTool || !slices.Equal(cut.lines(), []string{"error verify chain receipts.jsonl line 1"}) {
		t.Fatalf("the closed log less its first receipt: %v", cut)
	}
}

// TestVerifySupersededBy is U13 (close-4): with the founding set of a successor whose supersedes
// pins this Accord and this log's close, verify exits 0 with the pinned complete line; a successor
// that names another close or another Accord, one that supersedes nothing, and a log with no close,
// are tool error verify superseded; and --superseded-by needs --accord.
func TestVerifySupersededBy(t *testing.T) {
	t.Parallel()
	a := notedAccord(t)
	open := successor(t, a, a.id, "sha256:"+strings.Repeat("cd", 32))
	if r := verifyAccord(t, a, a.journal(), "--superseded-by", open); r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{"error verify superseded"}) {
		t.Fatalf("a log with no close: %v", r)
	}
	id := a.closeNow(t)
	next := successor(t, a, a.id, id)
	nb, _ := os.ReadFile(next)
	nextID, _ := canonical.TypedHash(bytes.TrimSuffix(nb, []byte("\n")))
	r := verifyAccord(t, a, a.journal(), "--superseded-by", next)
	if r.code != 0 || !slices.Contains(r.lines(), fmt.Sprintf(cli.LinePinnedComplete, id, nextID)) {
		t.Fatalf("a pinned close: %v", r)
	}
	for name, set := range map[string]string{
		"another close":  open,
		"another Accord": successor(t, a, "sha256:"+strings.Repeat("ab", 32), id),
		"no supersedes":  successor(t, a, "", ""),
	} {
		if r := verifyAccord(t, a, a.journal(), "--superseded-by", set); r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{"error verify superseded"}) {
			t.Fatalf("%s: %v", name, r)
		}
	}
	if r := run(t, env(), "verify", "--receipts", a.journal(), "--mediator", a.refs["mediator"], "--superseded-by", next); r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{"error usage"}) {
		t.Fatalf("--superseded-by without --accord: %v", r)
	}
}

// TestClosedReconcileExport is U24: reconcile reads a closed journal without a mismatch, and the log
// export writes no row for the close and names it.
func TestClosedReconcileExport(t *testing.T) {
	t.Parallel()
	a := notedAccord(t)
	a.start(t)
	a.drain(t, "p1")
	a.drain(t, "p2")
	a.window.stop(t)
	id := a.closeNow(t)
	r := must(t, "reconcile", "--mediator", a.refs["mediator"], "--accord", a.id, "--card", a.card, "--receipts", a.journal(),
		"--kept", a.state("p1"), "--kept", a.state("p2"), "--json")
	if r.stdout != `{"exit":0,"missing":[],"removed":[],"unrecorded":[]}`+"\n" {
		t.Fatalf("reconcile: %v", r)
	}
	l := must(t, "log", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--all")
	var lg struct {
		Close string
		Rows  []map[string]any
	}
	if err := json.Unmarshal([]byte(l.stdout), &lg); err != nil || lg.Close != id || len(lg.Rows) != 4 {
		t.Fatalf("log --all: %v, %v", l, err)
	}
}

// TestClosedWordInLogAndShow is U21 (log-1, receipt-8): a question the close closed reads closed in
// the log, its closing shows the reason closed by mediator close, and close.json shows one line per
// chain and per delivery not yet fetched.
func TestClosedWordInLogAndShow(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	a.window.stop(t)
	id := a.closeNow(t)
	l := must(t, "log", "--mediator", a.refs["mediator"], "--receipts", a.journal())
	var lg struct {
		Close string
		Rows  []map[string]any
	}
	if err := json.Unmarshal([]byte(l.stdout), &lg); err != nil || lg.Close != id || len(lg.Rows) != 1 || lg.Rows[0]["answer"] != cli.WordClosed ||
		lg.Rows[0]["proposal"] != q {
		t.Fatalf("log: %v, %v", l, err)
	}
	lines := a.journalLines(t)
	closing := filepath.Join(a.dir, "closing.json")
	if err := os.WriteFile(closing, lines[len(lines)-2], 0o600); err != nil {
		t.Fatal(err)
	}
	cs, err := receipts.Decode(lines[len(lines)-2])
	if err != nil {
		t.Fatal(err)
	}
	s := must(t, "show", "--receipt", closing)
	if !strings.Contains(s.stdout, "verdict block, reason closed\nby mediator close\n") || s.lines()[1] != "block closed proposal "+cs.Predicate.At {
		t.Fatalf("show of the closing: %v", s)
	}
	cl := filepath.Join(a.dir, "close.json")
	if err := os.WriteFile(cl, append(lines[len(lines)-1], '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := receipts.DecodeClose(lines[len(lines)-1])
	if err != nil {
		t.Fatal(err)
	}
	sc := must(t, "show", "--receipt", cl)
	out := strings.Split(strings.TrimSuffix(sc.stdout, "\n"), "\n")
	if len(out) != 4+len(c.Chains)+len(c.Unfetched) || out[1] != "close "+id || !strings.HasPrefix(out[4], "chain from ") ||
		!slices.Equal(sc.lines(), []string{cli.LineNotVerified, "close " + c.At, fmt.Sprintf(cli.ModuleLine, a.card), cli.ClaimClose, cli.Scope}) {
		t.Fatalf("show of close.json: %v", sc)
	}
}

// TestTornCloseAtStart is U14 at the command line (close-1): a window on a journal whose close was
// cut before the close receipt refuses to start, saying to run window close, with tool error
// journal receipts.jsonl close; window close then writes the close receipt alone.
func TestTornCloseAtStart(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	a.openOne(t, "Rename total.")
	a.window.stop(t)
	a.closeNow(t)
	full := a.receipts(t)
	lines := a.journalLines(t)
	cut := append(bytes.Join(lines[:len(lines)-1], []byte("\n")), '\n')
	if err := os.WriteFile(filepath.Join(a.journal(), journal.FileReceipts), cut, 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, env(), "window", "--key", a.key("mediator"), "--founding", filepath.Join(a.dir, "founding.json"),
		"--socket", a.socket(), "--journal", a.journal())
	if r.code != cli.ExitTool || !slices.Equal(r.lines(), []string{cli.LineTornClose, "error journal receipts.jsonl close"}) {
		t.Fatalf("a window on a torn close: %v", r)
	}
	c := must(t, a.closeArgs()...)
	if len(c.lines()) != 1 || !bytes.Equal(a.receipts(t), full) {
		t.Fatalf("completing the close: %v", c)
	}
}
