# 0001. The repository layout

Status: accepted. Date: 2026-11-11. Amended by records 0003, 0023 and 0024.

## Context

Placket is a mediator for content crossing between parties, with a Go core that conformance vectors can test byte for byte, and adapters that connect it to sockets, files and a command line. The repository starts empty. Builders who arrive later, human or agent, need to know where each kind of thing goes before any of it exists, so that the first package, the first vector and the first adapter each land where the checks and the reviewers expect them. A layout that grows by accident makes those checks impossible to write: a rule such as "the core imports nothing from the adapters" needs a fixed place for each.

## Decision

The repository has these top-level directories. Each is created by the change that first puts something in it, not before.

| Directory | What it holds | Created |
|---|---|---|
| `core/` | The Go core: a module of its own, `github.com/orilloai/placket/core`, with no dependency (record 0002). One package per protocol capability. | Now, with only `go.mod` and a README; its first package with the capability packages |
| `docs/adr/` | Decision records: any decision that spans packages or would be expensive to reverse, written ahead of the change built on it. Named `NNNN-<slug>.md`, numbered from 0001. | Now |
| `tests/` | The repository's own shell tests, `tests/test_*.sh`, such as the self-tests of the checks in `make ci`. Fixtures under `tests/testdata/`. | Now |
| `openspec/` | The normative requirement text of the protocol and the process for changing it. | With the protocol's first text |
| `spec/` | The frozen formats: their schemas and the register of type URIs, codes and reasons. | With the protocol's first text |
| `vectors/` | Conformance vectors, produced by generators independent of the code under test, and the coverage list that maps each requirement to its vectors. | With the protocol's first text for the format and coverage list; with the capability packages, and then questions and release, for the vectors |
| `review/` | The review passes a reviewer runs over a change. Changed only by the maintainer's own change. | With the protocol's first text |
| `adapters/` | The parts that touch the world: the socket door, the journal, key files, an actor's state directory, the log export and the command line, one package each. | With the door |
| `cmd/placket/` | The binary: `main` only, which parses arguments, builds a client and hands over to `adapters/cli`. | With the door |
| `scripts/` | Scripts a person runs by hand, such as the end-to-end demo behind `make demo`. | With the door |
| `recipes/<name>/` | One job's templates, agents and plugin, built on the core's exported API alone. | After v1 |
| `out/` | Working notes a builder leaves on a change's branch for the next round of the same change, removed before the change merges; never on `main`. | Now, on a change's branch |

`.github/workflows/` holds the workflows that run `make ci`. It is the maintainer's: no other change edits it.

The root of the repository holds `Makefile`, `AGENTS.md`, `CONTRIBUTING.md` and `README.md`. With the door it also becomes a Go module, `github.com/orilloai/placket`, holding `adapters/` and `cmd/`; that module requires the core module and has its own decision record. No `go.work` file is committed.

Dependencies point one way: `cmd` uses `adapters`, `adapters` use `core`, and `recipes` use `core`. Nothing lower uses anything higher.

`make ci` is the one entry point for every check. It finds every Go module in the repository and formats, vets and tests each, except a module under a `testdata/` directory, which is a fixture, and a module with no Go packages yet.

## Consequences

- A builder can tell from the path alone whether a file is core, adapter, binary, recipe, text, format or vector, and the checks in `make ci` can rely on it: a check on `core/` does not need to guess where the core ends.
- Directories that later changes add are empty promises until then. A change that adds a directory not named here, or puts something in the wrong one, needs a new record first.
- Fixtures for any check live under a `testdata/` directory, which Go's tools and `make ci` both skip, so a planted fault never breaks the real build.
- The core stays a module with no dependency even after the root module exists, so a verifier that needs only the core never pulls in an adapter.
