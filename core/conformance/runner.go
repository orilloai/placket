package conformance

import (
	"io/fs"

	"github.com/orilloai/placket/core/canonical"
	"sort"
	"strconv"
	"strings"
)

// A Vector is one vector file as a handler receives it, after the runner's own checks passed
// (conformance-vectors-1, record 0004).
type Vector struct {
	// Path is the file's path from the repository root, such as vectors/canonical/duplicate-key.json.
	Path string
	// Operation is the vector's operation member, such as canonical.
	Operation string
	// Bytes are the file's bytes, unchanged.
	Bytes []byte
}

// A Part is one result a handler compared, or could not: a step, a final list, an expected value.
// It passed when both Failure and Pending are empty (runner-no-silent-pass, record 0004).
type Part struct {
	// Name says which part, such as step-2 or final.
	Name string
	// Failure says how the implementation's result differed from the expected one.
	Failure string
	// Pending names what the part needs before it can run. A pending part fails its vector.
	Pending string
}

// A Handler runs a vector of one operation against the implementation and reports every part it
// compared, in order. A handler that reports no part fails the vector (record 0004).
type Handler func(Vector) []Part

// Handlers names the handler of each operation. An operation with none fails every vector of it
// (record 0004).
type Handlers map[string]Handler

// The checks a vector goes through, in order (section 10 of the protocol text; conformance-vectors-10,
// record 0004).
const (
	CheckHeader    = "header"
	CheckSchema    = "schema"
	CheckInputs    = "inputs"
	CheckOperation = "operation"
	CheckResults   = "results"
)

// A Result is what became of one vector (record 0004).
type Result struct {
	// Path is the vector's path from the repository root.
	Path string
	// Check names the check that failed the vector, and is empty when it passed.
	Check string
	// Reason says why it failed.
	Reason string
	// Parts counts the parts that passed.
	Parts int
}

// Passed reports whether the vector passed. A vector either passes or fails: none is skipped
// (runner-no-silent-pass, record 0004).
func (r Result) Passed() bool { return r.Check == "" }

// An Open entry of the coverage list is a requirement, or a scenario of one, that no vector covers
// yet, with the milestone that writes its vector; or one covered in part, with what stays open
// (coverage-reported, records 0004 and 0005).
type Open struct {
	ID        string
	Scenario  string // empty for the requirement itself
	Milestone string
	// Rest is what stays open of an entry covered in part, and empty for an entry open as a whole.
	Rest string
}

// A Report is the outcome of a run (record 0004).
type Report struct {
	// Results holds every vector found, in path order.
	Results []Result
	// Problems are failures of the run that belong to no one vector: the schema, its examples and
	// the coverage list.
	Problems []string
	// Open lists the coverage list's open entries, in the order of spec/ids.txt and spec/protocol.md.
	Open []Open
}

// Passed reports whether the run passed: every vector found passed and nothing else failed
// (runner-no-silent-pass, record 0004).
func (r Report) Passed() bool {
	if len(r.Problems) > 0 {
		return false
	}
	for _, v := range r.Results {
		if !v.Passed() {
			return false
		}
	}
	return true
}

// Lines writes the report as lines: each failure, each vector that passed, each open entry, and a
// count (record 0004).
func (r Report) Lines() []string {
	var out []string
	failed := 0
	for _, v := range r.Results {
		if v.Passed() {
			out = append(out, "pass: "+v.Path+": "+plural(v.Parts, "part"))
		} else {
			failed++
			out = append(out, "fail: "+v.Path+": "+v.Check+": "+v.Reason)
		}
	}
	for _, p := range r.Problems {
		out = append(out, "fail: "+p)
	}
	for _, o := range r.Open {
		line := "open: " + o.ID + ": "
		if o.Scenario != "" {
			line += "scenario " + strconv.Quote(o.Scenario) + ": "
		}
		line += o.Milestone
		if o.Rest != "" {
			line += ", in part: " + o.Rest
		}
		out = append(out, line)
	}
	out = append(out, plural(len(r.Results), "vector")+": "+strconv.Itoa(len(r.Results)-failed)+" passed, "+
		strconv.Itoa(failed)+" failed; "+plural(len(r.Problems), "other failure")+"; "+plural(len(r.Open), "entry")+" open")
	return out
}

