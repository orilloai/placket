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

`make demo` runs the worked example, `scripts/demo.sh`, with Go 1.27 on Linux or macOS: it builds the `placket` binary, makes the keys of a mediator, an operator, a principal and three parties (a proposer, an applier and a decider), signs their terms and bindings, seals each party's terms, founds the Accord from the seals, and starts a window, to which each party consents by accepting its own view. Then the proposer asks three proposals and waits on each; the decider accepts the first, changes the second with a comment, and declines the third with a comment, each from its own commands; the applier fetches what was released. The proposer leaves a fourth proposal open and is refused when it tries to answer it itself, and gives a part of a kind no term names, which is blocked. The window is killed and started again: the open proposal is closed as undeliverable, and the applier fetches the changed version it had not fetched yet. Last, a third party verifies the journal with the terms sealed, the operator verifies it again with every opening, each party checks its own receipts against the view it accepted, a third party reconciles each party's records with it and exports the log, four rows; then changes one character of a receipt's signature, and verification fails. `sh scripts/demo.sh DIR` keeps everything it made in `DIR`.

The journal of one run is committed under `scripts/demo/`, with the founding set, its `acceptances.json`, the mediator's public reference and, for the demo only, every party's opening, and no key. Anyone can verify it:

```
go build -o placket ./cmd/placket
./placket verify --accord "$(sed -n 's/^{"accord":"\(sha256:[0-9a-f]*\)".*/\1/p' scripts/demo/journal/accord)" --receipts scripts/demo/journal --founding scripts/demo/founding.json
```

Given only the Accord's id, its founding set and the acceptances beside it, it checks what every party signed: that every receipt is the mediator's and names this Accord, its card and its members, and that every member accepted its view. The terms behind each seal stay sealed, so it prints `valid: 13 receipts signed by mediator ...` and `terms: not checked: the declarations are sealed; with every member's opening, --open checks them`, and the journal's `answers.jsonl` shows each answer witnessed by the key that signed it. With every opening, as in a dispute, it checks each seal, each view and every decision:

```
./placket verify --accord "$(sed -n 's/^{"accord":"\(sha256:[0-9a-f]*\)".*/\1/p' scripts/demo/journal/accord)" --receipts scripts/demo/journal --founding scripts/demo/founding.json $(for o in scripts/demo/openings/*; do printf ' --open %s' "$o"; done)
```

It prints `terms: checked: every seal opens, and every view is the one its member accepted` and `conforms: every decision is one this Accord let its decider make`.

A party also checks its own receipts against the view it accepted, with no other party's terms: every give it sends or receives, and the closing and release of every question it asked or received, against the row of its view that names it. In the demo each party runs, with the view `placket view` kept in its state directory:

```
placket verify --accord "$accord" --receipts journal --founding founding.json --view "proposer.key.state/${accord#sha256:}/view.json"
```

It prints one line per receipt of the party's own, such as `view of <key>: receipts.jsonl line 1 receipt <id> matches give proposal to <key>: ask`, and then `view of <key>: every receipt of its own, 13, matches the view it accepted`. A receipt that does not match exits 1 and names its line; a view that is not the one the party accepted is `error founding acceptance-view`. It does not read the closings a party signs as an answerer on questions it neither asked nor received, nor crossings between two other parties.

A receiver that holds only a delivery, as `placket get --json` saved it, and the founding set checks that delivery without the journal:

```
placket verify --accord "$accord" --founding founding.json --delivery delivery.json
```

For a release, it prints `<id> signed, its question not checked: key <key>, which principal <key> bound to role decider, signed the call`, and the body line. The receipt is the mediator's, the body is the part it names, and the deciding key signed the call that the receipt names. Which question that call answered, and for an accept which text it signed, rest on the mediator. So a mediator could attach an earlier accept by the deciding key to a release of another part, and the delivery would still check. An applier keeps the call id of each release it acts on, the receipt's `by.answer`, and refuses a delivery whose call id it already kept; a call it never saw can still be spent once (decision record 0027).

## Found an Accord

An Accord is founded from five kinds of file: the policy, the mediator, the card, and each party's sealed declaration and binding. Each party seals its own declaration: the founding set holds the seal, and the opening, which holds the terms, goes to whoever runs the window and to no other party. Then each party is shown its own view of the Accord, what will cross for it, and signs it; the Accord is founded when every party has. These are the demo's founding steps, from `scripts/demo.sh`:

