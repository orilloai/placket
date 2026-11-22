package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/errtext"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/mcp"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/founding"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

// worked is api/testdata/worked/: the founding set these tests run windows of, and its keys.
const worked = "../../api/testdata/worked"

// A world is two windows, of the worked founding set and of the same set with another operator, so
// another Accord, and the key file of each party.
type world struct {
	dir     string
	refs    map[string]string
	accords [2]string
	sockets [2]string
	sets    [2]string
}

func (w *world) key(name string) string { return filepath.Join(w.dir, name+".key") }

func newWorld(t *testing.T) *world {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(worked, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ks struct {
		Keys map[string]struct{ Seed, Actor string }
	}
	if err := json.Unmarshal(b, &ks); err != nil {
		t.Fatal(err)
	}
	d, err := os.MkdirTemp("/tmp", "plm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	w := &world{dir: d, refs: map[string]string{}}
	for name, k := range ks.Keys {
		if err := os.WriteFile(w.key(name), []byte(k.Seed+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		w.refs[name] = k.Actor
	}
	set, err := os.ReadFile(filepath.Join(worked, "founding.json"))
	if err != nil {
		t.Fatal(err)
	}
	set = bytes.TrimSuffix(set, []byte{'\n'})
	// Every party's opening; the second Accord seals the same declarations.
	openings := map[string][]byte{}
	files, _ := filepath.Glob(filepath.Join(worked, "*.opening.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		openings["ed25519:"+strings.TrimSuffix(filepath.Base(f), ".opening.json")] = bytes.TrimSuffix(b, []byte{'\n'})
	}
	other := bytes.Replace(set, []byte(`"operator":"`+w.refs["operator"]+`"`), []byte(`"operator":"`+w.refs["principal"]+`"`), 1)
	seed, _ := hex.DecodeString(ks.Keys["mediator"].Seed)
	mk, err := signing.NewKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range [][]byte{set, other} {
		w.accords[i], _ = canonical.TypedHash(s)
		w.sockets[i] = filepath.Join(d, "s"+strconv.Itoa(i))
		w.sets[i] = filepath.Join(d, "founding"+strconv.Itoa(i)+".json")
		if err := os.WriteFile(w.sets[i], s, 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		ready, done := make(chan struct{}), make(chan error, 1)
		go func() {
			done <- local.Run(ctx, local.Window{Key: mk, Founding: s, Openings: openings, Socket: w.sockets[i], Journal: filepath.Join(d, "j"+strconv.Itoa(i)),
				Now: time.Now, Idle: 20 * time.Millisecond, Ready: ready})
		}()
		select {
		case <-ready:
		case err := <-done:
			t.Fatalf("window %d did not start: %v", i, err)
		}
		t.Cleanup(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("window %d ended with %v", i, err)
			}
		})
		// The window starts the Accord proposed; each party views and accepts its view, which founds
		// it, and keeps both in its state directory, where placket mcp reads them (setup-contract-15).
		for _, n := range []string{"proposer", "applier", "decider"} {
			w.accept(t, n, i)
		}
	}
	return w
}

// accept has name view and accept its view of window i's Accord, kept in its state directory.
func (w *world) accept(t *testing.T, name string, i int) {
	t.Helper()
	c, err := client.Open(w.entry("", name, i), client.Options{Now: time.Now, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := c.View(context.Background()); err != nil || r.View == nil {
		t.Fatalf("%s's view of window %d: %+v, %v", name, i, r, err)
	}
	if r, _, err := c.Accept(context.Background()); err != nil || !r.IsAccept {
		t.Fatalf("%s's acceptance of window %d: %+v, %v", name, i, r, err)
	}
}

// entry is a labels config entry for name's key on window i.
func (w *world) entry(label, name string, i int) client.Mediator {
	return client.Mediator{Label: label, Accord: w.accords[i], Key: w.key(name), Founding: w.sets[i], Socket: w.sockets[i]}
}

// A session is a Serve on pipes, and the replies read so far, by id.
type session struct {
	in      *io.PipeWriter
	mu      sync.Mutex
	replies map[int]map[string]any
	n       int
	done    chan error
	stderr  *bytes.Buffer
}

func serve(t *testing.T, s *mcp.Server) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	stderr := &bytes.Buffer{}
	s.Stderr = stderr
	s.Options = client.Options{Now: time.Now, Rand: rand.Reader}
	ss := &session{in: inW, replies: map[int]map[string]any{}, done: make(chan error, 1), stderr: stderr}
	go func() { ss.done <- s.Serve(context.Background(), inR, outW); _ = outW.Close() }()
	go func() {
		r := bufio.NewReaderSize(outR, 1<<20)
		for {
			b, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var m map[string]any
			if json.Unmarshal(b, &m) != nil {
				continue
			}
			id, _ := m["id"].(float64)
			ss.mu.Lock()
			ss.replies[int(id)] = m
			ss.mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = inW.Close()
		select {
		case <-ss.done:
		case <-time.After(10 * time.Second):
			t.Error("Serve did not end")
		}
	})
	ss.request(t, "initialize", map[string]any{"protocolVersion": mcp.ProtocolVersion})
	return ss
}

// request sends a request and waits for its reply.
func (s *session) request(t *testing.T, method string, params any) map[string]any {
	t.Helper()
	return s.await(t, s.send(t, method, params), 40*time.Second)
}

func (s *session) send(t *testing.T, method string, params any) int {
	t.Helper()
	s.mu.Lock()
	s.n++
	id := s.n
	s.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if _, err := s.in.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
	return id
}

func (s *session) await(t *testing.T, id int, d time.Duration) map[string]any {
	t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		s.mu.Lock()
		r, ok := s.replies[id]
		s.mu.Unlock()
		if ok {
			return r
		}
	}
	t.Fatalf("no reply to request %d", id)
	return nil
}

// call calls a tool and returns its result: the object its text holds, or its error text.
func (s *session) call(t *testing.T, tool string, args map[string]any) (map[string]any, string) {
	t.Helper()
	r := s.request(t, "tools/call", map[string]any{"name": tool, "arguments": args})
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("%s: no result: %v", tool, r)
	}
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if res["isError"] == true {
		return nil, text
	}
	var o map[string]any
	if err := json.Unmarshal([]byte(text), &o); err != nil {
		t.Fatalf("%s: %v: %s", tool, err, text)
	}
	return o, ""
}

// TestTheToolList lists give, get, questions, result, cancel and mediators, and nothing else; cancel
// takes no action and no answer.
func TestTheToolList(t *testing.T) {
	t.Parallel()
	s := serve(t, &mcp.Server{})
	r := s.request(t, "tools/list", map[string]any{})
	var names []string
	for _, tool := range r["result"].(map[string]any)["tools"].([]any) {
		m := tool.(map[string]any)
		names = append(names, m["name"].(string))
		if m["name"] == "cancel" {
			props := m["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if _, a := props["action"]; a || props["answer"] != nil || len(props) != 2 {
				t.Fatalf("cancel's input schema: %v", props)
			}
		}
	}
	if strings.Join(names, " ") != "give get questions result cancel mediators" {
		t.Fatalf("the tools: %v", names)
	}
	if !strings.Contains(s.stderr.String(), "serving no mediator") {
		t.Fatalf("with no label: %q", s.stderr.String())
	}
	if r := s.request(t, "resolve", map[string]any{}); r["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("a method that is not: %v", r)
	}
	if r := s.request(t, "tools/call", map[string]any{"name": "resolve", "arguments": map[string]any{}}); r["error"].(map[string]any)["code"] != float64(-32602) {
		t.Fatalf("the tool resolve: %v", r)
	}
}

// TestAnAnsweringKeyIsRefused starts placket mcp with a proposing label, one whose key answers
// questions, and one whose founding set is missing, which is refused founding: a give on the answering label is answering-key with its sentence, mediators lists it as
// refused, and a give on the proposing label passes; an answering entry it is not started with, whose
// founding set and key file are missing, is listed and never read. With --allow-answering both are
// served.
func TestAnAnsweringKeyIsRefused(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	missing := w.entry("elsewhere", "decider", 1)
	missing.Founding, missing.Key = filepath.Join(w.dir, "no-founding.json"), filepath.Join(w.dir, "no.key")
	broken := w.entry("broken", "applier", 0)
	broken.Founding = filepath.Join(w.dir, "no-founding.json")
	cfg := client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0), w.entry("approve", "decider", 0), missing, broken}}
	s := serve(t, &mcp.Server{Config: cfg, Labels: []string{"merge", "approve", "broken"}})
	want := errtext.Text(errtext.FromMCP, string(mcp.CodeAnsweringKey))
	if _, e := s.call(t, "give", map[string]any{"mediator": "approve", "to": w.refs["proposer"], "kind": "answer", "text": "No."}); e != want {
		t.Fatalf("a give on the answering label: %q, want %q", e, want)
	}
	o, e := s.call(t, "mediators", map[string]any{})
	list, _ := o["mediators"].([]any)
	if e != "" || len(list) != 4 || list[0].(map[string]any)["served"] != true || list[1].(map[string]any)["refused"] != want ||
		list[2].(map[string]any)["served"] != false || list[2].(map[string]any)["refused"] != nil ||
		list[3].(map[string]any)["refused"] != errtext.Text(errtext.FromMCP, string(mcp.CodeFounding)) {
		t.Fatalf("mediators: %v %s", o, e)
	}
	if o, e := s.call(t, "give", map[string]any{"mediator": "merge", "to": w.refs["applier"], "kind": "note", "text": "A note."}); e != "" ||
		o["outcome"].(map[string]any)["verdict"] != "pass" {
		t.Fatalf("a give on the proposing label: %v %s", o, e)
	}
	both := serve(t, &mcp.Server{Config: cfg, Labels: []string{"merge", "approve"}, AllowAnswering: true})
	if o, _ := both.call(t, "mediators", map[string]any{}); o["mediators"].([]any)[1].(map[string]any)["served"] != true {
		t.Fatalf("with --allow-answering: %v", o)
	}
}

