package cli_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/core/canonical"
)

// Every result under every flag (spec/protocol.md, section 11, commands-1): whatever combination of
// its flags a command runs with, its --json object holds every result the run produced. A result is
// what the run received from the window or made in the world: a receipt its work added to the
// journal, a delivery it fetched, a file it wrote. A list a command reads and prints, such as the
// open questions or the log, is one result, whose rows the plain output prints and the tests of its
// own command compare.
//
// commandFlags lists every command's flags, each with how it bears on the results of one run. Every
// combination of the flags that bear more or form, that holds a flag that bears more or is of a
// command whose runs produce more than one result without one, is run, and its object is checked
// against the journal, the delivered receipts and the files. A combination left out is one whose
// flags all bear input or form on a command with no base: the why of each says why it produces one
// result. A command whose flags are not exactly those of its row fails the test, so a flag lands
// with its row.

// How a flag bears on what one run of its command produces.
type bearing int

const (
	// input names an input, a place or a choice: one run produces no more results with it.
	input bearing = iota
	// more lets one run produce more than one result.
	more
	// form changes how each result is made or held, never how many there are.
	form
)

// A flagUse is how a flag bears on a run, why, and what a combination that holds it passes: a
// boolean flag alone, a value after it otherwise; {out} is the run's directory for files.
type flagUse struct {
	bearing bearing
	why     string
	arg     []string
}

// A commandRow is a command's words and its flags. base, when a run produces more than one result
// with no flag that bears more, says which. play runs one combination's arguments, with the socket
// flags and --json, and checks its object; it is nil for a command no combination of whose flags
// produces more than one result.
type commandRow struct {
	words []string
	base  string
	flags map[string]flagUse
	play  func(t *testing.T, args []string)
}

// The flags of every command, shared ones first.
var (
	jsonFlag   = flagUse{input, "prints the run's results as one object; makes none", nil}
	socketUses = map[string]flagUse{
		"json":   jsonFlag,
		"key":    {input, "the one key the run speaks for", nil},
		"socket": {input, "the one window the run calls", nil},
		"accord": {input, "the one Accord the run speaks under", nil},
		"state":  {input, "the one state directory the run records in", nil},
	}
	questionUse = flagUse{input, "the one question the run is about", nil}
)

// withSocket is the socket commands' flags and the command's own.
func withSocket(own map[string]flagUse) map[string]flagUse {
	m := map[string]flagUse{}
	for _, f := range []map[string]flagUse{socketUses, own} {
		for n, u := range f {
			m[n] = u
		}
	}
	return m
}

