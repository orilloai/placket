package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	schemaURL  = "https://placket.tech/schema/0.1/"
	draftURL   = "https://json-schema.org/draft/2020-12/schema"
	schemaDir  = "spec/schema"
	examples   = "spec/schema/testdata"
	registerMD = "spec/frozen-formats.md"
)

var schemeRE = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:`)

// schemas-hold: every schema in spec/schema/ is JSON, declares JSON Schema draft 2020-12, has the $id
// its file name gives under schemaURL, refers to nothing outside that prefix, is named by a row of
// spec/frozen-formats.md, has accepted and refused examples under spec/schema/testdata/<name>/, and
// compiles with nothing fetched; every accepted example validates and every refused one is refused.
// The validator is github.com/santhosh-tekuri/jsonschema/v6; its loader serves schemaURL from
// spec/schema/ and refuses every other URL. Each schema that holds is counted on a line of its own.
func schemasHold(t *tree) []finding {
	if !t.isDir(schemaDir) {
		return []finding{{file: schemaDir + "/", text: "missing"}}
	}
	reg, ok := t.read(registerMD)
	if !ok || !t.isFile(registerMD) {
		return []finding{{file: registerMD, text: "missing; every schema needs a row of the register"}}
	}
	var fs []finding
	refuse := func(file, text string, details ...string) {
		fs = append(fs, finding{file: file, text: text, details: details})
	}

	var names []string
	for _, n := range listDir(t, schemaDir) {
		if !strings.HasSuffix(n, ".json") {
			continue
		}
		f := schemaDir + "/" + n
		name := strings.TrimSuffix(n, ".json")
		b, _ := t.read(f)
		doc, err := parseOrdered(b)
		if err != nil {
			refuse(f, "not JSON")
			continue
		}
		if s, _ := doc.member("$schema"); s.str() != draftURL {
			refuse(f, "not a JSON Schema draft 2020-12 schema: its $schema is not "+draftURL)
			continue
		}
		if id, _ := doc.member("$id"); id.tostring() != schemaURL+name+".json" {
			refuse(f, "its $id is not "+schemaURL+name+".json")
			continue
		}
		if out, found := doc.outside(); found {
			refuse(f, "names a schema outside "+schemaURL+": "+out)
			continue
		}
		if !bytes.Contains(reg, []byte("| `"+f+"`")) && !bytes.Contains(reg, []byte(", `"+f+"`")) {
			refuse(f, "no row of "+registerMD+" names it")
			continue
		}
		complete := true
		for _, v := range []string{"accepted", "refused"} {
			if len(jsonFiles(t, examples+"/"+name+"/"+v)) == 0 {
				refuse(f, "no "+v+" example in "+examples+"/"+name+"/"+v+"/")
				complete = false
				break
			}
		}
		if complete {
			names = append(names, name)
		}
	}

	notJSON := map[string]bool{}
	for _, n := range listDir(t, examples) {
		d := examples + "/" + n
		if !t.isFile(schemaDir + "/" + n + ".json") {
			refuse(d+"/", "examples of no schema; there is no "+schemaDir+"/"+n+".json")
			continue
		}
		var deeper []string
		for _, e := range listDir(t, d) {
			if e != "accepted" && e != "refused" {
				refuse(d+"/"+e, "not an example; examples are "+d+"/accepted/*.json and "+d+"/refused/*.json")
			}
			if t.isDir(d + "/" + e) {
				deeper = append(deeper, e)
			}
		}
		for _, e := range deeper {
			for _, x := range listDir(t, d+"/"+e) {
				if (e != "accepted" && e != "refused") || !strings.HasSuffix(x, ".json") {
					refuse(d+"/"+e+"/"+x, "not an example; examples are "+d+"/accepted/*.json and "+d+"/refused/*.json")
				}
			}
		}
		for _, v := range []string{"accepted", "refused"} {
			for _, e := range jsonFiles(t, d+"/"+v) {
				if b, _ := t.read(e); !validJSON(b) {
					refuse(e, "not JSON")
					notJSON[e] = true
				}
			}
		}
	}

	var verdicts, counts []finding
	set := compileSchemas(t)
	for _, n := range names {
		sch, err := set.get(n)
		if err != nil {
			refuse(schemaDir+"/"+n+".json", "does not compile as JSON Schema draft 2020-12 with nothing fetched", firstLines(err.Error(), 5, false)...)
			continue
		}
		held := true
		count := map[string]int{}
		for _, v := range []string{"accepted", "refused"} {
			for _, e := range jsonFiles(t, examples+"/"+n+"/"+v) {
				if notJSON[e] {
					continue
				}
				count[v]++
				b, _ := t.read(e)
				err := set.validate(n, sch, b)
				switch {
				case v == "accepted" && err != nil:
					verdicts = append(verdicts, finding{file: e, text: "refused by " + schemaDir + "/" + n + ".json", details: firstLines(err.Error(), 3, true)})
					held = false
				case v == "refused" && err == nil:
					verdicts = append(verdicts, finding{file: e, text: "accepted by " + schemaDir + "/" + n + ".json"})
					held = false
				}
			}
		}
		if held {
			counts = append(counts, finding{file: schemaDir + "/" + n + ".json", text: fmt.Sprintf("ok, %d accepted, %d refused", count["accepted"], count["refused"]), note: true})
		}
	}
	fs = append(fs, verdicts...)
	return append(fs, counts...)
}

// validJSON reports whether b is JSON, keeping each answer by the bytes.
func validJSON(b []byte) bool {
	if v, ok := validity.Load(string(b)); ok {
		return v.(bool)
	}
	v := json.Valid(b)
	validity.Store(string(b), v)
	return v
}

var validity sync.Map

// listDir lists the names in a directory that do not start with ".", in lexical order, as a shell
// glob does.
func listDir(t *tree, d string) []string {
	es, err := os.ReadDir(t.abs(d))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range es {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// jsonFiles lists the paths of d/*.json, in lexical order.
func jsonFiles(t *tree, d string) []string {
	var out []string
	for _, n := range listDir(t, d) {
		if strings.HasSuffix(n, ".json") {
			out = append(out, d+"/"+n)
		}
	}
	return out
}

// firstLines returns up to n non-empty lines of s; with listed, only the lines that list a reason.
func firstLines(s string, n int, listed bool) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimRight(l, " \t")
		if l == "" || (listed && !strings.HasPrefix(strings.TrimLeft(l, " \t"), "- ")) {
			continue
		}
		if len(out) == n {
			break
		}
		out = append(out, l)
	}
	return out
}

// A schemaSet compiles the schemas of one spec/schema/, each when first asked for, in one compiler. A
// schema that fails there is compiled again in a compiler of its own, so that one schema's failure
// is never another's. The checker keeps the sets it compiled, by the names and bytes of the files in
// spec/schema/, so that trees holding the same schemas, as its tests make, compile them once.
type schemaSet struct {
	t        *tree
	mu       sync.Mutex
	c        *jsonschema.Compiler
	done     map[string]compiled
	verdicts map[string]error
}

type compiled struct {
	s   *jsonschema.Schema
	err error
}

var schemaSets sync.Map

func compileSchemas(t *tree) *schemaSet {
	h := sha256.New()
	for _, n := range listDir(t, schemaDir) {
		b, _ := t.read(schemaDir + "/" + n)
		fmt.Fprintf(h, "%d %s %d\n", len(n), n, len(b))
		h.Write(b)
	}
	key := string(h.Sum(nil))
	set := &schemaSet{t: t, done: map[string]compiled{}, verdicts: map[string]error{}}
	got, _ := schemaSets.LoadOrStore(key, set)
	return got.(*schemaSet)
}

func (s *schemaSet) get(name string) (*jsonschema.Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.done[name]; ok {
		return r.s, r.err
	}
	if s.c == nil {
		s.c = newCompiler(s.t)
	}
	sch, err := s.c.Compile(schemaURL + name + ".json")
	if err != nil {
		s.c = nil
		sch, err = newCompiler(s.t).Compile(schemaURL + name + ".json")
	}
	s.done[name] = compiled{sch, err}
	return sch, err
}

// validate validates an example against the schema of the name, which compiled. The set keeps each
// verdict, by the schema's name and the example's bytes.
func (s *schemaSet) validate(name string, sch *jsonschema.Schema, example []byte) error {
	key := name + "\x00" + string(example)
	s.mu.Lock()
	err, ok := s.verdicts[key]
	s.mu.Unlock()
	if ok {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(example))
	if err == nil {
		err = sch.Validate(inst)
	}
	s.mu.Lock()
	s.verdicts[key] = err
	s.mu.Unlock()
	return err
}

// newCompiler is a compiler that loads only from the tree.
func newCompiler(t *tree) *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.UseLoader(treeLoader{t})
	return c
}

// treeLoader serves the URLs under schemaURL from spec/schema/ and refuses every other: the checker
// fetches nothing. The draft's own meta-schemas are built into the validator.
type treeLoader struct{ t *tree }

var errFetch = errors.New("not under " + schemaURL + ", and nothing is fetched")

func (l treeLoader) Load(url string) (any, error) {
	rest, ok := strings.CutPrefix(url, schemaURL)
	if !ok || rest == "" || strings.Contains(rest, "/") || strings.Contains(rest, "..") || path.Clean(rest) != rest {
		return nil, fmt.Errorf("%s: %w", url, errFetch)
	}
	b, ok := l.t.read(schemaDir + "/" + rest)
	if !ok {
		return nil, fmt.Errorf("%s: no such file in %s/", url, schemaDir)
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}

// A node is a JSON value that keeps its object members in the order written.
type node struct {
	kind   byte // 'o' object, 'a' array, 's' string, 'v' any other scalar
	keys   []string
	vals   []node
	scalar string // a string's value, or another scalar's JSON text
}

// parseOrdered parses one JSON value, members in order.
func parseOrdered(b []byte) (node, error) {
	if !json.Valid(b) {
		return node{}, errors.New("not JSON")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	n, err := readNode(d)
	if err != nil {
		return node{}, err
	}
	if _, err := d.Token(); err != io.EOF {
		return node{}, errors.New("more than one value")
	}
	return n, nil
}

func readNode(d *json.Decoder) (node, error) {
	tok, err := d.Token()
	if err != nil {
		return node{}, err
	}
	switch v := tok.(type) {
	case json.Delim:
		n := node{kind: 'a'}
		if v == '{' {
			n.kind = 'o'
		}
		for d.More() {
			if n.kind == 'o' {
				k, err := d.Token()
				if err != nil {
					return node{}, err
				}
				n.keys = append(n.keys, k.(string))
			}
			c, err := readNode(d)
			if err != nil {
				return node{}, err
			}
			n.vals = append(n.vals, c)
		}
		if _, err := d.Token(); err != nil {
			return node{}, err
		}
		return n, nil
	case string:
		return node{kind: 's', scalar: v}, nil
	case json.Number:
		return node{kind: 'v', scalar: v.String()}, nil
	case bool:
		return node{kind: 'v', scalar: strconv.FormatBool(v)}, nil
	default:
		return node{kind: 'v', scalar: "null"}, nil
	}
}

// member returns an object's member by name.
func (n node) member(k string) (node, bool) {
	if n.kind != 'o' {
		return node{}, false
	}
	for i, key := range n.keys {
		if key == k {
			return n.vals[i], true
		}
	}
	return node{}, false
}

// str is a string's value, or "" for anything else.
func (n node) str() string {
	if n.kind == 's' {
		return n.scalar
	}
	return ""
}

// tostring is a member's text: a string's value, "" for a missing member, null or false, and the
// compact JSON of anything else.
func (n node) tostring() string {
	switch {
	case n.kind == 's':
		return n.scalar
	case n.kind == 0 || (n.kind == 'v' && (n.scalar == "null" || n.scalar == "false")):
		return ""
	}
	return n.compact()
}

func (n node) compact() string {
	switch n.kind {
	case 's':
		b, _ := json.Marshal(n.scalar)
		return string(b)
	case 'v':
		return n.scalar
	}
	var b strings.Builder
	open, end := "[", "]"
	if n.kind == 'o' {
		open, end = "{", "}"
	}
	b.WriteString(open)
	for i, v := range n.vals {
		if i > 0 {
			b.WriteString(",")
		}
		if n.kind == 'o' {
			k, _ := json.Marshal(n.keys[i])
			b.Write(k)
			b.WriteString(":")
		}
		b.WriteString(v.compact())
	}
	b.WriteString(end)
	return b.String()
}

// outside returns the first $ref, $dynamicRef, $schema or $id, objects read in document order and each
// object's members before the objects inside it, that is not a string, or that names a URL outside
// schemaURL other than the draft's.
func (n node) outside() (string, bool) {
	var objs []node
	var collect func(node)
	collect = func(x node) {
		if x.kind == 'o' {
			objs = append(objs, x)
		}
		for _, c := range x.vals {
			collect(c)
		}
	}
	collect(n)
	for _, o := range objs {
		for i, k := range o.keys {
			if k != "$ref" && k != "$dynamicRef" && k != "$schema" && k != "$id" {
				continue
			}
			v := o.vals[i]
			if v.kind != 's' {
				return v.compact(), true
			}
			if schemeRE.MatchString(v.scalar) && !strings.HasPrefix(v.scalar, schemaURL) && v.scalar != draftURL {
				return v.scalar, true
			}
		}
	}
	return "", false
}
