# 0001. The repository: its layout, its modules and one normative source

Status: accepted. Date: 2026-11-23.

## Context

Placket is a mediator for content that crosses between parties, with a Go core that conformance vectors test byte for byte, and adapters that connect it to sockets, files and a command line. It is built by many hands, human and agent, and read by people who check a receipt holding only the mediator's public key. Each of them needs to know where each kind of thing goes, and what says what placket does. A layout that grows by accident makes the checks impossible to write: a rule such as "the core imports nothing from the adapters" needs a fixed place for each. And if what placket does is spread over code, comments, design notes and pull requests, each reader assembles a different answer, and the vectors end up stating whatever the code happened to do.

## Decision

### The layout

The repository has these top-level directories. Each is created by the change that first puts something in it.

| Directory | What it holds |
|---|---|
| `core/` | The Go core: a module of its own, `github.com/orilloai/placket/core`, with no dependency, one package per protocol capability (record 0002). Nothing here touches the world. |
| `adapters/` | The world: the socket door, the journal, key files, the state directory, the log export, the judges and the command line. Root module. |
| `cmd/` | The binaries: `cmd/placket`, `main` only, and `cmd/judge-eval`, the judges' eval, run by hand (record 0009). |
| `api/` | The mediator API: the schemas of the door's calls and replies, their Go package, their two generators and their test data (record 0012). |
| `recipes/<name>/` | One job's templates, on the core's exported API alone. None yet: recipes come after v1. |
| `spec/` | The protocol: `protocol.md`, the normative requirement text; `schema/`, the frozen formats' schemas with their examples; `frozen-formats.md`, their register; `ids.txt` and `reserved.txt`, the id lists; and a README. |
| `vectors/` | Conformance vectors, generated independently of the code they test, and the coverage list that maps each requirement to its vectors. |
| `patterns/` | Runnable templates for a caller's job, using the `placket` binary and `jq` only, and `gh api` for `github-merge` alone. A pattern may hold an `apply.sh` its `pattern.sh` calls, and a stand-in for an outside tool with its fixtures. `make ci` runs each. |
| `docs/` | `docs/README.md`, the docs tree; `docs/adr/`, these decision records, `NNNN-<slug>.md`, written before the change that relies on them; `docs/rules/`, every rule once under its public name (record 0004); `docs/reference/`, generated reference pages; `docs/style/`, `docs/security/`, `docs/review.md` and the guides. |
| `evals/` | The cases of the evals run by hand, such as `evals/judge/` (record 0009). |
| `tests/` | The shell tests `make ci` runs, `tests/test_*.sh`, with fixtures under `tests/testdata/`. |
| `tools/` | `tools/guard/`, the checker behind the checks of `make ci` that read the tree, and `tools/redgreen/`, the checks `red-green-tests` and `outside-commits-signed-off` (record 0004). |
| `scripts/` | Scripts run by hand or by a workflow, such as the demo behind `make demo`, each with its test in `tests/`. |
| `.github/` | Workflows, templates and code owners. |
| `review/` | The review passes a reviewer runs over a change, the maintainer's to change. None yet. |
| `out/` | A builder's notes for the next round of a change, on its branch. |

- A change that adds a directory not named here, or puts something in the wrong one, needs a new record first.
- `.github/workflows/` is the maintainer's: no other change edits it.
- The root holds `Makefile`, `AGENTS.md`, `CONTRIBUTING.md` and `README.md`, and is a Go module, `github.com/orilloai/placket`, holding `adapters/` and `cmd/`. No `go.work` file is committed.
- Dependencies point one way: `cmd` uses `adapters`, `adapters` use `core`, and `recipes` use `core`. Nothing lower uses anything higher. The mediator API's packages join that direction (below).
- `make ci` is the one entry point for every check. It finds every Go module in the repository and formats, vets and tests each, except a module under a `testdata/` directory, which is a fixture, and a module with no Go packages yet.
- Fixtures for any check live under a `testdata/` directory, which Go's tools and `make ci` both skip, so a planted fault never breaks the real build.
- `out/` holds a builder's notes on a change's branch only. It is removed before the change merges, and is never on `main`.

### One normative source

- Placket's behaviour has one normative source: `spec/protocol.md`, one document organised by operation. Each section opens with the operation's ordered check table and then states its requirements, each under its id, each with scenarios. The formats it defines are held beside it: their JSON Schemas in `spec/schema/`, with accepted and refused examples, and their register, `spec/frozen-formats.md`. `spec/README.md` says what each part holds.
- No other document, repository or program is an authority on what placket does. Code, comments, decision records, the vectors and pull requests follow the text; where any of them disagrees with it, the text is right and the other is a defect. A change in behaviour is made in the text first.
- Requirement ids are permanent. An id names one requirement in every version and is never reused or renumbered. The ids in use are listed in `spec/ids.txt`, and the ids reserved for later versions, or never used, in `spec/reserved.txt`. `make ci` checks the text against both (`specs-name-behaviour`) and every schema against its examples and the register (`schemas-hold`).
- The text names behaviour, never an implementation's identifiers, so it outlives any one implementation.
- An external standard is cited by its name and version, and the text says which part of it applies: JSON (RFC 8259), UTF-8 (RFC 3629), UTF-16 code units as Unicode 16.0 defines them, SHA-256 (FIPS 180-4), Ed25519 (RFC 8032), base64 (RFC 4648), timestamps as a profile of RFC 3339, DSSE 1.0, the in-toto Statement v1, the A2A protocol 1.0 message format, and JSON Schema draft 2020-12.

