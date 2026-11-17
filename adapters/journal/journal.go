// Package journal keeps a window's journal: the directory from which a stopped window's mediator is
// made again (restart-reproducible). It is a door format, not a frozen one: verify, reconcile and
// log read its receipts, and nothing else of it is read by anyone but the window. The directory,
// mode 0700, holds:
//
//	accord           one line, {"accord","card","foundedAt","mediator"}: the Accord id, the card
//	                 hash, the mediator's reference and the time the Accord was founded at. Written
//	                 at the first start and never again. The window holds an exclusive flock on it
//	                 while it runs; a second window on the same journal is refused, journal-locked.
//	receipts.jsonl   one receipt per line, the DSSE envelope's JSON exactly as the mediator returned
//	                 it, in the order written. Made empty at the first start, before the accord file;
//	                 appended and synced before the reply; the receipts of one call in one write.
//	questions.jsonl  one line per ask, {"deadline","question","roles"}: the question's id, its
//	                 deadline and the role labels that may answer it, never a byte of its part.
//	                 Written before the ask's receipt, and kept after the question closes.
//	held/<hex>       one delivery not yet fetched, named by the hex of the id of the receipt that
//	                 queued it: {"answerCall"?,"delivery","to"}, the delivery's canonical JSON, the
//	                 member it is for, and beside it, never inside it, the answerer's signed call when
//	                 the delivery's body is that call's answer (answer-call-attached). Mode 0600.
//	                 Written before its receipt's line; removed, and the directory synced, before the
//	                 fetch that hands it out is answered.
//
// The receipts are evidence (journal-is-evidence): nothing rewrites or truncates receipts.jsonl but
// the one repair at the start, of a last line with no line feed, which was never answered. A last
// line of questions.jsonl with no line feed was never written either, and is removed. Both repairs
// print one line on the error writer. Read hands mediator.Reload what the files hold and judges
// none of it: every check of what was read is Reload's. Read's only clean-ups are the two Reload
// documents: a held file whose receipt was never journaled is removed by RemoveStrays once the
// rest reloaded, and a questions.jsonl line
// whose question is in no receipt line is left in the file and not handed on; each is noted on the
// error writer. Decision record 0011 says why the journal is laid out so.
package journal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/orilloai/placket/adapters/internal/lines"
	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/mediator"
	"github.com/orilloai/placket/core/receipts"
)

// Code says why a journal could not be used.
type Code string

// The codes of Error.
const (
	// CodeJournal: a file of the journal is not what a window wrote, or could not be read or written.
	CodeJournal Code = "journal"
	// CodeLocked: another window holds the journal.
	CodeLocked Code = "journal-locked"
)

// Error is a journal that could not be used: its code, the file and, where the fault has one, the
// line (from 1), and the core's code when the core refused what was read. It holds nothing the file
// holds.
type Error struct {
	Code Code
	File string
	Line int
	Core string
}

func (e *Error) Error() string {
	s := "journal: " + string(e.Code)
	if e.File != "" {
		s += ": " + e.File
	}
	if e.Line > 0 {
		s += fmt.Sprintf(" line %d", e.Line)
	}
	if e.Core != "" {
		s += ": " + e.Core
	}
	return s
}

// The files of a journal.
const (
	FileAccord    = "accord"
	FileReceipts  = "receipts.jsonl"
	FileQuestions = "questions.jsonl"
	DirHeld       = "held"
)

// Disk is how a journal writes. Each method returns only once what it wrote is synced, the
// directory included where a name was made or removed, by Append as by Create. OS is the operating system's; a test injects
// one that fails.
type Disk interface {
	// Append appends b to the file at path, making it, mode 0600, when absent.
	Append(path string, b []byte) error
	// Create makes the file at path, mode 0600, holding b; it is an error for it to exist.
	Create(path string, b []byte) error
	// Remove removes the file at path.
	Remove(path string) error
}

// OS is the operating system's Disk.
var OS Disk = osDisk{}

// osDisk writes through the operating system. syncDir, when set, stands in for syncing a
// directory, so a test can see which writes sync one.
type osDisk struct {
	syncDir func(path string) error
}

func (d osDisk) dir(path string) error {
	if d.syncDir != nil {
		return d.syncDir(path)
	}
	return syncDir(path)
}

// Append appends b, synced. When the file was not there, it makes it and syncs the directory too,
// so that the new name survives a crash as surely as a later write does.
func (d osDisk) Append(path string, b []byte) error {
	made := true
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		made = false
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	}
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil || !made {
		return err
	}
	return d.dir(filepath.Dir(path))
}

