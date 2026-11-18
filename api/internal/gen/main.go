// Command gen writes api/testdata/worked/: a founding set of fixed keys, and one worked signed call
// per op of the mediator API, each with the key, the clock and the nonce it fixes, for a give the
// message id, the salt and the envelope's seq, and for a resolve the answer's salt and the nested
// answer call's signature, and the call's expected canonical bytes and signature. It uses the
// standard library alone and imports nothing of placket, so the expected values never come from the
// code they test. Its inputs are chosen so that the standard library's encoding is the canonical
// form: ASCII strings without <, > or &, and whole numbers, with keys sorted by encoding/json and
// HTML escaping off.
//
// Run from the repository's root: go run ./api/internal/gen [-dir api/testdata/worked].
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// The protocol's names, written here, not imported: this program must not read them from the code
// the worked calls test.
const (
	typeDeclaration = "https://placket.tech/declaration/0.1"
	typeBinding     = "https://placket.tech/binding/0.1"
	typeEnvelope    = "https://placket.tech/envelope/0.1"
	typeAnswer      = "https://placket.tech/answer/0.1"
	callPayload     = "application/vnd.placket.call+json"
	extension       = "https://placket.tech/crossing/0.1"
	roleUser        = "ROLE_USER"
	at              = "2026-11-18T09:00:00Z"
)