### The root module

- The root module requires the core module. Until the core is first tagged, it requires `v0.0.0` and replaces it with `./core`. Once the core is tagged, it requires a tagged version, with no `replace` (record 0014 says how the core is tagged). The core, the vectors' generators and the checkers stay modules of their own.
- The root module requires nothing but the core: it uses the standard library only. The socket is a unix socket and the locks are `flock`, from the standard library's `net` and `syscall`, so the window and the commands run on Linux and macOS, the systems that have both. Windows is not a target of this version.
- Its packages, one per thing that touches the world:

  | Package | What it holds |
  |---|---|
  | `adapters/keyfile` | Key files: a seed as 64 lowercase hex digits and a line feed, mode 0600. Read, write, reference. |
  | `adapters/state` | A party's state directory (record 0011). |
  | `adapters/journal` | The window's journal: the directory a stopped mediator is made again from (record 0011). |
  | `adapters/local` | The door: the call and the reply, the server and the socket client, the door's codes, and the window that runs the mediator behind the door (record 0011). |
  | `adapters/client` | The mediator API's client and the labels config (record 0012). |
  | `adapters/mcp` | `placket mcp` (record 0012). |
  | `adapters/errtext` | The sentence of every code `placket mcp` returns and the command line prints (record 0012). |
  | `adapters/export` | The log: one row per question, from verified receipts and covered kept parts, as JSON and CSV (record 0011). |
  | `adapters/judge`, `adapters/judge/jev`, `adapters/judge/eval` | The judges' seam, its one adapter, and the eval (record 0009). |
  | `adapters/cli` | The commands, one file each; `table.go`, the command table; and `words.go`, which holds every product word and claim line. |
  | `adapters/cli/gen` | The CLI reference's generator (record 0012). |
  | `adapters/internal/strict` | One reader of the door formats' JSON objects, by their members' exact names: the standard library's decoder matches a member to a field without regard to case. |
  | `adapters/internal/lines` and `adapters/internal/seeds` | A capped reader of the files placket is handed, and the adapters' fuzz seeds (record 0011). |
  | `adapters/internal/pointer` | The value at a JSON Pointer inside a body, for `released --at`. |
  | `cmd/placket` | `main` only: it passes the system clock, `crypto/rand`, the signal context and `os.Getenv` to `adapters/cli`, ignores SIGPIPE, and exits with the code a command returns (`main-is-thin`). |
  | `cmd/judge-eval` | The eval's thin binary (record 0009). |
  | `tests/firsthour` | The first-hour test's scripted MCP client (record 0012). |

- The mediator API adds packages outside `cmd`, `adapters` and `core`:

  | Package | What it holds |
  |---|---|
  | `api` | The schemas, embedded; it imports nothing of placket. |
  | `api/gen` | The reference's generator: `api` and `adapters/errtext`. |
  | `api/internal/gen` | The worked calls' generator: the standard library alone. |

  Its import edges: `adapters/cli` imports `adapters/client` and `adapters/mcp`; `adapters/mcp` imports `adapters/client`, `api`, `adapters/errtext` and `adapters/export` (for a closing's word); `adapters/client` imports `adapters/local`, `adapters/state`, `adapters/keyfile`, `adapters/internal/strict` and core packages, and no adapter it imports imports it back. `api`'s external test package imports `adapters/local`, `adapters/client`, `adapters/mcp` and core packages. `adapters/cli/gen` imports `adapters/cli`, for its command table, and `adapters/errtext`. `docs/rules/modules.md` names these edges.
- No adapter re-exports a core type (`adapters-core-types`): each takes and returns the core's types, bytes and strings. A kept answer call is checked in one place, `core/answer`'s reader (record 0009), which the log, the client and the command line all call.
- Every command is a function of its arguments, its streams and an environment that carries the clock, the randomness, the interrupt and the environment's variables, so its tests run it without a process of its own. The few tests that need processes, two commands at once and a window killed with SIGKILL, run one binary that `adapters/cli`'s tests build once, with the race detector when the tests run under it.

### The files an open project has

The repository holds `LICENSE` (Apache-2.0, for the code and the requirement text), `CODE_OF_CONDUCT.md`, `SECURITY.md`, `CHANGELOG.md`, issue forms and a pull request template, and `docs/style/`, `docs/review.md` and `docs/security/`.

## Consequences

- A builder can tell from the path alone whether a file is core, adapter, binary, recipe, text, format or vector, and the checks in `make ci` can rely on it: a check on `core/` does not need to guess where the core ends.
- The core stays a module with no dependency beside the root module, so a verifier that needs only the core never pulls in an adapter, and nothing under `core/` can import the adapters.
- A fresh clone builds the binary with `go build ./cmd/placket`. Until the core is first tagged, nothing is fetched but the toolchain; once it is tagged, the build also fetches the tagged core, which the root's `go.sum` pins.
- A builder, a vector author and a verifier read the same text and reach the same result. A vector's expected code is derived from a check table, and a reviewer can check the derivation without reading code. A behaviour that exists only in code is a finding, and so is a code the text does not define.
- The schemas are checked on every run of `make ci`, so the formats cannot drift from their examples unseen.
- Changing what placket does costs a change to the text, and to a format's register row when a format changes, in the same change as the code. That is slower than changing code alone, and it is the point. An id freed by a removed requirement stays reserved, so the lists only grow.
- Supporting Windows later means a door on another kind of socket and another kind of lock, behind the same packages.
