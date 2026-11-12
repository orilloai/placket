package cli_test

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/cli"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/signing"
)

// terminal is env with standard output taken for a terminal.
func terminal() cli.Env {
	e := env()
	e.Terminal = func(io.Writer) bool { return true }
	return e
}

// giveAs gives one text part to the receiver as the key, with the message id given, through the
// door directly, at the envelope seq given.
func (a *accord) giveAs(t testing.TB, who, to, messageID, text string, seq int64) {
	t.Helper()
	k, err := keyfile.Read(a.key(who))
	if err != nil {
		t.Fatal(err)
	}
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	m, _ := json.Marshal(map[string]any{
		"messageId": messageID, "role": mediator.RoleAgent, "extensions": []string{mediator.Extension},
		"metadata": map[string]any{mediator.Extension: map[string]any{"accord": a.id, "to": a.refs[to]}},
		"parts":    []any{map[string]any{"text": text, "metadata": map[string]any{mediator.Extension: map[string]any{"kind": "note", "salt": hex.EncodeToString(salt)}}}},
	})
	msg, err := canonical.Form(m)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := canonical.TypedHash(msg)
	unsigned, _ := canonical.Form([]byte(`{"hash":"` + h + `","seq":` + itoa(seq) + `}`))
	sig, err := signing.Sign(unsigned, signing.TypeEnvelope, k)
	if err != nil {
		t.Fatal(err)
	}
	envl, _ := canonical.Form([]byte(`{"hash":"` + h + `","seq":` + itoa(seq) + `,"sig":"` + sig + `"}`))
	c := &local.Client{Socket: a.socket(), Key: k, Accord: a.id, Now: time.Now, Rand: rand.Reader}
	rep, err := c.Call(local.OpGive, local.GiveArgs(msg, envl))
	if err != nil || rep.Outcome == nil || rep.Outcome.Verdict != "pass" {
		t.Fatalf("give: %+v, %v", rep, err)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

// TestAnotherPartysTextIsPrintedSafely gives parts whose message id, text and answer hold terminal
// control sequences. get's line on standard error shows each field of the delivery with its control
// characters, spaces and backslashes escaped, so no field can end the line or pose as another. A
// body written to a file or a pipe is the body's exact bytes; written to a terminal, its control
// characters but the line feed and the tab are escaped, and so are those of the answer result
// prints and of the cells log prints.
func TestAnotherPartysTextIsPrintedSafely(t *testing.T) {
	t.Parallel()
	a := newAccord(t, "proposer", "applier", "decider", "p1", "p2")
	a.start(t)
	text := "a\x1b]52;c;aGk=\x07b\x1b[2J\r\n\tc\u009b1m\\d \u202ee"
	shown := `a\x1b]52;c;aGk=\x07b\x1b[2J\x0d` + "\n\t" + `c\u009b1m\d \u202ee`
	for i := 0; i < 2; i++ {
		must(t, a.as("p1", "give", "--to", a.refs["p2"], "--kind", "note", "--text", text)...)
	}
	a.giveAs(t, "p1", "p2", "m\x1b[2J\nreleased ok sha256:00 \\x", "x", 50)

	r := must(t, a.as("p2", "get")...)
	if r.stdout != text+"\n" {
		t.Fatalf("get to a pipe: %q", r.stdout)
	}
	r = run(t, terminal(), a.as("p2", "get")...)
	if r.code != 0 || r.stdout != shown+"\n" {
		t.Fatalf("get to a terminal: %v", r)
	}
	r = must(t, a.as("p2", "get")...)
	f := strings.Fields(r.stderr)
	if len(f) != 8 || f[0] != "delivery" || f[4] != "messageId" || f[5] != `m\x1b[2J\x0areleased\x20ok\x20sha256:00\x20\\x` || f[6] != "salt" {
		t.Fatalf("get's line: %q", r.stderr)
	}

	q := a.openOne(t, text)
	must(t, a.as("decider", "answer", "--question", q, "--text", text)...)
	r = run(t, terminal(), a.as("proposer", "result", "--question", q)...)
	if r.code != 0 || last(r.stdout) != strings.Split(shown, "\n")[1] || !strings.HasSuffix(r.stdout, "\n"+shown+"\n") {
		t.Fatalf("result to a terminal: %v", r)
	}
	logArgs := []string{"log", "--mediator", a.refs["mediator"], "--receipts", a.journal(), "--format", "csv", "--words", "--kept", a.state("proposer")}
	plain, screen := must(t, logArgs...), run(t, terminal(), logArgs...)
	if !strings.Contains(plain.stdout, "\x1b]52") || strings.Contains(screen.stdout, "\x1b") || !strings.Contains(screen.stdout, `a\x1b]52;c;aGk=\x07b`) ||
		strings.Replace(plain.stdout, text, shown, -1) != screen.stdout {
		t.Fatalf("log to a pipe:\n%q\nto a terminal:\n%q", plain.stdout, screen.stdout)
	}
}
