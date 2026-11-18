package local_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/judge"
	"github.com/orilloai/placket/adapters/judge/jev"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// The planted strings of the judge tests: a model's output and a key, each plain words, so the
// Makefile's secrets step takes neither for a secret.
const (
	plantedOutput = "planted model output words"
	plantedKey    = "planted judge key words"
)

// A fakeJudge is a judge a test scripts. It says verdict, with output; with hold set it waits until
// hold is closed or its context is done. It counts its calls and its returns, and logs each call's
// name on log, when set.
type fakeJudge struct {
	name    string
	verdict string
	hold    chan struct{}
	calls   atomic.Int32
	returns atomic.Int32
	log     *callLog
}

type callLog struct {
	mu    sync.Mutex
	names []string
}

func (l *callLog) add(n string) { l.mu.Lock(); l.names = append(l.names, n); l.mu.Unlock() }
func (l *callLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.names...)
}

func (f *fakeJudge) Judge(ctx context.Context, r judge.Request) (judge.Answer, error) {
	defer f.returns.Add(1)
	f.calls.Add(1)
	if f.log != nil {
		f.log.add(f.name)
	}
	if f.hold != nil {
		select {
		case <-f.hold:
		case <-ctx.Done():
			return judge.Answer{}, ctx.Err()
		}
	}
	if f.verdict == "" {
		return judge.Answer{Verdict: "abstain"}, nil
	}
	return judge.Answer{Verdict: f.verdict, Output: []byte(plantedOutput)}, nil
}

// configOf is judge name's config and its hash.
func configOf(t testing.TB, name string) judge.Loaded {
	t.Helper()
	cfg := judge.Config{Name: name, Provider: "jev", Host: "h.example", Model: "m", Threshold: 0,
		Instruction: "Block a report that deletes data."}
	b, _ := json.Marshal(map[string]any{"name": cfg.Name, "provider": cfg.Provider, "host": cfg.Host, "model": cfg.Model,
		"threshold": cfg.Threshold, "instruction": cfg.Instruction})
	c, err := canonical.Form(b)
	if err != nil {
		t.Fatal(err)
	}
	got, h, err := judge.ReadConfig(c)
	if err != nil || got != cfg {
		t.Fatalf("the config of %s: %v", name, err)
	}
	return judge.Loaded{Config: cfg, Hash: h}
}

// uses is a rule's judges: name and use, in order.
func uses(pairs ...string) []any {
	var out []any
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"name": pairs[i], "use": pairs[i+1]})
	}
	return out
}

// judgedMaybe is a maybe term of dir for report with the other side's role, asking the decider, and
// listing judges.
func judgedMaybe(dir, with string, judges []any) map[string]any {
	t := term("maybe", dir, []string{with}, []string{"report"})
	r := map[string]any{"to": []string{"role:decider"}, "timeout": 3600}
	if judges != nil {
		r["judges"] = judges
	}
	t["resolve"] = r
	return t
}

// A judged is a world whose Accord's card lists judges, with the fakes the window's judges are.
type judged struct {
	*world
	loaded []judge.Loaded
	fakes  map[string]*fakeJudge
	log    *callLog
}

// newJudged is a world whose card lists the named judges, each a fake, and whose proposer and applier
// both list the rule's judges for report, unless receiverOnly: then only the applier's term is
// maybe, and the proposer's is a want.
func newJudged(t testing.TB, names []string, rule []any, receiverOnly bool) *judged {
	t.Helper()
	j := &judged{fakes: map[string]*fakeJudge{}, log: &callLog{}}
	var card []any
	for _, n := range names {
		l := configOf(t, n)
		j.loaded = append(j.loaded, l)
		card = append(card, map[string]any{"name": n, "version": jev.Version, "hash": l.Hash})
		j.fakes[n] = &fakeJudge{name: n, log: j.log}
	}
	proposer := judgedMaybe("give", "applier", rule)
	if receiverOnly {
		proposer = term("want", "give", []string{"applier"}, []string{"report"})
	}
	set := askAndLogWith(t, "mediator", card, map[string][]map[string]any{
		"proposer": {proposer},
		"applier":  {judgedMaybe("get", "proposer", rule)},
	})
	w := newWorld(t)
	w.set = set
	w.accord, _ = canonical.TypedHash(set)
	j.world = w
	return j
}

