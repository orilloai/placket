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
- `placket verify --founding` prints that line after the module line, and still verifies.

Its first receipt, `seq` 1 of the proposer's chain to the applier, is the first proposal's ask:

```
{"payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJwcmVkaWNhdGUiOnsiYWNjb3JkIjoic2hhMjU2OjdmMWM2ZWE2ZjgwMGU4OGQ2NzBhZGEzZTVkNmU0ZDI5OGMyOTc2M2YzNjE3Y2ZhYWJiODI4NmY4ZDI3ZWY0ZjciLCJhdCI6IjIwMjYtMTEtMTdUMDM6NTI6MTRaIiwiY2FyZCI6InNoYTI1Njo2OTBlNWRiYjY3NzA0YjYwZmM0NzUxMmNkOGYwMjE0M2E3ODc2ZTg4NWNmODY1ZTRiZGU3OWVkN2ZiZTVjMzU2IiwiZW52ZWxvcGUiOnsiaGFzaCI6InNoYTI1Njo3ZDZlMWRlYmQxOGNlMzNkMmQyMmVkZTMyMjBmMzdmYmM1Y2NiNTZmNzA0YWMyNDk2YjI5ZmQ0N2U2N2M0OTA2Iiwic2VuZGVyIjoiZWQyNTUxOTozZDYxMjQ5NTgwNWViNmFkYjFkYmI2YTFhMDc1OTJhYTUyYzVkNzFhMGU0ZWM3ZTFiYzczMjlhNGViMmMxMzc3Iiwic2VxIjoxfSwiZnJvbSI6ImVkMjU1MTk6M2Q2MTI0OTU4MDVlYjZhZGIxZGJiNmExYTA3NTkyYWE1MmM1ZDcxYTBlNGVjN2UxYmM3MzI5YTRlYjJjMTM3NyIsImtpbmQiOiJjcm9zc2luZyIsIm1lZGlhdG9yIjoiZWQyNTUxOToyNWE3ZmM4OTRmYmI0NzU1Yjk3Nzg1YzFlNDI3ZDNmMzNhY2IwZjU5NjhhZTYwZjdmZGY2YjY5ZmIxZjExOWMxIiwicmVhc29uIjoibWF5YmUiLCJzZXEiOjEsInRvIjoiZWQyNTUxOToyMjFiNzg0MDJhMjBiNmMyMjkxZmYzOWJiYzdkYmIyOTQzYzRiMTkzYTYxMTU3NDI1MzE2NTczNWI5MDU2ZTc4IiwidmVyZGljdCI6ImVzY2FsYXRlIn0sInByZWRpY2F0ZVR5cGUiOiJodHRwczovL3BsYWNrZXQuZGV2L3JlY2VpcHQvMC4xIiwic3ViamVjdCI6W3siZGlnZXN0Ijp7InNhbHRlZC1zaGEyNTYiOiJlZDBhOTVmZDQ1ZmUyMmU3ZWE4ZWM2NGFmOTY0ZWU5OTFjY2ZmOGIxMDNhZGVlMTIwYWRkNTcyZTk1MDVjZjI0In0sIm5hbWUiOiJwcm9wb3NhbCJ9XX0=","payloadType":"application/vnd.in-toto+json","signatures":[{"keyid":"ed25519:25a7fc894fbb4755b97785c1e427d3f33acb0f5968ae60f7fdf6b69fb1f119c1","sig":"zD+o8W30UH0xMvCADFmkFSrYZ2UZ4TQKGAeOzAczMKdJ0OFwzUps6av125OKDqUDVfpVeo3wClzQsl5F0EZ6Ag=="}]}
```

and `placket show --receipt` prints, on standard error, then on standard output:

```
not verified
ask maybe proposal 2026-11-17T03:52:14Z
from ed25519:3d612495805eb6adb1dbb6a1a07592aa52c5d71a0e4ec7e1bc7329a4eb2c1377 to ed25519:221b78402a20b6c2291ff39bbc7dbb2943c4b193a611574253165735b9056e78
module: named by card sha256:690e5dbb67704b60fc47512cd8f02143a7876e885cf865e4bde79ed7fbe5c356.
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
not verified: this receipt was decoded without checking its signature, its mediator or its chain
receipt sha256:435d8054381d30571321f54e2918fccfb9fbeb6812fe38546e452f8f9daae7b9
crossing from ed25519:3d612495805eb6adb1dbb6a1a07592aa52c5d71a0e4ec7e1bc7329a4eb2c1377 to ed25519:221b78402a20b6c2291ff39bbc7dbb2943c4b193a611574253165735b9056e78
verdict escalate, reason maybe
subject proposal, salted-sha256 ed0a95fd45fe22e7ea8ec64af964ee991ccff8b103adee120add572e9505cf24
accord sha256:7f1c6ea6f800e88d670ada3e5d6e4d298c29763f3617cfaabb8286f8d27ef4f7, card sha256:690e5dbb67704b60fc47512cd8f02143a7876e885cf865e4bde79ed7fbe5c356
mediator ed25519:25a7fc894fbb4755b97785c1e427d3f33acb0f5968ae60f7fdf6b69fb1f119c1, seq 1, at 2026-11-17T03:52:14Z
envelope of ed25519:3d612495805eb6adb1dbb6a1a07592aa52c5d71a0e4ec7e1bc7329a4eb2c1377, seq 1, hash sha256:7d6e1debd18ce33d2d22ede3220f37fbc5ccb56f704ac2496b29fd47e67c4906
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
