# 0023. Judges run in the window, off its loop, and the core takes their judgment as an input

Status: accepted. Date: 2026-11-17. Amends records 0001, 0009, 0010, 0011 and 0012.

## Context

Record 0021 lets an Accord name judges: the card pins each judge by name, version and hash, and a `maybe` term's `resolve` lists judges in order, each with the use `decide` or `advise`. Composition takes a judge's pass or block as an input where both sides' terms would ask and both list the same judges (composition-7), and the receipt names the judge in `by`. Nothing ran a judge. A judge is a model behind a network service: it is slow, it fails, its output can be steered by the content it reads, and the content it reads leaves the machine. The core cannot run one (`core-no-world`), and the window's loop, which owns the mediator (`one-mediator-goroutine`), cannot wait on one without stalling every other party.

## Decision

### The core gains two reads, and decides nothing new

- **`mediator.Judges(caller, message, envelope) (Judging, bool)`** says which judges a give would reach. It runs every rejection row `Give` runs before composition, in `Give`'s order (membership, then `closed`, then the door's rows), and then `composition.Decide` with no judgment. It returns true only when that verdict is `escalate` and the agreed rule lists judges. `Judging` holds the part's kind, the form of its body, the body as `Question` reads it (a text's bytes, a raw body's decoded bytes, a data body's canonical bytes) and the agreed rule's judges in order. It writes nothing. Consent is decided once, in composition: the mediator matches no term and compares no list (`one-decision-module`, `reserved-evaluate`). `mediator.Question` (record 0010) is the precedent: a read the door needs, which reads state and runs the core's own rows. `Give` runs admission again when the judgment comes back, so a give whose state changed meanwhile is refused as it would be anyway. `Judging` holds a body, so it prints a fixed text under every verb.
- **`(*setup.Accord).JudgeKinds(name) []string`** returns the sorted, distinct kinds of every term, of any member, whose `resolve` lists the judge. Only a `maybe` term has a `resolve`, so it compares no stance. It is what the window prints at start: a superset of what can reach the judge, since both sides must agree.

### The judge seam is in the adapters

- **`adapters/judge`** is provider-neutral. It holds a judge's config (`Config`, read by `ReadConfig`), the interface `Judge`, a request and an answer, the stack of a rule's judges (`Stack`, `Configured`), the runners `Run` and `Advise`, and the record of each run (`Record`, written by `Line`). `Budget` is 20 seconds; `MaxOutput` is 64 KiB. Its `*Error` carries one code per cause.
- **`adapters/judge/jev`** is the one adapter of this version, behind `judge.Judge`, on `net/http` alone (record 0009). Swapping providers is one new adapter package: the runner, the core and every format stay as they are.
- **`adapters/judge/eval`** reads a file of labelled cases and runs them against a judge, and **`cmd/judge-eval`** is its thin binary, run by `make judge-eval`. A subcommand of `placket` would need a row in the protocol's command table, so the eval is a second binary, outside `make ci` and outside the release. The cases live in a new top-level directory, `evals/judge/`.

### A judge's config is a door format, hashed into the card

A config is one canonical JSON object of exactly `name`, `provider` (`jev`), `host`, `model`, `threshold` and `instruction`. Its typed hash (canonical-3) is the card's hash for that judge, and `jev.Version` is its version, so changing any byte of the config, the instruction included, is a new card and a new Accord. `ReadConfig` checks, in this order: the canonical form (`config-canonical`, a fraction included, which the canonical form refuses), the members (`config-shape`), the provider (`config-provider`), the host (`config-host`: a DNS name with an optional port, never a scheme, a path, a user or an IP literal) and the threshold, a whole number from 0 to 100 (`config-threshold`). Its callers trim one trailing line feed first, as `window` does for `--founding`. The format is not frozen.

`placket card --default --judge FILE...` lists the configs in flag order in the card's `judges`. `placket window --judge FILE...` compares them with the card at start, before anything is written to the journal: every judge the card lists needs exactly one config whose hash is the card's and whose version is `jev.Version`, and every config must be one the card lists. Otherwise the window prints `error founding judge` and starts nothing. A window that cannot take a give, `window close` and a window on a closed journal, skips the comparison and needs no `--judge`.

### The runner: decide off the loop, then write the receipt

When a give would be asked and its agreed rule lists a `decide` judge, the loop marks the caller busy, starts `judge.Run` on a goroutine it owns, and goes on serving. `Run` asks the `decide` judges in order within one budget for the whole stack, and settles on one judgment: the first pass or block, or none. Any error, an answer below the config's threshold, and a judge still running at the budget are none. The goroutine hands the judgment back on a channel; the loop calls `Give` with it and with the time the door read when the give arrived (`door-owns-clock`), journals the receipt, writes the run's records, replies and clears the busy mark. A `Give` that then rejects drops the records. A raw body is never sent: each listed judge's record is none, `raw-body`, and the give is asked.

