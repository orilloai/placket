package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/state"
)

// What released prints for everything a key kept, with no file named (spec/protocol.md, section
// 11: commands-7).

// keptMatched checks what released prints for kept parts that match the receipt ids, in order: the
// ids on standard output, one a line, and on standard error not verified, the claim line and the
// scope line.
func keptMatched(t testing.TB, r result, claim string, ids ...string) {
	t.Helper()
	if r.code != 0 || r.stdout != strings.Join(ids, "\n")+"\n" || r.stderr != "not verified\n"+claim+"\n"+cli.Scope+"\n" {
		t.Fatalf("released: %v, want %v", r, ids)
	}
}

// keptUnmatched checks what released prints when no kept part matches: the line, on standard
// error alone, and exit 1.
func keptUnmatched(t testing.TB, r result, line string) {
	t.Helper()
	if r.code != 1 || r.stdout != "" || r.stderr != line+"\n" {
		t.Fatalf("released: %v, want %q", r, line)
	}
}

// keptAccord is an Accord whose applier fetched and kept, in order: a proposal accepted and
// released, from the proposer, of kind proposal and no re; a changed version of a second question,
// from the decider, of kind changed-version with re that question; and a changed version of a third,
// from the decider, re the third. The first two bodies hold 1250 at /total, the third 7.
type keptAccord struct {
	*accord
	q2, q3     string
	r1, r2, r3 string
}

func newKeptAccord(t testing.TB) *keptAccord {
	t.Helper()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	k := &keptAccord{accord: a}
	q1 := a.openOne(t, `{"total":1250}`)
	must(t, a.as("decider", "answer", "--question", q1)...)
	k.r1 = deliveredID(t, must(t, a.as("applier", "get", "--keep")...))
	k.q2 = a.openOne(t, `{"total":99}`)
	must(t, a.as("decider", "change", "--question", k.q2, "--text", `{"total": 1250}`, "--keep")...)
	k.r2 = deliveredID(t, must(t, a.as("applier", "get", "--keep")...))
	k.q3 = a.openOne(t, `{"total":5}`)
	must(t, a.as("decider", "change", "--question", k.q3, "--text", `{"total":7}`)...)
	k.r3 = deliveredID(t, must(t, a.as("applier", "get", "--keep")...))
	return k
}

// released runs released on the applier's state directory.
func (k *keptAccord) released(t testing.TB, args ...string) result {
	t.Helper()
	return run(t, env(), append([]string{"released", "--state", k.state("applier")}, args...)...)
}

