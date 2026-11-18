// Package mcp is placket mcp: a Model Context Protocol server on standard input and output, one
// party's client of the mediator API, written on the standard library. It speaks JSON-RPC 2.0, one
// message per line, at protocol version ProtocolVersion, with tools only: one per mediator API call
// it lists, give, get, questions and result; cancel, the asker's withdrawal of its own question; and
// mediators, the local listing. Resolve is never listed beyond cancel: placket mcp never accepts,
// changes or declines a question, never runs a command, and never holds a mediator's key.
//
// It serves only the labels of the labels config it is started with, each a mediator on its own: its
// key, its roles and its checks come from its own entry and founding set, and nothing about one is
// checked against another's. A label whose key holds a role that a maybe term's resolve.to names, in
// that label's own founding set, answers questions, and is refused (CodeAnsweringKey) unless the
// server allows answering keys; a label whose founding set cannot be read, or does not found, is
// refused too (CodeFounding). A label it is not started with has none of its files opened.
//
// Each tool's input schema is its request schema under api/request/, with mediator, the served
// label, or the Accord id of an entry with none. A tool error is MCP error text: the code and its one
// sentence of cause and fix, from adapters/errtext, by the row the code arrived under.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/errtext"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/api"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/setup"
)

// ProtocolVersion is the MCP version the server speaks.
const ProtocolVersion = "2025-06-18"

// MaxMessage is the most bytes of one JSON-RPC message the server reads: a give's body is at most
// local.MaxCall, and its JSON escapes may double it.
const MaxMessage = 4 * local.MaxCall

// The JSON-RPC error codes the server answers a message that is not a request it can take with.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
)

// The tools, in the order tools/list lists them.
const (
	ToolGive      = "give"
	ToolGet       = "get"
	ToolQuestions = "questions"
	ToolResult    = "result"
	ToolCancel    = "cancel"
	ToolMediators = "mediators"
)

// Tools is every tool's name, in the order tools/list lists them.
var Tools = []string{ToolGive, ToolGet, ToolQuestions, ToolResult, ToolCancel, ToolMediators}

// A Server is what Serve needs: the labels config, the names of the entries it serves, whether it
// serves a key that answers questions, what its clients take from the world, and the version it
// names itself with.
type Server struct {
	Config         client.Config
	Labels         []string
	AllowAnswering bool
	Options        client.Options
	Version        string
	// Stderr takes one line about what is served, at start.
	Stderr io.Writer
}

// A served is one label the server was started with: its entry, and the code and the row it is
// refused with, or none.
type served struct {
	name     string
	m        client.Mediator
	from     string
	code     string
	accepted bool
}

// Check reads each label the server is started with, as Serve does at its start, and returns, by
// label, the code each is refused with under its row, or "" for one served. A label the config does
// not name is CodeUnknownMediator, and Serve refuses to start.
func (s *Server) Check() (map[string]string, error) {
	list, err := s.serve()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, v := range list {
		out[v.name] = v.code
	}
	return out, nil
}

// serve reads each label the server is started with: its entry, its founding set and its key's
// roles in it, and opens its client once, to check the key and the state directory. Nothing of a
// label it is not started with is opened.
func (s *Server) serve() ([]*served, error) {
	var list []*served
	now := time.Now
	if s.Options.Now != nil {
		now = s.Options.Now
	}
	for _, name := range s.Labels {
		m, err := s.Config.Find(name)
		if err != nil {
			return nil, err
		}
		v := &served{name: name, m: m}
		list = append(list, v)
		set, err := os.ReadFile(m.Founding)
		if err != nil {
			v.from, v.code = errtext.FromMCP, string(CodeFounding)
			continue
		}
		set = []byte(strings.TrimSuffix(string(set), "\n"))
		id, err := canonical.TypedHash(set)
		a, ferr := setup.Found(set, local.Stamp(now()))
		if err != nil || ferr != nil || id != m.Accord {
			v.from, v.code = errtext.FromMCP, string(CodeFounding)
			continue
		}
		k, err := keyfile.Read(m.Key)
		if err != nil {
			v.from, v.code = from(err)
			continue
		}
		if answers(a, k.Reference()) && !s.AllowAnswering {
			v.from, v.code = errtext.FromMCP, string(CodeAnsweringKey)
			continue
		}
		c, err := client.Open(m, s.Options)
		if err != nil {
			v.from, v.code = from(err)
			continue
		}
		_ = c
		v.accepted = true
	}
	return list, nil
}