// plural writes n and a noun, in the plural unless n is 1.
func plural(n int, noun string) string {
	if n != 1 {
		if strings.HasSuffix(noun, "y") {
			noun = noun[:len(noun)-1] + "ie"
		}
		noun += "s"
	}
	return strconv.Itoa(n) + " " + noun
}

// Run reads the protocol's spec/ directory from spec and the vectors/ directory from vectors, checks
// vector.json against its own examples, runs every vector with the handlers given, and checks the
// coverage list. A vector is every .json file under vectors/ but vectors/coverage.json
// (conformance-vectors-10, record 0004).
func Run(spec, vectors fs.FS, h Handlers) Report {
	schema, problems := loadSchema(spec)
	return runLoaded(spec, vectors, h, schema, problems)
}

// runLoaded is Run with spec/schema/ already loaded: schema and problems are what loadSchema
// returned for spec. It reads schema only, so one load serves any number of runs.
func runLoaded(spec, vectors fs.FS, h Handlers, schema *node, problems []string) Report {
	var r Report
	r.Problems = append(r.Problems, problems...)

	var files []string
	err := fs.WalkDir(vectors, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".json") && p != "coverage.json" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		r.Problems = append(r.Problems, "vectors/: cannot be read: "+err.Error())
	}
	sort.Strings(files)
	found := map[string]bool{}
	for _, f := range files {
		found[strings.TrimSuffix(f, ".json")] = true
		if schema == nil {
			r.Results = append(r.Results, Result{Path: "vectors/" + f, Check: CheckSchema, Reason: "spec/schema/vector.json did not load"})
			continue
		}
		b, err := fs.ReadFile(vectors, f)
		if err != nil {
			r.Results = append(r.Results, Result{Path: "vectors/" + f, Check: CheckHeader, Reason: "cannot be read: " + err.Error()})
			continue
		}
		r.Results = append(r.Results, runVector("vectors/"+f, b, schema, h))
	}

	problems, open := checkCoverage(spec, vectors, found)
	r.Problems = append(r.Problems, problems...)
	r.Open = open
	return r
}

// loadSchema compiles spec/schema/vector.json and every schema it refers to, then checks it against
// its own examples (conformance-vectors-10).
func loadSchema(spec fs.FS) (*node, []string) {
	set := newSchemaSet()
	names, _ := fs.Glob(spec, "schema/*.json")
	var problems []string
	vectorID := ""
	for _, name := range names {
		b, err := fs.ReadFile(spec, name)
		if err == nil {
			err = set.add("spec/"+name, b)
		}
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if name == "schema/vector.json" {
			o, _ := read(b)
			vectorID = o.(*object).values["$id"].(string)
		}
	}
	if vectorID == "" {
		return nil, append(problems, "spec/schema/vector.json: missing or unreadable; no vector can be checked")
	}
	root, err := set.compile(vectorID + "#")
	if err == nil {
		err = set.link()
	}
	if err != nil {
		return nil, append(problems, "spec/schema/vector.json: does not compile: "+err.Error())
	}
	for _, side := range []string{"accepted", "refused"} {
		examples, _ := fs.Glob(spec, "schema/testdata/vector/"+side+"/*.json")
		for _, e := range examples {
			b, err := fs.ReadFile(spec, e)
			var bad *schemaError
			if err == nil {
				var v any
				if v, err = read(b); err == nil {
					bad = root.validate(v, "/")
				}
			}
			switch {
			case side == "accepted" && err != nil:
				problems = append(problems, "spec/"+e+": an accepted example that cannot be read: "+err.Error())
			case side == "accepted" && bad != nil:
				problems = append(problems, "spec/"+e+": an accepted example the schema refuses: "+bad.Error())
			case side == "refused" && err == nil && bad == nil:
				problems = append(problems, "spec/"+e+": a refused example the schema accepts")
			}
		}
	}
	return root, problems
}

