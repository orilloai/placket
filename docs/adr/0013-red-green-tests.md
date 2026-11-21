# 0013. Tests are shown red against main and green at the head, however the branch was built

Status: accepted. Date: 2026-11-16. Amended 2026-11-16: the job runs on pull requests and on pushes to `main`, not on every push. Amended 2026-11-21: the same program checks a second rule, `outside-commits-signed-off`, on the change's own commits (below).

## Context

Tests come first in placket: a test is written before the code it proves, so it fails without that code and passes with it. Until now the `lock` job of the workflow checked this by the shape of a branch's commits. A branch had to carry a commit marked by a lock trailer that touched only tests, every test that commit added had to fail at it and pass at the head, no other commit could touch a test, and the branch could have no merge commit.

That shape is a habit of one way of working, not the property itself, and it refused real work for reasons that had nothing to do with tests coming first. The last four refusals on placket were a branch with no lock commit, two branches updated from `main` with a merge, and a branch whose follow-up commit fixed three test files. All four merged over the refusal. It would refuse most outside contributors too, since squashed commits, merges from `main` and commits that mix tests and code are normal everywhere else. And it ran only on pushes to `wip/**`, so it never reported on a pull request.

The property wanted is about the change as a whole: every test the change adds or changes fails without the change's code and passes with it, and no test is quietly removed or skipped.

## Decision

- **Base and head, not commits.** The check `red-green-tests` reads two commits: the base B, the merge base of the branch and `main`, and the head H. It ignores the branch's history between them: merge commits, the order of commits, and which commit touched which file. It reads only the trailers of the commits from B to H.
- **The tests a change touches are found by parsing Go,** with the standard library's parser, at B and at H, never by reading diff lines. A test is a top-level `Test...` or `Fuzz...` function of a `_test.go` file, as the go tool reads them; `TestMain` is not one. Tests are matched by package directory and name. A test is *added* when H has it and B does not, *changed* when its function differs, *removed* when B has it and H does not, and *skipped* when H's function calls `Skip`, `Skipf` or `SkipNow` more often than B's, or when a build tag leaves out of the default build a file it used to be in. Two functions are compared as Go tokens, without comments and without what layout alone decides, so a change that gofmt or a comment alone makes is not a change. A shell test, `tests/test_*.sh`, is added, changed or removed by its file.
- **Red on base.** In a scratch worktree of B, every test file of H is laid over B (each `_test.go` file, everything under a `testdata/` directory and everything under `tests/`), and the test files H removed are removed. Each added and changed test then runs there with `go test -count=1 -json -run '^(...)$'`, one run per package, and each touched shell test with `sh`. A test that ran and passed is *green on base*; anything else is red, printed by its kind: red (fails), red (skips), red (not run) when its package ran and it did not, and red (build) when the package does not build at B with H's tests. A test that did not run because another test of the same run panicked or exited runs again alone, so a test green on base is never counted red for a neighbour's sake.
- **Red (build) counts as red.** A test of a new function does not compile without that function. Counting it as red is what lets a test of a new API be written first without a stub; it is printed as its own kind so the reviewer sees which tests were red only by not compiling.
- **Green at head.** The same tests run the same way in a scratch worktree of H, and each must run and pass: a skip is not a pass, and neither is a test that does not show in the run's output.
- **It refuses** an added test green on base, a removed or newly skipped test, a test not green at head, and a change to non-test Go code under `core/`, `adapters/`, `cmd/`, `recipes/` or `tools/` with no test added or changed that runs. That last one keeps the teeth of the old "a lock commit is required" without its shape. `scripts/` and `patterns/` are not in it: a pattern is its own test, run by `tests/test_patterns.sh`.
- **It reports, without refusing,** a changed test that is green on base, since a refactored test is legitimate; a test behind a build tag the default build leaves out (`not run: build tag`); and the test files the base has whose helpers or fixtures changed (`helpers changed: <files>`), whose tests it does not run again on that account.
- **Three trailers** let an honest change through without the maintainer's override, each naming what it excuses and why, so every exception is in the history in plain words:
  - `Green-On-Base: <test>: <why>` excuses an added test that already passes on `main`, such as coverage of behaviour that already works;
  - `Test-Weakened: <test>: <why>` excuses a removed test or an added skip, such as a test retired with its feature;
  - `No-Test: <why>` excuses a code change with no test change, such as a comment or a rename.

  A `<test>` is a Go test's name or a shell test's path. A trailer that names a test the change does not touch is refused, so a stale trailer cannot excuse a later test; one with no reason is refused too. The names carry no tool or project prefix: the swarm and outside contributors use the same three. Every excused item is listed with its reason, for the reviewer to judge.
