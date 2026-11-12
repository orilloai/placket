# Contributing to placket

How a change to placket is made and how it is reviewed. The rules a change keeps are in `AGENTS.md`, each under a public name; this file is the process around them.

## Before you start

- Read `AGENTS.md`: where things go, and the rules by name.
- Read the decision records in `docs/adr/` that touch your change.
- Know which kind of change you are making. There are three, and they never mix:
  1. **A protocol or code change**: requirement text, formats, vectors, Go code.
  2. **A rules change**: `AGENTS.md`, this file, `review/`, or a check in `make ci`.
  3. **A workflow change**: anything under `.github/workflows/`.

  A rules or workflow change is its own pull request, flagged as such, and only the maintainer merges it. A change cannot alter the checks that judge it.

## Making a change

1. **Decide first.** If the change makes a decision that spans packages or would be expensive to reverse, write its record in `docs/adr/` (`NNNN-<slug>.md`, the next free number; context, decision, consequences) before or in the change that relies on it. If the change would alter what placket does for someone, what a capability covers, or what it must not do, stop and ask the maintainer. Any other choice is yours: take the sensible default and list it in the pull request, where it can be vetoed.
2. **Behaviour in the text first.** A change in behaviour starts in the requirement text (`spec/requirements/`) and the vectors (`vectors/`); the code follows. Behaviour that exists only in code is a finding. A frozen-format change carries its version note in the same change (`format-change-noted`).
3. **Tests first, locked.** Write the tests before the implementation and commit them alone:

   ```
   git commit -s --trailer 'Loom-Lock: tests' -m 'Add tests for <what>'
   ```

   That commit touches only test files (`*_test.go`, anything under a `testdata/` directory, anything under `tests/`). Every test it adds must fail at that commit and pass, not skip, at the head of the branch; the `lock` check runs each one at both points. Test files change only in such commits: a later need for new or changed tests is a new lock commit, so every test change is in plain sight. Never weaken, skip or delete a test to get a change through.
4. **Then the code**, in small commits.
5. **Run `make ci`** before you push. It runs the guardrail checks (`core-no-deps`, `core-imports`, `core-no-world`, `core-no-panic`), then every Go module's format, vet and tests, then `tests/test_*.sh`. A refusal starts with the rule's name and names the file and line; look the name up in `AGENTS.md`. GitHub runs the same `make ci` on every push and pull request, and that run is the one that counts.

## Commits

- Every commit is signed off: `git commit -s`.
- No merge commits on a branch: rebase onto `main` instead.
- No co-author trailer for a model or tool, and no "generated with" line anywhere.
- A commit message says what changed and why, in placket's own words.

## Pull requests

- Never a draft. The maintainer merges every pull request.
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
- **The tests themselves.** The reviewer confirms the tests came first and were red, that every test change is in a `Loom-Lock: tests` commit, and that no test, check or workflow was weakened. A change to `make ci`'s checks outside a rules change is a finding.
- **Mutation testing.** From the first code milestone, the reviewer of a code change breaks lines on purpose and reports which ones the tests did not notice.
- **When to stop.** When findings reach diminishing returns, the review stops and what is left goes to a follow-up, named in the pull request.
