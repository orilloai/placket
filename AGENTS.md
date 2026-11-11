# Rules for building placket

This file is for anyone who builds or reviews placket: agents first, and people too. It says where things go, the rules the code keeps, how a change is made and how a reviewer judges one. `CONTRIBUTING.md` has the steps of a change in full.

Each rule has a short public name. A check in `make ci` starts its message with that name, and a reviewer's finding cites it, so `core-no-world: core/x/x.go:12: ...` points straight at the rule below. Rules for parts not built yet are stated now, each marked with the milestone that first makes it checkable. A rule a machine can check is checked by `make ci`; the rest are the reviewer's.

## Milestones

Placket v1 is built in four milestones, each resting on the one before:

- **M0.** The empty repository refuses bad work: this layout and these rules, the protocol's requirement text, schemas and register, the machine checks in `make ci`, and the conformance runner with its coverage list. No product code.
- **M1.** The core's capability packages (canonical form and signing through to receipts), each with its vectors.
- **M2.** Questions, the three answers, release and reload in the core, with the resolution vectors. Every vector passes, none pending.
- **M3.** The root module, the adapters, the binary and the demo: placket runs end to end in terminals.

"After v1" marks a rule for a part v1 does not build.

## Where things go

| Path | What goes there | From |
|---|---|---|
| `core/` | The Go core, a module of its own (`github.com/orilloai/placket/core`) with no dependency. One package per protocol capability, named for it. Nothing that touches the world. | M0 (empty); packages from M1 |
| `docs/adr/` | Decision records, `NNNN-<slug>.md` from 0001: any decision that spans packages or would be expensive to reverse, written ahead of the change built on it. | M0 |
| `tests/` | The repository's shell tests, `tests/test_*.sh`, which `make ci` runs; their fixtures under `tests/testdata/`. | M0 |
| `openspec/` | The normative requirement text and the process for changing it. | M0 |
| `spec/` | The frozen formats: schemas and the register. | M0 |
| `vectors/` | Conformance vectors, generated, and the coverage list. | M0 (format, coverage list); M1 and M2 (vectors) |
| `review/` | The review passes. The maintainer's to change. | M0 |
| `adapters/` | Doors and stores: the socket door, the journal, key files, the state directory, the log export, the command line. | M3 |
| `cmd/placket/` | The binary: `main` only. | M3 |
| `scripts/` | Scripts run by hand, such as the demo behind `make demo`. | M3 |
| `recipes/<name>/` | One job's templates, agents and plugin, on the core's exported API only. | After v1 |
| `out/` | Notes a builder leaves on a change's branch for the next round of that change. Removed before the change merges, so never on `main`. | Any change |
| `.github/workflows/` | The workflows that run `make ci`. The maintainer's alone. | Present |

At the root: `Makefile`, this file, `CONTRIBUTING.md`, `README.md`, and from M3 the root module's `go.mod`. Dependencies point one way: `cmd` to `adapters` to `core`, and `recipes` to `core`. `docs/adr/0001-repository-layout.md` and `docs/adr/0002-core-boundaries.md` record why.

Fixtures for any check go under a `testdata/` directory. Go's tools skip it, and so does `make ci`'s module loop, so a planted fault there never breaks the build.

## The rules

### Boundaries and modules

