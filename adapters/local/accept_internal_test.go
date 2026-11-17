package local

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// timeoutError is a net.Error that timed out.
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestAcceptRetriesOnlyWhatMayPass tries an accept again only when the process or the system is out
// of file descriptors, or the accept timed out; a closed listener or any other failure ends the
// window. The pause doubles from 10 ms to 1 s.
func TestAcceptRetriesOnlyWhatMayPass(t *testing.T) {
	t.Parallel()
	op := func(err error) error {
		return &net.OpError{Op: "accept", Net: "unix", Err: os.NewSyscallError("accept4", err)}
	}
	for _, c := range []struct {
		err  error
		want bool
	}{
		{op(syscall.EMFILE), true},
		{op(syscall.ENFILE), true},
		{&net.OpError{Op: "accept", Net: "unix", Err: timeoutError{}}, true},
		{op(syscall.ECONNRESET), false},
		{op(syscall.EINVAL), false},
		{net.ErrClosed, false},
	} {
		if got := temporary(c.err); got != c.want {
			t.Fatalf("%v: got %v, want %v", c.err, got, c.want)
		}
	}
	if minAcceptPause.Milliseconds() != 10 || maxAcceptPause.Milliseconds() != 1000 {
		t.Fatalf("pauses %v to %v", minAcceptPause, maxAcceptPause)
	}
}

// TestAReplyThatCannotBeEncodedIsTheDoors reads the reply the door sends when it cannot encode one:
// a refusal under the door member, with door-internal.
func TestAReplyThatCannotBeEncodedIsTheDoors(t *testing.T) {
	t.Parallel()
	if internalReply != `{"door":"door-internal","ok":false}` {
		t.Fatalf("got %s", internalReply)
	}
	r, ok := ParseReply([]byte(internalReply))
	if !ok || r.Door != Internal || r.Error != "" {
		t.Fatalf("read as %v, door %q, error %q", ok, r.Door, r.Error)
	}
}

// TestTheSocketIsPrivateBeforeItIsReachable looks at the socket after it is bound and before it is
// at its path: mode 0600, in a directory of mode 0700, with nothing yet at the path. Once listen
// returns, the socket at the path answers, mode 0600, and the directory is gone.
func TestTheSocketIsPrivateBeforeItIsReachable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "w.sock")
	var seen string
	l, err := listen(path, func(bound string) {
		seen = bound
		fi, err := os.Lstat(bound)
		if err != nil || fi.Mode()&fs.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
			t.Errorf("the bound socket: %v, %v", fi, err)
		}
		di, err := os.Lstat(filepath.Dir(bound))
		if err != nil || !di.IsDir() || di.Mode().Perm() != 0o700 || filepath.Dir(filepath.Dir(bound)) != dir {
			t.Errorf("the directory it is bound in: %v, %v", di, err)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("the path before the link: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if seen == "" {
		t.Fatal("the socket was never bound apart from its path")
	}
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&fs.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the socket at its path: %v, %v", fi, err)
	}
	if _, err := os.Lstat(filepath.Dir(seen)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the directory it was bound in is left: %v", err)
	}
	go func() {
		if c, err := l.Accept(); err == nil {
			_ = c.Close()
		}
	}()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial the path: %v", err)
	}
	_ = c.Close()
}

// TestAQuestionsReplyLeavesOutWhatItsSizeCannotHold builds the questions reply for five questions
// of about 1 MiB each, even shown by size: the reply holds the oldest three, which fit within
// MaxReply, in order, and leaves out the two after them; the door reports nothing of its own.
func TestAQuestionsReplyLeavesOutWhatItsSizeCannotHold(t *testing.T) {
	t.Parallel()
	str := func(s string) *string { return &s }
	list := make([]wireAsked, 5)
	for i := range list {
		size := 10
		list[i] = wireAsked{Asker: str("a"), Deadline: str("2099-01-01T00:00:00Z"), Form: str("text"),
			ID: str(strings.Repeat(string(rune('a'+i)), 1<<20)), Kind: str("proposal"), Message: str("0123456789"), Size: &size}
	}
	var ev Event
	b := questionsReply(list, &ev)
	if len(b) > MaxReply || ev.Door != "" {
		t.Fatalf("a reply of %d bytes, door %q", len(b), ev.Door)
	}
	var r wireReply
	if err := json.Unmarshal(b, &r); err != nil || r.Questions == nil || len(*r.Questions) != 3 {
		t.Fatalf("read as %v, with questions %v", err, r.Questions != nil)
	}
	for i, q := range *r.Questions {
		if q.ID == nil || *q.ID != *list[i].ID {
			t.Fatalf("question %d is another", i)
		}
	}
}

// pathOfLength returns a path of exactly n bytes whose last element is base, in a new directory
// under /tmp, or the test's temporary directory where /tmp cannot be used, removed after the test.
func pathOfLength(t *testing.T, n int, base string) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "pl")
	if err != nil {
		root = t.TempDir()
	} else {
		t.Cleanup(func() { _ = os.RemoveAll(root) })
	}
	pad := n - len(root) - len("/") - len("/") - len(base)
	if pad < 1 {
		t.Skipf("the temporary directory %s leaves no room for a path of %d bytes", root, n)
	}
	dir := filepath.Join(root, strings.Repeat("d", pad))
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, base)
	if len(path) != n {
		t.Fatalf("a path of %d bytes, not %d", len(path), n)
	}
	return path
}

// TestASocketPathAtTheLimitIsBound listens at paths of exactly MaxSocketPath bytes, which CheckSocket
// takes, with last elements of 4 to 40 bytes: each is bound, at a path no longer than its own, and
// answers at its path.
func TestASocketPathAtTheLimitIsBound(t *testing.T) {
	t.Parallel()
	for _, base := range []string{"w.so", "w.sock", "placket.sock", strings.Repeat("s", 29), strings.Repeat("s", 40)} {
		t.Run(base, func(t *testing.T) {
			t.Parallel()
			path := pathOfLength(t, MaxSocketPath, base)
			if err := CheckSocket(path); err != nil {
				t.Fatalf("CheckSocket: %v", err)
			}
			var seen string
			l, err := listen(path, func(bound string) { seen = bound })
			if err != nil {
				t.Fatalf("listen at %d bytes: %v", len(path), err)
			}
			defer l.Close()
			if want := min(len(path), len(path)-len(base)+29); len(seen) != want {
				t.Fatalf("bound at %d bytes, not %d", len(seen), want)
			}
			go func() {
				if c, err := l.Accept(); err == nil {
					_ = c.Close()
				}
			}()
			c, err := net.Dial("unix", path)
			if err != nil {
				t.Fatalf("dial the path: %v", err)
			}
			_ = c.Close()
		})
	}
}

// TestABoundPathOverTheLimitIsSocketPath listens at a path of MaxSocketPath bytes whose last element
// has 2 bytes, so the path it would be bound at is a byte over the limit: listen is CodeSocketPath,
// never CodeSocket, and leaves nothing beside the path.
func TestABoundPathOverTheLimitIsSocketPath(t *testing.T) {
	t.Parallel()
	path := pathOfLength(t, MaxSocketPath, "ws")
	_, err := listen(path, func(string) { t.Error("bound") })
	var le *Error
	if !errors.As(err, &le) || le.Code != CodeSocketPath {
		t.Fatalf("listen: %v", err)
	}
	if left, err := os.ReadDir(filepath.Dir(path)); err != nil || len(left) != 0 {
		t.Fatalf("left beside the path: %v, %v", left, err)
	}
}
