# The mediator, the window and the door

The mediator is what runs an Accord: it decides each part under the terms, holds each question, queues each delivery and signs each receipt. The window is the process that runs it, and the door is how a party reaches the window ([section 4 of the protocol](../../spec/protocol.md#4-giving-and-getting-a-part-at-the-door)).

## The mediator

The founding set names the mediator: its key, which signs every receipt; its operator, the party that runs it; and how it runs, which in this version is `process` (setup-contract-7). The decision module the card names decides each part, and only it ([terms](terms.md)). A mediator founds only a set that names its own key.

## The window

`placket window` runs one Accord's mediator, until it is interrupted. It reads the founding set, the openings of every member, and the acceptances, and writes its journal: the receipts, the answer calls, the open questions, the deliveries not yet fetched and what each judge said. Every write is synced before the window replies.

```sh
placket window --key mediator.key --founding founding.json --openings openings --journal journal
```

With no `--socket` it listens on the Accord's own socket under the runtime directory, and with no `--journal` it writes to the Accord's own directory under placket's state directory ([the command line](../reference/cli.md#window)). A window restarted on its journal checks every receipt again, and closes each question left open as `undeliverable`; the asker gives again. A delivery not yet fetched stays on disk until it is fetched.

In this version the window is one process on one machine, run by one party to the Accord. Its user holds the mediator's key and every party's opening, so it reads every party's terms and can stop, edit or shorten the journal. Placket does not defend the other parties against that user; it gives them their own kept records to check the journal with, with `placket reconcile` ([the threat model](../security/threat-model.md)).

## The door

The door is a unix socket, mode 0600. A client connects, writes one signed call, reads one reply, and closes. The door checks each call in order: its size, its form, its signature, its Accord, a time within 60 seconds of the window's, and a nonce it has not seen; the first that fails is refused under the reply's `door` member. Then the mediator rejects what the caller did wrong, such as a key that is not a member, under `rejection`. A call of `questions`, `get` or `result` may wait up to 60 seconds for something to return.

These calls are [the mediator API](../reference/mediator-api.md). Three clients speak it:

- the command line, one command per call ([the command line](../reference/cli.md));
- `placket mcp`, which gives an agent the calls as MCP tools ([Claude Code over MCP](../guides/claude-code.md));
- any client of your own, built on the API's schemas.

One machine may run several windows, one per Accord, each with its own socket and journal ([several mediators](../guides/mediators.md)).