// TestGiveReturnsWhenAsked gives a proposal, which a maybe term with no judge asks about: give returns
// the question's id at once, and result with wait 30 returns the closing once the decider answers,
// with the answer.
func TestGiveReturnsWhenAsked(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	cfg := client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0)}}
	s := serve(t, &mcp.Server{Config: cfg, Labels: []string{"merge"}})
	began := time.Now()
	o, e := s.call(t, "give", map[string]any{"mediator": "merge", "to": w.refs["applier"], "kind": "proposal", "text": "Rename total."})
	q, _ := o["question"].(string)
	if e != "" || q == "" || o["outcome"].(map[string]any)["receiptId"] != q || time.Since(began) > 2*time.Second {
		t.Fatalf("give: %v %s, after %v", o, e, time.Since(began))
	}
	id := s.send(t, "tools/call", map[string]any{"name": "result", "arguments": map[string]any{"mediator": "merge", "question": q, "wait": 30}})
	time.Sleep(300 * time.Millisecond)
	d, err := client.Open(w.entry("", "decider", 0), client.Options{Now: time.Now, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Resolve(context.Background(), client.Answering{Question: q, Action: answer.Accepted, Text: "Yes.", Release: true}); err != nil {
		t.Fatal(err)
	}
	r := s.await(t, id, 5*time.Second)
	text := r["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var res struct {
		Open    bool
		Closing struct{ Word string }
		Answer  struct{ Text string }
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil || res.Open || res.Closing.Word != "accepted" || res.Answer.Text != "Yes." {
		t.Fatalf("result: %s", text)
	}
}

// TestOneServerReachesTwoMediators serves two labels of one key file on two windows: a give on each
// has a receipt of its own Accord, from that key; a mediator not served is unknown-mediator; and a
// wait above 60 is bad-arguments. An unlabelled entry is served by its Accord id.
func TestOneServerReachesTwoMediators(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	cfg := client.Config{Mediators: []client.Mediator{w.entry("approve", "proposer", 0), w.entry("merge", "proposer", 1), w.entry("", "applier", 1)}}
	s := serve(t, &mcp.Server{Config: cfg, Labels: []string{"approve", "merge", w.accords[1]}})
	for i, label := range []string{"approve", "merge"} {
		o, e := s.call(t, "give", map[string]any{"mediator": label, "to": w.refs["applier"], "kind": "note", "text": "A note."})
		if e != "" {
			t.Fatalf("%s: %s", label, e)
		}
		b, _ := json.Marshal(o["outcome"].(map[string]any)["receipt"])
		st, err := receipts.Decode(b)
		if err != nil || st.Predicate.Accord != w.accords[i] || st.Predicate.From != w.refs["proposer"] {
			t.Fatalf("%s: the receipt names %s from %s (%v)", label, st.Predicate.Accord, st.Predicate.From, err)
		}
	}
	if o, e := s.call(t, "get", map[string]any{"mediator": w.accords[1]}); e != "" || o["delivery"] == nil {
		t.Fatalf("a get by the Accord id: %v %s", o, e)
	}
	if _, e := s.call(t, "get", map[string]any{"mediator": "nope"}); e != errtext.Text(errtext.FromClient, string(client.CodeUnknownMediator)) {
		t.Fatalf("a mediator not served: %q", e)
	}
	if _, e := s.call(t, "get", map[string]any{"mediator": "approve", "wait": 61}); e != errtext.Text(errtext.FromMCP, string(mcp.CodeBadArguments)) {
		t.Fatalf("a wait above 60: %q", e)
	}
	if _, e := s.call(t, "give", map[string]any{"mediator": "approve", "to": w.refs["applier"], "kind": "note", "text": "x", "data": 1}); e != errtext.Text(errtext.FromMCP, string(mcp.CodeBadArguments)) {
		t.Fatalf("a give of text and data: %q", e)
	}
}

// TestCancelWithdrawsOnlyTheKeysOwnQuestion asks as the proposer's label and cancels: the question
// closes as cancelled, and result shows it. The applier's label cancelling another's question is
// not-an-answerer, with its sentence, and the question stays open.
func TestCancelWithdrawsOnlyTheKeysOwnQuestion(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	cfg := client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0), w.entry("watch", "applier", 0)}}
	s := serve(t, &mcp.Server{Config: cfg, Labels: []string{"merge", "watch"}})
	ask := func(text string) string {
		o, e := s.call(t, "give", map[string]any{"mediator": "merge", "to": w.refs["applier"], "kind": "proposal", "text": text})
		if e != "" {
			t.Fatal(e)
		}
		return o["question"].(string)
	}
	q1, q2 := ask("One."), ask("Two.")
	if o, e := s.call(t, "cancel", map[string]any{"mediator": "merge", "question": q1}); e != "" || o["outcome"].(map[string]any)["reason"] != "cancelled" {
		t.Fatalf("the asker's cancel: %v %s", o, e)
	}
	if o, e := s.call(t, "result", map[string]any{"mediator": "merge", "question": q1}); e != "" || o["closing"].(map[string]any)["word"] != "cancelled" {
		t.Fatalf("result after the cancel: %v %s", o, e)
	}
	if _, e := s.call(t, "cancel", map[string]any{"mediator": "watch", "question": q2}); e != errtext.Text(errtext.FromRejection, "not-an-answerer") {
		t.Fatalf("another key's cancel: %q", e)
	}
	if o, e := s.call(t, "result", map[string]any{"mediator": "merge", "question": q2}); e != "" || o["open"] != true {
		t.Fatalf("the question after another key's cancel: %v %s", o, e)
	}
}

