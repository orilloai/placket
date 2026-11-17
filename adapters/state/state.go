// Package state keeps an actor's state directory: what one key holds of its own crossings under one
// Accord. It is a door format, not a frozen one: no receipt carries it and no verifier reads it. The
// directory, mode 0700, holds:
//
//	actor            the key's actor reference and a line feed, written at first use
//	lock             empty; held under an exclusive flock while a socket command runs (state-lock)
//	seq              the last envelope seq the mediator took, in decimal, and a line feed; absent is 0
//	envelopes.jsonl  one {"hash","seq"} per envelope signed, appended before the call is sent and
//	                 removed again only on a rejection, so a lost submission shows as missing (crossing-5);
//	                 the next seq is above both this file's highest and the seq file's
//	delivered.jsonl  one receipt per line, the receipt of each delivery fetched
//	kept.jsonl       one line per thing the actor chose to keep: a part it gave,
//	                 {"message","receipt"}, the message as standard base64 of its exact bytes; a part
//	                 it fetched, or a version it gave as an answer, as the delivery holds it, the
//	                 answer call inside it when an answer queued it; or an answer call it signed,
//	                 {"answer","salt"}, the call and the salt it minted for the answer. An answer line
//	                 has no receipt, which tells it from a part: a reader of parts passes over it
//	salts.jsonl      one {"receiptId","salt"} per delivery fetched: the salt it came with, by the id
//	                 of its receipt, so the actor can later show which receipt a file's bytes are of
//	close.json       the Accord's close receipt and a line feed, once a fetch returned it (close-3);
//	                 written once, and never replaced by another
//
// Each .jsonl file is appended one whole line at a time and synced, with the directory when the line
// made the file. A last line with no line feed
// was never written: the reads leave it out, and Lock, which runs before anything is appended,
// removes it with one line on the error writer, so it never joins the next line.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/orilloai/placket/adapters/internal/lines"
	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/receipts"
)

// Code says what is wrong with a state directory.
type Code string

// The codes of Error.
const (
	// CodeState: a file of the directory could not be read or written, or is not in its format.
	CodeState Code = "state"
	// CodeActor: the directory belongs to another key.
	CodeActor Code = "state-actor"
)

// Error is a state directory that could not be used: its code, and the file at fault. Its text is
// the package and the code alone; File, which may be a path, is for the caller to print.
type Error struct {
	Code Code
	File string
}

// Error returns "state: " and the code.
func (e *Error) Error() string { return "state: " + string(e.Code) }

// The files of a state directory.
const (
	FileActor     = "actor"
	FileLock      = "lock"
	FileSeq       = "seq"
	FileEnvelopes = "envelopes.jsonl"
	FileDelivered = "delivered.jsonl"
	FileKept      = "kept.jsonl"
	FileSalts     = "salts.jsonl"
	FileClose     = "close.json"
)

// A Dir is one actor's state directory.
type Dir struct {
	path   string
	actor  string
	stderr io.Writer
	lock   *os.File
	// syncDir, when set, stands in for syncing the directory, so a test can see which writes sync it.
	syncDir func(path string) error
}

// sync syncs the directory, after a name in it was made.
func (d *Dir) sync() error {
	if d.syncDir != nil {
		return d.syncDir(d.path)
	}
	return syncDir(d.path)
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	serr := f.Sync()
	return errors.Join(serr, f.Close())
}

// DefaultPath is the state directory of the key file keyFile under the Accord accord:
// <key file>.state/<the Accord id's hex>/.
func DefaultPath(keyFile, accord string) string {
	return filepath.Join(keyFile+".state", strings.TrimPrefix(accord, canonical.HashPrefix))
}

// Open opens the state directory at path for the actor, making it on first use, and writing its
// actor file. A directory whose actor file names another key is refused. Repairs are reported to
// stderr.
func Open(path, actor string, stderr io.Writer) (*Dir, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, &Error{Code: CodeState, File: path}
	}
	d := &Dir{path: path, actor: actor, stderr: stderr}
	// The actor file is written under the lock, so that two first uses at once agree.
	f, err := os.OpenFile(d.file(FileLock), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, &Error{Code: CodeState, File: FileLock}
	}
	defer f.Close()
	if err := flock(f); err != nil {
		return nil, &Error{Code: CodeState, File: FileLock}
	}
	b, err := readSmall(d.file(FileActor))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := writeFile(d.file(FileActor), []byte(actor+"\n")); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, readErr(FileActor, err)
	case string(b) != actor+"\n":
		return nil, &Error{Code: CodeActor, File: FileActor}
	}
	return d, nil
}

