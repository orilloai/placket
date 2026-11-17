# The guarded export

A receiver exports a value only when exactly that value was released to it, and refuses anything else.

## The question

"I hold the current value. Was exactly this value released to me, so I may export it?"

## When to use it

Use it when a receiver publishes, loads or sends on values that came to it through an approval, and the export step must not run on a value nobody approved: one typed in by hand, one changed after it arrived, one that was proposed but declined, or one the approver changed before it crossed.

The guard holds only the value it is about to export. It needs no receipt id, no delivery and no file the fetch wrote: it asks the receiver's state directory, which the fetch kept.

It is not the pattern for a value that must be the latest one released. Placket keeps no notion of a current release: a value released and later replaced still matches its own receipt, so the guard alone lets it through. A caller that needs "only the latest" adds its own rule on top. Step 6 shows one: `released --every` lists every release of the same record in fetch order, and the caller refuses a match that is not the last. Which field identifies "the same record", and what "latest" means, is the caller's to decide.

It does not normalize for you. Placket compares bytes. The proposer normalizes each record before it crosses, and the guard normalizes the value it holds by the same rule before it asks. A value spelled another way matches only when the rule turns it into the same bytes.

## What it relies on

From `spec/protocol.md`:

- §3, Founding an Accord, setup-contract-2 and setup-contract-3: each party's binding is signed by its own principal and gives only the roles the policy lets that principal grant. Here one principal binds the proposer and the approver, and another binds the receiver.
- §6, Questions, the three answers and release: a proposal the terms make a question crosses only when the approver accepts it, or changes it, with release; a decline blocks it.
- §11, commands-3, What a command makes or fetches: `get --all --out --json` fetches every delivery to files and lists them.
- §11, commands-5, A file checked against what crossed: the guard's check, `released --file F --state DIR`. Bytes match only when a receipt that passed, addressed to the receiver, names their salted digest.
- §11, commands-6, A value checked inside what crossed: `--at` and `--value` read a value inside a part by its JSON Pointer, as step 6 does for a record's `sku`; `--at` and `--value-file` are the guard's check when the value sits inside a part's body (Making it yours).
- §11, commands-7, Everything a key kept, checked against what crossed: `released --state DIR --every`, without `--file`, lists every part the receiver kept that matches, in fetch order. Any release that matches is a match, not only the latest one.
- §12, The log of a journal: `log --all` has a row for every crossing, built from receipts that verified.
- §8, Verifying receipts: `verify` checks every receipt in the journal under the mediator's key.

## What it claims

The guard leans on the claim `released` prints on a match, and on the scope line that follows every claim line:

```
claim: these bytes, under the salt they were fetched with, are the part this receipt let cross to this key.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

With `--at` and `--every` (step 6):

```
claim: for each receipt shown, these bytes are the value at this path inside a part this key kept, the part that receipt let cross to this key.
```

The log and `verify` print:

```
claim: each receipt is this mediator's signed decision about one part under this Accord's terms.
```

`released` reads the receipts as the receiver's state directory holds them; it says `not verified`. `verify` on the journal is what shows the receipts are the mediator's. Nothing here says what the receiver did with a value after it crossed, or that the value the guard checked is the one the export step then wrote: the guard and the export must read the same bytes.

## What it does

`sh pattern.sh` runs it on invented price records, in seven steps, and prints `expected.txt`:

1. Makes keys, terms and a founding set for a proposer, an approver and a receiver. The proposer's proposals to the receiver are a question for the approver.
2. Starts a window on a socket under `$TMPDIR`.
3. The proposer normalizes three records and gives each one. The approver accepts the first, changes the second with release, and declines the third.
4. The receiver fetches every delivery to `inbox/` with `get --all --out inbox --json --keep`.
5. The guard checks six values before an export step: the accepted record, spelled another way, and the changed version are exported; the record as first proposed, the declined one, the accepted one with one byte changed, and one never proposed are refused.
6. A new price for the first record is accepted. The old record still matches its own receipt, and the guard exports it; `released --every` shows both releases, and a rule of the caller's own refuses the one that is not the latest.
7. `log --all` prints a row for every crossing, and `verify` passes on the journal.

## Making it yours

Change the lines marked `# CHANGE`: the normalization, who binds whom, the bindings' lifetime, the records, where the value sits inside a part, the export step, and the rule for "only the latest". Keep the normalization the same on both sides of the crossing, and check the value the export step is about to write, not a copy of it.

Here each record is a whole part, so `at` is empty and the guard asks `released --state DIR --file V`. When your part wraps the value in a body, such as `{"summary":...,"topic":...,"payload":...}` with the value at `/payload`, set `at=/payload`: the guard then asks `released --state DIR --at /payload --value-file V`, which matches when V's bytes are exactly the value at that path inside a part the receiver kept (§11, commands-6 and commands-7). The guard writes V without the newline after it, since a value inside a body has none. The normalization then applies to the value alone, and the proposer must have put the value inside the body in those same bytes.
