package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/orilloai/placket/adapters/internal/pointer"
	"github.com/orilloai/placket/adapters/state"
	"github.com/orilloai/placket/core/composition"
	"github.com/orilloai/placket/core/receipts"
)

func init() {
	commands[WordReleased] = releasedCommand
}

// releasedCommand is `released --file F --state DIR`: whether the bytes of F are a part that crossed
// to the state directory's key (commands-5). It reads the directory and writes nothing. A delivery
// it fetched matches when its receipt, as delivered.jsonl holds it, passed, is to this key, and
// names as its digest the salted digest of F's bytes under the salt the delivery came with, from
// salts.jsonl or, for a part fetched before salts were kept, kept.jsonl. Placket compares the
// bytes, as get --out writes a body; which of them count is the caller's. On a match it prints the
// receipt id on standard output, and on standard error not verified, the claim line and the scope
// line: the receipt was read, not verified, and verify does that. With no match it prints that no
// release matches and exits 1.
//
// With --at P and --value-file V, F must match as above and the value at the JSON pointer P in F's
// bytes must be exactly V's bytes, read by the pointer package (commands-6); the claim line is then
// the value's. --kind, given once or more, limits a match to receipts of those kinds.
func releasedCommand(r *run, args []string) int {
	fs := flags(WordReleased)
	file, dir := fs.String("file", "", "the file"), fs.String("state", "", "state directory")
	at, valueFile := fs.String("at", "", "a JSON pointer into the file"), fs.String("value-file", "", "the value's bytes")
	var kinds many
	fs.Var(&kinds, "kind", "a kind the part may have")
	if !r.parse(fs, args, "file", "state") {
		return r.tool(codeUsage)
	}
	atSet := false
	fs.Visit(func(f *flag.Flag) { atSet = atSet || f.Name == "at" })
	if atSet != (*valueFile != "") {
		fmt.Fprintln(r.stderr, "usage: placket released: --at and --value-file go together")
		return r.tool(codeUsage)
	}
	body, err := os.ReadFile(*file)
	if err != nil {
		return r.tool(codeFile)
	}
	claim, none := ClaimReleased, LineNoRelease
	var value []byte
	var tokens []string
	if atSet {
		claim, none = ClaimReleasedValue, LineNoValue
		if value, err = os.ReadFile(*valueFile); err != nil {
			return r.tool(codeFile)
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
	salts, err := fetchedSalts(d)
	if err != nil {
		return r.failed(err)
	}
	delivered, err := d.Delivered()
	if err != nil {
		return r.failed(err)
	}
	if atSet {
		if v, ok := pointer.Value(body, tokens); !ok || !bytes.Equal(v, value) {
			fmt.Fprintln(r.stderr, none)
			return ExitBlock
		}
	}
	for _, rc := range delivered {
		st, err := receipts.Decode(rc)
		if err != nil || st.Predicate.Verdict != string(composition.Pass) || st.Predicate.To != d.Actor() ||
			(len(kinds) > 0 && !slices.Contains(kinds, st.Subject.Name)) {
			continue
		}
		for _, s := range salts[st.ID] {
			salt, err := hex.DecodeString(s)
			if err != nil || receipts.Digest(salt, body) != st.Subject.Digest {
				continue
			}
			fmt.Fprintln(r.stdout, st.ID)
			fmt.Fprintln(r.stderr, LineNotVerified)
			fmt.Fprintln(r.stderr, claim)
			fmt.Fprintln(r.stderr, Scope)
			for name, v := range map[string]any{
				memberReceiptID: st.ID, memberReceipt: json.RawMessage(rc), "kind": st.Subject.Name, "verified": false,
				memberClaim: claim, memberNotClaimed: Scope,
			} {
				r.put(name, v)
			}
			return ExitOK
		}
	}
	fmt.Fprintln(r.stderr, none)
	return ExitBlock
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
		var m map[string]json.RawMessage
		var id, salt string
		if json.Unmarshal(l, &m) == nil && json.Unmarshal(m["receiptId"], &id) == nil && json.Unmarshal(m["salt"], &salt) == nil {
			out[id] = append(out[id], salt)
		}
	}
	return out, nil
}