var commandFlags = []commandRow{
	{words: []string{"key", "new"}, flags: map[string]flagUse{
		"json": jsonFlag, "out": {input, "the one key file it makes", nil}}},
	{words: []string{"key", "ref"}, flags: map[string]flagUse{
		"json": jsonFlag, "key": {input, "the one key file it reads the reference of", nil}}},
	{words: []string{"declare"}, flags: map[string]flagUse{
		"json": jsonFlag, "key": {input, "the one key that signs the declaration", nil},
		"terms": {input, "the terms of the one declaration it makes", nil}}},
	{words: []string{"bind"}, flags: map[string]flagUse{
		"json": jsonFlag, "key": {input, "the one principal key that signs the binding", nil},
		"actor":    {input, "the actor of the one binding it makes", nil},
		"mediator": {input, "the mediator the one binding names", nil},
		"until":    {input, "the time the one binding ends", nil},
		"role":     {input, "each role goes into the one binding it makes", nil},
		"policy":   {input, "the policy whose hash the one binding names", nil}}},
	{words: []string{"founding"}, flags: map[string]flagUse{
		"json": jsonFlag, "policy": {input, "the policy of the one set it makes", nil},
		"mediator":    {input, "the mediator of the one set", nil},
		"card":        {input, "the card of the one set", nil},
		"declaration": {input, "each declaration goes into the one set", nil},
		"binding":     {input, "each binding goes into the one set", nil}}},
	{words: []string{"card"}, flags: map[string]flagUse{
		"json": jsonFlag, "default": {input, "which one card it prints", nil}}},
	{words: []string{"version"}, flags: map[string]flagUse{"json": jsonFlag}},
	{words: []string{"window"}, flags: map[string]flagUse{
		"json": jsonFlag, "key": {input, "the one mediator key it runs under", nil},
		"founding": {input, "the one founding set it runs", nil},
		"socket":   {input, "the one socket it listens on", nil},
		"journal":  {input, "the one journal it writes; its receipts are the results of the commands that call it", nil}}},
	{words: []string{"give"}, play: playGive, flags: withSocket(map[string]flagUse{
		"to":    {input, "the one receiver of the one part it gives", nil},
		"kind":  {input, "the kind of the one part", nil},
		"text":  {input, "the body of the one part", nil},
		"file":  {input, "the body of the one part, from a file", nil},
		"as":    {input, "how the one body is read", nil},
		"re":    {input, "what the one part is about", nil},
		"topic": {input, "the topic of the one part", nil},
		"keep":  {form, "keeps the part it gives in kept.jsonl", []string{"--keep"}},
		"wait":  {more, "waits for the question's closing, its release, the answer and every delivery fetched on the way", []string{"--wait"}}})},
	{words: []string{"result"}, play: playResult, base: "the closing, the release, the answer and every other delivery it fetches",
		flags: withSocket(map[string]flagUse{"question": questionUse})},
	{words: []string{"answer"}, play: playAnswer, base: "the answer's outcome and its release",
		flags: withSocket(map[string]flagUse{"question": questionUse, "text": {input, "the one answer's text", nil},
			"no-release": {input, "asks the one answer for no release", nil},
			"keep":       {form, "keeps the call it signs in kept.jsonl", []string{"--keep"}}})},
	{words: []string{"change"}, play: playChange, base: "the version's outcome and its release",
		flags: withSocket(map[string]flagUse{"question": questionUse,
			"text":       {input, "the one version", nil},
			"file":       {input, "the one version, from a file", nil},
			"keep":       {form, "keeps the version in kept.jsonl", []string{"--keep"}},
			"no-release": {input, "asks the one version for no release", nil},
			"comment":    {more, "gives the asker a comment besides", []string{"--comment", "Hidden, not dropped."}}})},
	{words: []string{"decline"}, play: playDecline, base: "the decline's outcome and its comment's",
		flags: withSocket(map[string]flagUse{"question": questionUse,
			"comment": {input, "the one comment, which a decline needs", nil}})},
	{words: []string{"cancel"}, flags: withSocket(map[string]flagUse{"question": questionUse})},
	{words: []string{"get"}, play: playGet, flags: withSocket(map[string]flagUse{
		"all":  {more, "fetches every delivery waiting", []string{"--all"}},
		"out":  {form, "writes each body to a file of the directory, its path in place of its body", []string{"--out", "{out}"}},
		"keep": {form, "keeps each delivery in kept.jsonl", []string{"--keep"}}})},
	{words: []string{"questions"}, flags: withSocket(nil)},
	{words: []string{"show"}, flags: map[string]flagUse{
		"json": jsonFlag, "receipt": {input, "the one receipt it shows", nil}}},
	{words: []string{"verify"}, flags: map[string]flagUse{
		"json": jsonFlag, "mediator": {input, "the one mediator whose journal it verifies", nil},
		"receipts": {input, "the one journal it verifies", nil},
		"founding": {input, "the founding set the one verification checks against", nil},
		"accord":   {input, "the one Accord id the founding set is checked against", nil},
		"answers":  {input, "the answer calls the one verification reads besides", nil},
		"subset":   {input, "the one list is a subset", nil},
		"delivery": {input, "the one delivery whose body is checked", nil}}},
	{words: []string{"log"}, flags: map[string]flagUse{
		"json": jsonFlag, "mediator": {input, "the one mediator whose journal it reads", nil},
		"receipts": {input, "the one journal the log is of", nil},
		"format":   {input, "how the one log is printed", nil},
		"words":    {input, "adds the texts to the one log", nil},
		"no-words": {input, "empties the comments of the one log", nil},
		"all":      {input, "a row of the one log for every crossing", nil},
		"kept":     {input, "each state directory the one log reads kept parts from", nil}}},
	{words: []string{"reconcile"}, flags: map[string]flagUse{
		"json": jsonFlag, "mediator": {input, "the one mediator whose journal it reads", nil},
		"accord":   {input, "the Accord of the one reconciliation", nil},
		"card":     {input, "the card of the one reconciliation", nil},
		"receipts": {input, "the one journal", nil},
		"kept":     {input, "each state directory goes into the one reconciliation", nil}}},
	{words: []string{"released"}, flags: map[string]flagUse{
		"json": jsonFlag, "file": {input, "the one file it matches", nil},
		"state":      {input, "the one state directory it matches against", nil},
		"at":         {input, "the one value inside the file it matches", nil},
		"value-file": {input, "the one value's bytes", nil},
		"value":      {input, "the one value, as text", nil},
		"kind":       {input, "each kind narrows the one match", nil},
		"re":         {input, "narrows the one match to a kept part's re", nil},
		"from":       {input, "narrows the one match to a sender", nil},
		"every":      {input, "lists every match it reads in the one list it prints; it makes nothing and fetches nothing", nil}}},
}