// window is the world's window, with its judges' configs and the fakes as their adapters.
func (j *judged) window(t testing.TB) local.Window {
	cfg := j.world.window(t)
	cfg.Judges = j.loaded
	cfg.NewJudge = func(c judge.Config) judge.Judge { return j.fakes[c.Name] }
	return cfg
}

// giveAsync gives a part from one member to another at its next seq, in the background, its body
// as name and value; the reply comes on the channel.
func (j *judged) giveAsync(t testing.TB, from, to, id, kind, name string, value any) chan callResult {
	t.Helper()
	msg := j.message(t, id, key(t, to).Reference(), kind, name, value, "")
	j.seqs[from]++
	env := envelope(t, j.seqs[from], hashOf(msg), key(t, from))
	c := j.client(t, from)
	out := make(chan callResult, 1)
	go func() {
		rep, err := c.Call(local.OpGive, local.GiveArgs(msg, env))
		out <- callResult{rep, err}
	}()
	return out
}

type callResult struct {
	rep local.Reply
	err error
}

func wait(t testing.TB, c chan callResult) local.Reply {
	t.Helper()
	select {
	case r := <-c:
		if r.err != nil {
			t.Fatalf("the call: %v", r.err)
		}
		return r.rep
	case <-time.After(20 * time.Second):
		t.Fatal("no reply")
	}
	return local.Reply{}
}

// judgedLines are the lines of the judged file of the receipt.
func (j *judged) judgedLines(t testing.TB, receiptID string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(j.journal, journal.DirJudged, strings.TrimPrefix(receiptID, "sha256:")+".jsonl"))
	if err != nil {
		t.Fatalf("the judged file: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// byOf is the by of a receipt.
func byOf(t testing.TB, receipt []byte) receipts.By {
	t.Helper()
	st, err := receipts.Decode(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return st.Predicate.By
}

// TestJudgesRunOnlyWhereAsked gives a report both sides list the judges a and b for, to decide: a is
// asked, then b; a note, which no maybe term selects, asks none. When only the receiver's term is a
// maybe, no judge is asked (composition-5, composition-7).
func TestJudgesRunOnlyWhereAsked(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a", "b"}, uses("a", "decide", "b", "decide"), false)
	start(t, j.window(t))
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" || rep.Outcome.Reason != "maybe" {
		t.Fatalf("the report: %+v", rep.Outcome)
	}
	if got := j.log.all(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("judges asked: %v", got)
	}
	if rep := j.give(t, "proposer", "applier", "n-1", "note", "A note."); rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("the note: %+v", rep.Outcome)
	}
	if got := j.log.all(); len(got) != 2 {
		t.Fatalf("a note asked a judge: %v", got)
	}

	one := newJudged(t, []string{"a"}, uses("a", "decide"), true)
	start(t, one.window(t))
	if rep := one.give(t, "proposer", "applier", "r-1", "report", "A report."); rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
		t.Fatalf("one side's judge: %+v", rep.Outcome)
	}
	if n := one.fakes["a"].calls.Load(); n != 0 {
		t.Fatalf("one side's judge was asked %d times", n)
	}
}

