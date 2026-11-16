package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/state"
)

// What released prints for a value at a path inside a body (spec/protocol.md, section 11:
// commands-6).

// matchedValue checks what released prints for a value that matches the receipt id: the id on
// standard output, and on standard error not verified, the value's claim line and the scope line.
func matchedValue(t testing.TB, r result, id string) {
	t.Helper()
	if r.code != 0 || r.stdout != id+"\n" || r.stderr != "not verified\n"+cli.ClaimReleasedValue+"\n"+cli.Scope+"\n" {
		t.Fatalf("released --at: %v, want %s", r, id)
	}
}

// unmatchedValue checks what released prints for a value no release holds at the path.
func unmatchedValue(t testing.TB, r result) {
	t.Helper()
	if r.code != 1 || r.stdout != "" || r.stderr != "no release holds this value at this path\n" {
		t.Fatalf("released --at: %v", r)
	}
}

// fetchOut fetches the next delivery of who into a file of the Accord's out directory, and
// returns its receipt id and the file.
func fetchOut(t testing.TB, a *accord, who string) (id, path string) {
	t.Helper()
	out := filepath.Join(a.dir, "out")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	g := must(t, a.as(who, "get", "--out", out)...)
	return deliveredID(t, g), strings.TrimSuffix(g.stdout, "\n")
}

// TestReleasedAValueInADataBodyAndOneByteChanged gives bob a data body spelled loosely, fetches it,
// and checks values at paths inside it: each value's canonical bytes match the receipt, a string
// with its quotes and an object as canonical-1 writes it; the same bytes with any one byte changed,
// a line feed added, a string without its quotes, or an object spelled with a space, match nothing.
// With --json the object holds the receipt, its kind, verified false, the value's claim line and
// the scope line, or only exit 1 (commands-6).
func TestReleasedAValueInADataBodyAndOneByteChanged(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	src := writeFile(t, a.dir, "src.json", []byte(`{"lines": [ {"total": 1250} ], "customer": {"name": "Ann"}}`))
	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--file", src, "--as", "data")...)
	id, body := fetchOut(t, a, "bob")
	if b, err := os.ReadFile(body); err != nil || string(b) != `{"customer":{"name":"Ann"},"lines":[{"total":1250}]}` {
		t.Fatalf("get wrote %q, %v", b, err)
	}
	check := func(at string, v []byte, more ...string) result {
		return run(t, env(), append([]string{"released", "--file", body, "--at", at, "--value-file", writeFile(t, a.dir, "value", v), "--state", a.state("bob")}, more...)...)
	}
	for _, c := range []struct{ at, value string }{
		{"/customer/name", "\x22Ann\x22"},
		{"/lines/0/total", "1250"},
		{"/customer", `{"name":"Ann"}`},
		{"", `{"customer":{"name":"Ann"},"lines":[{"total":1250}]}`},
	} {
		matchedValue(t, check(c.at, []byte(c.value)), id)
		for i := range c.value {
			changed := []byte(c.value)
			changed[i] ^= 0x01
			unmatchedValue(t, check(c.at, changed))
		}
		unmatchedValue(t, check(c.at, []byte(c.value+"\x0a")))
	}
	unmatchedValue(t, check("/customer/name", []byte("Ann")))
	unmatchedValue(t, check("/customer", []byte(`{"name": "Ann"}`)))
	unmatchedValue(t, check("/lines/0/total", []byte("\x22Ann\x22")))
	unmatchedValue(t, check("/lines/1/total", []byte("1250")))

	r := check("/lines/0/total", []byte("1250"), "--json")
	o := object(t, r)
	members(t, o, "exit", "receiptId", "receipt", "kind", "verified", "claim", "notClaimed")
	is(t, o, "exit", "0")
	is(t, o, "receiptId", quote(id))
	is(t, o, "kind", `"note"`)
	is(t, o, "verified", "false")
	is(t, o, "claim", quote(cli.ClaimReleasedValue))
	is(t, o, "notClaimed", quote(cli.Scope))
	delivered, _ := os.ReadFile(filepath.Join(a.state("bob"), state.FileDelivered))
	if !sameJSON(t, string(o["receipt"]), strings.TrimSuffix(string(delivered), "\n")) {
		t.Fatalf("receipt %s, delivered %s", o["receipt"], delivered)
	}
	if r.stderr != "not verified\n"+cli.ClaimReleasedValue+"\n"+cli.Scope+"\n" {
		t.Fatalf("stderr %q", r.stderr)
	}
	r = check("/lines/0/total", []byte("1251"), "--json")
	if r.code != 1 || r.stdout != `{"exit":1}`+"\n" || r.stderr != "no release holds this value at this path\n" {
		t.Fatalf("released --at --json of nothing: %v", r)
	}
}

