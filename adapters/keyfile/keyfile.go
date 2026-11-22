// Package keyfile reads and writes key files: an Ed25519 seed, 32 bytes, as 64 lowercase hex digits
// and a line feed, in a file of mode 0600. A key file is a door format, not a frozen one: no receipt
// carries it and no verifier reads it.
//
// A seed never leaves this package but as a signing.Key, which prints as a fixed text
// (secrets-never-print), and no error of this package holds a byte of the file.
package keyfile

import (
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/orilloai/placket/adapters/internal/lines"
	"github.com/orilloai/placket/core/signing"
)

// Code says why a key file could not be read or written.
type Code string

// The codes of Error.
const (
	// CodeKey: the file is not one seed of 32 bytes as 64 lowercase hex digits and a line feed.
	CodeKey Code = "key"
	// CodeRead: the file could not be read.
	CodeRead Code = "key-read"
	// CodeExists: a key file is already there; it is never overwritten.
	CodeExists Code = "key-exists"
	// CodeWrite: the file could not be written.
	CodeWrite Code = "key-write"
	// CodeMode: the key file can be read or written by others (any of mode 0077 is set), or another
	// user owns it (keys-on-disk).
	CodeMode Code = "key-mode"
)

// Error is a key file that could not be read or written. It names the code only.
type Error struct{ Code Code }

func (e *Error) Error() string { return "keyfile: " + string(e.Code) }

// seedBytes is the length of an Ed25519 seed (RFC 8032).
const seedBytes = 32

// Parse reads a key file's bytes: 64 lowercase hex digits and a line feed, and nothing else.
func Parse(b []byte) (signing.Key, error) {
	if len(b) != 2*seedBytes+1 || b[2*seedBytes] != '\n' {
		return signing.Key{}, &Error{Code: CodeKey}
	}
	for _, c := range b[:2*seedBytes] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return signing.Key{}, &Error{Code: CodeKey}
		}
	}
	seed := make([]byte, seedBytes)
	if _, err := hex.Decode(seed, b[:2*seedBytes]); err != nil {
		return signing.Key{}, &Error{Code: CodeKey}
	}
	k, err := signing.NewKey(seed)
	clear(seed)
	if err != nil {
		return signing.Key{}, &Error{Code: CodeKey}
	}
	return k, nil
}

// maxFile is the most Read reads of a key file. A key file is 65 bytes; anything longer is refused
// as CodeKey without reading it all.
const maxFile = 4 << 10

// getuid is os.Getuid, a variable so a test can show the owner check.
var getuid = os.Getuid

// Read reads the key file at path. It follows a symbolic link and checks the file it reaches: a file
// that is not regular is CodeRead, and one whose mode lets others read or write it, or that another
// user owns, is CodeMode (keys-on-disk). It opens the file as lines.Open does, which checks the path
// before it opens it, so a FIFO or a device never holds it, and checks the file it opened again, so a
// swap in between is refused.
func Read(path string) (signing.Key, error) {
	f, err := lines.Open(path)
	if err != nil {
		return signing.Key{}, &Error{Code: CodeRead}
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return signing.Key{}, &Error{Code: CodeRead}
	}
	if err := private(opened); err != nil {
		return signing.Key{}, err
	}
	b, err := io.ReadAll(io.LimitReader(f, maxFile))
	defer clear(b)
	if err != nil {
		return signing.Key{}, &Error{Code: CodeRead}
	}
	return Parse(b)
}

// private refuses a file that is not regular (CodeRead), or that lines.Private refuses: one that
// others may read or write, or that another user owns (CodeMode).
func private(fi fs.FileInfo) error {
	if !fi.Mode().IsRegular() {
		return &Error{Code: CodeRead}
	}
	if !lines.Private(fi, getuid()) {
		return &Error{Code: CodeMode}
	}
	return nil
}

// Reference returns the actor reference of the key in the file at path (signing-1).
func Reference(path string) (string, error) {
	k, err := Read(path)
	if err != nil {
		return "", err
	}
	return k.Reference(), nil
}

// New writes a new key file at path, mode 0600, from 32 bytes of rand, and returns the key. An
// existing file is never overwritten (CodeExists), and a write that fails leaves no file (CodeWrite).
func New(path string, rand io.Reader) (signing.Key, error) {
	seed := make([]byte, seedBytes)
	defer clear(seed)
	if _, err := io.ReadFull(rand, seed); err != nil {
		return signing.Key{}, &Error{Code: CodeWrite}
	}
	k, err := signing.NewKey(seed)
	if err != nil {
		return signing.Key{}, &Error{Code: CodeWrite}
	}
	line := make([]byte, 2*seedBytes+1)
	defer clear(line)
	hex.Encode(line, seed)
	line[2*seedBytes] = '\n'
	if err := write(path, line); err != nil {
		return signing.Key{}, err
	}
	return k, nil
}

// write puts line in a new file at path, atomically: it writes a temporary file of mode 0600 in the
// same directory, syncs it, links it to path, removes the temporary name and syncs the directory. A
// crash leaves either no file at path or the whole key, and the link never replaces an existing file.
func write(path string, line []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".key-*")
	if err != nil {
		return &Error{Code: CodeWrite}
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return &Error{Code: CodeWrite}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return &Error{Code: CodeWrite}
	}
	if err := f.Close(); err != nil {
		return &Error{Code: CodeWrite}
	}
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return &Error{Code: CodeExists}
		}
		return &Error{Code: CodeWrite}
	}
	if err := os.Remove(tmp); err != nil {
		_ = os.Remove(path)
		return &Error{Code: CodeWrite}
	}
	if err := syncDir(dir); err != nil {
		_ = os.Remove(path)
		return &Error{Code: CodeWrite}
	}
	return nil
}

// syncDir syncs the directory dir, so the new name in it survives a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
