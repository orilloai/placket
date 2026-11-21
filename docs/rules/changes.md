# How changes are made

What a change holds, when it stops for the maintainer, and the checks that keep the rest in place. `CONTRIBUTING.md` has the steps of a change in full.

## `change-is-one-capability`

A protocol change is one capability, and its design lists every exported name it adds, by package and signature, so a builder never invents an entry point.

Why: a reviewer can hold one capability in mind; a builder should not have to design the API.

Checked by: the reviewer.

## `deltas-stop-for-intent`

A change to the requirement text that alters observable behaviour, a capability's scope or what it must not do stops for the maintainer. Anything else is the builder's choice, listed in the design and the pull request where the maintainer can veto it.

Why: the maintainer rules intent; builders keep building.

Checked by: the reviewer.

## `decisions-recorded-first`

Any decision spanning packages, or expensive to reverse, gets its record in `docs/adr/` ahead of the change built on it.

Why: the reason is lost once the code is written around it.

Checked by: the reviewer.

## `format-change-noted`

A change to a frozen format (a new type URI, reason or code, or a member emitted that was absent) carries a version note in the register and the vectors' README in the same change. A door format is not frozen and takes no note.

Why: a verifier elsewhere must learn of a format change from the text, not from a failure.

Checked by: the reviewer.

## `api-compat`

An exported name, a CLI flag or a door format changes only with a decision record. Deprecate before removing, and keep the default internal.

Why: callers, scripts and other implementations build on these, and a silent change breaks them.

Checked by: the reviewer.

## `machine-rules-in-ci`

Every rule a machine can check is checked by `make ci`, the one entry point, and GitHub runs it on every pull request and every push to `main`. The two exceptions are `red-green-tests` and `outside-commits-signed-off`, which compare a change with `main` and so run in a job of their own beside `ci`. A rule no machine can check stays on the reviewer's list.

Why: a rule that only a reviewer enforces is enforced only when the reviewer remembers.

Checked by: `make ci`, step `machine-rules-in-ci`, checks that every tool it fetches with `go run` or `go install` is pinned to an exact version, that every action a workflow uses is pinned to a commit, and that its `ci` target names every rule of the checker and every step. The tools the shell tests take from the system, such as `jq`, `perl` and `openssl`, are not pinned; the reviewer reads the rest.

## `ci-stays-fast`

`make ci` runs each step once, and every step prints its time. `make ci` exports a marker, `PLACKET_IN_CI=1`, to every step, and a `make ci` started where the marker is already set stops before any step, whatever `-k` says, printing `ci-stays-fast: refused: make ci inside make ci`. No script or test clears the marker: no empty assignment, `unset`, `env -u PLACKET_IN_CI`, `env -i`, `os.Unsetenv`, `os.Clearenv`, or Go `Env` list that does not start from `os.Environ()`. Setting it to a value, or reading it, is fine.

Why: a test that runs the whole of `make ci` again multiplies its time, and a writer pays for `make ci` twice a round.

Checked by: `make ci`, step `ci-stays-fast`: the `Makefile` refuses the nested run, and the checker refuses a `.sh` or `_test.go` file outside `testdata/` that clears the marker. The workflow's timeout is the hard limit.

## `no-hidden-text`

No tracked text file outside `testdata/` holds a bidi control character or invisible Unicode, and no tracked file there is a symbolic link or holds a NUL byte, so a tracked binary file outside `testdata/` is refused.

Why: such a character makes what a reviewer reads differ from what the compiler, the shell or an agent reads, and a link can lead a reader to a file the check does not read.

Checked by: `make ci`, step `no-hidden-text`. It reads only the files git tracks, so a binary built in a checkout, or an editor's scratch file, passes a local run as it passes a clean checkout.

## `outside-commits-signed-off`

No commit of a change carries a `Signed-off-by` in a bot's name: one that holds `[bot]` anywhere, since GitHub ends a bot's login in `[bot]` and a person's login cannot hold brackets. On a pull request opened from outside the project, every commit of the change that is not a merge carries a `Signed-off-by` whose email is its author's. The project is the pull requests opened by `orillo-swarm[bot]`, `orillo-loom[bot]` and `jlindenger`; they need no sign-off. Pull requests opened by `dependabot[bot]` are exempt from the whole rule. Only the change's own commits are read, from the merge base with `main` to the head.

Why: a sign-off is a person's statement under the Developer Certificate of Origin, so an outside contributor's commits carry their own, and a bot's certifies nothing.

Checked by: the workflow's `red-green-tests` job, with `main`'s copy of `tools/redgreen/`, after the `red-green-tests` verdict. The opener is the pull request's author, passed in `PLACKET_PR_OPENER`; with none, on a push to `main` or from `make red-green-tests`, only a bot's sign-off is refused. A person's sign-off added for them by an agent, and content a merge commit adds, are the reviewer's; `docs/adr/0013-red-green-tests.md` records why.

## `docs-tree-holds`

`docs/README.md` is the docs tree, the page order placket.tech reads: the sections Quickstarts, Concepts, Guides, Reference, Verify and Security, in that order, each a list of `- [<title>](<path>)` entries. A tree link is relative, names a file that exists, and leaves `docs/` only for `spec/` and `vectors/`; an `#anchor` on it is a heading of that file. Every Markdown page under `docs/concepts/`, `docs/quickstarts/`, `docs/guides/`, `docs/reference/` and `docs/security/`, and `docs/verify.md`, is listed. On every page listed, every relative link names a file or directory that exists, and an `#anchor` on a Markdown file is one of its headings. Every line of a fenced block on a listed page that starts `placket ` names a command, its flags and its arguments' values as the generated CLI reference, `docs/reference/cli.md`, holds them. The contributor pages, `docs/adr/`, `docs/rules/`, `docs/style/`, `docs/release.md` and `docs/review.md`, are not asked for.

Why: the site renders the docs from these files and invents no text, so a page the tree promises must exist, its links must land, and a command line on it must be one the binary takes.

Checked by: `make ci`, step `docs-tree-holds`. It reads each link and anchor as GitHub resolves it, and checks a command line against the CLI reference without running it: a word the shell fills in, such as `"$d"` or `$(...)`, is not checked. The reviewer reads each page against the code and `docs/style/writing.md`.

## `agents-md-short`

`AGENTS.md` stays within its limits, 150 lines and 10 KiB, and every public name has exactly one heading.

Why: every agent loads the entry file on every task, so its length is paid each time, and a rule with two homes drifts.

Checked by: `make ci`, step `agents-md-short`. It also checks that the index in `docs/rules/README.md` and the headings agree, that every rule the checker and `red-green-tests` print has its heading, and that every link to a rule's heading resolves.
