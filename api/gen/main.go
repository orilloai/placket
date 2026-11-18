// Command gen writes docs/reference/mediator-api.md, the mediator API's reference, from the schemas
// package api embeds, the wire layer, the request layer and the labels config's, and from the error
// table of adapters/errtext. api's tests fail when the committed page differs from a fresh run.
//
// Run from the repository's root: go run ./api/gen [-o docs/reference/mediator-api.md].
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/orilloai/placket/adapters/errtext"
	"github.com/orilloai/placket/api"
)

const base = "https://placket.tech/api/0.1/"

// ops are the door's calls, in the order the reference gives them.
var ops = []string{"give", "get", "questions", "resolve", "result", "view", "accept"}

// tools are the request layer's schemas, in the order the reference gives them.
var tools = []string{"give", "get", "questions", "resolve", "result", "cancel", "mediators", "view", "accept"}

type schema = map[string]any

// docs is every schema by its $id.
var docs = map[string]schema{}

func load() error {
	for _, dir := range []string{"wire", "request", "config"} {
		entries, err := api.FS.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			b, err := api.FS.ReadFile(dir + "/" + e.Name())
			if err != nil {
				return err
			}
			var s schema
			if err := json.Unmarshal(b, &s); err != nil {
				return fmt.Errorf("%s/%s: %v", dir, e.Name(), err)
			}
			docs[s["$id"].(string)] = s
		}
	}
	return nil
}

func doc(path string) schema { return docs[base+path] }

// deref follows a $ref, by absolute $id and JSON pointer, and returns the schema and the name of the
// definition it names, or "".
func deref(s schema) (schema, string) {
	r, ok := s["$ref"].(string)
	if !ok {
		return s, ""
	}
	id, ptr, _ := strings.Cut(r, "#")
	v := any(docs[id])
	name := ""
	for _, tok := range strings.Split(ptr, "/")[1:] {
		v = v.(schema)[tok]
		name = tok
	}
	t := v.(schema)
	if d, ok := s["description"]; ok {
		c := schema{}
		for k, x := range t {
			c[k] = x
		}
		c["description"] = d
		t = c
	}
	return t, name
}

// kind is a schema's type as a reader takes it: its const, its values, its type with its pattern and
// bounds, or the definition it names.
func kind(s schema) string {
	t, name := deref(s)
	var parts []string
	if name != "" {
		parts = append(parts, name)
	}
	switch {
	case t["const"] != nil:
		b, _ := json.Marshal(t["const"])
		return "`" + string(b) + "`"
	case t["enum"] != nil:
		var vs []string
		for _, v := range t["enum"].([]any) {
			b, _ := json.Marshal(v)
			vs = append(vs, "`"+string(b)+"`")
		}
		return strings.Join(vs, ", ")
	}
	if ty, ok := t["type"].(string); ok && name == "" {
		parts = append(parts, ty)
	} else if !ok && name == "" {
		parts = append(parts, "any JSON value")
	}
	if p, ok := t["pattern"].(string); ok {
		parts = append(parts, "`"+p+"`")
	}
	if lo, ok := t["minimum"].(float64); ok {
		parts = append(parts, "from "+strconv.Itoa(int(lo))+" to "+strconv.Itoa(int(t["maximum"].(float64))))
	}
	if n, ok := t["minLength"].(float64); ok && n > 0 {
		parts = append(parts, "not empty")
	}
	if n, ok := t["maxLength"].(float64); ok {
		parts = append(parts, "at most "+strconv.Itoa(int(n))+" characters")
	}
	return strings.Join(parts, ", ")
}

