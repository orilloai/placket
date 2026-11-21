package jev

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/orilloai/placket/adapters/judge"
)

// plantedKey is the key the tests hand the adapter: plain words, so the Makefile's secrets step
// takes it for no secret.
const plantedKey = "planted judge key words"

// A recorder is a TLS server on the loopback that records every request and answers with status
// and body.
type recorder struct {
	srv    *httptest.Server
	mu     sync.Mutex
	got    []recorded
	status int
	body   []byte
	header http.Header
}

type recorded struct {
	method, path string
	header       http.Header
	body         []byte
}

func newRecorder(t *testing.T, status int, body string) *recorder {
	t.Helper()
	r := &recorder{status: status, body: []byte(body), header: http.Header{}}
	r.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.got = append(r.got, recorded{req.Method, req.URL.RequestURI(), req.Header.Clone(), b})
		status, body := r.status, r.body
		for k, v := range r.header {
			w.Header()[k] = v
		}
		r.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	r.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	r.srv.StartTLS()
	t.Cleanup(r.srv.Close)
	return r
}

func (r *recorder) requests() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recorded(nil), r.got...)
}

func (r *recorder) host() string { return strings.TrimPrefix(r.srv.URL, "https://") }

// config is the review judge's config, at host.
func config(host string) judge.Config {
	return judge.Config{Name: "review", Provider: "jev", Host: host, Model: "jev-test-model", Threshold: 80,
		Instruction: "Block a part that deletes data."}
}

// ask runs the Jev judge of the recorder's host, through its trusted transport, on a request.
func (r *recorder) ask(t *testing.T, key string) (judge.Answer, error) {
	t.Helper()
	j := New(config(r.host()), key, r.srv.Client().Transport)
	return j.Judge(context.Background(), judge.Request{Kind: "patch", Instruction: "Block a part that deletes data.", Body: []byte("Drop the table.")})
}

func code(err error) judge.Code {
	var je *judge.Error
	if errors.As(err, &je) {
		return je.Code
	}
	return ""
}

// replyOf is a Jev reply whose one answer is choice, with the label and the confidence's JSON.
func replyOf(label, confidence string) string {
	c := ""
	if confidence != "" {
		c = `,"confidence":` + confidence
	}
	return `{"model":"jev-test-model","answers":{"verdict":{"type":"choice","choice":"` + label + `"` + c +
		`,"probabilities":{"pass":0.1,"block":0.9}}},"usage":{"input_tokens":3,"output_tokens":1}}`
}

// TestTheRequestCarriesOnlyKindBodyInstruction sends one request and reads what reached the host:
// a POST to the one path, whose content is the kind, the body and the instruction, beside the model
// and the adapter's fixed labels, with the key only as its bearer credential, and no other header
// carrying anything (docs/adr/0023-judges.md).
func TestTheRequestCarriesOnlyKindBodyInstruction(t *testing.T) {
	t.Parallel()
	r := newRecorder(t, http.StatusOK, replyOf("block", "0.93"))
	a, err := r.ask(t, plantedKey)
	if err != nil || a.Verdict != "block" || a.Score == nil || *a.Score != 93 || len(a.Output) != 0 {
		t.Fatalf("Judge = %v, %v, %v", a.Verdict, a.Score, err)
	}
	got := r.requests()
	if len(got) != 1 {
		t.Fatalf("%d requests", len(got))
	}
	req := got[0]
	want := `{"model":"jev-test-model","state":{"body":"Drop the table.","kind":"patch"},"questions":{"verdict":{"type":"choice",` +
		`"instructions":"Block a part that deletes data.","criteria":{"block":"The content does not meet the instruction.",` +
		`"pass":"The content meets the instruction."}}}}`
	if req.method != http.MethodPost || req.path != "/v1/systemone" || string(req.body) != want {
		t.Fatalf("sent %s %s %s", req.method, req.path, req.body)
	}
	names := []string{}
	for k := range req.header {
		names = append(names, k)
	}
	wantHeaders := map[string]string{"Authorization": "Bearer " + plantedKey, "Content-Type": "application/json",
		"Accept-Encoding": "gzip", "Content-Length": fmt.Sprint(len(want)), "User-Agent": "Go-http-client/1.1"}
	if len(req.header) != len(wantHeaders) {
		t.Fatalf("headers %v", names)
	}
	for k, v := range wantHeaders {
		if req.header.Get(k) != v {
			t.Fatalf("header %s = %q, want %q", k, req.header.Get(k), v)
		}
	}
}