```
# Keys: the mediator, its operator, the principal who binds the parties, and each party.
for k in mediator operator principal proposer applier decider; do
	placket key new --out "$k.key" >"$k.ref"
done
# The policy: the roles that must be held, and each principal with the roles it may grant. Here one
# principal stands behind all three parties.
echo "{\"roles\":[\"proposer\",\"applier\",\"decider\"],\"principals\":[{\"principal\":\"$(cat principal.ref)\",\"roles\":[\"proposer\",\"applier\",\"decider\"]}]}" >policy.json
# Each party signs its terms and seals them; the opening goes to the window's operator alone. Its
# principal binds it to its role under this mediator and this policy.
mkdir -p openings
for a in proposer applier decider; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket seal --key "$a.key" --declaration "$a.declaration" --out openings >"$a.seal"
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat mediator.ref)" --until 2099-01-01T00:00:00Z \
		--policy policy.json >"$a.binding"
done
echo "{\"actor\":\"$(cat mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator.json
# The card names the decision module this binary runs.
placket card --default >card.json
placket founding --policy policy.json --mediator mediator.json --card card.json \
	--seal proposer.seal --seal applier.seal --seal decider.seal \
	--binding proposer.binding --binding applier.binding --binding decider.binding >founding.json 2>founding.out
placket window --key mediator.key --founding founding.json --openings openings --socket /tmp/plk.s --journal journal &
# The Accord is proposed: each party views its own view and accepts it. The last acceptance founds
# it, and the window writes acceptances.json beside founding.json.
for a in proposer applier decider; do
	placket view --key "$a.key" --socket /tmp/plk.s --accord "$(sed -n 's/^accord //p' founding.out)"
	placket accept --key "$a.key" --socket /tmp/plk.s --accord "$(sed -n 's/^accord //p' founding.out)"
done
```

Each party's binding is signed by that party's own principal, under the policy it names by hash, and gives only roles the policy lets that principal grant. One principal may stand behind every party, as here, or each side may have its own, as in the guarded-export pattern (`patterns/guarded-export/`); `verify --founding` names each member's roles and principal.

`placket card --default` prints the binary's own card on standard output, and on standard error its card hash and `module <version> <hash>`. The hash is taken from the source of the decision module, `core/composition/composition.go`, that the binary was built from, so a binary with another module prints another card (`docs/adr/0012-the-kernel-hash.md`). Never copy a card from another build:

- `placket founding` founds under any card that reads, since whoever founds an Accord need not run its window, but prints `module <version> <hash> is not this build's decision module, <version> <hash>` when the card is not its own.
- `placket window` refuses to run an Accord whose card names another module: it prints that line and `error founding kernel`, and starts nothing. After an upgrade that changes the module, found the Accord again with the new binary's card; the old journal still verifies.
- `placket verify --founding` prints that line after the module line, still verifies, and says that conformance was not checked, since this build cannot decide again as another module would.

Its first receipt, `seq` 1 of the proposer's chain to the applier, is the first proposal's ask:

```
{"payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJwcmVkaWNhdGUiOnsiYWNjb3JkIjoic2hhMjU2OjA1YzlkMGU0M2NiODA3YTI3ODIwNTQyNWJmMjgzMTQ1ZTI4NDE5ZjM4MGM3ZmJhY2M5ZDc0ZjY3NGZjZDQzMTIiLCJhdCI6IjIwMjYtMTEtMThUMTc6Mjg6MDFaIiwiYnkiOiJ0ZXJtcyIsImNhcmQiOiJzaGEyNTY6ZTM2N2Q1ZTE4MjA5OGJiYjNhM2NlOTYwYjMyMjFhYTJhMWMwZDZlZDJmYzc5MGRmYTQ2MDRlNjA0MTkxZDZhNSIsImVudmVsb3BlIjp7Imhhc2giOiJzaGEyNTY6NmMxYmE4YTI1MWNhNGQ1YzMyOGUyY2Q2ODI2MjQ3OGE5YmViNzBhYzk0NDU0MGViYTAyN2M5M2FkYjg1NzMxYiIsInNlbmRlciI6ImVkMjU1MTk6NzBkZWU1OGZhMTlhMTc3OTVlOWU0ZjM5MjkwNmYwY2JhZTE5NDE0YzAxNTBkYjk3NGUyYjEwYTljMzFkMjA0ZCIsInNlcSI6MX0sImZyb20iOiJlZDI1NTE5OjcwZGVlNThmYTE5YTE3Nzk1ZTllNGYzOTI5MDZmMGNiYWUxOTQxNGMwMTUwZGI5NzRlMmIxMGE5YzMxZDIwNGQiLCJraW5kIjoiY3Jvc3NpbmciLCJtZWRpYXRvciI6ImVkMjU1MTk6Mzg3NmRkZGM1ZDUzZjcwZjE0YmUyYTk5NmRmZmJlMzU4NGQ3MmU4ZDJhMDExMTQ3NDgzMDg1N2E1NWNhMmQyMyIsInJlYXNvbiI6Im1heWJlIiwic2VxIjoxLCJ0byI6ImVkMjU1MTk6MGI1MzE2MWM3MWJlMTEyZTM4NjE3MDdmMzUwMWZhYTNhM2JjMDJkOTk3NWMxN2I3ZjA3MzQwOTc5Mjg1NDEzZiIsInZlcmRpY3QiOiJlc2NhbGF0ZSJ9LCJwcmVkaWNhdGVUeXBlIjoiaHR0cHM6Ly9wbGFja2V0LnRlY2gvcmVjZWlwdC8wLjEiLCJzdWJqZWN0IjpbeyJkaWdlc3QiOnsic2FsdGVkLXNoYTI1NiI6IjFhMGZiMTcwMTQxYzZmZTI0NzcwYTQ4Y2I0YjQ5MjMxMWNhNmM0Y2Y0MDBiODNiZTAyMmQ5NDk2YTIwYTE2ZDcifSwibmFtZSI6InByb3Bvc2FsIn1dfQ==","payloadType":"application/vnd.in-toto+json","signatures":[{"keyid":"ed25519:3876dddc5d53f70f14be2a996dffbe3584d72e8d2a0111474830857a55ca2d23","sig":"emfBC4s1Koswx9+uZiCEn3taVDvedYKdi3/R/Usau2y0T1Wva17bvtYvqNazNILcfxqp4miwQfT2Dd9XW7vdAw=="}]}
```

