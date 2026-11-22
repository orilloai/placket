# Terms

A party's terms say what may cross to it and from it, and with whom. Each party signs its own declaration of terms, and seals it, so the other parties never read it; each consents instead to its own view of what will cross ([the Accord](accord.md)). The mediator reads every party's terms, and decides each part from both sides' ([section 5 of the protocol](../../spec/protocol.md#5-composition-and-the-verdict)).

## A term

A declaration holds a list of terms (setup-contract-1). Each term has:

- a `stance`: `want` or `need`, which compose alike (composition-6); `no`, a refusal; or `maybe`, an ask;
- a `dir`: `give`, for parts this party sends, or `get`, for parts it receives;
- `with`: the roles the other side must hold, each `role:<label>`;
- `select`: the kinds of part it is about, under `kind`.

A `maybe` term also has `resolve`: `to`, the roles that answer the question, `timeout`, how many seconds the question stays open, and, optionally, `judges`, each to `decide` or to `advise` ([judges](../guides/judges.md)).

These are the agent's terms of `placket init approve`: it gives a `proposal` to the applier, asked of the role `decider` for up to an hour, and gets the decider's answers.

```json
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","changed-version","comment"]}}]}
```

## How a part is decided

A term matches a part when it is the sender's of `dir` `give` or the receiver's of `dir` `get`, its kind is among the term's, and one of its `with` roles is a role the other party's binding gives it (composition-1). Then, in this order, the first rule that applies decides:

1. A matching `no`, of either side, blocks the part, reason `no-match` (composition-3).
2. A part with no matching term on one side or the other is blocked, reason `no-match`: a kind no term names never crosses (composition-2).
3. Matching `maybe` terms must agree on who answers and on the timeout; when they differ, the part is blocked, reason `maybe` (composition-5).
4. A judge both sides list to `decide` may pass or block it (composition-7).
5. A matching `maybe` asks: the part is held as a question ([ask and answer](ask.md)).
6. Anything else passes, reason `ok`.

So a part crosses only when both sides said it may. The same terms decide the same part the same way every time, and a verifier who holds every party's opening decides each receipt again to check it ([verify](../verify.md)).

## Where terms are read

- `placket declare` signs a declaration, and `placket seal` seals it ([the command line](../reference/cli.md#declare)).
- A member's view, from `placket view`, shows each crossing of its own as `crosses`, `blocked` or `asked`, and never says whether the other side refused or said nothing (setup-contract-14).
- `placket verify` with every party's opening, `--open`, checks every decision against the terms; without them it checks what was signed and says the terms were not checked.
