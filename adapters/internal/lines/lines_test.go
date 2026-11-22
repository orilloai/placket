package lines_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"syscall"
	"testing"

	"github.com/orilloai/placket/adapters/internal/lines"
)

// TestReadSplitsWholeLinesUnderTheCap reads files of lines under a cap of 4 bytes: whole lines in
// order, an unterminated last line left out and marked, a line past the cap, ended or not, marked by its number
// with the lines before it, and an empty file with no lines.
func TestReadSplitsWholeLinesUnderTheCap(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in    string
		lines []string
		cut   bool
		long  int
	}{
		{"", nil, false, 0},
		{"a\n\nbcde\n", []string{"a", "", "bcde"}, false, 0},
		{"a\nbc", []string{"a"}, true, 0},
		{"a\nbcdef\ng\n", []string{"a"}, false, 2},
		{"abcdef", nil, false, 1},
		{"abcde", nil, false, 1},
		{"abcde\n", nil, false, 1},
		{"abcd", nil, true, 0},
	} {
		path := filepath.Join(t.TempDir(), "f")
		if err := os.WriteFile(path, []byte(c.in), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := lines.Read(path, 4)
		var got []string
		for _, l := range f.Lines {
			got = append(got, string(l))
		}
		if err != nil || !reflect.DeepEqual(got, c.lines) || f.Cut != c.cut || f.Long != c.long {
			t.Fatalf("%q: got %q, cut %v, long %d, %v", c.in, got, f.Cut, f.Long, err)
		}
	}
}

// TestReadTakesOnlyARegularFile refuses a FIFO and a directory with ErrNotRegular, never opening
// the FIFO, follows a link to a regular file, and gives fs.ErrNotExist for a missing path.
func TestReadTakesOnlyARegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{fifo, dir} {
		if _, err := lines.Read(p, 4); !errors.Is(err, lines.ErrNotRegular) {
			t.Fatalf("%s: got %v", p, err)
		}
		if _, _, err := lines.ReadAll(p, 4); !errors.Is(err, lines.ErrNotRegular) {
			t.Fatalf("%s: got %v", p, err)
		}
	}
	if _, err := lines.Read(filepath.Join(dir, "none"), 4); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file: got %v", err)
	}
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("ab\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if f, err := lines.Read(link, 4); err != nil || len(f.Lines) != 1 || string(f.Lines[0]) != "ab" {
		t.Fatalf("a link: got %q, %v", f.Lines, err)
	}
	if b, ok, err := lines.ReadAll(file, 3); err != nil || !ok || string(b) != "ab\n" {
		t.Fatalf("ReadAll of 3 bytes: got %q, %v, %v", b, ok, err)
	}
	if _, ok, err := lines.ReadAll(file, 2); err != nil || ok {
		t.Fatalf("ReadAll past the cap: got %v, %v", ok, err)
	}
}

// TestPrivateIsOnlyTheOwnersRegularFile checks the rule a key file and an opening are read under: a
// regular file of mode 0600 or tighter is private to the user that owns it; one that others may read
// or write, one another user owns (the uid swapped), and a directory are not.
func TestPrivateIsOnlyTheOwnersRegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	uid := os.Getuid()
	for _, c := range []struct {
		mode os.FileMode
		uid  int
		want bool
	}{
		{0o600, uid, true},
		{0o400, uid, true},
		{0o604, uid, false},
		{0o620, uid, false},
		{0o640, uid, false},
		{0o601, uid, false},
		{0o600, uid + 1, false},
	} {
		path := filepath.Join(dir, c.mode.String()+"-"+strconv.Itoa(c.uid))
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, c.mode); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := lines.Private(fi, c.uid); got != c.want {
			t.Errorf("mode %v, uid %d: Private = %v, want %v", c.mode, c.uid-uid, got, c.want)
		}
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || lines.Private(fi, uid) {
		t.Errorf("a directory of mode 0700: Private = true, %v", err)
	}
}
