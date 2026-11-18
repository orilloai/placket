# placket

Placket mediates content crossing between parties, and records what crossed in receipts anyone can verify.

It sits at a handoff between parties that do not fully trust each other: agents, people, tools. Each party declares, and signs, the terms under which a part may cross to the other. For every part offered, placket decides whether it passes, is blocked, or must be asked about: then a named person, holding the role the terms name, accepts it, changes it or declines it. Placket signs a receipt for every decision, chained per pair of parties, and a third party verifies the receipts with the mediator's public key alone.

The first job is ask and log: one system proposes, another applies, and a named person is asked about every proposal. An accepted proposal is released to the applier exactly as it was sent; a changed one is released as the person's version and returned to the proposer; a declined one releases nothing. The deliverable is the signed log of what was asked, answered and released.

## What a receipt claims

Each receipt states one decision about one part, under one Accord: the part's salted digest and kind, the two keys, the Accord, the card that names the decision module, and a time. Whoever holds the body and its salt can show that a part is the one judged. `placket show` prints, for each receipt, the line of what it claims, and `placket verify` prints the claim of a verified journal:

- A pass: `claim: the mediator judged this part under the Accord's terms and queued it for the receiver.`
- A block: `claim: the mediator refused this part for the reason shown. Nothing was queued.`
- An ask: `claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.`
- An accepted or changed answer: `claim: a key holding the answering role answered. The part named here was judged and queued for the receiver this receipt names.`
- An answer or a changed version that was refused: `claim: a key holding the answering role answered. The part named here was refused for the reason shown.`
- A decline or a cancel: `claim: the question closed without a yes. The receipt names the first holder of the answering role.`
- A timeout or an undeliverable question: `claim: nobody answered. The question closed for the reason shown.`
- A verified journal: `claim: each receipt is this mediator's signed decision about one part under this Accord's terms.`

Every claim line is followed by the scope line, which says what no receipt claims:

`not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.`

A receipt also does not show that the mediator is honest, or that its clock is true. In this version the window is one process on one machine, run by one party to the Accord: a receipt from it proves less to the other parties than a receipt from a mediator nobody in the Accord runs, and what they still hold is their own kept receipts and parts, which `placket reconcile` checks against any journal they are shown.

## Run the demo

`make demo` runs the worked example, `scripts/demo.sh`, with Go 1.27 on Linux or macOS: it builds the `placket` binary, makes the keys of a mediator, an operator, a principal and three parties (a proposer, an applier and a decider), signs their terms and bindings, founds the Accord, and starts a window. Then the proposer asks three proposals and waits on each; the decider accepts the first, changes the second with a comment, and declines the third with a comment, each from its own commands; the applier fetches what was released. The proposer leaves a fourth proposal open and is refused when it tries to answer it itself, and gives a part of a kind no term names, which is blocked. The window is killed and started again: the open proposal is closed as undeliverable, and the applier fetches the changed version it had not fetched yet. Last, a third party verifies the journal, reconciles each party's records with it, and exports the log, four rows; then changes one character of a receipt's signature, and verification fails. `sh scripts/demo.sh DIR` keeps everything it made in `DIR`.

The journal of one run is committed under `scripts/demo/`, with the founding set and the mediator's public reference, and no key. Anyone can verify it:

```
go build -o placket ./cmd/placket
./placket verify --accord "$(sed -n 's/^{"accord":"\(sha256:[0-9a-f]*\)".*/\1/p' scripts/demo/journal/accord)" --receipts scripts/demo/journal --founding scripts/demo/founding.json
```

Given only the Accord's id and its founding set, it checks that every receipt is the mediator's and that every decision was one the Accord let its decider make, without running anything again: it prints `conforms: every decision is one this Accord let its decider make`, and the journal's `answers.jsonl` shows each answer witnessed by the key that signed it.

## Found an Accord

An Accord is founded from five kinds of file: the policy, the mediator, the card, and each party's declaration and binding. These are the demo's founding steps, from `scripts/demo.sh`:

