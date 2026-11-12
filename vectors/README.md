# Conformance vectors

A conformance vector states inputs and the exact results an implementation of the placket protocol must produce from them. Section 10 of `spec/protocol.md` defines what a vector is, and `spec/schema/vector.json` is its format. This directory holds the vectors, the generators that write them, the coverage list that says which requirement each one covers, and this file.

The vectors cover every section of the protocol: the canonical form, signing, founding, the door and composition, questions with their answers, deadlines and release, receipts, verifying and reconciling. The coverage list leaves no requirement and no scenario open.

## Layout

| Path | What it is |
|---|---|
| `<group>/<name>.json` | One vector. Its `vector` member is this path without `.json`. The group is the section of the protocol it tests. |
| `<group>/*.go` | The group's generator: a Go command, run from this directory as `go run ./<group>`, that writes every vector of the group and removes any it no longer writes. |
| `signatures/kit/` | What the generators share: canonical bytes of the values they build, keys, salts, hashes, signatures, receipts, founding sets, messages, envelopes and the vector files. It is written from the text with the Go standard library alone. |
| `go.mod` | The generators' module, `github.com/orilloai/placket/vectors`. It requires nothing: no generator can reach the core. |
| `coverage.json` | The coverage list. |
| `README.md` | This file. |

Every `.json` file here but `coverage.json` is read as a vector. A file that is not a valid vector fails the run; nothing under `vectors/` is skipped.

## Groups and the packages they test

A group's vectors test the core packages in its row. Its generator must not import any of them, nor the conformance runner (`core/conformance`), which runs every group. The `vectors-independent` check in `make ci` reads this table: a generator in a group with no row is refused, and so is a Go file outside every group's directory.

| Group | Sections of the protocol | Packages it tests |
|---|---|---|
| `canonical` | 1 | `canonical` |
| `signatures` | 2 | `signing`, `receipts` |
| `founding` | 3 | `setup` |
| `crossing` | 4, 5 | `composition`, `mediator` |
| `receipts` | 7, 8, 9 | `receipts`, `mediator` |
| `questions` | 6 | `mediator`, `receipts` |

The `signatures` group holds, beside `sign` and `verify-signature` vectors, `verify` vectors of a receipt's payload signature, so a fault in `receipts` shows there too.

## How a vector is written and checked

A vector's expected result never comes from the code it tests (`vectors-independent` in `AGENTS.md`).

- **Codes, offsets and verdicts are written by hand** from the protocol's check tables and requirement text, in each generator's tables. The vector's `tests` line says what it tests, and opens with the requirement ids it covers.
- **Bytes, hashes and signatures are computed by the group's generator**, with the Go standard library and the kit. No generator imports any package of the core: the generators' module requires nothing, and a test of the kit (`TestGeneratorsImportNoCore`) reads the imports of every Go file here.
- **Every input an implementation could choose is given** (conformance-vectors-2): keys as seeds, salts, message ids and times. A key's seed is the SHA-256 of `placket vectors: key ` and its name, and a salt's bytes come from the SHA-256 of a label, so every run writes the same bytes.
- **Every accepted set and every accepted give is given as hex.** An accepted `found` vector gives its set as `inputHex`, its canonical bytes; every scenario's `founding` is, in canonical form, the set of an accepted `found` vector; and an accepted give gives its message as `messageHex` and its envelope as `envelopeHex`.
- **Every hash and signature is checked independently** by `tests/test_independent.sh`, which `make ci` runs. With OpenSSL 3.0 or later and shell tools, and no placket code, it re-computes the typed hashes, the salted digests, the actors' signatures and the receipts' signatures the vectors carry, prints the `openssl` version and a count per group, and fails on any one that does not hold. Where a preimage is canonical bytes the vector does not hold as hex, it rebuilds them with `jq` and proves them canonical, or fails.
- **A planted fault changes no byte.** `tests/test_vectors_independent.sh`, which `make ci` runs, plants each of its faults in turn in each package a group tests, runs the group's generator in a copy of the tree, and checks that no vector byte changed and that the runner fails on the group.
- **A landed vector is never edited to match the code.** It changes only with a format note (below), when the format itself changes.

