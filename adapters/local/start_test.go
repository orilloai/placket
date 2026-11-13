package local_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/mediator"
)

// journaled is a world whose journal holds a pass to the applier not yet fetched, and an ask that was
// accepted and released, with its answer and its release not yet fetched; its window is stopped.
func journaled(t *testing.T) *world {
	t.Helper()
	w := newWorld(t)
	win := start(t, w.window(t))
	w.give(t, "proposer", "applier", "n-1", "note", "A note.")
	q := w.give(t, "proposer", "applier", "p-1", "proposal", "Rename total.").Outcome.ReceiptID
	w.call(t, "decider", local.OpResolve, local.ResolveArgs(q, mediator.Accepted, "accepted"))
	win.stop(t)
	return w
}

// runOnce starts a window and returns its stderr, and what Run returned if it stopped by itself.
func runOnce(t *testing.T, cfg local.Window) (string, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	var stderr bytes.Buffer
	cfg.Ready, cfg.Stderr = ready, &lockedWriter{mu: new(syncMutex), b: &stderr}
	done := make(chan error, 1)
	go func() { done <- local.Run(ctx, cfg) }()
	select {
	case err := <-done:
		return stderr.String(), err
	case <-ready:
		cancel()
		<-done
		return stderr.String(), nil
	case <-time.After(10 * time.Second):
		t.Fatal("the window neither started nor stopped")
	}
	return "", nil
}

// TestAStartRepairsOnlyWhatWasNeverWritten starts windows on journals that end in a line with no
// line feed, of receipts.jsonl and of questions.jsonl, that hold a held file whose receipt was never
// journaled, or a questions.jsonl line whose question no receipt holds: each start goes on, after
// one line on stderr, and the window then serves its calls (LOG-10, LOG-11, LOG-29). Only the two
// unterminated lines and the held file are removed; the question line is left.
func TestAStartRepairsOnlyWhatWasNeverWritten(t *testing.T) {
	t.Parallel()
	stray := "sha256:" + strings.Repeat("9", 64)
	cases := []struct {
		name, file, add, note string
		gone                  bool
	}{
		{"an unterminated receipt", journal.FileReceipts, `{"payload":"e30=","payloadType":`, "receipts.jsonl line 5 had no line feed and was never answered; removed", true},
		{"an unterminated question", journal.FileQuestions, `{"deadline":"2026`, "questions.jsonl line 2 had no line feed and was never answered; removed", true},
		{"a held file never journaled", filepath.Join(journal.DirHeld, strings.Repeat("9", 64)),
			`{"delivery":{"receiptId":"` + stray + `"},"to":"` + key(t, "applier").Reference() + `"}`,
			"held/" + strings.Repeat("9", 64) + " holds a delivery whose receipt was never journaled; it never crossed, and is removed", true},
		{"a question no receipt holds", journal.FileQuestions, `{"deadline":"2026-11-12T10:00:00Z","question":"` + stray + `","roles":["decider"]}` + "\n",
			"questions.jsonl line 2 names a question no receipt holds; it never crossed, and is left out", false},
	}
	for _, c := range cases {
		w := journaled(t)
		path := filepath.Join(w.journal, c.file)
		was, _ := os.ReadFile(path)
		if err := os.WriteFile(path, append(was, c.add...), 0o600); err != nil {
			t.Fatal(err)
		}
		win := start(t, w.window(t))
		if got := win.stderr.String(); got != "journal: "+c.note+"\n" {
			t.Fatalf("%s: stderr %q", c.name, got)
		}
		now, _ := os.ReadFile(path)
		if c.gone && !bytes.Equal(now, was) || !c.gone && string(now) != string(was)+c.add {
			t.Fatalf("%s: the file holds %q", c.name, now)
		}
		if r := w.call(t, "applier", local.OpGet, nil); r.Delivery == nil {
			t.Fatalf("%s: the applier's note was not queued again", c.name)
		}
		win.stop(t)
	}
}