- **One tested Go program, `tools/redgreen/`,** a module of its own like `tools/guard/`, standard library only, so the check can be read and its cases tested. Its tests build small git repositories under a temporary directory from the fixture trees under `tools/redgreen/testdata/`. It prints one line per item, `red-green-tests: <file>:<line>: <test>: <outcome>`, then `red-green-tests: ok, <n> red on base, <n> green at head, <n> reported` or `red-green-tests: refused: <why> (AGENTS.md, red-green-tests)`. It exits 0 when ok, 1 when refused, and 3 when it could not judge: a bad argument, no `go` or `git`, a base that is not an ancestor of the head. A tool error is never a pass.
- **Not a step of `make ci`.** It needs `main` and two scratch worktrees, and it runs the tests a second time. `make red-green-tests BASE=origin/main` runs it from a checkout, for a builder before a push; the workflow runs it as its own job, beside `ci`, on every pull request, drafts included, and on every push to `main`.
- **Run from `main`'s copy.** The workflow builds `tools/redgreen` from a worktree of `origin/main`, never from the branch, so a change cannot alter the check that judges it. That was the reason the old check was written inline in the workflow file, where the writer could not change it; building from `main` meets the same reason with a program that can be tested. When `main` has no `tools/redgreen` yet, the job says so and passes, which can happen only between this change and the workflow change that follows it.
- **This supersedes one sentence of 0007**, that a check and its tests change "under the same lock commits as any other test". A check and its tests now change like any other code and test, judged by `red-green-tests`; a rules change still stands alone and only the maintainer merges it. 0007 is not edited.

## What it still cannot see

- **A test red only by not building** passes as red, and a test that calls a new function but asserts nothing would pass too. The reviewer catches it, and so does mutation testing, which stays the reviewer's.
- **Helpers.** A change to a helper (a function of a `_test.go` file that is not a test) or a fixture under `testdata/` is not a changed test by itself, and the tests that use it are not run again on its account. It is reported as `helpers changed: <files>`.
- **Mixed edits.** A test weakened and changed in one edit, with no removed function and no added skip (a loosened assertion, say), shows only as changed. If it is green on base it is listed; otherwise it passes. The reviewer reads the diff.
- **Build tags.** Tests run with the default build. A test whose file the default build leaves out is listed as `not run: build tag`.
- **Uncommitted work.** It reads commits only; `make red-green-tests` judges `HEAD`, not the working tree.
- **Trailers** are read as git reads them: in the trailer block at the end of a commit message.

## Consequences

- A branch's commits can be cut any way: squashed, mixed, merged from `main`. The lock trailer and the rule against merge commits are retired. A branch that still carries a lock commit passes like any other.
- Branches need no prefix: the job runs on every pull request, forks and drafts included, with a read-only token, and on every push to `main`. A branch with no pull request is not checked by GitHub; a builder who wants GitHub's run early opens a draft. One push to a branch with a pull request runs each job once, not once for the push and again for the pull request.
- Writing tests first stays the habit, and a branch's history may still show it, but the proof no longer depends on it.
- A test that only passes because of the change's code is the proof; a test that merely compiles against new code is the reviewer's to judge, from the report.

## Amendment of 2026-11-21: outside commits are signed off

### Context

Anyone may open a pull request once the repository is public. Each commit an outside contributor sends must carry their own `Signed-off-by`, their statement under the Developer Certificate of Origin, and nothing checked it. The project's own commits carry no sign-off, and an agent never adds one, yet an agent did add one in a bot's name, and a review round was spent finding it. A machine can see a sign-off in a bot's name. It cannot see an agent adding a person's sign-off for them: that stays the contributor's duty and the reviewer's eye.

### Decision

- **A rule of its own, `outside-commits-signed-off`, in the same program and the same job.** `tools/redgreen` judges it after `red-green-tests`, on the same base and head, and prints its lines after that verdict. The job exits 1 if either rule refuses, and 3 if either could not judge.
- **The change's own commits only.** It reads the commits reachable from the head and not from the base, never the older history, so the bot sign-offs already on `main` are not judged again.
- **A bot's sign-off is refused on every run**, except on Dependabot's pull requests. A sign-off is a bot's when its name, or the local part of its email, ends in `[bot]`.
- **On a pull request from outside the project,** every commit that is not a merge needs a `Signed-off-by` whose email equals its author's email, ignoring case; the name is not compared. A merge commit needs none, since a branch may be updated from `main`, but a bot's sign-off on it is still refused.
- **The project is a named list of pull request openers:** the Apps `orillo-swarm[bot]` and `orillo-loom[bot]`, and the maintainer's account, `jlindenger`. Their pull requests need no sign-off. `dependabot[bot]`'s are exempt from the whole rule, since Dependabot signs off its own commits in its own bot's name. An opener is matched in full, so a person's account named `orillo-swarm` is not the project.
- **The opener is the pull request's author,** as GitHub gives it, never the run's actor and never a commit's author name, which anyone can set. The workflow passes it in the environment variable `PLACKET_PR_OPENER`, not a flag, because the program refuses a flag it does not know and is built from `main`'s copy: the workflow line and the program can then land in either order. An unset or empty value is no opener: a push to `main`, or `make red-green-tests` from a checkout. With no opener, the rule refuses only a bot's sign-off and prints nothing otherwise. With an opener, a pass prints one `ok` line.
- **The tests it runs never see the opener.** Every process the program starts (`go test`, `sh`, `git`) gets the program's environment without `PLACKET_PR_OPENER`, so a test cannot pass or fail by who opened the pull request.

### What it still cannot see

- **A person's sign-off added by an agent** looks the same as one the person added. The contributor answers for it, and the reviewer reads it.
- **Content in a merge commit.** On an outside pull request, a merge commit needs no sign-off, so whatever a merge adds beyond its parents carries none. The reviewer reads a merge that does more than update from `main`.
- **Who the email is.** A sign-off matches by its email alone, and anyone can set a commit's author email. The sign-off is a statement, not proof of identity.