// Read opens the state directory at path to read it, as whatever actor its actor file names. It
// writes nothing.
func Read(path string) (*Dir, error) {
	b, err := readSmall(filepath.Join(path, FileActor))
	if err != nil {
		return nil, readErr(FileActor, err)
	}
	if len(b) < 2 || b[len(b)-1] != '\n' || bytes.ContainsAny(b[:len(b)-1], "\n\r") {
		return nil, &Error{Code: CodeState, File: FileActor}
	}
	return &Dir{path: path, actor: string(b[:len(b)-1]), stderr: io.Discard}, nil
}

// Actor returns the actor reference the directory belongs to.
func (d *Dir) Actor() string { return d.actor }

// Path returns the directory's path.
func (d *Dir) Path() string { return d.path }

func (d *Dir) file(name string) string { return filepath.Join(d.path, name) }

// Lock takes the directory's exclusive lock, waiting for another command to let it go (state-lock),
// and then removes an unterminated last line from each .jsonl file, with one line on the error
// writer for each.
func (d *Dir) Lock() error {
	if d.lock != nil {
		return nil
	}
	f, err := os.OpenFile(d.file(FileLock), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return &Error{Code: CodeState, File: FileLock}
	}
	if err := flock(f); err != nil {
		_ = f.Close()
		return &Error{Code: CodeState, File: FileLock}
	}
	d.lock = f
	for _, name := range []string{FileEnvelopes, FileDelivered, FileKept, FileSalts} {
		if err := d.repair(name); err != nil {
			d.Unlock()
			return err
		}
	}
	return nil
}

// flock takes the exclusive lock of f, waiting for it.
func flock(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// Unlock lets the lock go.
func (d *Dir) Unlock() {
	if d.lock != nil {
		_ = d.lock.Close()
		d.lock = nil
	}
}

// repair cuts an unterminated last line from the file name, if it has one.
func (d *Dir) repair(name string) error {
	f, err := os.OpenFile(d.file(name), os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return &Error{Code: CodeState, File: name}
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return &Error{Code: CodeState, File: name}
	}
	if len(b) == 0 || b[len(b)-1] == '\n' {
		return nil
	}
	keep := bytes.LastIndexByte(b, '\n') + 1
	if err := f.Truncate(int64(keep)); err != nil || f.Sync() != nil {
		return &Error{Code: CodeState, File: name}
	}
	n := bytes.Count(b[:keep], []byte{'\n'}) + 1
	fmt.Fprintf(d.stderr, "state: %s line %d had no line feed and was never written; removed\n", name, n)
	return nil
}

// Seq returns the last envelope seq the mediator took from this actor, 0 when none.
func (d *Dir) Seq() (int64, error) {
	b, err := readSmall(d.file(FileSeq))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, readErr(FileSeq, err)
	}
	n, ok := ParseSeq(b)
	if !ok {
		return 0, &Error{Code: CodeState, File: FileSeq}
	}
	return n, nil
}

// Next returns the seq for the next envelope: one above the larger of the seq file and the highest
// seq in envelopes.jsonl. An envelope line stays when its call got no reply, and the mediator may
// have taken that seq even though the seq file never heard of it; a seq may skip numbers, so the
// next give never reuses one (crossing-4).
func (d *Dir) Next() (int64, error) {
	last, err := d.Seq()
	if err != nil {
		return 0, err
	}
	envs, err := d.Envelopes()
	if err != nil {
		return 0, err
	}
	for _, e := range envs {
		last = max(last, e.Seq)
	}
	return last + 1, nil
}

