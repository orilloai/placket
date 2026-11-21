package cli

import (
	"fmt"
	"strings"

	"github.com/orilloai/placket/adapters/keyfile"
)

func init() {
	commands["key"] = keyCommand
}

// keyCommand is `key new --out F`, which writes a new key file and prints its reference, and
// `key ref --key F`, which prints the reference of a key file. Neither prints a seed. `key --help`
// names both.
func keyCommand(r *run, args []string) int {
	if len(args) == 0 {
		return r.usage()
	}
	if strings.HasPrefix(args[0], "-") {
		r.subs = []string{"key new", "key ref"}
		if !r.parse(flags("key"), args) {
			return r.tool(codeUsage)
		}
		return r.usage()
	}
	switch args[0] {
	case "new":
		fs := flags("key new")
		out := fs.String("out", "", "the key file to write")
		if !r.parse(fs, args[1:], "out") {
			return r.tool(codeUsage)
		}
		k, err := keyfile.New(*out, r.env.Rand)
		if err != nil {
			return r.failed(err)
		}
		fmt.Fprintln(r.stdout, k.Reference())
		r.put("actor", k.Reference())
		return ExitOK
	case "ref":
		fs := flags("key ref")
		path := fs.String("key", "", "the key file")
		if !r.parse(fs, args[1:], "key") {
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
	return r.usage()
}
