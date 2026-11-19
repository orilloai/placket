# Approve before apply

An agent proposes the text of a file. You accept it, change it or decline it. Only what you accepted, or your changed version, is written to the file, and every step has a receipt you can verify. This is placket's first example, and the one to try first.

This guide starts with `placket` on your `PATH`. The [install section of the README](../../README.md#install) says how to get it there. Nothing here needs a judge's key.

## Five words

- An **Accord** is the agreement between the parties: here the agent, you, and the applier that writes the file. A mediator, the window you start, runs it and signs a receipt for every decision.
- Each party's **terms** say what may cross to or from it. The agent's terms say its `proposal` to the applier is asked of the role `decider`, which your key holds. Each party's terms are sealed from the others; you consent to your own view of what will cross.
- An **ask** is a part held until someone answers it. Your **answer** is accept, change or decline. An accepted part is released to the applier as it was sent; a changed one is released as your version and returned to the agent; a declined one releases nothing.
- A **receipt** is the mediator's signed statement of one decision about one part. The window keeps them in its journal.
- To **verify** is to check the receipts: that the mediator signed every one, and, with every party's terms, that every decision is one the Accord let its decider make.

## 1. Found the Accord

```sh
placket init approve
```

It writes everything into `~/.config/placket/approve/` (`--dir` picks another, empty, directory): a key for the mediator, its operator, the principal who binds the parties, and one for each party, all mode 0600; each party's terms, in `approve/agent.terms`, `approve/person.terms` and `approve/applier.terms`; and the founding set. You hold every key, so `init` signs each party's acceptance of its own view, and the Accord is founded. Read your own view, what will cross for you, in `approve-person.key.state/<the Accord id's hex>/view.json`.

It prints the Accord's id and its three parties, `approve-agent`, `approve-person` and `approve-applier`, each with its key reference and role, and then the commands for the steps below. Each party is an entry of the labels config, so every command names it with `--label`.

## 2. Start the window

Run the `placket window` line `init` printed, in a terminal of its own, and leave it running. It prints `accord <id> founded`, then one line for each call it takes.

## 3. Let the agent propose

Add placket to Claude Code with the line `init` printed:

```sh
claude mcp add placket -- placket mcp --label approve-agent
```

`placket mcp` gives the agent the tools `give`, `get`, `questions`, `result`, `cancel` and `mediators`, as the agent's key on this Accord. It never answers a question. Ask the agent to propose the text of a file: a `give` on mediator `approve-agent`, of kind `proposal`, as text, to the applier's key reference, the one `init` printed on its `party approve-applier` line. The give returns the question's id: the terms ask you.

## 4. Answer

Run the `placket watch` line `init` printed:

```sh
placket watch --label approve-person
```

It prints each new question once, as it opens, with the three lines that answer it. Read what the agent proposed:

```sh
placket questions --label approve-person
```

Then run one of the lines `watch` printed: `placket answer` accepts, `placket change` with `--text` releases your version, and `placket decline` with `--comment` releases nothing. The agent's `result` tool tells it how you answered.

## 5. Apply

Fetch the applier with the line `init` printed. It takes `patterns/local-apply/apply.sh` from the repository at your build's version tag, or from `main` for a build with none. Read it before you run it: it is short. Then run it with the file to write in place of `FILE`:

```sh
sh ~/.config/placket/approve/applier/apply.sh approve-applier notes.txt
```

It fetches each release, checks that the answer travelling with it is an accept or a change by a key the founding set binds to `decider`, writes the text to the file and prints `applied accepted notes.txt`. A decline wrote nothing, so the file is untouched.

## 6. Verify

Every receipt is in the journal, `approve/journal/receipts.jsonl`. Verify it with every party's opening, `ID` being the Accord id `init` printed on its `accord` line:

```sh
d=~/.config/placket/approve
placket verify --accord ID --founding "$d/approve/founding.json" --acceptances "$d/approve/acceptances.json" \
	--receipts "$d/approve/journal" $(for o in "$d"/*.opening.json; do printf ' --open %s' "$o"; done)
```

It prints, among its lines, `valid: <n> receipts signed by mediator <key>` and `conforms: every decision is one this Accord let its decider make`, and for each of your answers that it was witnessed, signed by your key. Without the openings, as anyone holding only the founding set and the journal, it checks every signature and says the terms were not checked.

## Next

- `merge` (`docs/guides/merge.md`) gates an agent's merge on GitHub the same way, and `handoff` (`docs/guides/handoff.md`) hands work between agents across two mediators.
- To retire a key, close the Accord and found its successor: `docs/guides/rotation.md`.
- To found an Accord between parties who do not share a machine, each sealing its own terms and accepting its own view, see the README's "Found an Accord".

## Last, and optional: a judge that advises

A judge can read each proposal beside you and decide nothing. Jev, called with your own key from `PLACKET_JEV_KEY`, is v0.1.0's judge. The card lists the judge, and both sides' terms list it to `advise` on the asked kind, so the Accord is founded again: the agent's and the applier's terms for `proposal` each become a `maybe` asked of `role:decider` with `"judges":[{"name":"<name>","use":"advise"}]`, the card is `placket card --default --judge judge.json`, and the window runs with `--judge judge.json`. `patterns/shadow-judge/` does each step, and runs as it is with no key.