// ParseSeq reads a seq file's bytes: a decimal integer of at least 1 with no leading zero, and a
// line feed.
func ParseSeq(b []byte) (int64, bool) {
	if len(b) < 2 || b[len(b)-1] != '\n' || b[0] == '0' {
		return 0, false
	}
	for _, c := range b[:len(b)-1] {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(string(b[:len(b)-1]), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// SetSeq records n as the last envelope seq the mediator took.
func (d *Dir) SetSeq(n int64) error {
	return writeFile(d.file(FileSeq), []byte(strconv.FormatInt(n, 10)+"\n"))
}

// envelopeLine is one line of envelopes.jsonl.
func envelopeLine(seq int64, hash string) []byte {
	b, _ := json.Marshal(map[string]any{"seq": seq, "hash": hash})
	c, _ := canonical.Form(b)
	return c
}

// AddEnvelope records an envelope the actor signed, before its call is sent.
func (d *Dir) AddEnvelope(seq int64, hash string) error {
	return d.appendLine(FileEnvelopes, envelopeLine(seq, hash))
}

// DropEnvelope removes the line of an envelope the mediator rejected: it never crossed, and its seq
// is used again.
func (d *Dir) DropEnvelope(seq int64, hash string) error {
	lines, err := d.lines(FileEnvelopes)
	if err != nil {
		return err
	}
	want := envelopeLine(seq, hash)
	var out []byte
	dropped := false
	for _, l := range lines {
		if !dropped && bytes.Equal(l, want) {
			dropped = true
			continue
		}
		out = append(append(out, l...), '\n')
	}
	if !dropped {
		return nil
	}
	return writeFile(d.file(FileEnvelopes), out)
}

// ParseEnvelope reads one line of envelopes.jsonl: {"hash","seq"} in canonical form, seq at least
// 1 and hash a typed hash.
func ParseEnvelope(line []byte) (int64, string, bool) {
	c, err := canonical.Form(line)
	if err != nil || !bytes.Equal(c, line) {
		return 0, "", false
	}
	var e struct {
		Seq  *int64  `json:"seq"`
		Hash *string `json:"hash"`
	}
	if !strict.Decode(line, &e) || e.Seq == nil || e.Hash == nil || *e.Seq < 1 || !typedHash(*e.Hash) {
		return 0, "", false
	}
	return *e.Seq, *e.Hash, true
}

func typedHash(s string) bool {
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

// Envelopes returns the envelopes the actor signed, in the order signed, for reconciliation
// (receipt-7).
func (d *Dir) Envelopes() ([]receipts.Envelope, error) {
	lines, err := d.lines(FileEnvelopes)
	if err != nil {
		return nil, err
	}
	out := []receipts.Envelope{}
	for _, l := range lines {
		seq, hash, ok := ParseEnvelope(l)
		if !ok {
			return nil, &Error{Code: CodeState, File: FileEnvelopes}
		}
		out = append(out, receipts.Envelope{Sender: d.actor, Seq: seq, Hash: hash})
	}
	return out, nil
}

// AddDelivered records the receipt of a delivery the actor fetched.
func (d *Dir) AddDelivered(receipt []byte) error {
	c, err := canonical.Form(receipt)
	if err != nil {
		return &Error{Code: CodeState, File: FileDelivered}
	}
	return d.appendLine(FileDelivered, c)
}

// Delivered returns the receipts of the deliveries the actor fetched, in the order fetched.
func (d *Dir) Delivered() ([][]byte, error) { return d.lines(FileDelivered) }

// Keep records a part the actor keeps, one line of kept.jsonl.
func (d *Dir) Keep(line []byte) error {
	c, err := canonical.Form(line)
	if err != nil {
		return &Error{Code: CodeState, File: FileKept}
	}
	return d.appendLine(FileKept, c)
}

// Kept returns the parts the actor kept, one line each, in the order kept.
func (d *Dir) Kept() ([][]byte, error) { return d.lines(FileKept) }

// A Salt is one line of salts.jsonl: the salt, in hex, that the delivery fetched under the receipt
// id came with.
type Salt struct {
	ReceiptID string `json:"receiptId"`
	Salt      string `json:"salt"`
}

// Format prints the Salt as a fixed text, whatever the verb: it holds a salt (secrets-never-print).
func (Salt) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("state.Salt")) }

// GoString is the fixed text, for %#v.
func (Salt) GoString() string { return "state.Salt" }

// String is the fixed text.
func (Salt) String() string { return "state.Salt" }

// AddSalt records the salt of a delivery the actor fetched, after its receipt.
func (d *Dir) AddSalt(receiptID, salt string) error {
	b, _ := json.Marshal(Salt{ReceiptID: receiptID, Salt: salt})
	c, err := canonical.Form(b)
	if err != nil {
		return &Error{Code: CodeState, File: FileSalts}
	}
	return d.appendLine(FileSalts, c)
}

// Salts returns the salts of the deliveries the actor fetched, in the order fetched; a line that is
// not an object of exactly the two strings is error state.
func (d *Dir) Salts() ([]Salt, error) {
	lines, err := d.lines(FileSalts)
	if err != nil {
		return nil, err
	}
	out := []Salt{}
	for _, l := range lines {
		var s struct {
			ReceiptID *string `json:"receiptId"`
			Salt      *string `json:"salt"`
		}
		if !strict.Decode(l, &s) || s.ReceiptID == nil || s.Salt == nil {
			return nil, &Error{Code: CodeState, File: FileSalts}
		}
		out = append(out, Salt{ReceiptID: *s.ReceiptID, Salt: *s.Salt})
	}
	return out, nil
}

// KeepClose keeps the Accord's close receipt in close.json (close-3, docs/adr/0022-the-close.md): it
// makes the file, mode 0600, synced with the directory, when it is absent, and leaves it as it is
// when it holds the same receipt. A file that holds anything else is CodeState: one Accord has one
// close, and a second that differs is not written over the first.
func (d *Dir) KeepClose(receipt []byte) error {
	want := append(append([]byte(nil), receipt...), '\n')
	if bytes.ContainsAny(receipt, "\n") {
		return &Error{Code: CodeState, File: FileClose}
	}
	f, err := os.OpenFile(d.file(FileClose), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		got, ok, err := lines.ReadAll(d.file(FileClose), len(want))
		if err != nil || !ok || !bytes.Equal(got, want) {
			return &Error{Code: CodeState, File: FileClose}
		}
		return nil
	}
	if err != nil {
		return &Error{Code: CodeState, File: FileClose}
	}
	_, werr := f.Write(want)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil || d.sync() != nil {
		return &Error{Code: CodeState, File: FileClose}
	}
	return nil
}

// appendLine appends one whole line to the file name and syncs it; a line that makes the file
// syncs the directory too, so the new name survives a crash as surely as a later line does.
func (d *Dir) appendLine(name string, line []byte) error {
	if bytes.ContainsAny(line, "\n") {
		return &Error{Code: CodeState, File: name}
	}
	made := true
	f, err := os.OpenFile(d.file(name), os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		made = false
		f, err = os.OpenFile(d.file(name), os.O_WRONLY|os.O_APPEND, 0)
	}
	if err != nil {
		return &Error{Code: CodeState, File: name}
	}
	_, werr := f.Write(append(append([]byte(nil), line...), '\n'))
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil || (made && d.sync() != nil) {
		return &Error{Code: CodeState, File: name}
	}
	return nil
}

// lines returns the file's whole lines, leaving out an unterminated last one; a missing file has
// none.
func (d *Dir) lines(name string) ([][]byte, error) {
	f, err := lines.Read(d.file(name), maxLine)
	if errors.Is(err, fs.ErrNotExist) {
		return [][]byte{}, nil
	}
	if err != nil {
		return nil, readErr(name, err)
	}
	if f.Long > 0 {
		return nil, &Error{Code: CodeState, File: name}
	}
	if f.Lines == nil {
		return [][]byte{}, nil
	}
	return f.Lines, nil
}

// maxLine is the longest line of a state directory's file that is read: local.MaxReply, the most a
// window replies with, which holds any delivery and its answer call.
const maxLine = 4 << 20

// maxSmall is the most read of the actor and seq files, each one short line.
const maxSmall = 4 << 10

// readErr is CodeState for the file name, whose read failed with err. Where the file is not a regular
// file, errors.Is also finds lines.ErrNotRegular in it.
func readErr(name string, err error) error {
	e := &Error{Code: CodeState, File: name}
	if errors.Is(err, lines.ErrNotRegular) {
		return lines.NotRegular(e)
	}
	return e
}

// readSmall reads the actor or seq file at path, which must be a regular file of at most maxSmall
// bytes.
func readSmall(path string) ([]byte, error) {
	b, ok, err := lines.ReadAll(path, maxSmall)
	if err == nil && !ok {
		err = lines.ErrNotRegular
	}
	return b, err
}

// Lines splits b into its whole lines, without their line feeds, leaving out an unterminated last
// line.
func Lines(b []byte) [][]byte {
	out := [][]byte{}
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			return out
		}
		out = append(out, b[:i:i])
		b = b[i+1:]
	}
}

// writeFile replaces the file at path with b, mode 0600, through a file beside it, synced.
func writeFile(path string, b []byte) error {
	name := filepath.Base(path)
	f, err := os.CreateTemp(filepath.Dir(path), "."+name+".*")
	if err != nil {
		return &Error{Code: CodeState, File: name}
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil || os.Rename(tmp, path) != nil {
		_ = os.Remove(tmp)
		return &Error{Code: CodeState, File: name}
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