func (d osDisk) Create(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	serr := f.Sync()
	cerr := f.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		return err
	}
	return d.dir(filepath.Dir(path))
}

func (d osDisk) Remove(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return d.dir(filepath.Dir(path))
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	serr := d.Sync()
	return errors.Join(serr, d.Close())
}

// A Journal is a window's open journal, locked.
type Journal struct {
	dir    string
	disk   Disk
	stderr io.Writer
	lock   *os.File
}

// Open opens the journal directory dir, making it when absent, writing through the operating
// system, and takes its lock; another window holding it is journal-locked. Repairs and clean-ups
// are reported to stderr.
func Open(dir string, stderr io.Writer) (*Journal, error) { return OpenWith(dir, stderr, OS) }

// OpenWith opens the journal as Open does, writing through disk.
func OpenWith(dir string, stderr io.Writer, disk Disk) (*Journal, error) {
	if err := os.MkdirAll(filepath.Join(dir, DirHeld), 0o700); err != nil {
		return nil, &Error{Code: CodeJournal, File: DirHeld}
	}
	f, err := os.OpenFile(filepath.Join(dir, FileAccord), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: FileAccord}
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, &Error{Code: CodeLocked, File: FileAccord}
		}
		return nil, &Error{Code: CodeJournal, File: FileAccord}
	}
	return &Journal{dir: dir, disk: disk, stderr: stderr, lock: f}, nil
}

// Close lets the lock go. It writes nothing.
func (j *Journal) Close() error {
	if j.lock == nil {
		return nil
	}
	err := j.lock.Close()
	j.lock = nil
	return err
}

func (j *Journal) path(name string) string { return filepath.Join(j.dir, name) }

// The accord file's one line.
type accordLine struct {
	Accord    string `json:"accord"`
	Card      string `json:"card"`
	FoundedAt string `json:"foundedAt"`
	Mediator  string `json:"mediator"`
}

// A Record is what a journal holds, as Reload takes it, with where each value came from.
type Record struct {
	// Empty is true for a journal no window has founded an Accord in; the rest is then zero.
	Empty                             bool
	Accord, Card, Mediator, FoundedAt string
	// Receipts, in the order written: line i+1 of receipts.jsonl is Receipts[i].
	Receipts [][]byte
	// Open is the asks still open, and QuestionLines the line of questions.jsonl each came from.
	Open          []mediator.Open
	QuestionLines []int
	// Queued is the deliveries not yet fetched, and HeldFiles the file each came from.
	Queued    []mediator.Queued
	HeldFiles []string
	// Strays is the held files whose receipt is in no line of receipts.jsonl: they never crossed.
	// RemoveStrays removes them, once Reload took the rest.
	Strays []string
}

// RemoveStrays removes the record's held files whose receipt was never journaled, each with a line
// on the error writer. The window calls it once Reload has taken the record, so that a start that
// stops leaves the journal as it was.
func (j *Journal) RemoveStrays(r *Record) error {
	for _, rel := range r.Strays {
		if err := j.disk.Remove(j.path(rel)); err != nil {
			return &Error{Code: CodeJournal, File: rel}
		}
		fmt.Fprintf(j.stderr, "journal: %s holds a delivery whose receipt was never journaled; it never crossed, and is removed\n", rel)
	}
	return nil
}

// Found writes the accord file of a journal no window has founded an Accord in, and an empty
// receipts.jsonl beside it, so that a founded journal always has one for its readers.
func (j *Journal) Found(accord, card, mediatorRef, foundedAt string) error {
	f, err := os.OpenFile(j.path(FileReceipts), os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return &Error{Code: CodeJournal, File: FileReceipts}
	}
	serr := f.Sync()
	if err := errors.Join(serr, f.Close()); err != nil {
		return &Error{Code: CodeJournal, File: FileReceipts}
	}
	line, _ := json.Marshal(accordLine{Accord: accord, Card: card, FoundedAt: foundedAt, Mediator: mediatorRef})
	c, _ := canonical.Form(line)
	if _, err := j.lock.WriteAt(append(c, '\n'), 0); err != nil || j.lock.Sync() != nil {
		return &Error{Code: CodeJournal, File: FileAccord}
	}
	if err := syncDir(j.dir); err != nil {
		return &Error{Code: CodeJournal, File: FileAccord}
	}
	return nil
}

