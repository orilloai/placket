package state

import (
	"io"
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
