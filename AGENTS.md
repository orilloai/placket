# Rules for building placket

This file is for anyone who builds or reviews placket: agents first, and people too. It says where things go, the rules the code keeps, how a change is made and how a reviewer judges one. `CONTRIBUTING.md` has the steps of a change in full.

Each rule has a short public name. A check in `make ci` starts its message with that name, and a reviewer's finding cites it, so `core-no-world: core/x/x.go:12: ...` points straight at the rule below. Rules for parts not built yet are stated now, and each applies from the first change that builds its part. A rule's closing note says what checks it beyond the reviewer. A rule a machine can check is checked by `make ci`, or, for `red-green-tests`, by a job of its own beside it; the rest are the reviewer's.

## Where things go

| Path | What goes there |
|---|---|
| `core/` | The Go core, a module of its own (`github.com/orilloai/placket/core`) with no dependency. One package per protocol capability, named for it. Nothing that touches the world. |
| `docs/adr/` | Decision records, `NNNN-<slug>.md` from 0001: any decision that spans packages or would be expensive to reverse, written ahead of the change built on it. |
| `tests/` | The repository's shell tests, `tests/test_*.sh`, which `make ci` runs; their fixtures under `tests/testdata/`. |
| `tools/guard/` | The checker: one Go program, a module of its own, whose rules are the checks of `make ci` that read the tree. Its Go tests prove each rule's planted faults; their fixtures are under `tools/guard/testdata/`. A check, changed only in a rules change. |
| `tools/redgreen/` | The check `red-green-tests`: one Go program, a module of its own, standard library only, that compares a change's head with its base on `main`. Its Go tests build small git repositories from the fixtures under `tools/redgreen/testdata/`. Changed only in a rules change; the workflow runs `main`'s copy. |
| `spec/` | The protocol: `protocol.md`, the normative requirement text, one document organized by operation; `schema/` and `frozen-formats.md`, the frozen formats' schemas and the register of what binds each. |
| `vectors/` | Conformance vectors, generated, and the coverage list. |
| `review/` | The review passes. The maintainer's to change. |
| `adapters/` | Doors and stores: the socket door, the journal, key files, the state directory, the log export, the command line. |
| `cmd/placket/` | The binary: `main` only. |
| `scripts/` | Scripts run by hand, such as the demo behind `make demo`. |
| `recipes/<name>/` | One job's templates, agents and plugin, on the core's exported API only. |
| `patterns/` | Runnable ways to use placket: one directory per caller's job, `patterns/<job>/`, each a template to copy that uses the `placket` binary and `jq` only, with what a run prints; `patterns/README.md` states their shape and lists them. `make ci` runs each one (`tests/test_patterns.sh`). Not part of the requirement text or the frozen formats' register. |
| `out/` | Notes a builder leaves on a change's branch for the next round of that change. Removed before the change merges, so never on `main`. |
| `.github/workflows/` | The workflows that run `make ci`. The maintainer's alone. |

At the root: `Makefile`, this file, `CONTRIBUTING.md`, `README.md`, and, once it exists, the root module's `go.mod`. Dependencies point one way: `cmd` to `adapters` to `core`, and `recipes` to `core`. `docs/adr/0001-repository-layout.md` and `docs/adr/0002-core-boundaries.md` record why.

Fixtures for any check go under a `testdata/` directory. Go's tools skip it, and so does `make ci`'s module loop, so a planted fault there never breaks the build.

## The rules

### Boundaries and modules

