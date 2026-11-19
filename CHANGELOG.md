# Changelog

Every change to placket that a user can see is listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions will follow [Semantic Versioning](https://semver.org/) from the first tag.

A user-visible change adds its line under `Unreleased` in the same pull request. For an outside contribution, the reviewer or the project's own agents add the line; a contributor is never asked to.

This file covers the tool and its Go API. Changes to the protocol's frozen formats carry their own version notes, in the register (`spec/frozen-formats.md`) and the vectors' notes (`vectors/README.md`).

## Unreleased

### Added

- `placket verify --view V`, beside the founding set and its acceptances: a party checks its own receipts against the view it accepted, with no other party's terms. It reads every give the party sends or receives and the closing and release of every question it asked or received, prints one line per receipt with the row it was read against, and a line that every one matches; a receipt that does not match exits 1 with `does not conform: view-conformance`, a view that is not the one the party accepted is `error founding acceptance-view`, and `--view` beside the party's own `--open` is a usage error. `conform.Evidence` gains `View`, and `conform.Report` `Viewer` and `Viewed` (decision record 0026).
- Sealed terms: a party's terms stay sealed from the other parties. `placket seal` seals a party's signed declaration under a salt it mints, prints the seal entry `placket founding --seal` takes, and writes the opening, which only whoever runs the window holds. `placket window --openings DIR` starts an Accord proposed: it serves each member only its own view, what will cross for it, through `placket view`, and takes its signed acceptance through `placket accept`, refusing every other call with the door code `not-founded`; once every member has accepted, it founds the Accord and writes `acceptances.json` beside the founding set, which `--acceptances` and a restart take. `placket verify` founds with the acceptances and without the openings, and says the terms were not checked; `--open` checks an opening against its seal and, with every opening, each view against its acceptance, and refuses a second opening of one actor or an opening of an actor that is not a member. `placket mcp` reads what a label's key answers from the view it accepted, and refuses a label that has accepted none (decision record 0025).
- The mediator API gains `view` and `accept`, which `placket mcp` does not list.
- The mediator API: the door's calls and replies, with `wait` and the questions reply's `cursor`, written as JSON Schemas under `api/`, a wire layer and a request layer, with worked signed calls, recorded replies and a reference generated from them, `docs/reference/mediator-api.md`. `questions`, `get` and `result` may wait at the door up to 60 seconds; a `get` that waits replies `queued` and takes nothing. `adapters/client` is the client the command line and `placket mcp` share (decision record 0024).
- `placket mcp`, an MCP server on standard input and output: the tools `give`, `get`, `questions`, `result`, `cancel` and `mediators`, across the mediators its `--label`s name, each with its own proposing key. `placket watch` prints each new question once, as it opens, with the lines that answer it, and `placket mediators` lists the labels config, `mediators.json` in placket's config directory. Every socket command takes `--label` in place of `--key`, `--socket` and `--accord`, and `questions`, `get` and `result` take `--wait N`; a call that waits holds no lock. A window with no `--socket` or `--journal` takes its Accord's default socket and journal.
- `patterns/notifier/`: each new question runs one command, on `placket watch`.
- Judges run: where both sides' `maybe` terms list the same judges, the window asks the `decide` judges in order within 20 seconds, and the first pass or block decides the part; every error, a score below the threshold and a judge still running at the budget say none, and the part is asked. `advise` judges run after the receipt and decide nothing. v0.1.0's one judge provider is Jev, with the user's own key from `PLACKET_JEV_KEY` (decision record 0023).
- `placket card --default --judge FILE` lists judges' configs in the card; `placket window --judge FILE` runs only the configs its card lists, at their hashes, or prints `error founding judge` and starts nothing. At start the window prints which kinds each judge may be sent and to which host, and `advise <name> <outcome> <receipt id>` for each advise judge's result.
- The door code `judge-busy`: a caller's give while its last give is with a judge is refused, so its gives reach the mediator in order.
- The journal's `judged/` directory holds what each judge said of a receipt's part, one line per run; no reader of receipts needs it.
- `make judge-eval JUDGE=FILE` runs the synthetic labelled cases of `evals/judge/` against a judge's config, by hand, with the user's key.
- `patterns/shadow-judge/`: a judge that advises beside the approver. A pattern may hold data files of its own.
- `docs/adr/0023`: judges.
- `placket window close --key F --founding S --journal DIR --accord ID [--socket P]` closes the Accord of a stopped window's journal: it closes every open question, a timeout where its deadline passed and `closed` otherwise, and writes a close receipt, signed by the mediator under `https://placket.tech/close/0.1`, that pins the last receipt of every chain and lists every delivery not yet fetched. With `--socket` it then serves the closed Accord. A close whose receipt would not fit in one reply writes nothing and is tool error `journal`, `receipts.jsonl`, `close-size`; it can run again once members fetch (decision record 0022).
- A closed Accord never reopens: a give or an answer is rejected with `closed`, a restarted window comes up closed and prints `accord <id> closed <close id>`, and once a member's queue is empty every `placket get` returns the close and keeps it as `close.json`.
- `placket verify` prints `complete: against close receipt <id>; ...` for a whole journal that ends in its close, and fails `close` or `after-close` for a close that does not match its journal or a receipt after it. `--superseded-by NEW` checks that a successor Accord's `supersedes` pins the journal's close.
- `docs/guides/rotation.md`: rotate a mediator's key by closing the Accord and founding a successor that pins the close.
- `placket log` and `show` read the closing word `closed`, `by mediator close`; the log names the close, and `show` prints a close's chains and unfetched deliveries.
- `docs/adr/0022`: the close.
- Every receipt names who decided in its predicate's `by`: the terms, a judge of the card, an answer call by its typed hash, or the mediator with its event (decision record 0021).
- An answer is one signed call, under `https://placket.tech/answer/0.1`: `placket answer`, `change`, `decline` and `cancel` each sign one, over the answer's salted digest under a salt the answerer mints. The call travels inside every delivery it queues, and the journal keeps every accepted call in `answers.jsonl`.
- `placket verify --accord A --founding F` takes the mediator from the founding set whose typed hash is A, and checks conformance to the Accord: it prints `valid`, `conforms` and `complete` lines, one line per decision the terms did not make, and the custody line. `--answers`, `--subset` and `--delivery` witness answers, check a subset and check one delivery's body.
- `placket answer --no-release` and `change --no-release` sign an answer that asks for no release; `answer --keep` keeps the call and the salt it signed.
- `placket show` prints a receipt's decider on the line after its verdict, and `--json` holds it as `by`.
- A maybe term may list the judges it grants, each to decide or to advise, and the card lists the judges a term may name; founding refuses an unknown judge with `judge-unknown`.
- A part may carry `mediaType`, which its delivery carries; a policy may carry `supersedes`.
- Go packages `core/answer` and `core/conform`.
- `docs/adr/0021`: who decided, the answer call, judges as a format, and conformance in verify.
- `mediator.SaltBytes`, 32: the length of every salt (decision record 0020).
- `docs/adr/0020`: every placket URI under `placket.tech` with one protocol number, salts of exactly 32 bytes, and A2A's meaning of role.
- `placket version` prints which build a binary is: its version as Go stamped it, the commit and its time, the Go release and the decision module; `--json` prints the same as one object.
- `docs/adr/0016` to `0019`: verified receipts, the door's own code, releases and `placket version`, and `Reload`'s order.
- The door replies `door-internal` under its `door` member when it fails itself (its salt source, encoding a reply, or an error the mediator does not type), never the mediator's `internal`.
- `key-mode`: a key file that another user can read or write (any of mode 0077), or that another user owns, is refused when it is loaded. A key file that is not a regular file is `key-read`.
- Two checks in `make ci`: `recipes-core-only`, and action pins in `machine-rules-in-ci`.
- `LICENSE`: placket's code and its requirement text are under the Apache License 2.0.
- `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue forms and a pull request template, for outside contributors and security reporters.
- `docs/security/threat-model.md`: what placket protects, its security properties, assumptions, non-goals and the limits accepted for this version.
- `docs/rules/`: every rule placket's code keeps, each under its public name, with `docs/style/` and `docs/review.md`.
- Two checks in `make ci`: `no-hidden-text` and `agents-md-short`.

### Changed

- `placket founding` takes `--seal` entries in place of `--declaration`, and a founding set holds each declaration's seal, so every Accord id changes; `placket window` needs `--openings`, and on a founded journal the acceptances. The worked example's journal under `scripts/demo/` is written again, with its acceptances and, for the demo only, its openings.
- `placket verify` prints its `valid:` line, with the count and the mediator, after the counts, in place of `verified <n> receipts`, and its `--json` object's `checks` is `judges`. A `--delivery` that is not a delivery, or whose receipt is none of the receipts, is tool error `delivery`.
- `placket cancel` is the asker's: an answerer's cancel is refused with `not-an-answerer`.
- `placket answer`, `change` and `decline` check that the window shows the question and that it is the part its ask's receipt names before they sign, and stop with `error asked` when it does not or is not, so an answer to a question that is closed, or that the key cannot answer, stops there. `placket questions` prints only the questions that pass the same check, and ends with `error asked` when it left one out.
- The window mints no salt: an answer's salt is the answerer's. The answer's text, version and salt that do not fit its call are refused with `answer-body`, in place of the `salt` and `answer` errors.
- `log`'s `answer_signed` reads the journal's `answers.jsonl` and the calls inside kept deliveries, and says `yes` only for a call that fits its closing: signed by the closing's `from` for an accept or a change, by the asker for a cancel, and by any key for a decline, which any answerer may sign.
- The card's `checks`, the term's `checks` and the reason `check:<name>` are gone; the decision module's version is `0.2.0`.
- Every placket URI is under `https://placket.tech/`: every schema `$id`, the type URIs of declarations, bindings and envelopes, the receipt's predicate type and the crossing extension. Journals, state and receipts made under `https://placket.dev/` are refused like any foreign one.
- A salt is exactly 32 bytes, and placket mints 32: `give`, the window's answers, and every salt a give, an answer, a reloaded delivery, `get`, `released` and `log --kept` read.
- `placket give` writes `ROLE_USER`.
- A give with `role` `ROLE_AGENT` is refused with `extension-missing`.
- `placket key` writes a key file atomically: to a temporary file, synced, then linked into place, so a crash never leaves half a key. It still never overwrites an existing file.
- `bind` requires `--policy FILE`, and an Accord's policy gives each principal the roles it may grant (decision record 0014).
- Text and flags that are not valid UTF-8 are refused, never repaired: `give`, `answer` and `change --text` give tool error `text`, `decline` and `change --comment` give `comment`, and `give --to`, `--kind`, `--re` and `--topic`, `--question` on `answer`, `change`, `decline`, `cancel` and `result`, and `bind --actor`, `--role` and `--mediator` give `usage`.
- `--accord` must be an Accord id, `sha256:` and 64 lowercase hex characters, before any path is built from it; otherwise `usage`.
- `give --as raw --text` is `usage`: `--text` gives a text part only.
- `verify --founding` on a journal with no receipts checks the founding set and that its mediator is `--mediator`, with no Accord to compare, where it gave `error founding` before.
- The client refuses a reply longer than `MaxReply`, four times `MaxCall`, with its `reply` error, and the window sends none: past `MaxReply` the questions reply shows a text or data part by its size alone, and leaves out the newest questions that do not fit even so. A journal, state file or key file placket reads must be a regular file: a journal or state file that is not one is tool error `file`, naming it. Each line of a journal or state file is read with a cap of `MaxReply`.
- Every door format, handed file and reply is read strictly: a repeated member, ill-formed UTF-8 or a member name in another case is refused.
- `receipts.Verify` returns `[]receipts.Verified`, `receipts.Releases` takes it, and `receipts.Build` returns the receipt and its `Verified` statement in place of its id (decision record 0016).
- `mediator.Reload` takes its data first, then the times and the key: `Reload(founding, list, open, deliveries, foundedAt, key, at)` (decision record 0019).
- `reconcile` refuses a kept actor that is not an actor reference, before anything else, and never prints it.
- A state error's text is `state: <code>`; the path is no longer in it.
- A refusal from a check of `make ci` now ends by linking the rule's heading, `(docs/rules/<file>.md#<rule>)`, in place of `(AGENTS.md, <rule>)`.
- `AGENTS.md` is a short entry file that points to the rules.

### Deprecated

- `mediator.MinSaltBytes`, now equal to `mediator.SaltBytes`; removed at the release.

### Removed

- The reserved registration type URI and `signing.TypeRegistration`: placket signs objects of three kinds.