- **`core-imports`**: nothing under `core/`, test files included, imports `github.com/orilloai/placket/adapters`, `/cmd` or `/recipes`, or anything below them. Why: the core is what a vector can test with no process around it; one import upward brings the world in with it. *M0: `make ci` checks it; the reviewer reads the imports too.*
- **`core-no-deps`**: `core/go.mod` has no `require`. A dependency in the core needs a decision record first, naming the code path that uses it. Why: every byte the core writes must come from code the protocol stands behind, at a version that cannot drift. *M0: `make ci` checks it.*
- **`root-module`**: `adapters/` and `cmd/` live in one root module, `github.com/orilloai/placket`, which requires the core module and points that requirement at `./core` with a `replace` until the core is first tagged. Nobody commits a `go.work` file. Why: one module for everything that touches the world, and a build that works the same from a fresh clone. *M3, with its own decision record.*
- **`recipes-core-only`**: a recipe uses only the core's exported names, never an adapter. Why: a recipe is one job built on the protocol, and must work against any door. *After v1, with the first recipe.*
- **`core-no-world`**: no non-test file in `core/` reads the clock (`time.Now`, and `time.Since`, `Until`, `After`, `AfterFunc`, `Tick`, `NewTimer`, `NewTicker` and `Sleep`, which wait on it or measure against it), draws randomness (`crypto/rand`, `math/rand`), reads the environment (`os.Getenv`), opens a file (`os.Open`), touches the network (`net` and below), reaches the process, its files or its memory (`os`, `syscall`, `io/ioutil`, `runtime`, `unsafe`, and the packages below them), logs (`log` and below) or prints (`fmt.Print*`, `print`, `println`). The core takes the time, salts and keys as parameters. Why: output that depends on when or where it ran cannot be pinned by a vector. *M0: `make ci` checks it.*
- **`adapters-no-recheck`**: an adapter hands work to the core and does not do the core's checking again. A door never returns one of the core's rejection codes from its own code; its own codes are about the call, never the operation inside it. Why: two copies of a check drift apart, and the record would then blame the core for what the door refused. *M3.*
- **`adapters-core-types`**: an adapter package does not re-export a core type under a new name; it takes and returns the core's types, bytes and strings. Why: one name per thing, so a reader never has to map two. *M3.*

### The core's packages

- **`package-per-capability`**: each protocol capability has one package of the same name in `core/`, and the package comment names the requirement ids it implements. Why: a reader goes from a requirement to its code, and back, without a search. *M1.*
- **`packages-by-adr`**: a package beyond the planned map, a second role package, or a capability split across packages, needs a decision record before the code. Why: the package map is what the reviewers and the vectors are organised by. *M1.*
- **`mediator-no-logic`**: the mediator package holds state and operations and calls the capability packages; it holds no capability's logic. A mediator function that reads no state is misplaced, unless it reads a founding set. Why: logic in the mediator cannot be tested by a vector without a whole mediator. *M1, and M2 for the question and release operations.*
- **`trace-new-names`**: each name a change exports from `core/` has a reason on record, a requirement id or a decision, and its doc comment cites it. Why: an exported name is a promise; it needs a reason someone can check. *M1.*
- **`specs-name-behaviour`**: the requirement text names behaviour, never Go identifiers; the code names requirement ids in comments. Why: the text outlives any one implementation. *M0, with the requirement text.*

### Errors and panics

- **`core-no-panic`**: no function in `core/` panics on input. The machine half: no non-test file in `core/` calls `panic`. The other half: every exported function that takes bytes or a string has a fuzz test with a committed seed corpus, and a new entry point adds or extends one. Why: a mediator that crashes on a hostile message denies the service to every party at once. *M0: `make ci` checks the call; M1: the fuzz tests.*
- **`one-error-type`**: each package has one error type, `*Error`, whose `Code` is a constant of a stable string type, spelled as the requirement text spells it. Why: callers and vectors match on codes, never on text. *M1.*
- **`errors-as-codes`**: when input is refused, the caller gets a code, not a decoder's error wrapped up; the one wrap allowed is another placket package's `*Error`, found with `errors.As`. Why: a decoder's message quotes its input, and the input may be secret. *M1.*
- **`rejection-vs-error`**: what the caller did wrong (a protocol rejection) and what the caller's program did wrong (a tool error) are different types, and a door's own code is a third, never converted into either. Each new failure is placed in one, and the design says which. Why: the two go to different people and are recorded differently. *M1; M3 for the door's codes.*
- **`error-text-no-content`**: error text is the package and the code, plus an index, an actor or an offset where the requirement gives one; never a body, salt, seed, key, message or payload. Each error type has a table test that plants those and asserts they are absent. Why: errors end up in logs. *M1.*
- **`sentinels-uncoded`**: a sentinel error is only for a failure that is not a protocol refusal and carries no code, and is matched with `errors.Is`. Why: a coded failure matched by identity loses its code. *M1.*
- **`check-order`**: where the requirement text fixes the order of checks, the code keeps it, and a test feeds an input that fails two of them at once. Where it is silent, the design records the order chosen. Why: which code comes back is part of the API. *M1.*