// link compiles every schema a reference names, so that a broken reference fails the load instead of
// the first vector that reaches it.
func (s *schemaSet) link() error {
	for {
		var refs []string
		for _, n := range s.nodes {
			if n.ref != "" {
				if _, ok := s.nodes[n.ref]; !ok {
					refs = append(refs, n.ref)
				}
			}
		}
		if len(refs) == 0 {
			return nil
		}
		sort.Strings(refs)
		for _, ref := range refs {
			if _, err := s.compile(ref); err != nil {
				return err
			}
		}
	}
}

// runVector takes one vector through the checks of section 10, in order, and stops at the first
// that fails it.
func runVector(p string, b []byte, schema *node, h Handlers) Result {
	fail := func(check, reason string) Result { return Result{Path: p, Check: check, Reason: reason} }

	v, err := read(b)
	if err != nil {
		return fail(CheckHeader, err.Error())
	}
	// The runner's own reader refuses what the grammar, UTF-8 and repeated names refuse; the canonical
	// form refuses the rest, a fraction or an integer outside the safe range among them
	// (conformance-vectors-11).
	if _, err := canonical.Form(b); err != nil {
		return fail(CheckHeader, "the canonical form refuses the file: "+err.Error())
	}
	o, ok := v.(*object)
	if !ok {
		return fail(CheckHeader, "not a JSON object")
	}
	for _, m := range []string{"vector", "spec", "tests"} {
		w, ok := o.values[m]
		if !ok {
			return fail(CheckHeader, "no member "+strconv.Quote(m))
		}
		if _, ok := w.(string); !ok {
			return fail(CheckHeader, "its "+strconv.Quote(m)+" is not a string")
		}
	}
	own := strings.TrimSuffix(strings.TrimPrefix(p, "vectors/"), ".json")
	if name := o.values["vector"].(string); name != own {
		return fail(CheckHeader, "its \"vector\" is "+strconv.Quote(name)+", not its own path "+strconv.Quote(own))
	}

	if e := schema.validate(v, "/"); e != nil {
		return fail(CheckSchema, e.Error())
	}

	if reason := checkInputs(o); reason != "" {
		return fail(CheckInputs, reason)
	}

	// The schema holds every vector to a string operation; a schema that did not is a failed part,
	// never a panic (core-no-panic).
	op, ok := o.values["operation"].(string)
	if !ok {
		return fail(CheckOperation, "no member \"operation\" that is a string, so no part of the vector can run")
	}
	run, ok := h[op]
	if !ok || run == nil {
		return fail(CheckOperation, "no handler runs the operation "+strconv.Quote(op)+", so no part of the vector can run")
	}

	parts, crashed := call(run, Vector{Path: p, Operation: op, Bytes: b})
	if crashed != "" {
		return fail(CheckResults, "the handler stopped: "+crashed)
	}
	if len(parts) == 0 {
		return fail(CheckResults, "the handler reported no part, and a vector with no part compared does not pass")
	}
	for i, part := range parts {
		name := part.Name
		if name == "" {
			name = "#" + strconv.Itoa(i+1)
		}
		if part.Pending != "" {
			return fail(CheckResults, "part "+name+" is pending ("+part.Pending+"), and a pending part fails")
		}
		if part.Failure != "" {
			return fail(CheckResults, "part "+name+": "+part.Failure)
		}
	}
	return Result{Path: p, Parts: len(parts)}
}

// call runs a handler, turning a panic into a failure of the vector instead of the run.
func call(h Handler, v Vector) (parts []Part, crashed string) {
	defer func() {
		if x := recover(); x != nil {
			parts, crashed = nil, "it panicked"
		}
	}()
	return h(v), ""
}

// The forms of each byte input (conformance-vectors-8): a JSON value, text, or hex.
var (
	inputForms    = []string{"input", "inputText", "inputHex"}
	messageForms  = []string{"message", "messageText", "messageHex"}
	envelopeForms = []string{"envelope", "envelopeText", "envelopeHex"}
	receiptForms  = []string{"receipt", "receiptText", "receiptHex"}
	// The forms of an opening and of the acceptance list (setup-contract-13, setup-contract-15).
	openingForms    = []string{"opening", "openingText", "openingHex"}
	acceptanceForms = []string{"acceptances", "acceptancesText", "acceptancesHex"}
)

