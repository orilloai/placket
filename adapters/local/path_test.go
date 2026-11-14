package local_test

import (
	"errors"
	"os"
	"path/filepath"
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