### API surface

- **`bytes-at-boundaries`**: at every boundary a verifier or a vector passes through, values travel as bytes, not Go structs. A struct is fine only where neither a verifier nor a vector ever looks at the value's bytes. Why: the canonical bytes are the one value both sides agree on. *M1.*
- **`inputs-as-parameters`**: when a function's output gets hashed or signed, its time, salt and key arrive as arguments, placed after the data, and a test proves that altering any of them alters the result. Why: this is how the core stays free of the world (`core-no-world`) and stays testable. *M1.*
- **`spec-words`**: names in the code are the requirement text's words, and a verb from the text is not renamed in Go; codes keep the text's spelling. Why: one vocabulary across the text, the vectors and the code. *M1.*
- **`results-are-copies`**: a result never shares memory with internal state; a test mutates what was returned and checks the original is unchanged. Why: a caller must not be able to rewrite the mediator's state by accident. *M1.*
- **`secrets-never-print`**: unexported state is read through functions only, and any exported struct holding a secret prints the same fixed text whatever format verb is used. A test prints it every way and looks for the key, the body and the salt. Why: a secret in a log is a leak nobody notices. *M1.*
- **`constants-once`**: an exported constant that is part of a format (a type URI, a minimum length, a payload type) is defined in one place and imported everywhere else, never retyped. Why: a retyped constant is a second format waiting to drift. *M0: a later `make ci` check; it bites from M1.*
- **`test-helpers-placed`**: a `_test.go` file holds nothing production code needs, and a helper two packages' tests share lives in `core/internal/testing/`, added when a third copy would appear and not before. Why: test code should not become an unreviewed second API. *M1.*

### Tests and vectors

- **`tests-exact`**: every test compares exactly: the exact string, the exact code, `t.Fatalf` where the test cannot go on. A `strings.Contains` where the whole value is known, or `err != nil` where the code is known, is a finding. Why: a loose test passes on the wrong answer. *Any test, from M0.*
- **`raw-bytes-in-hex`**: a test about raw bytes spells them in hex or `\x` escapes and checks them by comparing bytes, never by trusting an editor. Why: editors normalise bytes silently. *M1.*
- **`vectors-independent`**: a vector's expected value never comes from the code it tests. Its generator imports no package its group tests, a check in `make ci` enforces that, and a planted fault in a tested package must change no regenerated byte. A landed vector changes only with a format version note. Why: a vector written by the code under test proves only that the code agrees with itself. *M0: a later `make ci` check; it bites from M1.*
- **`runner-no-silent-pass`**: a part of a vector the runner cannot run is never a pass. A pending part fails the run, and so does a vector the coverage list names that the runner cannot find. Why: a green run must mean every case was run. *M0, with the runner.*
- **`coverage-reported`**: coverage is reported, never gated; the number in a pull request comes from CI. Why: a gate invites tests written for the number. *M0: a later `make ci` step.*

### The door, the journal and the command line

