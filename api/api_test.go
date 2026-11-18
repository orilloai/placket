package api_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/mcp"
	"github.com/orilloai/placket/core/answer"
	"github.com/orilloai/placket/core/signing"
)

// update rewrites testdata/replies/ from this run's window instead of comparing with it.
var update = flag.Bool("update", false, "rewrite testdata/replies/")

// The worked calls' directory, the recorded replies' and the ops in the order the window is sent them.
const (
	workedDir  = "testdata/worked"
	repliesDir = "testdata/replies"
)

var ops = []string{local.OpGive, local.OpQuestions, local.OpGet, local.OpResult, local.OpResolve}

// A worked call, as the generator wrote it.
type worked struct {
	Op, Key, Seed, At, Nonce, Bytes, Sig string
	MessageID                            string          `json:"messageId"`
	Salt                                 string          `json:"salt"`
	Seq                                  int64           `json:"seq"`
	AnswerSalt                           string          `json:"answerSalt"`
	Request                              map[string]any  `json:"request"`
	Call                                 json.RawMessage `json:"call"`
}

func readWorked(t *testing.T, op string) worked {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(workedDir, op+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var w worked
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	return w
}

func keyOf(t *testing.T, seed string) signing.Key {
	t.Helper()
	s, err := hex.DecodeString(seed)
	if err != nil {
		t.Fatal(err)
	}
	k, err := signing.NewKey(s)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The worked set's keys and Accord id.
type keys struct {
	Accord string
	Keys   map[string]struct{ Seed, Actor string }
}

func readKeys(t *testing.T) keys {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(workedDir, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	var k keys
	if err := json.Unmarshal(b, &k); err != nil {
		t.Fatal(err)
	}
	return k
}

// shortDir is a directory short enough for a unix socket.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "pla")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// TestTheWorkedCallsAreTheGenerators runs the generator afresh: what it writes is exactly what
// testdata/worked/ holds, file for file.
func TestTheWorkedCallsAreTheGenerators(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("go", "run", "./internal/gen", "-dir", dir).CombinedOutput(); err != nil {
		t.Fatalf("the generator: %v\n%s", err, out)
	}
	fresh, _ := filepath.Glob(filepath.Join(dir, "*"))
	committed, _ := filepath.Glob(filepath.Join(workedDir, "*"))
	if len(fresh) != len(committed) || len(fresh) == 0 {
		t.Fatalf("the generator wrote %d files; testdata/worked holds %d", len(fresh), len(committed))
	}
	for _, f := range fresh {
		a, _ := os.ReadFile(f)
		b, err := os.ReadFile(filepath.Join(workedDir, filepath.Base(f)))
		if err != nil || !bytes.Equal(a, b) {
			t.Errorf("%s differs from a fresh go run ./api/internal/gen: %v", filepath.Base(f), err)
		}
	}
}

// counter is a source of bytes the same each run: a hash chain of its name.
type counter struct {
	mu   sync.Mutex
	name string
	n    uint64
}

func (c *counter) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < len(p); i += 32 {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], c.n)
		c.n++
		h := sha256.Sum256(append([]byte(c.name), b[:]...))
		copy(p[i:], h[:])
	}
	return len(p), nil
}

// proxy listens at a socket of its own and hands each call to the window at target, keeping the
// reply of each call by op, in the order they came.
type proxy struct {
	mu      sync.Mutex
	replies map[string][][]byte
}

func startProxy(t *testing.T, sock, target string) *proxy {
	t.Helper()
	p := &proxy{replies: map[string][][]byte{}}
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() { _ = l.Close(); wg.Wait() })
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			line, err := bufio.NewReader(c).ReadBytes('\n')
			if err == nil {
				var call struct {
					Call struct{ Op string } `json:"call"`
				}
				_ = json.Unmarshal(line, &call)
				reply := send(t, target, line)
				p.mu.Lock()
				p.replies[call.Call.Op] = append(p.replies[call.Call.Op], reply)
				p.mu.Unlock()
				_, _ = c.Write(append(append([]byte(nil), reply...), '\n'))
			}
			_ = c.Close()
		}
	}()
	return p
}

// last is the last reply the proxy kept for op.
func (p *proxy) last(op string) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	l := p.replies[op]
	return l[len(l)-1]
}

// send writes one line to the socket and returns the reply, its line feed removed.
func send(t *testing.T, sock string, line []byte) []byte {
	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer c.Close()
	if !bytes.HasSuffix(line, []byte{'\n'}) {
		line = append(append([]byte(nil), line...), '\n')
	}
	if _, err := c.Write(line); err != nil {
		t.Error(err)
		return nil
	}
	b, err := bufio.NewReaderSize(c, 1<<20).ReadBytes('\n')
	if err != nil {
		t.Error(err)
	}
	return bytes.TrimSuffix(b, []byte{'\n'})
}