// TestAJudgeOnAClosedAccord gives a report a judge is listed for on a closed Accord: the window starts
// with no judge configs and no reach, the reply is the rejection closed, and no judge is asked
// (close-3, docs/adr/0023-judges.md).
func TestAJudgeOnAClosedAccord(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	r := start(t, j.window(t))
	r.stop(t)
	if err := os.Remove(j.socket); err != nil {
		t.Fatal(err)
	}
	cfg := j.world.window(t)
	cfg.Close, cfg.Socket = true, ""
	if err := local.Run(context.Background(), cfg); err != nil {
		t.Fatalf("the close: %v", err)
	}
	closed := j.world.window(t)
	closed.NewJudge = func(c judge.Config) judge.Judge { return j.fakes[c.Name] }
	r = start(t, closed)
	rep := j.give(t, "proposer", "applier", "r-1", "report", "A report.")
	if rep.Rejection != "closed" || j.fakes["a"].calls.Load() != 0 {
		t.Fatalf("a give on a closed Accord: %+v, asked %d times", rep, j.fakes["a"].calls.Load())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range *r.events {
		if e.Kind == local.EventStarted && (e.Close == "" || len(e.Reach) != 0) {
			t.Fatalf("the closed start: %+v", e)
		}
	}
}

// TestAPassOrBlockIsFinal stacks a, which says none, b, which blocks, and c: the give is block,
// judge:b, by the judge b, and c is never asked; with all three saying none, the give is asked and a
// question opens (composition-7).
func TestAPassOrBlockIsFinal(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a", "b", "c"}, uses("a", "decide", "b", "decide", "c", "decide"), false)
	j.fakes["b"].verdict = "block"
	start(t, j.window(t))
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "block" || rep.Outcome.Reason != "judge:b" ||
		byOf(t, rep.Outcome.Receipt) != (receipts.By{Decider: receipts.ByJudge, Judge: "b"}) || j.fakes["c"].calls.Load() != 0 {
		t.Fatalf("the report: %+v, c asked %d times", rep.Outcome, j.fakes["c"].calls.Load())
	}
	want := []string{`{"abstained":true,"judge":"a","outcome":"none","use":"decide"}`,
		`{"judge":"b","outcome":"block","output":"` + plantedOutput + `","use":"decide"}`}
	if got := j.judgedLines(t, rep.Outcome.ReceiptID); !reflect.DeepEqual(got, want) {
		t.Fatalf("judged lines %v", got)
	}

	none := newJudged(t, []string{"a", "b", "c"}, uses("a", "decide", "b", "decide", "c", "decide"), false)
	start(t, none.window(t))
	rep = wait(t, none.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" || rep.Outcome.Reason != "maybe" ||
		byOf(t, rep.Outcome.Receipt) != (receipts.By{Decider: receipts.ByTerms}) {
		t.Fatalf("every judge none: %+v", rep.Outcome)
	}
	if qs := none.call(t, "decider", local.OpQuestions, map[string]any{}); len(qs.Questions) != 1 || qs.Questions[0].ID != rep.Outcome.ReceiptID {
		t.Fatalf("the question: %+v", qs.Questions)
	}
}

// TestAJudgeNeverStallsTheLoop holds a's run while the applier fetches: the fetch is answered first
// (one-mediator-goroutine).
func TestAJudgeNeverStallsTheLoop(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.fakes["a"].hold = make(chan struct{})
	start(t, j.window(t))
	pending := j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report.")
	for j.fakes["a"].calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	if rep := j.call(t, "applier", local.OpGet, map[string]any{}); rep.Delivery != nil || rep.Outcome != nil {
		t.Fatalf("the fetch: %+v", rep)
	}
	select {
	case r := <-pending:
		t.Fatalf("the give was answered before its judge: %+v", r)
	default:
	}
	close(j.fakes["a"].hold)
	if rep := wait(t, pending); rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
		t.Fatalf("the give: %+v", rep.Outcome)
	}
}

// TestTheBudget holds a judge until its context is done, with a budget of 50 ms: the give is asked,
// and a's line is none, timeout. The budget and the reply's margin are below the connection's
// deadline.
func TestTheBudget(t *testing.T) {
	t.Parallel()
	if judge.Budget+local.ReplyMargin >= local.ReadDeadline {
		t.Fatalf("budget %v and margin %v reach the deadline %v", judge.Budget, local.ReplyMargin, local.ReadDeadline)
	}
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.fakes["a"].hold = make(chan struct{})
	cfg := j.window(t)
	cfg.Budget = 50 * time.Millisecond
	start(t, cfg)
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" || rep.Outcome.Reason != "maybe" {
		t.Fatalf("the give: %+v", rep.Outcome)
	}
	if got := j.judgedLines(t, rep.Outcome.ReceiptID); !reflect.DeepEqual(got, []string{`{"error":"timeout","judge":"a","outcome":"none","use":"decide"}`}) {
		t.Fatalf("judged lines %v", got)
	}
}

