package cli_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/state"
)

// What released prints for a file (spec/protocol.md, section 11: commands-5).

// deliveredID is the receipt id of the delivery get printed on standard error.
func deliveredID(t testing.TB, r result) string {
	t.Helper()
	f := strings.Fields(r.lines()[0])
	if len(f) < 2 || f[0] != "delivery" || !strings.HasPrefix(f[1], "sha256:") {
		t.Fatalf("get: %v", r)
	}
	return f[1]
}

// writeFile writes b to a new file of dir and returns its path.
func writeFile(t testing.TB, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// matched checks what released prints for a file that matches the receipt id: the id on standard
// output, and on standard error not verified, the claim line and the scope line.
func matched(t testing.TB, r result, id string) {
	t.Helper()
	if r.code != 0 || r.stdout != id+"\n" || r.stderr != "not verified\n"+cli.ClaimReleased+"\n"+cli.Scope+"\n" {
		t.Fatalf("released: %v, want %s", r, id)
	}
}

// unmatched checks what released prints for a file no release matches.
func unmatched(t testing.TB, r result) {
	t.Helper()
	if r.code != 1 || r.stdout != "" || r.stderr != "no release matches these bytes\n" {
		t.Fatalf("released: %v", r)
	}
}

// TestReleasedAFileAndOneByteChanged gives bob a text, a raw body and a data body, fetches each
// with get --out, and checks each file: the bytes get wrote match the receipt they crossed under,
// and the same bytes with one byte changed, or the data as it was spelled before its canonical
// form, match nothing. A text fetched without --out matches too, through the salt kept beside its
// receipt, and a part fetched with --keep before any salt was kept matches through kept.jsonl. With
// --json the object holds the receipt id, the receipt, its kind, the claim and the scope line, or
// only exit 1 (commands-5).
func TestReleasedAFileAndOneByteChanged(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("\x00\xff\x10\x0a\xe2\x82")
	for _, c := range []struct {
		as      string
		body    []byte
		written []byte
	}{
		{"text", []byte("hello\x0a"), []byte("hello\x0a")},
		{"raw", raw, raw},
		{"data", []byte(`{"b":1, "a":[1, 2.0]}`), []byte(`{"a":[1,2],"b":1}`)},
	} {
		src := writeFile(t, a.dir, "src."+c.as, c.body)
		must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--file", src, "--as", c.as)...)
		g := must(t, a.as("bob", "get", "--out", out)...)
		id, path := deliveredID(t, g), strings.TrimSuffix(g.stdout, "\n")
		b, err := os.ReadFile(path)
		if err != nil || string(b) != string(c.written) {
			t.Fatalf("%s: get wrote %x, %v", c.as, b, err)
		}
		matched(t, run(t, env(), "released", "--file", path, "--state", a.state("bob")), id)
		for i := range b {
			changed := append([]byte(nil), b...)
			changed[i] ^= 0x01
			unmatched(t, run(t, env(), "released", "--file", writeFile(t, a.dir, "changed", changed), "--state", a.state("bob")))
		}
		unmatched(t, run(t, env(), "released", "--file", writeFile(t, a.dir, "longer", append(append([]byte(nil), b...), 0x0a)), "--state", a.state("bob")))
		if c.as == "data" {
			unmatched(t, run(t, env(), "released", "--file", src, "--state", a.state("bob")))
		}

		r := run(t, env(), "released", "--file", path, "--state", a.state("bob"), "--json")
		o := object(t, r)
		members(t, o, "exit", "receiptId", "receipt", "kind", "verified", "claim", "notClaimed")
		is(t, o, "exit", "0")
		is(t, o, "receiptId", quote(id))
		is(t, o, "kind", `"note"`)
		is(t, o, "verified", "false")
		is(t, o, "claim", quote(cli.ClaimReleased))
		is(t, o, "notClaimed", quote(cli.Scope))
		delivered, _ := os.ReadFile(filepath.Join(a.state("bob"), state.FileDelivered))
		l := strings.Split(strings.TrimSuffix(string(delivered), "\n"), "\n")
		if !sameJSON(t, string(o["receipt"]), l[len(l)-1]) {
			t.Fatalf("receipt %s, delivered %s", o["receipt"], l[len(l)-1])
		}
		if r.stderr != "not verified\n"+cli.ClaimReleased+"\n"+cli.Scope+"\n" {
			t.Fatalf("stderr %q", r.stderr)
		}
		r = run(t, env(), "released", "--file", writeFile(t, a.dir, "nothing", []byte("x")), "--state", a.state("bob"), "--json")
		if r.code != 1 || r.stdout != `{"exit":1}`+"\n" || r.stderr != "no release matches these bytes\n" {
			t.Fatalf("released --json of nothing: %v", r)
		}
	}

	// Fetched without --out: the body on standard output, the salt in the state directory.
	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "printed")...)
	g := must(t, a.as("bob", "get")...)
	if g.stdout != "printed\n" {
		t.Fatalf("get: %v", g)
	}
	matched(t, run(t, env(), "released", "--file", writeFile(t, a.dir, "printed", []byte("printed")), "--state", a.state("bob")), deliveredID(t, g))

	// Fetched with --keep, by a state directory that kept no salt: kept.jsonl holds it.
	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "kept")...)
	k := must(t, a.as("bob", "get", "--keep")...)
	if err := os.Remove(filepath.Join(a.state("bob"), state.FileSalts)); err != nil {
		t.Fatal(err)
	}
	matched(t, run(t, env(), "released", "--file", writeFile(t, a.dir, "kept", []byte("kept")), "--state", a.state("bob")), deliveredID(t, k))
	unmatched(t, run(t, env(), "released", "--file", writeFile(t, a.dir, "printed", []byte("printed")), "--state", a.state("bob")))
}

