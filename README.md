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
./placket verify --mediator "$(cat scripts/demo/mediator.ref)" --receipts scripts/demo/journal --founding scripts/demo/founding.json
```

That journal's card names a kernel hash typed by hand before the binary could print its own, so `verify` follows its module line with `module 0.1.0 sha256:a8755bef… is not this build's decision module, 0.1.0 sha256:…`. The receipts still verify: the line says only that another build judged them.

## Found an Accord

An Accord is founded from five kinds of file: the policy, the mediator, the card, and each party's declaration and binding. These are the demo's founding steps, from `scripts/demo.sh`:

```
# Keys: the mediator, its operator, the principal who binds the parties, and each party.
for k in mediator operator principal proposer applier decider; do
	placket key new --out "$k.key" >"$k.ref"
done
# Each party signs its terms; the principal binds it to its role under this mediator.
for a in proposer applier decider; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat mediator.ref)" --until 2099-01-01T00:00:00Z >"$a.binding"
done
echo "{\"roles\":[\"proposer\",\"applier\",\"decider\"],\"principals\":[\"$(cat principal.ref)\"]}" >policy.json
echo "{\"actor\":\"$(cat mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator.json
# The card names the decision module this binary runs.
placket card --default >card.json
placket founding --policy policy.json --mediator mediator.json --card card.json \
	--declaration proposer.declaration --declaration applier.declaration --declaration decider.declaration \
	--binding proposer.binding --binding applier.binding --binding decider.binding >founding.json
placket window --key mediator.key --founding founding.json --socket /tmp/plk.s --journal journal
```

`placket card --default` prints the binary's own card on standard output, and on standard error its card hash and `module <version> <hash>`. The hash is taken from the source of the decision module, `core/composition/composition.go`, that the binary was built from, so a binary with another module prints another card (`docs/adr/0012-the-kernel-hash.md`). Never copy a card from another build:

- `placket founding` founds under any card that reads, since whoever founds an Accord need not run its window, but prints `module <version> <hash> is not this build's decision module, <version> <hash>` when the card is not its own.
- `placket window` refuses to run an Accord whose card names another module: it prints that line and `error founding kernel`, and starts nothing. After an upgrade that changes the module, found the Accord again with the new binary's card; the old journal still verifies.
- `placket verify --founding` prints that line after the module line, and still verifies.

Its first receipt, `seq` 1 of the proposer's chain to the applier, is the first proposal's ask:

```
{"payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJwcmVkaWNhdGUiOnsiYWNjb3JkIjoic2hhMjU2OjhhMDY0NmFmZTA4OTIzZjhiM2Y5Y2MxNWM2M2JjZTE4NjAxYzU3M2ExNjA4ZGMwYzViNDA5NGM5OTRmYjYwYTQiLCJhdCI6IjIwMjYtMTEtMTJUMjI6MzI6NThaIiwiY2FyZCI6InNoYTI1Njo2OTBlNWRiYjY3NzA0YjYwZmM0NzUxMmNkOGYwMjE0M2E3ODc2ZTg4NWNmODY1ZTRiZGU3OWVkN2ZiZTVjMzU2IiwiZW52ZWxvcGUiOnsiaGFzaCI6InNoYTI1Njo5OTI4MTY0ZDNmZWExYTI1MWJlMGU5MjFkYjZlOGNjNTIwZWZjMmFiZWZjMjZkMDU4ODcxOWU4MTFhMTk2ZDQxIiwic2VuZGVyIjoiZWQyNTUxOTo2ZmMzNjg0ZDFhMzBjMzViZWNmZGQxMDk1MjliZGUxM2IxNGYyYzU2NmRiMmUxOTIxMGU5OGM4NDJlYmU5NDI1Iiwic2VxIjoxfSwiZnJvbSI6ImVkMjU1MTk6NmZjMzY4NGQxYTMwYzM1YmVjZmRkMTA5NTI5YmRlMTNiMTRmMmM1NjZkYjJlMTkyMTBlOThjODQyZWJlOTQyNSIsImtpbmQiOiJjcm9zc2luZyIsIm1lZGlhdG9yIjoiZWQyNTUxOToyNzRmNmFmNWE1NzZhYjRkNTU0NGMzM2JiOGU0ZDRiMTE0OGFjODY1NGM0ZmUyNmMxYmEyY2NlNDAwY2MyN2ZkIiwicmVhc29uIjoibWF5YmUiLCJzZXEiOjEsInRvIjoiZWQyNTUxOTo2MDQyNmUzODY5NjNmZTZkMWY0NGNiZTAyMmFkNGJjMjRlNzc0YjBhZWI3Y2IyZGRhMzJhNGJkMWFlNmMzNTNlIiwidmVyZGljdCI6ImVzY2FsYXRlIn0sInByZWRpY2F0ZVR5cGUiOiJodHRwczovL3BsYWNrZXQuZGV2L3JlY2VpcHQvMC4xIiwic3ViamVjdCI6W3siZGlnZXN0Ijp7InNhbHRlZC1zaGEyNTYiOiI0MzhlYThjNjhjYjQxM2MzNjU0NjNjNzMxMTc4MGU1MjdjODIxOTIyNjMwYTZkZDUyMWIyMjI1ZmFmYjNhOWYyIn0sIm5hbWUiOiJwcm9wb3NhbCJ9XX0=","payloadType":"application/vnd.in-toto+json","signatures":[{"keyid":"ed25519:274f6af5a576ab4d5544c33bb8e4d4b1148ac8654c4fe26c1ba2cce400cc27fd","sig":"6n12Nx0bP3aUcPjQfH3ys9/t4XsUMTuPesn5uNBVwSxLZ1LqOc5x6eMN2XvE8aBR2bUVcKKn5rZAKELTycSJAQ=="}]}
```

and `placket show --receipt` prints, on standard error, then on standard output:

```
not verified
ask maybe proposal 2026-11-12T22:32:58Z
from ed25519:6fc3684d1a30c35becfdd109529bde13b14f2c566db2e19210e98c842ebe9425 to ed25519:60426e386963fe6d1f44cbe022ad4bc24e774b0aeb7cb2dda32a4bd1ae6c353e
module: named by card sha256:690e5dbb67704b60fc47512cd8f02143a7876e885cf865e4bde79ed7fbe5c356.
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
not verified: this receipt was decoded without checking its signature, its mediator or its chain
receipt sha256:395ba3b3285246850b8ad4d66c5889f637a3c360f95e908da8c8dbd9a2a603dd
crossing from ed25519:6fc3684d1a30c35becfdd109529bde13b14f2c566db2e19210e98c842ebe9425 to ed25519:60426e386963fe6d1f44cbe022ad4bc24e774b0aeb7cb2dda32a4bd1ae6c353e
verdict escalate, reason maybe
subject proposal, salted-sha256 438ea8c68cb413c365463c7311780e527c821922630a6dd521b2225fafb3a9f2
accord sha256:8a0646afe08923f8b3f9cc15c63bce18601c573a1608dc0c5b4094c994fb60a4, card sha256:690e5dbb67704b60fc47512cd8f02143a7876e885cf865e4bde79ed7fbe5c356
mediator ed25519:274f6af5a576ab4d5544c33bb8e4d4b1148ac8654c4fe26c1ba2cce400cc27fd, seq 1, at 2026-11-12T22:32:58Z
envelope of ed25519:6fc3684d1a30c35becfdd109529bde13b14f2c566db2e19210e98c842ebe9425, seq 1, hash sha256:9928164d3fea1a251be0e921db6e8cc520efc2abefc26d0588719e811a196d41
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
