# core

Placket's core: the Go module `github.com/orilloai/placket/core`. It has no dependency, imports nothing from the rest of the repository, and never touches the world: no clock, randomness, environment, file, network or printing. What it needs from the world it takes as parameters.

| Package | What it implements |
|---|---|
| `canonical` | The canonical form and typed hashes (canonical-1 to canonical-11). |
| `signing` | Actor references, signing bytes, Ed25519 signatures and payload signatures (signing-1, signing-2, signing-4). |
| `setup` | Rows 1 to 17 of founding an Accord from its sealed founding set and the openings its founder holds (setup-contract-1 to setup-contract-13, setup-contract-16). |
| `composition` | The decision module: a crossing's verdict and reason (composition-1 to composition-7), and the crossings a member's view lists, each decided by it (setup-contract-14). |
| `founding` | Founding an Accord by every row, the acceptances' included; a member's view and its acceptance (setup-contract-14 to setup-contract-16). |
| `receipts` | Building, verifying, reading and reconciling receipts, and recognising a question's receipts (receipt-1 to receipt-10, crossing-5). |
| `answer` | Signing and reading the answer call, by which a member accepts, changes, declines or cancels a question (maybe-11). |
| `mediator` | One Accord's state and operations: the door, giving, fetching, asking, answering, deadlines, release and reload (maybe-1 to maybe-10). |
| `conform` | Checking a mediator's receipts for conformance to their Accord: validity first, then that every decision a receipt states is one the Accord let its decider make (verify-2). |
| `conformance` | The conformance runner, which runs every vector under `vectors/` through the packages above. |
| `internal/shape` | The format rules the packages share, such as the timestamp rule. |

`docs/adr/0002-the-core.md` lists the names the founding capabilities export, and `docs/adr/0006-founding.md` the names sealed terms added; each record of `docs/adr/` lists the names whose reason it holds. `docs/adr/0008-the-mediator-and-reload.md` says what the mediator holds, and how it is rebuilt after a stop. See `docs/adr/0002-the-core.md` for why the core is built this way, `docs/adr/0003-the-conformance-runner.md` for the runner, and `docs/rules/` for the rules `make ci` checks here.