// TestARawBodyIsNeverSent gives a raw report: no judge is asked, each judge's line is none,
// raw-body, and the give is asked.
func TestARawBodyIsNeverSent(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.fakes["a"].verdict = "pass"
	start(t, j.window(t))
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "raw", base64.StdEncoding.EncodeToString([]byte{0, 1, 2})))
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" || j.fakes["a"].calls.Load() != 0 {
		t.Fatalf("a raw report: %+v, asked %d times", rep.Outcome, j.fakes["a"].calls.Load())
	}
	if got := j.judgedLines(t, rep.Outcome.ReceiptID); !reflect.DeepEqual(got, []string{`{"error":"raw-body","judge":"a","outcome":"none","use":"decide"}`}) {
		t.Fatalf("judged lines %v", got)
	}
}

// TestNoJudgeAtRelease answers a judged question with an accept and release, and lets time pass: no
// judge is asked again (maybe-1, maybe-10).
func TestNoJudgeAtRelease(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	start(t, j.window(t))
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
		t.Fatalf("the report: %+v", rep.Outcome)
	}
	asked := j.fakes["a"].calls.Load()
	q := rep.Outcome.ReceiptID
	if qs := j.call(t, "decider", local.OpQuestions, map[string]any{}); len(qs.Questions) != 1 {
		t.Fatalf("questions: %+v", qs.Questions)
	}
	res := j.call(t, "decider", local.OpResolve, j.answerArgs(t, "decider", q, answer.Accepted, "A report."))
	if res.Outcome == nil || res.Release == nil || res.Release.Verdict != "pass" {
		t.Fatalf("the accept: %+v", res)
	}
	j.clock.Set(j.clock.Now().Add(2 * time.Hour))
	time.Sleep(100 * time.Millisecond)
	if n := j.fakes["a"].calls.Load(); n != asked {
		t.Fatalf("a judge was asked at release or a tick: %d, then %d", asked, n)
	}
}

// TestARequestHoldsOnlyKindBodyInstruction runs a judged give through the Jev adapter against a
// server that records each request: the request holds none of the message's id, its salt, the
// members, the Accord, the receipts or the role labels (docs/adr/0023-judges.md).
func TestARequestHoldsOnlyKindBodyInstruction(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var head bytes.Buffer
		_ = r.Header.Write(&head)
		mu.Lock()
		bodies = append(bodies, append(head.Bytes(), b...), []byte(r.URL.RequestURI()))
		mu.Unlock()
		_, _ = w.Write([]byte(`{"answers":{"verdict":{"type":"choice","choice":"pass","confidence":0.9}}}`))
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.loaded[0].Config.Host = strings.TrimPrefix(srv.URL, "https://")
	cfg := j.window(t)
	cfg.NewJudge = func(c judge.Config) judge.Judge { return jev.New(c, plantedKey, srv.Client().Transport) }
	start(t, cfg)
	first := j.give(t, "proposer", "applier", "n-1", "note", "A note.")
	rep := wait(t, j.giveAsync(t, "proposer", "applier", "message-id-planted", "report", "text", "A report."))
	if rep.Outcome == nil || rep.Outcome.Verdict != "pass" || rep.Outcome.Reason != "ok" {
		t.Fatalf("the report: %+v", rep.Outcome)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("%d requests", len(bodies)/2)
	}
	sent := bytes.Join(bodies, nil)
	if !bytes.Contains(sent, []byte(`"body":"A report."`)) || !bytes.Contains(sent, []byte(`"kind":"report"`)) {
		t.Fatalf("the request: %s", sent)
	}
	for name, s := range map[string]string{
		"the message id": "message-id-planted",
		"the salt":       hex.EncodeToString(saltOf("a salt of 32 bytes message-id-planted")),
		"the proposer":   key(t, "proposer").Reference(), "the applier": key(t, "applier").Reference(),
		"the mediator": key(t, "mediator").Reference(), "the Accord": j.accord, "a receipt": first.Outcome.ReceiptID,
		"the role decider": "decider", "the role applier": "applier", "the role proposer": "proposer",
		"the extension": "placket.tech",
	} {
		for _, form := range []string{s, strings.TrimPrefix(strings.TrimPrefix(s, "sha256:"), "ed25519:")} {
			if bytes.Contains(sent, []byte(form)) {
				t.Fatalf("the request holds %s", name)
			}
		}
	}
}

