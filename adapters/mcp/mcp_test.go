package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/errtext"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/mcp"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/canonical"
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
			done <- local.Run(ctx, local.Window{Key: mk, Founding: s, Socket: w.sockets[i], Journal: filepath.Join(d, "j"+strconv.Itoa(i)),
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
	}
	return w
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

// TestAnAnsweringKeyIsRefused starts placket mcp with a proposing label and one whose key answers
// questions: a give on the answering label is answering-key with its sentence, mediators lists it as
// refused, and a give on the proposing label passes; an answering entry it is not started with, whose
// founding set and key file are missing, is listed and never read. With --allow-answering both are
// served.
func TestAnAnsweringKeyIsRefused(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	missing := w.entry("elsewhere", "decider", 1)
	missing.Founding, missing.Key = filepath.Join(w.dir, "no-founding.json"), filepath.Join(w.dir, "no.key")
	cfg := client.Config{Mediators: []client.Mediator{w.entry("merge", "proposer", 0), w.entry("approve", "decider", 0), missing}}
	s := serve(t, &mcp.Server{Config: cfg, Labels: []string{"merge", "approve"}})
	want := errtext.Text(errtext.FromMCP, string(mcp.CodeAnsweringKey))
	if _, e := s.call(t, "give", map[string]any{"mediator": "approve", "to": w.refs["proposer"], "kind": "answer", "text": "No."}); e != want {
		t.Fatalf("a give on the answering label: %q, want %q", e, want)
	}
	o, e := s.call(t, "mediators", map[string]any{})
	list, _ := o["mediators"].([]any)
	if e != "" || len(list) != 3 || list[0].(map[string]any)["served"] != true || list[1].(map[string]any)["refused"] != want ||
		list[2].(map[string]any)["served"] != false || list[2].(map[string]any)["refused"] != nil {
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