`advise` judges run after the receipt is written, for every give whose agreed rule lists one, whatever decided it, but never on a raw body. Each runs within its own budget on an owned goroutine, its records come back on the same channel and are appended, and the window prints `advise <name> <pass|block|none> <receipt id>`. An advise judge decides nothing and delays nothing.

Stopping the window cancels every judge run; `Run` returns once every goroutine has ended, and nothing more is written. A judged give cut so gets no receipt.

### The door: `judge-busy`, and the reply's own deadline

A caller's second `give` while its first is with a judge is refused with the door code `judge-busy`. Envelope `seq` must rise (crossing-4), so a later give must never reach the core before an earlier one; queuing would hand the core the caller's gives out of order whenever the first is refused. The check is about the order of the caller's calls, never about the operation inside one, so it is the door's code and not a rejection (`adapters-no-recheck`). It runs in `handle` right after the six checks of `door-check-order`, the nonce last, so the refused call's nonce is spent and a retry is a new signed call, and before the tick, so the refused give writes nothing. Only a `give` is refused: the busy caller's `get`, `questions`, `result` and `resolve` go on. A party's own command line holds its state lock across a call, so it never meets this.

The connection deadline the door sets is `readDeadline`, 30 seconds. Before writing a reply, the door sets a fresh write deadline of `replyMargin`, 5 seconds, so a slow call does not eat the reply's time, and `judge.Budget` plus `replyMargin` is below `readDeadline`.

### The judge's output is content, kept apart

Each judged receipt's records go in `judged/<hex of the receipt id>.jsonl` in the journal directory, mode 0600, one canonical line per judge run: `{"abstained"?,"error"?,"judge","outcome","output"?,"outputBase64"?,"score"?,"use"}`. Output that is not valid UTF-8 is written as standard base64 in `outputBase64`. The decide lines are written after the receipt and before the reply; the advise lines are appended later. A failed append is a failed journal write (`journal-synced`). Nothing of a judge goes in a receipt beyond what record 0021 defined, and no reader of receipts reads `judged/`: `journal.Read`, reload, `verify`, `log`, `export` and `reconcile` never open it, and removing it changes nothing they print.

### The key, and what leaves the machine

The Jev key is the user's own credential. It is read once, from `PLACKET_JEV_KEY`, when the window or the eval starts, held only in memory, and written nowhere. With no key, the adapter sends nothing and the judge says none. A request carries the part's kind, its body (a text's text, a data body's canonical JSON) and the config's instruction, with the config's model and the key as its credential, and nothing else of the part, the message, the envelope or the Accord. It goes over HTTPS, with verified TLS, to the config's host only: the transport uses no proxy from the environment and follows no redirect, and a reply is read up to `MaxOutput`, strictly. At start the window prints, for each card judge, which kinds may go to which host and model, or that it sends nothing.

## Amendments

- **Record 0001, the layout.** A new top-level directory, `evals/`, holds the hand-run evals' cases; `cmd/` gains `cmd/judge-eval`.
- **Record 0009, the root module.** The packages `adapters/judge`, `adapters/judge/jev` and `adapters/judge/eval`, and the binary `cmd/judge-eval`, join the module. Still nothing but the standard library is required. `cli.Env` gains `Getenv`; `cmd/placket` passes `os.Getenv`, and a nil `Getenv` is no key.
- **Record 0010, the door.** The door code `judge-busy`, after the six checks. The connection deadline and the reply's margin as above. A judged give is answered once its judges settle, within the budget.
- **Record 0011, the journal.** `judged/` joins the layout, as above.
- **Record 0012, the kernel hash.** `placket card --default` takes `--judge`, and the window compares the card's judges with its `--judge` configs, as it compares the kernel.

## Consequences

- A judged give takes up to the budget to answer, and its receipt's `at` is when it arrived, so its receipt can follow one with a later `at`.
- A judge's pass is a model's. Text inside the part can steer it. The terms bound where a judge may act, and the eval measures how often an injection passes; neither proves it cannot.
- Model output may quote the body, and lies in clear in `judged/`, which may be removed at any time.
- What a provider keeps of a request is the provider's terms, not a claim of placket's.
- A second provider is a new adapter package, a new provider value in `ReadConfig`, and the window's version check against that adapter's version.
