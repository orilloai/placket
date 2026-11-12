# Conformance vectors

A conformance vector states inputs and the exact results an implementation of the placket protocol must produce from them. Section 10 of `spec/protocol.md` defines what a vector is, and `spec/schema/vector.json` is its format. This directory holds the vectors, the coverage list that says which requirement each one covers, and this file.

No vector is written yet. The format, the runner and the coverage list come first, so that the first vector lands where a runner already refuses to pass what it did not run. M1 writes the vectors of the core's capability packages, and M2 those of questions, the three answers and release.

## Layout

| Path | What it is |
|---|---|
| `<group>/<name>.json` | One vector. Its `vector` member is this path without `.json`. The group is the section of the protocol it tests. |
| `<group>/...` | The group's generator: the Go code that writes its vectors, and the generator's own tests. |
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

The package names are the planned ones. The change that creates a package under another name corrects its row in the same change.

## How a vector is written and checked

A vector's expected result never comes from the code it tests (`vectors-independent` in `AGENTS.md`).

- **Codes, offsets and verdicts are written by hand** from the protocol's check tables and requirement text. The vector's `tests` line says what it tests.
- **Bytes, hashes and signatures are computed by the group's generator**, with the Go standard library or with a lower package whose own vectors already pass. A generator imports no package its group tests, as the table above gives them.
- **Every signature and hash is checked independently** before the vector lands: verified with an Ed25519 and a SHA-256 other than the ones the generator used, and the result named in the change that adds the vector.
- **A planted fault changes no byte.** Breaking a package the group tests and running the generator again must leave every vector of the group unchanged.
- **Every input an implementation could choose is given**: keys as seeds, salts, message ids and times (conformance-vectors-2). Two implementations run on one vector produce the same bytes.
- **A landed vector is never edited to match the code.** It changes only with a format note (below), when the format itself changes.

## How the runner judges a vector

The runner is the Go package `core/conformance`; `make ci` runs it over this directory as the `runner-no-silent-pass` step. It takes each vector through the checks of section 10, in order, and the first that fails decides:

1. **Header.** The file is one JSON value with no repeated name at any depth, in well-formed UTF-8; `vector`, `spec` and `tests` are present and strings; `vector` is the file's own path. Until the canonical form's package exists, the runner reads the file with a strict reader of its own; it then reads it with the canonical form.
2. **Schema.** The whole vector, against `spec/schema/vector.json`, at every depth. The runner reads the schema itself: it refuses a schema keyword it does not implement instead of passing over it, and it checks the schema against its own accepted and refused examples before any vector.
3. **Inputs.** Each byte input given exactly once, in one of its forms, and every hex form decodes.
4. **Operation.** The runner has a handler for the vector's operation. A vector whose operation no handler runs fails; it is not skipped.
5. **Results.** The handler reports each part it compared. A part that differs fails the vector, and so does a pending part, one the handler cannot run yet, and a handler that reports no part at all.

A vector passes or fails. Nothing is skipped and nothing is pending in a green run.

## The coverage list

`coverage.json` maps every requirement id of `spec/ids.txt`, and every scenario of `spec/protocol.md` under it, to the vectors that cover it:

```json
"canonical-4": {
  "vectors": ["canonical/duplicate-key"],
  "scenarios": {
    "a repeated name": {"vectors": ["canonical/duplicate-key"]},
    "names that differ in case": {"open": "M1"}
  }
}
```

An entry has either `vectors`, the names of the vectors that cover it, or `open`, the milestone that writes them (`M1`, `M2` or `M3`). The runner prints every open entry on each run, and fails when:

- an id of `spec/ids.txt`, or a scenario of `spec/protocol.md`, has no entry;
- an entry names an id or a scenario the text does not have;
- an entry names a vector that is not here;
- a vector here is named by no entry;
- no vector is found while the list names one.

With every entry open, as now, a run that finds no vector passes and prints what is still open. The change that adds a vector moves its entries from `open` to `vectors` in the same change.

## Format notes

A change to a frozen format, the vector format included, carries a note here and in its row of `spec/frozen-formats.md`, in the same change (`format-change-noted`).

None yet: the vector format is at version 0.1 as written.

## Known limits

- `vectors-independent` reads import paths as written. A generator that reaches a tested package through another package it may import is not seen; the reviewer reads a generator's imports, and the planted-fault run catches the result.
- The runner's header reader refuses what the JSON grammar, UTF-8 and repeated names refuse, and nesting beyond 1000 levels. The canonical form's other refusals, such as a fraction or an integer outside the safe range, reach the header check with the canonical form's package.
