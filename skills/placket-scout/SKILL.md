---
name: placket-scout
description: Read the person's own repository or product description and list the places where an agent's action cannot be undone or leaves their system, such as a merge, a send, a publish, a payment, a deploy or an export, each with the placket pattern or kind of term that would gate it and what a receipt there would prove. Reads only locally, sends nothing and changes nothing.
---

# Find where placket could gate an agent

Placket mediates the parts that cross between parties under terms each party signs, asks a person where the terms say to, and signs a receipt for every decision. This skill finds the places in the person's own system where that would help: where an agent's action cannot be undone, or leaves their system.

## What this skill does not do

- **It reads only locally.** It reads the files of the repository the person points you at, or the product description they give you. It does not fetch anything, and it runs none of the repository's code, builds or tests.
- **It sends nothing anywhere.** No network command, upload, issue, comment or message. What you read stays in this conversation.
- **It changes nothing.** It writes no file, sets nothing up and installs nothing. Setting up is `placket-setup`'s, and only with the person's yes.
- **It claims only what a receipt proves.** Placket makes no claim about what a receiver does with a part after it crossed. Every suggestion says what its receipts would prove, in the words below, and nothing more.

## 1. Read

Ask the person which repository, or which product, to read. For a repository, read what tells you what agents and automation can do in it, for example:

- the agent's own instructions and tools: `AGENTS.md`, an MCP server config, the tools an agent is given, the commands it is allowed to run;
- CI and release workflows, deploy scripts, `Makefile` targets, infrastructure code;
- code that calls an outside service: a payment provider, mail or chat, a package registry, cloud storage, a forge's API, a database that other systems read.

For a product description, ask what an agent does in it, what it hands on, and to whom.

## 2. List the places

List each place where an action cannot be undone, or leaves the person's system. A starting list, not a complete one:

| Action | Look for | What would gate it |
|---|---|---|
| A merge | an agent or a workflow that merges pull requests, pushes to a protected branch, or calls a forge's merge API | the `github-merge` pattern: the agent gives a merge of a pull request, a person answers, and an applier that alone holds the right to merge merges only what was accepted, at the head commit accepted |
| A send | mail, chat or webhook calls, notifications to customers | a `maybe` term on the agent's give of the message's kind, such as `outbound-message`, asked of the role `decider`, and an applier that sends only what is released to it; the `local-apply` pattern is the shape of that applier |
| A publish | a package registry upload, a release, a post, a page made public | a `maybe` term on a kind such as `release-request`, asked of the role `decider`, with an applier that publishes only what is released |
| A payment | a payment provider's calls, refunds, payouts, transfers | a `maybe` term on a kind such as `payment-request`, asked of the role `decider`; or a `no` term on that kind, so that placket blocks every such part from the agent |
| A deploy | deploy scripts, infrastructure apply steps, production database migrations | a `maybe` term on a kind such as `deploy-request`, asked of the role `decider`, with an applier that deploys only what is released |
| An export | data written outside the system: an upload, a file shared, a report sent to a third party | the `guarded-export` pattern: the receiver's guard exports a value only when exactly that value was released to it |
| A file changed on the person's machine | an agent that writes files outside its sandbox | the `local-apply` pattern, which `placket init approve` sets up: an applier writes the file only with the text the person accepted, or their changed version |

For each place you find, write:

1. **Where:** the file and line, or the part of the description.
2. **The action:** what happens, and why it cannot be undone or leaves the system.
3. **The gate:** the pattern from the table, or the kind of term, and the kind of part the agent would give.
4. **What a receipt there proves:** for the ask, `claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.`; for an accepted or changed answer, `claim: a key holding the answering role answered. The part named here was judged and queued for the receiver this receipt names.`; for a decline, `claim: the question closed without a yes. The receipt names the first holder of the answering role.` Every claim is followed by the scope line: `not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.` So a receipt shows what the person's key answered and what was released to the applier. That the merge, the send or the deploy then did only that is the applier's doing, not a receipt's.
5. **What must hold for the gate to hold:** the agent must not be able to act around it. The credential that performs the action (the merge right, the API key, the deploy key) belongs to the applier alone, in an account the agent cannot read, and the agent reaches the mediator only through `placket mcp`. Say plainly where that is not the case today.

Order the list by what is hardest to undo. Say what you could not tell from what you read, rather than guess.

## 3. Offer the next step

End by offering to set up any one of the places with the `placket-setup` skill: from its pattern, or as an Accord of the person's own with the kind and term you named. Make no change yourself.
