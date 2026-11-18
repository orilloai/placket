package local

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/orilloai/placket/core/canonical"
)

// SocketHex is how many hex digits of an Accord id name its default socket: 48 bits, so the path
// fits the shortest sun_path a target system has, 103 bytes on macOS, under a long TMPDIR.
const SocketHex = 12

// DefaultSocket is the socket a window of the Accord listens on when it is given none, read from the
// process's environment: DefaultSocketWith(os.Getenv, accord).
func DefaultSocket(accord string) string { return DefaultSocketWith(os.Getenv, accord) }

// DefaultSocketWith is the default socket of the Accord under the environment getenv reads:
// <runtime>/<the first SocketHex hex digits of the Accord id>.sock, where <runtime> is
// $XDG_RUNTIME_DIR/placket, or else $TMPDIR/placket-<uid>, with /tmp for an empty TMPDIR. Two
// Accords whose ids share those digits are told apart by the labels config, which refuses two
// entries on one socket.
func DefaultSocketWith(getenv func(string) string, accord string) string {
	h := strings.TrimPrefix(accord, canonical.HashPrefix)
	if len(h) > SocketHex {
		h = h[:SocketHex]
	}
	return filepath.Join(runtimeDir(getenv), h+".sock")
}

// runtimeDir is the directory of the default sockets.
func runtimeDir(getenv func(string) string) string {
	if d := getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "placket")
	}
	tmp := getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	return filepath.Join(tmp, "placket-"+strconv.Itoa(os.Getuid()))
}

// DefaultJournal is the journal directory of a window of the Accord given none, read from the
// process's environment: DefaultJournalWith(os.Getenv, accord).
func DefaultJournal(accord string) string { return DefaultJournalWith(os.Getenv, accord) }

// DefaultJournalWith is the default journal of the Accord under the environment getenv reads:
// $XDG_STATE_HOME/placket/<the Accord id's hex>/, or else ~/.local/state/placket/<hex>/, with ~ the
// HOME it reads. The whole id names it, since a journal's path has no length limit. A value that is
// not an absolute path counts as unset, and with neither it is the empty string: there is no default
// journal, rather than one under whatever directory the window was started in.
func DefaultJournalWith(getenv func(string) string, accord string) string {
	h := strings.TrimPrefix(accord, canonical.HashPrefix)
	if d := getenv("XDG_STATE_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "placket", h)
	}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		return filepath.Join(home, ".local", "state", "placket", h)
	}
	return ""
}

// MakeSocketDir makes the directory a default socket is in, mode 0700, and checks it is this user's
// own: a directory, not a link, owned by the user the process runs as and open to nobody else. Any
// other is CodeSocket, since a socket bound there could be reached, or replaced, by another user.
func MakeSocketDir(socket string) error {
	dir := filepath.Dir(socket)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return &Error{Code: CodeSocket}
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return &Error{Code: CodeSocket}
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || fi.Mode().Perm()&0o077 != 0 {
		return &Error{Code: CodeSocket}
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); !ok || int64(st.Uid) != int64(os.Geteuid()) {
		return &Error{Code: CodeSocket}
	}
	return nil
}
