package local_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/mediator"
)

// failingRand is a salt source that always fails.
type failingRand struct{}

func (failingRand) Read([]byte) (int, error) { return 0, errors.New("no randomness") }

// TestTheDoorsOwnFailureHasItsOwnCode answers an ask while the window's salt source fails: the
// reply is refused under the door member with door-internal, never as the mediator's internal error,
// and the event says the same (rejection-vs-error).
func TestTheDoorsOwnFailureHasItsOwnCode(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	cfg := w.window(t)
	cfg.Rand = failingRand{}
	win := start(t, cfg)
	q := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.")
	if q.Outcome == nil || q.Outcome.Verdict != "escalate" {
		t.Fatalf("the proposal was not asked: %v", q.Outcome)
	}
	rep := w.call(t, "decider", local.OpResolve, local.ResolveArgs(q.Outcome.ReceiptID, mediator.Accepted, "accepted"))
	if rep.Door != local.Internal || rep.Error != "" || rep.Rejection != "" || rep.Outcome != nil {
		t.Fatalf("got door %q, error %q, rejection %q, want door %q alone", rep.Door, rep.Error, rep.Rejection, local.Internal)
	}
	if string(local.Internal) != "door-internal" {
		t.Fatalf("the code is %q, want door-internal", local.Internal)
	}
	win.mu.Lock()
	last := (*win.events)[len(*win.events)-1]
	win.mu.Unlock()
	if last.Door != local.Internal || last.Error != "" {
		t.Fatalf("the event says door %q, error %q", last.Door, last.Error)
	}
}

// replyFromSocket runs a socket that answers one call with reply and a line feed, and returns what a client
// read from it.
func replyFromSocket(t *testing.T, reply []byte) (local.Reply, error) {
	t.Helper()
	sock := filepath.Join(shortDir(t), "s")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	served := make(chan struct{})
	go func() {
		defer close(served)
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = bufio.NewReader(c).ReadBytes('\n')
		_, _ = c.Write(append(reply, '\n'))
	}()
	w := newWorld(t)
	c := w.client(t, "proposer")
	c.Socket = sock
	rep, err := c.Send([]byte(`{}`))
	<-served
	return rep, err
}

// TestAClientReadsNoReplyPastMaxReply refuses with code reply a reply one byte longer than
// MaxReply, four times MaxCall, and reads one of MaxReply bytes as far as its form.
func TestAClientReadsNoReplyPastMaxReply(t *testing.T) {
	t.Parallel()
	if local.MaxReply != 4*local.MaxCall {
		t.Fatalf("MaxReply is %d, want %d", local.MaxReply, 4*local.MaxCall)
	}
	long := []byte(`{"ok":true,"salt":"` + strings.Repeat("0", local.MaxReply) + `"}`)
	if _, err := replyFromSocket(t, long[:local.MaxReply+1]); err == nil || err.Error() != "local: reply" {
		t.Fatalf("a reply of MaxReply+1 bytes: got %v, want local: reply", err)
	}
	ok := []byte(`{"ok":true}`)
	exact := append(bytes.Repeat([]byte(" "), local.MaxReply-len(ok)), ok...)
	if rep, err := replyFromSocket(t, exact); err != nil || rep.Door != "" {
		t.Fatalf("a reply of MaxReply bytes: got %v", err)
	}
}

// TestRunEndsEveryConnectionItOpened connects to a window and sends nothing; once the window has
// stopped, the connection is closed at once, not when its deadline passes (goroutines-owned).
func TestRunEndsEveryConnectionItOpened(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	win := start(t, w.window(t))
	c, err := net.Dial("unix", w.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// The window's accept is not synchronised with the dial: give it the connection first.
	time.Sleep(100 * time.Millisecond)
	win.stop(t)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	// Closed or reset, the connection ends at once; only a read that waits out its deadline fails.
	if _, err := c.Read(make([]byte, 1)); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("reading after the window stopped: got %v, want the connection ended", err)
	}
}

// TestNoSecretPrintsFromACallReplyOrAsked prints a Call, a Reply and an Asked, alone and as a
// field of a struct, under every verb: none shows the body, the salt or the answer it
// holds, as text or in hex (secrets-never-print).
func TestNoSecretPrintsFromACallReplyOrAsked(t *testing.T) {
	t.Parallel()
	secret := "SEKRET-BODY"
	msg := secret
	for _, v := range []any{
		local.Call{Message: []byte(secret), Answer: secret, Bytes: []byte(secret)},
		local.Reply{Salt: []byte(secret), Delivery: []byte(secret), AnswerCall: []byte(secret)},
		local.Asked{Message: &msg, Kind: secret},
	} {
		noSecretPrints(t, v, secret)
	}
}

// TestNoSecretPrintsFromASignedAnswer prints a SignedAnswer every way: its answer never shows.
func TestNoSecretPrintsFromASignedAnswer(t *testing.T) {
	t.Parallel()
	secret := "SEKRET-ANSWER"
	a := local.SignedAnswer{Accord: "a", Question: "q", Action: "accepted", Answer: secret}
	noSecretPrints(t, a, secret)
	if a.String() != "local.SignedAnswer" || a.GoString() != "local.SignedAnswer" {
		t.Fatalf("String %q, GoString %q", a.String(), a.GoString())
	}
}

// noSecretPrints prints v, a pointer to it and a struct holding it in an exported field, under every
// verb, and fails if the secret, or its hex, shows. fmt reaches no method of a value in an unexported
// field, so no type can promise that case.
func noSecretPrints(t *testing.T, v any, secret string) {
	t.Helper()
	nested := struct{ Held any }{v}
	for _, x := range []any{v, &v, nested} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%X", "%q", "%d"} {
			out := fmt.Sprintf(verb, x)
			if strings.Contains(out, secret) || strings.Contains(strings.ToLower(out), hex.EncodeToString([]byte(secret))) {
				t.Fatalf("%T under %s printed %q", v, verb, out)
			}
		}
	}
}

// TestLocalErrorsQuoteNothing plants a secret in a reply that does not read and in one under
// another Accord: the client's error is its code alone (error-text-no-content).
func TestLocalErrorsQuoteNothing(t *testing.T) {
	t.Parallel()
	secret := "SEKRET-REPLY"
	for _, reply := range []string{
		`{"ok":true,"secret":"` + secret + `"}`,
		`{"ok":false,"error":"` + secret + `"}`,
		secret,
	} {
		_, err := replyFromSocket(t, []byte(reply))
		if err == nil || err.Error() != "local: reply" {
			t.Fatalf("%s: got %v, want local: reply", reply, err)
		}
	}
}
