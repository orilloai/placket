# Patterns

A pattern is a runnable template for one caller's job: a proposer, an approver, a receiver's guard, an applier. You copy its directory, change the lines it marks, and run it with your own keys, terms and window. A pattern uses only what Placket already does. It is not part of the requirement text (`spec/protocol.md`) or of the frozen formats' register (`spec/frozen-formats.md`), and it adds nothing to what a receipt claims.

`make ci` runs every pattern on the binary the tree builds, in `tests/test_patterns.sh`, so a pattern here works with the Placket beside it.

## The patterns

One line per pattern: its directory, the job it does, and the Placket version it was last checked against.

- `github-merge`: an applier merges a pull request only when placket released a merge of it that the deciding role accepted, at the head commit released, and records the delivery for an auditor. Checked against placket main@a3a1b1a.
- `guarded-export`: a receiver's guard exports a value only when exactly that value was released to it. Checked against placket main@6620793.
- `notifier`: each new question an approver may answer runs one command, with its label and id, as it opens. Checked against placket 75a6493.
- `shadow-judge`: a judge reads each part the terms ask a person about and decides nothing, beside the person's answer. Checked against placket e3972d6.

## What a pattern is made of

Each pattern is a directory `patterns/<job>/`, named for the caller's job in lower-case letters, digits and hyphens. It holds three files:

- `README.md`, with these four sections first, in this order:
  - `## The question`: the question the caller has, in the caller's words, before anything about Placket.
  - `## When to use it`: when this pattern fits, and when another one or none does.
  - `## What it relies on`: the sections of `spec/protocol.md` it relies on, by number and title.
  - `## What it claims`: the claim lines and the scope line it leans on, quoted as Placket prints them, so that nobody reads more into the pattern than Placket claims.
- `pattern.sh`, the template. It is a POSIX `sh` script that uses Placket's command line and `jq`, beside the shell's own utilities, and nothing else but the one exception below. Every line a caller changes for their own use carries the comment `# CHANGE`, with what to put there. As it is, it runs from start to end on invented data: it makes its own keys, terms and founding set in the directory it runs in, starts its own window on a socket under `$TMPDIR`, does its job, and stops its window before it exits, on failure too.
- `expected.txt`, exactly what a run of `pattern.sh` prints, standard output and standard error together. A run prints the same each time, so a pattern prints no key, hash, salt or time: it picks what it shows with `jq`.

A pattern may also hold:

- data files of its own, such as `shadow-judge/judge.json`, which its `pattern.sh` reads from its own directory;
- `apply.sh`, the applier alone, for a pattern whose job is to act on what Placket released. It takes what it serves as arguments, sets nothing up, reads the caller's environment, and marks the lines a caller changes `# CHANGE`, as `pattern.sh` does. `pattern.sh` sets up the invented data and calls it, so a person's own copy needs only `apply.sh`;
- a stand-in for a tool outside Placket that the pattern calls, with the fixtures it answers from. `pattern.sh` puts the stand-in first on `PATH` on a line marked `# CHANGE`, so `make ci` runs the pattern against the stand-in and a copy used for real calls the real tool.

A pattern depends on nothing else: only the `placket` binary and `jq` on `PATH`, and its own files, never a file elsewhere in this repository. So `patterns/` can move to a repository of its own as it is. There is one exception: `github-merge` also calls `gh api`, GitHub's command line, since merging on GitHub is its job.

## How make ci runs them

`tests/test_patterns.sh` finds every `patterns/*/pattern.sh`. A new pattern needs no change to `make ci`. For each one it:

- checks the shape above: the three files, the four sections, at least one `# CHANGE` line in `pattern.sh`, and in `apply.sh` where there is one, and one line in the list above for each pattern and none for a pattern that is not here;
- copies the pattern's directory to a fresh directory and runs `sh pattern.sh` there, with the `placket` it built from the tree first on `PATH`, a fresh `TMPDIR` of the pattern's own, and no `PLACKET_JEV_KEY`, so no pattern reaches a judge's host;
- passes the pattern when it exits 0 within 60 seconds, prints exactly its `expected.txt`, and leaves no process running.

Each run has a process group of its own, which the test kills whatever happens, so a pattern that fails or hangs leaves no window behind. With no patterns, the test passes.

## Adding or changing one

A line in the list reads:

```
- `<job>`: <the job it does>. Checked against placket <version>.
```

`<version>` names the Placket the pattern last ran green on: a release tag, or, until Placket has one, `main` at a short commit hash, such as `main@b997be6`. A pull request that changes a command a pattern uses keeps that pattern green in the same pull request, and updates its version. A pattern never scripts around a gap in Placket: a gap is an issue to file (`CONTRIBUTING.md`, Patterns).
