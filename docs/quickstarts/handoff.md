# Quickstart: handoffs between agents

A planner hands a plan to a coder, and the coder hands its work to a reviewer, across two mediators, each handoff with a receipt. Nobody is asked: the terms pass each handoff, and block any kind they do not name. It needs `placket` on your `PATH` ([install](../../README.md#install)) and no judge's key.

```sh
placket init handoff
```

`init` founds two example Accords for you alone, `handoff-plan` and `handoff-review`, all in `~/.config/placket/handoff/`. It prints each Accord's id, its parties, and every command that comes next:

1. Start both windows with the two `placket window` lines it printed, each in a terminal of its own.
2. Add placket to Claude Code with the `claude mcp add` line, which serves all four labels.
3. Ask for the handoffs: the planner gives the plan, the coder fetches it and gives its work on, the reviewer fetches that.
4. Verify each journal; the two are checked apart.

[The guide](../guides/handoff.md) takes each step in full, from `init` to two verified journals.
