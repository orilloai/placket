# Contributing to placket

How a change to placket is made and how it is reviewed. The rules a change keeps are in `AGENTS.md`, each under a public name; this file is the process around them.

## Before you start

- Read `AGENTS.md`: where things go, and the rules by name.
- Read the decision records in `docs/adr/` that touch your change.
- Know which kind of change you are making. There are three, and they never mix:
  1. **A protocol or code change**: requirement text, formats, vectors, Go code.
  2. **A rules change**: `AGENTS.md`, this file, `review/`, or a check: the `Makefile`'s checks and steps, the checker in `tools/guard/` and the check `red-green-tests` in `tools/redgreen/`, with their tests.
  3. **A workflow change**: anything under `.github/workflows/`.

  A rules or workflow change is its own pull request, flagged as such, and only the maintainer merges it. A change cannot alter the checks that judge it.

## Making a change

1. **Decide first.** If the change makes a decision that spans packages or would be expensive to reverse, write its record in `docs/adr/` (`NNNN-<slug>.md`, the next free number; context, decision, consequences) before or in the change that relies on it. If the change would alter what placket does for someone, what a capability covers, or what it must not do, stop and ask the maintainer. Any other choice is yours: take the sensible default and list it in the pull request, where it can be vetoed.
2. **Behaviour in the text first.** A change in behaviour starts in the requirement text (`spec/protocol.md`) and the vectors (`vectors/`); the code follows. Behaviour that exists only in code is a finding. A frozen-format change carries its version note in the same change (`format-change-noted`).
3. **Tests first.** Write the tests before the implementation; committing them before the code is a good habit, and lets the history show it. Every test the change adds or changes must fail against `main` and pass, not skip, at the head of the branch. The check `red-green-tests` proves it on every pull request: it runs each such test at the merge base with `main`, with the branch's test files laid over it, and then at the head (`docs/adr/0013-red-green-tests.md`). It reads only those two commits, so squash, merge `main` in, or mix tests and code in a commit as you like. A test that does not build without the new code counts as red. Run it yourself before a push, after committing, with `make red-green-tests BASE=origin/main`.

   Never weaken, skip or delete a test to get a change through. When a change honestly breaks the rule, a trailer on any of its commits says so and why, and the reviewer judges the reason:

   - `Green-On-Base: <test>: <why>`: an added test that already passes on `main`, such as coverage of behaviour that already works and was untested.
   - `Test-Weakened: <test>: <why>`: a test removed, or a skip added, such as a test retired with its feature, or the old half of a rename.
   - `No-Test: <why>`: a change to Go code under `core/`, `adapters/`, `cmd/`, `recipes/` or `tools/` with no test added or changed, such as a comment or a rename.

   A `<test>` is a Go test's name, or a shell test's path such as `tests/test_x.sh`. Add one with `git commit -s --trailer 'Green-On-Base: TestX: <why>'`. A trailer naming a test the change does not touch is refused.
4. **Then the code**, in small commits.
5. **Run `make ci`** before you push. It builds the checker in `tools/guard/` (`guard`) and runs each of its rules, the checks that read the tree (`core-no-deps`, `core-imports`, `core-no-world`, `core-no-panic`, `specs-name-behaviour`, `schemas-hold`, `vectors-independent`, `constants-once`, `product-words-once`, `claims-stop-at-crossing`, `main-is-thin`, `root-module`, `machine-rules-in-ci`, `ci-stays-fast`); then every Go module's format (`modules`), `vet`, `lint`, `vulnerabilities` and `secrets`; one test run per module with the race detector and coverage (`tests`), which `race`, `fuzz-seeds` and `coverage-reported` read; the conformance runner (`runner-no-silent-pass`); and `tests/test_*.sh` (`scripts`). `make <step>` runs one of them. A refusal starts with the rule's name and names the file and line; look the name up in `AGENTS.md`. Every step prints its time. Never run `make ci` from inside a test: a nested run is refused (`ci-stays-fast`). GitHub runs the same `make ci` on every pull request and every push to `main`, and that run is the one that counts. To get GitHub's run before the change is ready, open the pull request as a draft.

## Patterns

A pattern is a runnable template for one caller's job, under `patterns/<job>/`; `patterns/README.md` states what one is made of. `make ci` runs every pattern (`tests/test_patterns.sh`), and a new one needs no change to it.

- A pull request that changes a command a pattern uses keeps that pattern green in the same pull request: its `pattern.sh` and `expected.txt` change with the command.
- A pattern never scripts around a gap in placket. When a job needs something placket does not do, that gap is an issue to file, not a workaround in a pattern.
- A pattern depends only on the `placket` binary and `jq` on `PATH`, never on another file of this repository, so `patterns/` can move to a repository of its own as it is.

## Commits

- Every commit is signed off: `git commit -s`.
- No co-author trailer for a model or tool, and no "generated with" line anywhere.
- A commit message says what changed and why, in placket's own words.

## Pull requests

- A draft is fine while the change is in progress; mark it ready for review when it is. The maintainer merges every pull request.
- The body starts with a short summary, two to four sentences long, followed by:
  - **Why**: one to three sentences.
  - **Important points**: three to six bullets: what the maintainer might veto, surprises, behaviour changes.
  - **Risk**: a level, and why.
  - **How it was checked**: the tests in one line, then the reviews summarised, not finding by finding.
  - **Reading map**: what to read closely, what to skim.
  - Any review record goes last, collapsed.

## How a change is reviewed

- **By someone fresh.** A reviewer did not write the change. A review pass runs to its stop rule without pausing to check in.
- **Against the rules, by name.** The reviewer reads the diff against `AGENTS.md`. A finding names the rule (`core-no-world`), the file and line, and what is wrong.
- **Beyond the machine.** `make ci` passing is the floor. The reviewer reads what no check sees: the imports of every file changed in `core/`, where logic sits, whether each test compares exactly, whether each new exported name traces to a requirement or a decision.
- **The tests themselves.** The reviewer reads `red-green-tests`'s report: judges every trailer's reason, each test that was red only because it did not build (red (build)), each changed test that was green on base, and each changed helper. The reviewer confirms that no test, check or workflow was weakened. A change to `make ci`'s checks outside a rules change is a finding.
- **Mutation testing.** The reviewer of a code change breaks lines on purpose and reports which ones the tests did not notice.
- **When to stop.** When findings reach diminishing returns, the review stops and what is left goes to a follow-up, named in the pull request.