and `placket show --receipt` prints, on standard error, then on standard output:

```
not verified
ask maybe proposal 2026-11-17T23:16:47Z
from ed25519:09a2c472e9fa29ae1ae30e50e4ad1d571b29b560fa2e421a3757e5777b5f1882 to ed25519:8e7f0e0d4ba3528921802a367c7052acaedc6f3a3dfab36fd1d8bbee4a5a54c4
module: named by card sha256:e367d5e182098bbb3a3ce960b3221aa2a1c0d6ed2fc790dfa4604e604191d6a5.
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
not verified: this receipt was decoded without checking its signature, its mediator or its chain
receipt sha256:4925f6a58adb9bed2f9348a95c68d6d2860b67836dfc52d0793b25009129b73f
crossing from ed25519:09a2c472e9fa29ae1ae30e50e4ad1d571b29b560fa2e421a3757e5777b5f1882 to ed25519:8e7f0e0d4ba3528921802a367c7052acaedc6f3a3dfab36fd1d8bbee4a5a54c4
verdict escalate, reason maybe
by terms
subject proposal, salted-sha256 a0e94f005108aeb4a4a73830846f1219beb079f07f698557aaef8204087f3e78
accord sha256:bd42073727815c781d8e4ed9dad0f4623fca33452e80a646186d5b34bda6ca3e, card sha256:e367d5e182098bbb3a3ce960b3221aa2a1c0d6ed2fc790dfa4604e604191d6a5
mediator ed25519:45ab60359f021810335f6d356444bde8b6dfc42e1ef8210e524f0b10d9e511d7, seq 1, at 2026-11-17T23:16:47Z
envelope of ed25519:09a2c472e9fa29ae1ae30e50e4ad1d571b29b560fa2e421a3757e5777b5f1882, seq 1, hash sha256:0ee8df76c9eee8455da37f9866e4ec766307c9421442c7cfd2f30bdda776bb71
```

## Judges

A `maybe` term may list judges, each to `decide` or to `advise`, where the terms would ask a person. When both sides' matching terms list the same judges, the window asks the `decide` judges in order, within 20 seconds for them all: the first pass or block decides the part, and when every judge says none the part is asked as before. An `advise` judge runs after the receipt is written, and decides nothing. A judge never answers a question and never runs at release. In v0.1.0 the one judge provider is Jev, called with your own key.

```
placket card --default --judge judge.json >card.json
PLACKET_JEV_KEY=... placket window --key mediator.key --founding founding.json --socket /tmp/plk.s --journal journal --judge judge.json
```

`judge.json` is the judge's config: its name, provider, host, model, threshold and instruction. Its hash goes into the card, and the window runs only the configs its card lists. The key is read once from `PLACKET_JEV_KEY` and written nowhere; at start the window prints which kinds may go to which host, or that a judge with no key sends nothing. A request carries the part's kind, its body and the instruction, and nothing else; a raw body is never sent. What each judge said goes in the journal's `judged/` directory, never in a receipt. `patterns/shadow-judge/` runs a judge to advise, with no key; `docs/security/threat-model.md` says what a judge can and cannot be trusted with.

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