- **`core-imports`**: nothing under `core/`, test files included, imports `github.com/orilloai/placket/adapters`, `/cmd` or `/recipes`, or anything below them. Why: the core is what a vector can test with no process around it; one import upward brings the world in with it. *`make ci` checks it; the reviewer reads the imports too.*
- **`core-no-deps`**: `core/go.mod` has no `require`. A dependency in the core needs a decision record first, naming the code path that uses it. Why: every byte the core writes must come from code the protocol stands behind, at a version that cannot drift. *`make ci` checks it.*
- **`root-module`**: `adapters/` and `cmd/` live in one root module, `github.com/orilloai/placket`, which requires the core module and points that requirement at `./core` with a `replace` until the core is first tagged. Nobody commits a `go.work` file. Why: one module for everything that touches the world, and a build that works the same from a fresh clone. *`make ci` checks it. The root module comes with its own decision record.*
- **`recipes-core-only`**: a recipe uses only the core's exported names, never an adapter. Why: a recipe is one job built on the protocol, and must work against any door.
- **`core-no-world`**: no non-test file in `core/` reads the clock (`time.Now`, and `time.Since`, `Until`, `After`, `AfterFunc`, `Tick`, `NewTimer`, `NewTicker` and `Sleep`, which wait on it or measure against it), draws randomness (`crypto/rand`, `math/rand`), reads the environment (`os.Getenv`), opens a file (`os.Open`), touches the network (`net` and below), reaches the process, its files or its memory (`os`, `syscall`, `io/ioutil`, `runtime`, `unsafe`, and the packages below them), logs (`log` and below) or prints (`fmt.Print*`, `print`, `println`). The core is Go alone: no cgo (`import "C"`), and no assembly, C, C++ or other non-Go source file anywhere in `core/` outside `testdata/`, since those reach the world where no check of Go can see. The core takes the time, salts and keys as parameters. Why: output that depends on when or where it ran cannot be pinned by a vector. *`make ci` checks it.*
- **`adapters-no-recheck`**: an adapter hands work to the core and does not do the core's checking again. A door never returns one of the core's rejection codes from its own code; its own codes are about the call, never the operation inside it. Why: two copies of a check drift apart, and the record would then blame the core for what the door refused.
- **`adapters-core-types`**: an adapter package does not re-export a core type under a new name; it takes and returns the core's types, bytes and strings. Why: one name per thing, so a reader never has to map two.

### The core's packages

- **`package-per-capability`**: each protocol capability has one package of the same name in `core/`, and the package comment names the requirement ids it implements. Why: a reader goes from a requirement to its code, and back, without a search.
- **`packages-by-adr`**: a package beyond the planned map, a second role package, or a capability split across packages, needs a decision record before the code. Why: the package map is what the reviewers and the vectors are organised by.
- **`mediator-no-logic`**: the mediator package holds state and operations and calls the capability packages; it holds no capability's logic. A mediator function that reads no state is misplaced, unless it reads a founding set. Why: logic in the mediator cannot be tested by a vector without a whole mediator.
- **`trace-new-names`**: each name a change exports from `core/` has a reason on record, a requirement id or a decision, and its doc comment cites it. Why: an exported name is a promise; it needs a reason someone can check.
- **`specs-name-behaviour`**: the requirement text names behaviour, never Go identifiers; the code names requirement ids in comments. Why: the text outlives any one implementation. *`make ci` checks it.*

### Errors and panics

- **`core-no-panic`**: no function in `core/` panics on input. The machine half: no non-test file in `core/` calls `panic`. The other half: every exported function that takes bytes or a string has a fuzz test with a committed seed corpus, and a new entry point adds or extends one. Why: a mediator that crashes on a hostile message denies the service to every party at once. *`make ci` checks the call; the reviewer, the fuzz tests.*
- **`one-error-type`**: each package has one error type, `*Error`, whose `Code` is a constant of a stable string type, spelled as the requirement text spells it. Why: callers and vectors match on codes, never on text.
- **`errors-as-codes`**: when input is refused, the caller gets a code, not a decoder's error wrapped up; the one wrap allowed is another placket package's `*Error`, found with `errors.As`. Why: a decoder's message quotes its input, and the input may be secret.
- **`rejection-vs-error`**: what the caller did wrong (a protocol rejection) and what the caller's program did wrong (a tool error) are different types, and a door's own code is a third, never converted into either. Each new failure is placed in one, and the design says which. Why: the two go to different people and are recorded differently.
- **`error-text-no-content`**: error text is the package and the code, plus an index, an actor or an offset where the requirement gives one; never a body, salt, seed, key, message or payload. Each error type has a table test that plants those and asserts they are absent. Why: errors end up in logs.
- **`sentinels-uncoded`**: a sentinel error is only for a failure that is not a protocol refusal and carries no code, and is matched with `errors.Is`. Why: a coded failure matched by identity loses its code.
- **`check-order`**: where the requirement text fixes the order of checks, the code keeps it, and a test feeds an input that fails two of them at once. Where it is silent, the design records the order chosen. Why: which code comes back is part of the API.

### API surface

