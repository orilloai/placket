package lines

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestOpenRefusesAFileSwappedAfterItsCheck puts another regular file at the path between Open's check
// and its open: Open refuses it with ErrNotRegular, since what it opened is not the file it checked.
// It swaps openFile, so it does not run in parallel.
func TestOpenRefusesAFileSwappedAfterItsCheck(t *testing.T) {
	dir := t.TempDir()
	path, other := filepath.Join(dir, "f"), filepath.Join(dir, "other")
	for _, p := range []string{path, other} {
		if err := os.WriteFile(p, []byte("ab\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if f, err := Open(path); err != nil {
		t.Fatalf("no swap: %v", err)
	} else {
		_ = f.Close()
	}
	real := openFile
	defer func() { openFile = real }()
	openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		if err := os.Rename(other, name); err != nil {
			t.Fatal(err)
		}
		return real(name, flag, perm)
	}
	f, err := Open(path)
	if err == nil {
		_ = f.Close()
	}
	if !errors.Is(err, ErrNotRegular) {
		t.Fatalf("a swapped file: got %v, want ErrNotRegular", err)
	}
}