The independent check needs these tools: `openssl` (OpenSSL 3.0 or later; LibreSSL is refused), `jq`, `basenc` and `base64` (GNU coreutils), `od`, `sed`, `grep`, `awk`, `tr` and `wc`.

To write the vectors again after changing a generator: `cd vectors && for g in canonical signatures founding crossing receipts questions; do go run ./$g; done`. The planted-fault test fails when the committed vectors are not what the generators write.

## How the runner judges a vector

The runner is the Go package `core/conformance`; `make ci` runs it over this directory as the `runner-no-silent-pass` step. It takes each vector through the checks of section 10, in order, and the first that fails decides:

1. **Header.** The file is one JSON value with no repeated name at any depth, in well-formed UTF-8, that the canonical form accepts; `vector`, `spec` and `tests` are present and strings; `vector` is the file's own path. So a vector file holds no fraction, no integer outside the safe range and no noncharacter: an input that needs one is given as hex.
2. **Schema.** The whole vector, against `spec/schema/vector.json`, at every depth. The runner reads the schema itself: it refuses a schema keyword it does not implement instead of passing over it, and it checks the schema against its own accepted and refused examples before any vector.
3. **Inputs.** Each byte input given exactly once, in one of its forms, and every hex form decodes.
4. **Operation.** The runner has a handler for the vector's operation. A vector whose operation no handler runs fails; it is not skipped.
5. **Results.** The handler reports each part it compared. A part that differs fails the vector, and so does a pending part, one the handler cannot run yet, and a handler that reports no part at all.

A vector passes or fails. Nothing is skipped and nothing is pending in a green run.

## The coverage list

`coverage.json` maps every requirement id of `spec/ids.txt`, and every scenario of `spec/protocol.md` under it, to what covers it:

```json
"canonical-4": {
  "vectors": ["canonical/duplicate-key"],
  "scenarios": {
    "a repeated name": {"vectors": ["canonical/duplicate-key"]},
    "names that differ in case": {"open": "M1"}
  }
}
```

An entry has either what covers it, or `open`, the milestone that writes its vectors (`M1`, `M2` or `M3`). What covers it is `vectors`, the names of the vectors that cover it, or `tests`, the Go tests that cover a scenario no vector operation can run, or both. A Go test is named as the directory of its package and its name, joined by a colon: `core/receipts:TestAForgedReceiptShowsAsARealOne`. An entry covered only in part also has `rest`, with `open`, the milestone that writes the rest, and `what`, one line saying what stays open, so it never reads as covered in full:

```json
"a proposal is asked about": {
  "vectors": ["questions/a-proposal-is-asked-about"],
  "rest": {"open": "M2", "what": "the question's deadline, which only a tick step shows"}
}
```

The runner prints every open entry and every entry with a `rest` on each run, and fails when:

- an id of `spec/ids.txt`, or a scenario of `spec/protocol.md`, has no entry;
- an entry names an id or a scenario the text does not have;
- an entry names a vector that is not here;
- a vector here is named by no entry;
- an entry, or its `tests` or `rest`, does not have the form above;
- no vector is found while the list names one.

`tests/test_coverage_tests.sh`, which `make ci` runs, checks the Go tests: each one the list names exists and passes when run alone, and each scenario no vector operation can run names one.

## Format notes

A change to a frozen format, the vector format included, carries a note here and in its row of `spec/frozen-formats.md`, in the same change (`format-change-noted`).

None yet: the vector format is at version 0.1 as written. The coverage list is this directory's own record, not a frozen format; its `tests` and `rest` members were added with the first vectors.

## Known limits

- `vectors-independent` reads import paths as written. A generator that reaches a tested package through another package it may import is not seen; the generators' module, which requires nothing, the kit's import test and the planted-fault run cover that.
- The planted-fault run plants the faults listed in `tests/test_vectors_independent.sh`, at least one per package a group tests and two in the mediator, one in a give and one in an answer, written against the package's code as it is. A fault whose text the code no longer holds fails the run rather than passing, and is rewritten with the code.
- The independent check proves a rebuilt preimage canonical only when it is printable ASCII with no escape. Every preimage the vectors rebuild is; one that is not fails the check and must be given as hex.