// ParseAccord reads the accord file's bytes: one line, in canonical form, with exactly its four
// members, each a string.
func ParseAccord(b []byte) (accord, card, mediatorRef, foundedAt string, ok bool) {
	if len(b) < 2 || b[len(b)-1] != '\n' {
		return "", "", "", "", false
	}
	line := b[:len(b)-1]
	c, err := canonical.Form(line)
	if err != nil || !bytes.Equal(c, line) {
		return "", "", "", "", false
	}
	var a accordLine
	if !strict.Decode(line, &a) || a.Accord == "" || a.Card == "" || a.Mediator == "" || a.FoundedAt == "" {
		return "", "", "", "", false
	}
	return a.Accord, a.Card, a.Mediator, a.FoundedAt, true
}

// A questions.jsonl line.
type questionLine struct {
	Deadline string   `json:"deadline"`
	Question string   `json:"question"`
	Roles    []string `json:"roles"`
}

// QuestionLine is the questions.jsonl line of an ask.
func QuestionLine(o mediator.Open) []byte {
	roles := o.Roles
	if roles == nil {
		roles = []string{}
	}
	b, _ := json.Marshal(questionLine{Deadline: o.Deadline, Question: o.Question, Roles: roles})
	c, _ := canonical.Form(b)
	return c
}

// ParseQuestion reads one questions.jsonl line: an object in canonical form with exactly its three
// members, the roles a list of strings.
func ParseQuestion(line []byte) (mediator.Open, bool) {
	c, err := canonical.Form(line)
	if err != nil || !bytes.Equal(c, line) {
		return mediator.Open{}, false
	}
	var q questionLine
	if !strict.Decode(line, &q) || q.Question == "" || q.Deadline == "" || q.Roles == nil {
		return mediator.Open{}, false
	}
	return mediator.Open{Question: q.Question, Deadline: q.Deadline, Roles: q.Roles}, true
}

// A held file.
type heldFile struct {
	AnswerCall json.RawMessage `json:"answerCall,omitempty"`
	Delivery   json.RawMessage `json:"delivery"`
	To         string          `json:"to"`
}

// HeldFile is the bytes of the held file of a delivery queued for to, with the answer call beside
// it when it has one (nil otherwise).
func HeldFile(to string, delivery, answerCall []byte) ([]byte, error) {
	h := heldFile{Delivery: delivery, To: to}
	if answerCall != nil {
		h.AnswerCall = answerCall
	}
	b, err := json.Marshal(h)
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: DirHeld}
	}
	c, err := canonical.Form(b)
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: DirHeld}
	}
	return c, nil
}

// ParseHeld reads a held file's bytes: an object in canonical form with its members, the delivery an
// object whose receiptId is a typed hash. It returns the member the delivery is for, the delivery's
// canonical bytes, its receipt id and the answer call beside it, if any.
func ParseHeld(b []byte) (to string, delivery []byte, receiptID string, answerCall []byte, ok bool) {
	c, err := canonical.Form(b)
	if err != nil || !bytes.Equal(c, b) {
		return "", nil, "", nil, false
	}
	var h heldFile
	if !strict.Decode(b, &h) || h.To == "" || len(h.Delivery) == 0 || h.Delivery[0] != '{' {
		return "", nil, "", nil, false
	}
	rid, ok := receiptIDOf(h.Delivery)
	if !ok {
		return "", nil, "", nil, false
	}
	if len(h.AnswerCall) > 0 {
		if h.AnswerCall[0] != '{' {
			return "", nil, "", nil, false
		}
		answerCall = append([]byte(nil), h.AnswerCall...)
	}
	return h.To, append([]byte(nil), h.Delivery...), rid, answerCall, true
}

// receiptIDOf is a delivery's receiptId member, a typed hash.
func receiptIDOf(delivery []byte) (string, bool) {
	m, ok := strict.Member(delivery, "receiptId")
	var id string
	if !ok || json.Unmarshal(m, &id) != nil || !isHash(id) {
		return "", false
	}
	return id, true
}