// TestReleasedAFractionInATextBody gives bob, as text and as raw bytes, a JSON text with a fraction
// that a data body refuses, and checks values inside it: each is read as the body spells it, the
// number 12.50 as those five bytes and an array with the spaces inside it, so 12.5, "12.50" and
// [1,2] match nothing. A text that is not a JSON text holds no value at any path, though the file
// itself matches (commands-6).
func TestReleasedAFractionInATextBody(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	if r := run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--file",
		writeFile(t, a.dir, "fraction.json", []byte(`{"total": 12.50}`)), "--as", "data")...); r.code != 3 || r.stderr != "error data\n" {
		t.Fatalf("give --as data of a fraction: %v", r)
	}
	// The raw body ends in another line ending, so that its bytes are not the text's.
	for as, end := range map[string]string{"text": "\x0a", "raw": "\x0d\x0a"} {
		text := []byte("{\"total\": 12.50, \"items\": [ 1, 2 ]}" + end)
		src := writeFile(t, a.dir, "src."+as, text)
		must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--file", src, "--as", as)...)
		id, body := fetchOut(t, a, "bob")
		if b, err := os.ReadFile(body); err != nil || string(b) != string(text) {
			t.Fatalf("%s: get wrote %q, %v", as, b, err)
		}
		check := func(at, v string) result {
			return run(t, env(), "released", "--file", body, "--at", at, "--value-file", writeFile(t, a.dir, "value", []byte(v)), "--state", a.state("bob"))
		}
		matchedValue(t, check("/total", "12.50"), id)
		matchedValue(t, check("/items", "[ 1, 2 ]"), id)
		matchedValue(t, check("/items/1", "2"), id)
		matchedValue(t, check("", `{"total": 12.50, "items": [ 1, 2 ]}`), id)
		for _, v := range []string{"12.5", "1250", "\x2212.50\x22", "12.50\x0a", " 12.50", "12.51"} {
			unmatchedValue(t, check("/total", v))
		}
		unmatchedValue(t, check("/items", "[1,2]"))
		unmatchedValue(t, check("", string(text)))
	}

	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "total: 12.50")...)
	g := must(t, a.as("bob", "get")...)
	plain := writeFile(t, a.dir, "plain", []byte("total: 12.50"))
	matched(t, run(t, env(), "released", "--file", plain, "--state", a.state("bob")), deliveredID(t, g))
	unmatchedValue(t, run(t, env(), "released", "--file", plain, "--at", "", "--value-file", plain, "--state", a.state("bob")))
}

// TestReleasedAMatchLimitedToKinds has the decider give the proposer the same data body twice, as a
// comment and as an answer, and checks the file and a value inside it with --kind: each kind
// matches its own receipt and names it, two kinds match the first receipt of either, and a kind
// neither part has matches nothing (commands-6).
func TestReleasedAMatchLimitedToKinds(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	src := writeFile(t, a.dir, "src.json", []byte(`{"total":1250}`))
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "comment", "--file", src, "--as", "data")...)
	must(t, a.as("decider", "give", "--to", a.refs["proposer"], "--kind", "answer", "--file", src, "--as", "data")...)
	comment, body := fetchOut(t, a, "proposer")
	answer, _ := fetchOut(t, a, "proposer")
	value := writeFile(t, a.dir, "value", []byte("1250"))
	file := func(more ...string) result {
		return run(t, env(), append([]string{"released", "--file", body, "--state", a.state("proposer")}, more...)...)
	}
	at := func(more ...string) result {
		return file(append([]string{"--at", "/total", "--value-file", value}, more...)...)
	}
	matched(t, file(), comment)
	matched(t, file("--kind", "answer"), answer)
	matched(t, file("--kind", "comment"), comment)
	matched(t, file("--kind", "answer", "--kind", "comment"), comment)
	unmatched(t, file("--kind", "proposal"))
	matchedValue(t, at(), comment)
	matchedValue(t, at("--kind", "answer"), answer)
	matchedValue(t, at("--kind", "comment"), comment)
	unmatchedValue(t, at("--kind", "proposal"))
	unmatchedValue(t, at("--kind", "Answer"))

	o := object(t, at("--kind", "answer", "--json"))
	is(t, o, "receiptId", quote(answer))
	is(t, o, "kind", `"answer"`)
	o = object(t, file("--kind", "comment", "--json"))
	is(t, o, "receiptId", quote(comment))
	is(t, o, "kind", `"comment"`)
	is(t, o, "claim", quote(cli.ClaimReleased))
}

