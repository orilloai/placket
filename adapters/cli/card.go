package cli

import (
	"encoding/json"
	"fmt"

	"github.com/orilloai/placket/adapters/internal/strict"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/composition"
)

func init() {
	commands["card"] = cardCommand
}

// cardCommand is `card --default`: it prints this build's card on standard output, and its card hash
// and the module it names on standard error (docs/adr/0012-the-kernel-hash.md). No other card is
// built by the command line, so --default is required.
func cardCommand(r *run, args []string) int {
	fs := flags("card")
	def := fs.Bool("default", false, "this build's card")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	if !*def {
		fmt.Fprintln(r.stderr, "usage: placket card: --default is required")
		return r.tool(codeUsage)
	}
	card := composition.Card()
	h, err := canonical.TypedHash(card)
	if err != nil {
		return r.tool(codeCard)
	}
	line(r.stdout, card)
	fmt.Fprintln(r.stderr, "card "+h)
	fmt.Fprintln(r.stderr, LabelModule+" "+composition.KernelVersion+" "+composition.KernelHash())
	return ExitOK
}

// kernel is a card's kernel member, as the card holds it.
type kernel struct {
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

// kernelOf reads the kernel of a card's bytes; false when it has none that reads.
func kernelOf(card []byte) (kernel, bool) {
	m, ok := strict.Member(card, "kernel")
	var k struct {
		Version *string `json:"version"`
		Hash    *string `json:"hash"`
	}
	if !ok || !strict.Decode(m, &k) || k.Version == nil || k.Hash == nil {
		return kernel{}, false
	}
	return kernel{Version: *k.Version, Hash: *k.Hash}, true
}

// cardOf is the card member of a founding set's bytes, or nil.
func cardOf(set []byte) []byte {
	c, _ := strict.Member(set, "card")
	return c
}

// member is the value of the member of b reached through names, one object at a time, each read
// with strict.Member: a name matches only as written, and bytes the canonical form refuses have none.
func member(b []byte, names ...string) (json.RawMessage, bool) {
	for _, n := range names {
		m, ok := strict.Member(b, n)
		if !ok {
			return nil, false
		}
		b = m
	}
	return b, true
}

// memberString is the string member of b reached through names, or "" when there is none.
func memberString(b []byte, names ...string) string {
	m, ok := member(b, names...)
	var s string
	if !ok || json.Unmarshal(m, &s) != nil {
		return ""
	}
	return s
}

// otherModule is the line that names a kernel that is not this build's, or "" when it is.
func otherModule(k kernel) string {
	if k.Version == composition.KernelVersion && k.Hash == composition.KernelHash() {
		return ""
	}
	// The kernel is read from a card nobody has checked yet, so each of its fields is escaped.
	return fmt.Sprintf(ModuleOther, field(k.Version), field(k.Hash), composition.KernelVersion, composition.KernelHash())
}