// TestEveryFailureIsNone runs the judge against a host that fails each way: each is the error code
// the runner records as none.
func TestEveryFailureIsNone(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		status int
		body   string
		code   judge.Code
	}{
		{"a status that is not success", http.StatusInternalServerError, `{"detail":"no"}`, judge.CodeStatus},
		{"unauthorized", http.StatusUnauthorized, `{"detail":"no"}`, judge.CodeStatus},
		{"a reply too large", http.StatusOK, `{"pad":"` + strings.Repeat("a", judge.MaxOutput) + `"}`, judge.CodeTooLarge},
		{"a reply that is no JSON", http.StatusOK, `answer: pass`, judge.CodeUnparsable},
		{"a name twice", http.StatusOK, `{"answers":{"verdict":{"type":"choice","choice":"pass","choice":"block"}}}`, judge.CodeUnparsable},
		{"trailing data", http.StatusOK, replyOf("pass", "0.9") + ` {}`, judge.CodeUnparsable},
		{"a label of its own", http.StatusOK, replyOf("allow", "0.9"), judge.CodeUnparsable},
		{"a confidence above 1", http.StatusOK, replyOf("pass", "1.5"), judge.CodeUnparsable},
		{"a confidence as text", http.StatusOK, replyOf("pass", `"0.9"`), judge.CodeUnparsable},
		{"a confidence of null", http.StatusOK, replyOf("pass", "null"), judge.CodeUnparsable},
		{"no answer", http.StatusOK, `{"answers":{}}`, judge.CodeUnparsable},
		{"an answer of another type", http.StatusOK, `{"answers":{"verdict":{"type":"noul","noul":0.9}}}`, judge.CodeUnparsable},
		{"a name of another case", http.StatusOK, `{"Answers":{"verdict":{"type":"choice","choice":"pass"}}}`, judge.CodeUnparsable},
	} {
		r := newRecorder(t, c.status, c.body)
		if _, err := r.ask(t, plantedKey); code(err) != c.code {
			t.Fatalf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	// No key: nothing is sent.
	r := newRecorder(t, http.StatusOK, replyOf("pass", "0.9"))
	if _, err := r.ask(t, ""); code(err) != judge.CodeNoKey || len(r.requests()) != 0 {
		t.Fatalf("no key: %v, %d requests", err, len(r.requests()))
	}
	// A host that is gone.
	gone := newRecorder(t, http.StatusOK, replyOf("pass", "0.9"))
	gone.srv.Close()
	if _, err := gone.ask(t, plantedKey); code(err) != judge.CodeUnreachable {
		t.Fatalf("a host that is gone: %v", err)
	}
	// No confidence: no score, and the label stands.
	r = newRecorder(t, http.StatusOK, replyOf("pass", ""))
	if a, err := r.ask(t, plantedKey); err != nil || a.Verdict != "pass" || a.Score != nil {
		t.Fatalf("no confidence: %v, %v, %v", a.Verdict, a.Score, err)
	}
	// A context done: its error, which the runner reads as timeout.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	j := New(config(r.host()), plantedKey, r.srv.Client().Transport)
	if _, err := j.Judge(ctx, judge.Request{Kind: "patch", Body: []byte("x")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a context done: %v", err)
	}
}

// TestNoRedirectNoProxy answers with a redirect to another host: that host gets no request, and the
// judge says redirect. The transport New builds takes no proxy from the environment.
func TestNoRedirectNoProxy(t *testing.T) {
	t.Parallel()
	other := newRecorder(t, http.StatusOK, replyOf("pass", "0.9"))
	r := newRecorder(t, http.StatusTemporaryRedirect, "")
	r.header.Set("Location", other.srv.URL+"/v1/systemone")
	if _, err := r.ask(t, plantedKey); code(err) != judge.CodeRedirect || len(other.requests()) != 0 {
		t.Fatalf("a redirect: %v, %d requests to the other host", err, len(other.requests()))
	}
	j := New(config("h.example"), plantedKey, nil).(*jev)
	tr, ok := j.client.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil || tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("the transport: %T, proxy set %v", j.client.Transport, ok && tr.Proxy != nil)
	}
}

// TestUntrustedTLSIsUnreachable runs the judge New builds with no transport of the test's against a
// TLS server whose certificate it does not trust: unreachable, and the server records no request.
func TestUntrustedTLSIsUnreachable(t *testing.T) {
	t.Parallel()
	r := newRecorder(t, http.StatusOK, replyOf("pass", "0.9"))
	j := New(config(r.host()), plantedKey, nil)
	if _, err := j.Judge(context.Background(), judge.Request{Kind: "patch", Body: []byte("x")}); code(err) != judge.CodeUnreachable || len(r.requests()) != 0 {
		t.Fatalf("an untrusted certificate: %v, %d requests", err, len(r.requests()))
	}
}

// TestTheJudgeNeverPrintsItsKey prints the judge every way, and its failures: no key shows
// (secrets-never-print, error-text-no-content).
func TestTheJudgeNeverPrintsItsKey(t *testing.T) {
	t.Parallel()
	r := newRecorder(t, http.StatusInternalServerError, `{"detail":"`+plantedKey+`"}`)
	j := New(config(r.host()), plantedKey, r.srv.Client().Transport)
	_, err := j.Judge(context.Background(), judge.Request{Kind: "patch", Body: []byte("x")})
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%q"} {
		got := fmt.Sprintf(verb, j) + fmt.Sprintf(verb, []any{j}) + fmt.Sprintf(verb, err)
		if strings.Contains(got, "planted") || strings.Contains(got, hex.EncodeToString([]byte("planted"))) {
			t.Fatalf("%s prints the key: %s", verb, got)
		}
	}
	if got := fmt.Sprint(j); got != "jev.judge" {
		t.Fatalf("the judge prints %q", got)
	}
}

