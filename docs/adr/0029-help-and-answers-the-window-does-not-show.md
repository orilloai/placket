# 0029. `--help` prints usage, and an answer the window does not show goes to the window

Status: accepted. Date: 2026-11-21. Amends record 0021.

## Context

Two behaviours of the command line stopped a person with an error the docs did not lead them to expect.

`--help` was a flag no command defines. Every command reads its flags with Go's flag package, which reports `-h`, `-help` and `--help` as a request for usage, and commands-1 made every flag that cannot be read the error `usage`. So `placket verify --help` printed `error usage` and exited 3, and no command said what it takes.

`answer`, `change` and `decline` read the question as the window shows it before they sign, as record 0021 decided: the ask's receipt names the part, and the body under the salt makes the receipt's digest. Record 0021 made two different facts one error, `asked`, exit 3: the window shows the question but its part is not the one the receipt names; and the window does not show the question at all. The second is not a misstated part. The window shows every open question to each of its answerers, so a question it does not show is closed, or not this key's to answer. The mediator already has the refusals for both: `no-open-question` for a question that is not open (maybe-7) and `not-an-answerer` for a member the rule does not name (maybe-6). The client stood in front of them with an error that said neither. It also read only the first page of `questions`, so a question past the first page was taken for one the window does not show.

## Decision

### `--help`

`--help`, `-help` or `-h`, given as a flag and not as a flag's value, asks for the command's usage. It is carved out of commands-1's rule that a flag that cannot be read is the error `usage`; this record is that correction's note.

- The command prints its usage on standard output: `usage: placket <command> [flags]`, one line per flag it reads, in name order, each starting `--<name>` with the flag's kind, its usage and any default that is not the zero value, and then one line per subcommand, `placket <subcommand> [flags]`. Standard error is empty, and the exit is 0.
- It reads no file, socket or state, and checks no flag the command requires: the flags are read up to `--help` and the command stops there.
- With `--json` it prints one object: `command`, `exit` 0, `flags`, a list of `{"default","name","usage"}` in name order, as Go's flag package holds them, and `subcommands` when there are any.
- `key --help` names `key new` and `key ref`, and `window --help` prints the window's flags and names `window close`. `init --help` names one `init <example>` per example, since the example is a word before the flags. `key new --help`, `window close --help` and `init approve --help` print that subcommand's flags.
- `placket --help` prints the usage line and one line per command, or with `--json` `{"commands":[...],"exit":0}`.
- `mcp` takes no `--json`, because its standard output carries the MCP protocol. `mcp --help` prints its usage like any command; `mcp --help --json` is refused as `mcp --json` is, `{"error":"usage","exit":3}`, so that `mcp` keeps taking no `--json`.
- `--help` given as a flag's value, as in `give --text --help`, is that value. `placket` alone and a command that does not exist stay the error `usage`, exit 3.

The usage text is built from each command's flag set, so it cannot list a flag the command does not read. A declarative command table, which would also give a generated usage reference, is later work.

### Answers to a question the window does not show

This amends record 0021's section "What an answerer is shown, and what it checks".

- `asked` now means only that the part the window shows is not the one its ask's receipt names: the receipt is not the question's, not under this Accord, not of the kind shown, or the body under the salt does not make its digest. Nothing is signed or sent, as before.
- A question the window does not show, on any page of its `questions`, is no longer `asked`. The command signs the answer call and sends it, and prints the mediator's refusal as any refusal prints: `rejected no-open-question` for a question already closed, `rejected not-an-answerer` for a key that does not answer it, exit 2. A decline's comment is given only after the window takes the decline, so a refused decline gives none.
- Before it decides the window does not show a question, the client reads every page: while a reply says `more`, it asks again after the reply's cursor. One question always fits in a reply, since a reply may be four times the largest call, so every open question is on some page.
- A closed Accord is still the rejection `closed` before anything is signed, and `placket questions` still leaves out a misstated part and ends in `asked`.

This signs an answer call for a question the client has not checked. That is safe because of what the window shows: a question it does not show cannot be answered, and the mediator refuses the call, so the signed call is never part of a receipt. The call carries no time and no nonce (record 0021), and a question closes once, so the same call sent again is `no-open-question`.

### A fetched body is never lost

Record 0021 is silent on what `get` does when it cannot hand a body over. A body the door has handed over is no longer queued, so the command line keeps it rather than lose it.

- A body `get` cannot write to standard output is kept in `kept.jsonl`, unless `--keep` or `--all` kept it already, and `get` ends in `error file kept`, exit 3, as it already did for a body it could not write under `--out`. With `--all` it stops at the first body it cannot write. The binary ignores SIGPIPE, so a write to a closed pipe is an error `get` acts on, not a signal that kills the process.
- With `--json`, the object goes to standard output after the command's work. When it holds a body and cannot be written, `get` keeps each body it holds that it had not kept, and ends in `error file kept`, exit 3. Only `get` changes its exit code this way; what other commands do when their object cannot be written is unchanged.
- A delivery whose record in the state directory fails after the door handed it over, `delivered.jsonl`, `salts.jsonl`, or `kept.jsonl` under `--keep`, is still handed over: `client.Get` returns the delivery with the state directory's error, and `get` writes the body as it would, then prints the error. The order of the records is unchanged, and the ones after the failure are not written. `result` and `give --wait`, which fetch through `client.FetchOwn`, print such a delivery the same way, and keep it in `kept.jsonl` when its body cannot be written either, ending in `error file kept`.
- Every write to the state directory after a fetch is made under the state lock. `get --wait` holds no lock while it waits, so it takes the lock again before it writes the body; when that fails, it writes nothing more to the state directory, and ends in the lock's error.

This is the guarantee: the user still has the body, and is told what failed. A stronger one, such as keeping a body before any record is written, is a decision about the fetch contract and is not made here.

### Names and defaults

These go with the change, each recorded here as `api-compat` asks.

- `state.Error` gains `Detail`, and `state.DetailTooLarge`, `too-large`, is its one value: an `actor` or `seq` file over its 4 KiB limit was reported as a file that is not regular, `error file actor`, and is now `error state actor too-large`. The code stays `state`; no new code is added.
- `client.ConfigDir` and `local.DefaultSocketWith` treat an `XDG_CONFIG_HOME`, `XDG_RUNTIME_DIR`, `HOME` or `TMPDIR` that is not an absolute path as unset, as `local.DefaultJournalWith` already did, so no default lands under whatever directory a command was started in. Record 0024's description of both defaults reads with that rule.
- `client.Own` gains `Unkept`: with the error of a record that failed, the `kept.jsonl` line of the delivery the window handed over, which is not kept, for a caller that cannot write its body to keep it under the lock it holds.
- `local.Run`, for a window started proposed, refuses with setup's `opening-missing` before it listens when it does not hold every member's opening (setup-contract-13), as founding does once acceptances are given.

## Consequences

- A script that relied on `--help` exiting 3 now sees 0 and a usage text on standard output.
- A script that matched `error asked`, exit 3, for an answer to a closed question now sees `rejected no-open-question`, exit 2, the code `give --wait` and `result` already report for such a question. `asked` stays for a misstated part.
- `get` may exit 3 with `error file kept` where it exited 0 having lost a body, and may exit 3 with the state directory's error after printing the body, where before it printed nothing and lost it.
- `client.Get` may return a delivery and an error together. A caller must handle the delivery before the error. `client.CodeAsked`'s meaning narrows as above; its value is unchanged.
- The sentence `docs/reference/mediator-api.md` gives `asked` is regenerated from the client's error table.