- **`bytes-at-boundaries`**: at every boundary a verifier or a vector passes through, values travel as bytes, not Go structs. A struct is fine only where neither a verifier nor a vector ever looks at the value's bytes. Why: the canonical bytes are the one value both sides agree on.
- **`inputs-as-parameters`**: when a function's output gets hashed or signed, its time, salt and key arrive as arguments, placed after the data, and a test proves that altering any of them alters the result. Why: this is how the core stays free of the world (`core-no-world`) and stays testable.
- **`spec-words`**: names in the code are the requirement text's words, and a verb from the text is not renamed in Go; codes keep the text's spelling. Why: one vocabulary across the text, the vectors and the code.
- **`results-are-copies`**: a result never shares memory with internal state; a test mutates what was returned and checks the original is unchanged. Why: a caller must not be able to rewrite the mediator's state by accident.
- **`secrets-never-print`**: unexported state is read through functions only, and any exported struct holding a secret prints the same fixed text whatever format verb is used. A test prints it every way and looks for the key, the body and the salt. Why: a secret in a log is a leak nobody notices.
- **`constants-once`**: an exported constant that is part of a format (a type URI, a minimum length, a payload type) is defined in one place and imported everywhere else, never retyped. Why: a retyped constant is a second format waiting to drift. *`make ci` checks it.*
- **`test-helpers-placed`**: a `_test.go` file holds nothing production code needs, and a helper two packages' tests share lives in `core/internal/testing/`, added when a third copy would appear and not before. Why: test code should not become an unreviewed second API.

### Tests and vectors

- **`tests-exact`**: every test compares exactly: the exact string, the exact code, `t.Fatalf` where the test cannot go on. A `strings.Contains` where the whole value is known, or `err != nil` where the code is known, is a finding. Why: a loose test passes on the wrong answer.
- **`raw-bytes-in-hex`**: a test about raw bytes spells them in hex or `\x` escapes and checks them by comparing bytes, never by trusting an editor. Why: editors normalise bytes silently.
- **`vectors-independent`**: a vector's expected value never comes from the code it tests. Its generator imports no package its group tests, a check in `make ci` enforces that, and a planted fault in a tested package must change no regenerated byte. A landed vector changes only with a format version note. Why: a vector written by the code under test proves only that the code agrees with itself. *`make ci` checks it.*
- **`runner-no-silent-pass`**: a part of a vector the runner cannot run is never a pass. A pending part fails the run, and so does a vector the coverage list names that the runner cannot find. Why: a green run must mean every case was run. *The runner enforces it.*
- **`coverage-reported`**: coverage is reported, never gated; the number in a pull request comes from CI. Why: a gate invites tests written for the number. *`make ci` reports it.*

### The door, the journal and the command line

- **`main-is-thin`**: `cmd/placket/main.go` is under 40 lines and holds nothing a test could want; every command is testable from `adapters/cli` without exiting the process. Why: code in `main` is code no test reaches. *`make ci` checks its length.*
- **`call-signing`**: the door signs and verifies a call with the core's payload-signing functions under a media-type payload type defined once in `adapters/local`, and defines no second signing construction. Why: one signing construction is one thing to get right.
- **`one-mediator-goroutine`**: one goroutine owns the mediator; every call reaches it over a channel, and the door's tests run with `-race`. Why: the mediator's state has one writer, so its receipts have one order.
- **`journal-synced`**: every journal write is synced to disk before the window replies, and a failed write ends the process without a reply. Why: a reply promises a receipt that survives a crash.
- **`door-owns-clock`**: the door reads the clock once per call, at whole seconds, and uses that one time for the call; it also mints the salts and decides what the core cannot know. Why: the core takes the world as parameters (`core-no-world`), and one time per call keeps a call's receipts consistent.
- **`door-check-order`**: the door checks a call in a fixed order (size, parse, signature, the accord, staleness, nonce) and stops at the first failure, before the mediator sees anything. Why: an unauthenticated call must never reach the core.
- **`door-passes-bytes`**: what the mediator receives from the door is the operation's bytes as they came out of decoding, untouched and never encoded again. Why: the core must judge what was sent, not the door's reading of it.
- **`state-lock`**: every socket command takes the state directory's exclusive lock before reading its sequence number and keeps it until the reply is handled. Why: two commands at once must not reuse a sequence number.
- **`journal-is-evidence`**: no code rewrites or truncates the receipts file, except removing an unterminated last line at start; the journal adapter reads, and the core's reload judges what was read. Why: the journal is the evidence of what crossed.
- **`restart-reproducible`**: after a restart, the founding set plus the journal determine the state, and nothing else does; a window killed between calls carries on with the same receipts. Why: a restart must not change history.
- **`log-from-receipts`**: each cell the log export prints is taken from a receipt that verified, or from a kept part such a receipt covers; nothing comes from file names, the clock or lines no receipt covers. Why: the log is only as trustworthy as its weakest cell.
- **`product-words-once`**: the words the tool prints for outcomes and every claim line are constants defined once, in the command line's words file and the export; the core never spells the outcome words the command line owns. Why: one place to read and review every word a person sees. *`make ci` checks it.*
- **`claims-stop-at-crossing`**: nothing placket prints speaks of what a receiver does with content after receipt, and every claim line is followed by the scope line. A test collects every printed string and checks for words such as `retain` or `consent`. Why: placket makes no claim about use after receipt. *`make ci` checks string literals; the reviewer reads the rest.*
- **`answer-call-attached`**: the door attaches the answerer's signed call to a delivery whose body is that call's answer, and nowhere else; it is never written to the receipts and is gone from the journal once fetched. Why: the receiver can verify who answered, and the journal keeps no clear text it does not need.
- **`wait-and-result-shared`**: waiting on a proposal and asking for its result go through a single function, which prints the last line, sets the exit code and fetches. Why: two paths to one answer drift apart.