```
# Keys: the mediator, its operator, the principal who binds the parties, and each party.
for k in mediator operator principal proposer applier decider; do
	placket key new --out "$k.key" >"$k.ref"
done
# The policy: the roles that must be held, and each principal with the roles it may grant. Here one
# principal stands behind all three parties.
echo "{\"roles\":[\"proposer\",\"applier\",\"decider\"],\"principals\":[{\"principal\":\"$(cat principal.ref)\",\"roles\":[\"proposer\",\"applier\",\"decider\"]}]}" >policy.json
# Each party signs its terms; its principal binds it to its role under this mediator and this policy.
for a in proposer applier decider; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat mediator.ref)" --until 2099-01-01T00:00:00Z \
		--policy policy.json >"$a.binding"
done
echo "{\"actor\":\"$(cat mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator.json
# The card names the decision module this binary runs.
placket card --default >card.json
placket founding --policy policy.json --mediator mediator.json --card card.json \
	--declaration proposer.declaration --declaration applier.declaration --declaration decider.declaration \
	--binding proposer.binding --binding applier.binding --binding decider.binding >founding.json
placket window --key mediator.key --founding founding.json --socket /tmp/plk.s --journal journal
```

Each party's binding is signed by that party's own principal, under the policy it names by hash, and gives only roles the policy lets that principal grant. One principal may stand behind every party, as here, or each side may have its own, as in the guarded-export pattern (`patterns/guarded-export/`); `verify --founding` names each member's roles and principal.

`placket card --default` prints the binary's own card on standard output, and on standard error its card hash and `module <version> <hash>`. The hash is taken from the source of the decision module, `core/composition/composition.go`, that the binary was built from, so a binary with another module prints another card (`docs/adr/0012-the-kernel-hash.md`). Never copy a card from another build:

- `placket founding` founds under any card that reads, since whoever founds an Accord need not run its window, but prints `module <version> <hash> is not this build's decision module, <version> <hash>` when the card is not its own.
- `placket window` refuses to run an Accord whose card names another module: it prints that line and `error founding kernel`, and starts nothing. After an upgrade that changes the module, found the Accord again with the new binary's card; the old journal still verifies.
- `placket verify --founding` prints that line after the module line, still verifies, and says that conformance was not checked, since this build cannot decide again as another module would.

Its first receipt, `seq` 1 of the proposer's chain to the applier, is the first proposal's ask:

```
{"payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJwcmVkaWNhdGUiOnsiYWNjb3JkIjoic2hhMjU2OjE1YzA4MzkwOGEyODhlNzE5Yzk0YzM4ZTQzYzdmMjkzOTljYjVmMTBmOGRkOTJhYjM4MmRiMDVkODcwZjgyNTciLCJhdCI6IjIwMjYtMTEtMTdUMjA6MDg6NTRaIiwiYnkiOiJ0ZXJtcyIsImNhcmQiOiJzaGEyNTY6ZTM2N2Q1ZTE4MjA5OGJiYjNhM2NlOTYwYjMyMjFhYTJhMWMwZDZlZDJmYzc5MGRmYTQ2MDRlNjA0MTkxZDZhNSIsImVudmVsb3BlIjp7Imhhc2giOiJzaGEyNTY6NzgyNGJlODE5YzhiY2NmYTQ4ZTgxZWEwNjYwN2U3MjZhYzA4ZWM2YTE1YTdkMmMyYjY0ZGM1Yjk0NDA4MmMxMCIsInNlbmRlciI6ImVkMjU1MTk6MjcyM2ZhMmRmZTZhYzA5ZGU0NjAwNTk3NjQ3MDZkM2I0OWE5ZTBmMDlkYWY5ZjM4NWM5OWRlOTZmMzUzMmUxZSIsInNlcSI6MX0sImZyb20iOiJlZDI1NTE5OjI3MjNmYTJkZmU2YWMwOWRlNDYwMDU5NzY0NzA2ZDNiNDlhOWUwZjA5ZGFmOWYzODVjOTlkZTk2ZjM1MzJlMWUiLCJraW5kIjoiY3Jvc3NpbmciLCJtZWRpYXRvciI6ImVkMjU1MTk6OWRiYzBhYjEzMWVmZjc1MWVkN2RmMmRjOTliMjViYzEwZGJhZDEyNmEwNjI5OWVlMmU0YTk4YmE4ODU1NWI0MyIsInJlYXNvbiI6Im1heWJlIiwic2VxIjoxLCJ0byI6ImVkMjU1MTk6YWExMjgyODE4ZjZmYzI3YWEyNjM4YzU0YWZiY2Y5ZWE5ZDRkZDY0ODBmZmRmYWY5YjNiYWJhM2Y1OTg5MTVjZiIsInZlcmRpY3QiOiJlc2NhbGF0ZSJ9LCJwcmVkaWNhdGVUeXBlIjoiaHR0cHM6Ly9wbGFja2V0LnRlY2gvcmVjZWlwdC8wLjEiLCJzdWJqZWN0IjpbeyJkaWdlc3QiOnsic2FsdGVkLXNoYTI1NiI6IjY5MzJiOTg3OTlhZGU4NWNjMTViOGYwYTUzZTFiYmFkNzFhMDI4ZTRjY2UzYTgxNGUwMmU5ZTcxMGZmNTBhNjcifSwibmFtZSI6InByb3Bvc2FsIn1dfQ==","payloadType":"application/vnd.in-toto+json","signatures":[{"keyid":"ed25519:9dbc0ab131eff751ed7df2dc99b25bc10dbad126a06299ee2e4a98ba88555b43","sig":"yb9jroUogg7a2Kg0ZCrst6alGgCdyeppp4g79ICp98RikmWhoUy7PTZ6WO1clqv+SmosSEusI2NDnK+eRpbUBA=="}]}
```

