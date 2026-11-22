# placket

Placket mediates content crossing between parties, and records what crossed in receipts anyone can verify.

It sits at a handoff between parties that do not fully trust each other: agents, people, tools. Each party declares, and signs, the terms under which a part may cross to the other. For every part offered, placket decides whether it passes, is blocked, or must be asked about: then a named person, holding the role the terms name, accepts it, changes it or declines it. Placket signs a receipt for every decision, chained per pair of parties, and a third party verifies the receipts with the mediator's public key alone.

The first job is ask and log: one system proposes, another applies, and a named person is asked about every proposal. An accepted proposal is released to the applier exactly as it was sent; a changed one is released as the person's version and returned to the proposer; a declined one releases nothing. The deliverable is the signed log of what was asked, answered and released.

## Install

Placket is one binary, for Linux and macOS on amd64 and arm64. Any one of these lines installs it:

- **The install script**, which checks what it downloads, as [`docs/release.md`, "Install in one line"](docs/release.md#install-in-one-line) says, and puts `placket` in `~/.local/bin`:

  ```sh
  curl -fsSL https://github.com/orilloai/placket/releases/latest/download/install.sh | sh
  ```

  `curl | sh` runs the script before anything checks it. To check the script itself first, against the release's signed `checksums.txt`:

  ```sh
  base=https://github.com/orilloai/placket/releases/latest/download
  curl -fsSLO "$base/install.sh" -O "$base/checksums.txt" -O "$base/checksums.txt.sigstore.json"
  cosign verify-blob --bundle checksums.txt.sigstore.json \
    --certificate-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
  grep ' install.sh$' checksums.txt | shasum -a 256 -c
  sh install.sh
  ```

- **Homebrew**, on macOS or Linux: `brew install --cask orilloai/tap/placket`.
- **Claude Code**, through placket's plugin, whose skills install placket with your yes, set it up with you, and suggest where it could gate what your agents do: `/plugin marketplace add orilloai/placket`, then `/plugin install placket@placket`, then ask for `placket-setup`. Any other agent that reads `SKILL.md` files takes the same skills, as [`skills/README.md`](skills/README.md) says.

Or by hand:

1. Download the release archive for your platform, `placket_<version>_<os>_<arch>.tar.gz`, from the release's page, with `checksums.txt` and its signature beside it.
2. Check it, as [`docs/release.md`, "For users"](docs/release.md#for-users) says: the signature, the checksum and the provenance.
3. Unpack it, and put `placket` on your `PATH`:

   ```sh
   tar -xzf placket_0.1.0_linux_amd64.tar.gz placket
   mkdir -p ~/.local/bin && mv placket ~/.local/bin/
   placket version
   ```

## Quickstart

Each of placket's three uses starts with one `placket init`, which founds its example Accord for you alone and prints every command that comes next. Each guide takes it from there to a receipt you verify, with no judge's key.

1. **Approve before apply**, the one to start with: an agent proposes the text of a file, you accept, change or decline it, and an applier writes only what you accepted. `placket init approve`, then [`docs/guides/approve.md`](docs/guides/approve.md). As `init` sets it up, every key is in your own account, so the gate records and checks each decision but does not hold against an agent that can run commands as you; for that, the agent runs in an account of its own, as [the guide says](docs/guides/approve.md#what-this-setup-does-not-stop).
2. **An out-gate on a merge**: an agent proposes a merge on GitHub, you answer it, and an applier merges only what you accepted, at the head commit you accepted. `placket init merge`, then [`docs/guides/merge.md`](docs/guides/merge.md).
3. **Handoffs between agents**: a planner hands a plan to a coder, and the coder its work to a reviewer, across two Accords, each with a window of its own and both under one mediator key, each handoff with a receipt. `placket init handoff`, then [`docs/guides/handoff.md`](docs/guides/handoff.md).

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

It prints one line per receipt of the party's own, such as `view of <key>: receipts.jsonl line 1 receipt <id> matches give proposal to <key>: ask`, and then `view of <key>: every receipt of its own, 13, matches the view it accepted`. A receipt that does not match exits 1 and names its line; a view that is not the one the party accepted is `error founding acceptance-view`. It does not read the closings a party signs as an answerer on questions it neither asked nor received, nor crossings between two other parties. A view holds its party's own seal salt, the one its seal in the founding set is made with, so a party that hands its view over, to a verifier or anyone else, discloses its seal salt with it: whoever holds the salt can test a guess at that party's declaration against its seal.

## Found an Accord

An Accord is founded from five kinds of file: the policy, the mediator, the card, and each party's sealed declaration and binding. Each party seals its own declaration: the founding set holds the seal, and the opening, which holds the terms, goes to whoever runs the window and to no other party. Then each party is shown its own view of the Accord, what will cross for it, and signs it; the Accord is founded when every party has. For one person who holds every key, `placket init` does all of this in one command (Quickstart). Between parties who each hold their own, these are the steps, the demo's, from `scripts/demo.sh`:

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

The first receipt of the committed demo journal, line 1 of `scripts/demo/journal/receipts.jsonl`, is `seq` 1 of the proposer's chain to the applier, the first proposal's ask:

```
{"payload":"eyJfdHlwZSI6Imh0dHBzOi8vaW4tdG90by5pby9TdGF0ZW1lbnQvdjEiLCJwcmVkaWNhdGUiOnsiYWNjb3JkIjoic2hhMjU2OjA1YzlkMGU0M2NiODA3YTI3ODIwNTQyNWJmMjgzMTQ1ZTI4NDE5ZjM4MGM3ZmJhY2M5ZDc0ZjY3NGZjZDQzMTIiLCJhdCI6IjIwMjYtMTEtMThUMTc6Mjg6MDFaIiwiYnkiOiJ0ZXJtcyIsImNhcmQiOiJzaGEyNTY6ZTM2N2Q1ZTE4MjA5OGJiYjNhM2NlOTYwYjMyMjFhYTJhMWMwZDZlZDJmYzc5MGRmYTQ2MDRlNjA0MTkxZDZhNSIsImVudmVsb3BlIjp7Imhhc2giOiJzaGEyNTY6NmMxYmE4YTI1MWNhNGQ1YzMyOGUyY2Q2ODI2MjQ3OGE5YmViNzBhYzk0NDU0MGViYTAyN2M5M2FkYjg1NzMxYiIsInNlbmRlciI6ImVkMjU1MTk6NzBkZWU1OGZhMTlhMTc3OTVlOWU0ZjM5MjkwNmYwY2JhZTE5NDE0YzAxNTBkYjk3NGUyYjEwYTljMzFkMjA0ZCIsInNlcSI6MX0sImZyb20iOiJlZDI1NTE5OjcwZGVlNThmYTE5YTE3Nzk1ZTllNGYzOTI5MDZmMGNiYWUxOTQxNGMwMTUwZGI5NzRlMmIxMGE5YzMxZDIwNGQiLCJraW5kIjoiY3Jvc3NpbmciLCJtZWRpYXRvciI6ImVkMjU1MTk6Mzg3NmRkZGM1ZDUzZjcwZjE0YmUyYTk5NmRmZmJlMzU4NGQ3MmU4ZDJhMDExMTQ3NDgzMDg1N2E1NWNhMmQyMyIsInJlYXNvbiI6Im1heWJlIiwic2VxIjoxLCJ0byI6ImVkMjU1MTk6MGI1MzE2MWM3MWJlMTEyZTM4NjE3MDdmMzUwMWZhYTNhM2JjMDJkOTk3NWMxN2I3ZjA3MzQwOTc5Mjg1NDEzZiIsInZlcmRpY3QiOiJlc2NhbGF0ZSJ9LCJwcmVkaWNhdGVUeXBlIjoiaHR0cHM6Ly9wbGFja2V0LnRlY2gvcmVjZWlwdC8wLjEiLCJzdWJqZWN0IjpbeyJkaWdlc3QiOnsic2FsdGVkLXNoYTI1NiI6IjFhMGZiMTcwMTQxYzZmZTI0NzcwYTQ4Y2I0YjQ5MjMxMWNhNmM0Y2Y0MDBiODNiZTAyMmQ5NDk2YTIwYTE2ZDcifSwibmFtZSI6InByb3Bvc2FsIn1dfQ==","payloadType":"application/vnd.in-toto+json","signatures":[{"keyid":"ed25519:3876dddc5d53f70f14be2a996dffbe3584d72e8d2a0111474830857a55ca2d23","sig":"emfBC4s1Koswx9+uZiCEn3taVDvedYKdi3/R/Usau2y0T1Wva17bvtYvqNazNILcfxqp4miwQfT2Dd9XW7vdAw=="}]}
```

and `placket show --receipt` prints, on standard error, then on standard output:

```
not verified
ask maybe proposal 2026-11-18T17:28:01Z
from ed25519:70dee58fa19a17795e9e4f392906f0cbae19414c0150db974e2b10a9c31d204d to ed25519:0b53161c71be112e3861707f3501faa3a3bc02d9975c17b7f07340979285413f
module: named by card sha256:e367d5e182098bbb3a3ce960b3221aa2a1c0d6ed2fc790dfa4604e604191d6a5.
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
not verified: this receipt was decoded without checking its signature, its mediator or its chain
receipt sha256:bc209e7137c6c452d01f06a983f1af942b97df6f436d0b0ed3a98ce3b8edd0e8
crossing from ed25519:70dee58fa19a17795e9e4f392906f0cbae19414c0150db974e2b10a9c31d204d to ed25519:0b53161c71be112e3861707f3501faa3a3bc02d9975c17b7f07340979285413f
verdict escalate, reason maybe
by terms
subject proposal, salted-sha256 1a0fb170141c6fe24770a48cb4b492311ca6c4cf400b83be022d9496a20a16d7
accord sha256:05c9d0e43cb807a278205425bf283145e28419f380c7fbacc9d74f674fcd4312, card sha256:e367d5e182098bbb3a3ce960b3221aa2a1c0d6ed2fc790dfa4604e604191d6a5
mediator ed25519:3876dddc5d53f70f14be2a996dffbe3584d72e8d2a0111474830857a55ca2d23, seq 1, at 2026-11-18T17:28:01Z
envelope of ed25519:70dee58fa19a17795e9e4f392906f0cbae19414c0150db974e2b10a9c31d204d, seq 1, hash sha256:6c1ba8a251ca4d5c328e2cd68262478a9beb70ac944540eba027c93adb85731b
```

## Judges

A `maybe` term may list judges, each to `decide` or to `advise`, where the terms would ask a person. When both sides' matching terms list the same judges, the window asks the `decide` judges in order, within 20 seconds for them all: the first pass or block decides the part, and when every judge says none the part is asked as before. An `advise` judge runs after the receipt is written, and decides nothing. A judge never answers a question and never runs at release. In v0.1.0 the one judge provider is Jev, called with your own key.

```
placket card --default --judge judge.json >card.json
# Type the key: it is not echoed, and it stays out of the shell's history.
stty -echo; read -r PLACKET_JEV_KEY; stty echo; export PLACKET_JEV_KEY
placket window --key mediator.key --founding founding.json --openings openings --socket /tmp/plk.s --journal journal --judge judge.json
```

`judge.json` is the judge's config: its name, provider, host, model, threshold and instruction. Its hash goes into the card, and the window runs only the configs its card lists. The key is read once from `PLACKET_JEV_KEY` and written nowhere; at start the window prints which kinds may go to which host, or that a judge with no key sends nothing. A request goes to the path `/v1/systemone` on the config's host. It carries the part's kind, its body and the config's instruction and model, and the adapter's fixed words: one question, `verdict`, of type `choice`, between two criteria, `pass`, "The content meets the instruction.", and `block`, "The content does not meet the instruction." Nothing else of the part goes, and a raw body is never sent. What each judge said goes in the journal's `judged/` directory, never in a receipt. `patterns/shadow-judge/` runs a judge to advise, with no key; `docs/security/threat-model.md` says what a judge can and cannot be trusted with.

## Where to start

The repository's layout and rules come first:

- `AGENTS.md`: where everything goes, and which rules to read before you change a part; `docs/rules/` holds every rule, by name.
- `CONTRIBUTING.md`: how a change is made and reviewed.
- `spec/`: the protocol placket runs, operation by operation, and the formats it signs. `spec/README.md` says where to start.
- `vectors/`: the conformance vectors and their coverage list. `vectors/README.md` says how a vector is written and checked.
- `docs/README.md`: the docs, in order: quickstarts, concepts, guides, the references to the command line, the mediator API and the protocol, and how to verify a journal.
- `docs/adr/`: the decisions behind the layout, the core, the door, the journal and the conformance runner.
- `core/`: the Go core, which touches nothing of the world; `adapters/` and `cmd/placket/`: the door, the journal, the files each party keeps, the log and the command line.

`make ci` runs every check.

## Contributing

Contributions are welcome, however they are made, with a person who answers for them. `CONTRIBUTING.md` says how.

## Security

Report a vulnerability privately, as `SECURITY.md` says. `docs/security/threat-model.md` says what placket protects and what it does not.

## Licence

Apache License 2.0, for the code and the requirement text: see `LICENSE`.