### The decision module and release

- **`one-decision-module`**: the decision about what crosses is reached through one entry point in the composition package. No other code compares a stance, matches a term or runs the forbidden check, and a recorded verdict is never computed again, not on reload and not in the export. Why: the decision module is the seam that can be swapped; logic outside it would survive the swap.
- **`release-in-resolve`**: content is released only by the operation that resolves a question, during the very call that carries the answer permitting it, with every receipt of that call built before any state changes. One function recognises a release from receipts, and everything else calls it. Why: a release outside the answer is content crossing that nobody agreed to.
- **`reserved-evaluate`**: reserved for evaluating a proposal before it is sent. When it is built, it shares one matching function with the decision and restates none of its rules. Why: a second copy of the matching rules would let the preview and the real decision disagree.

### How changes are made

- **`machine-rules-in-ci`**: every rule a machine can check is checked by `make ci`, the one entry point, and GitHub runs it on every pull request and every push to `main`. The one exception is `red-green-tests`, which compares a change with `main` and so runs in a job of its own beside `ci`. A rule no machine can check stays on the reviewer's list. Why: a rule that only a reviewer enforces is enforced only when the reviewer remembers. *`make ci` checks that every tool it runs is pinned to an exact version, and that its `ci` target names every rule of the checker and every step.*
- **`red-green-tests`**: every test a change adds or changes fails without the change's code and passes with it, and no test is removed or skipped unexplained, however the branch's commits were cut. The check reads two commits only: the base, the merge base with `main`, and the head. It runs each added and changed test at the base, with the head's test files laid over it, and then at the head. It refuses an added test green on base, a removed or newly skipped test, a test that does not run and pass at the head, and a change to non-test Go code under `core/`, `adapters/`, `cmd/`, `recipes/` or `tools/` with no test added or changed. A test that does not build at the base counts as red, and is printed as red (build). Three trailers on any commit of the change excuse an honest exception, each with its reason: `Green-On-Base: <test>: <why>`, `Test-Weakened: <test>: <why>` and `No-Test: <why>`. A trailer that names a test the change does not touch is refused. A changed test green on base, a test behind a build tag, changed helpers and every excuse are reported for the reviewer. Why: a test proves the code only if it fails without it, and a removed or skipped test proves nothing; the shape of a branch's commits proves neither. *The workflow's `red-green-tests` job checks it on every pull request and every push to `main`, with `main`'s copy of `tools/redgreen/`, so a change cannot alter its own judge; `make red-green-tests` runs it from a checkout. `docs/adr/0013-red-green-tests.md` records why and what it cannot see.*
- **`decisions-recorded-first`**: any decision spanning packages, or expensive to reverse, gets its record in `docs/adr/` ahead of the change built on it. Why: the reason is lost once the code is written around it.
- **`change-is-one-capability`**: a protocol change is one capability, and its design lists every exported name it adds, by package and signature, so a builder never invents an entry point. Why: a reviewer can hold one capability in mind; a builder should not have to design the API.
- **`deltas-stop-for-intent`**: a change to the requirement text that alters observable behaviour, a capability's scope or what it must not do stops for the maintainer. Anything else is the builder's choice, listed in the design and the pull request where the maintainer can veto it. Why: the maintainer rules intent; builders keep building.
- **`format-change-noted`**: a change to a frozen format (a new type URI, reason or code, or a member emitted that was absent) carries a version note in the register and the vectors' README in the same change. A door format is not frozen and takes no note. Why: a verifier elsewhere must learn of a format change from the text, not from a failure.
- **`ci-stays-fast`**: `make ci` runs each step once, and every step prints its time. `make ci` exports a marker, `PLACKET_IN_CI=1`, to every step, and a `make ci` started where the marker is already set stops before any step, whatever `-k` says, printing `ci-stays-fast: refused: make ci inside make ci`. No script or test clears the marker: no empty assignment, `unset`, `env -u PLACKET_IN_CI`, `env -i`, `os.Unsetenv`, `os.Clearenv`, or Go `Env` list that does not start from `os.Environ()`. Setting it to a value, or reading it, is fine. Why: a test that runs the whole of `make ci` again multiplies its time, and a writer pays for `make ci` twice a round. *`make ci` checks it: the `Makefile` refuses the nested run, and the checker refuses a `.sh` or `_test.go` file outside `testdata/` that clears the marker. The workflow's timeout is the hard limit.*
- **`schemas-hold`**: every frozen format's schema compiles as JSON Schema draft 2020-12 from the tree, with nothing fetched; its accepted examples validate and its refused examples are refused; and every schema file is named by a row of the register. Why: a schema no example tests, or that the register does not name, drifts from the text unseen. *`make ci` checks it.*

