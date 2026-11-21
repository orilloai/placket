# The principal

A principal is the key that vouches for a party: it signs the binding that gives the party's key its roles. Roles come from bindings and from nowhere else: not from a declaration, a policy or a message (setup-contract-2).

## The binding

A binding is a principal's signed statement that one actor holds a list of roles, under one mediator and one policy, until a time. `placket bind` signs one:

```sh
placket bind --key principal.key --actor "$(cat applier.ref)" --role applier --mediator "$(cat mediator.ref)" \
	--until 2099-01-01T00:00:00Z --policy policy.json
```

It names the policy by the typed hash of its canonical bytes, so the binding counts only under the policy its principal signed for. The terms name roles, never keys: a term `with` `role:decider` reaches whichever key a binding makes the decider.

## The policy

The founding set's policy lists the roles that must be held, and each principal with the roles it may grant. Founding refuses a binding that gives a role its principal may not grant, or one signed for another policy, mediator or time ([section 3 of the protocol](../../spec/protocol.md#3-founding-an-accord)). The policy does not list actors: each side names its own keys, and rotates them, without the other side's say.

One principal may stand behind several parties, or all of them, as in `placket init`, where you are the principal of every party. Between two owners, each owner's principal binds its own side, and the policy says which roles each may fill.

## Who stands behind each member

`placket verify` with the founding set prints one line per member, its roles and the principal that signed its binding, and a custody line, how many members the operator is the principal of:

```
member ed25519:0b53... roles applier principal ed25519:7c77...
custody: operator ed25519:1578... is principal of 0 of 3 members
```

A receipt names actors; the founding set says who stands behind them. When an answer is witnessed, `verify` names the key that signed it and the principal that bound it to its role ([verify](../verify.md)).
