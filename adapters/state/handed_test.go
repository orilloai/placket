package state_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/internal/lines"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
)

// TestStateErrorTextIsTheCodeAlone opens a state directory under a path that holds a secret and
// cannot be made: the error's text is the package and the code, and the path is in its File only
// (error-text-no-content).
func TestStateErrorTextIsTheCodeAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "SEKRET")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "s")
	_, err := state.Open(path, actor, &bytes.Buffer{})
	e, ok := err.(*state.Error)
	if !ok || err.Error() != "state: state" || e.File != path {
		t.Fatalf("got %v, want state: state with the path in File", err)
	}
}

// TestStateErrorsQuoteNothing plants a secret in each file a state directory reads: a salt line, a
// kept line past the cap, an actor file and a seq file, each of the last two also past its cap. Each
// error's text is the package and the code, and the error, printed whole, holds nothing of the file
// but its name and the detail (error-text-no-content).
func TestStateErrorsQuoteNothing(t *testing.T) {
	t.Parallel()
	secret := "5e4b5e4b5e4b5e4bSEKRET"
	readActor := func(d *state.Dir) error { _, err := state.Read(d.Path()); return err }
	for _, c := range []struct {
		file, body string
		read       func(d *state.Dir) error
		detail     string
	}{
		{state.FileSalts, `{"receiptId":"r","salt":"` + secret + `","x":1}` + "\n", func(d *state.Dir) error { _, err := d.Salts(); return err }, ""},
		{state.FileSeq, secret + "\n", func(d *state.Dir) error { _, err := d.Seq(); return err }, ""},
		{state.FileSeq, strings.Repeat(secret, 200) + "\n", func(d *state.Dir) error { _, err := d.Seq(); return err }, state.DetailTooLarge},
		{state.FileActor, strings.Repeat(secret, 200) + "\n", readActor, state.DetailTooLarge},
		{state.FileKept, strings.Repeat(secret, 1<<18) + "\n", func(d *state.Dir) error { _, err := d.Kept(); return err }, ""},
	} {
		path := filepath.Join(t.TempDir(), "s")
		d := open(t, path, &bytes.Buffer{})
		if err := os.WriteFile(filepath.Join(path, c.file), []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		err := c.read(d)
		var se *state.Error
		if !errors.As(err, &se) || err.Error() != "state: state" || se.File != c.file || se.Detail != c.detail {
			t.Fatalf("%s: got %#v, want state: state, file %s, detail %q", c.file, err, c.file, c.detail)
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			if out := fmt.Sprintf(verb, err); strings.Contains(out, "SEKRET") {
				t.Fatalf("%s: %s printed %q", c.file, verb, out)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "s")
	open(t, path, &bytes.Buffer{})
	if err := os.WriteFile(filepath.Join(path, state.FileActor), []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Open(path, actor, &bytes.Buffer{}); err == nil || err.Error() != "state: state-actor" {
		t.Fatalf("another actor: got %v, want state: state-actor", err)
	}
}

// TestStateReadsOnlyRegularFiles reads a state directory whose kept.jsonl is a FIFO, and one whose
// actor file is: each is refused with code state, in which errors.Is finds lines.ErrNotRegular,
// without waiting on the FIFO. A kept.jsonl line longer than local.MaxReply, 4 MiB, is refused too,
// while one of MaxReply is read, so the cap follows the most a window replies with.
func TestStateReadsOnlyRegularFiles(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s")
	d := open(t, path, &bytes.Buffer{})
	if err := syscall.Mkfifo(filepath.Join(path, state.FileKept), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := unblock(filepath.Join(path, state.FileKept))
	_, err := d.Kept()
	stop()
	if err == nil || err.Error() != "state: state" || !errors.Is(err, lines.ErrNotRegular) {
		t.Fatalf("a FIFO kept.jsonl: got %v", err)
	}
	other := filepath.Join(t.TempDir(), "s")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(other, state.FileActor), 0o600); err != nil {
		t.Fatal(err)
	}
	stop = unblock(filepath.Join(other, state.FileActor))
	_, err = state.Read(other)
	stop()
	if err == nil || err.Error() != "state: state" || !errors.Is(err, lines.ErrNotRegular) {
		t.Fatalf("a FIFO actor file: got %v", err)
	}
	long := filepath.Join(t.TempDir(), "s")
	d = open(t, long, &bytes.Buffer{})
	under := bytes.Repeat([]byte("a"), local.MaxReply)
	if err := os.WriteFile(filepath.Join(long, state.FileKept), append(under, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if kept, err := d.Kept(); err != nil || len(kept) != 1 || len(kept[0]) != local.MaxReply || local.MaxReply != 4<<20 {
		t.Fatalf("a line of 4 MiB: got %d lines, %v", len(kept), err)
	}
	if err := os.WriteFile(filepath.Join(long, state.FileKept), append(under, 'a', '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Kept(); err == nil || err.Error() != "state: state" {
		t.Fatalf("a line of 4 MiB and a byte: got %v", err)
	}
}

// TestASaltPrintsAsFixedText prints a Salt under every verb, alone, by pointer and as a field: the
// salt never shows, as text or in hex (secrets-never-print).
func TestASaltPrintsAsFixedText(t *testing.T) {
	t.Parallel()
	secret := "5e4b5e4b5e4b5e4b5e4b5e4b5e4b5e4b"
	s := state.Salt{ReceiptID: "r", Salt: secret}
	for _, x := range []any{s, &s, struct{ Held state.Salt }{s}} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x", "%X", "%q", "%d"} {
			out := fmt.Sprintf(verb, x)
			if strings.Contains(out, secret) || strings.Contains(strings.ToLower(out), hex.EncodeToString([]byte(secret))) {
				t.Fatalf("%s printed %q", verb, out)
			}
		}
	}
	if s.String() != "state.Salt" || s.GoString() != "state.Salt" {
		t.Fatalf("String %q, GoString %q", s.String(), s.GoString())
	}
}

// unblock opens the FIFO at path for writing, and closes it, once a reader has opened it, so a reader
// that opens a FIFO still returns. The returned stop ends the wait for a reader that never comes.
func unblock(path string) (stop func()) {
	done, ended := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(ended)
		for {
			select {
			case <-done:
				return
			default:
			}
			if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				_ = w.Close()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	return func() { close(done); <-ended }
}

// TestStateReadsAnActorFileOfAtMost4KiB reads a state directory whose actor file is one line of
// 4 KiB, then one of 4 KiB and a byte: the first is read, the second refused with code state.
func TestStateReadsAnActorFileOfAtMost4KiB(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	line := append(bytes.Repeat([]byte("a"), 4<<10-1), '\n')
	if err := os.WriteFile(filepath.Join(path, state.FileActor), line, 0o600); err != nil {
		t.Fatal(err)
	}
	if d, err := state.Read(path); err != nil || len(d.Actor()) != 4<<10-1 {
		t.Fatalf("an actor file of 4 KiB: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, state.FileActor), append([]byte("a"), line...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Read(path); err == nil || err.Error() != "state: state" {
		t.Fatalf("an actor file of 4 KiB and a byte: got %v", err)
	}
}

// TestAnActorOrSeqFileOverItsCapIsTooLarge reads a state directory whose actor file, then whose seq
// file, is 4 KiB and a byte: each is code state, naming the file, with the detail too-large, and
// errors.Is finds no lines.ErrNotRegular in it, so it is not taken for a file of another kind. A seq
// file that is a directory is still not regular, with no detail.
func TestAnActorOrSeqFileOverItsCapIsTooLarge(t *testing.T) {
	t.Parallel()
	over := append(bytes.Repeat([]byte("1"), 4<<10), '\n')
	check := func(name string, err error, want state.Error, notRegular bool) {
		t.Helper()
		var se *state.Error
		if !errors.As(err, &se) || *se != want || errors.Is(err, lines.ErrNotRegular) != notRegular {
			t.Fatalf("%s: got %#v, want %#v", name, err, want)
		}
	}
	tooLarge := func(file string) state.Error {
		return state.Error{Code: state.CodeState, File: file, Detail: state.DetailTooLarge}
	}

	path := filepath.Join(t.TempDir(), "s")
	open(t, path, &bytes.Buffer{})
	if err := os.WriteFile(filepath.Join(path, state.FileActor), over, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := state.Read(path)
	check("an actor file read", err, tooLarge(state.FileActor), false)
	_, err = state.Open(path, actor, &bytes.Buffer{})
	check("an actor file opened", err, tooLarge(state.FileActor), false)

	path = filepath.Join(t.TempDir(), "s")
	d := open(t, path, &bytes.Buffer{})
	if err := os.WriteFile(filepath.Join(path, state.FileSeq), over, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = d.Seq()
	check("a seq file", err, tooLarge(state.FileSeq), false)
	_, err = d.Next()
	check("the next seq", err, tooLarge(state.FileSeq), false)

	if err := os.Remove(filepath.Join(path, state.FileSeq)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(path, state.FileSeq), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = d.Seq()
	check("a seq file that is a directory", err, state.Error{Code: state.CodeState, File: state.FileSeq}, true)
}
