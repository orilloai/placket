# Contributing to placket

Thank you for helping. This file says how a change to placket is made and reviewed, for everyone: people, and people working with tools and agents. The rules a change keeps are in `docs/rules/`, each under a public name; this file is the process around them and links to them rather than repeating them.

Everyone here follows the [code of conduct](CODE_OF_CONDUCT.md). Report a security problem as [`SECURITY.md`](SECURITY.md) says, never in a public issue or pull request.

## Kinds of change

There are three, and they never mix in one pull request:

1. **A protocol or code change**: the requirement text, formats, vectors, Go code, docs.
2. **A rules change**: `AGENTS.md`, this file, `docs/rules/`, `docs/review.md`, `docs/security/secure-coding.md`, `docs/style/`, `review/`, or a check: the `Makefile`'s checks and steps, the checker in `tools/guard/` and the check `red-green-tests` in `tools/redgreen/`, with their tests.
3. **A workflow change**: anything under `.github/workflows/`. These are the maintainer's alone.

A rules or workflow change is its own pull request, flagged as such. A change cannot alter the checks that judge it.

## Talk first about a large change

Open an issue before you start on any of these, using the **proposal** form, and wait for an answer:

- a change to the requirement text (`spec/protocol.md`) or a frozen format (`spec/`);
- a change to an exported API, a command-line flag or a door format;
- a new dependency, anywhere;
- anything that would change what placket does for someone, or what it must not do.

The maintainer rules on intent. Anything else is your choice: take the sensible default and say so in the pull request, where it can be questioned. For a bug, use the **bug** form; for something wrong or unclear in the requirement text, the **erratum** form. A small fix needs no issue first.

## Making a change

1. **Read first.** `AGENTS.md` maps the repository and says which file of `docs/rules/` to read before you touch each part. Read the decision records in `docs/adr/` that touch your change. If your change makes a decision that spans packages or would be expensive to reverse, write its record in `docs/adr/` first (`NNNN-<slug>.md`, the next free number; context, decision, consequences).
2. **Behaviour in the text first.** A change in behaviour starts in the requirement text (`spec/protocol.md`) and the vectors (`vectors/`); the code follows. A frozen-format change carries its version note in the same change ([`format-change-noted`](docs/rules/changes.md#format-change-noted)).
3. **Tests first.** Write the tests before the code. Every test your change adds or changes must fail against `main` and pass, not skip, at the head of your branch. The check `red-green-tests` proves it ([`red-green-tests`](docs/rules/tests.md#red-green-tests), `docs/adr/0013-red-green-tests.md`). It reads only the merge base with `main` and your head, so squash, merge `main` in, or mix tests and code in a commit as you like. A test that does not build without your code counts as red. Run it yourself, after committing, with `make red-green-tests BASE=origin/main`.

   Never weaken, skip or delete a test to get a change through. When a change honestly breaks the rule, a trailer on any of its commits says so and why, and the reviewer judges the reason:

   - `Green-On-Base: <test>: <why>`: an added test that already passes on `main`, such as coverage of behaviour that already works.
   - `Test-Weakened: <test>: <why>`: a test removed, or a skip added, such as a test retired with its feature.
   - `No-Test: <why>`: a change to Go code under `core/`, `adapters/`, `cmd/`, `recipes/` or `tools/` with no test added or changed, such as a comment or a rename.

   A `<test>` is a Go test's name, or a shell test's path such as `tests/test_x.sh`. Add one with `git commit --trailer 'Green-On-Base: TestX: <why>'`, plus `-s` if you are a person signing off (an agent never adds one). A trailer naming a test the change does not touch is refused.
4. **Then the code**, in small commits.
5. **Run `make ci`** before you push, with Go 1.27, `jq`, a POSIX shell, OpenSSL 3, GNU `basenc` (from coreutils), `perl`, `git` and `tar`. It runs every check: the rules of the checker in `tools/guard/`, every module's format, vet, lint, the vulnerability and secret scans, the tests with the race detector, the fuzz seeds, the conformance runner and the shell tests. `make <step>` runs one of them. A refusal starts with the rule's name, names the file and line, and its last line links the rule's heading in `docs/rules/`; `docs/rules/checks.md` says what each step prints. Never run `make ci` from inside a test ([`ci-stays-fast`](docs/rules/changes.md#ci-stays-fast)).

## Patterns

A pattern is a runnable template for one caller's job, under `patterns/<job>/`; `patterns/README.md` states what one is made of. `make ci` runs every pattern (`tests/test_patterns.sh`), and a new one needs no change to it.

- A pull request that changes a command a pattern uses keeps that pattern green in the same pull request: its `pattern.sh` and `expected.txt` change with the command.
- A pattern never scripts around a gap in placket. When a job needs something placket does not do, that gap is an issue to file, not a workaround in a pattern.
- A pattern depends only on the `placket` binary and `jq` on `PATH`, never on another file of this repository, so `patterns/` can move to a repository of its own as it is. The one exception is `gh api`, for `github-merge` alone. Beside its `pattern.sh`, a pattern may hold `apply.sh`, the applier alone, which its `pattern.sh` calls, and a stand-in for a tool outside placket with its fixtures (`patterns/README.md`).

## Commits

- **Sign off your commits yourself**, with `git commit -s`. The `Signed-off-by` line is your statement, as a person, that you may contribute the change under the project's licence, as the [Developer Certificate of Origin](https://developercertificate.org/) says.
- A commit message says what changed and why, in placket's own words (`docs/style/writing.md`).
- Placket's code and its requirement text are under the Apache License 2.0 ([`LICENSE`](LICENSE)); your contribution is too.

## Pull requests

- Open it as a draft while the change is in progress, and mark it ready for review when it is.
- Fill in the pull request template: a short summary, then **Why**, **Important points**, **Risk**, **How it was checked** and **Reading map**, and the two checkboxes.
- A user-visible change gets a line in `CHANGELOG.md`. If you leave it out, the reviewer adds it.

## How review works

- A reviewer who did not write the change reads it against the rules, by name. `docs/review.md` says how a change is judged, what a finding holds and what blocks.
- The maintainer, as code owner of every path (`.github/CODEOWNERS`), approves every change. A change merges once that approval is in, given after the last push, and the checks are green.

## How we work with tools and agents

- **Welcome, however it is made.** Use any tools you like, agents included, and an agent may open the pull request. Every contribution needs a person in the loop who answers for it.
- **You answer for every line**, and can explain it. We do not ask which tools you used.
- **Signing off.** Sign off your commits yourself, with `git commit -s`, under the Developer Certificate of Origin. The sign-off is your own statement as a person, so an agent never adds one, whether its own or yours.
- **When nobody answers.** A change that nobody answers for, or whose author cannot explain it, gets one reply asking for that. If nothing changes, it is closed.
- **This project holds itself to the same rules.** Agents build most of placket. Its own commits carry no sign-off, because Orillo owns that work and the maintainer answers for it by approving each change.
- **Your agent** should read `AGENTS.md`, the same file this project's own agents read.
