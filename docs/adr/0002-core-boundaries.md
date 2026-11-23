# 0002. The core is a module of its own, with no dependency and no world

Status: accepted. Date: 2026-11-11. Amended by record 0015.

## Context

Placket's core decides what crosses between parties and writes the receipts that prove it. Anyone holding the published vectors must be able to check that core, and get the same bytes on every run, on every machine. Three things break that promise. A dependency brings code nobody in the protocol agreed to, and a version bump can change a byte. An import of an adapter brings sockets, files and processes with it. And a call that reads the world, such as the clock, the environment, a random source, a file, the network or the terminal, makes the output depend on when and where the code ran, which no vector can pin. Each of these is easy to add by accident in a single line, and hard to remove once other code relies on it.

## Decision

- The core is its own Go module, `github.com/orilloai/placket/core`, in `core/`. Its `go.mod` names the module and the Go version (`go 1.27`, `toolchain go1.27.4`) and has no `require`. A dependency in the core needs a new decision record first, saying which code path uses it and why the standard library cannot.
- Nothing under `core/`, test files included, imports `github.com/orilloai/placket/adapters`, `github.com/orilloai/placket/cmd` or `github.com/orilloai/placket/recipes`, or any package below them.
- No code built into the core reads the clock, draws randomness, reads the environment, opens a file, touches the network or prints. What the core needs from the world, such as the time, a salt or a key, it takes as a parameter from its caller, an adapter. Test files may use any of these.
- The core is Go alone: no cgo and no assembly, C or other non-Go source, which could reach the world out of sight of any check of Go. No file under `core/` is a symbolic link, and no import path in it is written with an escape, so what the checks read is what the Go tool builds.
- `make ci` enforces each of these as a named check: `core-imports`, `core-no-deps` and `core-no-world`. Their names, and what each matches, are in `docs/rules/`.

## Consequences

- Every function in the core is a function of its inputs. The same inputs give the same receipts on two machines, and a vector can state them byte for byte.
- An adapter does the work of the world: it reads the clock once per call, mints salts and nonces, reads key files and prints. That puts more code in the adapters, and it is the right place for it.
- The checks are line-based greps, not a type-aware analysis. They refuse the packages that reach the world by import path, under any alias, and the clock and printing calls by their usual names; a call under an unusual alias, or one that reaches the world inside another package, is left to the reviewer, who reads the imports of every change to `core/`.
- A verifier can build against the core alone and needs nothing else from this repository.
