package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/internal/lines"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/setup"
	"github.com/orilloai/placket/core/signing"
)

func init() {
	commands["seal"] = sealCommand
}

// codeSeal is seal's own error: the declaration is not one the key signed, or its opening could not
// be written.
const codeSeal code = "seal"

// OpeningSuffix ends the name of an opening's file: <the hex of the actor reference>.opening.json
// (setup-contract-13).
const OpeningSuffix = ".opening.json"

// OpeningName is the file name of the opening of actor's seal, the hex of its reference and
// OpeningSuffix.
func OpeningName(actor string) string {
	return strings.TrimPrefix(actor, signing.ReferencePrefix) + OpeningSuffix
}

// sealCommand is `seal --key K --declaration D [--out DIR]` (setup-contract-12, setup-contract-13):
// it seals the key's own signed declaration under a salt of setup.SaltBytes fresh random bytes,
// prints the seal entry, {"actor","seal"}, on standard output, for founding's --seal, and writes the
// opening, {"declaration","salt"}, to DIR/<hex of the actor reference>.opening.json, mode 0600, never
// replacing a file there. DIR is the current directory when --out is not given. The opening goes to
// whoever runs the window, and to no other party. A declaration that is not one object whose actor is
// the key's, signed by it, is tool error seal; an opening that cannot be written is tool error file
// opening. On standard error it prints the opening's path, never its bytes.
func sealCommand(r *run, args []string) int {
	fs := newFlags("seal")
	keyPath, declPath := fs.text("key"), fs.text("declaration")
	out := fs.text("out")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	k, err := keyfile.Read(*keyPath)
	if err != nil {
		return r.failed(err)
	}
	decl, ok := readJSON(*declPath)
	if !ok {
		return r.tool(codeFile, "declaration")
	}
	if memberString(decl, "actor") != k.Reference() || !signing.Verify(decl, signing.TypeDeclaration, k.Reference()) {
		return r.tool(codeSeal)
	}
	salt := make([]byte, setup.SaltBytes)
	if _, err := io.ReadFull(r.env.Rand, salt); err != nil {
		return r.tool(codeInternal)
	}
	entry, opening, err := setup.Seal(decl, salt)
	if err != nil {
		return r.tool(codeSeal)
	}
	path := filepath.Join(*out, OpeningName(k.Reference()))
	if err := writeNew(path, append(opening, '\n')); err != nil {
		return r.tool(codeFile, "opening")
	}
	line(r.stdout, entry)
	fmt.Fprintln(r.stderr, "opening "+path)
	r.put("seal", json.RawMessage(entry))
	r.put("opening", path)
	return ExitOK
}

// writeNew writes b to a new file at path, mode 0600, synced; a file already there is refused, and
// a write that fails leaves no file.
func writeNew(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	serr := f.Sync()
	cerr := f.Close()
	if werr != nil || serr != nil || cerr != nil {
		_ = os.Remove(path)
		return errors.Join(werr, serr, cerr)
	}
	return nil
}

// errOpening is an opening the window does not read: one that others may read or write, that
// another user owns, or that is over local.MaxCall.
var errOpening = errors.New("cli: opening")

// readOpenings reads, for each actor, the opening dir holds for it, DIR/<hex>.opening.json, its one
// trailing line feed removed. An actor that is not an actor reference names no file inside dir, so
// it is left out, as is a file that is missing, and founding refuses the set (row 4 or
// opening-missing). An opening is private to its actor (setup-contract-13), so it is read as a key
// file is: a file that is not regular, that others may read or write, or that another user owns, is
// tool error file opening, and so is one over local.MaxCall, read no further than one byte past it.
func readOpenings(dir string, actors []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, a := range actors {
		if _, ok := signing.ParseReference(a); !ok {
			continue
		}
		b, err := readOpening(filepath.Join(dir, OpeningName(a)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[a] = trimLF(b)
	}
	return out, nil
}

// readOpening reads the opening at path, as readOpenings checks it.
func readOpening(path string) ([]byte, error) {
	f, err := lines.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !lines.Private(fi, os.Getuid()) {
		return nil, errOpening
	}
	b, err := io.ReadAll(io.LimitReader(f, local.MaxCall+1))
	if err != nil {
		return nil, err
	}
	if len(b) > local.MaxCall {
		return nil, errOpening
	}
	return b, nil
}

// trimLF removes one trailing line feed, as the commands write one after each JSON value.
func trimLF(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		return b[:n-1]
	}
	return b
}

// sealedActors are the actors of the set's seal entries, as the set lists them; nil for a set that
// does not read.
func sealedActors(set []byte) []string {
	var s struct {
		Declarations []struct {
			Actor string `json:"actor"`
		} `json:"declarations"`
	}
	if json.Unmarshal(set, &s) != nil {
		return nil
	}
	out := make([]string, 0, len(s.Declarations))
	for _, d := range s.Declarations {
		out = append(out, d.Actor)
	}
	return out
}

// openingActor is the actor of an opening's declaration, as the file holds it, for verify's --open.
func openingActor(b []byte) string {
	var o struct {
		Declaration struct {
			Actor string `json:"actor"`
		} `json:"declaration"`
	}
	if json.Unmarshal(b, &o) != nil {
		return ""
	}
	return o.Declaration.Actor
}
