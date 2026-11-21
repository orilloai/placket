# Several mediators

One machine may run several mediators, one per Accord and so one per gate: a window each, with its own socket, journal and keys. A party reaches each by a label of its own, and `placket mcp` serves several at once. Each mediator stands alone: nothing about one, its actors, keys, roles or terms, is checked against another's.

[The handoff guide](handoff.md) runs two, `handoff-plan` and `handoff-review`, with the coder a member of both.

## A window per Accord

Each Accord runs in a window of its own. With no `--socket`, a window listens on its Accord's own socket, `<runtime>/<the first 12 hex digits of the Accord id>.sock`, where `<runtime>` is `$XDG_RUNTIME_DIR/placket`, else `$TMPDIR/placket-<uid>`. With no `--journal`, it writes to `$XDG_STATE_HOME/placket/<the Accord id's hex>/`, else `~/.local/state/placket/<hex>/`. So two windows on one machine never share a socket or a journal unless you give them one.

```sh
placket window --key mediator.key --founding plan/founding.json --openings openings
placket window --key mediator.key --founding review/founding.json --openings openings
```

## The labels config

The labels config, `mediators.json` in placket's config directory (`$XDG_CONFIG_HOME/placket`, else `~/.config/placket`), names the mediators one user reaches. Each entry names an Accord id, a key file, the founding set's path, an optional socket and an optional label:

```json
{"mediators":[
 {"accord":"sha256:...","founding":"handoff/handoff-plan/founding.json","key":"handoff/handoff-plan-coder.key","label":"handoff-plan-coder"},
 {"accord":"sha256:...","founding":"handoff/handoff-review/founding.json","key":"handoff/handoff-review-coder.key","label":"handoff-review-coder"}]}
```

A label is a local name, never signed: 1 to 63 of `a-z`, `0-9` and `-`, starting with a letter or a digit. Relative paths are under the config directory. `placket init` adds an entry for each party it founds. [The mediator API](../reference/mediator-api.md#the-labels-config) gives the format in full.

The config is refused whole when two entries collide: two with one label (`label-taken`), or two of different Accords on one socket, two on one state directory, or two unlabelled entries of one Accord (`mediator-collision`). The error names the two entries by their index.

```sh
placket mediators
```

lists the entries, one JSON object per line, in the order the file lists them.

## Commands by label

Every command that calls a window takes `--label` in place of `--key`, `--socket` and `--accord`, naming an entry by its label, or by its Accord id when it has none:

```sh
placket get --label handoff-plan-coder --wait 60
placket give --label handoff-review-coder --to REVIEWER --kind work --text "the work"
```

`placket watch` takes `--label` once per mediator, and waits on the questions of all of them at once, printing each new question with its label.

## One agent, several mediators

`placket mcp` takes `--label` once per mediator, and serves each with its own key. Every tool call names the mediator it is for, by label, and the `mediators` tool lists what is served. A label `placket mcp` is not started with has none of its files opened.

```sh
claude mcp add placket -- placket mcp --label handoff-plan-coder --label handoff-review-coder
```

Give each agent only its own labels, so that no agent acts under another's key ([Claude Code over MCP](claude-code.md)).
