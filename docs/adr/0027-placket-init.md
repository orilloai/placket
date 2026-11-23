# 0027. `placket init` founds an example Accord, and a test runs each first hour

Status: accepted. Date: 2026-11-19. Amends records 0001 and 0009.

## Context

Founding the simplest Accord by hand takes the keys of a mediator, its operator, a principal and each party, a terms file, a declaration, a seal and a binding per party, a policy, a mediator file, a card and the founding set, then a window and each party's view and acceptance: about twenty commands and five files written by hand. A person who came to try one of placket's three uses, approve before apply, an out-gate or a handoff between agents, has to learn all of that before the first receipt.

In the single-person case one person holds every key. Every file the founding commands write can then be written by one command, and every member's acceptance signed by it, as each member would sign its own: the person reads each view before they use the Accord, as they would with `placket view`.

## Decision

### The command

`placket init <example> [--dir D] [--label L]` founds one example Accord, or two, for one person, in one command. `<example>` is one of:

- `approve`: approve before apply. The parties are `agent`, `person` and `applier`, holding the roles `agent`, `decider` and `applier`. The agent's `proposal` to the applier is asked of `decider`, the person accepts, changes or declines it, and the applier is handed what was released.
- `merge`: an out-gate on a merge. The same parties and roles; the agent's `merge`, data naming a pull request, is asked of `decider`, and the applier merges what was released.
- `handoff`: handoffs between agents across two mediators. The Accord `<label>-plan` has the parties `planner` and `coder`, and the planner's `plan` passes to the coder by the terms; the Accord `<label>-review` has `coder` and `reviewer`, and the coder's `work` passes to the reviewer. Nobody is asked.

The label is the example's name unless `--label` gives one. The person's key is a party key holding `decider`; the principal's key only grants roles.

### What it writes

Everything goes in D, by default `<placket config dir>/<label>/`, made mode 0700. A D that holds anything is refused with `dir-not-empty`, before anything is written. In D:

- the keys `mediator.key`, `operator.key` and `principal.key`, and one key per party and Accord, `<entry>.key`, each mode 0600 (`keys-on-disk`). In `handoff` the coder has one key in each Accord;
- each party's opening, `<actor hex>.opening.json`, so D is the window's `--openings`;
- for each Accord a directory of its label: each party's terms file, `<party>.terms`, the policy, the mediator file, the card, `founding.json` and `acceptances.json`. The window writes its journal there, in `journal/`;
- for each party, its view, `view.json`, and its acceptance, `acceptance.json`, in its state directory, `<entry>.key.state/<the Accord id's hex>/`, as `placket view` and `placket accept` keep them.

It seals each party's declaration under a salt it mints, founds the set with every opening, builds each member's view and signs that member's acceptance with its key, and founds again with the acceptances, so the window starts founded.

It adds one entry per party key to the labels config, named `<accord label>-<party>`, such as `approve-agent`, each with the Accord's default socket written out. A label taken already in the config is `label-taken`, and a label whose longest entry name would be over 63 characters is `usage`, as is the label `applier` for `approve` and `merge`, whose Accord's directory would be `D/applier/`, where the applier is fetched; each before anything is written. A failure after D is made removes what it wrote.

### What it prints

For each Accord, its id, each party's label, key reference and role, and the window's command line in full, with `--socket` and `--journal`. Then the `claude mcp add` line that serves the agents' labels, and, for `approve` and `merge`, the `placket watch` line of the person's label. For `approve` and `merge`, the line that fetches the example's applier from the repository into `D/applier/`, and the line that runs it. The applier is fetched at the binary's version tag, pinned to the tag's commit when the build carries it (Go stamps it as `vcs.revision`), since a tag can be moved and a commit cannot; a build with no version tag fetches it from `main`, and says so. Nothing checks the file fetched against a checksum: the guides say to read it before running it. The binary holds no pattern, so the applier a person runs is the one they can read in the repository.

`init` does not start the window. The person starts it with the printed line, and keeps it running.

### The window's default socket

A window given `--socket` that is its Accord's default socket makes the socket's directory as a window given no `--socket` does. The labels config names the socket of each entry `init` writes, and the runtime directory that holds it may be gone after a restart of the system.

### The example appliers

`patterns/local-apply/` is `approve`'s applier: `apply.sh LABEL FILE` writes the text of each release a `decider` key accepted or changed to FILE. `merge` uses `patterns/github-merge/`. Both are patterns, outside the requirement text, run by `make ci`.

### The first hour

`tests/test_first_hour.sh` runs each example from a clean `HOME`, as a person would: it builds the host's archive as `.goreleaser.yaml` builds it, from the tree, unpacks it and puts its `placket` first on `PATH`, runs `placket init`, starts each window with its printed line, gives through `placket mcp` as Claude Code would, answers with the line `placket watch` prints, runs the applier as the fetch line places it, and runs `placket verify` with every opening on each journal. An entry passes when verify prints its `valid` and `conforms` lines, within 60 seconds. The three run side by side.

`tests/firsthour/` is a package of the root module, a `main` the test builds: a scripted MCP client that starts the command the `claude mcp add` line names, speaks to it over standard input and output, and makes the tool calls a script lists. It is not Claude Code and runs no model.

### Not in this

No protocol change: `init` writes the files the founding commands write, and signs each acceptance as its member would. No format, requirement or vector changes.

## Consequences

- A person reaches a founded Accord in one command, and a verified receipt from the lines it prints.
- `init` holds every key in one directory, so it is for one person trying placket. An Accord between parties who do not share a machine is founded with the founding commands, each party sealing its own terms and accepting its own view.
- The agent learns the receiver's key reference from what `init` prints. The MCP tools do not list an Accord's members.
- The first-hour test builds the binary without the race detector, as a release does, and runs in `make ci`'s `scripts` step.