// answers reports whether the actor holds a role that some maybe term's resolve.to names: it answers
// questions under the Accord.
func answers(a *setup.Accord, actor string) bool {
	roles := map[string]bool{}
	for _, r := range a.Roles(actor) {
		roles[r] = true
	}
	for _, m := range a.Members() {
		for _, t := range a.Terms(m) {
			if t.Stance != setup.StanceMaybe || t.Resolve == nil {
				continue
			}
			for _, r := range t.Resolve.To {
				if roles[strings.TrimPrefix(r, "role:")] {
					return true
				}
			}
		}
	}
	return false
}

// from is the row an error arrived under, and its code: a client's, the state directory's, the key
// file's or the door client's own; any other is CodeInternal.
func from(err error) (string, string) {
	var ce *client.Error
	var se *state.Error
	var ke *keyfile.Error
	var le *local.Error
	var me *Error
	switch {
	case errors.As(err, &ce):
		return errtext.FromClient, string(ce.Code)
	case errors.As(err, &me):
		return errtext.FromMCP, string(me.Code)
	case errors.As(err, &se):
		return errtext.FromState, string(se.Code)
	case errors.As(err, &ke):
		return errtext.FromKeyfile, string(ke.Code)
	case errors.As(err, &le):
		return errtext.FromLocal, string(le.Code)
	}
	return errtext.FromMCP, string(CodeInternal)
}

// A message is one JSON-RPC message as the server reads it.
type message struct {
	JSONRPC *string         `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  *string         `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// conn is one session's output and the calls in flight.
type conn struct {
	mu  sync.Mutex
	out io.Writer
	// calls cancels a tools/call in flight by its id, for notifications/cancelled.
	calls map[string]context.CancelFunc
}

// write writes one message and a line feed.
func (c *conn) write(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.out.Write(append(b, '\n'))
}

func (c *conn) result(id json.RawMessage, v any) {
	c.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": v})
}

func (c *conn) fail(id json.RawMessage, code int, msg string) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	c.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

// Serve serves the session on in and out until in ends or ctx does, then cancels every tool call in
// flight and waits for each to end (goroutines-owned). Each tools/call runs on a goroutine of its
// own, with a client of its own, so a call that waits never holds another back; the state lock is
// taken by each client's own file, as two commands take it. A label the config does not name ends
// Serve before it reads anything, with CodeUnknownMediator.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	list, err := s.serve()
	if err != nil {
		return err
	}
	stderr := s.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if len(list) == 0 {
		_, _ = io.WriteString(stderr, "placket mcp: serving no mediator; start it with --label\n")
	}
	for _, v := range list {
		state := "served"
		if !v.accepted {
			state = "refused " + v.code
		}
		_, _ = io.WriteString(stderr, "placket mcp: "+v.name+" "+state+"\n")
	}
	ctx, stop := context.WithCancel(ctx)
	c := &conn{out: out, calls: map[string]context.CancelFunc{}}
	var wg sync.WaitGroup
	defer func() {
		stop()
		wg.Wait()
	}()
	lines := make(chan []byte)
	readErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(lines)
		r := bufio.NewReaderSize(in, 64<<10)
		for {
			b, err := readLine(r)
			if b != nil {
				select {
				case lines <- b:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					readErr <- err
				}
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case b, ok := <-lines:
			if !ok {
				return nil
			}
			s.handle(ctx, c, b, list, &wg)
		case <-readErr:
			return nil
		}
	}
}

// readLine reads one line, its line feed removed; a line longer than MaxMessage is read to its end
// and returned as an empty message, which is not JSON.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	long := false
	for {
		chunk, err := r.ReadSlice('\n')
		if !long {
			line = append(line, chunk...)
			if len(line) > MaxMessage+1 {
				long, line = true, nil
			}
		}
		if err == nil {
			if long {
				return []byte{}, nil
			}
			return line[:len(line)-1], nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(line) > 0 && !long {
			return line, err
		}
		return nil, err
	}
}