## Code principles

- The core is pure: its outputs are a function of its inputs. The world is the adapters' job.
- Bytes are the truth at every boundary a verifier crosses. Decode for convenience in the adapter.
- Errors are codes, never quotes of the input.
- No panic on input, anywhere in the core.
- One definition of every constant, word and check.
- Build what the requirement says, at the altitude it says. No speculative interfaces: v1 adds no interface for the decision module and no second module.

## How a change is made

In short, `CONTRIBUTING.md` in full:

- **Tests first.** Write a change's tests before its code. `red-green-tests` shows each test the change adds or changes red against `main` and green at the head, whatever the commits look like. `Green-On-Base`, `Test-Weakened` and `No-Test` trailers state an honest exception, each with its reason, in plain sight.
- **Behaviour is written down before it is coded.** A change in what placket does starts in the requirement text (`spec/protocol.md`) and the vectors (`vectors/`); code alone is never where behaviour lives.
- **Changes to rules, workflows or review stand alone**, and only the maintainer merges them: this file, `CONTRIBUTING.md`, `.github/workflows/`, `review/`, and the checks in `make ci`. Never part of a feature change.
- **`make ci` passes** before a push. GitHub's run is the one that counts.
- **Decisions are recorded first** (`decisions-recorded-first`), and a change that would alter intent stops for the maintainer (`deltas-stop-for-intent`).

## How a reviewer judges a change

- A reviewer did not write the change, and reads the diff against this file. Each finding cites a rule by its public name, names the file and line, and says what is wrong.
- `make ci` passing is the floor, not the verdict. The reviewer reads what no machine checks: the imports of every file in `core/`, the placement of logic, the exactness of tests, the trace of each new name.
- The reviewer reads `red-green-tests`'s report: judges every trailer's reason, each test red only by not building (red (build)), each changed test green on base and each changed helper. The reviewer checks that no test was weakened, skipped or deleted to pass, and that the change touches no check, workflow or rule unless it is that kind of change.
- A reviewer of a code change runs mutation testing and reports which broken lines the tests did not notice.
- Review stops when findings reach diminishing returns; what is left goes to a follow-up, named in the pull request.

## What `make ci` prints

Every step of `make ci` is a target of its own, so `make <rule>` runs one check. The checks that read the tree are the rules of the checker in `tools/guard/`, built once by the step `guard`.

