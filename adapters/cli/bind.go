package cli

import (
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/orilloai/placket/adapters/keyfile"
	"github.com/orilloai/placket/adapters/local"
	"github.com/orilloai/placket/core/canonical"
	"github.com/orilloai/placket/core/signing"
)

func init() {
	commands["bind"] = bindCommand
}

// bindCommand is `bind --key P --actor A --role r... --mediator M --until T --policy FILE`: the
// principal P signs a binding giving the actor the roles, labels such as decider, under the mediator
// and the policy until T, and prints it. FILE is the founding set's policy: bind puts it in canonical
// form and writes its typed hash into the binding's policy (setup-contract-2, commands-3). A FILE it
// cannot read, or that the canonical form refuses, is tool error file with the detail policy, and no
// binding is printed. Whether the policy lets P grant the roles is founding's to judge, not bind's.
func bindCommand(r *run, args []string) int {
	fs := flags("bind")
	keyPath, actor := fs.String("key", "", "the principal's key file"), fs.String("actor", "", "the actor")
	med, until := fs.String("mediator", "", "the mediator"), fs.String("until", "", "until")
	policyPath := fs.String("policy", "", "the founding set's policy")
	var roles many
	fs.Var(&roles, "role", "a role label")
	if !r.parse(fs, args, "key", "actor", "mediator", "until", "role", "policy") {
		return r.tool(codeUsage)
	}
	// A flag that is not UTF-8 is refused, never repaired into another value (parse-strict).
	for _, s := range append([]string{*actor, *med}, roles...) {
		if !utf8.ValidString(s) {
			return r.tool(codeUsage)
		}
	}
	if t, err := time.Parse(time.RFC3339, *until); err != nil || local.Stamp(t) != *until {
		return r.tool(codeUntil)
	}
	k, err := keyfile.Read(*keyPath)
	if err != nil {
		return r.failed(err)
	}
	policy, ok := readJSON(*policyPath)
	if !ok {
		return r.tool(codeFile, "policy")
	}
	policyHash, err := canonical.TypedHash(policy)
	if err != nil {
		return r.tool(codeFile, "policy")
	}
	b := map[string]any{"actor": *actor, "principal": k.Reference(), "roles": []string(roles), "policy": policyHash, "mediator": *med, "until": *until}
	sig, err := signing.Sign(canonicalJSON(b), signing.TypeBinding, k)
	if err != nil {
		return r.tool(codeInternal)
	}
	b["sig"] = sig
	signed := canonicalJSON(b)
	line(r.stdout, signed)
	r.put("binding", json.RawMessage(signed))
	return ExitOK
}
