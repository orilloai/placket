# 0024. The mediator API, the client, and placket mcp

Status: accepted. Date: 2026-11-18. Amends records 0001, 0009, 0010, 0021 and 0023. Amended 2026-11-21: `adapters/errtext` holds the command line's codes too, and `adapters/cli/gen` writes the CLI reference from the command line's command table. Amended by records 0025 and 0029.

## Context

An agent reaches a tool through MCP, and a person who answers questions should hear of one when it opens. Until now the only way to reach a mediator was the command line, one process per call, one mediator per command; a party learned of a question by asking for the open questions again and again. The door's calls were written down only in `adapters/local`'s package comment, which had fallen behind the code: it still listed resolve's args as an action and a text, gave the door the salt to mint, and left out the questions reply's receipt, salt and `more`. And the command line's client, the code that signs calls, keeps the seq and the state lock and checks every reply, lived inside `adapters/cli`, where nothing else could use it.

## Decision

### The mediator API is the door's calls, written as schemas

The door's five calls, `give`, `get`, `questions`, `resolve` and `result`, and their replies, are the mediator API, versioned with the release. They stay door formats, outside the freeze: the answer call a resolve carries, and the receipts, deliveries and close a reply carries, are frozen formats of `spec/`, and the schemas describe them as opaque objects, as the door carries them, rather than restate them.

`api/` holds the schemas, the one source of the door formats, written from the code. `adapters/local`'s package comment points to them instead of listing the members. They come in two layers:

- **the wire layer**, `api/wire/<op>.call.json` and `<op>.reply.json`, with `api/wire/common.json` for the pieces they share, for a client in any language;
- **the request layer**, `api/request/<op>.json`, what `adapters/client` and the MCP tools take: a give takes the message's receiver, kind, body and its `re` and `topic`, and the client builds the message, mints its id and salt and signs the envelope at its seq; a resolve takes the question, the action and, for an accept or a change, the answer, and the client mints the answer's salt. `cancel.json` is a resolve with action `cancelled`, the asker's; `mediators.json` is the listing tool's input, and no door call.

`api/config/mediators.json` is the labels config's schema. Every `$id` is `https://placket.tech/api/0.1/<path>`, and no schema names one outside `api/`. They use only the keywords `core/conformance`'s validator compiles. `api` is a Go package that embeds them (`api.FS`, `api.Request`) for `placket mcp`'s tool input schemas, and imports nothing of placket.

`api/internal/gen` writes `api/testdata/worked/`: a founding set of fixed keys, and one worked signed call per op, each fixing the key, the clock and the nonce, a give's message id, salt and seq, a resolve's answer salt and nested signature, with the expected canonical bytes and signature. It uses the standard library alone, so the expected values never come from the code they test, and chooses its inputs (ASCII strings with no `<`, `>` or `&`, whole numbers) so that `encoding/json`'s sorted encoding, HTML escaping off, is the canonical form. `api/gen` writes `docs/reference/mediator-api.md` from both layers, the config schema and the error table. `api`'s tests send the worked calls to a window, record its replies in `api/testdata/replies/`, and fail when either generated directory or page differs from a fresh run. `core/conformance` validates the worked calls, the recorded replies and the config fixtures by path (`TestAPISchemas`), as it reads `spec/` and `vectors/`, so the core gains no exported name and neither module imports the other's code. The worked calls live outside `vectors/`, whose runner and checks are about protocol operations, not door calls.

### Waiting, at the door

`questions`, `get` and `result` take an optional `wait`, whole seconds from 0 to 60; anything else is `bad-call`. The window parks a call that waits and has nothing to return; its one loop looks at every parked call again after each call it handles, each tick and each judge's result, and answers it when it has something, or when its wait ends, with the reply it would give at that moment (`one-mediator-goroutine`). The connection's goroutine only carries the reply. The door's base deadline, 30 seconds, becomes the window's `Deadline`; a connection whose call waits may stay open for its wait plus 10 seconds when that is longer, and a client's zero timeout becomes the larger of a minute and its wait plus 15 seconds. A give, which has no wait and may take up to 20 seconds with a judge, keeps today's limits.

A `get` that waits never takes a delivery. It replies `{"ok":true,"queued":true}` once one is queued for the caller, and the client then makes an ordinary `get`, which takes it as before. So a client that hangs up while it waits loses nothing. The window counts the deliveries held for each member to know this without taking one.