// TestTheJudgedFileIsDroppable gives a judged report, checks its judged file and that the receipts
// hold nothing of the model's output, removes judged/, and restarts: the receipts and the open
// question are as before (docs/adr/0023-judges.md).
func TestTheJudgedFileIsDroppable(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a", "b"}, uses("a", "decide", "b", "decide"), false)
	j.fakes["a"].verdict = "pass"
	r := start(t, j.window(t))
	passed := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	j.fakes["a"].verdict = ""
	asked := wait(t, j.giveAsync(t, "proposer", "applier", "r-2", "report", "text", "Another report."))
	if passed.Outcome.Verdict != "pass" || asked.Outcome.Verdict != "escalate" {
		t.Fatalf("the reports: %+v, %+v", passed.Outcome, asked.Outcome)
	}
	path := filepath.Join(j.journal, journal.DirJudged, strings.TrimPrefix(passed.Outcome.ReceiptID, "sha256:")+".jsonl")
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the judged file: %v", err)
	}
	if bytes.Contains(j.receipts(t), []byte("planted")) {
		t.Fatal("receipts.jsonl holds the model's output")
	}
	r.stop(t)
	before := j.receipts(t)
	if err := os.RemoveAll(filepath.Join(j.journal, journal.DirJudged)); err != nil {
		t.Fatal(err)
	}
	start(t, j.window(t))
	if got := j.receipts(t); !bytes.Equal(got[:len(before)], before) {
		t.Fatal("the restart changed the receipts")
	}
	if qs := j.call(t, "decider", local.OpQuestions, map[string]any{}); len(qs.Questions) != 0 {
		t.Fatalf("the restart left questions open: %+v", qs.Questions)
	}
	if rep := j.call(t, "applier", local.OpGet, map[string]any{}); rep.Delivery == nil {
		t.Fatal("the passed report is not queued after the restart")
	}
}

// TestNothingPrintsTheOutput gives judged reports whose judge's output is planted: neither the
// window's error writer nor any event holds it (secrets-never-print, error-text-no-content).
func TestNothingPrintsTheOutput(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.fakes["a"].verdict = "block"
	r := start(t, j.window(t))
	wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report."))
	r.stop(t)
	r.mu.Lock()
	defer r.mu.Unlock()
	printed := r.stderr.String()
	for _, e := range *r.events {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
			printed += fmt.Sprintf(verb, e)
		}
	}
	if strings.Contains(printed, "planted") || strings.Contains(printed, hex.EncodeToString([]byte("planted"))) {
		t.Fatal("an event or the error writer holds the model's output")
	}
}

// TestStoppingStopsTheJudges stops the window while a judge holds a give: Run returns once the judge
// has returned, no receipt is written for the give, and no judged line after.
func TestStoppingStopsTheJudges(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.fakes["a"].hold = make(chan struct{})
	r := start(t, j.window(t))
	pending := j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report.")
	for j.fakes["a"].calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	before := j.receipts(t)
	r.stop(t)
	if j.fakes["a"].returns.Load() != 1 {
		t.Fatal("Run returned before the judge did")
	}
	if res := <-pending; res.err == nil {
		t.Fatalf("the give was answered: %+v", res.rep)
	}
	time.Sleep(50 * time.Millisecond)
	if !bytes.Equal(j.receipts(t), before) {
		t.Fatal("a receipt was written for the cut give")
	}
	if _, err := os.Stat(filepath.Join(j.journal, journal.DirJudged)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("judged/ after the stop: %v", err)
	}
}

