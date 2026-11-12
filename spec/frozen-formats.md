# The register of frozen formats

One row per format of protocol version 0.1. A format is frozen once it is released: from then on a change to it is a new version of it, with a note in its row and in `vectors/README.md`, in the same change. Before release, a format may still change within 0.1; its row says how it stands, and a change carries a note in the row. A new type URI, reason or code, and a member written that was absent, are each a format change.

Every format here is at version `0.1`, and every one is unreleased. No schema is fetched: each `$id` is under `https://placket.dev/schema/0.1/`, and `make ci` loads them from this directory.

| Format | Requirements | Schema files | Version | What binds it | What changing it costs |
|---|---|---|---|---|---|
| Canonical form | canonical-1, canonical-2, canonical-4 to canonical-11 | none: a byte form | 0.1, unreleased | Every hash and signature is over canonical bytes | Every Accord id, receipt id and signature, and every vector |
| Typed hash | canonical-3 | `spec/schema/hash.json` | 0.1, unreleased | Accord ids, card hashes, message hashes and receipt ids are typed hashes | Every id, every receipt and every vector |
| Actor reference | signing-1 | `spec/schema/reference.json` | 0.1, unreleased | Every key in every format is named so | Every declaration, binding, founding set and receipt |
| Timestamp | receipt-10, setup-contract-2 | `spec/schema/timestamp.json` | 0.1, unreleased | Every time in every format | Every binding, receipt and vector |
| Signing construction and type URIs | signing-2, signing-4 | none: a byte construction | 0.1, unreleased | Every signature by an actor, and the mediator's receipt signature | Every signature, so every declaration, binding, envelope and receipt |
| Declaration and term | setup-contract-1, composition-1 to composition-6 | `spec/schema/declaration.json`, `spec/schema/term.json` | 0.1, unreleased | Signed by its actor and hashed into the Accord id | Every declaration signed again, and a new Accord id |
| Binding | setup-contract-2, setup-contract-3 | `spec/schema/binding.json` | 0.1, unreleased | Signed by a principal and hashed into the Accord id | Every binding signed again, and a new Accord id |
| Card | setup-contract-4 | `spec/schema/card.json` | 0.1, unreleased | Hashed into the Accord id and named by every receipt | A new card hash and Accord id, and every receipt |
| Founding set | setup-contract-5 to setup-contract-10 | `spec/schema/founding.json` | 0.1, unreleased | Its typed hash is the Accord id | A new Accord id for every set |
| Crossing message and part | crossing-1, crossing-2, maybe-8, maybe-9 | `spec/schema/message.json`, `spec/schema/part.json` | 0.1, unreleased | Hashed into the envelope; its part's digest is in the receipt | Every envelope and every scenario vector |
| Envelope | crossing-4 | `spec/schema/envelope.json` | 0.1, unreleased | Signed by the sender and echoed by its receipt | Every submission and its echo, and reconciliation |
| Delivery | crossing-3 | `spec/schema/delivery.json` | 0.1, unreleased | Carries the receipt that passed the part, and what a receiver checks the digest with | Every receiver, and every scenario vector |
| Receipt: envelope, statement and predicate | receipt-1 to receipt-5, receipt-9, receipt-10, crossing-5 | `spec/schema/receipt.json`, `spec/schema/statement.json`, `spec/schema/predicate.json` | 0.1, unreleased | Signed by the mediator, chained per pair, read by every verifier | Every receipt and journal, every verifier, and every vector |
| Reasons and codes | receipt-4, and each section's check table | none: closed lists in `protocol.md` | 0.1, unreleased | Every receipt's reason and every result a caller or verifier sees | Every caller that matches on them, and every vector |
| Conformance vector | conformance-vectors-1 to conformance-vectors-11 | `spec/schema/vector.json` | 0.1, unreleased | Every vector under `vectors/` and every runner | Every vector and every runner |

## Notes

None yet: no format has changed since version 0.1 was written.