- A check of the tree prints `<rule>: ok` when it passes. When it refuses, every line it prints starts `<rule>: <file>`, with `:<line>` where the fault has a line, or is an indented detail line under one, and its last line is `<rule>: refused: <why> (AGENTS.md, <rule>)`. `schemas-hold` first prints one count line per schema that holds, `schemas-hold: <schema>: ok, <n> accepted, <n> refused`.
- A toolchain step (`vet`, `lint`, `vulnerabilities`, `secrets`, `race`, `fuzz-seeds`, `coverage-reported`) prints `<step>: ok`, or `<step>: <module>: refused` after the tool's own output. `race`, `fuzz-seeds` and `coverage-reported` read one shared test run per module, the step `tests`, with the race detector and coverage, that also runs every fuzz test on its seed corpus.
- Every step then prints its time on a line of its own, `time: <step>: <n>s`, whether it passed or refused. No time fails a step.
- The two lines that name no file are the time lines and `ci-stays-fast: refused: make ci inside make ci`.

## Known limits of the checks

The checks that read the tree are rules of one Go program, `tools/guard/` (`docs/adr/0007-one-checker.md`). It reads Go with the standard library's parser and resolves names without types; it reads `go.mod` files, the `Makefile`, scripts and the requirement text as lines. What that sees, and what it still misses:

- Every rule reads every `.go` file in its scope, whatever its build constraints, and refuses a file it cannot parse, at the line of the error; no file is skipped. Code `gofmt` would rewrite fails `make ci` anyway, in the `modules` step, which reads every Go file of a module outside `testdata/`.
- The checker does not follow a symbolic link, and the Go tool does. So every check of the core, and `product-words-once`, refuses a link anywhere under `core/`, `core/` and `core/go.mod` included, and `vectors-independent` refuses one under `vectors/`.
- An import path written with an escape, such as `"\x6fs"` for `os`, is refused by `core-imports` and `core-no-world` whatever it spells, and by `vectors-independent` in a generator: a reader of the text would not see the path the Go tool reads.
- A comment is never code, and a string is never an import: `"panic("`, `"os"` or an adapters path inside a string that is not an import path passes. `core-no-world` refuses cgo (`import "C"`) and every non-Go source file outside `testdata/`.
- `core-no-world` refuses the packages it names by import path, under any name. The calls of `time` it names, and `fmt.Print*`, are refused wherever they are referred to, called or not, through whatever name the file imports `time` or `fmt` under; a dot import of either is refused itself. `os.Getenv` and `os.Open` are matched by the name `os` too. Names are resolved without types, so a local variable that shadows an import's name is read as the package: a false positive, and the fix is another name.
- `print`, `println` and `panic` are refused as calls of the built-ins. A call through a selector, such as a field `s.print(...)`, passes, and so does a call of a function the package declares at its top level under that name, in a file of the same package clause that every build includes: no `//go:build` or `// +build` line, no system or architecture suffix such as `_linux` or `_amd64`, and a name that does not start with `_` or `.`. A name declared only in a file some build leaves out does not count, and the call stays refused, since that build calls the built-in. So a package that declares its own `print` only for some systems has its calls refused: a false positive. A parameter or local variable of that name is read as the built-in: a false positive too.
- `core-no-world` allows `fmt.Fprint*` and `fmt.Sprint*`: writing to an `io.Writer` the caller passes in is not printing, and the core cannot reach `os.Stdout` without importing `os`. A standard-library call that reaches the world inside itself, such as `time.LoadLocation` reading the time zone files, is not matched; the reviewer catches it.
- `vectors-independent` reads a generator's imports, and refuses a string literal that spells the path of a package its group tests. A package reached through another package is not seen (`vectors/README.md`, Known limits).
- `constants-once` and `claims-stop-at-crossing` read string literals as written, escapes included; a constant built from pieces, or a word spelled with an escape, is not seen. `product-words-once` reads a literal's value.
- `machine-rules-in-ci` reads each `go run` and `go install` as written, and a tool named by a `make` variable from its assignment in the same file; a version spelled through a shell variable or another program is not seen. It reads `ci`'s prerequisites as literal target names: a variable there names nothing.
- `ci-stays-fast` reads each line of a `.sh` or `_test.go` file. It misses a clear spelled through a variable (`unset "$v"`) or another program, and a `make ci` reached in some way that loses the environment; the workflow's timeout backstops it. A Go `Env` list set on a line that does not name `os.Environ()` is refused, even when the list was built from it elsewhere: build it on that line.
