# Claude Code over MCP, a worked example

Claude Code reaches a mediator through `placket mcp`, a local MCP server that is one party's client: the agent gives, fetches and asks for results as its own key, and never answers a question. This guide wires it up on the example of `placket init approve` and follows one proposal from the agent's tool call to the person's answer.

It starts with `placket` on your `PATH` ([install](../../README.md#install)) and Claude Code's `claude` command. Nothing here needs a judge's key.

## 1. Found the Accord and start its window

```sh
placket init approve
```

Then run the `placket window` line `init` printed, in a terminal of its own, and leave it running. The labels config now holds one entry per party: `approve-agent`, `approve-person` and `approve-applier` ([several mediators](mediators.md)).

## 2. Add placket to Claude Code

Run the line `init` printed:

```sh
claude mcp add placket -- placket mcp --label approve-agent
```

Claude Code starts `placket mcp` when it needs it. On standard error it prints `placket mcp: approve-agent served`. It serves only the labels it is started with, each with its own key, and reads nothing of any other entry.

The agent gets six tools, each named for its call, and each taking `mediator`, the label it acts under ([the MCP tools](../reference/mediator-api.md#the-request-layer)):

- `give` gives one part, as text or data, and returns its receipt; when the terms ask about it, it returns the question's id at once;
- `get` fetches the next delivery queued for the key, and with `wait` waits for one;
- `questions` lists the open questions the key may answer;
- `result` says how a question the key asked closed, and with `wait` waits for it;
- `cancel` withdraws a question the key asked;
- `mediators` lists the labels config's entries, and which are served.

No tool accepts, changes or declines. A label whose key answers questions under its Accord is refused, `answering-key`, unless `placket mcp` is started with `--allow-answering`; keep that off for an agent's key.

## 3. The agent proposes

Ask the agent to propose the text of a file through placket: a `give` on mediator `approve-agent`, of kind `proposal`, as text, to the applier's key reference from the `party approve-applier` line `init` printed. The terms ask the role `decider` about every proposal, so the give returns an ask: an `outcome` of verdict `escalate` and reason `maybe`, with its receipt, and `question`, the receipt's id, which is the question's id.

The agent then calls `result` with that question and a `wait` of up to 60 seconds. While you have not answered, it returns `"open": true`, and the agent calls it again.

## 4. You answer

In a terminal of your own:

```sh
placket watch --label approve-person
```

`watch` prints the new question, with its kind, asker and deadline, and the three lines that answer it. Read what the agent proposed, then accept it:

```sh
placket questions --label approve-person
placket answer --label approve-person --question ID
```

or change it with `placket change` and `--text`, or decline it with `placket decline` and `--comment`. Your answer is signed with your key, not the agent's.

## 5. The agent hears, and the applier applies

The agent's next `result` holds the closing, whose `word` is `accepted`, its `release`, a `pass`, and `answer`, the delivery of your answer: the mediator queued the proposal for the applier, exactly as it was given ([ask and answer](../concepts/ask.md)). After a change, the closing's word is `changed`, and `answer` holds your version.

The applier fetches the release and writes the file, as step 5 of [the approve guide](approve.md#5-apply) says, and the journal verifies as step 6 says ([verify](../verify.md)).

## When a tool call fails

A tool error's text is the code, a colon and one sentence of cause and fix, such as `not-founded: The Accord is proposed, not founded: ...`. [The errors table](../reference/mediator-api.md#errors) lists every code `placket mcp` can return.

## Keep the agent apart

As `init` sets it up, every key is in your own account. An agent that runs commands as you, as Claude Code with a shell does, can read your key and answer its own proposal, so this setup shows the flow but does not hold against the agent. For a gate that holds, run the agent in an operating system account of its own, and the window, your key and the applier in another, with `placket mcp` behind a forced command the agent's account may run ([custody](custody.md)).

## Several agents, several mediators

`placket mcp` takes `--label` once per mediator, and serves each on its own, with its own key; the `mediators` tool lists them. To keep agents apart, add placket in each agent's own project with only that agent's labels, as [the handoff guide](handoff.md#3-let-the-agents-hand-work-on) does.
