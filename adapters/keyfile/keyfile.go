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

// Read reads the key file at path.
func Read(path string) (signing.Key, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return signing.Key{}, &Error{Code: CodeRead}
	}
	defer clear(b)
	return Parse(b)
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
// existing file is never overwritten.
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
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return signing.Key{}, &Error{Code: CodeExists}
		}
		return signing.Key{}, &Error{Code: CodeWrite}
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return signing.Key{}, &Error{Code: CodeWrite}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return signing.Key{}, &Error{Code: CodeWrite}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return signing.Key{}, &Error{Code: CodeWrite}
	}
	return k, nil
}