Every `questions` reply carries `cursor`, the position, in the order the window journaled the asks, of the last question the reply carries, sealed as one AES block (`crypto/aes`) under a key drawn at each start from `Window.CursorRand`: the position's eight bytes and eight zero bytes, which the window checks when it reads a cursor back. So a cursor is opaque to every caller, and the window keeps no table of the cursors it minted. Under `more` the position is the last question kept. A reply that carries none hands back the `after` it was given, so passing it back still asks for what opens after the last question this caller was shown. `questions` with `after` a cursor of this start's returns only the questions opened after it; any other `after`, one from before a restart or one no window minted, is taken for none. So a caller that has seen every open question waits instead of asking again. The window keeps the position of each open ask, and drops those of closed ones.

### The client, extracted

`adapters/client` is the command line's client, made a package of its own: it reads the key file, signs every call, owns the seq (crossing-4) and the state lock in the command line's state directory, persists a fetched delivery before it returns it, and checks every reply as the command line did. Every call takes a `context.Context`, which closes the connection when it ends (`local.Client.CallContext`). The command line's socket commands call it, and their flags, output and exit codes are unchanged; `adapters/cli` drops its constants `reply`, `asked`, `rand` and `envelope`, which are the client's now, and prints a client error's code as it is.

### The state lock, while waiting (`state-lock`)

A call that waits holds no lock: only `give` and `resolve` hold it from the seq to the handled reply. `questions` reads no seq and writes nothing, so it holds none. `get` and `result` wait without it, then take it to fetch and persist. `give --wait` lets it go before it waits, as before. So `placket watch`, or an agent's `result` with a wait, never stops the same key's `answer` or `give`. This changes `state-lock`'s text in `docs/rules/adapters.md`, a rules change made in this unit rather than in a change of its own, because it exists only because `wait` does: the rule as it stood would have the lock held for a minute by every waiting call.

### Several mediators, by label

The labels config is `mediators.json` in placket's config directory, `$XDG_CONFIG_HOME/placket`, else `~/.config/placket`. Each entry names an Accord id, a key file, the founding set's path, an optional socket and an optional label, a local name that is never signed: 1 to 63 of `a-z`, `0-9` and `-`, starting with a letter or a digit, so it is safe in a shell and never looks like an Accord id. Relative paths are under the config directory, and paths are compared cleaned. An entry's state directory is the command line's. `placket mediators` lists it; every socket command takes `--label` in place of `--key`, `--socket` and `--accord`, naming an entry by its label or, with none, its Accord id.

Each mediator stands alone: nothing about one, its actors, keys, roles or terms, is checked against another's. The one check across entries is local, so mediators on one machine never collide: after the schema (`bad-config`), in this order, two entries with one label (`label-taken`), two of different Accords on one socket, two on one state directory, and two unlabelled entries of one Accord (`mediator-collision`), the first pair by its first index, then its second (`check-order`). A config that fails any is not used at all.

A window with no `--socket` listens on `<runtime>/<the first 12 hex digits of the Accord id>.sock`, `<runtime>` being `$XDG_RUNTIME_DIR/placket` or else `$TMPDIR/placket-<uid>`, made mode 0700 and checked to be the user's own. Twelve digits keep the path within macOS's 103 bytes under a long `TMPDIR`; two Accords whose ids share them are caught by the labels config, and one takes its own socket. With no `--journal` it writes to `$XDG_STATE_HOME/placket/<the whole id's hex>/`, else `~/.local/state/placket/<hex>/`.

### placket mcp and placket watch

`placket mcp` is a hand-written MCP server on standard input and output, on the standard library: JSON-RPC 2.0, one message per line, protocol version `2025-06-18`, tools only. It is one party's client. Its tools are `give`, `get`, `questions` and `result`, one per call it lists, each its request plus `mediator`; `cancel`, the asker's withdrawal of its own question, which always sends action `cancelled`; and `mediators`, the listing. `resolve` is never listed: `placket mcp` never accepts, changes or declines, never runs a command and never holds a mediator's key. Each tool call runs on a goroutine of its own, with a client of its own, so a call that waits never holds another back. `give` returns at once with the question's id when the part is asked about; the agent follows with `result` and its wait.

It serves only the labels it is started with, each with its own key, and reads nothing of a label it is not given. It holds proposing keys only: a label whose key holds a role that a `maybe` term's `resolve.to` names, in that label's own founding set, is refused, `answering-key`, unless it is started with `--allow-answering`; a label whose founding set cannot be read, or does not found, is refused, `founding`. The other labels are served as usual. Once sealed declarations land, the same check reads the party's own accepted view. Amended 2026-11-22: a label is also refused, `founding`, when its kept acceptance is not signed by its key for this Accord, by founding's rows 19 and 20, which any party runs with no opening.