// encode is the canonical bytes of v: encoding/json's, keys sorted, HTML escaping off, no line feed.
func encode(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

// pae is DSSE 1.0's pre-authentication encoding.
func pae(typ string, payload []byte) []byte {
	return []byte("DSSEv1 " + strconv.Itoa(len(typ)) + " " + typ + " " + strconv.Itoa(len(payload)) + " " + string(payload))
}

// A party is a fixed key: its seed, from its name, and its actor reference.
type party struct {
	seed []byte
	key  ed25519.PrivateKey
}

func newParty(name string) party {
	s := sha256.Sum256([]byte("placket api worked: key " + name))
	return party{seed: s[:], key: ed25519.NewKeyFromSeed(s[:])}
}

func (p party) ref() string {
	return "ed25519:" + hex.EncodeToString(p.key.Public().(ed25519.PublicKey))
}

// signObject signs an object under its type URI: the hex signature over the PAE of its canonical
// bytes, sig aside, put in its sig member.
func (p party) signObject(o map[string]any, typ string) map[string]any {
	c := map[string]any{}
	for k, v := range o {
		c[k] = v
	}
	c["sig"] = hex.EncodeToString(ed25519.Sign(p.key, pae(typ, encode(o))))
	return c
}

// fixed is n bytes drawn from a name, the same each run.
func fixed(name string, n int) []byte {
	var out []byte
	for i := 0; len(out) < n; i++ {
		s := sha256.Sum256([]byte("placket api worked: " + name + " " + strconv.Itoa(i)))
		out = append(out, s[:]...)
	}
	return out[:n]
}

func typedHash(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }

func main() {
	dir := flag.String("dir", filepath.Join("api", "testdata", "worked"), "the directory to write")
	flag.Parse()
	if err := write(*dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func write(dir string) error {
	names := []string{"mediator", "operator", "principal", "proposer", "applier", "decider"}
	ps := map[string]party{}
	for _, n := range names {
		ps[n] = newParty(n)
	}
	set, accord := founding(ps)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string][]byte{"founding.json": append(set, '\n')}
	keys := map[string]any{}
	for _, n := range names {
		keys[n] = map[string]any{"seed": hex.EncodeToString(ps[n].seed), "actor": ps[n].ref()}
	}
	files["keys.json"] = indent(map[string]any{"accord": accord, "keys": keys})
	for op, w := range worked(ps, accord) {
		files[op+".json"] = indent(w)
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// indent is v as an indented JSON file, keys sorted, HTML escaping off.
func indent(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// term is a term of a declaration.
func term(stance, dir string, with, kinds []string) map[string]any {
	sel := make([]any, len(with))
	for i, w := range with {
		sel[i] = "role:" + w
	}
	ks := make([]any, len(kinds))
	for i, k := range kinds {
		ks[i] = k
	}
	return map[string]any{"stance": stance, "dir": dir, "with": sel, "select": map[string]any{"kind": ks}}
}

// founding is the worked founding set and its Accord id: the proposer asks the decider before a
// proposal crosses to the applier, and gives the applier notes freely.
func founding(ps map[string]party) ([]byte, string) {
	maybe := term("maybe", "give", []string{"applier"}, []string{"proposal"})
	maybe["resolve"] = map[string]any{"to": []any{"role:decider"}, "timeout": 3600}
	terms := map[string][]any{
		"proposer": {maybe, term("want", "give", []string{"applier"}, []string{"note"}),
			term("want", "get", []string{"decider"}, []string{"answer", "changed-version", "comment"})},
		"applier": {term("want", "get", []string{"proposer"}, []string{"proposal", "note"}),
			term("want", "get", []string{"decider"}, []string{"changed-version"})},
		"decider": {term("want", "give", []string{"proposer"}, []string{"answer", "changed-version", "comment"}),
			term("want", "give", []string{"applier"}, []string{"changed-version"})},
	}
	roles := []any{"proposer", "applier", "decider"}
	policy := map[string]any{"roles": roles, "principals": []any{map[string]any{"principal": ps["principal"].ref(), "roles": roles}}}
	type entry struct {
		actor      string
		decl, bind map[string]any
	}
	var es []entry
	for _, n := range []string{"proposer", "applier", "decider"} {
		p := ps[n]
		es = append(es, entry{p.ref(),
			p.signObject(map[string]any{"actor": p.ref(), "terms": terms[n]}, typeDeclaration),
			ps["principal"].signObject(map[string]any{"actor": p.ref(), "principal": ps["principal"].ref(), "roles": []any{n},
				"policy": typedHash(encode(policy)), "mediator": ps["mediator"].ref(), "until": "2099-01-01T00:00:00Z"}, typeBinding)})
	}
	// The references are ASCII, so their byte order is the canonical form's.
	sort.Slice(es, func(i, j int) bool { return es[i].actor < es[j].actor })
	var decls, binds []any
	for _, e := range es {
		decls, binds = append(decls, e.decl), append(binds, e.bind)
	}
	set := encode(map[string]any{
		"policy":   policy,
		"mediator": map[string]any{"actor": ps["mediator"].ref(), "operator": ps["operator"].ref(), "runs": "process"},
		"card": map[string]any{"spec": "0.1", "kernel": map[string]any{"version": "0.1.0", "hash": "sha256:" + hex.EncodeToString(make([]byte, 32))},
			"judges": []any{}},
		"declarations": decls, "bindings": binds,
	})
	return set, typedHash(set)
}

// signCall is a call of op with args, as the party, at the worked time with the nonce: the call's
// canonical bytes, its signature in base64, and the line {"call","sig"}.
func signCall(p party, op, accord string, nonce []byte, args map[string]any) (map[string]any, string, []byte) {
	call := map[string]any{"op": op, "caller": p.ref(), "accord": accord, "at": at, "nonce": hex.EncodeToString(nonce), "args": args}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(p.key, pae(callPayload, encode(call))))
	line := map[string]any{"call": call, "sig": sig}
	return line, sig, encode(line)
}

// uuid is the message id of 16 fixed bytes, with the version and variant bits of a UUID version 4.
func uuid(b []byte) string {
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// worked is each op's worked call.
func worked(ps map[string]party, accord string) map[string]map[string]any {
	out := map[string]map[string]any{}
	add := func(op, who string, nonce []byte, args map[string]any, more map[string]any) {
		line, sig, b := signCall(ps[who], op, accord, nonce, args)
		w := map[string]any{"op": op, "key": who, "seed": hex.EncodeToString(ps[who].seed), "at": at,
			"nonce": hex.EncodeToString(nonce), "bytes": string(b), "sig": sig, "call": line}
		for k, v := range more {
			w[k] = v
		}
		out[op] = w
	}

	// give: a proposal, which the terms ask the decider about.
	id := uuid(fixed("message id", 16))
	salt := hex.EncodeToString(fixed("salt", 32))
	text := "Rename the column total to amount."
	msg := encode(map[string]any{"messageId": id, "role": roleUser, "extensions": []any{extension},
		"metadata": map[string]any{extension: map[string]any{"accord": accord, "to": ps["applier"].ref()}},
		"parts":    []any{map[string]any{"text": text, "metadata": map[string]any{extension: map[string]any{"kind": "proposal", "salt": salt}}}}})
	env := encode(ps["proposer"].signObject(map[string]any{"seq": 1, "hash": typedHash(msg)}, typeEnvelope))
	add("give", "proposer", fixed("nonce give", 16),
		map[string]any{"message": base64.StdEncoding.EncodeToString(msg), "envelope": base64.StdEncoding.EncodeToString(env)},
		map[string]any{"request": map[string]any{"to": ps["applier"].ref(), "kind": "proposal", "text": text},
			"messageId": id, "salt": salt, "seq": 1})

	// questions: the decider's, which the open proposal answers at once.
	add("questions", "decider", fixed("nonce questions", 16), map[string]any{"wait": 5},
		map[string]any{"request": map[string]any{"wait": 5}})

	// get: the applier's, with nothing queued, which waits a second.
	add("get", "applier", fixed("nonce get", 16), map[string]any{"wait": 1},
		map[string]any{"request": map[string]any{"wait": 1}})

	// result: the proposer's, on a question id the generator cannot know the window gives, so a
	// question of no receipt's.
	none := "sha256:" + hex.EncodeToString(fixed("no question", 32))
	add("result", "proposer", fixed("nonce result", 16), map[string]any{"question": none},
		map[string]any{"request": map[string]any{"question": none}})

	// resolve: the decider's accept of that question, its answer call signed with its own salt.
	answerSalt := fixed("answer salt", 32)
	answerText := "Yes, rename it."
	digest := sha256.Sum256(append(append([]byte(nil), answerSalt...), answerText...))
	answer := ps["decider"].signObject(map[string]any{"accord": accord, "question": none, "action": "accepted",
		"digest": hex.EncodeToString(digest[:]), "release": true, "from": ps["decider"].ref()}, typeAnswer)
	add("resolve", "decider", fixed("nonce resolve", 16),
		map[string]any{"answer": answer, "text": answerText, "salt": hex.EncodeToString(answerSalt)},
		map[string]any{"request": map[string]any{"question": none, "action": "accepted", "answer": answerText},
			"answerSalt": hex.EncodeToString(answerSalt), "answerSig": answer["sig"]})
	return out
}