and `placket show --receipt` prints, on standard error, then on standard output:

```
not verified
ask maybe proposal 2026-11-17T20:08:54Z
from ed25519:2723fa2dfe6ac09de460059764706d3b49a9e0f09daf9f385c99de96f3532e1e to ed25519:aa1282818f6fc27aa2638c54afbcf9ea9d4dd6480ffdfaf9b3baba3f598915cf
module: named by card sha256:e367d5e182098bbb3a3ce960b3221aa2a1c0d6ed2fc790dfa4604e604191d6a5.
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
not verified: this receipt was decoded without checking its signature, its mediator or its chain
receipt sha256:3f71a315579ab04ee9633fdcbb1534be1fb7a7dedbd11a22aa6ad84b79dabf60
crossing from ed25519:2723fa2dfe6ac09de460059764706d3b49a9e0f09daf9f385c99de96f3532e1e to ed25519:aa1282818f6fc27aa2638c54afbcf9ea9d4dd6480ffdfaf9b3baba3f598915cf
verdict escalate, reason maybe
by terms
subject proposal, salted-sha256 6932b98799ade85cc15b8f0a53e1bbad71a028e4cce3a814e02e9e710ff50a67
accord sha256:15c083908a288e719c94c38e43c7f29399cb5f10f8dd92ab382db05d870f8257, card sha256:e367d5e182098bbb3a3ce960b3221aa2a1c0d6ed2fc790dfa4604e604191d6a5
mediator ed25519:9dbc0ab131eff751ed7df2dc99b25bc10dbad126a06299ee2e4a98ba88555b43, seq 1, at 2026-11-17T20:08:54Z
envelope of ed25519:2723fa2dfe6ac09de460059764706d3b49a9e0f09daf9f385c99de96f3532e1e, seq 1, hash sha256:7824be819c8bccfa48e81ea06607e726ac08ec6a15a7d2c2b64dc5b944082c10
```

## Where to start

The repository's layout and rules come first:

- `AGENTS.md`: where everything goes, and which rules to read before you change a part; `docs/rules/` holds every rule, by name.
- `CONTRIBUTING.md`: how a change is made and reviewed.
- `spec/`: the protocol placket runs, operation by operation, and the formats it signs. `spec/README.md` says where to start.
- `vectors/`: the conformance vectors and their coverage list. `vectors/README.md` says how a vector is written and checked.
- `docs/adr/`: the decisions behind the layout, the core, the door, the journal and the conformance runner.
- `core/`: the Go core, which touches nothing of the world; `adapters/` and `cmd/placket/`: the door, the journal, the files each party keeps, the log and the command line.

`make ci` runs every check.

## Contributing

Contributions are welcome, however they are made, with a person who answers for them. `CONTRIBUTING.md` says how.

## Security

Report a vulnerability privately, as `SECURITY.md` says. `docs/security/threat-model.md` says what placket protects and what it does not.

## Licence

Apache License 2.0, for the code and the requirement text: see `LICENSE`.
