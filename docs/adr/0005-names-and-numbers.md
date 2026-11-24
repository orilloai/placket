# 0005. Names and numbers: placket.tech, one protocol number, salts and roles

Status: accepted. Date: 2026-11-23. Changed by record 0015.

## Context

A type URI is signed into every declaration, binding, envelope, answer call and acceptance, and the receipt's predicate type into every receipt, so the host and the number in each URI are part of every signature. Once a format freezes, each of them can change only with a new protocol version. Four things about them had to be settled before the formats freeze:

- **The host.** Placket's home is `placket.tech`.
- **The protocol number.** Each URI ends in a version, and the card's `spec` names one; nothing else would say that these are the same number, or that they move together.
- **The salt.** The salted digest of crossing-3 is the SHA-256 of the salt's bytes followed by the body's bytes, with nothing between them. With a salt of any length, one digest opens as more than one (salt, body) pair: the salt and the body, or a longer salt that takes the body's first bytes and a shorter body that is the rest. A holder of a part could show a suffix of what crossed as if it were the part the receipt names.
- **The role.** The crossing message is an A2A 1.0 message. In A2A 1.0 the client sends `ROLE_USER` and the agent answers `ROLE_AGENT`. Reusing A2A's names with another meaning is a trap for anyone who reads the frame as A2A.

## Decision

- **One host.** Every schema `$id`, every signed type URI (signing-4), the receipt's predicate type (receipt-1), the close's predicate type (close-2) and the crossing extension's URI (crossing-1) are under `https://placket.tech/`. A type URI is `https://placket.tech/<kind>/0.1`; a schema's `$id` is `https://placket.tech/schema/0.1/<name>.json`. The payload types (`application/vnd.placket.call+json`, `application/vnd.in-toto+json`), the in-toto Statement URI, the digest name `salted-sha256` and the door's 16-byte `nonce`, which is not a salt, are not placket's URIs.
- **One protocol number.** Every placket URI carries the version the card's `spec` names (setup-contract-4): a type URI ends in it, and a schema's `$id` holds it as the segment after `schema/`. This version knows one value, `0.1`; a card of any other `spec` is refused at founding. A later version changes every URI and the card's `spec` together, in one change. The number stays in each URI as a whole literal in its one home (`signing`, `receipts`, `mediator`): `signing` cannot import `setup`, since `setup` imports `signing`, and a package of its own for one string is not worth it. `TestEveryURIUnderTheCardsSpec` in `core/mediator` holds each URI to `setup.Spec`.
- **Five type URIs.** Signing-4 names the declaration, the binding, the envelope, the answer call (record 0009) and the acceptance (record 0006). No type URI is reserved for an object no version signs: a later version that designs one adds its type URI under the protocol number it ships in.
- **A salt is exactly 32 bytes**, 64 lowercase hex digits, wherever one is minted, given, checked or read back: a give's part (crossing-2), every salt the mediator forms a part with (crossing-3, maybe-8, maybe-9), an answer's salt (maybe-11), a seal's salt (setup-contract-12), the salt a `resolve` step of a vector gives (conformance-vectors-6), a delivery reloaded after a restart, and the salts a party's state directory keeps. With one length, a digest splits into a salt and a body one way only. A salt of another length is refused by the code that refuses a malformed one, with no code of its own: `part-metadata` for a give, `answer-body` for an answer, `held` for a reloaded delivery; the command line skips or refuses it as it does a malformed one. What a caller does about a 31-byte salt is what it does about any defect of a part's metadata.
- **`mediator.SaltBytes = 32`** is the length in code, and `setup.SaltBytes` the seal's.
- **A2A 1.0's meaning of role.** The frame is a closed profile of the A2A 1.0 Message: it fixes the members, and the values A2A leaves open. The mediator stands where A2A's agent stands. A member's give is the client's message, `ROLE_USER`; a part the mediator forms from an answer is the agent's, `ROLE_AGENT`. The door refuses a member's give with `ROLE_AGENT` at the frame's row, with `extension-missing`, the code every other defect of the frame gets. `placket give` writes `ROLE_USER`. The schema of the message keeps both values, since it describes the message the mediator forms too. No delivery, receipt or vector carries the role of a part the mediator forms.

## Consequences

- A party's salts are 64 hex digits on disk and on the wire.
- A member that writes `ROLE_AGENT` in a give is refused; it writes `ROLE_USER`.
- The checker's loader serves the schemas under `https://placket.tech/schema/0.1/` (record 0004), and `constants-once` reads the literals that hold `placket.tech/`.
- Moving to a new protocol number is one change to every URI, every schema id, the card's `spec`, and so every signature, receipt, Accord id and vector.
