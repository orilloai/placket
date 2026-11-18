package local_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/receipts"
)

// TestTheSocketIsTheWindowsOwn starts a window and looks at its socket: a socket, mode 0600. A second
// window, of another journal, given the socket a live window answers on, is refused with tool error
// socket, and the first window still answers there. A path that holds a file that is no socket is
// refused too, and the file is left as it was.
func TestTheSocketIsTheWindowsOwn(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	start(t, w.window(t))
	fi, err := os.Lstat(w.socket)
	if err != nil || fi.Mode()&fs.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the socket: %v, %v", fi.Mode(), err)
	}

	other := w.window(t)
	other.Journal += "2"
	_, err = runOnce(t, other)
	var le *local.Error
	if !errors.As(err, &le) || le.Code != local.CodeSocket {
		t.Fatalf("a second window on a live socket: got %v", err)
	}
	rep, err := w.client(t, "applier").Call(local.OpGet, nil)
	if err != nil || rep.Door != "" || rep.Rejection != "" || rep.Error != "" {
		t.Fatalf("the first window after the second was refused: %v", err)
	}

	v := newWorld(t)
	if err := os.WriteFile(v.socket, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runOnce(t, v.window(t))
	if !errors.As(err, &le) || le.Code != local.CodeSocket {
		t.Fatalf("a window on a path that is no socket: got %v", err)
	}
	if b, _ := os.ReadFile(v.socket); string(b) != "mine" {
		t.Fatalf("the file at the path holds %q", b)
	}
}

// TestTheClientCallsOnlyASocket calls a window through a link to its socket, and through a path
// that holds a plain file: each is tool error socket, and the call is never sent. A window that
// answers with an outcome whose receipt names another Accord is tool error reply. Through the socket
// itself the call is answered.
func TestTheClientCallsOnlyASocket(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	win := start(t, w.window(t))
	link := w.socket + "l"
	if err := os.Symlink(w.socket, link); err != nil {
		t.Fatal(err)
	}
	plain := w.socket + "f"
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c := w.client(t, "applier")
	for _, p := range []string{link, plain} {
		c.Socket = p
		var le *local.Error
		if _, err := c.Call(local.OpGet, nil); !errors.As(err, &le) || le.Code != local.CodeSocket {
			t.Fatalf("%s: got %v", filepath.Base(p), err)
		}
	}
	win.mu.Lock()
	calls := 0
	for _, e := range *win.events {
		if e.Kind != local.EventStarted {
			calls++
		}
	}
	win.mu.Unlock()
	if calls != 0 {
		t.Fatalf("the window saw %d calls", calls)
	}

	// An outcome whose receipt names another Accord, from a window that is not this Accord's.
	rep := w.give(t, "proposer", "applier", "n-1", "note", "A note.")
	if rep.Outcome == nil {
		t.Fatal("the note was not given")
	}
	st, err := receipts.Decode(rep.Outcome.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	p := st.Predicate
	p.Accord = "sha256:" + strings.Repeat("0", 64)
	b, v, err := receipts.Build(st.Subject, p, key(t, "mediator"))
	if err != nil {
		t.Fatal(err)
	}
	id := v.Statement().ID
	fake := filepath.Join(shortDir(t), "f")
	l, err := net.Listen("unix", fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reply := replyOf(t, map[string]any{"ok": true, "outcome": map[string]any{"verdict": "pass", "reason": "ok", "receiptId": id, "receipt": json.RawMessage(b)}})
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = bufio.NewReader(conn).ReadBytes('\n')
			_, _ = conn.Write(append(reply, '\n'))
			_ = conn.Close()
		}
	}()
	c.Socket = fake
	var le *local.Error
	if _, err := c.Call(local.OpGet, nil); !errors.As(err, &le) || le.Code != local.CodeReply {
		t.Fatalf("an outcome of another Accord: got %v", err)
	}
	c.Socket = w.socket
	if rep, err := c.Call(local.OpGet, nil); err != nil || rep.Delivery == nil {
		t.Fatalf("through the socket: %v", err)
	}
}

// TestAMediatorKeyIsShownNoQuestion runs a window whose own key is also a member's, the applier's,
// who answers nothing: the questions call it signs, with a question open, is shown none, since the
// door never hands the mediator's own reference to the read for a call. The decider, who answers
// the question, is shown it.
func TestAMediatorKeyIsShownNoQuestion(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	w.use(askAndLog(t, "applier"))
	cfg := w.window(t)
	cfg.Key = key(t, "applier")
	start(t, cfg)
	q := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.")
	if q.Outcome == nil || q.Outcome.Verdict != "escalate" {
		t.Fatal("the proposal was not asked about")
	}
	if r := w.call(t, "applier", local.OpQuestions, nil); !r.IsQuestions || len(r.Questions) != 0 || r.Rejection != "" {
		t.Fatalf("the mediator's key: %d questions, %q", len(r.Questions), r.Rejection)
	}
	if r := w.call(t, "decider", local.OpQuestions, nil); len(r.Questions) != 1 || r.Questions[0].ID != q.Outcome.ReceiptID {
		t.Fatalf("the decider: %d questions", len(r.Questions))
	}
}
