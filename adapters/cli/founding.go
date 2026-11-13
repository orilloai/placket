package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/setup"
)

func init() {
	commands["founding"] = foundingCommand
}

// foundingCommand is `founding --policy P --mediator M --card C --declaration D... --binding B...`:
// it puts the parts in one founding set, the declarations and the bindings in actor-reference order
// (setup-contract-10), founds it at the current second to check it, and prints the set on standard
// output and its Accord id and card hash on standard error.
func foundingCommand(r *run, args []string) int {
	fs := flags("founding")
	policy, med, card := fs.String("policy", "", "policy"), fs.String("mediator", "", "mediator"), fs.String("card", "", "card")
	var decls, binds many
	fs.Var(&decls, "declaration", "a declaration")
	fs.Var(&binds, "binding", "a binding")
	if !r.parse(fs, args, "policy", "mediator", "card", "declaration", "binding") {
		return r.tool(codeUsage)
	}
	set := map[string]any{}
	for name, path := range map[string]string{"policy": *policy, "mediator": *med, "card": *card} {
		b, ok := readJSON(path)
		if !ok {
			return r.tool(codeFile, name)
		}
		set[name] = json.RawMessage(b)
	}
	for name, paths := range map[string]many{"declarations": decls, "bindings": binds} {
		list, ok := byActor(paths)
		if !ok {
			return r.tool(codeFile, name)
		}
		set[name] = list
	}
	b := canonicalJSON(set)
	a, err := setup.Found(b, local.Stamp(r.env.Now()))
	if err != nil {
		var se *setup.Error
		if errors.As(err, &se) {
			return r.tool(codeFound, string(se.Code))
		}
		return r.tool(codeFound)
	}
	line(r.stdout, b)
	fmt.Fprintln(r.stderr, "accord "+a.ID())
	fmt.Fprintln(r.stderr, "card "+a.Card())
	return ExitOK
}

// byActor reads the files, each one JSON object, and orders them by their actor members.
func byActor(paths []string) ([]json.RawMessage, bool) {
	type entry struct {
		actor string
		b     []byte
	}
	var es []entry
	for _, p := range paths {
		b, ok := readJSON(p)
		if !ok {
			return nil, false
		}
		var o struct {
			Actor string `json:"actor"`
		}
		if json.Unmarshal(b, &o) != nil {
			return nil, false
		}
		es = append(es, entry{actor: o.Actor, b: b})
	}
	sort.SliceStable(es, func(i, j int) bool { return canonical.Less(es[i].actor, es[j].actor) })
	out := make([]json.RawMessage, len(es))
	for i, e := range es {
		out[i] = e.b
	}
	return out, true
}