func cell(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ") }

// members writes a table row for each member of an object schema, at prefix, and of every object
// under it that names its own members.
func members(b *bytes.Buffer, s schema, prefix string) {
	s, _ = deref(s)
	props, _ := s["properties"].(map[string]any)
	required := map[string]bool{}
	for _, r := range asList(s["required"]) {
		required[r.(string)] = true
	}
	var names []string
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p, _ := deref(props[n].(schema))
		desc, _ := p["description"].(string)
		req := "no"
		if required[n] {
			req = "yes"
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", prefix+n, cell(kind(props[n].(schema))), req, cell(desc))
		if _, has := p["properties"]; has {
			members(b, p, prefix+n+".")
		}
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func table(b *bytes.Buffer, s schema) {
	b.WriteString("| Member | Value | Required | What it is |\n|---|---|---|---|\n")
	members(b, s, "")
	b.WriteString("\n")
}

// alternatives writes each form a reply may take.
func alternatives(b *bytes.Buffer, s schema) {
	for _, alt := range asList(s["oneOf"]) {
		a, name := deref(alt.(schema))
		if inner := asList(a["oneOf"]); inner != nil {
			alternatives(b, a)
			continue
		}
		desc, _ := a["description"].(string)
		var shape []string
		props, _ := a["properties"].(map[string]any)
		var names []string
		for n := range props {
			names = append(names, n)
		}
		sort.Strings(names)
		shape = append(shape, names...)
		if name != "" {
			fmt.Fprintf(b, "- `{%s}` (%s): %s\n", strings.Join(shape, ","), name, desc)
		} else {
			fmt.Fprintf(b, "- `{%s}`\n", strings.Join(shape, ","))
		}
		for _, n := range names {
			p, _ := deref(props[n].(schema))
			d, _ := p["description"].(string)
			fmt.Fprintf(b, "  - `%s`: %s\n", n, strings.TrimSpace(kind(props[n].(schema))+". "+d))
		}
	}
	b.WriteString("\n")
}

func render() []byte {
	var b bytes.Buffer
	b.WriteString(`# The mediator API

<!-- Generated by go run ./api/gen from api/ and adapters/errtext; do not edit. -->

The mediator API is the calls a window's door takes, and their replies. A client in any language reaches a mediator through them; ` + "`adapters/client`" + ` is placket's own, and ` + "`placket mcp`" + ` wraps it as MCP tools. The API is versioned with the release: these schemas are those of placket 0.1, every ` + "`$id`" + ` under ` + "`" + base + "`" + `. It is a door format, outside the frozen formats; the answer call a resolve carries, and the receipts, deliveries and close a reply carries, are frozen formats of ` + "`spec/`" + `, described here as the door carries them.

It has two layers. The **wire layer** (` + "`api/wire/`" + `) is what travels on the socket: one signed call and one reply per connection. The **request layer** (` + "`api/request/`" + `) is what ` + "`adapters/client`" + ` and the MCP tools take: the client builds the wire call from it, signs it, and checks the reply.

## The wire layer

A window listens on a unix socket, mode 0600, by default ` + "`<runtime>/<first 12 hex of the Accord id>.sock`" + `, where ` + "`<runtime>`" + ` is ` + "`$XDG_RUNTIME_DIR/placket`" + `, else ` + "`$TMPDIR/placket-<uid>`" + `. A client connects, writes one call and a line feed, reads one reply and a line feed, and closes. It must not close its side of the socket for writing before it has read the reply. A call is at most 1 MiB, a reply at most 4 MiB, each in canonical JSON.

A call is ` + "`{\"call\",\"sig\"}`" + `: ` + "`sig`" + ` is standard base64 of the Ed25519 signature, by the caller's key, over the DSSE pre-authentication encoding of the payload type ` + "`application/vnd.placket.call+json`" + ` and the canonical bytes of ` + "`call`" + `. The door checks a call in this order and refuses it at the first that fails, under the reply's ` + "`door`" + ` member: its size (` + "`too-large`" + `), its form (` + "`bad-call`" + `), its signature (` + "`bad-signature`" + `), its Accord (` + "`wrong-accord`" + `), its time within 60 seconds of the door's (` + "`stale`" + `), and its nonce, unseen in the last 120 seconds (` + "`replayed`" + `). The mediator then rejects what the caller did wrong under ` + "`rejection`" + `, or names what its program did wrong under ` + "`error`" + `.

A call of ` + "`questions`" + `, ` + "`get`" + ` or ` + "`result`" + ` may carry ` + "`wait`" + `, whole seconds up to 60: the door holds it until it has something to return, or the wait ends, and then replies as it would have at that moment. A connection may stay open 30 seconds, or its wait and 10 seconds more when that is longer; a give, which has no wait, may take up to 20 seconds when a judge decides it.

`)
	for _, op := range ops {
		call, reply := doc("wire/"+op+".call.json"), doc("wire/"+op+".reply.json")
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", op, call["description"])
		fmt.Fprintf(&b, "The call, `%s`:\n\n", strings.TrimPrefix(call["$id"].(string), base))
		table(&b, call)
		fmt.Fprintf(&b, "The reply, `%s`, one of:\n\n", strings.TrimPrefix(reply["$id"].(string), base))
		alternatives(&b, reply)
	}
	b.WriteString("## The request layer\n\nEach tool of `placket mcp` takes its request, and `mediator`: the label of a mediator it serves, or the Accord id of one with none. `resolve` is the client's alone: `placket mcp` lists only its asker's slice, `cancel`. `view` and `accept` are the client's alone too: `placket mcp` lists neither, since a member consents to an Accord itself.\n\n")
	for _, t := range tools {
		s := doc("request/" + t + ".json")
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", t, s["description"])
		if p, _ := s["properties"].(map[string]any); len(p) == 0 {
			b.WriteString("It takes no member.\n\n")
			continue
		}
		table(&b, s)
		if one := asList(s["oneOf"]); one != nil {
			var alts []string
			for _, o := range one {
				alts = append(alts, "`"+asList(o.(schema)["required"])[0].(string)+"`")
			}
			fmt.Fprintf(&b, "Exactly one of %s.\n\n", strings.Join(alts, " and "))
		}
		if _, ok := s["if"]; ok {
			b.WriteString("`answer` is required for `accepted` and `changed`, and absent otherwise.\n\n")
		}
	}
	c := doc("config/mediators.json")
	fmt.Fprintf(&b, "## The labels config\n\n%s\n\n", c["description"])
	b.WriteString("| Member | Value | Required | What it is |\n|---|---|---|---|\n")
	members(&b, c, "")
	items := c["properties"].(map[string]any)["mediators"].(schema)["items"].(schema)
	members(&b, items, "mediators[].")
	b.WriteString("\nThe client checks, in this order, after the schema (`bad-config`): no two entries have one label (`label-taken`); no two of different Accords use one socket, given or default; no two use one state directory; and no two of one Accord are both unlabelled (`mediator-collision`). The first that fails is reported, naming its two entries by their index, the first pair by the first index and then the second.\n\n")
	b.WriteString("## Errors\n\nEvery code `placket mcp` can return, by where it comes from: a code in a door reply by its member, `door`, `rejection` or `error`; any other by the package that declares it. A tool error's text is the code, a colon and its sentence.\n\n| From | Code | Cause and fix |\n|---|---|---|\n")
	for _, r := range errtext.Rows() {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", r.From, r.Code, cell(r.Sentence))
	}
	return b.Bytes()
}

func main() {
	out := flag.String("o", filepath.Join("docs", "reference", "mediator-api.md"), "the page to write")
	flag.Parse()
	if err := load(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, render(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
