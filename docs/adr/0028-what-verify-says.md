# 0028. What verify, released and show say is exactly what they checked

Status: accepted. Date: 2026-11-21. Amends records 0021, 0022 and 0026. Amended by record 0031.

## Context

A person reads a line of `verify`, `released` or `show` as a claim. Before placket's formats freeze, each of those lines must mean what it says. A reading of the requirement text against the code found places where they did not:

- `verify` could print `witnessed` for a receipt whose call given failed row 4 of verify-1. A release after an accept whose call another member signed passed its own check, which read only that the call asks for a release, and was printed as witnessed.
- A member's view check could claim more than it read. With `--subset` the view line said that every receipt of the viewer's matched, though a subset may leave some out. With no receipt of the viewer's in the list it said that every one, 0, matched. The command read validity before the view, so a view that was not the one accepted, beside a journal that did not verify, printed the validity code. A part the mediator formed with no row in the viewer's view matched as `block` `maybe`, though verify-2 reads a give with no row as `block` `no-match` alone.
- The text of close-4 and the verifier failed two closings with reason `closed` at two times at different indexes.
- `verified` was the number of receipts in the object of `verify --json`, and a boolean, always `false`, in those of `show` and `released`.
- Smaller gaps: the `body` line was left out when the log did not conform; a delivery with an empty or absent `receiptId` matched the close, whose statement is empty, and failed at the close's line rather than as no receipt; the text did not say whose roles the unwitnessed line names; `reconcile` given one party's kept records twice read them as two parties; `--json=1` and other true spellings did not turn on the object when a flag could not be read; and `deliveries` was left out of `give --wait --json` when the wait fetched no other delivery.

Where the text and the code disagreed, each case is settled one way here, and pinned by a test or a vector.

## Decision

### Contradicted, a third witness outcome

A receipt an answer decided is witnessed by a call given that passes row 4, unwitnessed with no call given, and contradicted by a call given that does not pass row 4. A contradicted receipt fails `witness`, as before; it is never printed witnessed or unwitnessed. `verify` prints `<receipt id> contradicted: the call given for it does not fit`. Contradicted is a printed word and an outcome under the existing code `witness`: no code, reason or format changes.

A release reads its closing's call. So a release after a contradicted accept is contradicted too. An accept whose call fits stays witnessed when only its release fails, as when the call asks for no release: the failure is the release's. This amends record 0021, whose witness rule read a release's call alone.

### The member's view check

- `--view` with `--subset` is the command's error `usage`: the view line speaks of every receipt of the viewer's, which a subset cannot hold for sure.
- With no receipt of the viewer's in the list, the view line says so: `view of <viewer>: no receipt of its own is in this list, so none was checked against the view it accepted`. The exit code is unchanged.
- A formed part with no row in the viewer's view matches only as `block` `no-match`, as a give with no row does (composition-2). The text wins; the code narrows. A `blocked` row still matches `no-match` or `maybe`.
- Row 0a, the view, is read before validity in the command, as the text and the core already did. A view that is not the one accepted is the command's error `founding` with `acceptance-view`, and no validity code is printed with it.

These amend record 0026.

### close-4

A closing with reason `closed` whose `at` is not the first such closing's fails `close` at its own index, whether the list ends there or a close follows. The code wins: the text now says what the verifier does, and two vectors in `vectors/close/` pin it. This amends record 0022.

### The count in `verify --json`

The member of `verify --json` that holds the number of receipts is `receiptCount`, beside `questionsClosed` and `blockedReleases`. `verify --json` has no `verified` member. In the commands' objects, `verified` is only ever the boolean of `show` and `released`. The vector format's `verify` expectation keeps its count `verified`: it is a frozen format, read by other implementations, and the rename stops at the command line. No release has shipped, so the old name is removed with no deprecation.

### Smaller cases

- **The body line.** With a delivery, `verify` prints the `body` line whether or not the log conforms (the text wins). The `conforms` line keeps the first failure's code. The conformance vectors are unchanged: the runner still checks a body only after a log conforms.
- **A delivery of no receipt.** A close is no receipt a delivery names. A delivery whose `receiptId` is empty or absent, or names no receipt of the list, fails `body` at index -1, and `verify` prints `error delivery` (the text wins). No vector carries it: a vector's delivery must hold a typed hash.
- **Whose roles the unwitnessed line names.** The roles the answerer's binding gives it (the code wins). The question's rule is not known when the terms are sealed; the binding's roles always are. Only the text of commands-8 changes.
- **One party twice in `reconcile`.** A second kept record for a party already given fails `kept` at its index in the list of kept records, naming no party, before row 1: a new refusal of section 9, in its row 0, beside the check that each record names its party by an actor reference. `reconcile` prints `error verify kept --kept N actor`. Merging the two records was not chosen: a party's records are one record, and two copies of it are a caller's mistake that a merge would hide. The vector format keys kept records by party, so this is a Go test, not a vector.
- **`--json` in every spelling of true.** When a command's flags cannot be read, `--json` still turns on the object in every spelling a boolean flag reads as true, `1`, `t`, `T`, `true`, `TRUE` and `True`, and the last such flag before `--` decides.
- **`deliveries`.** It is there, `[]` when empty, wherever `fetched` is (commands-1 wins over section 11's "when it fetched any", which is amended).

### The core's names

- **`conform`**: `Witness` gains the value `Contradicted`, `"contradicted"`. A contradicted receipt's `Decision.Witness` is `Contradicted`.
- **`cli`**: the lines `LineContradicted` and `LineViewNone`.

No other exported name changes.

### The formats

No frozen format changes: no new code, reason, verdict or type URI, and no member emitted that was absent. The new vectors use the existing operations and expectations.

## Consequences

- A `witnessed` line means a call given passed row 4, and nothing less; a `contradicted` line means one was given and did not.
- A view line never claims a receipt it did not read.
- A script that read `.verified` from `verify --json` reads `.receiptCount`; `show` and `released` are unchanged.
- A verifier elsewhere learns the close-4 index, the formed-part rule and the empty `receiptId` from the text and the vectors, not from a failure.
