package journal_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/journal"
)

// TestAddJudged appends a receipt's judge records to judged/<hex of its id>.jsonl, mode 0600, in a
// directory of mode 0700 made on first use, one line each, the later lines after the earlier; and
// Read opens none of it: a judged/ that holds anything at all reads as before (docs/adr/0023-judges.md).
func TestAddJudged(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "j")
	j, err := journal.Open(dir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err := j.Found(accord, card, med, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	before, err := j.Read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, journal.DirJudged)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("judged/ before any record: %v", err)
	}
	if err := j.AddJudged(id, [][]byte{[]byte(`{"judge":"a"}`), []byte(`{"judge":"b"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := j.AddJudged(id, [][]byte{[]byte(`{"judge":"c"}`)}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, journal.DirJudged, strings.TrimPrefix(id, "sha256:")+".jsonl")
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "{\"judge\":\"a\"}\n{\"judge\":\"b\"}\n{\"judge\":\"c\"}\n" {
		t.Fatalf("judged file: %q, %v", b, err)
	}
	fi, _ := os.Stat(path)
	di, _ := os.Stat(filepath.Join(dir, journal.DirJudged))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Fatalf("modes %v and %v", fi.Mode().Perm(), di.Mode().Perm())
	}
	if err := os.WriteFile(filepath.Join(dir, journal.DirJudged, "not a record"), []byte("\x00{"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := j.Read()
	if err != nil || after.Accord != before.Accord || len(after.Receipts) != len(before.Receipts) {
		t.Fatalf("Read with judged/: %+v, %v", after, err)
	}
	if err := j.AddJudged(id, [][]byte{[]byte("two\nlines")}); jerr(err) == nil || jerr(err).File != filepath.Join(journal.DirJudged, strings.TrimPrefix(id, "sha256:")+".jsonl") {
		t.Fatalf("a line with a line feed: %v", err)
	}
	if err := j.AddJudged("not an id", [][]byte{[]byte("{}")}); jerr(err) == nil || jerr(err).File != journal.DirJudged {
		t.Fatalf("an id that is no typed hash: %v", err)
	}
}

// refusing is a Disk that fails every append.
type refusing struct{}

func (refusing) Append(string, []byte) error     { return errors.New("injected") }
func (refusing) Create(p string, b []byte) error { return journal.OS.Create(p, b) }
func (refusing) Remove(p string) error           { return journal.OS.Remove(p) }

// TestAddJudgedThatFails is a judged/ append that fails: a journal error naming the file
// (journal-synced).
func TestAddJudgedThatFails(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "j")
	j, err := journal.OpenWith(dir, &bytes.Buffer{}, refusing{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	err = j.AddJudged(id, [][]byte{[]byte("{}")})
	if e := jerr(err); e == nil || e.Code != journal.CodeJournal || e.File != filepath.Join(journal.DirJudged, strings.TrimPrefix(id, "sha256:")+".jsonl") {
		t.Fatalf("got %v", err)
	}
}
