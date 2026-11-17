// Package kit is what the vector generators share: a writer of canonical bytes for the values a
// generator builds, keys from names, salts from labels, typed hashes, the pre-authentication
// encoding, signatures, receipts, founding sets, messages and envelopes, and the vector files
// themselves.
//
// It is written from the protocol text with the Go standard library alone, and imports no package of
// the core: a vector's bytes must not come from the code the vector tests (vectors-independent in
// docs/rules/tests.md). Codes, offsets and verdicts are never computed here; each generator writes
// them by hand.
package kit

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Obj is a JSON object a generator builds.
type Obj map[string]any

// Raw is a JSON value already written as canonical bytes.
type Raw []byte

// Canon writes v in the canonical form of canonical-1: objects sorted by the UTF-16 code units of
// their names, no whitespace, integers in plain decimal, strings with exactly the escapes the form
// allows. It takes Obj, map[string]any, []any, []string, string, int, int64, bool, nil and Raw.
func Canon(v any) []byte {
	var b bytes.Buffer
	write(&b, v)
	return b.Bytes()
}

func write(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(x))
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case string:
		writeString(b, x)
	case Raw:
		b.Write(x)
	case []string:
		b.WriteByte('[')
		for i, s := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, s)
		}
		b.WriteByte(']')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			write(b, e)
		}
		b.WriteByte(']')
	case Obj:
		writeObject(b, x)
	case map[string]any:
		writeObject(b, x)
	default:
		panic(fmt.Sprintf("kit: no canonical form for %T", v))
	}
}

func writeObject(b *bytes.Buffer, o map[string]any) {
	names := make([]string, 0, len(o))
	for k := range o {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return Less16(names[i], names[j]) })
	b.WriteByte('{')
	for i, k := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		writeString(b, k)
		b.WriteByte(':')
		write(b, o[k])
	}
	b.WriteByte('}')
}

// Less16 orders two strings by their UTF-16 code units.
func Less16(a, b string) bool {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}

