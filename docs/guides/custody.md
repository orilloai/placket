# Custody

A receipt says what the mediator released. That an action happened only after a release rests on something placket does not hold: who else can do the action. This guide says what custody means for any gate. For a merge on GitHub, [the custody checklist](../../patterns/github-merge/custody.md) gives every step.

## The agent's credential must not do the gated action

Placket gates what crosses the mediator. If the agent's own credential can merge the pull request, write the file or send the message, the agent can act without asking, and the receipts record only the times it asked. So:

- the agent's credential cannot do the gated action, nor change the settings that keep it from it;
- the applier's credential can, and the applier is the only thing that holds it;
- the agent runs in an operating system account of its own, which cannot read the applier's credential, the deciding role's key, the mediator's key or the applier's files, and reaches the window only through `placket mcp`.

As `placket init` sets an example up, every key is in your own account, which shows the flow and does not hold against an agent that runs commands as you ([approve](approve.md#what-this-setup-does-not-stop)).

## What the applier checks

The applier acts only on a release whose answer call is an accept by a key the founding set's bindings give the deciding role, and only while what the release names still holds; it acts once per release, and records where its action can be checked from ([writing an applier](appliers.md)). It holds the one credential that can act, so the agent must not be able to change the applier, its configuration or which keys it takes for the deciding role's.

## What each side claims

- **Placket claims** that the mediator released this part, and, from the answer call, which key accepted it. `placket verify` checks both from the journal, with the Accord's id alone ([verify](../verify.md)).
- **The forge's settings, and the accounts, claim** that nothing did the action any other way: that only the applier's credential could merge, that the agent could not reach it. Placket neither sets nor checks them.

A status, a label or a comment that anyone with write access can set is for the auditor to check against a receipt, never a gate. Check custody with the agent's own credential: the gated action, done as the agent, must be refused.
