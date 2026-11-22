---
name: placket-setup
description: Install placket and set it up with the person, either from one of placket's patterns or as an Accord of their own, then print the line that lets their agent reach it through placket mcp. Use when someone wants to try placket, gate an agent's action behind a person's yes, or set up a placket pattern.
---

# Set up placket

Placket mediates the parts that cross between parties under terms each party signs, asks a person where the terms say to, and signs a receipt for every decision. This skill installs it and sets up one Accord with the person. Every step that installs, writes or signs waits for the person's yes, given after you show them what it will do.

You never answer a question placket asks, and never accept a view for the person without asking them first. You send nothing of the person's files anywhere.

## 1. Is placket installed?

Run `placket version`. Its first line is `version <version>`; keep `<version>` for step 2.

If there is no `placket` on `PATH`, offer the install and run it only with the person's yes. The one line:

```sh
curl -fsSL https://github.com/orilloai/placket/releases/latest/download/install.sh | sh
```

It downloads the signed release for this machine, checks the signature on `checksums.txt` with `cosign`, the archive against its line there, and the provenance with `gh` when `gh` is installed and signed in, says which checks it ran and which it skipped, checks that the binary is the version it asked for, and puts `placket` in `~/.local/bin` without `sudo`. To pin a release, add `-s -- --version vX.Y.Z` after `sh`. A person who wants to read the script first, or to check it against the release's signed `checksums.txt` before it runs, follows the README's Install section. With Homebrew, on macOS or Linux, instead:

```sh
brew install --cask orilloai/tap/placket
```

When the install ends `install: refused: cosign: ...`, there is no `cosign` to check the signature, and it installed nothing. Tell the person so, and offer either to install `cosign` (for example `brew install cosign`, or the package or release its own project gives for their system) and run the install line again, or to install with Homebrew instead. Do not go past the refusal yourself: run the line with `PLACKET_INSTALL_UNSIGNED=1`, as `curl -fsSL https://github.com/orilloai/placket/releases/latest/download/install.sh | PLACKET_INSTALL_UNSIGNED=1 sh`, only when the person says to install without the signature check, and then show them the lines that say what it did not check. Any other `install: refused:` line means a check ran and failed: show it to the person, and do not try another way around it.

When the install says its directory is not on `PATH`, say so, and use the full path, `~/.local/bin/placket`, for the rest of this skill.

The skill follows placket's `main`. When a command named here prints `error usage` and the binary's usage line does not list that command, the installed placket is older than this skill: say so, and offer to upgrade with the same install line, or `brew upgrade --cask placket`.

## 2. Choose a path

Ask the person which they want:

- **A pattern**: one of the runnable templates in placket's repository, such as an applier that writes a file only with the text a person accepted, or merges a pull request only after a person accepted it. Go to 3.
- **An Accord of their own**: terms written for what their agent does. Go to 4.

If they came from `placket-scout`, take the place it suggested and the path it named.

## 3. A pattern

`fetch-pattern.sh`, in this skill's own directory, takes a pattern from the repository at the tag of the person's `placket version`, so the pattern matches the binary that runs it. A build with no release tag takes `main`, and the script says so; tell the person that `main` can change at any time.

List the patterns of that release:

```sh
sh <this skill's directory>/fetch-pattern.sh --list <version>
```

Show the list as it prints it, and ask which one. Then, with the person's yes to the directory, copy it:

```sh
sh <this skill's directory>/fetch-pattern.sh <version> <pattern> <dir>
```

`<dir>` must not exist, or be empty. The script prints the commit it took. Then read `<dir>/README.md` with the person and follow it: `## The question` and `## When to use it` say whether it fits; `## What it claims` says what its receipts prove, quoted as placket prints it. `sh pattern.sh` runs the whole pattern on invented data in the directory it runs in, with its own keys and window. The lines marked `# CHANGE` are what the person changes for real use; change none without their yes.

Two of the patterns are also placket's examples, which `placket init` founds for the person in one command: `placket init approve` takes its applier from `patterns/local-apply/`, and `placket init merge` from `patterns/github-merge/`. Offer that when the person picks one of them. A third example, `placket init handoff`, hands work between agents across two windows and has no pattern; offer it when the person wants agents to hand work to each other. Each `placket init` writes into a new directory under placket's config directory, adds one labels config entry per party, `<label>-<party>`, and prints every command that comes next, the `claude mcp add` line included. Run it only with their yes.

Then go to 5.

## 4. An Accord of their own

### Ask

Ask, in plain words, and wait for the answers:

1. What does the agent do, and what does it hand on: a file's text, a merge, a message, a release?
2. Who or what receives it: the program that applies it (the applier), another agent, a service?
3. Which of those actions need a person's yes before they happen? Which may pass with no question, and which never?
4. Who answers: the person themselves, or someone else who holds a key of their own?

### Draft the terms

Write, for the person to read, one sentence per term in placket's own words, and then the terms as placket reads them. Each party has its own terms. Each term has a stance, `need`, `want`, `maybe` or `no`; a direction, `give` or `get`; the role it is with; and the kinds of part it selects. A `maybe` term asks the roles its `resolve.to` names; a question nobody answers closes as `timeout` at its deadline, `timeout` seconds after it was asked. For the common case, an agent whose `proposal` to an applier a person decides:

- The agent's terms: it gives `proposal` to the applier, and that part is asked of the role `decider`; it gets the decider's `answer`, `changed-version` and `comment`.
- The applier's terms: it gets `proposal` from the agent, and `changed-version` from the decider.
- The decider's terms: it gives `answer`, `changed-version` and `comment` to the agent, and `changed-version` to the applier.

