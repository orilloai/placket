package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/errtext"
)

// The tool calls of the server's own tests, each a tool and its arguments, with the members of every
// tool's input schema, a wait of each kind and an after: the seeds of the fuzz tests that read a
// message or a tool's arguments.
var seedCalls = []struct {
	tool string
	args string
}{
	{ToolGive, `{"kind":"answer","mediator":"approve","text":"No.","to":"ed25519:` + strings.Repeat("ab", 32) + `"}`},
	{ToolGive, `{"kind":"note","mediator":"merge","text":"A note.","to":"ed25519:` + strings.Repeat("cd", 32) + `"}`},
	{ToolGive, `{"data":{"a":[1,"b"]},"kind":"proposal","mediator":"merge","re":"m","topic":"t","to":"ed25519:` + strings.Repeat("cd", 32) + `"}`},
	{ToolGive, `{"data":1,"kind":"note","mediator":"approve","text":"x","to":"ed25519:` + strings.Repeat("cd", 32) + `"}`},
	{ToolGet, `{"mediator":"sha256:` + strings.Repeat("ef", 32) + `"}`},
	{ToolGet, `{"mediator":"nope"}`},
	{ToolGet, `{"mediator":"approve","wait":61}`},
	{ToolGet, `{"mediator":"approve","wait":60}`},
	{ToolQuestions, `{"mediator":"approve","wait":10}`},
	{ToolQuestions, `{"after":"` + strings.Repeat("0a", 16) + `","mediator":"approve","wait":0}`},
	{ToolQuestions, `{"after":"","mediator":"approve"}`},
	{ToolResult, `{"mediator":"merge","question":"sha256:` + strings.Repeat("12", 32) + `","wait":30}`},
	{ToolResult, `{"mediator":"merge","question":"sha256:` + strings.Repeat("12", 32) + `"}`},
	{ToolCancel, `{"mediator":"merge","question":"sha256:` + strings.Repeat("34", 32) + `"}`},
	{ToolCancel, `{"action":"accepted","mediator":"watch","question":"sha256:` + strings.Repeat("34", 32) + `"}`},
	{ToolMediators, `{}`},
	{ToolMediators, `null`},
	{ToolMediators, ``},
}

// fuzzServer is a server whose one label, approve, was refused at start, so no call it takes opens a
// key file, a state directory or a socket.
func fuzzServer() (*Server, []*served) {
	m := client.Mediator{Label: "approve", Accord: "sha256:" + strings.Repeat("ef", 32)}
	return &Server{Config: client.Config{Mediators: []client.Mediator{m}}, Labels: []string{"approve"}},
		[]*served{{name: "approve", m: m, from: errtext.FromMCP, code: string(CodeAnsweringKey)}}
}

// errorText is the form of a tool error's text: a code, then its sentence.
var errorText = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*: .`)

// FuzzReadLine reads any bytes as the server's input, through a buffer far smaller than a message, so
// a line runs over it. It never panics, ends at the input's end, and every line it returns holds no
// line feed and at most MaxMessage bytes. Its seeds are the messages of the server's own tests, one to
// a line, with a last line that has no line feed.
func FuzzReadLine(f *testing.F) {
	for _, c := range seedCalls {
		f.Add([]byte(`{"id":1,"jsonrpc":"2.0","method":"tools/call","params":{"arguments":` + orEmpty(c.args) + `,"name":"` + c.tool + `"}}` + "\n"))
	}
	f.Add([]byte(`{"id":1,"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"` + ProtocolVersion + `"}}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	f.Add([]byte("\n\n\r\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		r := bufio.NewReaderSize(bytes.NewReader(b), 16)
		for range len(b) + 2 {
			line, err := readLine(r)
			if bytes.IndexByte(line, '\n') >= 0 || len(line) > MaxMessage {
				t.Fatalf("a line of %d bytes, with a line feed: %q", len(line), line)
			}
			if err != nil {
				return
			}
		}
		t.Fatal("the input never ended")
	})
}

// FuzzHandle answers any bytes as one message from the agent. It never panics, and every message it
// writes is one JSON-RPC 2.0 response, a result or an error, on a line of its own. Its seeds are the
// requests of the server's own tests, notifications and messages that are not requests.
func FuzzHandle(f *testing.F) {
	for _, c := range seedCalls {
		f.Add([]byte(`{"id":1,"jsonrpc":"2.0","method":"tools/call","params":{"arguments":` + orEmpty(c.args) + `,"name":"` + c.tool + `"}}`))
	}
	for _, m := range []string{
		`{"id":"a","jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"` + ProtocolVersion + `"}}`,
		`{"id":2,"jsonrpc":"2.0","method":"tools/list","params":{}}`,
		`{"id":3,"jsonrpc":"2.0","method":"ping"}`,
		`{"id":4,"jsonrpc":"2.0","method":"resolve","params":{}}`,
		`{"id":5,"jsonrpc":"2.0","method":"tools/call","params":{"arguments":{},"name":"resolve"}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"id":{},"jsonrpc":"2.0","method":"ping"}`,
		`{"id":6,"jsonrpc":"1.0","method":"ping"}`,
		`[1]`,
		`{`,
	} {
		f.Add([]byte(m))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		s, list := fuzzServer()
		var out bytes.Buffer
		c := &conn{out: &out, calls: map[string]context.CancelFunc{}}
		var wg sync.WaitGroup
		s.handle(context.Background(), c, b, list, &wg)
		wg.Wait()
		for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if line == "" && out.Len() == 0 {
				continue
			}
			var r struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Result  json.RawMessage `json:"result"`
				Error   json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal([]byte(line), &r); err != nil || r.JSONRPC != "2.0" || len(r.ID) == 0 || (len(r.Result) == 0) == (len(r.Error) == 0) {
				t.Fatalf("wrote %q", line)
			}
		}
	})
}

// FuzzCall runs any tool's name with any arguments, as a tools/call carries them. It never panics,
// and every result has content; a tool error's text is a code and its sentence. Its seeds are the tool
// calls of the server's own tests.
func FuzzCall(f *testing.F) {
	for _, c := range seedCalls {
		f.Add(c.tool, []byte(c.args))
	}
	f.Fuzz(func(t *testing.T, name string, args []byte) {
		if !known(name) {
			return
		}
		s, list := fuzzServer()
		r := s.call(context.Background(), name, args, list)
		content, _ := r["content"].([]any)
		if len(content) != 1 {
			t.Fatalf("a result with content %v", r["content"])
		}
		text, _ := content[0].(map[string]any)["text"].(string)
		if r["isError"] == true && !errorText.MatchString(text) {
			t.Fatalf("a tool error's text %q", text)
		}
	})
}

// orEmpty is the arguments of a seed as a message carries them: absent ones as null.
func orEmpty(args string) string {
	if args == "" {
		return "null"
	}
	return args
}
