package state_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/receipts"
)

const actor = "ed25519:" + "1111111111111111111111111111111111111111111111111111111111111111"

func open(t *testing.T, path string, stderr *bytes.Buffer) *state.Dir {
	t.Helper()
	d, err := state.Open(path, actor, stderr)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func hash(c string) string { return "sha256:" + strings.Repeat(c, 64) }

// TestSeqAndEnvelopes records seq numbers and envelopes: seq is 0 until set, an envelope dropped
// after a rejection is gone and the others stay, in order, named for the directory's actor; and
// the directory and its files are its owner's alone.
func TestSeqAndEnvelopes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s")
	var stderr bytes.Buffer
	d := open(t, path, &stderr)
	if n, err := d.Seq(); n != 0 || err != nil {
		t.Fatalf("a new directory's seq: %d, %v", n, err)
	}
	for i, h := range []string{hash("a"), hash("b"), hash("c")} {
		if err := d.AddEnvelope(int64(i+1), h); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.DropEnvelope(2, hash("b")); err != nil {
		t.Fatal(err)
	}
	if err := d.SetSeq(3); err != nil {
		t.Fatal(err)
	}
	got, err := d.Envelopes()
	want := []receipts.Envelope{{Sender: actor, Seq: 1, Hash: hash("a")}, {Sender: actor, Seq: 3, Hash: hash("c")}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if n, _ := d.Seq(); n != 3 {
		t.Fatalf("seq %d", n)
	}
	b, _ := os.ReadFile(filepath.Join(path, state.FileEnvelopes))
	if string(b) != `{"hash":"`+hash("a")+`","seq":1}`+"\n"+`{"hash":"`+hash("c")+`","seq":3}`+"\n" {
		t.Fatalf("envelopes.jsonl holds %q", b)
	}
	for _, name := range []string{"", state.FileActor, state.FileSeq, state.FileEnvelopes} {
		fi, err := os.Stat(filepath.Join(path, name))
		want := os.FileMode(0o600)
		if name == "" {
			want = 0o700
		}
		if err != nil || fi.Mode().Perm() != want {
			t.Fatalf("%q: %v, %v", name, fi.Mode().Perm(), err)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr: %q", stderr.String())
	}
}

// TestOpenRefusesAnotherKey opens a directory first used by another actor: state-actor.
func TestOpenRefusesAnotherKey(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s")
	open(t, path, &bytes.Buffer{})
	_, err := state.Open(path, "ed25519:"+strings.Repeat("2", 64), &bytes.Buffer{})
	var e *state.Error
	if !errors.As(err, &e) || e.Code != state.CodeActor {
		t.Fatalf("got %v", err)
	}
	d, err := state.Read(path)
	if err != nil || d.Actor() != actor {
		t.Fatalf("read: %v", err)
	}
}

// TestLockRemovesAnUnterminatedLine is the repair of each .jsonl file: a last line with no line feed
// is left out of every read, and Lock removes it, with one line on stderr naming the file and the
// line, so that the next line starts on a line of its own.
func TestLockRemovesAnUnterminatedLine(t *testing.T) {
	t.Parallel()
	for _, name := range []string{state.FileEnvelopes, state.FileDelivered, state.FileKept} {
		path := filepath.Join(t.TempDir(), "s")
		var stderr bytes.Buffer
		d := open(t, path, &stderr)
		whole := `{"hash":"` + hash("a") + `","seq":1}` + "\n"
		if err := os.WriteFile(filepath.Join(path, name), []byte(whole+`{"hash":"sha`), 0o600); err != nil {
			t.Fatal(err)
		}
		read := map[string]func() (int, error){
			state.FileEnvelopes: func() (int, error) { l, err := d.Envelopes(); return len(l), err },
			state.FileDelivered: func() (int, error) { l, err := d.Delivered(); return len(l), err },
			state.FileKept:      func() (int, error) { l, err := d.Kept(); return len(l), err },
		}[name]
		if n, err := read(); n != 1 || err != nil {
			t.Fatalf("%s: read %d lines, %v", name, n, err)
		}
		if err := d.Lock(); err != nil {
			t.Fatal(err)
		}
		if err := d.Keep([]byte(`{"a":1}`)); err != nil {
			t.Fatal(err)
		}
		d.Unlock()
		b, _ := os.ReadFile(filepath.Join(path, name))
		want := whole
		if name == state.FileKept {
			want += `{"a":1}` + "\n"
		}
		if string(b) != want {
			t.Fatalf("%s holds %q", name, b)
		}
		if stderr.String() != "state: "+name+" line 2 had no line feed and was never written; removed\n" {
			t.Fatalf("stderr: %q", stderr.String())
		}
	}
}

// TestLockIsExclusive takes a directory's lock through one Dir and finds that a second waits until
// the first lets it go (state-lock).
func TestLockIsExclusive(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s")
	a, b := open(t, path, &bytes.Buffer{}), open(t, path, &bytes.Buffer{})
	if err := a.Lock(); err != nil {
		t.Fatal(err)
	}
	got := make(chan time.Time, 1)
	go func() {
		if err := b.Lock(); err == nil {
			got <- time.Now()
			b.Unlock()
		}
	}()
	time.Sleep(100 * time.Millisecond)
	released := time.Now()
	a.Unlock()
	select {
	case at := <-got:
		if at.Before(released) {
			t.Fatal("the second lock was taken while the first was held")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second lock was never taken")
	}
}

// FuzzParseEnvelope reads any bytes as an envelopes.jsonl line. It never panics, and what it
// accepts writes back as the same bytes.
func FuzzParseEnvelope(f *testing.F) {
	f.Add([]byte(`{"hash":"` + hash("a") + `","seq":1}`))
	f.Add([]byte(`{"seq":1,"hash":"x"}`))
	f.Add([]byte(`{"hash":"` + hash("a") + `","seq":0}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		seq, h, ok := state.ParseEnvelope(b)
		if !ok {
			return
		}
		d := open(t, filepath.Join(t.TempDir(), "s"), &bytes.Buffer{})
		if err := d.AddEnvelope(seq, h); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(d.Path(), state.FileEnvelopes))
		if !bytes.Equal(got, append(append([]byte(nil), b...), '\n')) {
			t.Fatalf("%q wrote back as %q", b, got)
		}
	})
}

// FuzzParseSeq reads any bytes as a seq file. It never panics, and what it accepts is a positive
// decimal and a line feed.
func FuzzParseSeq(f *testing.F) {
	f.Add([]byte("1\n"))
	f.Add([]byte("01\n"))
	f.Add([]byte("9223372036854775808\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		n, ok := state.ParseSeq(b)
		if ok && (n < 1 || string(b) != strings.TrimSpace(string(b))+"\n") {
			t.Fatalf("accepted %q as %d", b, n)
		}
	})
}

// FuzzLines splits any bytes into lines. It never panics, and the lines with their line feeds are
// the bytes up to the last line feed.
func FuzzLines(f *testing.F) {
	f.Add([]byte("a\nb\nc"))
	f.Add([]byte("\n\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		var joined []byte
		for _, l := range state.Lines(b) {
			joined = append(append(joined, l...), '\n')
		}
		if !bytes.Equal(joined, b[:bytes.LastIndexByte(b, '\n')+1]) {
			t.Fatalf("%q split as %q", b, joined)
		}
	})
}
