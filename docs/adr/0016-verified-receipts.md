# 0016. Verified receipts have a type of their own

Status: accepted. Date: 2026-11-17. Amends records 0005 and 0009.

## Context

`receipts.Verify` and `receipts.Decode` both return `Statement`, a struct with exported fields (record 0005). `Releases` takes a `[]Statement` from either. So a statement read without any check, or built by hand, cannot be told apart from one whose signature, chain and questions the verifier checked, and `Releases` reads both alike. The adapters already rely on statements to say which questions are open and what a log shows. That is what `verified-by-type` forbids: data that passed verification has a type only verification can produce.

Two more things sat in the wrong place. The mediator built a delivery, checked a restored one and decoded a part's body and salt (crossing-3) itself, and the decoding was written three times, in `Give`, `Reload` and `Question`, each a little differently. A delivery restored from the journal could then hash differently from the one the mediator admitted. None of those functions reads the mediator's state, so they break `mediator-no-logic`.

## Decision

- **`receipts.Verified`.** `type Verified struct` with unexported fields: a statement that passed `Verify`, or one this package built and signed. `func (v Verified) Statement() Statement` returns a copy (`results-are-copies`). No other code can make one.
- **The functions that change.**
  - `func Verify(mediator string, list [][]byte) ([]Verified, error)`, was `[]Statement`.
  - `func Releases(list []Verified) ([]Place, error)`, was `[]Statement`. Inside, `Verify` calls an unexported `releases([]Statement)`, so the order of section 8's checks does not change.
  - `func Build(subject Subject, predicate Predicate, key signing.Key) ([]byte, Verified, error)`, was `([]byte, string, error)`. The id is `Verified.Statement().ID`.
  - The mediator keeps its receipts as `[]receipts.Verified`.
- **What does not change.** `Decode`, `Show`, `Reconcile`, `Statement` and `Digest`. `Decode` still reads without verifying (receipt-8) and returns a `Statement`, which nothing takes as trusted.
- **`Digest(salt, body)` keeps its order.** `inputs-as-parameters` puts the salt after the data. `Digest` is the exception: its arguments follow the order of crossing-3's formula, which hashes the salt before the body, and its doc comment says so. Changing it would make every caller read against the requirement text.
- **The journal reads open questions from verified receipts.** At start the journal adapter runs `receipts.Verify` under the mediator's reference before it reads which questions are open. When that fails, it leaves the open entries as they are, and `Reload` refuses the list as it did before.
- **`core/internal/crossing`.** A new internal package holds what crossing-3 asks of a delivery: building it, checking a restored one against its receipt, and decoding a part's body and salt, written once. `Give`, `Reload` and `Question` call it, and `mediator` keeps only state. It is internal, so it adds no exported name, and this record is its reason (`packages-by-adr`).
- **Internal helpers beside it.** Three more internal packages, none exporting a name outside its module, have their reason here too. `core/internal/testing/` (package `ptesting`) holds the helpers the core's tests share, the vectors' keys and founding sets (`test-helpers-placed`), and `core/internal/testing/seeds` reads the vectors for fuzz seeds; `seeds` imports nothing of the core, so the tests of `signing` and `shape`, which `ptesting` imports, can use it too. Only tests import either, so they may read files, and `core-no-world` refuses a file built into the core that imports them. `adapters/internal/seeds` does the same for the adapters' fuzz tests, which cannot import the core's internal packages. `adapters/internal/lines` reads a file placket is handed, a journal or a state directory's file, only when it is a regular file, with a cap on each line, and `keyfile` opens a key file through it.

## Consequences

- A caller holding a `Verified` knows the verifier passed it, or that placket built it. A `Statement` from `Decode` cannot be handed to `Releases`.
- `Build`, `Verify` and `Releases` change signature before the first tag, with a `CHANGELOG.md` line (`api-compat`).
- A restored delivery and an admitted one are hashed by the same code.