// checkInputs checks that each byte input of a vector is given exactly once, in one of its forms, and
// that every hex form decodes. It returns why the vector fails, or "".
func checkInputs(o *object) string {
	op, _ := o.values["operation"].(string)
	need := op == "canonical" || op == "sign" || op == "verify-signature" || op == "found" || op == "view"
	if r := once(o, "/", "the input", inputForms, need); r != "" {
		return r
	}
	if r := once(o, "/", "the acceptances", acceptanceForms, false); r != "" {
		return r
	}
	if l, ok := o.values["openings"].([]any); ok {
		for i, x := range l {
			item, ok := x.(*object)
			at := pointer("/openings", strconv.Itoa(i))
			if !ok {
				return at + ": not an object holding an opening"
			}
			if r := once(item, at, "the opening", openingForms, true); r != "" {
				return r
			}
		}
	}
	if op == "canonical" {
		if _, ok := o.values["input"]; ok {
			return "/: a canonical vector takes its input as inputText or inputHex only"
		}
	}
	if op == "sign" {
		if _, ok := o.values["input"]; !ok {
			return "/: a sign vector takes its input as input, an object"
		}
	}
	if steps, ok := o.values["steps"].([]any); ok {
		for i, s := range steps {
			step, ok := s.(*object)
			if !ok || step.values["op"] != "give" {
				continue
			}
			at := "/steps/" + strconv.Itoa(i)
			if r := once(step, at, "the message", messageForms, true); r != "" {
				return r
			}
			if r := once(step, at, "the envelope", envelopeForms, true); r != "" {
				return r
			}
		}
	}
	if r := receipts(o.values["receipts"], "/receipts"); r != "" {
		return r
	}
	if kept, ok := o.values["kept"].(*object); ok {
		for _, party := range kept.names {
			k, ok := kept.values[party].(*object)
			if !ok {
				continue
			}
			if r := receipts(k.values["delivered"], pointer(pointer("/kept", party), "delivered")); r != "" {
				return r
			}
		}
	}
	return ""
}

func receipts(list any, at string) string {
	l, ok := list.([]any)
	if !ok {
		return ""
	}
	for i, x := range l {
		item, ok := x.(*object)
		if !ok {
			return pointer(at, strconv.Itoa(i)) + ": not an object holding a receipt"
		}
		if r := once(item, pointer(at, strconv.Itoa(i)), "the receipt", receiptForms, true); r != "" {
			return r
		}
	}
	return ""
}

// once checks that at most one of forms is a member of o, at least one when need is set, and that a
// hex form decodes. forms are the JSON value, text and hex forms, in that order.
func once(o *object, at, what string, forms []string, need bool) string {
	var given []string
	for _, f := range forms {
		if _, ok := o.values[f]; ok {
			given = append(given, f)
		}
	}
	switch {
	case len(given) > 1:
		return at + ": " + what + " is given in " + strconv.Itoa(len(given)) + " forms, " + strings.Join(given, " and ") + "; it is given exactly once"
	case len(given) == 0 && need:
		return at + ": " + what + " is missing; it is given as one of " + strings.Join(forms, ", ")
	}
	if len(given) == 1 && given[0] == forms[2] {
		h, ok := o.values[forms[2]].(string)
		if !ok || !isHex(h) {
			return at + ": " + forms[2] + " does not decode: it is not an even number of lowercase hex digits"
		}
	}
	if len(given) == 1 && given[0] == forms[1] {
		if _, ok := o.values[forms[1]].(string); !ok {
			return at + ": " + forms[1] + " is not a string"
		}
	}
	return ""
}

func isHex(s string) bool {
	if len(s)%2 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// vectorName reports whether s names a vector: a group and a name, each in lowercase words joined by
// hyphens (conformance-vectors-1).
func vectorName(s string) bool {
	group, name, ok := strings.Cut(s, "/")
	return ok && words(group) && words(name)
}

func words(s string) bool {
	if s == "" {
		return false
	}
	for _, w := range strings.Split(s, "-") {
		if w == "" {
			return false
		}
		for i := 0; i < len(w); i++ {
			if (w[i] < 'a' || w[i] > 'z') && (w[i] < '0' || w[i] > '9') {
				return false
			}
		}
	}
	return true
}
