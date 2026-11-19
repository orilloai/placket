# The local applier

A file on your machine is written only with what Placket released: the text a person in the deciding role accepted, or the version they changed it to. A declined proposal writes nothing. It is the applier of `placket init approve`, the first example of approve before apply.

## The question

"My agent wants to change a file. How do I make sure the file only ever holds text I said yes to, and show later what I said yes to?"

## When to use it

Use it when an agent proposes the whole text of one file, a person answers each proposal, and something on the person's own machine applies what they accepted. The agent gives a `proposal` to the applier through Placket, the terms ask the deciding role, and `apply.sh` writes the text released to it.

It is not the pattern for a change that leaves the machine, such as a merge on a forge: `github-merge` is. Nor does it edit part of a file: each release replaces the file's text with the text released. It is not a guard on the file either: anything else that can write the file still can, and Placket does not watch it.

## What it relies on

From `spec/protocol.md`:

- §3, Founding an Accord, setup-contract-2 and setup-contract-12 to setup-contract-15: each party seals its terms, the bindings name each key's roles in the clear, and the Accord is founded once every member accepts its own view. The applier reads the deciding role's keys from the bindings, since the terms that name the role are sealed.
- §4, Giving and getting a part at the door, crossing-3: a delivery holds its receipt id, its body and its salt.
- §6, Questions, the three answers and release, maybe-3, maybe-10, maybe-11 and maybe-12: an accept releases the proposal to the applier as it was sent; a change releases the person's version, of kind `changed-version`; a decline releases nothing. The answer call travels inside the delivery, as its `answer`, naming its signer and its action.
- §8, Verifying receipts, verify-1: `verify` with every opening checks that every decision is one the Accord let its decider make.
- §11, What the command line prints for a script, commands-3 and commands-8: `get --json` prints the delivery the applier handles, and `verify` prints the lines read at the end.

## What it claims

The pattern claims nothing a receipt does not. It leans on the claim line `placket show` prints for the receipt of an accepted or changed answer, and on the scope line that follows every claim line:

```
claim: a key holding the answering role answered. The part named here was judged and queued for the receiver this receipt names.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

That the file holds the text released is the applier's doing, not a receipt's: a receipt says what was released to the applier's key, and nothing about the file.

## What it does

`sh pattern.sh` runs it on invented text and prints `expected.txt`:

1. `placket init approve` founds the Accord, with an agent, a person in role `decider` and the applier, and the script starts its window with the line `init` printed.
2. The agent proposes a text, the person accepts it, and `apply.sh` writes it to `accepted.txt`.
3. The agent proposes another, the person changes it, and `apply.sh` writes the changed text to `changed.txt`.
4. The agent proposes a third, the person declines it, and `apply.sh` finds nothing released: `declined.txt` is never made.
5. The window stops, and `placket verify`, with every party's opening, prints `valid` and `conforms`.

## Making it yours

Copy `apply.sh` alone, or fetch it with the line `placket init approve` prints, and change its lines marked `# CHANGE`: the deciding role, as your terms name it, and `forever=yes`, to wait for releases until interrupted. Run it as `sh apply.sh LABEL FILE`, with the applier's label from the labels config.

`apply.sh` refuses, and writes nothing, for a delivery that is not a text of kind `proposal` or `changed-version` (`body`), and for one whose answer call is missing, is not an accept of a proposal or a change of a changed version, or is from a key the bindings do not give the deciding role (`unwitnessed`). `placket get` has refused a delivery whose answer call is not the one its receipt names; the call's signature rests on the mediator, and `placket verify` checks it against the journal.
