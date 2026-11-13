package keyfile_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/signing"
)

func code(err error) keyfile.Code {
	var e *keyfile.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// TestNewWritesASeedFile writes a key file from 32 known bytes: 64 lowercase hex digits and a line
// feed, mode 0600, whose reference is the seed's public key; a second write to the same path is
// key-exists and leaves the file as it was.
func TestNewWritesASeedFile(t *testing.T) {
	t.Parallel()
	seed := bytes.Repeat([]byte{0xab}, 32)
	path := filepath.Join(t.TempDir(), "k.key")
	k, err := keyfile.New(path, bytes.NewReader(seed))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("ab", 32) + "\n"; string(b) != want {
		t.Fatalf("the file holds %q", b)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	want := signing.ReferencePrefix + hex.EncodeToString(ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey))
	ref, err := keyfile.Reference(path)
	if err != nil || ref != want || k.Reference() != want {
		t.Fatalf("got %q and %q, want %q: %v", ref, k.Reference(), want, err)
	}
	if _, err := keyfile.New(path, bytes.NewReader(bytes.Repeat([]byte{1}, 32))); code(err) != keyfile.CodeExists {
		t.Fatalf("a second write: got %v", err)
	}
	if again, _ := os.ReadFile(path); !bytes.Equal(again, b) {
		t.Fatal("a second write changed the file")
	}
	if _, err := keyfile.New(filepath.Join(t.TempDir(), "short.key"), bytes.NewReader([]byte{1, 2})); code(err) != keyfile.CodeWrite {
		t.Fatalf("too few random bytes: got %v", err)
	}
	if _, err := keyfile.Read(filepath.Join(t.TempDir(), "none")); code(err) != keyfile.CodeRead {
		t.Fatalf("no file: got %v", err)
	}
}

// TestParseRefusesAnyOtherForm refuses every file that is not 64 lowercase hex digits and a line
// feed with code key, and its error names nothing of the file.
func TestParseRefusesAnyOtherForm(t *testing.T) {
	t.Parallel()
	good := strings.Repeat("0f", 32)
	for _, b := range []string{
		"", "\n", good, good + "\r\n", good + "\n\n", good[:62] + "\n", good + "00\n",
		strings.ToUpper(good) + "\n", good[:63] + "g\n", " " + good[1:] + "\n",
	} {
		_, err := keyfile.Parse([]byte(b))
		if code(err) != keyfile.CodeKey || err.Error() != "keyfile: key" {
			t.Fatalf("%q: got %v", b, err)
		}
	}
	if _, err := keyfile.Parse([]byte(good + "\n")); err != nil {
		t.Fatal(err)
	}
}

// FuzzParse reads any bytes as a key file. It never panics, and what it accepts is exactly 64
// lowercase hex digits and a line feed.
func FuzzParse(f *testing.F) {
	f.Add([]byte(strings.Repeat("0f", 32) + "\n"))
	f.Add([]byte(strings.Repeat("AB", 32) + "\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		k, err := keyfile.Parse(b)
		if err != nil {
			return
		}
		if len(b) != 65 || b[64] != '\n' || k.Reference() == "" {
			t.Fatalf("accepted %q", b)
		}
		if _, herr := hex.DecodeString(string(b[:64])); herr != nil || strings.ToLower(string(b)) != string(b) {
			t.Fatalf("accepted %q", b)
		}
	})
}
