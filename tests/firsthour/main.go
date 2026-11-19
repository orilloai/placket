// Command firsthour is the first-hour test's scripted MCP client (tests/test_first_hour.sh,
// docs/adr/0027-placket-init.md). It stands where Claude Code stands, with a script in place of a
// model:
//
//	firsthour COMMAND [ARG]...
//
// It starts COMMAND, the server a claude mcp add line names, speaks MCP to it over the server's
// standard input and output, one JSON-RPC message a line, and initializes it as a client does. Then,
// for each line of its own standard input, a JSON object {"name": TOOL, "arguments": {...}}, it calls
// the tool and prints the tool's structured result as one line of JSON. A tool the server does not
// list, a result that is a tool error, or a server that stops answering, ends it with exit 1 and a
// line on standard error. The server's standard error is its own.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// protocolVersion is the MCP version the client asks for.
const protocolVersion = "2025-06-18"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: firsthour COMMAND [ARG]...")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "firsthour: "+err.Error())
		os.Exit(1)
	}
}

// A client is one server's session: its input, its output read a line at a time, and the next id.
type client struct {
	in   io.Writer
	out  *bufio.Scanner
	next int
}

// A response is a JSON-RPC response, or a message the client skips: a notification or a request.
type response struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call sends one request and returns its result, skipping every message that is not its response.
func (c *client) call(method string, params any) (json.RawMessage, error) {
	c.next++
	id := c.next
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		return nil, fmt.Errorf("%s: the server takes no more input", method)
	}
	for c.out.Scan() {
		var r response
		if err := json.Unmarshal(c.out.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s: the server wrote a line that is not JSON", method)
		}
		if r.ID == nil || *r.ID != id {
			continue
		}
		if r.Error != nil {
			return nil, fmt.Errorf("%s: error %d: %s", method, r.Error.Code, r.Error.Message)
		}
		return r.Result, nil
	}
	return nil, fmt.Errorf("%s: the server ended before it answered", method)
}

// notify sends one notification.
func (c *client) notify(method string) error {
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func run(name string, args []string, script io.Reader, stdout io.Writer) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	err = session(&client{in: in, out: lines(out)}, script, stdout)
	_ = in.Close()
	werr := cmd.Wait()
	if err != nil {
		return err
	}
	if werr != nil {
		return fmt.Errorf("the server exited: %v", werr)
	}
	return nil
}

// lines reads a server's output a line at a time, a line as long as a message the server writes.
func lines(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 1<<16), 1<<25)
	return s
}

// session initializes the server, lists its tools, and calls each tool the script names in turn.
func session(c *client, script io.Reader, stdout io.Writer) error {
	if _, err := c.call("initialize", map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "firsthour", "version": "0"}}); err != nil {
		return err
	}
	if err := c.notify("notifications/initialized"); err != nil {
		return err
	}
	raw, err := c.call("tools/list", map[string]any{})
	if err != nil {
		return err
	}
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return errors.New("tools/list: the result does not read")
	}
	listed := map[string]bool{}
	for _, t := range list.Tools {
		listed[t.Name] = true
	}
	steps := bufio.NewScanner(script)
	steps.Buffer(make([]byte, 0, 1<<16), 1<<25)
	for steps.Scan() {
		var step struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(steps.Bytes(), &step); err != nil {
			return errors.New("a line of the script does not read")
		}
		if !listed[step.Name] {
			return fmt.Errorf("the server lists no tool %s", step.Name)
		}
		raw, err := c.call("tools/call", map[string]any{"name": step.Name, "arguments": step.Arguments})
		if err != nil {
			return err
		}
		var res struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Structured json.RawMessage `json:"structuredContent"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return fmt.Errorf("%s: the result does not read", step.Name)
		}
		if res.IsError {
			text := ""
			if len(res.Content) > 0 {
				text = res.Content[0].Text
			}
			return fmt.Errorf("%s: %s", step.Name, text)
		}
		if _, err := fmt.Fprintf(stdout, "%s\n", res.Structured); err != nil {
			return err
		}
	}
	return steps.Err()
}
