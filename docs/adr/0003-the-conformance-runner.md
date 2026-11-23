# 0003. The conformance runner

Status: accepted. Date: 2026-11-23.

## Context

Section 10 of `spec/protocol.md` says how a runner judges a conformance vector: the header first, then the whole vector against `vector.json`, then its byte inputs, then its operation, then each result, and the first check that fails decides. A green run must mean that every vector ran and passed (`runner-no-silent-pass`): a part the runner cannot run, a pending part, an input given twice and a vector the coverage list names but nobody wrote must each fail the run, never pass it. The runner was needed before any of the code it runs, so that the first vectors were judged by a runner already seen to refuse.

## Decision

- **The runner is `core/conformance`, a package of the core module.** It sits beside the packages it runs, so anyone who holds the core and the vectors can check an implementation with nothing else from this repository. It keeps every rule of the core (record 0002): it has no dependency, and it never touches the world. It reads `spec/` and `vectors/` through the `io/fs` file systems its caller passes, and returns a report; it opens no file and prints nothing.
- **`make ci` runs it through a test.** `TestVectors`, in the package's tests, passes the repository's `spec/` and `vectors/` and prints the report. The `runner-no-silent-pass` step runs that test alone and shows its lines. Tests may touch the world; the package may not.
- **It reads JSON Schema itself, for the keywords placket's schemas use.** A dependency on a schema library would break `core-no-deps`. The runner implements the draft 2020-12 keywords the schemas in `spec/schema/` use, and refuses to compile a schema that uses any other: a keyword it does not read fails the load, so it can never validate less than the schema says. It compares numbers by exact value at any exponent. Its tests hold it to every accepted and refused example of every schema, the same examples `schemas-hold` checks with an independent validator, so the two cannot disagree unseen.
- **An operation runs through a handler.** A handler is a function of one vector that reports each part it compared, as matched, differing or pending. The runner has no notion of a skip: a pending part fails its vector, an operation with no handler fails every vector of it, and a handler that reports no part fails too.
- **The coverage list is `vectors/coverage.json`.** It maps each id of `spec/ids.txt`, and each scenario of the text under it, to the vectors that cover it, or names it open with what stays open. The runner fails a run on a missing entry, an entry for an id or a scenario the text does not have, a listed vector that is absent and a vector no entry names, and prints every open entry. `vectors/README.md` describes the form.
- **Its report is outside the error rules.** The runner returns a report, not an `*Error`, and its lines may quote a vector, such as a member name repeated in one of its objects. A vector is public test data, and the report is read by the person running it, so this is what the report is for. The error rules (`docs/rules/errors.md`) cover the errors placket's packages return. A failure of the runner itself, such as a vector that cannot be read, is a line of the report, never a panic (`core-no-panic`).

### Its names

**`conformance`** (conformance-vectors-1 to conformance-vectors-10)

- `Run`: reads `spec/` and `vectors/`, runs every vector with the handlers given, and checks the coverage list.
- `Handler` and `Handlers`: what runs one operation's vectors, and the handler of each operation.
- `Default`: the handlers of every operation the core can run, and every step of a scenario.
- `Vector`: one vector file as a handler receives it, after the runner's own checks.
- `Vector.Path`, `Vector.Operation` and `Vector.Bytes`: the file's path from the repository root, its operation member, and its bytes unchanged.
- `Part`: one result a handler compared, or could not.
- `Part.Name`, `Part.Failure` and `Part.Pending`: which part, how the result differed from the one expected, and what the part needs before it can run; a part passed when both of the last are empty.
- `Result` and `Result.Passed`: what became of one vector, and whether it passed.
- `Result.Path`, `Result.Check`, `Result.Reason` and `Result.Parts`: the vector's path, the check that failed it, empty when it passed, why it failed, and how many parts passed.
- `Report`, `Report.Lines` and `Report.Passed`: the outcome of a run, written as lines, and whether it passed.
- `Report.Results`, `Report.Problems` and `Report.Open`: every vector found, in path order; the failures of the run that belong to no one vector; and the coverage list's open entries.
- `Open`: an entry of the coverage list that no vector covers yet, or covers in part, with what stays open.
- `Open.ID`, `Open.Scenario`, `Open.Milestone` and `Open.Rest`: the requirement id, the scenario, empty for the requirement itself, the milestone that writes its vector, and what stays open of an entry covered in part.
- `CheckHeader`, `CheckSchema`, `CheckInputs`, `CheckOperation` and `CheckResults`: the checks a vector goes through, in order.

## Consequences

- Every refusal of the runner is proved on fixture vectors under `core/conformance/testdata/`. A change adds a handler and its vectors; it does not have to trust a runner it has not seen fail.
- The schema reader is code to maintain. A new keyword in a schema fails `make ci` until the runner reads it, which is a cost paid on purpose: the alternative is a runner that passes over a rule it does not know.
- A handler can still compare too little. The protocol's comparison rules (conformance-vectors-5) are the handler's to keep, and a reviewer reads each handler against them; the runner only guarantees that what a handler did not run is never a pass.
- The runner is in the core's module but is not a protocol capability, so it adds nothing to the core's exported surface that a verifier of receipts needs.
