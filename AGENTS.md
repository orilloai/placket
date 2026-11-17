# placket

Placket is a protocol, and its Go reference implementation, for mediating content that crosses between parties and signing a receipt for every decision. It is built mostly by agents; a human maintainer, the code owner of everything, approves every change.

This file is the entry point for anyone who builds or reviews placket, agents first. It holds the commands, the map, what is true of every change, and where to read before you change a part. Each rule lives once, under its public name, in `docs/rules/`; a check of `make ci` starts its message with that name and ends it with a link to the rule.

## Commands

- `make ci`: every check, the one entry point. `make <step>` runs one step, named as `make ci` prints it.
- `make red-green-tests BASE=origin/main`: every test the change adds or changes is red on `main` and green at the head. It reads commits, so commit first.
- `make demo`: the worked example, run by hand.
- Go 1.27 (toolchain go1.27.4); `jq` and `sh` for the shell tests and patterns.
- GitHub runs `make ci` and `red-green-tests` on every pull request and every push to `main`; that run is the one that counts.

## The map

- `core/`: the Go core, its own module with no dependency; one package per protocol capability. Nothing here touches the world.
- `adapters/`: the world: the socket door, the journal, key files, the state directory, the log export, the command line. Root module.
- `cmd/placket/`: the binary, `main` only, under 40 lines.
- `recipes/<name>/` (none yet): one job's templates, on the core's exported API only.
- `spec/`: the protocol. `protocol.md` is the normative requirement text; `schema/` and `frozen-formats.md` are the frozen formats and their register.
- `vectors/`: conformance vectors, generated independently of the code they test, and the coverage list.
- `patterns/`: runnable templates for a caller's job, using the `placket` binary and `jq` only; `make ci` runs each.
- `docs/adr/`: decision records, `NNNN-<slug>.md`, written before the change that relies on them.
- `docs/rules/`: every rule, once, under its public name. `docs/style/`, `docs/security/`, `docs/review.md`: style, security, review.
- `tests/`: the shell tests `make ci` runs, `tests/test_*.sh`, with fixtures under `tests/testdata/`.
- `tools/guard/`: the checker behind the checks of `make ci` that read the tree. Changed only in a rules change.
- `tools/redgreen/`: the `red-green-tests` check. Changed only in a rules change; GitHub runs `main`'s copy.
- `scripts/`: scripts run by hand, such as the demo.
- `.github/`: workflows, templates and code owners. Workflows are the maintainer's alone.
- `review/` (none yet): the review passes, the maintainer's to change.
- `out/`: a builder's notes for the next round of a change; removed before it merges.

Fixtures for any check go under a `testdata/` directory, which Go's tools and `make ci` skip.

## Always true, on every change

- Dependencies point one way: `cmd` to `adapters` to `core`, and `recipes` to `core` (`core-imports`, `root-module`).
- The core is pure: it takes the time, salts and keys as parameters, and never reads the world (`core-no-world`).
- Behaviour lands in `spec/protocol.md` and the vectors before code (`specs-name-behaviour`, `vectors-independent`).
- Every test a change adds or changes is red against `main` and green at the head, and none is removed or skipped unexplained (`red-green-tests`).
- Rules, checks, workflows and review change only in a change of their own, which the maintainer approves (`CONTRIBUTING.md`, Kinds of change; no public name of its own).
- A change that alters what placket does for someone, a capability's scope or what it must not do stops for the maintainer (`deltas-stop-for-intent`).
- Secrets never print, in errors, logs or any format verb (`secrets-never-print`, `error-text-no-content`).
- An agent never adds a `Signed-off-by` line, its own or anyone's. Only a person signs off, for themselves. (`CONTRIBUTING.md`, Commits; no public name of its own)

## Read before you change

| If your change touches | Read first |
|---|---|
| `core/` | `docs/rules/core.md`, `docs/rules/errors.md`, `docs/rules/modules.md` |
| the door, the journal or the command line (`adapters/`, `cmd/`) | `docs/rules/adapters.md`, `docs/rules/errors.md` |
| a test or a vector | `docs/rules/tests.md` |
| key handling, cryptography, parsing or the socket | `docs/security/secure-coding.md` |
| the requirement text, a frozen format, an exported name or a dependency | `docs/rules/changes.md` |
| anything a check refused | the rule its last line links, and `docs/rules/checks.md` |
| Go code, for style | `docs/style/go.md` |
| prose: docs, comments, messages | `docs/style/writing.md` |
| a review | `docs/review.md` |
| a rule, a check, `AGENTS.md` or `CONTRIBUTING.md` | `docs/rules/README.md` and `CONTRIBUTING.md` |

`docs/rules/README.md` lists every rule by name.

## Definition of done

1. While working: `make <step>` for what you touched, and the package's own `go test`.
2. Before a push: commit, then `make ci` and `make red-green-tests BASE=origin/main`, both passing.
3. Then the pull request, as `.github/pull_request_template.md` shapes it. A user-visible change adds a line to `CHANGELOG.md`.

## Needs a human

Stop and ask the maintainer before:

- a licence change;
- a change to a frozen format (`format-change-noted`);
- a dependency in `core/` (`core-no-deps`);
- a workflow, a rule or a check;
- a release or a tag;
- anything security-sensitive: report it as `SECURITY.md` says, never fix it in public first.
