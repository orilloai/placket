package cli_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// hex64 is a salt as placket mints it: 32 bytes in 64 lowercase hex digits.
var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestGiveWritesTheUsersRoleAndA32ByteSalt reads the call placket give sends: its message is in
// ROLE_USER, the role of an A2A client, and its part's salt is 32 bytes (crossing-1, crossing-3).
func TestGiveWritesTheUsersRoleAndA32ByteSalt(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	w := newFakeWindow(t, a.dir)
	w.set(t, map[string]any{"ok": false, "rejection": "not-a-member"})
	run(t, env(), w.as(a, "p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "a note")...)
	var c struct {
		Call struct{ Args struct{ Message string } }
	}
	if err := json.Unmarshal(w.lastCall(), &c); err != nil {
		t.Fatalf("the call: %v", err)
	}
	msg, _ := base64.StdEncoding.DecodeString(c.Call.Args.Message)
	var m struct {
		Role  string `json:"role"`
		Parts []struct {
			Metadata map[string]struct {
				Salt string `json:"salt"`
			} `json:"metadata"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(msg, &m); err != nil || len(m.Parts) != 1 {
		t.Fatalf("the message: %s, %v", msg, err)
	}
	if salt := m.Parts[0].Metadata[mediator.Extension].Salt; m.Role != mediator.RoleUser || !hex64.MatchString(salt) {
		t.Fatalf("the message's role is %q and its salt %q", m.Role, salt)
	}
}

// TestDeliveriesCarry32ByteSalts gives a proposal that is asked about, accepts it, and fetches both
// deliveries with get --json: the released proposal carries the salt give minted, and the answer the
// salt the window minted, each 32 bytes (crossing-3, maybe-8).
func TestDeliveriesCarry32ByteSalts(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider")
	a.start(t)
	q := a.openOne(t, "Rename total.")
	must(t, a.as("decider", "answer", "--question", q)...)
	for who, kind := range map[string]string{"applier": "proposal", "proposer": "answer"} {
		o := object(t, must(t, a.as(who, "get", "--json")...))
		d := sub(t, o["delivery"])
		if str(d["kind"]) != kind || !hex64.MatchString(str(d["salt"])) {
			t.Fatalf("the %s fetched a %s with salt %s", who, str(d["kind"]), d["salt"])
		}
	}
}

// forged is a delivery to the key named to, of a text under a salt of n bytes, whose receipt is a
// pass the mediator's key signed over that salt and text, like the one that passed a real part: so
// only the salt's length is wrong when n is not 32. It returns the delivery and its receipt id.
func forged(t *testing.T, a *accord, real json.RawMessage, n int, text string) (map[string]any, string) {
	t.Helper()
	st, err := receipts.Decode(real)
	if err != nil {
		t.Fatal(err)
	}
	k, err := keyfile.Read(a.key("mediator"))
	if err != nil {
		t.Fatal(err)
	}
	salt := bytes.Repeat([]byte{byte(n)}, n)
	r, _, err := receipts.Build(receipts.Subject{Name: st.Subject.Name, Digest: receipts.Digest(salt, []byte(text))}, st.Predicate, k)
	if err != nil {
		t.Fatal(err)
	}
	f, err := receipts.Decode(r)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"receiptId": f.ID, "receipt": json.RawMessage(r), "messageId": "m-" + text, "kind": st.Subject.Name,
		"salt": hex.EncodeToString(salt), "text": text}, f.ID
}

// realDelivery gives a note from p1 to p2 and fetches it with --keep: it returns the receipt that
// passed it.
func realDelivery(t *testing.T, a *accord) json.RawMessage {
	t.Helper()
	must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", "a note")...)
	must(t, a.as("p2", "get", "--keep")...)
	return keptLines(t, a.state("p2"))[0]["receipt"]
}

// TestGetRecordsOnlyA32ByteSalt fetches from a window whose delivery carries a salt of 16 bytes:
// it is refused as a malformed reply, exit 3, and nothing is added to delivered.jsonl, salts.jsonl
// or kept.jsonl. The same delivery under a salt of 32 bytes is recorded (crossing-2, crossing-3).
func TestGetRecordsOnlyA32ByteSalt(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	real := realDelivery(t, a)
	files := []string{state.FileDelivered, state.FileSalts, "kept.jsonl"}
	before := map[string]string{}
	for _, f := range files {
		before[f] = fileOf(t, filepath.Join(a.state("p2"), f))
	}
	w := newFakeWindow(t, a.dir)
	short, _ := forged(t, a, real, 16, "sixteen")
	w.set(t, map[string]any{"ok": true, "delivery": short})
	if r := run(t, env(), w.as(a, "p2", "get", "--keep")...); r.code != 3 || r.stderr != "error reply\n" || r.stdout != "" {
		t.Fatalf("get of a 16-byte salt: %v", r)
	}
	for _, f := range files {
		if got := fileOf(t, filepath.Join(a.state("p2"), f)); got != before[f] {
			t.Fatalf("get of a 16-byte salt changed %s", f)
		}
	}
	good, id := forged(t, a, real, 32, "thirty-two")
	w.set(t, map[string]any{"ok": true, "delivery": good})
	must(t, w.as(a, "p2", "get", "--keep")...)
	for _, f := range files {
		if got := fileOf(t, filepath.Join(a.state("p2"), f)); !strings.HasPrefix(got, before[f]) || strings.Count(got, "\n") != strings.Count(before[f], "\n")+1 {
			t.Fatalf("get of a 32-byte salt added no line to %s", f)
		}
	}
	if !strings.Contains(fileOf(t, filepath.Join(a.state("p2"), state.FileSalts)), id) {
		t.Fatal("get of a 32-byte salt kept no salt for its receipt")
	}
}

// TestReleasedMatchesOnlyA32ByteSalt plants in a state directory a pass receipt to its key over a
// text and a salt of 16 bytes, the salt in salts.jsonl and the part in kept.jsonl: released --file
// and released --state find no match and exit 1. The same under a salt of 32 bytes matches
// (commands-5, commands-7).
func TestReleasedMatchesOnlyA32ByteSalt(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "p1", "p2")
	a.start(t)
	real := realDelivery(t, a)
	dir := a.state("p2")
	// The real note, kept, would match --state as it always did: kept.jsonl starts empty here.
	if err := os.WriteFile(filepath.Join(dir, "kept.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	plant := func(n int, text string) (string, string) {
		d, id := forged(t, a, real, n, text)
		receipt, _ := json.Marshal(d["receipt"])
		kept, _ := json.Marshal(d)
		appendTo(t, dir, state.FileDelivered, string(receipt))
		appendTo(t, dir, state.FileSalts, `{"receiptId":"`+id+`","salt":"`+d["salt"].(string)+`"}`)
		appendTo(t, dir, "kept.jsonl", string(kept))
		return writeFile(t, a.dir, text, []byte(text)), id
	}
	file16, _ := plant(16, "sixteen")
	unmatched(t, run(t, env(), "released", "--file", file16, "--state", dir))
	if r := run(t, env(), "released", "--state", dir); r.code != 1 || r.stdout != "" {
		t.Fatalf("released --state with a 16-byte salt: %v", r)
	}
	file32, id32 := plant(32, "thirty-two")
	matched(t, run(t, env(), "released", "--file", file32, "--state", dir), id32)
	if r := run(t, env(), "released", "--state", dir); r.code != 0 || r.stdout != id32+"\n" {
		t.Fatalf("released --state with a 32-byte salt: %v", r)
	}
}
