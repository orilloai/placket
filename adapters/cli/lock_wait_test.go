package cli_test

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
)

// TestAWaitHoldsNoLock parks, on one key's state directory, the commands that wait: watch on the
// decider's key, while the printed answer line runs on that key; and result --wait 30, get --wait 30
// and questions --wait 30 on the proposer's, while a give runs on it. Each command that runs beside
// a wait completes at once (state-lock).
func TestAWaitHoldsNoLock(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	e, _ := labelsEnv(t, a.entry("decider", "approve"), a.entry("proposer", "merge"))
	g := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", "One.")
	q := last(g.stdout)
	w := background(t, e, strings.NewReader(""), "watch", "--label", "approve", "--count", "2")
	lines := w.until(t, 8)
	began := time.Now()
	if r := run(t, e, strings.Fields(lines[5])[1:]...); r.code != 0 || time.Since(began) > 2*time.Second {
		t.Fatalf("the answer beside a watch, after %v: %v", time.Since(began), r)
	}
	// The answer is queued for the proposer: fetched here, so the get below has nothing to wait for.
	if r := run(t, e, "get", "--label", "merge"); r.code != 0 || !strings.HasPrefix(r.stderr, "delivery ") {
		t.Fatalf("the proposer's queue: %v", r)
	}
	parked := []*proc{
		background(t, e, strings.NewReader(""), "result", "--label", "merge", "--question", "sha256:"+strings.Repeat("0", 64), "--wait", "30"),
		background(t, e, strings.NewReader(""), "get", "--label", "merge", "--wait", "30"),
		background(t, e, strings.NewReader(""), "questions", "--label", "merge", "--wait", "30"),
	}
	time.Sleep(300 * time.Millisecond)
	began = time.Now()
	if r := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Two."); r.code != 4 || time.Since(began) > 2*time.Second {
		t.Fatalf("a give beside the waits, after %v: %v", time.Since(began), r)
	}

	for _, p := range parked {
		select {
		case p.code = <-p.done:
			p.ended = true
			t.Fatalf("a wait ended early, %d: %s %s", p.code, p.stdout.String(), p.stderr.String())
		default:
		}
	}
	if q == "" || w.wait(t, 10*time.Second) != 0 {
		t.Fatalf("watch: %s", w.stderr.String())
	}
}

// mcpSession is a placket mcp run in this process, with a pipe for its input, which a goroutine of
// the session's writes, in order, so a command that never reads its input holds up no test.
type mcpSession struct {
	p     *proc
	lines chan []byte
	n     int
}

func startMCP(t testing.TB, e cli.Env, args ...string) *mcpSession {
	t.Helper()
	r, w := io.Pipe()
	s := &mcpSession{p: background(t, e, r, append([]string{"mcp"}, args...)...), lines: make(chan []byte, 16)}
	go func() {
		defer w.Close()
		for b := range s.lines {
			if _, err := w.Write(b); err != nil {
				return
			}
		}
	}()
	t.Cleanup(s.close)
	return s
}

// close ends the session's input, once.
func (s *mcpSession) close() {
	if s.lines != nil {
		close(s.lines)
		s.lines = nil
	}
}

// send writes one request and returns its id.
func (s *mcpSession) send(t testing.TB, method string, params any) int {
	t.Helper()
	s.n++
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": s.n, "method": method, "params": params})
	s.lines <- append(b, '\n')
	return s.n
}

// TestAResultParkedInMCPHoldsNoLock parks the MCP tool result with wait 30 on the proposer's key: a
// give on that key, from the command line, completes at once.
func TestAResultParkedInMCPHoldsNoLock(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	e, _ := labelsEnv(t, a.entry("proposer", "merge"))
	g := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", "One.")
	s := startMCP(t, e, "--label", "merge")
	s.send(t, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "0"}})
	s.send(t, "tools/call", map[string]any{"name": "result", "arguments": map[string]any{"mediator": "merge", "question": last(g.stdout), "wait": 30}})
	s.p.until(t, 1)
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	if r := run(t, e, "give", "--label", "merge", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Two."); r.code != 4 || time.Since(began) > 2*time.Second {
		t.Fatalf("a give beside the parked result, after %v: %v", time.Since(began), r)
	}
	if n := strings.Count(s.p.stdout.String(), "\n"); n != 1 {
		t.Fatalf("the parked result replied: %q", s.p.stdout.String())
	}
	s.close()
	if code := s.p.wait(t, 10*time.Second); code != 0 {
		t.Fatalf("placket mcp exited %d: %s", code, s.p.stderr.String())
	}
}