func isHash(s string) bool {
	h, ok := strings.CutPrefix(s, canonical.HashPrefix)
	if !ok || len(h) != 64 {
		return false
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// heldName is the name of the held file of the delivery the receipt queued.
func heldName(receiptID string) string {
	return filepath.Join(DirHeld, strings.TrimPrefix(receiptID, canonical.HashPrefix))
}

// Read reads the journal for Reload. A journal no window founded an Accord in is Empty. It repairs
// what was never written, an unterminated last line of receipts.jsonl or questions.jsonl, and makes
// the clean-ups Reload documents, each with a line on the error writer. A file whose lines or
// members are not what a window writes is CodeJournal, with the file and line.
func (j *Journal) Read() (*Record, error) {
	b, err := io.ReadAll(io.NewSectionReader(j.lock, 0, 1<<20))
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: FileAccord}
	}
	if len(b) == 0 {
		// Found writes an empty receipts.jsonl before the accord file: a start stopped between the
		// two left a journal no Accord was founded in.
		if fi, err := os.Stat(j.path(FileReceipts)); !errors.Is(err, fs.ErrNotExist) && (err != nil || fi.Size() != 0) {
			return nil, &Error{Code: CodeJournal, File: FileAccord, Line: 1}
		}
		if _, err := os.Stat(j.path(FileQuestions)); !errors.Is(err, fs.ErrNotExist) {
			return nil, &Error{Code: CodeJournal, File: FileAccord, Line: 1}
		}
		return &Record{Empty: true}, nil
	}
	r := &Record{}
	var ok bool
	if r.Accord, r.Card, r.Mediator, r.FoundedAt, ok = ParseAccord(b); !ok {
		return nil, &Error{Code: CodeJournal, File: FileAccord, Line: 1}
	}
	if r.Receipts, err = j.repairedLines(FileReceipts); err != nil {
		return nil, err
	}
	questions, err := j.repairedLines(FileQuestions)
	if err != nil {
		return nil, err
	}

	// Which questions the receipts name, and which they leave open, by the one reader of questions,
	// read only from receipts that verify under the mediator's reference (verified-by-type). A list
	// that does not verify leaves both unknown, and every entry as it is: Reload refuses the list
	// before it reads an open entry.
	ids := map[string]bool{}
	open := map[string]bool{}
	verified, err := receipts.Verify(r.Mediator, r.Receipts)
	var places []receipts.Place
	if err == nil {
		places, err = receipts.Releases(verified)
	}
	known := err == nil
	if known {
		closed := map[int]bool{}
		for _, p := range places {
			if p.Role == receipts.RoleClosing {
				closed[p.Question] = true
			}
		}
		for i, v := range verified {
			id := v.Statement().ID
			ids[id] = true
			if places[i].Role == receipts.RoleQuestion && !closed[i] {
				open[id] = true
			}
		}
	}

	for i, line := range questions {
		o, ok := ParseQuestion(line)
		if !ok {
			return nil, &Error{Code: CodeJournal, File: FileQuestions, Line: i + 1}
		}
		switch {
		case !known:
			r.Open = append(r.Open, o)
			r.QuestionLines = append(r.QuestionLines, i+1)
		case !ids[o.Question]:
			fmt.Fprintf(j.stderr, "journal: %s line %d names a question no receipt holds; it never crossed, and is left out\n", FileQuestions, i+1)
		case open[o.Question]:
			r.Open = append(r.Open, o)
			r.QuestionLines = append(r.QuestionLines, i+1)
		}
	}

	entries, err := os.ReadDir(j.path(DirHeld))
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: DirHeld}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		rel := filepath.Join(DirHeld, name)
		hb, err := os.ReadFile(j.path(rel))
		if err != nil {
			return nil, &Error{Code: CodeJournal, File: rel}
		}
		to, delivery, id, _, ok := ParseHeld(hb)
		clear(hb)
		if !ok || heldName(id) != rel {
			return nil, &Error{Code: CodeJournal, File: rel}
		}
		if known && !ids[id] {
			r.Strays = append(r.Strays, rel)
			continue
		}
		r.Queued = append(r.Queued, mediator.Queued{To: to, Delivery: delivery})
		r.HeldFiles = append(r.HeldFiles, rel)
	}
	return r, nil
}

