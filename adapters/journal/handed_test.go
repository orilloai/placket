package journal_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
)

// TestReadReceiptsReadsOnlyARegularFile hands ReadReceipts a journal whose receipts.jsonl is a FIFO,
// and one that is a directory: each is code journal, naming the file, and the FIFO is never waited
// on.
func TestReadReceiptsReadsOnlyARegularFile(t *testing.T) {
	t.Parallel()
	fifo := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(fifo, journal.FileReceipts), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, journal.FileReceipts), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{fifo, dir} {
		stop := unblock(filepath.Join(d, journal.FileReceipts))
		_, err := journal.ReadReceipts(d, &bytes.Buffer{})
		stop()
		if e := jerr(err); e == nil || e.Code != journal.CodeJournal || e.File != journal.FileReceipts || e.Line != 0 {
			t.Fatalf("got %v, want journal: receipts.jsonl", err)
		}
	}
}

// TestReadReceiptsCapsEachLine reads a receipts.jsonl whose second line is one byte past 4 MiB:
// code journal at line 2. A line of 4 MiB exactly is read, and the file as a whole has no cap.
func TestReadReceiptsCapsEachLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	full := bytes.Repeat([]byte("a"), 4<<20)
	b := append(append(append([]byte(nil), full...), '\n'), full...)
	if err := os.WriteFile(filepath.Join(dir, journal.FileReceipts), append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if l, err := journal.ReadReceipts(dir, &bytes.Buffer{}); err != nil || len(l) != 2 || len(l[1]) != 4<<20 {
		t.Fatalf("two lines of 4 MiB: got %d lines, %v", len(l), err)
	}
	if err := os.WriteFile(filepath.Join(dir, journal.FileReceipts), append(append(b, 'a'), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := journal.ReadReceipts(dir, &bytes.Buffer{})
	if e := jerr(err); e == nil || e.Code != journal.CodeJournal || e.File != journal.FileReceipts || e.Line != 2 {
		t.Fatalf("a second line of 4 MiB and a byte: got %v, want journal at line 2", err)
	}
}

// TestJournalErrorsQuoteNothing plants a secret in each file a window's journal reads: the accord
// file, a questions.jsonl line and a held file. Each error's text names the file and line, never the
// secret (error-text-no-content).
func TestJournalErrorsQuoteNothing(t *testing.T) {
	t.Parallel()
	secret := "SEKRET-5e4b"
	for _, c := range []struct {
		file, body, want string
	}{
		{journal.FileAccord, `{"accord":"` + secret + `"}` + "\n", "journal: journal: accord line 1"},
		{journal.FileQuestions, `{"question":"` + secret + `"}` + "\n", "journal: journal: questions.jsonl line 1"},
	} {
		dir := t.TempDir()
		j, err := journal.Open(dir, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if c.file != journal.FileAccord {
			if err := j.Found(accord, card, med, "2026-11-12T09:00:00Z"); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, c.file), []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err = j.Read()
		j.Close()
		if err == nil || err.Error() != c.want || strings.Contains(err.Error(), secret) {
			t.Fatalf("%s: got %v, want %s", c.file, err, c.want)
		}
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
