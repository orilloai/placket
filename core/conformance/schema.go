package conformance

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A schemaSet holds JSON Schema draft 2020-12 documents by their $id, and compiles from them the
// keywords placket's schemas use. A keyword outside that list is refused when a schema is compiled,
// never ignored, so a schema that starts to use one fails loudly here instead of validating less.
type schemaSet struct {
	docs  map[string]any
	nodes map[string]*node
}

// node is one compiled schema. loc names it for messages: its document's file name and the JSON
// pointer to it, such as vector.json#/$defs/step.
type node struct {
	loc string

	always, never bool // the boolean schemas true and false

	types         []string
	hasConst      bool
	constant      any
	enum          []any
	pattern       *regexp.Regexp
	minLength     int
	maxLength     int
	minimum       *decimal
	maximum       *decimal
	required      []string
	properties    map[string]*node
	additional    *node
	propertyNames *node
	minProperties int
	maxProperties int
	items         *node
	minItems      int
	maxItems      int
	uniqueItems   bool
	allOf         []*node
	anyOf         []*node
	oneOf         []*node
	not           *node
	ifNode        *node
	thenNode      *node
	elseNode      *node
	ref           string // the absolute reference, resolved when first validated
	set           *schemaSet
}

// annotations are keywords that never change a result.
var annotations = map[string]bool{
	"$schema": true, "$id": true, "$defs": true, "$comment": true, "title": true,
	"description": true, "examples": true, "default": true,
}

const draft = "https://json-schema.org/draft/2020-12/schema"

func newSchemaSet() *schemaSet {
	return &schemaSet{docs: map[string]any{}, nodes: map[string]*node{}}
}

// add reads one schema document and files it under its $id.
func (s *schemaSet) add(name string, b []byte) error {
	v, err := read(b)
	if err != nil {
		return errors.New(name + ": " + err.Error())
	}
	o, ok := v.(*object)
	if !ok {
		return errors.New(name + ": not a JSON object")
	}
	if sc, _ := o.values["$schema"].(string); sc != draft {
		return errors.New(name + ": its $schema is not " + draft)
	}
	id, _ := o.values["$id"].(string)
	if id == "" || strings.Contains(id, "#") {
		return errors.New(name + ": no $id naming the document")
	}
	if _, dup := s.docs[id]; dup {
		return errors.New(name + ": a second schema with the $id " + id)
	}
	s.docs[id] = v
	return nil
}

// compile returns the schema the absolute reference names: a document's $id, optionally followed by
// # and a JSON pointer into it.
func (s *schemaSet) compile(ref string) (*node, error) {
	if n, ok := s.nodes[ref]; ok {
		return n, nil
	}
	doc, ptr, _ := strings.Cut(ref, "#")
	v, ok := s.docs[doc]
	if !ok {
		return nil, errors.New("no schema has the $id " + doc)
	}
	for _, tok := range strings.Split(ptr, "/")[1:] {
		tok = strings.ReplaceAll(strings.ReplaceAll(tok, "~1", "/"), "~0", "~")
		switch c := v.(type) {
		case *object:
			next, ok := c.values[tok]
			if !ok {
				return nil, errors.New(ref + ": the pointer names nothing")
			}
			v = next
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(c) {
				return nil, errors.New(ref + ": the pointer names nothing")
			}
			v = c[i]
		default:
			return nil, errors.New(ref + ": the pointer names nothing")
		}
	}
	return s.build(v, doc, ptr)
}

func fileOf(doc string) string { return doc[strings.LastIndexByte(doc, '/')+1:] }

