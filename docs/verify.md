# Verify a journal

`placket verify` checks a journal's receipts: that the mediator signed every one, that they chain, and, with the Accord's founding set, that every decision is one the Accord let its decider make. It never runs a judge again, never fetches anything and never needs a body. Its usage and flags are in [the command line reference](reference/cli.md#verify); what it prints is commands-8 of [the protocol](../spec/protocol.md#commands-8-what-verify-prints), and what it is given is verify-1 ([what a verifier is given](../spec/protocol.md#verify-1-what-a-verifier-is-given)).

## What to give it

The one value it needs out of band is the Accord's id. With it, the founding set whose typed hash it is, and the acceptances beside it, it takes the mediator's key from the set, never from a receipt:

```sh
placket verify --accord ID --receipts journal --founding founding.json
```

Add each party's opening with `--open`, as in a dispute, to check every decision against the terms; `--view` with one member's own view, to check that member's receipts against the view it accepted; `--delivery`, to check one delivery's body against its receipt; `--subset`, for a part of the journal; and `--superseded-by`, with `--successor-accord` and `--successor-acceptances`, to check that a successor pins this journal's close. The successor's id comes from your own record, the id you accepted, never from a file sent with the journal, and the successor is founded with its members' acceptances at the time of the close, so a successor its members did not accept pins nothing ([rotating a mediator's key](guides/rotation.md)). With `--mediator` and no founding set, it checks validity alone.

## What each line says

Every line goes to standard error, in this order; each receipt's predicate goes to standard output. With `--json`, the object holds each line under its own member.

1. **The Accord.** `mediator <key> accord <id> card <hash>`: the key the receipts were checked under, and the Accord and card they name.
2. **Who runs it and who stands behind each member**, with a founding set. `operator <key> runs process`; `module <version> <hash>`, the decision module the card names, and a line after it when that is not this build's module; `judge <name> <version> <hash>` for each judge of the card; `member <key> roles <roles> principal <key>` for each member; `custody: operator <key> is principal of <n> of <m> members`; and `supersedes accord <id> close <id>` when the policy replaces an earlier Accord. Without a founding set: `operator and module not shown: give --founding`.
3. **The sealed terms.** `seal <key>: opens: ...` for each opening given; `view <key>: matches its acceptance` for each member, once every opening is given, or `view <key>: not checked: ...` for an opened member otherwise; then `terms: checked: ...`, or `terms: not checked: the declarations are sealed; ...`.
4. **The counts.** `pass <n> block <n> ask <n>`, the receipts by product word; `questions closed <n> open <n>`; `releases <n> blocked releases <n>`.
5. **A subset's late chains.** `chain <key> to <key> not checked before seq <n>`, for each chain a subset starts after its first receipt.
6. **Each decision the terms did not make**, with a founding set, one line per receipt in journal order:
   - `<id> witnessed: signed by key <key>, which principal <key> bound to role <role>`: an answer the call given shows that key signed (verify-3);
   - `<id> unwitnessed: only the mediator says a key holding role <role> answered`, or `... says the asker cancelled`: an answer with no call given; it is never a failure;
   - `<id> judge: the mediator says judge <name> of the card decided`;
   - `<id> mediator: the deadline passed`, `... nobody could be reached to answer`, or `... the Accord was closed`.
7. **The rest.** `<n> decisions not checked: their question is not in the subset`; `terms decided <n>`; `answer calls matching no receipt <n>`, when any.
8. **Validity.** `valid: <n> receipts signed by mediator <key>`: every receipt is the mediator's, well formed, and chained. A receipt that fails validity ends the run there with `error verify`, its code, `receipts.jsonl` and its line, exit 3, and none of the lines above.
9. **Conformance.** `conforms: every decision is one this Accord let its decider make`, with every opening; with some seals unopened, `conforms: every receipt names this Accord, its card and its members, and every answer call given fits; decisions checked against the terms only where the seals they read are opened`; `conformance not checked: give --founding`, or `... the card's module is not this build's`; or `does not conform: <code> at receipts.jsonl line <n>`, exit 1, with every other line still printed (verify-2).
10. **A member's own receipts**, with `--view`. One line per receipt of the viewer's, `view of <key>: receipts.jsonl line <n> receipt <id> matches <row>`, or `does not match`, and then `view of <key>: every receipt of its own, <n>, matches the view it accepted`, or that it does not, at its line, exit 1.
11. **Completeness.** `complete: not claimed: this Accord has no close receipt`, or `... only a subset was given`; or `complete: against close receipt <id>; not claimed: that the mediator signed no other log for this Accord`, and with `--superseded-by`, the accepted successor, named by the id you gave, that pins it (verify-4, close-4).
12. **The body**, with `--delivery`. `body: the part of receipt <id> is the one it names`, or `body: does not conform at receipts.jsonl line <n>`, exit 1.
13. **The claim and its scope.** `claim: each receipt is this mediator's signed decision about one part under this Accord's terms.`, then the scope line.

## What it does not claim

The scope line says it, after every claim:

```
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

Besides it: without a close, a journal cut short after any receipt still verifies, and even against a close, `verify` never claims that the mediator's key signed no other journal for the Accord. A party that kept its own records checks the journal against them with `placket reconcile`. `verify` does not show that the mediator is honest or its clock true, or that a judge was right ([what placket does not do](concepts/limits.md)).
