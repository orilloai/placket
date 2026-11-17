# 0004. The conformance runner is a package of the core, and reads the schema itself

Status: accepted. Date: 2026-11-12.

## Context

Section 10 of `spec/protocol.md` says how a runner judges a conformance vector: the header first, then the whole vector against `vector.json`, then its byte inputs, then its operation, then each result, and the first check that fails decides. A green run must mean that every vector ran and passed (`runner-no-silent-pass`): a part the runner cannot run, a pending part, an input given twice and a vector the coverage list names but nobody wrote must each fail the run, never pass it.

The runner is needed before any of the code it runs. The capability packages come next, each with its vectors, and then the questions and release. If the runner came with the first vectors, the first vectors would be judged by a runner nobody had yet seen refuse anything.

Three things had to be decided: where the runner lives, how it reads JSON Schema with the core's rule of no dependency, and how it learns to run an operation.

## Decision

- **The runner is `core/conformance`, a package of the core module.** It sits beside the packages it runs, so anyone who holds the core and the vectors can check an implementation with nothing else from this repository. It keeps every rule of the core (record 0002): it has no dependency, and it never touches the world. It reads `spec/` and `vectors/` through the `io/fs` file systems its caller passes, and returns a report; it opens no file and prints nothing.
- **`make ci` runs it through a test.** `TestVectors`, in the package's tests, passes the repository's `spec/` and `vectors/` and prints the report. The `runner-no-silent-pass` step runs that test alone and shows its lines. Tests may touch the world; the package may not.
- **It reads JSON Schema itself, for the keywords placket's schemas use.** A dependency on a schema library would break `core-no-deps`. The runner implements the draft 2020-12 keywords the schemas in `spec/schema/` use, and refuses to compile a schema that uses any other: a keyword it does not read fails the load, so it can never validate less than the schema says. It compares numbers by exact value at any exponent. Its tests hold it to every accepted and refused example of every schema, the same examples `schemas-hold` checks with an independent validator, so the two cannot disagree unseen.
- **An operation runs through a handler.** A handler is a function of one vector that reports each part it compared, as matched, differing or pending. The runner itself has no notion of a skip: a pending part fails its vector, an operation with no handler fails every vector of it, and a handler that reports no part fails too. `Default()` returns the handlers of the operations the core can run; with no capability package yet, it returns none. Each later change adds the handlers of what it builds.
- **The coverage list is `vectors/coverage.json`.** It maps each id of `spec/ids.txt`, and each scenario of the text under it, to the vectors that cover it, or names it open with the stage that writes them. The runner fails a run on a missing entry, an entry for an id or a scenario the text does not have, a listed vector that is absent and a vector no entry names, and prints every open entry. `vectors/README.md` describes the form.
- **Until the canonical form exists, the header check reads with a strict reader of the runner's own.** It refuses what the JSON grammar, UTF-8, a repeated name at any depth and nesting beyond 1000 levels refuse. When the canonical form's package is added, the header check reads with it instead, and gains the canonical form's other refusals.

## Consequences

- Every refusal of the runner is proved now, on fixture vectors under `core/conformance/testdata/`, before any vector exists. A later change adds a handler and its vectors; it does not have to trust a runner it has not seen fail.
- The schema reader is code to maintain. A new keyword in a schema fails `make ci` until the runner reads it, which is a cost paid on purpose: the alternative is a runner that passes over a rule it does not know.
- A handler can still compare too little. The protocol's comparison rules (conformance-vectors-5) are the handler's to keep, and a reviewer reads each handler against them; the runner only guarantees that what a handler did not run is never a pass.
- The runner is in the core's module but is not a protocol capability, so it adds nothing to the core's exported surface a verifier of receipts needs.
