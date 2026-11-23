# 0004. How a change is checked: one checker, rules in topic files, red-green tests

Status: accepted. Date: 2026-11-23.

## Context

A writer pays for `make ci` twice a round, once before pushing and once in CI, so its checks must be fast, and each must read the tree as Go does: a check written as a line scan refuses things that are not faults (`"panic("` in a string) and misses things that are (`time` imported under another name). Its self-tests must be fast too: a planted fault that copies the repository and runs the whole of `make ci` again costs minutes to see one line.

Every agent loads the entry file on every task, whatever the task touches. Some harnesses stop reading instruction files at 32 KiB and drop the rest without a word, and an agent follows long lists of instructions less well than short ones. Outside contributors need the files an open project has.

Tests come first in placket: a test is written before the code it proves, so it fails without that code and passes with it. The property wanted is about the change as a whole, not the shape of its commits: squashed commits, merges from `main` and commits that mix tests and code are normal everywhere, and a check of the commits' shape refuses real work for reasons that have nothing to do with tests coming first. Once the repository is public, anyone may open a pull request, and each commit an outside contributor sends must carry their own sign-off.

## Decision

### One checker

- **One Go program, `tools/guard/`.** Every check of `make ci` that reads the tree is a rule of one Go program, a module of its own, `github.com/orilloai/placket/tools/guard`, outside `core/`, so the core keeps its rule of no dependency, and the module loop formats, vets, lints, scans and tests it like every other module. `docs/rules/README.md` lists every rule, the checker's among them.
- **What it reads.** It reads Go with the standard library's parser (`go/parser`), every `.go` file in a rule's scope whatever its build constraints, and refuses a file it cannot parse, at its line, rather than skip it. It resolves names without types: a selector `x.Now` is the clock when `x` is the name the file imports `time` under; a call `print(...)` is the built-in unless the package declares a `print` of its own in a file every build includes, with no build line, no system or architecture suffix, and the caller's package clause. It reads `go.mod` files, the `Makefile`, scripts and the requirement text as lines. It reads the tree from the directory it runs in and changes nothing.
- **One dependency, for `schemas-hold`.** The schemas are checked with the JSON Schema draft 2020-12 validator `github.com/santhosh-tekuri/jsonschema/v6`, pinned at v6.0.1 in `tools/guard/go.sum`. Its loader serves the schemas' own URLs, under `https://placket.tech/schema/0.1/`, from `spec/schema/` and refuses every other URL, so nothing is fetched.
- **One build, one target per rule.** `make ci` builds the checker once (target `guard`); each rule has its own target, `make <rule>`, which runs the built checker for that rule alone. Each prints `<rule>: ok`, or a line per fault starting `<rule>: <file>[:<line>]: ` and then `<rule>: refused: <why> (docs/rules/<file>.md#<rule>)`, which links the rule's heading. The checker's rule table names each rule's topic file.
- **Its cases are Go tests.** Every planted fault is a case of the checker's tests (`tools/guard/*_test.go`), with its fixture under `tools/guard/testdata/`. A case copies the repository once, lays its fixture over the copy, runs every rule in-process and asserts the lines of the rule it plants a fault for and the silence of all the others, in milliseconds rather than a run of `make ci`. The cases that plant faults in the requirement text, the schemas and the `Makefile` plant them in a copy of the live files and compute their expected lines from them. A test fails when a rule of the checker has no case that refuses. The fixtures of the cases that stay shell tests, the toolchain steps and the module loop, are in `tests/testdata/checks/`.
- **One wiring test.** `tests/test_wiring.sh` plants one fault for every rule in one copy, runs the rules' targets and `modules` once with `make -k`, and checks that each rule refuses its own fault and no other, and that every step prints its time. It also checks that a `make ci` started inside another is refused at once.
- **One test run per module.** The race detector, the fuzz seeds and coverage share one `go test -race -count=1` run per module with a coverage profile (target `tests`); `race`, `fuzz-seeds` and `coverage-reported` each read it and print their own lines.
- **Times, and a marker.** Every step prints `time: <step>: <n>s` after its output. `make ci` exports `PLACKET_IN_CI=1`, and a `make ci` that finds it set stops before any step (`ci-stays-fast`); the checker refuses a test or script that clears it. The hard limit on time is the workflow's.
- **How a rule is added.** A rule is added in a rules change (`CONTRIBUTING.md`): a function in `tools/guard/`, a row of its rule table, a target in the `Makefile` named in `ci`, its cases in the checker's tests (at least one refused), and its text in its topic file of `docs/rules/`. `machine-rules-in-ci` refuses a `Makefile` whose `ci` does not name every rule of the table, so a rule cannot be added and left out of `ci`.
- What the checks still miss is in `docs/rules/checks.md`, "Known limits of the checks".

### The rules live in topic files

