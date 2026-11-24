package journal

import (
	"bytes"
	"path/filepath"
	"syscall"
	"testing"
)

// TestCloseLetsTheLockGoWhileItsFileIsShared closes a journal while a second descriptor still shares
// its lock's file, as a child process does between fork and exec, and opens the journal again. The
// lock belongs to the file's open description, so closing one descriptor alone would keep it held,
// and the window that stopped would refuse its own restart, journal-locked.
func TestCloseLetsTheLockGoWhileItsFileIsShared(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "j")
	a, err := OpenWith(dir, &bytes.Buffer{}, OS)
	if err != nil {
		t.Fatal(err)
	}
	dup, err := syscall.Dup(int(a.lock.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Close(dup) })
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	b, err := OpenWith(dir, &bytes.Buffer{}, OS)
	if err != nil {
		t.Fatalf("reopen: got %v, want nil", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close again: %v", err)
	}
}