// repairedLines reads the whole lines of the file name, after removing an unterminated last line
// with a line on the error writer. A missing file has none.
func (j *Journal) repairedLines(name string) ([][]byte, error) {
	b, err := os.ReadFile(j.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return [][]byte{}, nil
	}
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: name}
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		// Opened for writing only when there is a line to cut.
		keep := bytes.LastIndexByte(b, '\n') + 1
		f, err := os.OpenFile(j.path(name), os.O_RDWR, 0)
		if err != nil {
			return nil, &Error{Code: CodeJournal, File: name}
		}
		terr := f.Truncate(int64(keep))
		serr := f.Sync()
		if err := errors.Join(terr, serr, f.Close()); err != nil {
			return nil, &Error{Code: CodeJournal, File: name}
		}
		fmt.Fprintf(j.stderr, "journal: %s line %d had no line feed and was never answered; removed\n", name, bytes.Count(b[:keep], []byte{'\n'})+1)
		b = b[:keep]
	}
	out := [][]byte{}
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		out = append(out, append([]byte(nil), b[:i]...))
		b = b[i+1:]
	}
	return out, nil
}

// AddQuestion appends an ask's line to questions.jsonl, synced. The window writes it before the
// ask's receipt.
func (j *Journal) AddQuestion(o mediator.Open) error {
	if err := j.disk.Append(j.path(FileQuestions), append(QuestionLine(o), '\n')); err != nil {
		return &Error{Code: CodeJournal, File: FileQuestions}
	}
	return nil
}

// AddReceipts appends the receipts of one call to receipts.jsonl in one write, synced.
func (j *Journal) AddReceipts(list [][]byte) error {
	if len(list) == 0 {
		return nil
	}
	var b []byte
	for _, r := range list {
		if bytes.IndexByte(r, '\n') >= 0 {
			return &Error{Code: CodeJournal, File: FileReceipts}
		}
		b = append(append(b, r...), '\n')
	}
	if err := j.disk.Append(j.path(FileReceipts), b); err != nil {
		return &Error{Code: CodeJournal, File: FileReceipts}
	}
	return nil
}

// PutHeld writes the held file of a delivery not yet fetched, queued for to, with the answer call
// beside it when it has one. The window writes it before the receipt that queued it.
func (j *Journal) PutHeld(to string, delivery, answerCall []byte) error {
	id, ok := receiptIDOf(delivery)
	if !ok {
		return &Error{Code: CodeJournal, File: DirHeld}
	}
	b, err := HeldFile(to, delivery, answerCall)
	if err != nil {
		return err
	}
	defer clear(b)
	rel := heldName(id)
	if err := j.disk.Create(j.path(rel), b); err != nil {
		return &Error{Code: CodeJournal, File: rel}
	}
	return nil
}

// TakeHeld removes the held file of the delivery the receipt queued, synced, and returns the answer
// call that was beside it, or nil. The window calls it before it answers the fetch.
func (j *Journal) TakeHeld(receiptID string) ([]byte, error) {
	rel := heldName(receiptID)
	b, err := os.ReadFile(j.path(rel))
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: rel}
	}
	_, _, id, answerCall, ok := ParseHeld(b)
	clear(b)
	if !ok || id != receiptID {
		return nil, &Error{Code: CodeJournal, File: rel}
	}
	if err := j.disk.Remove(j.path(rel)); err != nil {
		return nil, &Error{Code: CodeJournal, File: rel}
	}
	return answerCall, nil
}

// ReadReceipts reads the receipts of the journal directory dir, one per whole line of
// receipts.jsonl, in file order, as verify, reconcile and log read them. It takes no lock and
// writes nothing. A directory with no receipts.jsonl is no journal: CodeJournal, naming the file,
// since every window writes one when it founds, and so is one that is not a regular file, where
// errors.Is also finds lines.ErrNotRegular. A line longer than maxLine is CodeJournal at that line.
// A last line with no line feed is left out, with one line on stderr saying the file was cut there.
func ReadReceipts(dir string, stderr io.Writer) ([][]byte, error) {
	f, err := lines.Read(filepath.Join(dir, FileReceipts), maxLine)
	if errors.Is(err, lines.ErrNotRegular) {
		return nil, lines.NotRegular(&Error{Code: CodeJournal, File: FileReceipts})
	}
	if err != nil {
		return nil, &Error{Code: CodeJournal, File: FileReceipts}
	}
	if f.Long > 0 {
		return nil, &Error{Code: CodeJournal, File: FileReceipts, Line: f.Long}
	}
	if f.Cut {
		fmt.Fprintf(stderr, "journal: %s line %d has no line feed; the file was cut there, and the line is left out\n", FileReceipts, len(f.Lines)+1)
	}
	if f.Lines == nil {
		return [][]byte{}, nil
	}
	return f.Lines, nil
}

// maxLine is the longest line ReadReceipts reads: local.MaxReply, the most a window replies with,
// which holds any receipt a window writes.
const maxLine = 4 << 20
