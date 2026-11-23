# 0009. The root module holds the adapters and the binary

Status: accepted. Date: 2026-11-12. Amended by records 0016, 0021, 0023, 0024 and 0027.

## Context

The core is a module of its own with no dependency (record 0002): it never reads a clock, a file or a socket, so a vector can test it with no process around it. Placket also needs code that does touch the world: a door on a local socket, a journal on disk, key files, each party's state directory, the log export and the command line, and one binary that holds them. That code must live somewhere that can import the core but that the core can never import (`core-imports`), and it must build the same from a fresh clone, with no local setup (`root-module`).

## Decision

- **One root module.** `go.mod` at the repository's root is the module `github.com/orilloai/placket`. It requires `github.com/orilloai/placket/core` at `v0.0.0` and replaces it with `./core`, until the core is first tagged. No `go.work` is committed. Everything under `adapters/` and `cmd/` is in this module; the core, the vectors' generators and the checker stay modules of their own.
- **The standard library only.** The root module requires nothing but the core. The socket is a unix socket and the locks are `flock`, both from the standard library's `net` and `syscall`. So the window and the commands run on Linux and macOS, the systems that have both; Windows is not a target of this version.
- **The packages**, one per thing that touches the world:

  | Package | What it holds |
  |---|---|
  | `adapters/keyfile` | Key files: a seed as 64 lowercase hex digits and a line feed, mode 0600. Read, write, reference. |
  | `adapters/state` | A party's state directory: its lock, its last `seq`, the envelopes it signed, the receipts of what it fetched, and the parts it kept. |
  | `adapters/journal` | The window's journal: the directory a stopped mediator is made again from (record 0011). |
  | `adapters/local` | The door: the call and the reply, the server and the client, the door's codes, and the window that runs the mediator behind the door (record 0010). |
  | `adapters/export` | The log: one row per question, from verified receipts and covered kept parts, as JSON and CSV. |
  | `adapters/cli` | The commands, one file each, and `words.go`, which holds every product word and claim line. |
  | `adapters/internal/strict` | One reader of the door formats' JSON objects, by their members' exact names: the standard library's decoder matches a member to a field without regard to case. |
  | `cmd/placket` | `main` only: it passes the system clock, `crypto/rand` and the signal context to `adapters/cli` and exits with the code a command returns (`main-is-thin`). |

- **Dependencies point one way.** `cmd/placket` imports `adapters/cli`, which imports the other adapters, which import the core. `adapters/export` imports `adapters/local` for one function, the check of a kept answer call, so that a call's signature is verified in one place (`call-signing`). No adapter re-exports a core type (`adapters-core-types`); each takes and returns the core's types, bytes and strings.
- **Tested in process.** Every command is a function of its arguments, its streams and an environment that carries the clock, the randomness and the interrupt, so its tests run it without a process of its own. The few tests that need processes, two commands at once and a window killed with SIGKILL, run one binary that `adapters/cli`'s tests build once, with the race detector when the tests run under it.
- **The wiring test's root-module case.** `tests/test_wiring.sh` plants a root-module fault in a copy of the tree and still needs `go list` to work there, to check that `modules` lists the core. It planted a root module of another name, which breaks every import of the root module's own packages once the adapters exist. It now plants a root module of the right name whose replace names the core by another path, `./core/../core`: the check refuses it, and `go list` still works.

## Consequences

- The core stays free of the world: nothing under `core/` can import the adapters, and the checks keep it so.
- A fresh clone builds the binary with `go build ./cmd/placket`, with nothing fetched but the toolchain.
- Every Go module of the repository is read by every toolchain step of `make ci`, the root module included.
- Supporting Windows later means a door on another kind of socket and another kind of lock, behind the same packages.