// TestReleasedAValueNeverFromABlockAnotherKeyOrABodyNeverReleased plants in bob's state directory
// the receipt and salt of a data body blocked for him, and in ann's those of the same body that
// crossed to bob: the value inside matches neither. A body holding the same value at the same path
// that never crossed, in other bytes, matches nothing either. --at without --value-file, or the
// reverse, is a usage error, a path that is not a JSON pointer the error path, and a value file that
// cannot be read the error file (commands-6).
func TestReleasedAValueNeverFromABlockAnotherKeyOrABodyNeverReleased(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	src := writeFile(t, a.dir, "src.json", []byte(`{"total":1250}`))
	value := writeFile(t, a.dir, "value", []byte("1250"))
	r := run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "memo", "--file", src, "--as", "data", "--keep")...)
	if r.code != 1 || !strings.HasPrefix(r.stderr, "block no-match ") {
		t.Fatalf("give: %v", r)
	}
	blockID := strings.Fields(r.stderr)[2]
	salt := saltOf(t, a.state("ann"))
	must(t, a.as("bob", "get")...)
	appendTo(t, a.state("bob"), state.FileDelivered, journalLines(t, a)[0])
	appendTo(t, a.state("bob"), state.FileSalts, `{"receiptId":"`+blockID+`","salt":"`+salt+`"}`)
	check := func(file, dir string) result {
		return run(t, env(), "released", "--file", file, "--at", "/total", "--value-file", value, "--state", dir)
	}
	unmatchedValue(t, check(src, a.state("bob")))

	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--file", src, "--as", "data")...)
	id, body := fetchOut(t, a, "bob")
	matchedValue(t, check(body, a.state("bob")), id)
	for _, name := range []string{state.FileDelivered, state.FileSalts} {
		b, err := os.ReadFile(filepath.Join(a.state("bob"), name))
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			appendTo(t, a.state("ann"), name, l)
		}
	}
	unmatchedValue(t, check(body, a.state("ann")))
	for _, other := range []string{`{"total":1250,"x":1}`, `{"total": 1250}`, `{"total":1250}` + "\x0a", `[1250]`} {
		unmatchedValue(t, check(writeFile(t, a.dir, "other", []byte(other)), a.state("bob")))
	}

	for _, c := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"released", "--file", body, "--at", "/total", "--state", a.state("bob")}, "usage: placket released: --at goes with one of --value and --value-file\nerror usage\n"},
		{[]string{"released", "--file", body, "--value-file", value, "--state", a.state("bob")}, "usage: placket released: --at goes with one of --value and --value-file\nerror usage\n"},
		{[]string{"released", "--file", body, "--at", "total", "--value-file", value, "--state", a.state("bob")}, "error path\n"},
		{[]string{"released", "--file", body, "--at", "/~2", "--value-file", value, "--state", a.state("bob")}, "error path\n"},
		{[]string{"released", "--file", body, "--at", "/total", "--value-file", filepath.Join(a.dir, "nowhere"), "--state", a.state("bob")}, "error file\n"},
	} {
		if r := run(t, env(), c.args...); r.code != 3 || r.stdout != "" || r.stderr != c.stderr {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
	o := object(t, run(t, env(), "released", "--file", body, "--at", "total", "--value-file", value, "--state", a.state("bob"), "--json"))
	members(t, o, "exit", "error")
	is(t, o, "error", `"path"`)
}
