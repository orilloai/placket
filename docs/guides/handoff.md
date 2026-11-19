# Handoffs between agents

One agent hands work to another under terms, and each handoff has a receipt. A planner hands a plan to a coder through one mediator, and the coder hands its work to a reviewer through another. Nobody is asked: the terms pass each handoff, and refuse anything they do not name. This guide sets up `placket init handoff`.

This guide starts with `placket` on your `PATH`. The [install section of the README](../../README.md#install) says how to get it there. Nothing here needs a judge's key.

## Five words

- An **Accord** is the agreement between the parties of one mediator. Here there are two: `handoff-plan`, between the planner and the coder, and `handoff-review`, between the coder and the reviewer. Each has its own window, journal and keys, and the coder holds one key in each.
- Each party's **terms** say what may cross to or from it. The planner's terms let it give a `plan` to the coder, and the coder's let it get one; in the other Accord, the coder gives its `work` to the reviewer. A part of a kind no term names is blocked. Each party's terms are sealed from the others; each consents to its own view of what will cross.
- An **ask** holds a part until a role the terms name answers it, with accept, change or decline. These terms ask nobody, so every handoff passes or is blocked at once; [the `approve` guide](approve.md) shows an ask.
- A **receipt** is the mediator's signed statement of one decision about one part. Each window keeps its own in its journal, and each delivery a party fetches carries its receipt.
- To **verify** is to check the receipts: that the mediator signed every one, and, with every party's terms, that every decision is one the Accord let its decider make.

## 1. Found the two Accords

```sh
placket init handoff
```

It writes everything into `~/.config/placket/handoff/` (`--dir` picks another, empty, directory): one mediator key for both Accords, its operator's and the principal's keys, and a key per party and Accord, all mode 0600; each party's terms, under `handoff-plan/` and `handoff-review/`; and each founding set, with each party's acceptance of its own view, so both are founded. It prints each Accord's id and parties, `handoff-plan-planner`, `handoff-plan-coder`, `handoff-review-coder` and `handoff-review-reviewer`, with their key references and roles, and the commands for the steps below.

## 2. Start both windows

Run each of the two `placket window` lines `init` printed, each in a terminal of its own, and leave them running. Each listens on its own socket and writes its own journal.

## 3. Let the agents hand work on

Add placket to Claude Code with the line `init` printed:

```sh
claude mcp add placket -- placket mcp --label handoff-plan-planner --label handoff-plan-coder --label handoff-review-coder --label handoff-review-reviewer
```

It serves all four labels, each as its own mediator with its own key, and `mediators` lists them. To keep the agents apart, add placket in each agent's own project with only that agent's labels instead: the planner's, the coder's two, the reviewer's.

Then ask for the handoffs, each a tool call on the label of the party that makes it:

1. The planner gives the plan: `give` on mediator `handoff-plan-planner`, of kind `plan`, as text, to the coder's key reference from the `party handoff-plan-coder` line. It passes by the terms, and the reply holds its receipt.
2. The coder fetches it: `get` on mediator `handoff-plan-coder`, with a `wait` of up to 60 seconds. The delivery holds the plan and its receipt id.
3. The coder hands on its work: `give` on mediator `handoff-review-coder`, of kind `work`, to the reviewer's key reference from the `party handoff-review-reviewer` line.
4. The reviewer fetches it: `get` on mediator `handoff-review-reviewer`.

## 4. Verify each journal

Verify each Accord's journal with its parties' openings, `PLAN` and `REVIEW` being the two Accord ids `init` printed:

```sh
d=~/.config/placket/handoff
opens() { for k in "$d/handoff-$1-"*.key; do printf ' --open %s/%s.opening.json' "$d" "$(placket key ref --key "$k" | sed 's/^ed25519://')"; done; }
placket verify --accord PLAN --founding "$d/handoff-plan/founding.json" --acceptances "$d/handoff-plan/acceptances.json" \
	--receipts "$d/handoff-plan/journal" $(opens plan)
placket verify --accord REVIEW --founding "$d/handoff-review/founding.json" --acceptances "$d/handoff-review/acceptances.json" \
	--receipts "$d/handoff-review/journal" $(opens review)
```

Each prints `valid: <n> receipts signed by mediator <key>` and `conforms: every decision is one this Accord let its decider make`. The two journals are checked apart: one mediator's receipts say nothing of the other's.

## Next

- [`approve`](approve.md) asks a person before an agent's proposal is applied, and [`merge`](merge.md) gates a merge on GitHub the same way.
- To retire a key, close each Accord and found its successor: [the rotation guide](rotation.md).

## Last, and optional: a judge that advises

A judge acts only where the terms would ask, so in these Accords, which ask nobody, it has nothing to read. To have Jev, called with your own key from `PLACKET_JEV_KEY`, advise on a handoff, the handoff is asked first: both sides' terms for its kind become a `maybe` asked of a role a person holds, listing `"judges":[{"name":"<name>","use":"advise"}]`, the card is `placket card --default --judge judge.json`, the Accord is founded again, and its window runs with `--judge judge.json`. [`patterns/shadow-judge/`](../../patterns/shadow-judge/) does each step, and runs as it is with no key.
