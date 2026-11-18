package client

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/canonical"
)

// ConfigFile is the labels config's name in placket's config directory.
const ConfigFile = "mediators.json"

// MaxConfig is the most bytes of a labels config read.
const MaxConfig = 1 << 20

// ConfigDir is placket's config directory under the environment getenv reads: $XDG_CONFIG_HOME/placket,
// or else ~/.config/placket, with ~ the HOME it reads; empty when neither is set.
func ConfigDir(getenv func(string) string) string {
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "placket")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "placket")
	}
	return ""
}

// A Config is the labels config: the mediators a user reaches, in the order the file lists them.
// A label is a local, human name for one, and is never signed.
type Config struct {
	Mediators []Mediator
}

type configFile struct {
	Mediators *[]configEntry `json:"mediators"`
}

type configEntry struct {
	Accord   json.RawMessage `json:"accord"`
	Key      json.RawMessage `json:"key"`
	Founding json.RawMessage `json:"founding"`
	Socket   json.RawMessage `json:"socket"`
	Label    json.RawMessage `json:"label"`
}

// member reads an entry's member as a string: whether it is there, and whether it is a string, which
// null is not.
func member(raw json.RawMessage) (s string, there, ok bool) {
	if len(raw) == 0 {
		return "", false, true
	}
	if raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", true, false
	}
	return s, true, true
}

// ReadConfig reads the labels config of the config directory getenv names. A missing file, or no
// config directory, is an empty list; a file that cannot be read is CodeBadConfig.
func ReadConfig(getenv func(string) string) (Config, error) {
	dir := ConfigDir(getenv)
	if dir == "" {
		return Config{}, nil
	}
	f, err := os.Open(filepath.Join(dir, ConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fail(CodeBadConfig)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxConfig+1))
	if err != nil || len(b) > MaxConfig {
		return Config{}, fail(CodeBadConfig)
	}
	return DecodeConfig(b, dir, getenv)
}

// DecodeConfig reads the bytes of a labels config whose relative paths are under dir, the defaults
// under the environment getenv reads, and checks it (api/config/mediators.json): an object whose one
// member, mediators, lists entries, each with an Accord id, a key file and a founding set's path, and
// optionally a socket and a label of 1 to 63 lowercase letters, digits and hyphens, starting with a
// letter or a digit. Any other is CodeBadConfig, wrapping nothing. A relative path is taken under
// dir, and every path is cleaned; an entry with no socket has the Accord's default one
// (local.DefaultSocketWith), and its state directory is the command line's,
// state.DefaultPath(key, accord).
//
// Then it checks the entries against each other, in this order, and refuses the first check that
// fails, naming its two entries: two with one label, CodeLabelTaken; two of different Accords on one
// socket, two on one state directory, and two of one Accord both unlabelled, CodeMediatorCollision.
// Where several pairs fail one check, the pair (i, j), i < j, first by i and then by j, is named.
// Nothing else about an entry is compared with another's.
func DecodeConfig(b []byte, dir string, getenv func(string) string) (Config, error) {
	var f configFile
	if !strict.Decode(b, &f) || f.Mediators == nil {
		return Config{}, fail(CodeBadConfig)
	}
	path := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		return filepath.Clean(p)
	}
	var c Config
	for _, e := range *f.Mediators {
		accord, hasAccord, ok1 := member(e.Accord)
		key, hasKey, ok2 := member(e.Key)
		founding, hasFounding, ok3 := member(e.Founding)
		socket, hasSocket, ok4 := member(e.Socket)
		label, hasLabel, ok5 := member(e.Label)
		if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !hasAccord || !AccordID(accord) || !hasKey || key == "" ||
			!hasFounding || founding == "" || (hasSocket && socket == "") || (hasLabel && !Label(label)) {
			return Config{}, fail(CodeBadConfig)
		}
		m := Mediator{Accord: accord, Key: path(key), Founding: path(founding), Label: label}
		if hasSocket {
			m.Socket = path(socket)
		} else {
			m.Socket = filepath.Clean(local.DefaultSocketWith(getenv, m.Accord))
		}
		m.State = filepath.Clean(state.DefaultPath(m.Key, m.Accord))
		c.Mediators = append(c.Mediators, m)
	}
	checks := []struct {
		code  Code
		clash func(a, b Mediator) bool
	}{
		{CodeLabelTaken, func(a, b Mediator) bool { return a.Label != "" && a.Label == b.Label }},
		{CodeMediatorCollision, func(a, b Mediator) bool { return a.Socket == b.Socket && a.Accord != b.Accord }},
		{CodeMediatorCollision, func(a, b Mediator) bool { return a.State == b.State }},
		{CodeMediatorCollision, func(a, b Mediator) bool { return a.Label == "" && b.Label == "" && a.Accord == b.Accord }},
	}
	for _, k := range checks {
		for i := range c.Mediators {
			for j := i + 1; j < len(c.Mediators); j++ {
				if k.clash(c.Mediators[i], c.Mediators[j]) {
					return Config{}, &Error{Code: k.code, Entries: []int{i, j}}
				}
			}
		}
	}
	return c, nil
}

// Find returns the entry name names: the entry of that label, or, when name is an Accord id, the
// entry of that Accord with no label. A labelled entry is named by its label only. Any other name is
// CodeUnknownMediator.
func (c Config) Find(name string) (Mediator, error) {
	for _, m := range c.Mediators {
		if m.Label != "" && m.Label == name || m.Label == "" && m.Accord == name {
			return m, nil
		}
	}
	return Mediator{}, fail(CodeUnknownMediator)
}

// Label reports whether s is a label: 1 to 63 of a-z, 0-9 and -, starting with a letter or a digit.
// So a label is safe in a shell, and never looks like an Accord id.
func Label(s string) bool {
	if s == "" || len(s) > 63 || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// AccordID reports whether s is an Accord id: a typed hash, the prefix and 64 lowercase hex digits.
func AccordID(s string) bool {
	h, ok := strings.CutPrefix(s, canonical.HashPrefix)
	if !ok || len(h) != 64 {
		return false
	}
	for i := 0; i < len(h); i++ {
		if c := h[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
