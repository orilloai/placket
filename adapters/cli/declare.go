package cli

import (
	"encoding/json"
	"os"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/core/signing"
)

func init() {
	commands["declare"] = declareCommand
}

// readTerms reads a terms file: {"terms":[...]}, the declaration without actor and sig, and
// nothing else. It returns the terms' JSON.
func readTerms(b []byte) (json.RawMessage, bool) {
	var t struct {
		Terms json.RawMessage `json:"terms"`
	}
	if !strict.Decode(b, &t) || len(t.Terms) == 0 || t.Terms[0] != '[' {
		return nil, false
	}
	return t.Terms, true
}

// declareCommand is `declare --key F --terms T.json`: it signs the actor's declaration of the terms
// and prints it. Founding checks the terms themselves.
func declareCommand(r *run, args []string) int {
	fs := flags("declare")
	keyPath, termsPath := fs.String("key", "", "key file"), fs.String("terms", "", "terms file")
	if !r.parse(fs, args, "key", "terms") {
		return r.tool(codeUsage)
	}
	k, err := keyfile.Read(*keyPath)
	if err != nil {
		return r.failed(err)
	}
	b, err := os.ReadFile(*termsPath)
	if err != nil {
		return r.tool(codeFile)
	}
	terms, ok := readTerms(b)
	if !ok {
		return r.tool(codeTerms)
	}
	decl := canonicalJSON(map[string]any{"actor": k.Reference(), "terms": terms})
	if decl == nil {
		return r.tool(codeTerms)
	}
	sig, err := signing.Sign(decl, signing.TypeDeclaration, k)
	if err != nil {
		return r.tool(codeTerms)
	}
	line(r.stdout, canonicalJSON(map[string]any{"actor": k.Reference(), "terms": terms, "sig": sig}))
	return ExitOK
}
