package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/orilloai/placket/adapters/client"
	"github.com/orilloai/placket/adapters/internal/pointer"
	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands[WordReleased] = releasedCommand
}

// releasedCommand is `released [--file F] --state DIR`: whether a part crossed to the state
// directory's key. It reads the directory and writes nothing.
//
// With --file, whether the bytes of F are such a part (commands-5). A delivery it fetched matches
// when its receipt, as delivered.jsonl holds it, passed, is to this key, and names as its digest the
// salted digest of F's bytes under the salt the delivery came with, from salts.jsonl or, for a part
// fetched before salts were kept, kept.jsonl. Placket compares the bytes, as get --out writes a
// body; which of them count is the caller's.
//
// Without --file, every part the key fetched and kept in kept.jsonl is tried instead, in fetch
// order, its body as get writes it (commands-7): it matches when the receipt of the id it names, as
// delivered.jsonl holds it, passed, is to this key, names the part's kind, and names as its digest
// the salted digest of the body under the salt the part was kept with. A part the key gave, or a
// version it gave as an answer, has no such receipt and is never tried.
//
// Conditions narrow a match, and all of them must hold: --kind, given once or more, the receipt's
// kind; --at P with --value-file V or --value S, the value at the JSON pointer P in the body, read
// by the pointer package, is exactly V's bytes or S's (commands-6); --from, the receipt's sender;
// and, without --file only, --re, the re the kept delivery carried, which no receipt states. Any
// release that matches is a match: the first in fetch order is the result, and with --every, also
// without --file only, every match is.
//
// On a match it prints the receipt id of each match on standard output, and on standard error not
// verified, the claim line and the scope line: the receipt was read, not verified, and verify does
// that. With no match it prints that no release matches and exits 1.
func releasedCommand(r *run, args []string) int {
	fs := flags(WordReleased)
	file, dir := fs.String("file", "", "the file"), fs.String("state", "", "state directory")
	at, valueFile := fs.String("at", "", "a JSON pointer into the body"), fs.String("value-file", "", "the value's bytes")
	valueText := fs.String("value", "", "the value, as text")
	re, from := fs.String("re", "", "the part's re"), fs.String("from", "", "the receipt's sender")
	every := fs.Bool("every", false, "every match")
	var kinds many
	fs.Var(&kinds, "kind", "a kind the part may have")
	if !r.parse(fs, args, "state") {
		return r.tool(codeUsage)
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	atSet, fileSet := set["at"], set["file"]
	if atSet != (set["value-file"] || set["value"]) || (set["value-file"] && set["value"]) {
		fmt.Fprintln(r.stderr, "usage: placket released: --at goes with one of --value and --value-file")
		return r.tool(codeUsage)
	}
	if fileSet && (set["re"] || set["every"]) {
		fmt.Fprintln(r.stderr, "usage: placket released: --re and --every read kept parts: give them without --file")
		return r.tool(codeUsage)
	}
	var body []byte
	if fileSet {
		var err error
		if body, err = os.ReadFile(*file); err != nil {
			return r.tool(codeFile)
		}
	}
	claim, none := ClaimReleasedKept, LineNoKept
	switch {
	case fileSet && atSet:
		claim, none = ClaimReleasedValue, LineNoValue
	case fileSet:
		claim, none = ClaimReleased, LineNoRelease
	case atSet:
		claim, none = ClaimReleasedKeptValue, LineNoKeptValue
	}
	value := []byte(*valueText)
	var tokens []string
	if atSet {
		if set["value-file"] {
			var err error
			if value, err = os.ReadFile(*valueFile); err != nil {
				return r.tool(codeFile)
			}
		}
		var ok bool
		if tokens, ok = pointer.Parse(*at); !ok {
			return r.tool(codePath)
		}
	}
	d, err := state.Read(*dir)
	if err != nil {
		return r.failed(err)
	}
	var tried []crossed
	if fileSet {
		tried, err = fileCrossed(d, body)
	} else {
		tried, err = keptCrossed(d)
	}
	if err != nil {
		return r.failed(err)
	}
	var found []crossed
	for _, c := range tried {
		if (len(kinds) > 0 && !slices.Contains(kinds, c.st.Subject.Name)) || (set["from"] && c.st.Predicate.From != *from) ||
			(set["re"] && c.re != *re) {
			continue
		}
		if atSet {
			if v, ok := pointer.Value(c.body, tokens); !ok || !bytes.Equal(v, value) {
				continue
			}
		}
		found = append(found, c)
		if !*every {
			break
		}
	}
	if *every {
		list := []any{}
		for _, c := range found {
			m := map[string]any{memberReceiptID: c.st.ID, "kind": c.st.Subject.Name, "at": c.st.Predicate.At, "from": c.st.Predicate.From}
			if c.re != "" {
				m["re"] = c.re
			}
			list = append(list, m)
		}
		r.put(memberMatches, list)
	}
	if len(found) == 0 {
		fmt.Fprintln(r.stderr, none)
		return ExitBlock
	}
	for _, c := range found {
		fmt.Fprintln(r.stdout, c.st.ID)
	}
	fmt.Fprintln(r.stderr, LineNotVerified)
	fmt.Fprintln(r.stderr, claim)
	fmt.Fprintln(r.stderr, Scope)
	r.put("verified", false)
	r.put(memberClaim, claim)
	r.put(memberNotClaimed, Scope)
	if !*every {
		r.put(memberReceiptID, found[0].st.ID)
		r.put(memberReceipt, json.RawMessage(found[0].rc))
		r.put("kind", found[0].st.Subject.Name)
	}
	return ExitOK
}

// A part that crossed to a state directory's key: the receipt it crossed under, as delivered.jsonl
// holds it, read; the part's body; and its re, as its kept delivery carried it, when it was kept.
type crossed struct {
	st   receipts.Statement
	rc   []byte
	body []byte
	re   string
}

// passedToKey are the receipts of delivered.jsonl that passed and are addressed to the directory's
// key, read and in fetch order.
func passedToKey(d *state.Dir) ([]crossed, error) {
	delivered, err := d.Delivered()
	if err != nil {
		return nil, err
	}
	var out []crossed
	for _, rc := range delivered {
		st, err := receipts.Decode(rc)
		if err == nil && st.Predicate.Verdict == string(composition.Pass) && st.Predicate.To == d.Actor() {
			out = append(out, crossed{st: st, rc: rc})
		}
	}
	return out, nil
}

// fileCrossed are the deliveries the bytes of a file are, in fetch order: each receipt that passed
// to this key whose digest is that of the bytes under a salt the delivery came with (commands-5).
func fileCrossed(d *state.Dir, body []byte) ([]crossed, error) {
	salts, err := fetchedSalts(d)
	if err != nil {
		return nil, err
	}
	all, err := passedToKey(d)
	if err != nil {
		return nil, err
	}
	var out []crossed
	for _, c := range all {
		for _, s := range salts[c.st.ID] {
			if salt, ok := client.SaltBytes(s); ok && receipts.Digest(salt, body) == c.st.Subject.Digest {
				c.body = body
				out = append(out, c)
				break
			}
		}
	}
	return out, nil
}

// keptCrossed are the parts of kept.jsonl that crossed to this key, in the order they were kept,
// which is fetch order: each a delivery with one body whose receipt id names a receipt that passed
// to this key, of the part's kind, whose digest is that of the body under the part's salt
// (commands-7). A kept line that is not such a delivery, a part the key gave among them, is not one.
func keptCrossed(d *state.Dir) ([]crossed, error) {
	all, err := passedToKey(d)
	if err != nil {
		return nil, err
	}
	byID := map[string]crossed{}
	for _, c := range all {
		if _, has := byID[c.st.ID]; !has {
			byID[c.st.ID] = c
		}
	}
	kept, err := d.Kept()
	if err != nil {
		return nil, err
	}
	var out []crossed
	for _, l := range kept {
		var k keptDelivery
		if !strict.Decode(l, &k) || !k.OneBody() {
			continue
		}
		c, has := byID[k.ReceiptID]
		salt, ok := client.SaltBytes(k.Salt)
		if !has || !ok || c.st.Subject.Name != k.Kind {
			continue
		}
		if body := k.Bytes(); receipts.Digest(salt, body) == c.st.Subject.Digest {
			c.body, c.re = body, k.Re
			out = append(out, c)
		}
	}
	return out, nil
}

// fetchedSalts are the salts of the deliveries the directory's key fetched, by receipt id: those of
// salts.jsonl, and those of the fetched parts of kept.jsonl.
func fetchedSalts(d *state.Dir) (map[string][]string, error) {
	out := map[string][]string{}
	salts, err := d.Salts()
	if err != nil {
		return nil, err
	}
	for _, s := range salts {
		out[s.ReceiptID] = append(out[s.ReceiptID], s.Salt)
	}
	kept, err := d.Kept()
	if err != nil {
		return nil, err
	}
	for _, l := range kept {
		var k keptDelivery
		if strict.Decode(l, &k) && k.ReceiptID != "" && k.Salt != "" {
			out[k.ReceiptID] = append(out[k.ReceiptID], k.Salt)
		}
	}
	return out, nil
}
