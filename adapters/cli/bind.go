package cli

import (
	"encoding/json"
	"time"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/signing"
)

func init() {
	commands["bind"] = bindCommand
}

// bindCommand is `bind --key P --actor A --role r... --mediator M --until T`: the principal P signs
// a binding giving the actor the roles, labels such as decider, under the mediator until T, and
// prints it.
func bindCommand(r *run, args []string) int {
	fs := flags("bind")
	keyPath, actor := fs.String("key", "", "the principal's key file"), fs.String("actor", "", "the actor")
	med, until := fs.String("mediator", "", "the mediator"), fs.String("until", "", "until")
	var roles many
	fs.Var(&roles, "role", "a role label")
	if !r.parse(fs, args, "key", "actor", "mediator", "until", "role") {
		return r.tool(codeUsage)
	}
	if t, err := time.Parse(time.RFC3339, *until); err != nil || local.Stamp(t) != *until {
		return r.tool(codeUntil)
	}
	k, err := keyfile.Read(*keyPath)
	if err != nil {
		return r.failed(err)
	}
	b := map[string]any{"actor": *actor, "principal": k.Reference(), "roles": []string(roles), "mediator": *med, "until": *until}
	sig, err := signing.Sign(canonicalJSON(b), signing.TypeBinding, k)
	if err != nil {
		return r.tool(codeUsage)
	}
	b["sig"] = sig
	signed := canonicalJSON(b)
	line(r.stdout, signed)
	r.put("binding", json.RawMessage(signed))
	return ExitOK
}
