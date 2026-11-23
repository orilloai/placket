# Changelog

Every change to placket that a user can see is listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions will follow [Semantic Versioning](https://semver.org/) from the first tag.

A user-visible change adds its line under `Unreleased` in the same pull request. For an outside contribution, the reviewer or the project's own agents add the line; a contributor is never asked to.

This file covers the tool and its Go API. Changes to the protocol's frozen formats carry their own version notes, in the register (`spec/frozen-formats.md`) and the vectors' notes (`vectors/README.md`).

## Unreleased

The first release. Placket mediates the parts that cross between parties under the terms each signed, asks a person where the terms say to, and signs a receipt for every decision, which anyone can verify.

### Try it in one command

- `placket init approve`, `merge` or `handoff` founds an example Accord for one person in one command: every key, mode 0600, in a directory of mode 0700; each party's sealed terms, binding and acceptance of its own view; the founding set; and one labels config entry per party, `<label>-<party>`. It prints each Accord's id and parties, the window's command line, the `claude mcp add` line of the agents, the `placket watch` line of the person, and the lines that fetch the example's applier and run it. The fetch line takes the applier at the commit of the binary's version tag when the build carries that commit, else at the tag itself, which can be moved, and at `main` for a build with no version tag. It never writes into a directory that holds anything (`dir-not-empty`) or over a label the config holds (`label-taken`).
- Three guides, `docs/guides/approve.md`, `merge.md` and `handoff.md`, take each example from `placket init` to a verified receipt, with no judge's key; the README gains an install section and a quickstart that leads with approve before apply.
- `install.sh`, a release asset, installs the signed release for this machine in one line, `curl -fsSL https://github.com/orilloai/placket/releases/latest/download/install.sh | sh`: it checks the signature on `checksums.txt` with `cosign`, the archive against its line there, and the provenance with `gh` when `gh` is installed and signed in, says which checks it ran and skipped, installs nothing when one fails, and puts `placket` in `~/.local/bin` without `sudo`, through a file `mktemp` makes in that directory. `--version vX.Y.Z` pins a release; `PLACKET_RELEASE_BASE` takes only an `https://` URL or a `file://` directory, and every download is held to its scheme. A download cut short runs nothing.
- `install.sh` installs nothing when it cannot check the signature on `checksums.txt`, for want of `cosign`, unless `PLACKET_INSTALL_UNSIGNED=1` says to go on, and then ends by saying the binary's origin was not checked; it takes only the archive of the version `--version` names, from `PLACKET_RELEASE_BASE` too, refuses an empty `--version`, and replaces an installed `placket` only with a binary whose `placket version` is that archive's. The README's block that checks `install.sh` before it runs it stops at the first step that fails, in a new directory, and its verify of the demo journal writes the Accord's id out.
- A Homebrew cask for macOS and Linux, `brew install --cask orilloai/tap/placket`, from the release's archives at their checksums; each release attaches it, and the tap takes it from the published release.
- Two skills, in `skills/`, for any agent that reads `SKILL.md`: `placket-setup` installs placket with the person's yes and sets up a pattern, fetched at the binary's release tag, or an Accord of their own, ending with the line that adds `placket mcp` to their agent; `placket-scout` reads a repository or product description, only locally, and lists where an agent's action cannot be undone or leaves the system, with the gate and what its receipts would prove. A Claude Code plugin bundles them: `/plugin marketplace add orilloai/placket`, then `/plugin install placket@placket`.
- Each release archive holds `NOTICE` beside `LICENSE` and `README.md`.
- `placket --help` lists the commands, and `--help`, `-help` or `-h` on any command prints its usage on standard output, its flags one a line, and exits 0; with `--json` it prints one object of the command's flags. This corrects commands-1, under which `--help` was the error `usage`; `docs/adr/0012-the-mediator-api-and-the-command-line.md` says why.
- `placket version` prints which build the binary is: its version, its commit and the commit's time when the build carries them, the Go release it was built with, and the decision module's version and hash.
- `patterns/guarded-export/`: a receiver's guard exports a value only when exactly that value was released to it, and refuses a record that a later release of the same record replaced, before it appends it.
- `patterns/local-apply/`: an applier writes a file only with the text the deciding role accepted, or the version it changed it to, and nothing for a decline.

### Found an Accord

- Each party declares and signs its terms, and seals them with `placket seal`: the founding set holds the seal, and the opening, which holds the terms, goes only to whoever runs the window. `placket founding --seal ... --binding ...` builds the set and prints its Accord id.
- `placket window --openings DIR` starts an Accord proposed: it shows each member only its own view, what will cross for it, through `placket view`, takes its signed acceptance through `placket accept`, and refuses every other call with the door code `not-founded`. Once every member has accepted, it founds the Accord and writes `acceptances.json` beside the founding set, which `--acceptances` and a restart take.
- A principal binds each party to its roles with `placket bind --policy FILE`, under a policy that says which roles each principal may grant. A policy may name, in `supersedes`, the earlier Accord and the close it replaces.
- `placket card --default` prints the card of the decision module the binary runs, and a window refuses a card that names another.
- `placket key new` writes a key file atomically, mode 0600, and never over a file; a key file that another user can read or write, or owns, is refused with `key-mode`.
- A window with no `--socket` or `--journal` takes its Accord's default socket and journal, and a window given its default socket makes the socket's directory. An `XDG_` variable, `HOME` or `TMPDIR` that is not an absolute path counts as unset for the default socket, the labels config and the default journal alike.
- A window started proposed, with no acceptances, refuses to start without every member's opening, `opening-missing`, before it listens.
- `placket window` and `placket window close` read an opening as a key file is read: one that others may read or write, that another user owns, or that is over 1 MiB is `error file opening`, before the window listens.

### Ask, answer and release

- A part the terms ask about is held, and a person in the role the terms name accepts it, changes it or declines it with `placket answer`, `change` or `decline`. An accept releases the part as it was sent; a change releases the person's version and returns it to the asker; a decline releases nothing. `placket give --wait` and `placket result` wait for the outcome.
- Each answer is one call the answerer signs, over the answer's salted digest under a salt it mints. The call travels inside every delivery it releases, and the journal keeps every call it took. `--no-release` signs an answer that asks for no release, and `answer --keep` keeps the call and its salt.
- `placket answer`, `change` and `decline` first read every page of the questions the window shows, and stop with `error asked`, signing nothing, when the part it shows is not the one its ask's receipt names. A question the window does not show goes to the window, whose refusal is printed: `rejected no-open-question` for one already closed, `rejected not-an-answerer` for a key that does not answer it. `placket cancel` is the asker's withdrawal of its own question.
- Every salt is exactly 32 bytes, and placket mints 32. A text or a flag that is not valid UTF-8 is refused, never repaired.
- A part may carry its `mediaType`, which its delivery carries.
- A body `placket get` fetched is never lost. One it cannot write to standard output, as to a closed pipe, or to its `--out` file is kept in `kept.jsonl`, and `get` ends in `error file kept`; with `--json`, so is each body an object it cannot write holds. A delivery whose record in the state directory cannot be written is still printed, then the error. `placket watch` ends, `error file`, once its standard output closes.

### Run agents, and several mediators

- `placket mcp` is an MCP server on standard input and output, with the tools `give`, `get`, `questions`, `result`, `cancel` and `mediators`, across the mediators its `--label`s name, each with its own key. It never answers a question. It refuses a label that has accepted no view, or whose kept acceptance is not signed by its key for that Accord, and one whose key answers questions unless started with `--allow-answering`.
- The labels config, `mediators.json` in placket's config directory, names each mediator a user reaches; `placket mediators` lists it, and every socket command takes `--label` in place of `--key`, `--socket` and `--accord`.
- `placket watch` prints each new question once, as it opens, with the lines that answer it. `questions`, `get` and `result` take `--wait N`, up to 60 seconds, and a call that waits holds no lock.
- The mediator API: the door's calls and replies, written as JSON Schemas under `api/`, with worked calls, recorded replies and a reference generated from them, `docs/reference/mediator-api.md`. `adapters/client` is the client the command line and `placket mcp` share.
- `patterns/notifier/`: each new question runs one command of yours, on `placket watch`.
- `patterns/github-merge/`: an agent's merge on GitHub happens only through `apply.sh`, after placket released it and the deciding role accepted it, at exactly the head commit released. The applier posts the delivery as a comment on the pull request and sets a `placket` commit status naming its receipt, and an auditor follows a merge commit back to `placket verify`. The auditor passes a status only when `verify`, over the journal's receipts without its answer calls, witnessed the comment's own call for its receipt, and that receipt's predicate is a release of a merge to the applier that passed. `custody.md` lists the GitHub ruleset, the accounts and the credentials the applier rests on. `apply.sh` says why it stopped: `apply: interrupted at pr <N>` on INT or TERM, and which post failed when GitHub takes the comment or the status and not the other.

### Judges

- A `maybe` term may list judges, each to `decide` or to `advise`, where the terms would ask a person, and the card lists the judges a term may name; founding refuses an unknown judge with `judge-unknown`. Where both sides list the same judges, the window asks the `decide` judges in order within 20 seconds, and the first pass or block decides the part; otherwise the part is asked. `advise` judges run after the receipt and decide nothing. A judge never answers a question and never runs at release.
- v0.1.0's one judge provider is Jev, with your own key from `PLACKET_JEV_KEY`. A Jev reply whose confidence is `null` is unparsable, so the part falls through to a person, never a score of 0. `placket card --default --judge FILE` lists a judge's config in the card, and `placket window --judge FILE` runs only the configs its card lists. At start the window prints which kinds each judge may be sent and to which host.
- A caller's give while its last give is with a judge is refused with the door code `judge-busy`. What each judge said is kept in the journal's `judged/` directory, never in a receipt.
- `make judge-eval JUDGE=FILE` runs the synthetic labelled cases of `evals/judge/` against a judge's config, by hand, with your key; `patterns/shadow-judge/` runs a judge to advise beside the approver.

### Verify, reconcile and export

- Every receipt names who decided: the terms, a judge of the card, an answer call by its hash, or the mediator with its event.
- `placket verify --accord A --founding F` checks every receipt's signature and chain and that each names this Accord, its card and its members, and prints `valid: <n> receipts signed by mediator <key>`. With every member's opening (`--open`), as in a dispute, it checks each seal, each view and conformance: `conforms: every decision is one this Accord let its decider make`. Without them it says the terms were not checked.
- `placket verify --view V`: a party checks its own receipts against the view it accepted, with no other party's terms.
- `placket verify` says which key witnessed each answer, checks a subset with `--subset` and one delivery's body with `--delivery`, and prints `complete` for a whole journal that ends in its close.
- `placket show` prints what a receipt claims and who decided; `placket log` exports the journal's rows, and `placket reconcile` compares each party's records with the journal.
- A state directory's `actor` or `seq` file over 4 KiB is `error state <file> too-large`, and a state file replaced whose directory could not be synced is `error state <file>`.
- Every journal, state file, door format, handed file and reply is read strictly: a repeated member, ill-formed UTF-8 or a member name in another case is refused, and a journal or state file must be a regular file.
- `placket verify` prints `<receipt id> contradicted: the call given for it does not fit` for an answer whose call given does not fit its receipt, and for a release after such an answer; such a receipt fails `witness` and is never printed `witnessed` or `unwitnessed`.
- `placket verify --view` with `--subset` is `usage`; with no receipt of the viewer's in the list, the view line says none was checked; a view that is not the one accepted is `error founding acceptance-view` before any validity code; and a formed part with no row in the view matches only as `block` `no-match`.
- `placket verify --json` names its number of receipts `receiptCount`, not `verified`; in `show` and `released`, `verified` stays the boolean `false`.
- `placket verify --delivery` prints the `body` line also when the log does not conform, and a delivery with an empty or absent `receiptId` is `error delivery`.
- A closing with reason `closed` at another time than the first such closing fails `close` at its own line; the requirement text now says so (close-4).
- `placket reconcile` refuses one party's kept records given twice: `error verify kept --kept N actor`.
- `placket verify` prints `claim: each receipt is this mediator's signed decision about one part under this Accord's terms.` only when it checked every decision against the terms and the log conforms; otherwise, with `--mediator` alone, another module, sealed terms or a failure, it prints `claim: each receipt and close is signed by this mediator.`, and `--json`'s `claim` holds the line printed.
- `placket verify --delivery` reads the file as a delivery first: one with a member missing or a member a delivery never holds is `error delivery`, exit 3. The `body` line says the body's bytes are the receipt's part, not their form or that the part passed.
- `placket verify` on a journal with no receipt checks no binding's `until`, whatever the clock says, as the core does; it used to found at the wall clock.
- The requirement text says a part's digest names its body's bytes, not the form they crossed in, and that a part may hold `mediaType` beside its body and `metadata` (crossing-2, crossing-3); decision record 0013 says why.
- A receipt verifies only when its payload is exactly the canonical bytes of its statement: a payload with added whitespace, members in another order or a number spelled another way, even signed by the mediator, is `error verify not-canonical` in `verify`, `log` and `show`, and `show` prints no receipt id for it. The envelope around the payload may be written in any spelling the canonical form accepts (receipt-1; `docs/adr/0013-what-a-verifier-checks-and-says.md`).
- `--json` turns on the object in every spelling of true, such as `--json=1`, even when a flag cannot be read; `give --wait --json` and `result --json` always hold `deliveries` beside `fetched`, `[]` when the wait fetched no other delivery.

### Close and rotate

- `placket window close --accord ID` closes the Accord of a stopped window's journal: it closes every open question and writes a close receipt that pins the last receipt of every chain and lists every delivery not yet fetched. A closed Accord never reopens, and once a member's queue is empty every `placket get` returns the close.
- `placket verify --superseded-by NEW --successor-accord ID --successor-acceptances FILE` checks that a successor Accord pins the journal's close: the successor is the one whose id the reader gives from its own record, it is founded with its members' acceptances, and it is founded at the close's time, so the result does not change with the day it is run. `docs/guides/rotation.md` rotates a mediator's key by closing the Accord, founding its successor in a directory of its own, running its window until every member has accepted, and checking the old journal against it; `tests/test_rotation.sh` runs every command of it.

### Go API

- The core's packages, one per protocol capability: `canonical`, `signing`, `setup`, `founding`, `composition`, `mediator`, `answer`, `receipts`, `conform` and the conformance runner. `mediator.SaltBytes` is 32, the length of every salt.
- The core is its own module, tagged `core/v0.1.0`: a Go project requires it with `go get github.com/orilloai/placket/core@v0.1.0`. Its packages outside `internal/` promise "0.x, may change"; the root module's packages, under `adapters/` and `cmd/`, are not a supported Go API. The README's Go section holds a program that verifies receipts with `receipts.Verify`. The root module requires the tagged core, with no `replace`, so the binary builds against the core a Go user gets.
- `conform.FoundedAt` gives the time a founding set is founded at for a list of receipts: the first receipt's, or with none the earliest, so no binding's `until` is checked.
- Removed before v0.1.0, each deprecated first, so no release carries them: `mediator.MinSaltBytes`, which was equal to `mediator.SaltBytes`; `mediator.Action` and its constants `Accepted`, `Changed`, `Declined` and `Cancelled`, use `answer.Action` and its constants; `mediator.CodeSalt`, `CodeAnswer` and `CodeAction`, which nothing returned; `setup.CheckUnknown`, `setup.Check` and `setup.Term.Checks`, which founding never returned, built or set; `local.Window.Rand`, which the window never read; `local.Reply.Salt` and `local.Reply.AnswerCall`, never set, since the answer call is inside the delivery; and `local.Call.Action`, never set, since the action is inside the signed call.

### The repository

- Placket's code and its requirement text are under the Apache License 2.0 (`LICENSE`); `NOTICE` names its copyright holders.
- `spec/protocol.md`, the requirement text, with its frozen formats and their register; the conformance vectors and their coverage list; and the decision records in `docs/adr/`.
- `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue forms and a pull request template; `docs/security/threat-model.md`, what placket protects and what it does not; and `docs/rules/`, every rule placket's code keeps, each checked by `make ci` where a machine can check it.
- `make demo` runs the worked example, and its journal is committed under `scripts/demo/`, which anyone can verify.
- `docs/README.md` lists every page of the docs, in order, by section: a quickstart per entry point, the concepts, the guides, the references and how to verify a journal. New pages: the concepts, from the Accord to what placket does not do; the guides to Claude Code over MCP, judges and shadow mode, several mediators, writing an applier, and custody; and `docs/verify.md`, what each line `placket verify` prints means.
- `docs/reference/cli.md`, every command's usage, flags, `--json` requirements and exit codes, and the cause and fix of every code the command line prints, is generated from the command line's own command table, `go run ./adapters/cli/gen`, so it cannot drift from the code. `--help` prints each flag with the sentence the reference gives it, from the same table.
