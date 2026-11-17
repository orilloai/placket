# The placket protocol specification

This directory is the one normative source of what placket does. Placket is a mediator between parties that do not fully trust each other: at a handoff it decides whether a part may cross under the terms both sides declared, asks a named person where the terms say to, and signs a receipt for every decision, which a third party verifies with the mediator's public key alone.

Its scope, in its own words: placket ends where a signed call is made. It judges what crosses a handoff, records who decided, and nothing after; it makes no claim about what a receiver does with what it received, not training on it, sharing it, retaining it, or the purpose it is put to, and runs no command on any party's behalf.

Version 0.1 covers the first job, ask and log, and its proof: founding an Accord, giving and fetching a part, the judges a term may grant, the three answers to a question as signed calls, and release, receipts that name who decided, their verification, their conformance to the Accord and reconciliation, and the conformance vectors that check an implementation; and, in its last two sections, the JSON object each command of placket's command line prints for a script, the check of a file, of a value inside it, or of everything a key kept, against what crossed, and the log of a journal.

## What each part holds

Read them in this order.

| File | What it holds |
|---|---|
| `protocol.md` | The requirement text, one section per operation, in the order a reader meets them. Each section opens with its ordered check table, then its requirements under their ids, each with scenarios. Start here. |
| `frozen-formats.md` | The register: one row per format, with the requirements that define it, its schema files, its version, what binds it and what changing it costs. |
| `schema/` | The JSON Schema (draft 2020-12) of each format that has a JSON shape. Each schema's examples are under `schema/testdata/<name>/`: `accepted/` holds examples it must validate, and `refused/` holds one example per rule, named for the rule it breaks. |
| `ids.txt` | Every requirement id this version defines, one per line. |
| `reserved.txt` | Every id this version reserves, one per line. |

## Requirement ids

A requirement id is permanent. Once given, it names the same requirement in every later version: it is never reused for another and never renumbered, and a requirement that is removed leaves its id reserved. `make ci` checks that every heading of `protocol.md` is an id of `ids.txt`, each once, each with a scenario, and that no id of `reserved.txt` is used.

The reserved ids, and why each is reserved:

| Id | Why |
|---|---|
| `signing-3` | Never used. The signing requirements are numbered 1, 2 and 4, and 3 stays empty so the others keep their numbers. |
| `maybe-2` | A question put to its answerers as a Model Context Protocol elicitation request. It comes after this version, with the door that would carry it. |
| `forbidden-check-1` to `forbidden-check-8` | The forbidden check and the registration of what it looks for. They come after this version, if ever; a card names judges, not checks, and a registration has no type URI in this version (signing-4). |
| `composition-4` | The checks a term named, run before a maybe was read. A term names judges in its `resolve` instead (composition-7), and no check slot remains. |

## What this specification commits to

- **A verifier for every released version.** Placket keeps a verifier available for every format version it releases, in whatever form makes sense, so a journal written under a released version can always be checked.
- **A countersignature is a statement of its own.** A later signature over a receipt, by anyone, is a separate statement over that receipt's id. It never changes the receipt or what the receipt claims.
- **One Accord, one version.** An Accord is founded under one protocol version, the card's `spec`, and every receipt it writes is of that version. A later version means a new Accord; the earlier one may be named in the new policy's `supersedes`.
- **Five invariants.** Each holds of every version:
  1. *Parties sign locally.* Every declaration, binding, envelope and answer call is signed by its own actor's key; the mediator signs only receipts. Held by the founding table, the door's table and section 6's table, and by `call-signing` and `keys-on-disk`.
  2. *No global state in the core.* The core takes the time, salts and keys as parameters and reads nothing of the world. Held by `core-no-world` and `inputs-as-parameters`.
  3. *One tool per call.* A door call does one operation, and an answer is one signed call. Held by review, and by the door's check order (`door-check-order`).
  4. *Verify never needs a body.* Validity and conformance read receipts, the founding set and answer calls, none of which holds a body or a salt; only the check of one delivery reads its own. Held by verify-1's table and `core/conform`'s tests.
  5. *No judge changes the terms.* A judge decides only where both sides' terms would ask and both granted it, never at release and never on a part the mediator formed. Held by composition-7 and `one-decision-module`.
- **Scope.** Placket ends where a signed call is made: what the edges do with a part, a release or a decision is theirs.

## Changing the protocol

A change to what placket does starts here, in `protocol.md` and the vectors, before any code. A change to a format carries a note in its row of `frozen-formats.md` in the same change. `CONTRIBUTING.md` at the root of the repository says how a requirement or a format changes and who decides.
