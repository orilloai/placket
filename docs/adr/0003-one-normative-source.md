# 0003. Placket's behaviour has one normative source

Status: accepted. Date: 2026-11-12. Amends record 0001.

## Context

Placket is built by many hands in several stages: the core's capability packages, then questions and release, then the door, the journal and the command line. Each needs to know exactly what placket does, and so do the people who write the conformance vectors, who must derive every code, offset and verdict by hand, and the third parties who check a receipt holding only the mediator's public key. If what placket does is spread over code, comments, design notes and pull requests, each reader assembles a different answer, and the vectors end up stating whatever the code happened to do.

Record 0001 laid out the repository before any of this text existed. It named two places for it: an `openspec/` directory for the requirement text and a process for changing it, and `spec/` for the frozen formats. One place is simpler to read, to check and to keep in step with the formats, and the requirement text needs no process of its own beyond the repository's.

## Decision

- Placket's behaviour has one normative source: `spec/protocol.md`, the protocol's requirement text, one document organized by operation. Each section opens with the operation's ordered check table and then states its requirements, each under its id, each with scenarios. The formats it defines are held beside it: their JSON Schemas in `spec/schema/`, with accepted and refused examples, and the register of formats, `spec/frozen-formats.md`. `spec/README.md` says what each part holds.
- No other document, repository or program is an authority on what placket does. Code, comments, decision records, the vectors and pull requests follow the text; where any of them disagrees with it, the text is right and the other is a defect. A change in behaviour is made in the text first.
- Requirement ids are permanent. An id names one requirement in every version and is never reused or renumbered. The ids in use are listed in `spec/ids.txt` and the ids reserved for later versions, or never used, in `spec/reserved.txt`; `make ci` checks the text against both (`specs-name-behaviour`) and checks every schema against its examples and the register (`schemas-hold`).
- The text names behaviour, never an implementation's identifiers, so it outlives any one implementation.
- An external standard is cited by its name and version, and the text says which part of it applies: JSON (RFC 8259), UTF-8 (RFC 3629), UTF-16 code units as Unicode 16.0 defines them, SHA-256 (FIPS 180-4), Ed25519 (RFC 8032), base64 (RFC 4648), timestamps as a profile of RFC 3339, DSSE 1.0, the in-toto Statement v1, the A2A protocol 1.0 message format, and JSON Schema draft 2020-12.
- This record replaces two rows of record 0001's layout, which is not edited: there is no `openspec/` directory, and `spec/` holds the protocol document, the schemas with their examples, the register, the README and the two id lists.

## Consequences

- A builder, a vector author and a verifier read the same text and reach the same result. A vector's expected code is derived from a check table, and a reviewer can check the derivation without reading code.
- The schemas are checked on every run of `make ci`: a schema that does not compile, an example on the wrong side, or a schema the register does not name fails the build, so the formats cannot drift from their examples unseen.
- A behaviour that exists only in code is a finding, and so is a code that the text does not define.
- Changing what placket does costs a change to the text, and to a format's register row when a format changes, in the same change as the code. That is slower than changing code alone, and it is the point.
- An id freed by a removed requirement stays reserved, so the lists only grow.