- **`AGENTS.md` is the entry file**, at most 150 lines and 10 KiB. It holds the commands, the map, what is true of every change, a table of what to read before changing each part, the definition of done, and what needs a person. It holds no rule's text.
- **Every rule lives once, in `docs/rules/`**, as a heading that is its public name, in one of seven topic files: `modules.md`, `core.md`, `errors.md`, `adapters.md`, `tests.md`, `changes.md`, and `checks.md` for what the checks print and miss. Under each heading: the rule, a `Why:` line and a `Checked by:` line. `docs/rules/README.md` is the index, the one place every name appears together.
- **Two checks hold this in place**, both rules of the checker: `agents-md-short` keeps the entry file within its limits, keeps the index and the headings in step, one heading per name, and resolves every link to a rule's heading; `no-hidden-text` refuses a bidi control or an invisible character in any tracked text file outside `testdata/`, and a tracked binary there, since instruction files and code are read by agents and people who cannot see them.
- **No nested instruction files and no vendor-specific ones.** Harnesses load a nested `AGENTS.md` at different times, or not at all, so a pointer in the entry file is the one that works everywhere.

### Tests are red on base and green at head

- **Base and head, not commits.** The check `red-green-tests` reads two commits: the base B, the merge base of the branch and `main`, and the head H. It ignores the branch's history between them: merge commits, the order of commits, and which commit touched which file. It reads only the trailers of the commits from B to H.
- **The tests a change touches are found by parsing Go,** with the standard library's parser, at B and at H, never by reading diff lines. A test is a top-level `Test...` or `Fuzz...` function of a `_test.go` file, as the go tool reads them; `TestMain` is not one. Tests are matched by package directory and name. A test is *added* when H has it and B does not, *changed* when its function differs, *removed* when B has it and H does not, and *skipped* when H's function calls `Skip`, `Skipf` or `SkipNow` more often than B's, or when a build tag leaves out of the default build a file it used to be in. Two functions are compared as Go tokens, without comments and without what layout alone decides. A shell test, `tests/test_*.sh`, is added, changed or removed by its file.
- **Red on base.** In a scratch worktree of B, every test file of H is laid over B (each `_test.go` file, everything under a `testdata/` directory and everything under `tests/`), and the test files H removed are removed. Each added and changed test then runs there with `go test -count=1 -json -run '^(...)$'`, one run per package, and each touched shell test with `sh`. A test that ran and passed is *green on base*; anything else is red, printed by its kind: red (fails), red (skips), red (not run) when its package ran and it did not, and red (build) when the package does not build at B with H's tests. A test that did not run because another test of the same run panicked or exited runs again alone, so a test green on base is never counted red for a neighbour's sake.
- **Red (build) counts as red.** A test of a new function does not compile without that function. Counting it as red is what lets a test of a new API be written first without a stub; it is printed as its own kind so the reviewer sees which tests were red only by not compiling.
- **Green at head.** The same tests run the same way in a scratch worktree of H, and each must run and pass: a skip is not a pass, and neither is a test that does not show in the run's output.
- **It refuses** an added test green on base, a removed or newly skipped test, a test not green at head, and a change to non-test Go code under `core/`, `adapters/`, `cmd/`, `recipes/` or `tools/` with no test added or changed that runs. `scripts/` and `patterns/` are not in it: a pattern is its own test, run by `tests/test_patterns.sh`.
- **It reports, without refusing,** a changed test that is green on base, since a refactored test is legitimate; a test behind a build tag the default build leaves out (`not run: build tag`); and the test files the base has whose helpers or fixtures changed (`helpers changed: <files>`), whose tests it does not run again on that account.
- **Three trailers** let an honest change through, each naming what it excuses and why, so every exception is in the history in plain words:
  - `Green-On-Base: <test>: <why>` excuses an added test that already passes on `main`, such as coverage of behaviour that already works;
  - `Test-Weakened: <test>: <why>` excuses a removed test or an added skip, such as a test retired with its feature;
  - `No-Test: <why>` excuses a code change with no test change, such as a comment or a rename.

  A `<test>` is a Go test's name or a shell test's path. A trailer that names a test the change does not touch is refused, so a stale trailer cannot excuse a later test; one with no reason is refused too. The names carry no tool or project prefix. Every excused item is listed with its reason, for the reviewer to judge.
- **One tested Go program, `tools/redgreen/`,** a module of its own, standard library only, so the check can be read and its cases tested. Its tests build small git repositories under a temporary directory from the fixture trees under `tools/redgreen/testdata/`. It prints one line per item, `red-green-tests: <file>:<line>: <test>: <outcome>`, then `red-green-tests: ok, <n> red on base, <n> green at head, <n> reported` or `red-green-tests: refused: <why> (docs/rules/tests.md#red-green-tests)`. It exits 0 when ok, 1 when refused, and 3 when it could not judge: a bad argument, no `go` or `git`, a base that is not an ancestor of the head. A tool error is never a pass.
- **Not a step of `make ci`.** It needs `main` and two scratch worktrees, and it runs the tests a second time. `make red-green-tests BASE=origin/main` runs it from a checkout, for a builder before a push; the workflow runs it as its own job, beside `ci`, on every pull request, drafts included, and on every push to `main`.
- **Run from `main`'s copy.** The workflow builds `tools/redgreen` from a worktree of `origin/main`, never from the branch, so a change cannot alter the check that judges it.

