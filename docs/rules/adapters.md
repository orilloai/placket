# The door, the journal and the command line

The rules for `adapters/` and `cmd/`: the socket door, the journal, key files, the state directory, the log export and the command line. An adapter hands work to the core and takes the world off its hands. `docs/adr/0011-the-door-and-the-journal.md` records the design.

## `adapters-no-recheck`

An adapter hands work to the core and does not do the core's checking again. A door never returns one of the core's rejection codes from its own code; its own codes are about the call, never the operation inside it.

Why: two copies of a check drift apart, and the record would then blame the core for what the door refused.

Checked by: the reviewer.

## `adapters-core-types`

An adapter package does not re-export a core type under a new name; it takes and returns the core's types, bytes and strings.

Why: one name per thing, so a reader never has to map two.

Checked by: the reviewer.

## `main-is-thin`

`cmd/placket/main.go` is under 40 lines and holds nothing a test could want; every command is testable from `adapters/cli` without exiting the process.

Why: code in `main` is code no test reaches.

Checked by: `make ci` checks its length, step `main-is-thin`; the reviewer checks that it holds nothing a test could want.

## `call-signing`

The door signs and verifies a call with the core's payload-signing functions under a media-type payload type defined once in `adapters/local`, and defines no second signing construction.

Why: one signing construction is one thing to get right.

Checked by: the reviewer.

## `one-mediator-goroutine`

One goroutine owns the mediator; every call reaches it over a channel, and the door's tests run with `-race`.

Why: the mediator's state has one writer, so its receipts have one order.

Checked by: the reviewer.

## `journal-synced`

Every journal write is synced to disk before the window replies, and a failed write ends the process without a reply.

Why: a reply promises a receipt that survives a crash.

Checked by: the reviewer.

## `door-owns-clock`

The door reads the clock once per call, at whole seconds, and uses that one time for the call, and decides what the core cannot know. A salt is minted with `crypto/rand` by the party that gives or answers, and the door mints none.

Why: the core takes the world as parameters (`core-no-world`), one time per call keeps a call's receipts consistent, and an answerer that minted its own salt can show what it signed (`docs/adr/0009-who-decided.md`).

Checked by: the reviewer.

## `door-check-order`

The door checks a call in a fixed order (size, parse, signature, the accord, staleness, nonce) and stops at the first failure, before the mediator sees anything.

Why: an unauthenticated call must never reach the core.

Checked by: the reviewer.

## `door-passes-bytes`

What the mediator receives from the door is the operation's bytes as they came out of decoding, untouched and never encoded again.

Why: the core must judge what was sent, not the door's reading of it.

Checked by: the reviewer.

## `state-lock`

A `give` or a `resolve` takes the state directory's exclusive lock before it reads the sequence number, or anything it writes from, and keeps it until the reply is handled. A `get` and a `result` take it only to fetch and persist what they fetch. A call that waits at the door holds no lock while it waits, and `questions`, which reads no sequence number and writes nothing, takes none (`docs/adr/0012-the-mediator-api-and-the-command-line.md`).

Why: two commands at once must not reuse a sequence number, and a call that waits for up to a minute must not stop the same key's answer or give.

Checked by: the reviewer.

## `journal-is-evidence`

No code rewrites or truncates the receipts file, except removing an unterminated last line at start; the journal adapter reads, and the core's reload judges what was read.

Why: the journal is the evidence of what crossed.

Checked by: the reviewer.

## `files-like-siblings`

A new file of the journal or of the state directory is written, synced and repaired as an existing file of the same kind is. There are three kinds: a file appended to, a line at a time; a file written once and never again; and a file replaced whole. A file used in two of these ways keeps the way of each. The reviewer names the sibling whose use matches the new file's; the contributor need not. No repair goes further than `journal-is-evidence` allows.

Why: a crash finds the file written with the least care, and a new file kept less safely than its siblings loses or tears what they would keep.

Checked by: the reviewer.

## `restart-reproducible`

After a restart, the founding set plus the journal determine the state, and nothing else does; a window killed between calls carries on with the same receipts.

Why: a restart must not change history.

Checked by: the reviewer.

## `log-from-receipts`

Each cell the log export prints is taken from a receipt that verified, or from a kept part such a receipt covers; nothing comes from file names, the clock or lines no receipt covers.

Why: the log is only as trustworthy as its weakest cell.

Checked by: the reviewer.

## `product-words-once`

The words the tool prints for outcomes and every claim line are constants defined once, in the command line's words file and the export; the core never spells the outcome words the command line owns.

Why: one place to read and review every word a person sees.

Checked by: `make ci`, step `product-words-once`, refuses a file built into the core that spells `ask` or `released` as a string; the reviewer checks that every word is defined once.

## `claims-stop-at-crossing`

Nothing placket prints speaks of what a receiver does with content after receipt, and every claim line is followed by the scope line. A test collects every printed string and checks for words such as `retain` or `consent`.

Why: placket makes no claim about use after receipt.

Checked by: `make ci` checks string literals, step `claims-stop-at-crossing`; the reviewer reads the rest.

## `answer-call-attached`

The answerer's signed call travels inside every delivery its answer queues, as the delivery's `answer`, the released part included. The journal keeps each call it accepted in `answers.jsonl`, with no body and no salt, before the receipts of its closing. The call is never in a receipt: a receipt names it by its typed hash.

Why: every party an answer reaches, and the operator's journal, can show who answered, so a verifier can say an answer is witnessed, and the journal keeps no clear text it does not need (`docs/adr/0009-who-decided.md`).

Checked by: the reviewer.

## `wait-and-result-shared`

Waiting on a proposal and asking for its result go through a single function, which prints the last line, sets the exit code and fetches.

Why: two paths to one answer drift apart.

Checked by: the reviewer.

## `keys-on-disk`

Key files are created 0600 with `O_EXCL` and written atomically. A key file that others can read is refused when it is loaded.

Why: a key another user can read, or a key file left half written, is a key lost.

Checked by: the reviewer.

## `goroutines-owned`

Every goroutine has an owner, a way to be stopped, and a way it ends. Nothing is fire-and-forget.

Why: a goroutine nobody owns leaks, races, or outlives the state it reads.

Checked by: the reviewer.
