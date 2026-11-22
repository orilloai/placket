# 0030. What a body check, an empty journal and verify's claim line prove

Status: accepted. Date: 2026-11-22. Amends records 0021 and 0028.

## Context

Before placket's formats freeze, a second implementation that follows the requirement text must reach the result the Go verifier reaches. A reading of the text against the code found five places where the two said different things, or where the text said two things:

- **A body's form.** crossing-3 said a part's digest covers its salt and its body's bytes. It did not say whether the form the body crossed in, `text`, `raw` or `data`, is part of what the digest names. The code hashes the bytes alone, so a `text` body and a `data` body of the same bytes have the same digest, and so does a `raw` one.
- **`mediaType`.** crossing-2 said a part with any member but its body and `metadata` is `one-body`. Row 10 of the door's table, the part's schema and crossing-2's own scenario allow `mediaType` beside them.
- **A delivery's shape.** `verify --delivery` read a file's `receiptId`, `receipt`, `kind`, `salt` and one body member, and nothing else. A file with no `messageId`, or with a member a delivery never holds, passed as a delivery. The text did not say what the `body` line means of a part that did not pass.
- **An Accord with no receipt.** verify-1 founds the set at the first receipt's time. With no receipt the core founds at the earliest time, so no binding's `until` is checked, while `placket verify` founded at the wall clock, so a set whose bindings had lapsed failed `founding` in the command and founded in the core.
- **The claim line.** `verify` printed `claim: each receipt is this mediator's signed decision about one part under this Accord's terms.` on every run that verified, even when conformance was not checked, the terms were sealed or the log did not conform. A run with `--mediator` alone printed both `conformance not checked` and that claim.

Each case is settled one way here. None changes a digest, a schema, a code or a landed vector's result.

## Decision

### The digest names bytes, not form

A part's salted digest names its kind and its body's bytes. The form the body crossed in is not part of it: a `text`, a `raw` and a `data` body with the same bytes have the same digest. A reader who needs the form takes it from the kind's own rule, the agreement of the parties on what a part of that kind holds, not from the receipt. The code is unchanged; the text says what it does, and a `crossing` vector pins three bodies of one byte string to one digest. Putting the form in the digest was not chosen: it would change every digest, and a receipt would still not say what the bytes mean.

The `body` line of `verify` speaks of bytes in the same way: it says the delivery's body bytes, under its salt, are the part its receipt names. It says neither the form nor that the part passed.

### A part may hold `mediaType`

crossing-2's sentence reads as row 10 does: a part with any member but its body, `metadata` and `mediaType` is `one-body`. Text only; every landed vector passes unchanged.

### A delivery is read as a delivery first

Row 5 of verify-1 first reads the delivery as `delivery.json` shapes it: an object with `receiptId`, `receipt`, `messageId`, `kind` and `salt`, exactly one body, `text`, `raw` or `data`, and no member but those, `re`, `mediaType` and `answer`. A file that is not one fails `body` at index -1, as a delivery of no receipt does, and `verify` prints `error delivery`, exit 3, with no `body` line. A delivery an answer queued holds its answer call as `answer` (maybe-12), so a file that carries the call beside the body is still a delivery.

The check is the one the mediator runs on a held delivery after a restart, shared in `core/internal/crossing`; no schema validator is added to the binary. The values inside the members are still row 5's: a salt or a kind that does not make the receipt's digest fails `body` at the receipt's index, as before.

A delivery whose receipt did not let its part cross is not refused: the `body` line says what it means, and the receipt's own claim says whether the part passed. Refusing it was not chosen, since a delivery's bytes against a block's digest are still a fact a reader may want shown.

### An Accord with no receipt is founded at no time

With no receipt there is no time to found at, so no binding's `until` is checked. The rule is the core's: `conform` founds such a set at `0001-01-01T00:00:00Z`, and `placket verify` founds at the time `conform.FoundedAt` gives, so the command and the core cannot differ. A wall clock was not chosen: the result of a verification must not depend on when it runs.

A journal, or a subset, whose first receipt is after a binding's `until` still fails `founding`, a close that is the only receipt included. The threat model's L20 says so.

### Two claim lines

`verify` prints today's claim line only when its `conforms` line is the full one, that every decision is one the Accord let its decider make: a founding set, this build's module, every opening, and no failure. Otherwise it prints `claim: each receipt and close is signed by this mediator.`: with `--mediator` alone, a card of another module, sealed terms, or a log or a body that does not conform. `verify --json`'s `claim` holds the line printed. The full line keeps its words whether or not the list ends in a close: the `complete` line is what speaks of a close.

### The names

- **`conform`**: `FoundedAt(list [][]byte) string`, the time a founding set is founded at for a list (verify-1 row 0): the first receipt's `at`, a close's when it is the first, or `0001-01-01T00:00:00Z` with no receipt. `Check` founds at it.
- **`cli`**: the claim line `ClaimSigned`.

No other exported name changes.

### The formats

No frozen format changes: no new code, reason, verdict or type URI, and no member emitted that was absent. The new vectors use the existing operations and expectations: a `crossing` scenario of three gives, and two `conform` vectors whose delivery is not one, which expect `body` at index -1.

## Consequences

- A second implementation hashes a body's bytes alone, allows `mediaType` on a part, reads a delivery's shape before its body, founds a set with no receipt without checking `until`, and learns each from the text.
- `verify --delivery` refuses a file with a missing or unknown member as `error delivery`, where it used to read the body anyway.
- `placket verify` on a journal with no receipt gives the same result whatever the clock says.
- A script that read `claim` from `verify --json` reads the narrower line on every run that did not check every decision against the terms.
