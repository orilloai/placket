package composition

import (
	// The kernel hash is taken from the source the build compiled (docs/adr/0012-the-kernel-hash.md).
	_ "embed"
	"encoding/json"

	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/setup"
)

// KernelVersion is this decision module's version, the card's kernel version (setup-contract-4,
// docs/adr/0012-the-kernel-hash.md). It is raised when the verdicts the module returns change.
const KernelVersion = "0.2.0"

// source is the module's logic, as the build compiled it.
//
//go:embed composition.go
var source string

// kernelHash is computed once, from source.
var kernelHash = hashOf(map[string]string{"composition.go": source})

// hashOf is the typed hash of the object of the module's files by name (canonical-3), or "" when
// the canonical form refuses it, which no Go source file of the module can cause.
func hashOf(files map[string]string) string {
	b, err := json.Marshal(files)
	if err != nil {
		return ""
	}
	h, err := canonical.TypedHash(b)
	if err != nil {
		return ""
	}
	return h
}

// KernelHash is this decision module's hash, the card's kernel hash: the typed hash of the JSON
// object {"composition.go": the file's bytes}, from the source the build compiled, so a build whose
// module differs by any byte has another (setup-contract-4, docs/adr/0012-the-kernel-hash.md).
func KernelHash() string { return kernelHash }

// Card is this module's card in canonical bytes: spec 0.1, its kernel and no judges, a new slice at
// each call (setup-contract-4, docs/adr/0012-the-kernel-hash.md).
func Card() []byte {
	type component struct {
		Version string `json:"version"`
		Hash    string `json:"hash"`
	}
	b, err := json.Marshal(struct {
		Spec   string    `json:"spec"`
		Kernel component `json:"kernel"`
		Judges []any     `json:"judges"`
	}{setup.Spec, component{KernelVersion, kernelHash}, []any{}})
	if err != nil {
		return nil
	}
	c, err := canonical.Form(b)
	if err != nil {
		return nil
	}
	return c
}
