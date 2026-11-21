package state

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestALineThatMakesItsFileSyncsTheDirectory appends the first line of kept.jsonl and of
// envelopes.jsonl, and a second line of each: each first line makes its file and syncs the
// directory too, so that a crash cannot lose the new name while a later write survives; a second
// line syncs only its file.
func TestALineThatMakesItsFileSyncsTheDirectory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s")
	d, err := Open(path, "ed25519:"+strings.Repeat("1", 64), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var synced []string
	d.syncDir = func(p string) error { synced = append(synced, p); return nil }
	h := "sha256:" + strings.Repeat("2", 64)
	steps := []struct {
		name string
		do   func() error
		want []string
	}{
		{"the first kept line", func() error { return d.Keep([]byte(`{"a":1}`)) }, []string{path}},
		{"the second kept line", func() error { return d.Keep([]byte(`{"a":2}`)) }, []string{path}},
		{"the first envelope line", func() error { return d.AddEnvelope(1, h) }, []string{path, path}},
		{"the second envelope line", func() error { return d.AddEnvelope(2, h) }, []string{path, path}},
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if !reflect.DeepEqual(synced, s.want) {
			t.Fatalf("%s: synced %q", s.name, synced)
		}
	}
}

// TestAReplacedFileWhoseDirectorySyncFailsIsReported replaces the seq file, a kept view and a kept
// acceptance in a directory whose sync fails: each write is code state, naming its file, since the
// new name may not survive a crash and the caller must not go on as if it would. The file was
// replaced all the same, and the directory was synced once for each.
func TestAReplacedFileWhoseDirectorySyncFailsIsReported(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "s")
	d, err := Open(path, "ed25519:"+strings.Repeat("1", 64), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var synced []string
	d.syncDir = func(p string) error { synced = append(synced, p); return errors.New("sync failed") }
	for _, c := range []struct {
		file  string
		write func() error
		want  string
	}{
		{FileSeq, func() error { return d.writeFile(FileSeq, []byte("7\n")) }, "7\n"},
		{FileSeq, func() error { return d.SetSeq(8) }, "8\n"},
		{FileView, func() error { return d.KeepView([]byte(`{"v":1}`)) }, `{"v":1}` + "\n"},
		{FileAcceptance, func() error { return d.KeepAcceptance([]byte(`{"a":1}`)) }, `{"a":1}` + "\n"},
	} {
		synced = nil
		err := c.write()
		if e, ok := err.(*Error); !ok || *e != (Error{Code: CodeState, File: c.file}) {
			t.Fatalf("%s: got %#v, want code state naming %s", c.file, err, c.file)
		}
		if b, err := os.ReadFile(filepath.Join(path, c.file)); err != nil || string(b) != c.want {
			t.Fatalf("%s: holds %q, %v; want %q", c.file, b, err, c.want)
		}
		if !reflect.DeepEqual(synced, []string{path}) {
			t.Fatalf("%s: synced %q", c.file, synced)
		}
	}
}