// TestAJudgedCallerIsBusy holds the proposer's judged give: its next give is refused judge-busy and
// writes nothing, its other calls are answered, and the decider's give is decided as usual. Released,
// the first give gets its receipt (crossing-4).
func TestAJudgedCallerIsBusy(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.fakes["a"].hold = make(chan struct{})
	start(t, j.window(t))
	pending := j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report.")
	for j.fakes["a"].calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	before := j.receipts(t)
	if rep := wait(t, j.giveAsync(t, "proposer", "applier", "n-1", "note", "text", "A note.")); rep.Door != local.JudgeBusy {
		t.Fatalf("the busy caller's give: %+v", rep)
	}
	if !bytes.Equal(j.receipts(t), before) {
		t.Fatal("the refused give wrote")
	}
	for _, op := range []string{local.OpGet, local.OpQuestions} {
		if rep := j.call(t, "proposer", op, map[string]any{}); rep.Door != "" || rep.Rejection != "" {
			t.Fatalf("the busy caller's %s: %+v", op, rep)
		}
	}
	if rep := j.call(t, "proposer", local.OpResult, map[string]any{"question": "sha256:" + strings.Repeat("0", 64)}); rep.Door != "" {
		t.Fatalf("the busy caller's result: %+v", rep)
	}
	if rep := wait(t, j.giveAsync(t, "decider", "proposer", "c-1", "comment", "text", "A comment.")); rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("the decider's give: %+v", rep)
	}
	close(j.fakes["a"].hold)
	if rep := wait(t, pending); rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
		t.Fatalf("the first give: %+v", rep.Outcome)
	}
	j.seqs["proposer"]--
	if rep := wait(t, j.giveAsync(t, "proposer", "applier", "n-2", "note", "text", "A note.")); rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("the next give: %+v", rep)
	}
}

