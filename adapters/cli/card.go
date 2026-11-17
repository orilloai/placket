package cli

import (
	"encoding/json"
	"fmt"

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
	var c struct {
		Kernel *kernel `json:"kernel"`
	}
	if json.Unmarshal(card, &c) != nil || c.Kernel == nil {
		return kernel{}, false
	}
	return *c.Kernel, true
}

// cardOf is the card member of a founding set's bytes, or nil.
func cardOf(set []byte) []byte {
	var s struct {
		Card json.RawMessage `json:"card"`
	}
	if json.Unmarshal(set, &s) != nil {
		return nil
	}
	return s.Card
}

// otherModule is the line that names a kernel that is not this build's, or "" when it is.
func otherModule(k kernel) string {
	if k.Version == composition.KernelVersion && k.Hash == composition.KernelHash() {
		return ""
	}
	return fmt.Sprintf(ModuleOther, k.Version, k.Hash, composition.KernelVersion, composition.KernelHash())
}