// startWindow runs a window of the worked founding set, its clock at the worked calls' time and its
// cursors' source fixed, and returns its socket.
func startWindow(t *testing.T, k keys, now time.Time) string {
	t.Helper()
	set, err := os.ReadFile(filepath.Join(workedDir, "founding.json"))
	if err != nil {
		t.Fatal(err)
	}
	d := shortDir(t)
	sock := filepath.Join(d, "w")
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- local.Run(ctx, local.Window{Key: keyOf(t, k.Keys["mediator"].Seed), Founding: bytes.TrimSuffix(set, []byte{'\n'}),
			Socket: sock, Journal: filepath.Join(d, "j"), Now: func() time.Time { return now }, Idle: 20 * time.Millisecond,
			CursorRand: bytes.NewReader(make([]byte, 16)), Ready: ready})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("the window did not start: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the window did not start")
	}
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("the window ended with %v", err)
		}
	})
	return sock
}

// TestTheWireSchemasDescribeTheDoor starts a window of the worked founding set, sends it each op's
// worked call and keeps each reply in testdata/replies/<op>.json; then, through adapters/client and
// against the question the worked give opened, the decider's accept, the applier's get that waits
// and the get that follows, and the proposer's result, each kept as <op>.<what>.json. Every reply
// kept is the one committed, so the conformance package's TestAPISchemas validates the replies this
// window gives. Run with -update to write them.
func TestTheWireSchemasDescribeTheDoor(t *testing.T) {
	t.Parallel()
	k := readKeys(t)
	now, err := time.Parse(time.RFC3339, readWorked(t, local.OpGive).At)
	if err != nil {
		t.Fatal(err)
	}
	sock := startWindow(t, k, now)
	got := map[string][]byte{}
	for _, op := range ops {
		got[op] = send(t, sock, []byte(readWorked(t, op).Bytes))
	}
	rep, ok := local.ParseReply(got[local.OpGive])
	if !ok || rep.Outcome == nil || rep.Outcome.Verdict != "escalate" {
		t.Fatalf("the worked give: %s", got[local.OpGive])
	}
	question := rep.Outcome.ReceiptID
	p := startProxy(t, filepath.Join(filepath.Dir(sock), "p"), sock)
	open := func(name string) *client.Client {
		dir := t.TempDir()
		keyFile := filepath.Join(dir, name+".key")
		if err := os.WriteFile(keyFile, []byte(k.Keys[name].Seed+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := client.Open(client.Mediator{Accord: k.Accord, Key: keyFile, Socket: filepath.Join(filepath.Dir(sock), "p")},
			client.Options{Now: func() time.Time { return now }, Rand: &counter{name: name + " salts"}, Nonces: &counter{name: name + " nonces"}})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	ctx := context.Background()
	decider, applier, proposer := open("decider"), open("applier"), open("proposer")
	if r, _, err := decider.Resolve(ctx, client.Answering{Question: question, Action: answer.Accepted, Text: "Yes, rename it.", Release: true}); err != nil || r.Release == nil {
		t.Fatalf("the decider's accept: %+v, %v", r, err)
	}
	got["resolve.accepted"] = p.last(local.OpResolve)
	if g, err := applier.Get(ctx, 1, false); err != nil || g.Delivery == nil {
		t.Fatalf("the applier's get: %+v, %v", g, err)
	}
	got["get.queued"], got["get.delivery"] = p.replies[local.OpGet][0], p.replies[local.OpGet][1]
	if r, err := proposer.Result(ctx, question, 1); err != nil || r.Outcome == nil {
		t.Fatalf("the proposer's result: %+v, %v", r, err)
	}
	got["result.accepted"] = p.last(local.OpResult)
	if *update {
		if err := os.MkdirAll(repliesDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, b := range got {
		b = append(append([]byte(nil), b...), '\n')
		path := filepath.Join(repliesDir, name+".json")
		if *update {
			if err := os.WriteFile(path, b, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(b, want) {
			t.Errorf("%s: the reply is not the one committed (%v):\n%s", name, err, b)
		}
	}
	files, _ := filepath.Glob(filepath.Join(repliesDir, "*.json"))
	if !*update && len(files) != len(got) {
		t.Errorf("testdata/replies holds %d replies; the run kept %d", len(files), len(got))
	}
}

// recorder listens at a socket of its own, keeps the line of the one call it is sent, and replies
// {"ok":true}, sending nothing on: no nonce or seq is spent at a window.
func recorder(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	sock := filepath.Join(shortDir(t), "r")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan []byte, 1)
	go func() {
		defer l.Close()
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		line, _ := bufio.NewReader(c).ReadBytes('\n')
		out <- bytes.TrimSuffix(line, []byte{'\n'})
		_, _ = c.Write([]byte(`{"ok":true}` + "\n"))
	}()
	return sock, out
}

// TestTheClientSendsTheWorkedCalls has adapters/client make each op's call with the worked call's
// key, clock, nonce, and for a give its message id, salt and seq: the bytes it sends, caught at a
// recorder, are the worked call's, sig included. A resolve's client checks the question shown
// against a real receipt first, so its comparison is at the signing layer: local.SignCall with the
// worked call's key, time, nonce and args gives its bytes.
func TestTheClientSendsTheWorkedCalls(t *testing.T) {
	t.Parallel()
	k := readKeys(t)
	ctx := context.Background()
	for _, op := range []string{local.OpGive, local.OpGet, local.OpQuestions, local.OpResult} {
		w := readWorked(t, op)
		now, _ := time.Parse(time.RFC3339, w.At)
		sock, line := recorder(t)
		dir := t.TempDir()
		keyFile := filepath.Join(dir, "key")
		if err := os.WriteFile(keyFile, []byte(w.Seed+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rand := bytes.NewReader(append(unhex(t, w.MessageID), unhex(t, w.Salt)...))
		c, err := client.Open(client.Mediator{Accord: k.Accord, Key: keyFile, Socket: sock},
			client.Options{Now: func() time.Time { return now }, Rand: rand, Nonces: bytes.NewReader(unhex(t, w.Nonce))})
		if err != nil {
			t.Fatal(err)
		}
		wait := 0
		if v, ok := w.Request["wait"].(float64); ok {
			wait = int(v)
		}
		switch op {
		case local.OpGive:
			// A fresh state directory gives seq 1; a later seq is set as the one before it.
			if w.Seq > 1 {
				if err := c.Dir().SetSeq(w.Seq - 1); err != nil {
					t.Fatal(err)
				}
			}
			text := w.Request["text"].(string)
			_, _ = c.Give(ctx, client.Message{To: w.Request["to"].(string), Kind: w.Request["kind"].(string), Body: client.Body{Form: client.FormText, Text: text}}, false)
		case local.OpGet:
			_, _ = c.Get(ctx, wait, false)
		case local.OpQuestions:
			_, _ = c.Questions(ctx, wait, "")
		case local.OpResult:
			_, _ = c.Result(ctx, w.Request["question"].(string), wait)
		}
		select {
		case b := <-line:
			if string(b) != w.Bytes {
				t.Errorf("%s: the client sent\n%s\nnot the worked call\n%s", op, b, w.Bytes)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the client sent nothing", op)
		}
	}
	w := readWorked(t, local.OpResolve)
	var call struct {
		Call struct {
			Args map[string]any `json:"args"`
		} `json:"call"`
	}
	if err := json.Unmarshal(w.Call, &call); err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339, w.At)
	b, err := local.SignCall(keyOf(t, w.Seed), local.OpResolve, k.Accord, now, unhex(t, w.Nonce), call.Call.Args)
	if err != nil || string(b) != w.Bytes {
		t.Fatalf("resolve: local.SignCall gives\n%s\nnot the worked call\n%s (%v)", b, w.Bytes, err)
	}
}

// TestTheToolsTakeTheRequestLayer compares each tool placket mcp lists with its request schema,
// api/request/<tool>.json: the input schema is the request schema, the member mediator aside.
func TestTheToolsTakeTheRequestLayer(t *testing.T) {
	t.Parallel()
	tools, err := mcp.ToolList()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools {
		name := tool["name"].(string)
		names = append(names, name)
		b, err := os.ReadFile(filepath.Join("request", name+".json"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var want map[string]any
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatal(err)
		}
		in, _ := json.Marshal(tool["inputSchema"])
		var got map[string]any
		if err := json.Unmarshal(in, &got); err != nil {
			t.Fatal(err)
		}
		if props, ok := got["properties"].(map[string]any); ok && name != mcp.ToolMediators {
			if _, ok := props["mediator"]; !ok {
				t.Errorf("%s: no mediator", name)
			}
			delete(props, "mediator")
			var req []any
			for _, r := range got["required"].([]any) {
				if r != "mediator" {
					req = append(req, r)
				}
			}
			if req == nil {
				delete(got, "required")
			} else {
				got["required"] = req
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the input schema is not request/%s.json, mediator aside:\n%s", name, name, in)
		}
	}
	if strings.Join(names, " ") != "give get questions result cancel mediators" {
		t.Fatalf("the tools: %v", names)
	}
}

// TestTheReferenceIsGenerated runs the reference's generator afresh: docs/reference/mediator-api.md
// is exactly what it writes, so a schema changed without it fails here.
func TestTheReferenceIsGenerated(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "mediator-api.md")
	if b, err := exec.Command("go", "run", "./gen", "-o", out).CombinedOutput(); err != nil {
		t.Fatalf("the generator: %v\n%s", err, b)
	}
	fresh, _ := os.ReadFile(out)
	committed, err := os.ReadFile(filepath.Join("..", "docs", "reference", "mediator-api.md"))
	if err != nil || !bytes.Equal(fresh, committed) {
		t.Fatalf("docs/reference/mediator-api.md differs from a fresh go run ./api/gen (%v)", err)
	}
}
