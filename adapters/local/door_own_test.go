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
	"syscall"
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
	return streamFromSocket(t, func(c net.Conn) { _, _ = c.Write(append(reply, '\n')) })
}

// streamFromSocket runs a socket that answers one call by running write on its connection, and
// returns what a client read from it.
func streamFromSocket(t *testing.T, write func(net.Conn)) (local.Reply, error) {
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
		write(c)
	}()
	w := newWorld(t)
	c := w.client(t, "proposer")
	c.Socket = sock
	c.Timeout = time.Minute
	rep, err := c.Send([]byte(`{}`))
	<-served
	return rep, err
}

// TestAClientReadsNoReplyPastMaxReply refuses with code reply a well-formed questions reply longer
// than MaxReply, four times MaxCall, and a reply that never ends, which it stops reading at once
// rather than at its deadline; it reads one of MaxReply bytes as far as its form.
func TestAClientReadsNoReplyPastMaxReply(t *testing.T) {
	t.Parallel()
	if local.MaxReply != 4*local.MaxCall {
		t.Fatalf("MaxReply is %d, want %d", local.MaxReply, 4*local.MaxCall)
	}
	head := `{"ok":true,"questions":[{"asker":"` + key(t, "decider").Reference() + `","deadline":"2026-01-01T00:00:00Z","form":"text","id":"sha256:` +
		strings.Repeat("0", 64) + `","kind":"proposal","message":"`
	tail := `"}]}`
	wellFormed := func(n int) []byte { return []byte(head + strings.Repeat("a", n-len(head)-len(tail)) + tail) }
	if rep, err := replyFromSocket(t, wellFormed(local.MaxReply)); err != nil || len(rep.Questions) != 1 {
		t.Fatalf("a questions reply of MaxReply bytes: got %d questions, %v", len(rep.Questions), err)
	}
	if _, err := replyFromSocket(t, wellFormed(local.MaxReply+1)); err == nil || err.Error() != "local: reply" {
		t.Fatalf("a questions reply of MaxReply+1 bytes: got %v, want local: reply", err)
	}
	stopped := make(chan struct{})
	began := time.Now()
	_, err := streamFromSocket(t, func(c net.Conn) {
		defer close(stopped)
		chunk := bytes.Repeat([]byte("a"), 64<<10)
		for {
			if _, err := c.Write(chunk); err != nil {
				return
			}
		}
	})
	if err == nil || err.Error() != "local: reply" {
		t.Fatalf("a reply that never ends: got %v, want local: reply", err)
	}
	<-stopped
	if d := time.Since(began); d > 20*time.Second {
		t.Fatalf("the client read a reply that never ends for %v", d)
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

// TestAQuestionsReplyFitsMaxReply asks about six text parts of 700 KiB, more than MaxReply holds
// in full: the answerer's questions call still reads, lists all six, shows the oldest in full while
// the reply fits, and shows the rest by their size alone, as a raw part is.
func TestAQuestionsReplyFitsMaxReply(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	const parts, size = 6, 700 << 10
	text := strings.Repeat("a", size)
	ids := make([]string, 0, parts)
	for i := range parts {
		q := w.give(t, "proposer", "applier", fmt.Sprintf("p-%d", i), "proposal", text)
		if q.Outcome == nil || q.Outcome.Verdict != "escalate" {
			t.Fatalf("part %d was not asked about: %v", i, q.Outcome)
		}
		ids = append(ids, q.Outcome.ReceiptID)
	}
	rep, err := w.client(t, "decider").Call(local.OpQuestions, nil)
	if err != nil {
		t.Fatalf("questions: %v", err)
	}
	if len(rep.Questions) != parts {
		t.Fatalf("got %d questions, want %d", len(rep.Questions), parts)
	}
	for i, q := range rep.Questions {
		full := q.Message != nil && *q.Message == text && q.Size == nil
		sized := q.Message == nil && q.Size != nil && *q.Size == size
		if q.ID != ids[i] || q.Form != "text" || full != (i < parts-1) || sized != (i == parts-1) {
			t.Fatalf("question %d: id %s, form %s, shown in full %v, by size %v", i, q.ID, q.Form, full, sized)
		}
	}
}

// TestRunReturnsAfterEveryReaderEnded connects to a window many times and sends nothing. When Run
// has returned, every connection is already closed: a read that does not wait finds its end, never
// a connection still open (goroutines-owned).
func TestRunReturnsAfterEveryReaderEnded(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	win := start(t, w.window(t))
	conns := make([]*net.UnixConn, 0, 50)
	for range cap(conns) {
		c, err := net.Dial("unix", w.socket)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		conns = append(conns, c.(*net.UnixConn))
	}
	// The window's accept is not synchronised with the dial: give it the connections first.
	time.Sleep(200 * time.Millisecond)
	win.stop(t)
	for i, c := range conns {
		raw, err := c.SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		n, rerr := -1, error(nil)
		if err := raw.Control(func(fd uintptr) {
			n, _, rerr = syscall.Recvfrom(int(fd), make([]byte, 1), syscall.MSG_PEEK|syscall.MSG_DONTWAIT)
		}); err != nil {
			t.Fatal(err)
		}
		// A connection the window closed reads its end, 0 bytes; one it still holds would block.
		if !(rerr == nil && n == 0) && !errors.Is(rerr, syscall.ECONNRESET) {
			t.Fatalf("connection %d after Run returned: read %d, %v, want its end", i, n, rerr)
		}
	}
}
