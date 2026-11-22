# 0030. A receipt's payload is its canonical bytes; its envelope is read loose

Status: accepted. Date: 2026-11-22.

## Context

A receipt is a DSSE envelope whose `payload` is the standard base64 of its statement, and whose id is the typed hash of the statement's canonical bytes (receipt-1). The mediator writes the payload in canonical form. The text did not say what a verifier does with a payload that the canonical form accepts but would write otherwise: with a space after `{`, its members in another order, or a number such as `seq` spelled `1.0`.

The Go verifier read such a payload, checked the signature over the bytes as given, and took the id from their canonical form. So one receipt id had many validly signed byte forms. A verifier that followed receipt-1 to the letter, hashing or signing over what the payload holds, would refuse a receipt the Go verifier accepted, or give it another id. Two verifiers that follow the protocol must agree on every receipt: whether it verifies, and what its id is. The receipt format freezes at 0.1, so the rule had to be settled first.

The envelope around the payload raises the same question less sharply. A delivery carries its receipt as a JSON value, and a delivery is re-encoded when it is read, so the envelope's bytes do not survive a delivery as the mediator wrote them. The payload string does: it is one JSON string, the same whichever way the object around it is written.

## Decision

- **The payload is strict.** A receipt verifies only when its payload is exactly the canonical bytes of its statement. A payload the canonical form accepts but would write otherwise fails section 8's row 1 with `not-canonical`, at that receipt's index, even when the mediator signed exactly those bytes. A close receipt follows the same rule, at the same row.
- **The envelope is loose.** A receipt's own bytes are read in any spelling the canonical form accepts: its members in any order, with any whitespace. Its `payload` string is read as written, and the id comes from the payload alone, so a re-encoded envelope verifies under the same id. receipt-1 says so, and a vector pins it.
- **One code, widened.** `not-canonical` keeps its row and gains one meaning: beside bytes the canonical form refuses, a receipt's payload that is not its own canonical form. The conventions' sentence on the code names both. No code is added.
- **Readers that do not verify follow the rule.** `receipts.Decode` and `receipts.DecodeClose`, which `placket show` uses, refuse such a payload with `not-canonical` (receipt-8). No path yields an id for a payload `verify` would refuse.

The payload's base64 is not touched: row 3 already takes it only in its one encoding. The answer call is not touched either: it already requires its own canonical bytes and fails as `answer-call` (section 6, maybe-11), and the widened sentence names only the receipt's payload.

## Consequences

- One receipt id has one signed payload. A verifier in any language checks the signature over the payload's bytes and hashes those same bytes for the id, with no canonical form step between them that could disagree.
- Every receipt a mediator has written is already in canonical form, so no journal, id or landed vector changes its result, and no migration is needed. The demo journal verifies as before.
- A tool that rewrites a receipt's envelope, as a delivery does, keeps it verifiable. A tool that rewrites the decoded statement and re-signs it with the mediator's key makes a receipt that no longer verifies.
- Rejected: holding the envelope's bytes to canonical form too. It would refuse every receipt a delivery carried, and buys nothing, since the signature and the id read only the payload.