As files, in placket's format (`spec/schema/term.json` in the repository holds the whole of it):

```json
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","changed-version","comment"]}}]}
```

```json
{"terms":[
 {"stance":"want","dir":"get","with":["role:agent"],"select":{"kind":["proposal"]}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["changed-version"]}}]}
```

```json
{"terms":[
 {"stance":"want","dir":"give","with":["role:agent"],"select":{"kind":["answer","changed-version","comment"]}},
 {"stance":"want","dir":"give","with":["role:applier"],"select":{"kind":["changed-version"]}}]}
```

Name each kind for what crosses, such as `merge-request` or `release-note`: 1 to 64 of a-z and 0-9, in words joined by single hyphens. Show the sentences and the files, and change them until the person says they are what they mean. A part of a kind no term selects is blocked, and a `no` term blocks what it selects.

If the drafted Accord is the approve example as it stands, `placket init approve` founds it in one command (3). Otherwise, go on.

### Found it

Show the person each command before you run it, and run it only with their yes. Work in one new directory of their own, mode 0700, and keep the window's socket in it, with a short path (a socket's path is limited to about 100 characters). For one person who holds every key, the steps are:

```sh
# Keys: the mediator, its operator, the principal who binds the parties, and each party.
for k in mediator operator principal agent applier decider; do
	placket key new --out "$k.key" >"$k.ref"
done
# The policy: the roles, and the principal that may grant them.
echo "{\"roles\":[\"agent\",\"applier\",\"decider\"],\"principals\":[{\"principal\":\"$(cat principal.ref)\",\"roles\":[\"agent\",\"applier\",\"decider\"]}]}" >policy.json
# Each party signs its terms and seals them; the openings go to the window alone.
mkdir -m 700 openings
for a in agent applier decider; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket seal --key "$a.key" --declaration "$a.declaration" --out openings >"$a.seal"
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat mediator.ref)" \
		--until 2099-01-01T00:00:00Z --policy policy.json >"$a.binding"
done
echo "{\"actor\":\"$(cat mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator.json
placket card --default >card.json
placket founding --policy policy.json --mediator mediator.json --card card.json \
	--seal agent.seal --seal applier.seal --seal decider.seal \
	--binding agent.binding --binding applier.binding --binding decider.binding >founding.json 2>founding.out
```

`founding.out` holds the line `accord <id>`: the Accord's id, `ID` below. Start the window in a terminal of its own, and leave it running:

```sh
placket window --key mediator.key --founding founding.json --openings openings --socket "$PWD/s" --journal journal
```

Then each party is shown its own view, what will cross for it. For each party, run `placket view`, show the person what it prints, and only with their yes to that view run `placket accept`:

```sh
placket view --key agent.key --socket "$PWD/s" --accord ID
placket accept --key agent.key --socket "$PWD/s" --accord ID
```

The window prints `accord <id> founded` once every party has accepted.

### One give, verified

Make one give as the agent, of the kind its `maybe` term asks about, to the applier's reference, and show the person what the window and the give print:

```sh
placket give --key agent.key --socket "$PWD/s" --accord ID --to "$(cat applier.ref)" --kind proposal --text 'A first proposal.'
```

The terms ask the decider. The person answers it, never you: `placket questions --key decider.key --socket "$PWD/s" --accord ID` lists it, and `placket answer`, `placket change` or `placket decline` with the question's id answers it, when the person runs it. Then verify the journal with every opening:

```sh
placket verify --accord ID --receipts journal --founding founding.json --acceptances acceptances.json \
	$(for o in openings/*; do printf ' --open %s' "$o"; done)
```

It prints `valid: <n> receipts signed by mediator <key>` and `conforms: every decision is one this Accord let its decider make`.

### The labels config

`placket mcp` and every socket command reach a mediator by a label in `mediators.json`, in placket's config directory (`$XDG_CONFIG_HOME/placket`, else `~/.config/placket`). Draft one entry per party key, in the shape of `api/config/mediators.json` in the repository: the `label` (1 to 63 of a-z, 0-9 and -, starting with a letter or a digit, unique in the file), the `accord` id, the `key` file, the `founding` set and the `socket`, each path absolute:

```json
{"mediators":[
 {"label":"notes-agent","accord":"sha256:<hex>","key":"/path/agent.key","founding":"/path/founding.json","socket":"/path/s"},
 {"label":"notes-applier","accord":"sha256:<hex>","key":"/path/applier.key","founding":"/path/founding.json","socket":"/path/s"},
 {"label":"notes-decider","accord":"sha256:<hex>","key":"/path/decider.key","founding":"/path/founding.json","socket":"/path/s"}]}
```

Show the entries. Write them only with the person's yes, after the entries the file already holds, and never over a label it holds. `placket mediators` then lists them.

## 5. Reach it from the agent

End by printing the line that adds `placket mcp` to the person's agent. Name only the labels whose key holds no answering role: the agent's, never the decider's. `placket mcp` refuses a label whose key answers questions, and it never answers one.

For Claude Code:

```sh
claude mcp add placket -- placket mcp --label notes-agent
```

For any other agent, register a stdio MCP server whose command is `placket` and whose arguments are `mcp --label notes-agent`, with one `--label` per label. Use the labels `placket init` printed, or those the person wrote in 4. The person answers each question from their own terminal, with `placket watch --label <the decider's label>`.