`placket watch` loops over `questions` with the longest wait and the last cursor, on one or more labels, and prints each new question once, with its label, kind, asker and deadline, and the `placket answer`, `change` and `decline` lines that answer it. It never answers. With `--json` it prints one object per question, as it opens, and its object last: the one exception to commands-1's single object, since it prints for as long as it runs. `patterns/notifier/` runs a command for each.

### One sentence per code

Every code `placket mcp` can return has one sentence of cause and fix in `adapters/errtext`, which imports nothing of placket. A row is keyed by where its code comes from: a code in a door reply by its member (`door`, `rejection`, `error`); every other by the package that declares its `Code` type (`adapters/client`, `adapters/mcp`, `adapters/state`, `adapters/keyfile`, `adapters/local`). The same spelling under two keys has two sentences. A tool error's text is `<code>: <sentence>`. The command line's own codes, of its type `code`, have rows under `adapters/cli`, and the journal's under `adapters/journal`; a code the command line prints from another package's `Code` type keeps that package's one row, and one only spelled like a code of `placket mcp`, such as `founding`, has a row of its own. The command line still prints only the code: the sentences are in `docs/reference/cli.md`, whose Errors section `adapters/cli/gen` writes from these rows, as the mediator API's Errors section lists every row but those two keys'. Two kinds of code that follow another have no row: the verifier's code after `error verify`, which the protocol defines (verify-1), and a judge config's code after `error judge` (record 0023). `adapters/mcp`'s codes are `answering-key`, `founding`, `bad-arguments` (arguments that fail the tool's input schema or the 60-second cap) and `internal`, a failure no other code names; malformed JSON-RPC is answered with JSON-RPC's own codes.

### The layout and the package map

Record 0001's directory table gains `api/` (the mediator API's schemas, their Go package, their two generators and their test data) and `docs/reference/` (generated reference pages). Record 0009's package map gains:

| Package | What it holds |
|---|---|
| `api` | The schemas, embedded; it imports nothing of placket. |
| `api/gen` | The reference's generator: `api` and `adapters/errtext`. |
| `api/internal/gen` | The worked calls' generator: the standard library alone. |
| `adapters/client` | The mediator API's client and the labels config. |
| `adapters/mcp` | `placket mcp`. |
| `adapters/errtext` | The sentence of every code `placket mcp` returns and the command line prints. |
| `adapters/cli/gen` | The CLI reference's generator: `adapters/cli` and `adapters/errtext`. |

The import edges: `adapters/cli` imports `adapters/client` and `adapters/mcp`; `adapters/mcp` imports `adapters/client`, `api`, `adapters/errtext` and `adapters/export` (for a closing's word); `adapters/client` imports `adapters/local`, `adapters/state`, `adapters/keyfile`, `adapters/internal/strict` and core packages, and no adapter it imports imports it back. `api`, `api/gen` and `api/internal/gen` are the packages outside `cmd`, `adapters` and `core`; `api`'s external test package imports `adapters/local`, `adapters/client`, `adapters/mcp` and core packages. `docs/rules/modules.md` names the edges `adapters/mcp` → `api`, `api/gen` → `api`, `api/gen` → `adapters/errtext` and the test's. `adapters/cli/gen` imports `adapters/cli`, for its command table, and `adapters/errtext`; `adapters/cli` keeps one declarative table of its commands and subcommands, each command reads its flags from its row, and `adapters/cli`'s tests fail when `docs/reference/cli.md` differs from a fresh `go run ./adapters/cli/gen`.

## Consequences

- An agent reaches several mediators through one `placket mcp`, each with its own key, and a person hears of a question when it opens. Neither changes a frozen format, a requirement or a vector, and the answer call is unchanged.
- A client in any language has schemas, worked calls and a reference to build on, and a change to the door's formats fails a test until they are updated.
- A parked call holds a connection and a goroutine for up to a minute. Nothing caps how many a local process opens (threat model, L29).
- The labels config is trusted as the user's own file: placket checks its shape and that its entries do not collide, not who wrote it.
- `placket mcp`'s refusal of answering keys reads the founding set the config names; a user can lift it with `--allow-answering`, and a config that names another founding set than the window's is refused only when the set's id is not the entry's Accord.