func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if c < 0x20 {
				fmt.Fprintf(b, `\u%04x`, c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte('"')
}

// Hash is the typed hash of canonical bytes: sha256: and 64 lowercase hex (canonical-3).
func Hash(canonicalBytes []byte) string {
	s := sha256.Sum256(canonicalBytes)
	return "sha256:" + hex.EncodeToString(s[:])
}

// HashOf is the typed hash of a value's canonical bytes.
func HashOf(v any) string { return Hash(Canon(v)) }

// Sum is the lowercase hex SHA-256 of the bytes given, one after the other.
func Sum(parts ...[]byte) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// A Key is a named actor's key, its seed derived from its name so that every run writes the same
// vectors.
type Key struct {
	Name    string
	Seed    string // 64 lowercase hex
	Ref     string // ed25519: and the public key in hex
	private ed25519.PrivateKey
}

// NewKey derives the key of name: its seed is the SHA-256 of "placket vectors: key " and the name.
func NewKey(name string) Key {
	seed := sha256.Sum256([]byte("placket vectors: key " + name))
	p := ed25519.NewKeyFromSeed(seed[:])
	return Key{
		Name: name, Seed: hex.EncodeToString(seed[:]),
		Ref:     "ed25519:" + hex.EncodeToString(p.Public().(ed25519.PublicKey)),
		private: p,
	}
}

// Sign signs message with the key, raw Ed25519 (RFC 8032).
func (k Key) Sign(message []byte) []byte { return ed25519.Sign(k.private, message) }

// Salt derives a salt of n bytes from a label, in lowercase hex.
func Salt(label string, n int) string {
	var out []byte
	for i := 0; len(out) < n; i++ {
		s := sha256.Sum256([]byte("placket vectors: salt " + label + " " + strconv.Itoa(i)))
		out = append(out, s[:]...)
	}
	return hex.EncodeToString(out[:n])
}

// PAE is DSSE 1.0's pre-authentication encoding (signing-2).
func PAE(t string, payload []byte) []byte {
	return []byte("DSSEv1 " + strconv.Itoa(len(t)) + " " + t + " " + strconv.Itoa(len(payload)) + " " + string(payload))
}

// Root is the host every placket URI is under, and Spec the one protocol number each carries: the
// card's spec (setup-contract-4).
const (
	Root = "https://placket.tech/"
	Spec = "0.1"
)

// The type URIs of signing-4, and the other fixed strings of the formats.
const (
	TypeDeclaration = Root + "declaration/" + Spec
	TypeBinding     = Root + "binding/" + Spec
	TypeEnvelope    = Root + "envelope/" + Spec
	Extension       = Root + "crossing/" + Spec
	PayloadType     = "application/vnd.in-toto+json"
	StatementType   = "https://in-toto.io/Statement/v1"
	PredicateType   = Root + "receipt/" + Spec
)

// Unsigned returns o without its sig member.
func Unsigned(o Obj) Obj {
	c := Obj{}
	for k, v := range o {
		if k != "sig" {
			c[k] = v
		}
	}
	return c
}

// SigningBytes are the signing bytes of o under t: the pre-authentication encoding of t and the
// canonical bytes of o without its sig (signing-2).
func SigningBytes(o Obj, t string) []byte { return PAE(t, Canon(Unsigned(o))) }

// Signed returns o with sig, the key's signature of its signing bytes under t, in hex.
func Signed(o Obj, t string, k Key) Obj {
	c := Unsigned(o)
	c["sig"] = hex.EncodeToString(k.Sign(SigningBytes(o, t)))
	return c
}

// B64 is standard base64 with padding.
func B64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// FromB64 decodes standard base64.
func FromB64(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// HexOf is lowercase hex.
func HexOf(b []byte) string { return hex.EncodeToString(b) }

// FromHex decodes hex.
func FromHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Copy returns a deep copy of a value a generator built.
func Copy(v any) any {
	switch x := v.(type) {
	case Obj:
		c := Obj{}
		for k, e := range x {
			c[k] = Copy(e)
		}
		return c
	case map[string]any:
		c := Obj{}
		for k, e := range x {
			c[k] = Copy(e)
		}
		return c
	case []any:
		c := make([]any, len(x))
		for i, e := range x {
			c[i] = Copy(e)
		}
		return c
	case []string:
		return append([]string(nil), x...)
	case Raw:
		return append(Raw(nil), x...)
	}
	return v
}

// Pretty writes a vector file: the value with two-space indentation, strings with the canonical
// form's escapes, objects sorted, and a final line feed.
func Pretty(v any) []byte {
	var b bytes.Buffer
	pretty(&b, v, "")
	b.WriteByte('\n')
	return b.Bytes()
}

func pretty(b *bytes.Buffer, v any, indent string) {
	in := indent + "  "
	switch x := v.(type) {
	case Obj:
		prettyObject(b, x, indent)
	case map[string]any:
		prettyObject(b, x, indent)
	case []any:
		if len(x) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, e := range x {
			b.WriteString(in)
			pretty(b, e, in)
			if i < len(x)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString(indent + "]")
	case []string:
		l := make([]any, len(x))
		for i, s := range x {
			l[i] = s
		}
		pretty(b, l, indent)
	default:
		write(b, v)
	}
}

func prettyObject(b *bytes.Buffer, o map[string]any, indent string) {
	if len(o) == 0 {
		b.WriteString("{}")
		return
	}
	in := indent + "  "
	names := make([]string, 0, len(o))
	for k := range o {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return Less16(names[i], names[j]) })
	b.WriteString("{\n")
	for i, k := range names {
		b.WriteString(in)
		writeString(b, k)
		b.WriteString(": ")
		pretty(b, o[k], in)
		if i < len(names)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent + "}")
}

// A Vector is one vector file: its name under vectors/, what it tests, its operation and its other
// members.
type Vector struct {
	Name, Tests, Operation string
	Members                Obj
}

// Write writes every vector of a group into dir, one file each, and removes the group's other vector
// files, so a vector the generator no longer writes cannot linger. It refuses a vector named outside
// the group and two vectors of one name.
func Write(dir, group string, vs []Vector) error {
	seen := map[string]bool{}
	for _, v := range vs {
		g, name, ok := strings.Cut(v.Name, "/")
		if !ok || g != group || name == "" {
			return fmt.Errorf("vector %q is not in the group %s", v.Name, group)
		}
		if seen[v.Name] {
			return fmt.Errorf("vector %q is written twice", v.Name)
		}
		if strings.Contains(v.Tests, "\n") || strings.TrimSpace(v.Tests) == "" {
			return fmt.Errorf("vector %q: its tests line is not one line", v.Name)
		}
		seen[v.Name] = true
	}
	old, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, f := range old {
		if !seen[group+"/"+strings.TrimSuffix(filepath.Base(f), ".json")] {
			if err := os.Remove(f); err != nil {
				return err
			}
		}
	}
	for _, v := range vs {
		o := Obj{"vector": v.Name, "spec": Spec, "tests": v.Tests, "operation": v.Operation}
		for k, m := range v.Members {
			o[k] = m
		}
		_, name, _ := strings.Cut(v.Name, "/")
		if err := os.WriteFile(filepath.Join(dir, name+".json"), Pretty(o), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Main is a generator's main: it writes the group's vectors into the directory named by its one
// argument, or into the group's directory under the working directory, vectors/.
func Main(group string, vs []Vector) {
	dir := group
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := Write(dir, group, vs); err != nil {
		fmt.Fprintln(os.Stderr, group+": "+err.Error())
		os.Exit(1)
	}
}

// Parse reads JSON bytes into the values Canon writes: Obj, []any, string, int64, bool and nil. A
// number that is not a whole number in the safe range is refused.
func Parse(b []byte) any {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		panic(err)
	}
	return convert(v)
}

func convert(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := Obj{}
		for k, e := range x {
			o[k] = convert(e)
		}
		return o
	case []any:
		for i, e := range x {
			x[i] = convert(e)
		}
		return x
	case json.Number:
		n, err := strconv.ParseInt(string(x), 10, 64)
		if err != nil {
			panic(err)
		}
		return n
	}
	return v
}