// TestAStartStopsAtAFault starts windows on journals holding a fault, LOG-06 to LOG-09: an edited
// receipt line, a line removed, two lines swapped, a line that is not a receipt, another Accord's founding set,
// a held file whose body does not give its digest, a held file for a receipt that queued nothing
// for its party, and a question line for a question the receipts left closed. Each stops with tool
// error journal naming the file and line and the core's code, and the journal is byte for byte
// what it was. A second window on one journal is journal-locked.
func TestAStartStopsAtAFault(t *testing.T) {
	t.Parallel()
	edit := func(name string, f func(b []byte) []byte) func(w *world) {
		return func(w *world) {
			p := filepath.Join(w.journal, name)
			b, _ := os.ReadFile(p)
			if err := os.WriteFile(p, f(b), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	lines := func(f func(l [][]byte) [][]byte) func(b []byte) []byte {
		return func(b []byte) []byte {
			l := bytes.Split(bytes.TrimSuffix(b, []byte{'\n'}), []byte{'\n'})
			return append(bytes.Join(f(l), []byte{'\n'}), '\n')
		}
	}
	held := func(w *world) string {
		es, _ := os.ReadDir(filepath.Join(w.journal, journal.DirHeld))
		for _, e := range es {
			b, _ := os.ReadFile(filepath.Join(w.journal, journal.DirHeld, e.Name()))
			if bytes.Contains(b, []byte(`"kind":"note"`)) {
				return filepath.Join(journal.DirHeld, e.Name())
			}
		}
		t.Fatal("no held note")
		return ""
	}
	cases := []struct {
		name  string
		fault func(w *world)
		other bool
		want  func(w *world) journal.Error
	}{
		{"an edited receipt", edit(journal.FileReceipts, lines(func(l [][]byte) [][]byte {
			l[1] = bytes.Replace(l[1], []byte(`"payload":"ey`), []byte(`"payload":"eX`), 1)
			return l
		})), false, func(*world) journal.Error {
			return journal.Error{Code: journal.CodeJournal, File: journal.FileReceipts, Line: 2, Core: "not-canonical"}
		}},
		{"the first line removed", edit(journal.FileReceipts, lines(func(l [][]byte) [][]byte { return l[1:] })), false,
			func(*world) journal.Error {
				return journal.Error{Code: journal.CodeJournal, File: journal.FileReceipts, Line: 1, Core: "chain"}
			}},
		{"the ask's line removed", edit(journal.FileReceipts, lines(func(l [][]byte) [][]byte { return append(l[:1], l[2:]...) })), false,
			func(*world) journal.Error {
				return journal.Error{Code: journal.CodeJournal, File: journal.FileReceipts, Line: 2, Core: "release"}
			}},
		{"two lines swapped", edit(journal.FileReceipts, lines(func(l [][]byte) [][]byte { l[0], l[1] = l[1], l[0]; return l })), false,
			func(*world) journal.Error {
				return journal.Error{Code: journal.CodeJournal, File: journal.FileReceipts, Line: 1, Core: "chain"}
			}},
		{"a line that is no receipt", edit(journal.FileReceipts, lines(func(l [][]byte) [][]byte { return append(l, []byte(`{"payload":`)) })), false,
			func(*world) journal.Error {
				return journal.Error{Code: journal.CodeJournal, File: journal.FileReceipts, Line: 5, Core: "not-canonical"}
			}},
		{"another Accord's founding set", func(*world) {}, true, func(*world) journal.Error {
			return journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1, Core: "accord"}
		}},
		{"a held body that does not give its digest", func(w *world) {
			edit(held(w), func(b []byte) []byte { return bytes.Replace(b, []byte("A note."), []byte("A nota."), 1) })(w)
		}, false, func(w *world) journal.Error {
			return journal.Error{Code: journal.CodeJournal, File: held(w), Core: "held"}
		}},
		{"a held file for another party", func(w *world) {
			edit(held(w), func(b []byte) []byte {
				return bytes.Replace(b, []byte(key(t, "applier").Reference()), []byte(key(t, "decider").Reference()), 1)
			})(w)
		}, false, func(w *world) journal.Error {
			return journal.Error{Code: journal.CodeJournal, File: held(w), Core: "held"}
		}},
		{"a question line for a closed question, twice", edit(journal.FileQuestions, func(b []byte) []byte { return append(b, b...) }), false,
			func(*world) journal.Error { return journal.Error{} }},
	}
	for _, c := range cases {
		w := journaled(t)
		c.fault(w)
		before := snapshot(t, w.journal)
		cfg := w.window(t)
		if c.other {
			cfg.Founding = askAndLogOther(t)
		}
		_, err := runOnce(t, cfg)
		want := c.want(w)
		if want == (journal.Error{}) {
			if err != nil {
				t.Fatalf("%s: got %v", c.name, err)
			}
			continue
		}
		var je *journal.Error
		if !errors.As(err, &je) || !reflect.DeepEqual(*je, want) {
			t.Fatalf("%s: got %v, want %v", c.name, err, &want)
		}
		if after := snapshot(t, w.journal); !reflect.DeepEqual(after, before) {
			t.Fatalf("%s: a start that stopped changed the journal", c.name)
		}
	}

	w := newWorld(t)
	start(t, w.window(t))
	other := w.window(t)
	other.Socket += "2"
	_, err := runOnce(t, other)
	var je *journal.Error
	if !errors.As(err, &je) || je.Code != journal.CodeLocked {
		t.Fatalf("a second window: got %v", err)
	}
}
