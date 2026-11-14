# The placket protocol specification

This directory is the one normative source of what placket does. Placket is a mediator between parties that do not fully trust each other: at a handoff it decides whether a part may cross under the terms both sides declared, asks a named person where the terms say to, and signs a receipt for every decision, which a third party verifies with the mediator's public key alone.

Its scope, in its own words: placket judges what crosses a handoff and nothing after; it makes no claim about what a receiver does with what it received, not training on it, sharing it, retaining it, or the purpose it is put to.

Version 0.1 covers the first job, ask and log, and its proof: founding an Accord, giving and fetching a part, the three answers to a question and release, receipts, their verification and reconciliation, and the conformance vectors that check an implementation; and, in its last two sections, the JSON object each command of placket's command line prints for a script and the log of a journal.

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
| `forbidden-check-1` to `forbidden-check-8` | The forbidden check and the registration of what it looks for. They come after this version. Until then a card loads no check, the term's `checks` and the reason `check:<name>` are reserved slots, and the type URI of a registration is reserved (signing-4). |

## Changing the protocol

A change to what placket does starts here, in `protocol.md` and the vectors, before any code. A change to a format carries a note in its row of `frozen-formats.md` in the same change. `CONTRIBUTING.md` at the root of the repository says how a requirement or a format changes and who decides.