// TestQuestionsLeavesOutWhatTheWindowMisstates asks two proposals, then serves the decider's label
// from a window that shows both, the first under a salt that does not make its ask's receipt's
// digest: questions returns the second and the window's cursor, and lists the first in misstated.
func TestQuestionsLeavesOutWhatTheWindowMisstates(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	s := serve(t, &mcp.Server{Config: client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0)}}, Labels: []string{"merge"}})
	var asked []string
	for _, text := range []string{"One.", "Two."} {
		o, e := s.call(t, "give", map[string]any{"mediator": "merge", "to": w.refs["applier"], "kind": "proposal", "text": text})
		if e != "" {
			t.Fatal(e)
		}
		asked = append(asked, o["question"].(string))
	}
	k, err := keyfile.Read(w.key("decider"))
	if err != nil {
		t.Fatal(err)
	}
	call, err := local.SignCall(k, local.OpQuestions, w.accords[0], time.Now(), []byte(strings.Repeat("n", 16)), nil)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct{ Questions []map[string]any }
	if err := json.Unmarshal(exchange(t, w.sockets[0], call), &rep); err != nil || len(rep.Questions) != 2 {
		t.Fatalf("the window shows %d questions: %v", len(rep.Questions), err)
	}
	rep.Questions[0]["salt"] = strings.Repeat("ab", 32)
	reply, _ := json.Marshal(map[string]any{"ok": true, "questions": rep.Questions, "cursor": "c"})
	reply, _ = canonical.Form(reply)
	fake := filepath.Join(w.dir, "fake")
	l, err := net.Listen("unix", fake)
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
			_, _ = bufio.NewReader(c).ReadBytes('\n')
			_, _ = c.Write(append(reply, '\n'))
			_ = c.Close()
		}
	}()
	approve := w.entry("approve", "decider", 0)
	approve.Socket = fake
	d := serve(t, &mcp.Server{Config: client.Config{Mediators: []client.Mediator{approve}}, Labels: []string{"approve"}, AllowAnswering: true})
	o, e := d.call(t, "questions", map[string]any{"mediator": "approve"})
	list, _ := o["questions"].([]any)
	if e != "" || len(list) != 1 || list[0].(map[string]any)["id"] != asked[1] || o["cursor"] != "c" {
		t.Fatalf("questions: %v %s", o, e)
	}
	if m, _ := o["misstated"].([]any); len(m) != 1 || m[0] != asked[0] {
		t.Fatalf("misstated: %v", o["misstated"])
	}
}