// TestReleasedAProposalReleasedAfterAnAccept asks about a proposal, accepts it with its release,
// and fetches it as the applier: the file matches the release receipt, a crossing receipt from the
// proposer to the applier; the proposer's state matches nothing, since it fetched no proposal; and
// the proposer's file of the answer it fetched matches the answer's receipt (commands-5).
func TestReleasedAProposalReleasedAfterAnAccept(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	out := filepath.Join(a.dir, "out")
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	q := a.openOne(t, "Rename total to amount_due.")
	must(t, a.as("decider", "answer", "--question", q)...)
	g := must(t, a.as("applier", "get", "--out", out)...)
	id, path := deliveredID(t, g), strings.TrimSuffix(g.stdout, "\n")
	j := journalLines(t, a)
	if len(j) != 3 {
		t.Fatalf("journal %v", j)
	}
	shown := must(t, "show", "--receipt", writeFile(t, a.dir, "release.receipt", []byte(j[2]+"\n")), "--json")
	so := object(t, shown)
	is(t, so, "receiptId", quote(id))
	is(t, so, "from", quote(a.refs["proposer"]))
	is(t, so, "to", quote(a.refs["applier"]))
	is(t, so, "word", `"pass"`)
	matched(t, run(t, env(), "released", "--file", path, "--state", a.state("applier")), id)
	unmatched(t, run(t, env(), "released", "--file", path, "--state", a.state("proposer")))
	unmatched(t, run(t, env(), "released", "--file", writeFile(t, a.dir, "shorter", []byte("Rename total to amount_due")), "--state", a.state("applier")))

	ans := must(t, a.as("proposer", "get", "--out", out)...)
	matched(t, run(t, env(), "released", "--file", strings.TrimSuffix(ans.stdout, "\n"), "--state", a.state("proposer")), deliveredID(t, ans))
}

// saltOf is the salt in the message of a part given with --keep, as its line of kept.jsonl holds it.
func saltOf(t testing.TB, dir string) string {
	t.Helper()
	l := keptLines(t, dir)
	m, err := base64.StdEncoding.DecodeString(str(l[len(l)-1]["message"]))
	if err != nil {
		t.Fatal(err)
	}
	s := regexp.MustCompile(`"salt":"([0-9a-f]+)"`).FindSubmatch(m)
	if s == nil {
		t.Fatalf("no salt in %s", m)
	}
	return string(s[1])
}

// appendTo appends a line to a file of a state directory.
func appendTo(t testing.TB, dir, name, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestReleasedNeverABlockOrAnotherKeysPart plants in bob's state directory the receipt of a part
// blocked for him, with its salt, and in ann's the receipt and salt of a part that crossed to bob:
// neither matches, though the bytes have the receipt's digest under that salt. A flag missing, a
// file that cannot be read and a directory that is no state directory are tool errors (commands-5).
func TestReleasedNeverABlockOrAnotherKeysPart(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "ann", "bob")
	a.start(t)
	r := run(t, env(), a.as("ann", "give", "--to", a.refs["bob"], "--kind", "memo", "--text", "blocked", "--keep")...)
	if r.code != 1 || !strings.HasPrefix(r.stderr, "block no-match ") {
		t.Fatalf("give: %v", r)
	}
	blockID := strings.Fields(r.stderr)[2]
	salt := saltOf(t, a.state("ann"))
	must(t, a.as("bob", "get")...)
	appendTo(t, a.state("bob"), state.FileDelivered, journalLines(t, a)[0])
	appendTo(t, a.state("bob"), state.FileSalts, `{"receiptId":"`+blockID+`","salt":"`+salt+`"}`)
	file := writeFile(t, a.dir, "blocked", []byte("blocked"))
	unmatched(t, run(t, env(), "released", "--file", file, "--state", a.state("bob")))

	must(t, a.as("ann", "give", "--to", a.refs["bob"], "--kind", "note", "--text", "crossed")...)
	g := must(t, a.as("bob", "get")...)
	crossed := writeFile(t, a.dir, "crossed", []byte("crossed"))
	matched(t, run(t, env(), "released", "--file", crossed, "--state", a.state("bob")), deliveredID(t, g))
	for _, name := range []string{state.FileDelivered, state.FileSalts} {
		b, err := os.ReadFile(filepath.Join(a.state("bob"), name))
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			appendTo(t, a.state("ann"), name, l)
		}
	}
	unmatched(t, run(t, env(), "released", "--file", crossed, "--state", a.state("ann")))

	for _, c := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"released", "--file", crossed}, "usage: placket released: --state is required\nerror usage\n"},
		{[]string{"released", "--file", filepath.Join(a.dir, "nowhere"), "--state", a.state("bob")}, "error file\n"},
		{[]string{"released", "--file", crossed, "--state", a.dir}, "error state actor\n"},
	} {
		if r := run(t, env(), c.args...); r.code != 3 || r.stdout != "" || r.stderr != c.stderr {
			t.Fatalf("%v: %v", c.args, r)
		}
	}
}
