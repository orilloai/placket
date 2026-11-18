package local_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/orilloai/placket/adapters/local"
)

// TestASocketPathTooLong gives a window and a client a socket path one byte longer than the system
// takes, its sun_path less the closing zero byte: each is tool error socket-path, the window before
// it opens its journal, the client before it looks at the path. A client given a path of exactly
// the limit, where a live window's socket is linked, tries it and is error socket.
func TestASocketPathTooLong(t *testing.T) {
	t.Parallel()
	limit := len(syscall.RawSockaddrUnix{}.Path) - 1
	w := newWorld(t)
	dir := filepath.Dir(w.socket)
	long := filepath.Join(dir, strings.Repeat("s", limit-len(dir)))
	if len(long) != limit+1 {
		t.Fatalf("the long path is %d bytes", len(long))
	}
	win := w.window(t)
	win.Socket = long
	_, err := runOnce(t, win)
	var le *local.Error
	if !errors.As(err, &le) || string(le.Code) != "socket-path" || err.Error() != "local: socket-path" {
		t.Fatalf("a window on a path too long: got %v", err)
	}
	if _, err := os.Lstat(w.journal); !os.IsNotExist(err) {
		t.Fatalf("the window opened its journal: %v", err)
	}
	if err := os.Symlink(w.socket, long); err != nil {
		t.Fatal(err)
	}
	c := w.client(t, "applier")
	c.Socket = long
	if _, err := c.Call(local.OpGet, nil); !errors.As(err, &le) || string(le.Code) != "socket-path" {
		t.Fatalf("a client on a path too long: got %v", err)
	}
	c.Socket = long[:limit]
	if _, err := c.Call(local.OpGet, nil); !errors.As(err, &le) || le.Code != local.CodeSocket {
		t.Fatalf("a client at the limit: got %v", err)
	}
}

// TestTheDefaultSocketFitsMacOS sets a TMPDIR of 64 bytes and no XDG_RUNTIME_DIR: the default socket
// of an Accord is at most 103 bytes, macOS's limit, whatever system the test runs on; it is
// <TMPDIR>/placket-<uid>/<12 hex of the id>.sock; and a window listens on it, in a directory of mode
// 0700 made for it. With XDG_RUNTIME_DIR set it is under <XDG_RUNTIME_DIR>/placket. The default
// journal is under XDG_STATE_HOME, or HOME, named by the whole id.
func TestTheDefaultSocketFitsMacOS(t *testing.T) {
	base := shortDir(t)
	tmp := base + "/" + strings.Repeat("t", 64-len(base)-1)
	if len(tmp) != 64 {
		t.Fatalf("TMPDIR is %d bytes", len(tmp))
	}
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	t.Setenv("XDG_RUNTIME_DIR", "")
	w := newWorld(t)
	hex := strings.TrimPrefix(w.accord, "sha256:")
	sock := local.DefaultSocket(w.accord)
	if want := filepath.Join(tmp, "placket-"+strconv.Itoa(os.Getuid()), hex[:12]+".sock"); sock != want || len(sock) > 103 {
		t.Fatalf("the default socket is %q, %d bytes; want %q", sock, len(sock), want)
	}
	if err := local.MakeSocketDir(sock); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Dir(sock)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("the socket's directory: %v, %v", fi.Mode(), err)
	}
	w.socket = sock
	start(t, w.window(t))
	if rep := w.call(t, "applier", local.OpGet, nil); rep.Door != "" || rep.Delivery != nil {
		t.Fatalf("a get on the default socket: %+v", rep)
	}
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := local.DefaultSocket(w.accord); got != "/run/user/1000/placket/"+hex[:12]+".sock" {
		t.Fatalf("under XDG_RUNTIME_DIR: %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "/state")
	if got := local.DefaultJournal(w.accord); got != "/state/placket/"+hex {
		t.Fatalf("under XDG_STATE_HOME: %q", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	if got := local.DefaultJournal(w.accord); got != "/home/u/.local/state/placket/"+hex {
		t.Fatalf("under HOME: %q", got)
	}
}

// TestASocketDirOfAnotherIsRefused makes the default socket's directory open to others, then a link
// in its place: each is error socket.
func TestASocketDirOfAnotherIsRefused(t *testing.T) {
	t.Parallel()
	d := shortDir(t)
	open := filepath.Join(d, "open")
	if err := os.Mkdir(open, 0o777); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(open, 0o777)
	if err := os.Symlink(open, filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"open", "link"} {
		var le *local.Error
		if err := local.MakeSocketDir(filepath.Join(d, dir, "s.sock")); !errors.As(err, &le) || le.Code != local.CodeSocket {
			t.Fatalf("%s: %v", dir, err)
		}
	}
}
