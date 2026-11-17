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
// kept line past the cap, an actor file and a seq file. Each error's text is the package and the
// code (error-text-no-content).
func TestStateErrorsQuoteNothing(t *testing.T) {
	t.Parallel()
	secret := "5e4b5e4b5e4b5e4bSEKRET"
	for _, c := range []struct {
		file, body string
		read       func(d *state.Dir) error
	}{
		{state.FileSalts, `{"receiptId":"r","salt":"` + secret + `","x":1}` + "\n", func(d *state.Dir) error { _, err := d.Salts(); return err }},
		{state.FileSeq, secret + "\n", func(d *state.Dir) error { _, err := d.Seq(); return err }},
		{state.FileKept, strings.Repeat(secret, 1<<18) + "\n", func(d *state.Dir) error { _, err := d.Kept(); return err }},
	} {
		path := filepath.Join(t.TempDir(), "s")
		d := open(t, path, &bytes.Buffer{})
		if err := os.WriteFile(filepath.Join(path, c.file), []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := c.read(d); err == nil || err.Error() != "state: state" {
			t.Fatalf("%s: got %v, want state: state", c.file, err)
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
