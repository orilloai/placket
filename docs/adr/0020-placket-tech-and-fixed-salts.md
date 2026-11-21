# 0020. Every placket URI under placket.tech and one protocol number, salts of exactly 32 bytes, and A2A's meaning of role

Status: accepted. Date: 2026-11-17. Amends records 0005, 0006, 0007, 0010, 0012 and 0014.

## Context

Placket 0.1 is not released. No format is frozen yet, no verifier of it is in anyone's hands, and every id, signature and vector can still be written again. Four mechanical defects in the formats would each cost a full regeneration of every vector, id and signature if fixed one at a time, and would cost a new protocol version each if fixed after the release:

- **The domain.** Every schema `$id` and every signed type URI was under `https://placket.dev/`. Placket's home is `placket.tech`, and `placket.dev` will only redirect there. A type URI is signed into every declaration, binding and envelope, and the receipt's predicate type into every receipt, so the host is part of every signature.
- **The protocol number.** Each URI ended in `0.1`, and the card's `spec` was `0.1`, but nothing said that these were the same number, or that they move together.
- **The salt.** A salt was "at least 16 bytes". The salted digest of crossing-3 is the SHA-256 of the salt's bytes followed by the body's bytes, with nothing between them. With a salt of any length from 16 bytes up, one digest opens as more than one (salt, body) pair: the salt and the body, or a longer salt that takes the body's first bytes and a shorter body that is the rest. A holder of a part could show a suffix of what crossed as if it were the part the receipt names.
- **The role.** The crossing message is an A2A 1.0 message, but placket gave its `role` the reverse of A2A's meaning: `ROLE_AGENT` for a part a member gives and `ROLE_USER` for one the mediator forms, and the door accepted either from a member. In A2A 1.0 the client sends `ROLE_USER` and the agent answers `ROLE_AGENT`. Reusing A2A's names with another meaning is a trap for anyone who reads the frame as A2A.

Signing-4 also reserved a fourth type URI, for a registration that no object of 0.1 is signed or verified under. Freezing 0.1 would commit placket to that URI for a feature nobody has designed.

## Decision

- **One host.** Every schema `$id`, every signed type URI (signing-4), the receipt's predicate type (receipt-1) and the crossing extension's URI (crossing-1) move to `https://placket.tech/`. Only the host changes: a type URI is `https://placket.tech/<kind>/0.1`, a schema's `$id` is `https://placket.tech/schema/0.1/<name>.json`. The payload types (`application/vnd.placket.call+json`, `application/vnd.in-toto+json`), the in-toto Statement URI, the digest name `salted-sha256` and the door's 16-byte `nonce`, which is not a salt, do not change.
- **One protocol number.** Every placket URI carries the version the card's `spec` names (setup-contract-4): a type URI ends in it, and a schema's `$id` holds it as the segment after `schema/`. This version knows one value, `0.1`; a card of any other `spec` is refused at founding, as before. A later version changes every URI and the card's `spec` together, in one change. The number stays in each URI as a whole literal in its one home (`signing`, `receipts`, `mediator`): `signing` cannot import `setup`, since `setup` imports `signing`, and a package of its own for one string is not worth it. `TestEveryURIUnderTheCardsSpec` in `core/mediator` holds each URI to `setup.Spec`.
- **A salt is exactly 32 bytes**, 64 lowercase hex digits, wherever one is minted, given, checked or read back: a give's part (crossing-2), every salt the mediator forms a part with (crossing-3, maybe-8, maybe-9), the salt a `resolve` step of a vector gives (conformance-vectors-6), a delivery reloaded after a restart (record 0008), and the salts a party's state directory keeps. With one length, a digest splits into a salt and a body one way only. A salt of another length is refused by the code that refused a short one before: `part-metadata` for a give, the mediator's `salt` error for an answer, `held` for a reloaded delivery; the command line skips or refuses it as it did a malformed one. There is no new protocol code: what a caller does about a 31-byte salt is what it does about any defect of a part's metadata.
- **`mediator.SaltBytes = 32`.** It replaces `MinSaltBytes`, which stayed as a deprecated alias equal to it, as `api-compat` asks, and was removed before v0.1.0, so no release carries it.
- **A2A 1.0's meaning of role.** The frame is a closed profile of the A2A 1.0 Message: it fixes the members, and the values A2A leaves open. The mediator stands where A2A's agent stands. A member's give is the client's message, `ROLE_USER`; a part the mediator forms from an answer is the agent's, `ROLE_AGENT`. The door refuses a member's give with `ROLE_AGENT` at the frame's row, with `extension-missing`, the code every other defect of the frame gets. `placket give` writes `ROLE_USER`. The schema of the message keeps both values, since it describes the message the mediator forms too. No delivery, receipt or vector carries the role of a part the mediator forms.
- **No reserved registration type.** Signing-4 names three type URIs, for the declaration, the binding and the envelope. `signing.TypeRegistration` is removed with no deprecated alias, since an alias would keep the reservation alive. A later version that designs a registration adds its type URI under the protocol number it ships in.
- **Nothing old stays readable.** No reader, flag or mode accepts a `placket.dev` URI or a salt of another length. A journal, state directory or receipt made before this change is refused like any foreign one; there is no migration command, because no release has made one.
- **The committed demo journal is founded again** under `placket card --default`, so its card names the module this build runs and `verify --founding` on it prints no other-module line. `tests/test_demo.sh` asserts that. So a change that alters the kernel hash (`core/composition/composition.go`) or a frozen format writes the demo journal again, and quotes the README's first receipt and its `show` output again, in its own pull request; otherwise `make ci` fails.

## What it amends

- **0005.** `signing`'s constants are `TypeDeclaration`, `TypeBinding` and `TypeEnvelope`; `TypeRegistration` is gone, and `SignPayload` and `VerifyPayload` refuse those three.
- **0006.** An accept or a change whose salt is not exactly 32 bytes is the `salt` error; it was one of fewer than 16 bytes.
- **0007.** The checker's loader serves the schemas under `https://placket.tech/schema/0.1/`, and `constants-once` reads the literals that hold `placket.tech/`.
- **0010.** The door mints a 32-byte salt for every accept and change.
- **0012 and 0014.** The committed demo journal is founded under the default card and names the module this build runs; `verify --founding` on it prints no other-module line.

## Consequences

- Every schema id, every signature by an actor, every receipt, every Accord id, every vector and the demo journal change once. No released verifier exists, so nothing outside the repository breaks.
- A party's salts are twice as long on disk and on the wire, 64 hex digits instead of 32.
- A member that wrote `ROLE_AGENT` because the old text asked for it is refused; it writes `ROLE_USER`.
- `placket.dev` stays only in refusal fixtures that prove an old URI fails, in `CHANGELOG.md` and in the records accepted before this one, which are not edited.