- **`main-is-thin`**: `cmd/placket/main.go` is under 40 lines and holds nothing a test could want; every command is testable from `adapters/cli` without exiting the process. Why: code in `main` is code no test reaches. *M3; a `make ci` limit from M0's later checks.*
- **`call-signing`**: the door signs and verifies a call with the core's payload-signing functions under a media-type payload type defined once in `adapters/local`, and defines no second signing construction. Why: one signing construction is one thing to get right. *M3.*
- **`one-mediator-goroutine`**: one goroutine owns the mediator; every call reaches it over a channel, and the door's tests run with `-race`. Why: the mediator's state has one writer, so its receipts have one order. *M3.*
- **`journal-synced`**: every journal write is synced to disk before the window replies, and a failed write ends the process without a reply. Why: a reply promises a receipt that survives a crash. *M3.*
- **`door-owns-clock`**: the door reads the clock once per call, at whole seconds, and uses that one time for the call; it also mints the salts and decides what the core cannot know. Why: the core takes the world as parameters (`core-no-world`), and one time per call keeps a call's receipts consistent. *M3.*
- **`door-check-order`**: the door checks a call in a fixed order (size, parse, signature, the accord, staleness, nonce) and stops at the first failure, before the mediator sees anything. Why: an unauthenticated call must never reach the core. *M3.*
- **`door-passes-bytes`**: what the mediator receives from the door is the operation's bytes as they came out of decoding, untouched and never encoded again. Why: the core must judge what was sent, not the door's reading of it. *M3.*
- **`state-lock`**: every socket command takes the state directory's exclusive lock before reading its sequence number and keeps it until the reply is handled. Why: two commands at once must not reuse a sequence number. *M3.*
- **`journal-is-evidence`**: no code rewrites or truncates the receipts file, except removing an unterminated last line at start; the journal adapter reads, and the core's reload judges what was read. Why: the journal is the evidence of what crossed. *M3.*
- **`restart-reproducible`**: after a restart, the founding set plus the journal determine the state, and nothing else does; a window killed between calls carries on with the same receipts. Why: a restart must not change history. *M3; the core half in M2.*
- **`log-from-receipts`**: each cell the log export prints is taken from a receipt that verified, or from a kept part such a receipt covers; nothing comes from file names, the clock or lines no receipt covers. Why: the log is only as trustworthy as its weakest cell. *M3.*
- **`product-words-once`**: the words the tool prints for outcomes and every claim line are constants defined once, in the command line's words file and the export; the core never spells the outcome words the command line owns. Why: one place to read and review every word a person sees. *M0: a later `make ci` check; it bites from M3.*
- **`claims-stop-at-crossing`**: nothing placket prints speaks of what a receiver does with content after receipt, and every claim line is followed by the scope line. A test collects every printed string and checks for words such as `retain` or `consent`. Why: placket makes no claim about use after receipt. *M0: a later `make ci` check; the text in M3.*
- **`answer-call-attached`**: the door attaches the answerer's signed call to a delivery whose body is that call's answer, and nowhere else; it is never written to the receipts and is gone from the journal once fetched. Why: the receiver can verify who answered, and the journal keeps no clear text it does not need. *M3.*
- **`wait-and-result-shared`**: waiting on a proposal and asking for its result go through a single function, which prints the last line, sets the exit code and fetches. Why: two paths to one answer drift apart. *M3.*

### The decision module and release

- **`one-decision-module`**: the decision about what crosses is reached through one entry point in the composition package. No other code compares a stance, matches a term or runs the forbidden check, and a recorded verdict is never computed again, not on reload and not in the export. Why: the decision module is the seam that can be swapped; logic outside it would survive the swap. *M2.*
- **`release-in-resolve`**: content is released only by the operation that resolves a question, during the very call that carries the answer permitting it, with every receipt of that call built before any state changes. One function recognises a release from receipts, and everything else calls it. Why: a release outside the answer is content crossing that nobody agreed to. *M2.*
- **`reserved-evaluate`**: reserved for evaluating a proposal before it is sent. When it is built, it shares one matching function with the decision and restates none of its rules. Why: a second copy of the matching rules would let the preview and the real decision disagree. *After v1.*

### How changes are made