// build compiles the schema v found at doc#ptr and every schema under it.
func (s *schemaSet) build(v any, doc, ptr string) (*node, error) {
	key := doc + "#" + ptr
	if n, ok := s.nodes[key]; ok {
		return n, nil
	}
	n := &node{loc: fileOf(doc) + "#" + ptr, maxLength: -1, maxProperties: -1, maxItems: -1, set: s}
	s.nodes[key] = n
	if b, ok := v.(bool); ok {
		n.always, n.never = b, !b
		return n, nil
	}
	o, ok := v.(*object)
	if !ok {
		return nil, errors.New(n.loc + ": a schema that is neither an object nor a boolean")
	}
	sub := func(name string, w any) (*node, error) { return s.build(w, doc, ptr+"/"+name) }
	list := func(name string, w any) ([]*node, error) {
		l, ok := w.([]any)
		if !ok || len(l) == 0 {
			return nil, errors.New(n.loc + ": " + name + " is not a list of schemas")
		}
		var out []*node
		for i, x := range l {
			c, err := s.build(x, doc, ptr+"/"+name+"/"+strconv.Itoa(i))
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, nil
	}
	count := func(name string, w any) (int, error) {
		if d, ok := w.(number); ok {
			if i, err := strconv.Atoi(string(d)); err == nil && i >= 0 {
				return i, nil
			}
		}
		return 0, errors.New(n.loc + ": " + name + " is not a whole number written in digits")
	}
	var err error
	for _, k := range o.names {
		w := o.values[k]
		switch k {
		case "type":
			switch t := w.(type) {
			case string:
				n.types = []string{t}
			case []any:
				for _, x := range t {
					s, ok := x.(string)
					if !ok {
						return nil, errors.New(n.loc + ": type is not a list of names")
					}
					n.types = append(n.types, s)
				}
			default:
				return nil, errors.New(n.loc + ": type is not a name or a list of names")
			}
			for _, t := range n.types {
				switch t {
				case "null", "boolean", "object", "array", "number", "integer", "string":
				default:
					return nil, errors.New(n.loc + ": type " + strconv.Quote(t) + " is not a JSON Schema type")
				}
			}
		case "const":
			n.hasConst, n.constant = true, w
		case "enum":
			l, ok := w.([]any)
			if !ok {
				return nil, errors.New(n.loc + ": enum is not a list")
			}
			n.enum = l
		case "pattern":
			p, ok := w.(string)
			if !ok {
				return nil, errors.New(n.loc + ": pattern is not a string")
			}
			if n.pattern, err = regexp.Compile(p); err != nil {
				return nil, errors.New(n.loc + ": pattern does not compile: " + err.Error())
			}
		case "minLength":
			n.minLength, err = count(k, w)
		case "maxLength":
			n.maxLength, err = count(k, w)
		case "minProperties":
			n.minProperties, err = count(k, w)
		case "maxProperties":
			n.maxProperties, err = count(k, w)
		case "minItems":
			n.minItems, err = count(k, w)
		case "maxItems":
			n.maxItems, err = count(k, w)
		case "minimum", "maximum":
			d, ok := w.(number)
			if !ok {
				return nil, errors.New(n.loc + ": " + k + " is not a number")
			}
			x := parseDecimal(d)
			if k == "minimum" {
				n.minimum = &x
			} else {
				n.maximum = &x
			}
		case "uniqueItems":
			b, ok := w.(bool)
			if !ok {
				return nil, errors.New(n.loc + ": uniqueItems is not a boolean")
			}
			n.uniqueItems = b
		case "required":
			l, ok := w.([]any)
			if !ok {
				return nil, errors.New(n.loc + ": required is not a list")
			}
			for _, x := range l {
				s, ok := x.(string)
				if !ok {
					return nil, errors.New(n.loc + ": required is not a list of names")
				}
				n.required = append(n.required, s)
			}
		case "properties":
			p, ok := w.(*object)
			if !ok {
				return nil, errors.New(n.loc + ": properties is not an object")
			}
			n.properties = map[string]*node{}
			for _, name := range p.names {
				c, err := s.build(p.values[name], doc, ptr+"/properties/"+strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1"))
				if err != nil {
					return nil, err
				}
				n.properties[name] = c
			}
		case "additionalProperties":
			n.additional, err = sub(k, w)
		case "propertyNames":
			n.propertyNames, err = sub(k, w)
		case "items":
			n.items, err = sub(k, w)
		case "not":
			n.not, err = sub(k, w)
		case "if":
			n.ifNode, err = sub(k, w)
		case "then":
			n.thenNode, err = sub(k, w)
		case "else":
			n.elseNode, err = sub(k, w)
		case "allOf":
			n.allOf, err = list(k, w)
		case "anyOf":
			n.anyOf, err = list(k, w)
		case "oneOf":
			n.oneOf, err = list(k, w)
		case "$ref":
			r, ok := w.(string)
			if !ok {
				return nil, errors.New(n.loc + ": $ref is not a string")
			}
			n.ref = resolve(doc, r)
		case "$defs":
			d, ok := w.(*object)
			if !ok {
				return nil, errors.New(n.loc + ": $defs is not an object")
			}
			for _, name := range d.names {
				if _, err := s.build(d.values[name], doc, ptr+"/$defs/"+name); err != nil {
					return nil, err
				}
			}
		default:
			if !annotations[k] {
				return nil, errors.New(n.loc + ": the keyword " + strconv.Quote(k) + " is not one this runner reads")
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return n, nil
}

// resolve makes the reference r, found in the document with $id doc, absolute: a fragment alone
// stays in doc, and a relative path replaces the last segment of doc's path.
func resolve(doc, r string) string {
	if strings.HasPrefix(r, "#") {
		return doc + r
	}
	if strings.Contains(r, "://") {
		if !strings.Contains(r, "#") {
			return r + "#"
		}
		return r
	}
	abs := doc[:strings.LastIndexByte(doc, '/')+1] + r
	if !strings.Contains(abs, "#") {
		abs += "#"
	}
	return abs
}

// A schemaError says where a value breaks a schema: at, the JSON pointer of the value ("/" for the
// whole), and what is wrong with it.
type schemaError struct {
	at, what string
}

func (e *schemaError) Error() string { return e.at + ": " + e.what }

func pointer(at, name string) string {
	name = strings.ReplaceAll(strings.ReplaceAll(name, "~", "~0"), "/", "~1")
	if at == "/" {
		return "/" + name
	}
	return at + "/" + name
}

// validate checks v against n and returns the first rule it breaks, or nil. The keywords are taken
// in a fixed order, so the same value always gets the same message.
func (n *node) validate(v any, at string) *schemaError {
	fail := func(what string) *schemaError { return &schemaError{at: at, what: what} }
	if n.always {
		return nil
	}
	if n.never {
		return fail("not allowed here (" + n.loc + ")")
	}
	if n.ref != "" {
		r, err := n.set.compile(n.ref)
		if err != nil {
			return fail("the schema at " + n.loc + " refers to " + n.ref + ", which does not compile: " + err.Error())
		}
		if e := r.validate(v, at); e != nil {
			return e
		}
	}
	if len(n.types) > 0 {
		ok := false
		for _, t := range n.types {
			if isType(v, t) {
				ok = true
			}
		}
		if !ok {
			return fail("is " + typeName(v) + ", not " + strings.Join(n.types, " or "))
		}
	}
	if n.hasConst && !equal(v, n.constant) {
		return fail("is not " + show(n.constant))
	}
	if n.enum != nil {
		ok := false
		for _, e := range n.enum {
			if equal(v, e) {
				ok = true
			}
		}
		if !ok {
			return fail("is not one of " + show(n.enum))
		}
	}
	switch v := v.(type) {
	case string:
		l := utf8.RuneCountInString(v)
		if l < n.minLength {
			return fail("is shorter than " + strconv.Itoa(n.minLength) + " characters")
		}
		if n.maxLength >= 0 && l > n.maxLength {
			return fail("is longer than " + strconv.Itoa(n.maxLength) + " characters")
		}
		if n.pattern != nil && !n.pattern.MatchString(v) {
			return fail("does not match " + n.pattern.String())
		}
	case number:
		d := parseDecimal(v)
		if n.minimum != nil && d.cmp(*n.minimum) < 0 {
			return fail("is less than the minimum (" + n.loc + ")")
		}
		if n.maximum != nil && d.cmp(*n.maximum) > 0 {
			return fail("is more than the maximum (" + n.loc + ")")
		}
	case *object:
		for _, r := range n.required {
			if _, ok := v.values[r]; !ok {
				return fail("has no member " + strconv.Quote(r))
			}
		}
		for _, name := range v.names {
			p := pointer(at, name)
			if c, ok := n.properties[name]; ok {
				if e := c.validate(v.values[name], p); e != nil {
					return e
				}
			} else if n.additional != nil {
				if n.additional.never {
					return fail("has a member " + strconv.Quote(name) + " the format does not list")
				}
				if e := n.additional.validate(v.values[name], p); e != nil {
					return e
				}
			}
			if n.propertyNames != nil {
				if e := n.propertyNames.validate(name, p); e != nil {
					return fail("has a member " + strconv.Quote(name) + " whose name is refused: it " + e.what)
				}
			}
		}
		if len(v.names) < n.minProperties {
			return fail("has fewer than " + strconv.Itoa(n.minProperties) + " members")
		}
		if n.maxProperties >= 0 && len(v.names) > n.maxProperties {
			return fail("has more than " + strconv.Itoa(n.maxProperties) + " members")
		}
	case []any:
		if n.items != nil {
			for i, x := range v {
				if e := n.items.validate(x, pointer(at, strconv.Itoa(i))); e != nil {
					return e
				}
			}
		}
		if len(v) < n.minItems {
			return fail("has fewer than " + strconv.Itoa(n.minItems) + " items")
		}
		if n.maxItems >= 0 && len(v) > n.maxItems {
			return fail("has more than " + strconv.Itoa(n.maxItems) + " items")
		}
		if n.uniqueItems {
			for i := range v {
				for j := 0; j < i; j++ {
					if equal(v[i], v[j]) {
						return fail("repeats an item: " + strconv.Itoa(j) + " and " + strconv.Itoa(i) + " are equal")
					}
				}
			}
		}
	}
	for _, c := range n.allOf {
		if e := c.validate(v, at); e != nil {
			return e
		}
	}
	if n.anyOf != nil {
		ok := false
		for _, c := range n.anyOf {
			if c.validate(v, at) == nil {
				ok = true
				break
			}
		}
		if !ok {
			return fail("holds none of the schemas of " + n.loc + "/anyOf")
		}
	}
	if n.oneOf != nil {
		held := 0
		var errs []*schemaError
		for _, c := range n.oneOf {
			if e := c.validate(v, at); e == nil {
				held++
			} else {
				errs = append(errs, e)
			}
		}
		if held == 0 {
			if e := deepest(errs); e != nil {
				return e
			}
		}
		if held != 1 {
			return fail("holds " + strconv.Itoa(held) + " of the " + strconv.Itoa(len(n.oneOf)) + " schemas of " + n.loc + "/oneOf, not exactly one")
		}
	}
	if n.not != nil && n.not.validate(v, at) == nil {
		return fail("holds the schema of " + n.loc + "/not")
	}
	if n.ifNode != nil {
		if n.ifNode.validate(v, at) == nil {
			if n.thenNode != nil {
				return n.thenNode.validate(v, at)
			}
		} else if n.elseNode != nil {
			return n.elseNode.validate(v, at)
		}
	}
	return nil
}

// deepest returns the error found deepest in the value, when one is deeper than every other: the
// alternative that read furthest into the value is the one the value was most likely meant to be.
func deepest(errs []*schemaError) *schemaError {
	var best *schemaError
	bestDepth, ties := -1, 0
	for _, e := range errs {
		d := strings.Count(e.at, "/")
		if e.at == "/" {
			d = 0
		}
		switch {
		case d > bestDepth:
			best, bestDepth, ties = e, d, 1
		case d == bestDepth:
			ties++
		}
	}
	if ties != 1 {
		return nil
	}
	return best
}

func isType(v any, t string) bool {
	switch v := v.(type) {
	case nil:
		return t == "null"
	case bool:
		return t == "boolean"
	case string:
		return t == "string"
	case number:
		return t == "number" || (t == "integer" && parseDecimal(v).integral())
	case []any:
		return t == "array"
	case *object:
		return t == "object"
	}
	return false
}

func typeName(v any) string {
	switch v := v.(type) {
	case nil:
		return "null"
	case bool:
		return "a boolean"
	case string:
		return "a string"
	case number:
		if parseDecimal(v).integral() {
			return "an integer"
		}
		return "a number"
	case []any:
		return "an array"
	}
	return "an object"
}

// equal is JSON equality: numbers by value, objects by their members whatever their order.
func equal(a, b any) bool {
	switch a := a.(type) {
	case number:
		b, ok := b.(number)
		return ok && parseDecimal(a).cmp(parseDecimal(b)) == 0
	case []any:
		b, ok := b.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !equal(a[i], b[i]) {
				return false
			}
		}
		return true
	case *object:
		b, ok := b.(*object)
		if !ok || len(a.names) != len(b.names) {
			return false
		}
		for _, k := range a.names {
			w, ok := b.values[k]
			if !ok || !equal(a.values[k], w) {
				return false
			}
		}
		return true
	}
	return a == b
}

// show writes a schema's value back as compact JSON, for a message.
func show(v any) string {
	switch v := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case number:
		return string(v)
	case string:
		return quote(v)
	case []any:
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = show(x)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *object:
		names := append([]string(nil), v.names...)
		sort.Strings(names)
		parts := make([]string, len(names))
		for i, k := range names {
			parts[i] = quote(k) + ":" + show(v.values[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return "?"
}

// quote writes s as a JSON string: a quote, a backslash and the controls escaped, nothing else.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[r>>4])
			b.WriteByte("0123456789abcdef"[r&15])
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
