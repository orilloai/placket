package keyfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/keyfile"
)

// written writes a key file of mode perm in a new directory and returns its path.
func written(t *testing.T, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "k.key")
	if err := os.WriteFile(path, []byte(strings.Repeat("0f", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReadRefusesAKeyOthersCanReach refuses, with key-mode, a key file whose mode sets any bit of
// 0077, and loads one of mode 0600 or 0400.
func TestReadRefusesAKeyOthersCanReach(t *testing.T) {
	t.Parallel()
	for _, perm := range []os.FileMode{0o644, 0o640, 0o604, 0o602, 0o620, 0o601, 0o610, 0o666, 0o707, 0o777} {
		_, err := keyfile.Read(written(t, perm))
		if code(err) != keyfile.CodeMode || err.Error() != "keyfile: key-mode" {
			t.Fatalf("mode %#o: got %v, want key-mode", perm, err)
		}
	}
	for _, perm := range []os.FileMode{0o600, 0o400} {
		if _, err := keyfile.Read(written(t, perm)); err != nil {
			t.Fatalf("mode %#o: got %v, want the key", perm, err)
		}
	}
}

// TestReadFollowsALinkAndChecksItsTarget loads a key through a symbolic link to a file of mode 0600,
// and refuses one through a link to a file of mode 0644 with key-mode, whatever the link's own mode.
func TestReadFollowsALinkAndChecksItsTarget(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		perm os.FileMode
		want keyfile.Code
	}{{0o600, ""}, {0o644, keyfile.CodeMode}} {
		link := filepath.Join(t.TempDir(), "link.key")
		if err := os.Symlink(written(t, c.perm), link); err != nil {
			t.Fatal(err)
		}
		if _, err := keyfile.Read(link); code(err) != c.want {
			t.Fatalf("a link to mode %#o: got %v, want %q", c.perm, err, c.want)
		}
	}
}

// TestReadRefusesWhatIsNotAFile refuses a directory and a FIFO with key-read, and never waits on
// the FIFO for a writer; a file past 4 KiB is refused as code key.
func TestReadRefusesWhatIsNotAFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := keyfile.Read(dir); code(err) != keyfile.CodeRead {
		t.Fatalf("a directory: got %v, want key-read", err)
	}
	fifo := filepath.Join(t.TempDir(), "fifo.key")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	// A writer that opens the FIFO once a reader has, so a Read that opens it still returns.
	done, ended := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(ended)
		for {
			select {
			case <-done:
				return
			default:
			}
			if w, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				_ = w.Close()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	_, err := keyfile.Read(fifo)
	close(done)
	<-ended
	if code(err) != keyfile.CodeRead {
		t.Fatalf("a FIFO: got %v, want key-read", err)
	}
	big := filepath.Join(t.TempDir(), "big.key")
	if err := os.WriteFile(big, bytes.Repeat([]byte("0f"), 1<<12), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := keyfile.Read(big); code(err) != keyfile.CodeKey {
		t.Fatalf("a file of 8 KiB: got %v, want key", err)
	}
}

// TestNewLeavesOnlyTheKey leaves the key file alone in its directory after a write, after a write
// refused as key-exists, and after one refused as key-write, so no temporary file survives.
func TestNewLeavesOnlyTheKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "k.key")
	if _, err := keyfile.New(path, bytes.NewReader(bytes.Repeat([]byte{7}, 32))); err != nil {
		t.Fatal(err)
	}
	if _, err := keyfile.New(path, bytes.NewReader(bytes.Repeat([]byte{8}, 32))); code(err) != keyfile.CodeExists {
		t.Fatalf("a second write: got %v, want key-exists", err)
	}
	if _, err := keyfile.New(filepath.Join(dir, "short.key"), bytes.NewReader([]byte{1})); code(err) != keyfile.CodeWrite {
		t.Fatalf("too few random bytes: got %v, want key-write", err)
	}
	if _, err := keyfile.New(filepath.Join(dir, "none", "k.key"), bytes.NewReader(bytes.Repeat([]byte{7}, 32))); code(err) != keyfile.CodeWrite {
		t.Fatalf("no directory: got %v, want key-write", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "k.key" {
		t.Fatalf("the directory holds %q, want only k.key", names)
	}
	if _, err := keyfile.Read(path); err != nil {
		t.Fatalf("the key written: %v", err)
	}
}

// TestNewNeverReplacesALink refuses, with key-exists, a path that is a symbolic link to nowhere, and
// creates nothing where it points.
func TestNewNeverReplacesALink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "k.key")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := keyfile.New(path, bytes.NewReader(bytes.Repeat([]byte{7}, 32))); code(err) != keyfile.CodeExists {
		t.Fatalf("got %v, want key-exists", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("the link's target was made: %v", err)
	}
}
