package cli

import (
	"fmt"

	"github.com/orilloai/placket/adapters/keyfile"
)

func init() {
	commands["key"] = keyCommand
	subcommands["key new"] = keyNewCommand
	subcommands["key ref"] = keyRefCommand
}

// keyCommand is key with no subcommand, or one it does not have: error usage. Neither subcommand
// prints a seed.
func keyCommand(r *run, _ []string) int { return r.usage() }

// keyNewCommand is `key new --out F`, which writes a new key file and prints its reference.
func keyNewCommand(r *run, args []string) int {
	fs := newFlags("key new")
	out := fs.text("out")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	k, err := keyfile.New(*out, r.env.Rand)
	if err != nil {
		return r.failed(err)
	}
	fmt.Fprintln(r.stdout, k.Reference())
	r.put("actor", k.Reference())
	return ExitOK
}

// keyRefCommand is `key ref --key F`, which prints the reference of a key file.
func keyRefCommand(r *run, args []string) int {
	fs := newFlags("key ref")
	path := fs.text("key")
	if !r.parse(fs, args) {
		return r.tool(codeUsage)
	}
	ref, err := keyfile.Reference(*path)
	if err != nil {
		return r.failed(err)
	}
	fmt.Fprintln(r.stdout, ref)
	r.put("actor", ref)
	return ExitOK
}