// TestTheKeyIsWrittenNowhere runs judged gives through the Jev adapter with a planted key, against a
// server that answers and one that fails, and restarts: no file of the journal holds the key.
func TestTheKeyIsWrittenNowhere(t *testing.T) {
	t.Parallel()
	status := atomic.Int32{}
	status.Store(http.StatusOK)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"answers":{"verdict":{"type":"choice","choice":"block","confidence":0.9}}}`))
	}))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.StartTLS()
	defer srv.Close()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.loaded[0].Config.Host = strings.TrimPrefix(srv.URL, "https://")
	cfg := j.window(t)
	cfg.NewJudge = func(c judge.Config) judge.Judge { return jev.New(c, plantedKey, srv.Client().Transport) }
	r := start(t, cfg)
	if rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report.")); rep.Outcome.Verdict != "block" {
		t.Fatalf("the report: %+v", rep.Outcome)
	}
	status.Store(http.StatusInternalServerError)
	if rep := wait(t, j.giveAsync(t, "proposer", "applier", "r-2", "report", "text", "A report.")); rep.Outcome.Verdict != "escalate" {
		t.Fatalf("the failed request: %+v", rep.Outcome)
	}
	r.stop(t)
	start(t, cfg)
	for p, b := range snapshot(t, j.journal) {
		if strings.Contains(b, "planted") {
			t.Fatalf("%s holds the key", p)
		}
	}
}

// failingJudged is a Disk that fails every append under judged/, and does what the system does for
// the rest.
type failingJudged struct{}

func (failingJudged) Append(path string, b []byte) error {
	if filepath.Base(filepath.Dir(path)) == journal.DirJudged {
		return errors.New("injected")
	}
	return journal.OS.Append(path, b)
}
func (failingJudged) Create(path string, b []byte) error { return journal.OS.Create(path, b) }
func (failingJudged) Remove(path string) error           { return journal.OS.Remove(path) }

// TestAJudgedAppendThatFails fails the judged/ append of a judged give: the caller gets no reply,
// Run returns the journal's error, and receipts.jsonl holds the give's receipt (journal-synced).
func TestAJudgedAppendThatFails(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	j.disk = failingJudged{}
	r := start(t, j.window(t))
	res := <-j.giveAsync(t, "proposer", "applier", "r-1", "report", "text", "A report.")
	var le *local.Error
	if !errors.As(res.err, &le) || le.Code != local.CodeSocket {
		t.Fatalf("the call got %v, not no reply", res.err)
	}
	select {
	case err := <-r.done:
		var je *journal.Error
		if !errors.As(err, &je) || !strings.HasPrefix(je.File, journal.DirJudged+"/") {
			t.Fatalf("the window ended with %v", err)
		}
		r.cancel = nil
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not end")
	}
	if lines := bytes.Count(j.receipts(t), []byte("\n")); lines != 1 {
		t.Fatalf("receipts.jsonl holds %d lines", lines)
	}
}

// TestTheConfigsMustBeTheCards starts a window whose configs are not the card's: a config whose
// instruction differs by a byte, none for a judge the card lists, one the card does not list, and a
// card whose version is not this adapter's. Each is CodeJudge, and the journal stays unfounded; a
// window whose configs are the card's prints each judge's reach (docs/adr/0023-judges.md).
func TestTheConfigsMustBeTheCards(t *testing.T) {
	t.Parallel()
	j := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	other := configOf(t, "a")
	other.Config.Instruction += "."
	b, _ := json.Marshal(map[string]any{"name": "a", "provider": "jev", "host": "h.example", "model": "m", "threshold": 0,
		"instruction": other.Config.Instruction})
	c, _ := canonical.Form(b)
	other.Hash, _ = canonical.TypedHash(c)
	for name, judges := range map[string][]judge.Loaded{
		"another instruction":     {other},
		"no config":               nil,
		"a config the card lacks": {j.loaded[0], configOf(t, "b")},
		"the card's config twice": {j.loaded[0], j.loaded[0]},
	} {
		cfg := j.window(t)
		cfg.Judges = judges
		if err := local.Run(context.Background(), cfg); !isJudgeError(err) {
			t.Fatalf("%s: %v", name, err)
		}
		unfounded(t, j.journal)
	}
	v := newJudged(t, []string{"a"}, uses("a", "decide"), false)
	v.set = bytes.Replace(v.set, []byte(`"version":"`+jev.Version+`"`), []byte(`"version":"9.9.9"`), 1)
	v.accord, _ = canonical.TypedHash(v.set)
	if err := local.Run(context.Background(), v.window(t)); !isJudgeError(err) {
		t.Fatalf("another version: %v", err)
	}
	r := start(t, j.window(t))
	r.mu.Lock()
	started := (*r.events)[0]
	r.mu.Unlock()
	if want := []local.Reach{{Judge: "a", Host: "h.example", Model: "m", Kinds: []string{"report"}}}; started.Kind != local.EventStarted || !reflect.DeepEqual(started.Reach, want) {
		t.Fatalf("the start: %+v", started)
	}
}

func isJudgeError(err error) bool {
	var le *local.Error
	return errors.As(err, &le) && le.Code == local.CodeJudge
}

// unfounded fails the test unless the journal at dir reads Empty.
func unfounded(t *testing.T, dir string) {
	t.Helper()
	jn, err := journal.Open(dir, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer jn.Close()
	rec, err := jn.Read()
	if err != nil || !rec.Empty {
		t.Fatalf("the journal: %+v, %v", rec, err)
	}
	if _, err := os.Stat(filepath.Join(dir, journal.FileReceipts)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("receipts.jsonl: %v", err)
	}
}