// TestJSONEveryResultUnderEveryFlag checks that the table lists every command and exactly its
// flags, each with a reason, and runs every combination that produces more than one result.
func TestJSONEveryResultUnderEveryFlag(t *testing.T) {
	t.Parallel()
	listed := map[string]bool{}
	for _, row := range commandFlags {
		listed[row.words[0]] = true
		var names []string
		for n, u := range row.flags {
			names = append(names, n)
			if u.why == "" {
				t.Errorf("%v --%s: no why", row.words, n)
			}
			if (u.bearing == input) != (u.arg == nil) {
				t.Errorf("%v --%s: an input flag takes no argument in a combination, and only it", row.words, n)
			}
		}
		sort.Strings(names)
		if got := cli.Flags(row.words...); !reflect.DeepEqual(got, names) {
			t.Errorf("%v takes the flags %v, and its row lists %v", row.words, got, names)
		}
		combos := combinations(row)
		if (len(combos) > 0) != (row.play != nil) {
			t.Errorf("%v: %d combinations produce more than one result, and play is set: %v", row.words, len(combos), row.play != nil)
			continue
		}
		for _, c := range combos {
			c, row := c, row
			t.Run(strings.Join(append(append([]string(nil), row.words...), c...), " "), func(t *testing.T) {
				t.Parallel()
				var args []string
				for _, n := range c {
					args = append(args, row.flags[n].arg...)
				}
				row.play(t, args)
			})
		}
	}
	for _, n := range cli.Commands() {
		if !listed[n] {
			t.Errorf("the command %s has no row", n)
		}
	}
}

// combinations are the combinations of a row's flags that bear more or form, each sorted, that
// produce more than one result: those that hold a flag that bears more, and every one when the
// command's runs produce more than one without.
func combinations(row commandRow) [][]string {
	var some []string
	for n, u := range row.flags {
		if u.bearing != input {
			some = append(some, n)
		}
	}
	sort.Strings(some)
	var out [][]string
	for set := 0; set < 1<<len(some); set++ {
		var c []string
		many := row.base != ""
		for i, n := range some {
			if set&(1<<i) != 0 {
				c = append(c, n)
				many = many || row.flags[n].bearing == more
			}
		}
		if many {
			out = append(out, c)
		}
	}
	return out
}

// receiptIDs are the receipt ids of a file of one receipt per line, in order; none when it is
// missing.
func receiptIDs(t testing.TB, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" {
			continue
		}
		var env struct{ Payload string }
		if err := json.Unmarshal([]byte(l), &env); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		p, err := base64.StdEncoding.DecodeString(env.Payload)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		c, err := canonical.Form(p)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		id, _ := canonical.TypedHash(c)
		ids = append(ids, id)
	}
	return ids
}

// A held is what an object holds: every receiptId anywhere in it, every delivery (an object with a
// messageId) and the lists of deliveries, in order.
type held struct {
	ids        map[string]bool
	deliveries []map[string]any
	lists      [][]map[string]any
}

func (h *held) walk(v any) {
	switch v := v.(type) {
	case map[string]any:
		if id, ok := v["receiptId"].(string); ok {
			h.ids[id] = true
		}
		if _, ok := v["messageId"]; ok {
			h.deliveries = append(h.deliveries, v)
		}
		for _, m := range v {
			h.walk(m)
		}
	case []any:
		var list []map[string]any
		for _, m := range v {
			if d, ok := m.(map[string]any); ok && d["messageId"] != nil {
				list = append(list, d)
			}
			h.walk(m)
		}
		if len(list) > 0 {
			h.lists = append(h.lists, list)
		}
	}
}