// TestReleasedKeptEveryConditionAloneAndCombined asks the applier's directory, with no file, with
// no condition and with each of --kind, --at with --value or --value-file, --re and --from, alone
// and together: each answer is the first kept part in fetch order for which every condition holds,
// and conditions each met by some part but never by one part together match nothing (commands-7).
func TestReleasedKeptEveryConditionAloneAndCombined(t *testing.T) {
	t.Parallel()
	k := newKeptAccord(t)
	proposer, decider := k.refs["proposer"], k.refs["decider"]
	value := writeFile(t, k.dir, "value", []byte("7"))
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, k.r1},
		{[]string{"--kind", "proposal"}, k.r1},
		{[]string{"--kind", "changed-version"}, k.r2},
		{[]string{"--kind", "comment", "--kind", "changed-version"}, k.r2},
		{[]string{"--from", proposer}, k.r1},
		{[]string{"--from", decider}, k.r2},
		{[]string{"--re", k.q2}, k.r2},
		{[]string{"--re", k.q3}, k.r3},
		{[]string{"--from", decider, "--re", k.q3}, k.r3},
		{[]string{"--kind", "changed-version", "--from", decider, "--re", k.q2}, k.r2},
	} {
		keptMatched(t, k.released(t, c.args...), cli.ClaimReleasedKept, c.want)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--at", "/total", "--value", "1250"}, k.r1},
		{[]string{"--at", "/total", "--value", "7"}, k.r3},
		{[]string{"--at", "/total", "--value-file", value}, k.r3},
		{[]string{"--at", "/total", "--value", "1250", "--from", decider}, k.r2},
		{[]string{"--at", "/total", "--value", "1250", "--re", k.q2}, k.r2},
		{[]string{"--at", "/total", "--value", "1250", "--kind", "changed-version", "--from", decider, "--re", k.q2}, k.r2},
		{[]string{"--at", "", "--value", `{"total":7}`, "--re", k.q3}, k.r3},
	} {
		keptMatched(t, k.released(t, c.args...), cli.ClaimReleasedKeptValue, c.want)
	}
	for _, args := range [][]string{
		{"--kind", "answer"},
		{"--kind", "Proposal"},
		{"--from", k.refs["applier"]},
		{"--re", k.r1},
		{"--kind", "proposal", "--from", decider},
		{"--from", proposer, "--re", k.q2},
		{"--kind", "proposal", "--re", k.q3},
	} {
		keptUnmatched(t, k.released(t, args...), cli.LineNoKept)
	}
	for _, args := range [][]string{
		{"--at", "/total", "--value", "99"},
		{"--at", "/total", "--value", "1251"},
		{"--at", "/total", "--value", "7", "--re", k.q2},
		{"--at", "/total", "--value", "1250", "--re", k.q3},
		{"--at", "/total", "--value", "7", "--from", proposer},
		{"--at", "/total", "--value", "1250", "--kind", "proposal", "--from", decider},
	} {
		keptUnmatched(t, k.released(t, args...), cli.LineNoKeptValue)
	}

	o := object(t, k.released(t, "--re", k.q2, "--json"))
	members(t, o, "exit", "receiptId", "receipt", "kind", "verified", "claim", "notClaimed")
	is(t, o, "exit", "0")
	is(t, o, "receiptId", quote(k.r2))
	is(t, o, "kind", `"changed-version"`)
	is(t, o, "verified", "false")
	is(t, o, "claim", quote(cli.ClaimReleasedKept))
	is(t, o, "notClaimed", quote(cli.Scope))
	delivered, _ := os.ReadFile(filepath.Join(k.state("applier"), state.FileDelivered))
	if l := strings.Split(strings.TrimSuffix(string(delivered), "\n"), "\n"); len(l) != 3 || !sameJSON(t, string(o["receipt"]), l[1]) {
		t.Fatalf("receipt %s, delivered %s", o["receipt"], delivered)
	}
	o = object(t, k.released(t, "--at", "/total", "--value", "7", "--json"))
	is(t, o, "receiptId", quote(k.r3))
	is(t, o, "claim", quote(cli.ClaimReleasedKeptValue))
	if r := k.released(t, "--kind", "answer", "--json"); r.code != 1 || r.stdout != `{"exit":1}`+"\n" || r.stderr != cli.LineNoKept+"\n" {
		t.Fatalf("released --json of nothing: %v", r)
	}

	// The flags that go together, and those that read kept parts only.
	body := writeFile(t, k.dir, "body", []byte(`{"total":1250}`))
	for _, c := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"--at", "/total"}, "usage: placket released: --at goes with one of --value and --value-file\nerror usage\n"},
		{[]string{"--value", "7"}, "usage: placket released: --at goes with one of --value and --value-file\nerror usage\n"},
		{[]string{"--at", "/total", "--value", "7", "--value-file", value}, "usage: placket released: --at goes with one of --value and --value-file\nerror usage\n"},
		{[]string{"--file", body, "--re", k.q2}, "usage: placket released: --re and --every read kept parts: give them without --file\nerror usage\n"},
		{[]string{"--file", body, "--every"}, "usage: placket released: --re and --every read kept parts: give them without --file\nerror usage\n"},
		{[]string{"--at", "total", "--value", "7"}, "error path\n"},
		{[]string{"--at", "/total", "--value-file", filepath.Join(k.dir, "nowhere")}, "error file\n"},
	} {
		if r := k.released(t, c.args...); r.code != 3 || r.stdout != "" || r.stderr != c.stderr {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
	if r := run(t, env(), "released"); r.code != 3 || r.stderr != "usage: placket released: --state is required\nerror usage\n" {
		t.Fatalf("released: %v", r)
	}

	// With --file, --value and --from are taken as without it.
	out := filepath.Join(k.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	q4 := k.openOne(t, `{"total":3}`)
	must(t, k.as("decider", "answer", "--question", q4)...)
	r4, path := fetchOut(t, k.accord, "applier")
	matchedValue(t, k.released(t, "--file", path, "--at", "/total", "--value", "3"), r4)
	unmatchedValue(t, k.released(t, "--file", path, "--at", "/total", "--value", "4"))
	matched(t, k.released(t, "--file", path, "--from", proposer), r4)
	unmatched(t, k.released(t, "--file", path, "--from", decider))
}

// receiptAt is the time of the receipt of the id, as show reads it from the journal.
func receiptAt(t testing.TB, a *accord, id string) string {
	t.Helper()
	for i, l := range journalLines(t, a) {
		o := object(t, must(t, "show", "--receipt", writeFile(t, a.dir, "at.receipt", []byte(l+"\n")), "--json"))
		if string(o["receiptId"]) == quote(id) {
			var at string
			if json.Unmarshal(o["at"], &at) != nil || at == "" {
				t.Fatalf("receipt %d: at %s", i, o["at"])
			}
			return at
		}
	}
	t.Fatalf("no receipt %s in the journal", id)
	return ""
}

// TestReleasedKeptEveryMatchInFetchOrder asks the applier's directory with --every: the receipt id
// of each match on standard output, one a line, in fetch order, two releases of one value both
// among them; with --json, matches holds each one's receipt id, kind, time, sender and re when it
// has one, and an empty list when nothing matches (commands-7).
func TestReleasedKeptEveryMatchInFetchOrder(t *testing.T) {
	t.Parallel()
	k := newKeptAccord(t)
	decider := k.refs["decider"]
	keptMatched(t, k.released(t, "--every"), cli.ClaimReleasedKept, k.r1, k.r2, k.r3)
	keptMatched(t, k.released(t, "--every", "--from", decider), cli.ClaimReleasedKept, k.r2, k.r3)
	keptMatched(t, k.released(t, "--every", "--kind", "proposal"), cli.ClaimReleasedKept, k.r1)
	keptMatched(t, k.released(t, "--every", "--at", "/total", "--value", "1250"), cli.ClaimReleasedKeptValue, k.r1, k.r2)
	keptMatched(t, k.released(t, "--every", "--at", "/total", "--value", "1250", "--re", k.q2), cli.ClaimReleasedKeptValue, k.r2)
	keptUnmatched(t, k.released(t, "--every", "--kind", "answer"), cli.LineNoKept)
	keptUnmatched(t, k.released(t, "--every", "--at", "/total", "--value", "99"), cli.LineNoKeptValue)

	r := k.released(t, "--every", "--json")
	o := object(t, r)
	members(t, o, "exit", "matches", "verified", "claim", "notClaimed")
	is(t, o, "exit", "0")
	is(t, o, "verified", "false")
	is(t, o, "claim", quote(cli.ClaimReleasedKept))
	is(t, o, "notClaimed", quote(cli.Scope))
	want := `[{"receiptId":` + quote(k.r1) + `,"kind":"proposal","at":` + quote(receiptAt(t, k.accord, k.r1)) + `,"from":` + quote(k.refs["proposer"]) + `},` +
		`{"receiptId":` + quote(k.r2) + `,"kind":"changed-version","at":` + quote(receiptAt(t, k.accord, k.r2)) + `,"from":` + quote(decider) + `,"re":` + quote(k.q2) + `},` +
		`{"receiptId":` + quote(k.r3) + `,"kind":"changed-version","at":` + quote(receiptAt(t, k.accord, k.r3)) + `,"from":` + quote(decider) + `,"re":` + quote(k.q3) + `}]`
	is(t, o, "matches", want)
	if r.stderr != "not verified\n"+cli.ClaimReleasedKept+"\n"+cli.Scope+"\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
	for _, c := range []struct {
		args []string
		line string
	}{
		{[]string{"--every", "--kind", "answer", "--json"}, cli.LineNoKept},
		{[]string{"--every", "--at", "/total", "--value", "99", "--json"}, cli.LineNoKeptValue},
	} {
		if r := k.released(t, c.args...); r.code != 1 || r.stdout != `{"exit":1,"matches":[]}`+"\n" || r.stderr != c.line+"\n" {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
}

// TestReleasedKeptALineChangedAndNothingKept copies the applier's directory and changes its kept
// parts: a body with one byte changed, a salt changed, a kind changed and a receipt id the
// directory does not hold never match, though the part beside them does. A directory that kept
// nothing, and one that holds only parts its key gave or versions it gave as answers, match
// nothing (commands-7).
func TestReleasedKeptALineChangedAndNothingKept(t *testing.T) {
	t.Parallel()
	k := newKeptAccord(t)
	lines := keptLines(t, k.state("applier"))
	if len(lines) != 3 {
		t.Fatalf("kept %d lines", len(lines))
	}
	flip := func(s string, i int) string {
		b := []byte(s)
		b[i] ^= 0x01
		return string(b)
	}
	for _, c := range []struct {
		member string
		change func(string) string
	}{
		{"text", func(s string) string { return flip(s, 3) }},
		{"text", func(s string) string { return s + "\x0a" }},
		{"salt", func(s string) string { return flip(s, 0) }},
		{"kind", func(string) string { return "proposal" }},
		{"receiptId", func(string) string { return k.r3 }},
		{"receiptId", func(s string) string { return flip(s, len(s)-1) }},
	} {
		dir := copyDir(t, k.state("applier"))
		var s string
		if err := json.Unmarshal(lines[1][c.member], &s); err != nil {
			t.Fatalf("%s: %v", c.member, err)
		}
		changed := map[string]json.RawMessage{}
		for n, v := range lines[1] {
			changed[n] = v
		}
		changed[c.member] = json.RawMessage(quote(c.change(s)))
		b, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		var kept []string
		for i, l := range lines {
			if i == 1 {
				kept = append(kept, string(b))
				continue
			}
			m, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			kept = append(kept, string(m))
		}
		if err := os.WriteFile(filepath.Join(dir, state.FileKept), []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		check := func(args ...string) result {
			return run(t, env(), append([]string{"released", "--state", dir}, args...)...)
		}
		keptUnmatched(t, check("--re", k.q2), cli.LineNoKept)
		keptUnmatched(t, check("--at", "/total", "--value", "1250", "--from", k.refs["decider"]), cli.LineNoKeptValue)
		keptMatched(t, check("--every"), cli.ClaimReleasedKept, k.r1, k.r3)
	}

	// The proposer kept only the parts it gave, and the decider only a version it gave as an
	// answer; a directory that kept nothing matches nothing either.
	must(t, "released", "--state", k.state("applier"))
	keptUnmatched(t, run(t, env(), "released", "--state", k.state("proposer")), cli.LineNoKept)
	keptUnmatched(t, run(t, env(), "released", "--state", k.state("decider")), cli.LineNoKept)
	if n := len(keptLines(t, k.state("proposer"))); n != 3 {
		t.Fatalf("the proposer kept %d lines", n)
	}
	if n := len(keptLines(t, k.state("decider"))); n != 1 {
		t.Fatalf("the decider kept %d lines", n)
	}
	b := newAccord(t, "ann", "bob")
	b.start(t)
	must(t, b.as("ann", "give", "--to", b.refs["bob"], "--kind", "note", "--text", "not kept")...)
	g := must(t, b.as("bob", "get")...)
	keptUnmatched(t, run(t, env(), "released", "--state", b.state("bob")), cli.LineNoKept)
	if r := run(t, env(), "released", "--state", b.state("bob"), "--every", "--json"); r.code != 1 || r.stdout != `{"exit":1,"matches":[]}`+"\n" {
		t.Fatalf("released --every --json of nothing kept: %v", r)
	}
	matched(t, run(t, env(), "released", "--file", writeFile(t, b.dir, "f", []byte("not kept")), "--state", b.state("bob")), deliveredID(t, g))
}