// exchange sends one line to the socket and returns the line it answers with.
func exchange(t *testing.T, socket string, line []byte) []byte {
	t.Helper()
	c, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
	b, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestALabelWithNoAcceptedViewIsRefused is U17's founding case: placket mcp reads what a label's key
// answers from the view it accepted, kept in its state directory (setup-contract-15). A label whose
// state directory keeps no view, or keeps a view its acceptance does not name, is refused founding;
// the label beside it, whose view was accepted, is served; and view and accept are no tools.
func TestALabelWithNoAcceptedViewIsRefused(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	none, changed := w.entry("none", "applier", 0), w.entry("changed", "proposer", 1)
	if err := os.Remove(filepath.Join(state.DefaultPath(none.Key, none.Accord), state.FileView)); err != nil {
		t.Fatal(err)
	}
	viewPath := filepath.Join(state.DefaultPath(changed.Key, changed.Accord), state.FileView)
	b, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(viewPath, bytes.Replace(b, []byte(`"crosses"`), []byte(`"blocked"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0), none, changed}}
	s := serve(t, &mcp.Server{Config: cfg, Labels: []string{"merge", "none", "changed"}})
	o, e := s.call(t, "mediators", map[string]any{})
	list, _ := o["mediators"].([]any)
	refused := errtext.Text(errtext.FromMCP, string(mcp.CodeFounding))
	if e != "" || len(list) != 3 || list[0].(map[string]any)["served"] != true || list[1].(map[string]any)["refused"] != refused ||
		list[2].(map[string]any)["refused"] != refused {
		t.Fatalf("mediators: %v %s", o, e)
	}
	for _, tool := range []string{"view", "accept"} {
		if r := s.request(t, "tools/call", map[string]any{"name": tool, "arguments": map[string]any{"mediator": "merge"}}); r["error"].(map[string]any)["code"] != float64(-32602) {
			t.Fatalf("the tool %s: %v", tool, r)
		}
	}
}

// TestAGetThatCannotRecordStillHandsOver gives the applier a note and calls get when the applier's
// state directory cannot take delivered.jsonl, made read-only before the file is: the tool error's
// text is the state directory's error, and the delivery the window handed over is beside it, as text
// and as structured content. A get after it, the directory writable again, finds the queue empty.
func TestAGetThatCannotRecordStillHandsOver(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	applier := w.entry("", "applier", 0)
	s := serve(t, &mcp.Server{Config: client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0), applier}}, Labels: []string{"merge", w.accords[0]}})
	if _, e := s.call(t, "give", map[string]any{"mediator": "merge", "to": w.refs["applier"], "kind": "note", "text": "A note."}); e != "" {
		t.Fatal(e)
	}
	dir := state.DefaultPath(applier.Key, applier.Accord)
	if _, err := os.Stat(filepath.Join(dir, state.FileDelivered)); !os.IsNotExist(err) {
		t.Fatalf("delivered.jsonl before the get: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	r := s.request(t, "tools/call", map[string]any{"name": "get", "arguments": map[string]any{"mediator": w.accords[0]}})
	res, _ := r["result"].(map[string]any)
	content, _ := res["content"].([]any)
	if res["isError"] != true || len(content) != 2 || content[0].(map[string]any)["text"] != errtext.Text(errtext.FromState, string(state.CodeState)) {
		t.Fatalf("get: %v", r)
	}
	var o struct{ Delivery struct{ Kind, Text string } }
	if err := json.Unmarshal([]byte(content[1].(map[string]any)["text"].(string)), &o); err != nil || o.Delivery.Kind != "note" || o.Delivery.Text != "A note." {
		t.Fatalf("get handed over %v (%v)", content[1], err)
	}
	if d, _ := res["structuredContent"].(map[string]any)["delivery"].(map[string]any); d == nil || d["text"] != "A note." {
		t.Fatalf("get's structured content: %v", res["structuredContent"])
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if o, e := s.call(t, "get", map[string]any{"mediator": w.accords[0]}); e != "" || o["delivery"] != nil {
		t.Fatalf("the get after: %v %s", o, e)
	}
}

// TestAResultThatCannotRecordStillHandsOver asks a question the decider accepts, and calls result
// when the proposer's state directory cannot take delivered.jsonl: the tool error's text is the state
// directory's error, and the closing and the answer the window handed over are beside it.
func TestAResultThatCannotRecordStillHandsOver(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	proposer := w.entry("merge", "proposer", 0)
	s := serve(t, &mcp.Server{Config: client.Config{Mediators: []client.Mediator{proposer}}, Labels: []string{"merge"}})
	o, e := s.call(t, "give", map[string]any{"mediator": "merge", "to": w.refs["applier"], "kind": "proposal", "text": "Rename total."})
	q, _ := o["question"].(string)
	if e != "" || q == "" {
		t.Fatalf("give: %v %s", o, e)
	}
	d, err := client.Open(w.entry("", "decider", 0), client.Options{Now: time.Now, Rand: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Resolve(context.Background(), client.Answering{Question: q, Action: answer.Accepted, Text: "Yes.", Release: true}); err != nil {
		t.Fatal(err)
	}
	dir := state.DefaultPath(proposer.Key, proposer.Accord)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	r := s.request(t, "tools/call", map[string]any{"name": "result", "arguments": map[string]any{"mediator": "merge", "question": q}})
	res, _ := r["result"].(map[string]any)
	content, _ := res["content"].([]any)
	if res["isError"] != true || len(content) != 2 || content[0].(map[string]any)["text"] != errtext.Text(errtext.FromState, string(state.CodeState)) {
		t.Fatalf("result: %v", r)
	}
	var got struct {
		Closing struct{ Word string }
		Answer  struct{ Text string }
	}
	if err := json.Unmarshal([]byte(content[1].(map[string]any)["text"].(string)), &got); err != nil || got.Closing.Word != "accepted" || got.Answer.Text != "Yes." {
		t.Fatalf("result handed over %v (%v)", content[1], err)
	}
}

// TestALabelWhoseAcceptanceDoesNotCheckIsRefused serves five labels of the proposer's key on one
// Accord, each with a state directory of its own that keeps the proposer's accepted view and an
// acceptance naming that view's typed hash, so the state directory takes each as accepted. The
// acceptance is, in turn: another member's, signed by that member's key; the proposer's, carrying
// another member's signature (row 19); the proposer's, signed by its key but of another Accord (row
// 20); the proposer's with one hex digit of its sig changed; and the proposer's own. placket mcp
// refuses the first four with founding and serves the last (setup-contract-15, setup-contract-16).
func TestALabelWhoseAcceptanceDoesNotCheckIsRefused(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	keyOf := func(name string) signing.Key {
		t.Helper()
		k, err := keyfile.Read(w.key(name))
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	proposer, applier := keyOf("proposer"), keyOf("applier")
	view, err := os.ReadFile(filepath.Join(state.DefaultPath(w.key("proposer"), w.accords[0]), state.FileView))
	if err != nil {
		t.Fatal(err)
	}
	view = bytes.TrimSuffix(view, []byte{'\n'})
	accept := func(accord string, k signing.Key) []byte {
		t.Helper()
		b, err := founding.Accept(accord, view, k)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	sigOf := func(b []byte) string {
		t.Helper()
		var a struct{ Sig string }
		if err := json.Unmarshal(b, &a); err != nil || a.Sig == "" {
			t.Fatalf("the acceptance's sig: %s, %v", b, err)
		}
		return a.Sig
	}
	own := accept(w.accords[0], proposer)
	others := accept(w.accords[0], applier)
	sig := sigOf(own)
	flipped := "1" + sig[1:]
	if sig[0] == '1' {
		flipped = "0" + sig[1:]
	}
	cases := []struct {
		label      string
		acceptance []byte
	}{
		{"another-member", others},
		{"another-signature", bytes.Replace(own, []byte(sig), []byte(sigOf(others)), 1)},
		{"another-accord", accept(w.accords[1], proposer)},
		{"changed-sig", bytes.Replace(own, []byte(sig), []byte(flipped), 1)},
		{"own", own},
	}
	cfg := client.Config{}
	var labels []string
	for _, c := range cases {
		dir := filepath.Join(w.dir, "state-"+c.label)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, b := range map[string][]byte{state.FileView: view, state.FileAcceptance: c.acceptance} {
			if err := os.WriteFile(filepath.Join(dir, name), append(append([]byte(nil), b...), '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		m := w.entry(c.label, "proposer", 0)
		m.State = dir
		cfg.Mediators = append(cfg.Mediators, m)
		labels = append(labels, c.label)
	}
	s := serve(t, &mcp.Server{Config: cfg, Labels: labels})
	o, e := s.call(t, "mediators", map[string]any{})
	list, _ := o["mediators"].([]any)
	if e != "" || len(list) != len(cases) {
		t.Fatalf("mediators: %v %s", o, e)
	}
	refused := errtext.Text(errtext.FromMCP, string(mcp.CodeFounding))
	for i, c := range cases {
		got := list[i].(map[string]any)
		if c.label == "own" {
			if got["served"] != true {
				t.Errorf("%s: %v, want served", c.label, got)
			}
		} else if got["refused"] != refused {
			t.Errorf("%s: %v, want refused %q", c.label, got, refused)
		}
	}
}