- **`machine-rules-in-ci`**: every rule a machine can check is checked by `make ci`, the one entry point, and GitHub runs it on every push and pull request. A rule no machine can check stays on the reviewer's list. Why: a rule that only a reviewer enforces is enforced only when the reviewer remembers. *M0: this file's first four checks now, the rest later in M0.*
- **`decisions-recorded-first`**: any decision spanning packages, or expensive to reverse, gets its record in `docs/adr/` ahead of the change built on it. Why: the reason is lost once the code is written around it. *M0.*
- **`change-is-one-capability`**: a protocol change is one capability, and its design lists every exported name it adds, by package and signature, so a builder never invents an entry point. Why: a reviewer can hold one capability in mind; a builder should not have to design the API. *M1, with the first capability.*
- **`deltas-stop-for-intent`**: a change to the requirement text that alters observable behaviour, a capability's scope or what it must not do stops for the maintainer. Anything else is the builder's choice, listed in the design and the pull request where the maintainer can veto it. Why: the maintainer rules intent; builders keep building. *M0.*
- **`format-change-noted`**: a change to a frozen format (a new type URI, reason or code, or a member emitted that was absent) carries a version note in the register and the vectors' README in the same change. A door format is not frozen and takes no note. Why: a verifier elsewhere must learn of a format change from the text, not from a failure. *M0, with the register.*

## Code principles

- The core is pure: its outputs are a function of its inputs. The world is the adapters' job.
- Bytes are the truth at every boundary a verifier crosses. Decode for convenience in the adapter.
- Errors are codes, never quotes of the input.
- No panic on input, anywhere in the core.
- One definition of every constant, word and check.
- Build what the requirement says, at the altitude it says. No speculative interfaces: v1 adds no interface for the decision module and no second module.

## How a change is made

In short, `CONTRIBUTING.md` in full:

- **Tests first.** A change's tests land in their own commit, marked `Loom-Lock: tests`, before the code. Each new test fails at that commit and passes at the head. Tests change only in such a commit, in plain sight.
- **Behaviour is written down before it is coded.** A change in what placket does starts in the requirement text (`openspec/`) and the vectors (`vectors/`); code alone is never where behaviour lives.
- **Changes to rules, workflows or review stand alone**, and only the maintainer merges them: this file, `CONTRIBUTING.md`, `.github/workflows/`, `review/`, and the checks in `make ci`. Never part of a feature change.
- **`make ci` passes** before a push. GitHub's run is the one that counts.
- **Decisions are recorded first** (`decisions-recorded-first`), and a change that would alter intent stops for the maintainer (`deltas-stop-for-intent`).

## How a reviewer judges a change

- A reviewer did not write the change, and reads the diff against this file. Each finding cites a rule by its public name, names the file and line, and says what is wrong.
- `make ci` passing is the floor, not the verdict. The reviewer reads what no machine checks: the imports of every file in `core/`, the placement of logic, the exactness of tests, the trace of each new name.
- The reviewer checks that no test was weakened, skipped or deleted to pass, that every test change sits in a `Loom-Lock: tests` commit, and that the change touches no check, workflow or rule unless it is that kind of change.
- From M1, a reviewer of a code change runs mutation testing and reports which broken lines the tests did not notice.
- Review stops when findings reach diminishing returns; what is left goes to a follow-up, named in the pull request.

## Known limits of the checks

The four core checks in `make ci` are line-based greps over `core/`, not a type-aware analysis:

- Every check removes comments before matching: a `//` comment to the end of its line, and a `/* */` comment, which may span lines. A name in a comment passes. Comment markers inside a string or rune literal, a raw string over several lines included, are text, and the code after the literal is checked.
- `core-no-panic` matches `panic(` as a word in code, string literals included: `"panic("` inside a string is refused. This is a known false positive; write the string another way.
- `core-no-world` refuses the packages it names by import path, under any alias, so a renamed `os` or `net` is caught. The calls of `time` and `fmt` it names are matched only under those package names: a renamed or dot import of `time` or `fmt` gets past it, and the reviewer reads the imports.
- `core-no-world` allows `fmt.Fprint*` and `fmt.Sprint*`: writing to an `io.Writer` the caller passes in is not printing, and the core cannot reach `os.Stdout` without importing `os`. A standard-library call that reaches the world inside itself, such as `time.LoadLocation` reading the time zone files, is not matched; the reviewer catches it.
- `core-no-world` matches an import path it refuses, and a call of `print(` or `println(` as a word, anywhere in code, string literals included: `"os"` in a string, or a function of the core's own named `print`, is refused. These are known false positives; spell them another way.
- `core-imports` matches the import path in any file under `core/`, wherever it appears in code, including in a string.
