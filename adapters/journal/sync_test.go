package journal

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestAnAppendThatMakesItsFileSyncsTheDirectory appends twice to a file that is not there yet, as
// the first ask does to questions.jsonl. The first append makes the file and syncs its directory
// too, so that a crash cannot lose the new name while a later write survives (journal-synced); the
// second finds the file and syncs only it. The file holds both lines, mode 0600.
func TestAnAppendThatMakesItsFileSyncsTheDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var synced []string
	d := osDisk{syncDir: func(p string) error { synced = append(synced, p); return nil }}
	p := filepath.Join(dir, FileQuestions)
	if err := d.Append(p, []byte("a\n")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(synced, []string{dir}) {
		t.Fatalf("the append that made the file synced %q", synced)
	}
	if err := d.Append(p, []byte("b\n")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(synced, []string{dir}) {
		t.Fatalf("the append to the file there synced %q", synced)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "a\nb\n" {
		t.Fatalf("the file holds %q, %v", b, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}
