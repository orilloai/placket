package journal_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/journal"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/core/signing"
)

func jerr(err error) *journal.Error {
	var e *journal.Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

var (
	accord = "sha256:" + strings.Repeat("a", 64)
	card   = "sha256:" + strings.Repeat("c", 64)
	med    = "ed25519:" + strings.Repeat("d", 64)
	id     = "sha256:" + strings.Repeat("e", 64)
)

// TestASecondWindowIsLocked opens a journal twice: the second is journal-locked, and once the first
// closes the journal opens again.
func TestASecondWindowIsLocked(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "j")
	a, err := journal.Open(dir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Open(dir, &bytes.Buffer{}); jerr(err) == nil || jerr(err).Code != journal.CodeLocked || err.Error() != "journal: journal-locked: accord" {
		t.Fatalf("got %v", err)
	}
	_ = a.Close()
	b, err := journal.Open(dir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	_ = b.Close()
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

// TestAJournalRoundTrips founds a journal, writes an ask, its receipt and a held delivery, and reads
// back what was written; an empty journal is Empty.
func TestAJournalRoundTrips(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "j")
	j, err := journal.Open(dir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	r, err := j.Read()
	if err != nil || !r.Empty {
		t.Fatalf("a new journal: %+v, %v", r, err)
	}
	if err := j.Found(accord, card, med, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, journal.FileAccord))
	if string(b) != `{"accord":"`+accord+`","card":"`+card+`","foundedAt":"2026-11-12T09:00:00Z","mediator":"`+med+`"}`+"\n" {
		t.Fatalf("accord holds %q", b)
	}
	if err := j.AddQuestion(mediator.Open{Question: id, Deadline: "2026-11-12T10:00:00Z", Roles: []string{"decider"}}); err != nil {
		t.Fatal(err)
	}
	q, _ := os.ReadFile(filepath.Join(dir, journal.FileQuestions))
	if string(q) != `{"deadline":"2026-11-12T10:00:00Z","question":"`+id+`","roles":["decider"]}`+"\n" {
		t.Fatalf("questions.jsonl holds %q", q)
	}
	delivery := []byte(`{"kind":"note","receiptId":"` + id + `","text":"a"}`)
	call := []byte(`{"call":"e30=","sig":"AA=="}`)
	if err := j.PutHeld(med, delivery, call); err != nil {
		t.Fatal(err)
	}
	if err := j.PutHeld(med, delivery, nil); jerr(err) == nil {
		t.Fatal("a second held file for one receipt was written")
	}
	h, _ := os.ReadFile(filepath.Join(dir, journal.DirHeld, strings.Repeat("e", 64)))
	to, d, rid, ac, ok := journal.ParseHeld(h)
	if !ok || to != med || !bytes.Equal(d, delivery) || rid != id || !bytes.Equal(ac, call) {
		t.Fatalf("the held file holds %q", h)
	}
	got, err := j.TakeHeld(id)
	if err != nil || !bytes.Equal(got, call) {
		t.Fatalf("take: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, journal.DirHeld, strings.Repeat("e", 64))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a taken held file is still there")
	}
}

// TestReadRepairsWhatWasNeverWritten reads a journal whose receipts.jsonl and questions.jsonl each
// end in a line with no line feed: both are removed, each with one line on stderr, and nothing
// else is touched.
func TestReadRepairsWhatWasNeverWritten(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "j")
	j, err := journal.Open(dir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Found(accord, card, med, "2026-11-12T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	if err := os.WriteFile(filepath.Join(dir, journal.FileReceipts), []byte(`{"payload":"x"`), 0o600); err != nil {
		t.Fatal(err)
	}
	line := `{"deadline":"2026-11-12T10:00:00Z","question":"` + id + `","roles":["decider"]}`
	if err := os.WriteFile(filepath.Join(dir, journal.FileQuestions), []byte(line+"\n"+line[:20]), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	j, err = journal.Open(dir, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	r, err := j.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Receipts) != 0 || r.Accord != accord || r.FoundedAt != "2026-11-12T09:00:00Z" {
		t.Fatalf("got %+v", r)
	}
	want := "journal: receipts.jsonl line 1 had no line feed and was never answered; removed\n" +
		"journal: questions.jsonl line 2 had no line feed and was never answered; removed\n" +
		"journal: questions.jsonl line 1 names a question no receipt holds; it never crossed, and is left out\n"
	if stderr.String() != want {
		t.Fatalf("stderr: %q", stderr.String())
	}
	rb, _ := os.ReadFile(filepath.Join(dir, journal.FileReceipts))
	qb, _ := os.ReadFile(filepath.Join(dir, journal.FileQuestions))
	if len(rb) != 0 || string(qb) != line+"\n" {
		t.Fatalf("receipts %q, questions %q", rb, qb)
	}
}

// TestReadRefusesWhatNoWindowWrote reads journals whose files are not in their formats: each is
// CodeJournal, naming the file and the line.
func TestReadRefusesWhatNoWindowWrote(t *testing.T) {
	t.Parallel()
	good := `{"accord":"` + accord + `","card":"` + card + `","foundedAt":"2026-11-12T09:00:00Z","mediator":"` + med + `"}` + "\n"
	cases := []struct {
		name, file, content string
		want                journal.Error
	}{
		{"accord not canonical", journal.FileAccord, `{ "accord":"x"}` + "\n", journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1}},
		{"accord with another member", journal.FileAccord, strings.Replace(good, `{"accord"`, `{"a":1,"accord"`, 1), journal.Error{Code: journal.CodeJournal, File: journal.FileAccord, Line: 1}},
		{"a question line that is not one", journal.FileQuestions, "{\"x\":1}\n", journal.Error{Code: journal.CodeJournal, File: journal.FileQuestions, Line: 1}},
		{"a held file of another name", filepath.Join(journal.DirHeld, strings.Repeat("f", 64)),
			`{"delivery":{"receiptId":"` + id + `"},"to":"` + med + `"}`, journal.Error{Code: journal.CodeJournal, File: filepath.Join(journal.DirHeld, strings.Repeat("f", 64))}},
	}
	for _, c := range cases {
		dir := filepath.Join(t.TempDir(), "j")
		if err := os.MkdirAll(filepath.Join(dir, journal.DirHeld), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, journal.FileAccord), []byte(good), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, c.file), []byte(c.content), 0o600); err != nil {
			t.Fatal(err)
		}
		j, err := journal.Open(dir, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		_, err = j.Read()
		_ = j.Close()
		if e := jerr(err); e == nil || !reflect.DeepEqual(*e, c.want) {
			t.Fatalf("%s: got %v", c.name, err)
		}
	}
}

// FuzzParseAccord reads any bytes as an accord file. It never panics, and what it accepts has its
// four members.
func FuzzParseAccord(f *testing.F) {
	f.Add([]byte(`{"accord":"a","card":"c","foundedAt":"t","mediator":"m"}` + "\n"))
	f.Add([]byte(`{"accord":"a"}` + "\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		a, c, m, at, ok := journal.ParseAccord(b)
		if ok && (a == "" || c == "" || m == "" || at == "") {
			t.Fatalf("accepted %q", b)
		}
	})
}

// FuzzParseQuestion reads any bytes as a questions.jsonl line. It never panics, and what it accepts
// writes back as the same bytes.
func FuzzParseQuestion(f *testing.F) {
	f.Add([]byte(`{"deadline":"2026-11-12T10:00:00Z","question":"q","roles":["decider"]}`))
	f.Add([]byte(`{"deadline":"d","question":"q","roles":[]}`))
	f.Add([]byte(`{"deadline":"d","question":"q","roles":null}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		o, ok := journal.ParseQuestion(b)
		if ok && !bytes.Equal(journal.QuestionLine(o), b) {
			t.Fatalf("%q wrote back as %q", b, journal.QuestionLine(o))
		}
	})
}

// FuzzParseHeld reads any bytes as a held file. It never panics, and what it accepts writes back as
// the same bytes.
func FuzzParseHeld(f *testing.F) {
	f.Add([]byte(`{"delivery":{"receiptId":"` + id + `"},"to":"` + med + `"}`))
	f.Add([]byte(`{"answerCall":{"call":"x","sig":"y"},"delivery":{"receiptId":"` + id + `"},"to":"m"}`))
	f.Add([]byte(`{"delivery":[],"to":"m"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		to, d, _, ac, ok := journal.ParseHeld(b)
		if !ok {
			return
		}
		again, err := journal.HeldFile(to, d, ac)
		if err != nil || !bytes.Equal(again, b) {
			t.Fatalf("%q wrote back as %q, %v", b, again, err)
		}
	})
}

// TestReadLeavesOpenEntriesWhenReceiptsDoNotVerify reads a journal whose receipt decodes but was
// signed by a key other than the mediator's: which questions are open is read only from receipts
// that verify, so the questions.jsonl entry is left as it is, for Reload to refuse the list, and no
// line says it was left out.
func TestReadLeavesOpenEntriesWhenReceiptsDoNotVerify(t *testing.T) {
	t.Parallel()
	keyOf := func(name string) signing.Key {
		seed := sha256.Sum256([]byte("placket vectors: key " + name))
		k, err := signing.NewKey(seed[:])
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	mediatorKey, other := keyOf("mediator"), keyOf("other-mediator")
	from := keyOf("proposer").Reference()
	forged, _, err := receipts.Build(receipts.Subject{Name: "note", Digest: strings.Repeat("ab", 32)}, receipts.Predicate{
		Accord: accord, Card: card, Kind: receipts.KindCrossing, From: from, To: from, Seq: 1, At: "2026-11-12T09:00:00Z",
		Envelope: receipts.Envelope{Sender: from, Seq: 1, Hash: id}, Verdict: "pass", Reason: "ok",
	}, other)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "j")
	j, err := journal.Open(dir, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Found(accord, card, mediatorKey.Reference(), "2026-11-12T09:00:00Z"); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	if err := os.WriteFile(filepath.Join(dir, journal.FileReceipts), append(forged, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	line := `{"deadline":"2026-11-12T10:00:00Z","question":"` + id + `","roles":["decider"]}`
	if err := os.WriteFile(filepath.Join(dir, journal.FileQuestions), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	j, err = journal.Open(dir, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	r, err := j.Read()
	if err != nil {
		t.Fatal(err)
	}
	want := []mediator.Open{{Question: id, Deadline: "2026-11-12T10:00:00Z", Roles: []string{"decider"}}}
	if !reflect.DeepEqual(r.Open, want) || !reflect.DeepEqual(r.QuestionLines, []int{1}) || stderr.String() != "" {
		t.Fatalf("open %+v, lines %v, stderr %q", r.Open, r.QuestionLines, stderr.String())
	}
}
