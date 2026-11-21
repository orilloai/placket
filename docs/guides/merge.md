# An out-gate on a merge

An agent proposes to merge a pull request on GitHub. You accept it, or decline it. The merge happens only through an applier, after placket released it with your accept, and exactly at the head commit you accepted; a receipt records each step, and anyone can follow the merge commit back to it and verify it. This guide sets up `placket init merge` and its applier, [`patterns/github-merge/`](../../patterns/github-merge/).

This guide starts with `placket` on your `PATH`. The [install section of the README](../../README.md#install) says how to get it there. Nothing here needs a judge's key. The applier also needs GitHub's command line, `gh`, and `jq`.

## Six words

- An **Accord** is the agreement between the parties: here the agent, you, and the applier that merges. A mediator, the window you start, runs it and signs a receipt for every decision.
- Each party's **terms** say what may cross to or from it. The agent's terms say its `merge` to the applier is asked of the role `decider`, which your key holds, and list no judge, since a merge cannot be undone. Each party's terms are sealed from the others; you consent to your own view of what will cross.
- An **ask** is a part held until someone answers it. Your **answer** to a merge is accept or decline. An accepted merge is released to the applier as it was sent; a declined one releases nothing.
- A **receipt** is the mediator's signed statement of one decision about one part. The window keeps them in its journal, and the applier posts the delivery it merged, with its receipt, on the pull request.
- To **verify** is to check the receipts: that the mediator signed every one, which key witnessed each answer, and, with every party's terms, that every decision is one the Accord let its decider make.

## 1. Set up the repository first

The receipt says what the mediator released. That nothing merged any other way rests on the repository's settings: a ruleset that lets only the applier merge, the accounts the agent and the window run in, and the applier's credential. These are setup steps on your repository, which placket neither makes nor checks. Follow every step of the [custody checklist](../../patterns/github-merge/custody.md) before the agent merges anything. Until then, the steps below show the flow and protect nothing.

## 2. Found the Accord

```sh
placket init merge
```

It writes everything into `~/.config/placket/merge/` (`--dir` picks another, empty, directory): the keys of the mediator, its operator, the principal and each party, all mode 0600; each party's terms; and the founding set, with each party's acceptance of its own view, signed with its key, so the Accord is founded. It prints the Accord's id, its three parties, `merge-agent`, `merge-person` and `merge-applier`, with their key references and roles, and the commands for the steps below. Custody step 3 keeps your key, the window and the applier in an account the agent cannot reach: run `init` there.

## 3. Start the window

Run the `placket window` line `init` printed, in a terminal of its own, and leave it running.

## 4. Let the agent propose a merge

Add placket to Claude Code with the line `init` printed, or behind the forced command of custody step 3:

```sh
claude mcp add placket -- placket mcp --label merge-agent
```

Ask the agent to propose a merge: a `give` on mediator `merge-agent`, of kind `merge`, as data, to the applier's key reference from the `party merge-applier` line, with exactly the repository as `owner/name`, the pull request's number, its base branch and its head commit:

```json
{"repo": "OWNER/NAME", "pr": 7, "base": "main", "head": "<the head commit's SHA>"}
```

The give returns the question's id: the terms ask you.

## 5. Answer

Run the `placket watch` line `init` printed, `placket watch --label merge-person`. It prints each new question with the lines that answer it. Read the merge the agent proposed with `placket questions --label merge-person`, check the pull request at that head yourself, and run the `placket answer` line to accept it, or `placket decline` with `--comment` to refuse it.

## 6. Apply

Fetch the applier with the line `init` printed. It takes [`patterns/github-merge/apply.sh`](../../patterns/github-merge/apply.sh) from the repository at the commit of your build's version tag when the build carries that commit, else at the tag itself, which can be moved, or from `main` for a build with no version tag, which can change at any time. It holds your merge credential and is what lets only accepted merges through, and nothing checks the file it fetches, so read it, and its [`README.md`](../../patterns/github-merge/README.md), before you run it. Then run it, as custody step 4 says, with the repository in place of `OWNER/NAME`:

```sh
sh ~/.config/placket/merge/applier/apply.sh merge-applier OWNER/NAME
```

It merges only a release that a key bound to `decider` accepted, and only while the pull request is open, unmerged and exactly what was released, at the released head commit. It posts the delivery as a comment on the pull request, sets a `placket` status on the merge commit naming the receipt, and prints `applied pr 7`. Anything else it refuses, with its reason, or it stops and says what to do.

## 7. Verify

Verify the journal with every party's opening, `ID` being the Accord id `init` printed:

```sh
d=~/.config/placket/merge
placket verify --accord ID --founding "$d/merge/founding.json" --acceptances "$d/merge/acceptances.json" \
	--receipts "$d/merge/journal" $(for o in "$d"/*.opening.json; do printf ' --open %s' "$o"; done)
```

It prints `valid: <n> receipts signed by mediator <key>`, `conforms: every decision is one this Accord let its decider make`, and that your accept was witnessed by your key. An auditor who holds no opening follows a merge commit back to its receipt, as [the pattern's README](../../patterns/github-merge/README.md), "The auditor's path", says.

## Next

- [`approve`](approve.md) is the same flow for a file on your own machine, and [`handoff`](handoff.md) hands work between agents across two mediators.
- To retire a key, close the Accord and found its successor: [the rotation guide](rotation.md).

## Last, and optional: a judge that advises

A judge can read each merge beside you and decide nothing. Jev, called with your own key from `PLACKET_JEV_KEY`, is v0.1.0's judge. Never list it to `decide` on a merge: a merge cannot be undone, so a person answers every one. To have it `advise`, the card lists the judge and both sides' terms for `merge` list it, so the Accord is founded again: the agent's and the applier's terms for `merge` each become a `maybe` asked of `role:decider` with `"judges":[{"name":"<name>","use":"advise"}]`, the card is `placket card --default --judge judge.json`, and the window runs with `--judge judge.json`. [`patterns/shadow-judge/`](../../patterns/shadow-judge/) does each step, and runs as it is with no key.
