# 0007. The checks that read the tree are one Go program, proved by its own Go tests

Status: accepted. Date: 2026-11-12. Amends record 0002. Amended by records 0013, 0015 and 0020.

## Context

`make ci` took 17 to 25 minutes on GitHub's runner, and a writer pays for it twice a round: once before pushing and once in CI. Most of that time was not the checks but their self-tests. Each planted fault of `tests/test_guardrails.sh`, `tests/test_spec.sh` and `tests/test_checks.sh` copied the repository and ran the whole of `make ci` again, or a large part of it, to see one line. The checks themselves were awk and shell programs inside the `Makefile`, reading Go line by line, so they also refused things that were not faults (`"panic("` in a string) and missed things that were (`time` imported under another name).

The tests ran three times as well: once with the race detector, once for the fuzz seeds, and once for coverage.

## Decision

- **One checker, `tools/guard/`.** Every check of `make ci` that reads the tree is a rule of one Go program: `core-no-deps`, `core-imports`, `core-no-world`, `core-no-panic`, `specs-name-behaviour`, `schemas-hold`, `vectors-independent`, `constants-once`, `product-words-once`, `claims-stop-at-crossing`, `main-is-thin`, `root-module`, `machine-rules-in-ci` and `ci-stays-fast`. It is a Go module of its own, `github.com/orilloai/placket/tools/guard`, outside `core/`, so the core keeps its rule of no dependency, and the module loop formats, vets, lints, scans and tests it like every other module.
- **What it reads.** It reads Go with the standard library's parser (`go/parser`), every `.go` file in a rule's scope whatever its build constraints, and refuses a file it cannot parse, at its line, rather than skip it. It resolves names without types: a selector `x.Now` is the clock when `x` is the name the file imports `time` under; a call `print(...)` is the built-in unless the package declares a `print` of its own in a file every build includes, with no build line, no system or architecture suffix, and the caller's package clause. It reads `go.mod` files, the `Makefile`, scripts and the requirement text as lines, as the checks did before. It reads the tree from the directory it runs in and changes nothing.
- **One dependency, for `schemas-hold`.** The schemas are checked with the JSON Schema draft 2020-12 validator `github.com/santhosh-tekuri/jsonschema/v6`, at v6.0.1, the version the `jv` command the check used before is built from, so the validator is the same code as before. Its loader serves the schemas' own URLs, under `https://placket.dev/schema/0.1/`, from `spec/schema/` and refuses every other URL, so nothing is fetched.
- **One build, one target per rule.** `make ci` builds the checker once (target `guard`); each rule keeps its own target, `make <rule>`, which runs the built checker for that rule alone. Each prints what the checks printed before: `<rule>: ok`, or a line per fault starting `<rule>: <file>[:<line>]: ` and then `<rule>: refused: <why> (AGENTS.md, <rule>)`.
- **Its cases are Go tests.** Every planted fault of the removed scripts is a case of the checker's tests (`tools/guard/*_test.go`), with its fixture under `tools/guard/testdata/`. A case copies the repository once, lays its fixture over the copy, runs every rule in-process and asserts the lines of the rule it plants a fault for and the silence of all the others, as the scripts did, but in milliseconds rather than a run of `make ci`. The cases that plant faults in the protocol text, the schemas and the `Makefile` plant them in a copy of the live files and compute their expected lines from them. A test fails when a rule of the checker has no case that refuses.
- **The fixtures moved with their cases.** The fixtures of the checker's cases live beside its tests, in `tools/guard/testdata/`. The fixtures of the cases that stay shell tests, the toolchain steps and the module loop, are in `tests/testdata/checks/`. `tests/test_guardrails.sh`, `tests/test_spec.sh` and `tests/testdata/guardrails/` are gone.
- **One wiring test.** `tests/test_wiring.sh` plants one fault for every rule in one copy, runs the rules' targets and `modules` once with `make -k`, and checks that each rule refuses its own fault and no other, and that every step prints its time. It also checks that a `make ci` started inside another is refused at once.
- **One test run per module.** The race detector, the fuzz seeds and coverage share one `go test -race -cover -count=1` run per module (target `tests`); `race`, `fuzz-seeds` and `coverage-reported` each read it and print their own lines.
- **Times, and a marker.** Every step prints `time: <step>: <n>s` after its output. `make ci` exports `PLACKET_IN_CI=1`, and a `make ci` that finds it set stops before any step (`ci-stays-fast`); the checker refuses a test or script that clears it. The hard limit on time is the workflow's.

## How a check is added

A rule is added to the checker in a rules change (`CONTRIBUTING.md`, kind 2): a function in `tools/guard/`, a row of its rule table, a target in the `Makefile` named in `ci`, its cases in the checker's tests (at least one refused), and its text in `AGENTS.md`. `machine-rules-in-ci` refuses a `Makefile` whose `ci` does not name every rule of the table, so a rule cannot be added and left out of `ci`.

## Consequences

- `make ci` drops from about 17 minutes to a few. The checker's tests take seconds, and a planted fault still shows the exact lines a writer would see.
- The parser sees what the line scans could not: a renamed or dot import of `time` or `fmt`, and a string that only looks like a call or an import. What it still misses is in `AGENTS.md`, "Known limits of the checks".
- The checker is code with a dependency, pinned in `tools/guard/go.sum`. That file also lets the workflow's Go cache take effect, since it is keyed on every module's `go.sum`.
- A check and its tests now change in Go, in one module, under the same lock commits as any other test; a rules change still stands alone and only the maintainer merges it.