// play runs args as who with --json, while during runs beside it when it is set, and checks that
// its object holds every receipt the run added to the journal, exactly the deliveries it fetched,
// each list of them in the order fetched, and the path of every file it wrote to {out}, each beside
// its delivery. The run must produce more than one result. It returns the run.
func play(t *testing.T, a *accord, who string, args []string, during func()) result {
	t.Helper()
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	var full []string
	for _, s := range args {
		full = append(full, strings.ReplaceAll(s, "{out}", out))
	}
	full = append(a.as(who, full...), "--json")
	journal, delivered := filepath.Join(a.journal(), "receipts.jsonl"), filepath.Join(a.state(who), "delivered.jsonl")
	j0, d0 := len(receiptIDs(t, journal)), len(receiptIDs(t, delivered))
	var r result
	if during == nil {
		r = run(t, env(), full...)
	} else {
		done := make(waiting, 1)
		go func() { done <- run(t, env(), full...) }()
		during()
		r = done.end(t)
	}
	added, fetched := receiptIDs(t, journal)[j0:], receiptIDs(t, delivered)[d0:]
	var v any
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil || strings.Count(r.stdout, "\n") != 1 {
		t.Fatalf("not one object: %v", r)
	}
	h := &held{ids: map[string]bool{}}
	h.walk(v)
	for _, id := range append(append([]string(nil), added...), fetched...) {
		if !h.ids[id] {
			t.Errorf("the object leaves out %s: %s", id, r.stdout)
		}
	}
	var got []string
	for _, d := range h.deliveries {
		got = append(got, d["receiptId"].(string))
	}
	if want := append([]string(nil), fetched...); !sameSet(got, want) {
		t.Errorf("the object holds the deliveries %v, and the run fetched %v", got, want)
	}
	at := map[string]int{}
	for i, id := range fetched {
		at[id] = i
	}
	for _, list := range h.lists {
		for i := 1; i < len(list); i++ {
			if at[list[i-1]["receiptId"].(string)] >= at[list[i]["receiptId"].(string)] {
				t.Errorf("a list out of fetch order: %s", r.stdout)
			}
		}
	}
	files, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, d := range h.deliveries {
		if p, ok := d["path"].(string); ok {
			paths[p] = true
			if filepath.Base(p) != strings.TrimPrefix(d["receiptId"].(string), canonical.HashPrefix)+"."+d["kind"].(string) {
				t.Errorf("the path %s beside the delivery %s", p, d["receiptId"])
			}
		}
	}
	for _, f := range files {
		if !paths[filepath.Join(out, f.Name())] {
			t.Errorf("the object leaves out the file %s: %s", f.Name(), r.stdout)
		}
	}
	if len(paths) != len(files) {
		t.Errorf("the object names %d paths, and the run wrote %d files", len(paths), len(files))
	}
	if n := len(added) + len(fetched) + len(files); n < 2 {
		t.Fatalf("the combination produced %d result; its row says why, or it bears more: %v", n, r)
	}
	return r
}

// sameSet reports whether a and b hold the same strings, as many times each.
func sameSet(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b) || len(a) == 0 && len(b) == 0
}

// playGet fetches three notes queued for p2.
func playGet(t *testing.T, args []string) {
	a := newAccord(t, "p1", "p2")
	a.start(t)
	give(t, a, []string{"one", "two", "three"})
	if r := play(t, a, "p2", append([]string{"get"}, args...), nil); r.code != 0 {
		t.Fatalf("get: %v", r)
	}
}

// commentTo gives the proposer a comment on the question from the decider.
func commentTo(t *testing.T, a *accord, q, text string) {
	t.Helper()
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "comment", "--text", text, "--re", q, "--topic", "billing")...)
}

// playGive gives a proposal that asks; with --wait, the decider comments twice and accepts it.
func playGive(t *testing.T, args []string) {
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	r := play(t, a, "proposer", append([]string{"give", "--to", a.refs["applier"], "--kind", "proposal", "--text", "Rename total.", "--topic", "billing"}, args...), func() {
		q := a.question(t, "decider")
		commentTo(t, a, q, "first")
		commentTo(t, a, q, "second")
		must(t, a.as("decider", "answer", "--question", q)...)
	})
	if r.code != 0 {
		t.Fatalf("give: %v", r)
	}
}

// playResult asks for the result of a proposal the decider commented on twice and accepted.
func playResult(t *testing.T, args []string) {
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	commentTo(t, a, q, "first")
	commentTo(t, a, q, "second")
	must(t, a.as("decider", "answer", "--question", q)...)
	if r := play(t, a, "proposer", append([]string{"result", "--question", q}, args...), nil); r.code != 0 {
		t.Fatalf("result: %v", r)
	}
}

// playAnswer accepts an open proposal.
func playAnswer(t *testing.T, args []string) {
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	if r := play(t, a, "decider", append([]string{"answer", "--question", q}, args...), nil); r.code != 0 {
		t.Fatalf("answer: %v", r)
	}
}

// playChange changes an open proposal.
func playChange(t *testing.T, args []string) {
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Drop notes.")
	if r := play(t, a, "decider", append([]string{"change", "--question", q, "--text", "Hide notes."}, args...), nil); r.code != 0 {
		t.Fatalf("change: %v", r)
	}
}

// playDecline declines an open proposal with a comment.
func playDecline(t *testing.T, args []string) {
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Delete invoices.")
	if r := play(t, a, "decider", append([]string{"decline", "--question", q, "--comment", "Kept seven years."}, args...), nil); r.code != 0 {
		t.Fatalf("decline: %v", r)
	}
}