// handle answers one message.
func (s *Server) handle(ctx context.Context, c *conn, b []byte, list []*served, wg *sync.WaitGroup) {
	b = []byte(strings.TrimSpace(string(b)))
	if len(b) == 0 {
		return
	}
	var m message
	if !json.Valid(b) {
		c.fail(nil, codeParse, "parse error")
		return
	}
	if b[0] != '{' || json.Unmarshal(b, &m) != nil || m.JSONRPC == nil || *m.JSONRPC != "2.0" || m.Method == nil {
		c.fail(m.ID, codeInvalidRequest, "invalid request")
		return
	}
	notification := len(m.ID) == 0
	if !notification && !validID(m.ID) {
		c.fail(nil, codeInvalidRequest, "invalid request")
		return
	}
	switch *m.Method {
	case "initialize":
		c.result(m.ID, map[string]any{"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":   map[string]any{"name": "placket", "version": s.Version}})
	case "ping":
		c.result(m.ID, map[string]any{})
	case "tools/list":
		tools, err := ToolList()
		if err != nil {
			c.fail(m.ID, codeInvalidParams, "no tool list")
			return
		}
		c.result(m.ID, map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(m.Params, &p) != nil || !known(p.Name) {
			c.fail(m.ID, codeInvalidParams, "invalid params: no such tool")
			return
		}
		cctx, cancel := context.WithCancel(ctx)
		id := string(m.ID)
		c.mu.Lock()
		c.calls[id] = cancel
		c.mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				c.mu.Lock()
				delete(c.calls, id)
				c.mu.Unlock()
				cancel()
			}()
			c.result(m.ID, s.call(cctx, p.Name, p.Arguments, list))
		}()
	case "notifications/cancelled":
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			c.mu.Lock()
			if cancel, ok := c.calls[string(p.RequestID)]; ok {
				cancel()
			}
			c.mu.Unlock()
		}
	default:
		if !notification {
			c.fail(m.ID, codeNoMethod, "method not found")
		}
	}
}

// validID reports whether a request's id is a string or a number, as JSON-RPC takes it.
func validID(id json.RawMessage) bool {
	var v any
	if json.Unmarshal(id, &v) != nil {
		return false
	}
	switch v.(type) {
	case string, float64:
		return true
	}
	return false
}

func known(name string) bool {
	for _, t := range Tools {
		if t == name {
			return true
		}
	}
	return false
}

// descriptions are the tools' descriptions, as tools/list gives them.
var descriptions = map[string]string{
	ToolGive:      "Give one part to a member of a mediator's Accord. Returns its receipt; when the part is asked about, the question's id at once: follow with result and its wait.",
	ToolGet:       "Fetch the next delivery queued for the key, recorded before it is returned. With wait, wait for one.",
	ToolQuestions: "List the open questions the key may answer. With after, the cursor of an earlier result, only those opened since; with wait, wait for one.",
	ToolResult:    "How a question the key asked closed, and the answer or version after an accept or a change. With wait, wait for the closing.",
	ToolCancel:    "Withdraw a question the key asked. It never accepts, changes or declines.",
	ToolMediators: "List the mediators of the labels config, and whether each is served here.",
}

// ToolList is the tools/list result's tools: each tool's name, description and input schema, its
// request schema with mediator added, but mediators', which is its request schema as it is.
func ToolList() ([]map[string]any, error) {
	var out []map[string]any
	for _, name := range Tools {
		schema, err := InputSchema(name)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"name": name, "description": descriptions[name], "inputSchema": schema})
	}
	return out, nil
}

// InputSchema is a tool's input schema: its request schema, api/request/<name>.json (cancel's
// request/cancel.json), with the member mediator added and required, but for mediators.
func InputSchema(name string) (map[string]any, error) {
	b, err := api.Request(name)
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		return nil, err
	}
	if name == ToolMediators {
		return schema, nil
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	props["mediator"] = map[string]any{"type": "string", "minLength": 1,
		"description": "The mediator to call: its label in the labels config, or its Accord id when it has none."}
	schema["properties"] = props
	req, _ := schema["required"].([]any)
	schema["required"] = append([]any{"mediator"}, req...)
	return schema, nil
}
