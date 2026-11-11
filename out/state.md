# State of this branch

## Done
- Lock commit: `tests/test_guardrails.sh` and its fixtures in `tests/testdata/guardrails/` (23 cases).
- `Makefile`: four guardrail targets (`core-no-deps`, `core-imports`, `core-no-world`, `core-no-panic`) run before the module loop; the module loop skips `testdata/` modules and modules with no Go packages, and gofmt skips `testdata/`.
- `core/go.mod` (no require, go 1.27, toolchain go1.27.4) and `core/README.md`.
- `AGENTS.md`, `CONTRIBUTING.md`, `docs/adr/0001-repository-layout.md`, `docs/adr/0002-core-boundaries.md`, root `README.md`.

## Decisions to keep
- Every line a guardrail check prints starts with its rule name: `<rule>: ok` on pass, `<rule>: <file>:<line>: <code>` per hit, then `<rule>: refused: <reason> (AGENTS.md, <rule>)`. The self-tests rely on this.
- Checks are inline in the Makefile (no `scripts/` dir yet). One awk scanner (`GUARD_SCAN`) strips `//` comments, treating `//` inside "...", '...' and `...` on the same line as not a comment; string literals stay in the code, so `"panic("` is refused (stated in AGENTS.md).
- The comment strip applies to all four checks, not only `core-no-panic`.
- `core-imports` covers every `.go` file under `core/`, tests and testdata included. `core-no-world` and `core-no-panic` skip `_test.go` and `testdata/`.
- `core-no-world` matches import paths for `crypto/rand`, `math/rand(/v2)`, `net(/...)`, and `time.Now`, `os.Getenv`, `os.Open` (so `OpenFile` too), `fmt.Print*` by name.
- Self-tests copy the repo minus `.git` and `tests/test_*.sh` and run `make ci` in the copy, so the nested run never recurses.
- Lists of the four core rules (Makefile targets, the self-tests' `rules`, CONTRIBUTING.md) put `core-no-deps` first, then `core-imports`, `core-no-world`, `core-no-panic`. Keep that order; a later round that adds a list follows it.
- The fixture module `tests/testdata/guardrails/modules/broken` does not compile on purpose: the clean-tree case proves testdata modules are skipped.

## Gotchas
- The lock check treats every `*_test.go` it sees added, fixtures under `testdata/` included, as a Go test to run at the head. A fixture `_test.go` must not add a `func Test...` or `func Fuzz...` line; `world-in-test` uses `ExampleLater` instead.

## Open
- Nothing blocking. The other machine checks, the requirement text and the runner come in later changes.
