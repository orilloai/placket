# State of this branch

## Done
- Round 1: `tests/test_guardrails.sh` with fixtures in `tests/testdata/guardrails/`; four guardrail targets in the `Makefile` (`core-no-deps`, `core-imports`, `core-no-world`, `core-no-panic`) before the module loop; `core/go.mod`, `core/README.md`; `AGENTS.md`, `CONTRIBUTING.md`, `docs/adr/0001-repository-layout.md`, `docs/adr/0002-core-boundaries.md`.
- Round 2: comment-aware scanning across lines, wider `core-no-world`, `go list` failures fail the module loop.
- Round 3 (this one), 41 self-test cases:
  - every check refuses a symbolic link anywhere under `core/` (`<rule>: <path>: a symbolic link`);
  - `core-imports` and `core-no-world` refuse any backslash on an import line (escaped import paths);
  - `core-no-world` refuses `import "C"` (import lines only) and non-Go sources outside `testdata/` (`<rule>: <file>: a source file that is not Go`), by the extensions the Go tool builds;
  - `core-no-world` reads a selector split after its dot (line break, spaces or comment) as one name;
  - `world-allowed` pins what `core-no-world` lets through (`fmt.Fprint*`, `fmt.Sprint*`, a field call `s.print(`, `"C"` and escapes in plain strings, assembly under `testdata/`).

## Decisions to keep
- Output format: `<rule>: ok`; `<rule>: <file>:<line>: <code>` per hit, or `<rule>: <file>: <what>` for a whole-file refusal; then `<rule>: refused: <reason> (AGENTS.md, <rule>)`. The self-tests rely on it.
- Checks stay shell and awk in the `Makefile`; no parser. Import lines are found by state in `GUARD_SCAN` (an `import` line, or the lines of its parenthesised block), relying on `gofmt` form, which the module loop enforces.
- Refusals are plain: no symlinks, no backslash in an import path, no cgo or non-Go sources. None of these is listed as a known limit because each is refused.
- Lists of the four core rules keep the order `core-no-deps`, `core-imports`, `core-no-world`, `core-no-panic`.
- Self-tests that need a symlink make it at run time through `$setup`; no symlink is committed.
- The fixture module `tests/testdata/guardrails/modules/broken` does not compile on purpose.

## Gotchas
- The lock job treats every added `*_test.go`, fixtures included, as a Go test. A fixture `_test.go` must not add a `func Test...` or `func Fuzz...` line.
- A lock commit that only edits `tests/test_guardrails.sh` gets a warning, not a red/green check; the reviewer judges it. In this round's lock commit the eight new refusal cases are red and `world-allowed` is green by design (it guards existing behaviour).

## Open
- Nothing blocking. A renamed or dot import of `time` or `fmt` still gets past `core-no-world` (listed in `AGENTS.md`).
- Review finding on the `make ci` rewrite: the maintainer ruled it won't fix; nothing changed for it.