What it cannot see:

- **A test red only by not building** passes as red, and a test that calls a new function but asserts nothing would pass too. The reviewer catches it, and so does mutation testing, which stays the reviewer's.
- **Helpers.** A change to a helper (a function of a `_test.go` file that is not a test) or a fixture under `testdata/` is not a changed test by itself, and the tests that use it are not run again on its account. It is reported as `helpers changed: <files>`.
- **Mixed edits.** A test weakened and changed in one edit, with no removed function and no added skip (a loosened assertion, say), shows only as changed. If it is green on base it is listed; otherwise it passes. The reviewer reads the diff.
- **Build tags.** Tests run with the default build. A test whose file the default build leaves out is listed as `not run: build tag`.
- **Uncommitted work.** It reads commits only; `make red-green-tests` judges `HEAD`, not the working tree.
- **Trailers** are read as git reads them: in the trailer block at the end of a commit message.

### Outside commits are signed off

- **A rule of its own, `outside-commits-signed-off`, in the same program and the same job.** `tools/redgreen` judges it after `red-green-tests`, on the same base and head, and prints its lines after that verdict. The job exits 1 if either rule refuses, and 3 if either could not judge.
- **The change's own commits only.** It reads the commits reachable from the head and not from the base, never the older history.
- **A bot's sign-off is refused on every run**, except on Dependabot's pull requests. A sign-off is a bot's when it holds `[bot]` anywhere: in its name, in its email or beside them. GitHub ends a bot's login in `[bot]`, and a person's login cannot hold brackets.
- **On a pull request from outside the project,** every commit that is not a merge needs a `Signed-off-by` whose email equals its author's email, ignoring case; the name is not compared. A merge commit needs none, since a branch may be updated from `main`, but a bot's sign-off on it is still refused.
- **The project is a named list of pull request openers:** the Apps `orillo-swarm[bot]` and `orillo-loom[bot]`, and the maintainer's account, `jlindenger`. Their pull requests need no sign-off. `dependabot[bot]`'s are exempt from the whole rule, since Dependabot signs off its own commits in its own bot's name. An opener is matched in full, so a person's account named `orillo-swarm` is not the project.
- **The opener is the pull request's author,** as GitHub gives it, never the run's actor and never a commit's author name, which anyone can set. The workflow passes it in the environment variable `PLACKET_PR_OPENER`, not a flag, because the program refuses a flag it does not know and is built from `main`'s copy: the workflow line and the program can then land in either order. An unset or empty value is no opener: a push to `main`, or `make red-green-tests` from a checkout. With no opener, the rule refuses only a bot's sign-off and prints nothing otherwise. With an opener, a pass prints one `ok` line.
- **The tests it runs never see the opener.** Every process the program starts (`go test`, `sh`, `git`) gets the program's environment without `PLACKET_PR_OPENER`, so a test cannot pass or fail by who opened the pull request.

What it cannot see:

- **A person's sign-off added by an agent** looks the same as one the person added. The contributor answers for it, and the reviewer reads it.
- **Content in a merge commit.** On an outside pull request, a merge commit needs no sign-off, so whatever a merge adds beyond its parents carries none. The reviewer reads a merge that does more than update from `main`.
- **Who the email is.** A sign-off matches by its email alone, and anyone can set a commit's author email. The sign-off is a statement, not proof of identity.

## Consequences

- `make ci` takes a few minutes. The checker's tests take seconds, and a planted fault still shows the exact lines a writer would see.
- The parser sees what a line scan cannot: a renamed or dot import of `time` or `fmt`, and a string that only looks like a call or an import.
- The checker is code with a dependency, pinned in `tools/guard/go.sum`. That file also lets the workflow's Go cache take effect, since it is keyed on every module's `go.sum`.
- An agent on a small task reads about 5 KiB, then only the topic file its change touches. A new rule gets its name in `docs/rules/`, a heading and an index line, in the change that adds it; `agents-md-short` refuses one without the other.
- A branch's commits can be cut any way: squashed, mixed, merged from `main`. Writing tests first stays the habit, and a branch's history may still show it, but the proof no longer depends on it.
- Branches need no prefix: the job runs on every pull request, forks and drafts included, with a read-only token, and on every push to `main`. A branch with no pull request is not checked by GitHub; a builder who wants GitHub's run early opens a draft.
- A test that only passes because of the change's code is the proof; a test that merely compiles against new code is the reviewer's to judge, from the report.