// replyCases are replies readReply reads, and some it refuses, for its tests and its fuzz test's
// seeds.
var replyCases = []struct {
	reply string
	ok    bool
	score int
}{
	{replyOf("pass", "0.9"), true, 90},
	{replyOf("block", "0.5"), true, 50},
	{replyOf("block", "0"), true, 0},
	{replyOf("pass", "1"), true, 100},
	{replyOf("pass", "0.875"), true, 88},
	{replyOf("pass", "-0.1"), false, 0},
	{replyOf("pass", "null"), false, 0},
	{`{"answers":{"verdict":{"type":"choice","choice":"pass"}}}`, true, -1},
	{`[]`, false, 0},
	{`{"answers":null}`, false, 0},
	{`{"answers":{"verdict":{"type":"choice","choice":"pass"}}}x`, false, 0},
	{`{"a":{"b":{"c":1,"c":2}},"answers":{"verdict":{"type":"choice","choice":"pass"}}}`, false, 0},
	{strings.Repeat("[", 100) + strings.Repeat("]", 100), false, 0},
}

// TestReadReply reads each reply: its verdict and score, or a refusal.
func TestReadReply(t *testing.T) {
	t.Parallel()
	for _, c := range replyCases {
		a, ok := readReply([]byte(c.reply))
		if ok != c.ok {
			t.Fatalf("%s: read %v", c.reply, ok)
		}
		if !ok {
			continue
		}
		if (c.score < 0) != (a.Score == nil) || (a.Score != nil && *a.Score != c.score) {
			t.Fatalf("%s: score %v, want %d", c.reply, a.Score, c.score)
		}
	}
	if b, _ := requestBody("m", judge.Request{Kind: "k", Body: []byte("<a> & b")}); !bytes.Equal(b,
		[]byte(`{"model":"m","state":{"body":"<a> & b","kind":"k"},"questions":{"verdict":{"type":"choice","instructions":"",`+
			`"criteria":{"block":"The content does not meet the instruction.","pass":"The content meets the instruction."}}}}`)) {
		t.Fatal("the request body escapes the body's text")
	}
}

// TestTheDocsNameTheFixedWords reads each place the docs say what a judge's request carries, and
// finds in it the adapter's fixed words beside the kind, the body and the instruction: the path, the
// one question and its type, and the two criteria's keys and sentences, as requestBody writes them.
func TestTheDocsNameTheFixedWords(t *testing.T) {
	t.Parallel()
	words := []string{"`" + path + "`", "`" + question + "`", "`choice`", "`pass`", "`block`", criterionPass, criterionBlock}
	places := []struct{ file, start string }{
		{"SECURITY.md", "- A judge's request "},
		{"SECURITY.md", "## What a judge sends"},
		{"README.md", "## Judges"},
		{"docs/security/threat-model.md", "- **P21."},
		{"docs/security/threat-model.md", "## The Jev data flow"},
		{"docs/adr/0023-judges.md", "### The key, and what leaves the machine"},
	}
	for _, p := range places {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", p.file))
		if err != nil {
			t.Fatal(err)
		}
		text, ok := docPlace(string(b), p.start)
		if !ok {
			t.Fatalf("%s: no %q", p.file, p.start)
		}
		for _, w := range words {
			if !strings.Contains(text, w) {
				t.Errorf("%s, %q: does not name %s", p.file, p.start, w)
			}
		}
	}
}

// docPlace returns the place of a Markdown doc that starts with the line start: a heading's section,
// to the next heading of its level or above outside a code block, or else the one line.
func docPlace(doc, start string) (string, bool) {
	lines := strings.Split(doc, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, start) {
			continue
		}
		level := strings.Index(l, "# ") + 1
		if !strings.HasPrefix(l, "#") {
			return l, true
		}
		end, fence := len(lines), false
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "```") {
				fence = !fence
			}
			if h := strings.Index(lines[j], "# "); !fence && strings.HasPrefix(lines[j], "#") && h+1 <= level {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n"), true
	}
	return "", false
}
