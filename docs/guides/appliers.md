# Writing an applier

An applier is the party that acts on what placket released: it writes the file, merges the pull request, sends the message. Placket itself runs no command; the applier is yours, and it is the one place where a release becomes an action. This guide says what an applier must check, after [`patterns/local-apply/apply.sh`](../../patterns/local-apply/apply.sh) and [`patterns/github-merge/apply.sh`](../../patterns/github-merge/apply.sh), two short ones to start from.

## It is a member, with a key of its own

The applier is a party of the Accord, bound to its own role, with terms that let the proposal cross to it. It fetches what is queued for its key, and nothing else reaches it. Give it a label in the labels config ([several mediators](mediators.md)), and run it where the agent cannot change it or read its credential ([custody](custody.md)).

## Fetch each release

```sh
placket get --label approve-applier --keep --wait 5 --json
```

The object holds `delivery`, the next part queued for the key, with its receipt, its body and its salt; nothing, when the queue is empty; or `close`, once the Accord is closed and the queue is empty, which is the applier's cue to stop. `--keep` keeps each delivery in the state directory's `kept.jsonl`, so a release the applier fails on is never lost. `get` refuses a delivery whose answer call is not the one its receipt names.

## Check the release before acting

1. **The body.** Check the delivery's kind and the form of its body, and refuse anything that is not the job's exact shape. A merge applier reads a data body of exactly the repository, the pull request, the base and the head; a file applier, a text of kind `proposal` or `changed-version`.
2. **The answer.** A release an answer queued carries the answer call, as `answer`. Check that its `from` is a key the founding set's bindings give the deciding role, and that its `action` is the one that releases this kind: `accepted` for the proposal, `changed` for a `changed-version`. The terms that name the role are sealed, but the bindings are in the clear, so read the deciding keys from the founding set the labels config names:

   ```sh
   founding=$(placket mediators --json | jq -r --arg l approve-applier '.mediators[] | select(.label == $l) | .founding')
   jq -c '[.bindings[] | select(.roles | index("decider")) | .actor]' "$founding"
   ```

   A release with no answer call, one the terms passed with nobody asked, is not a person's yes. An applier of an action that cannot be undone refuses it.
3. **The world.** Check that what the release names still holds when the applier acts: a merge applier checks that the pull request is open, unmerged and at the released head.

Refuse a release for a fault of its own, once, with its reason. Stop, exit non-zero and leave the queue as it is, for a fault of the world's: a network that does not answer, a credential refused. A release refused is never tried again; one left queued is.

## Act once

Record each receipt id the applier acts on, synced to disk, before it acts, and refuse an id it has recorded already, so that a delivery piped in twice acts once. `github-merge` keeps them in `applied.jsonl`. After acting, record where the action can be checked from: `github-merge` posts the delivery on the pull request and sets a status naming the receipt.

## What it can show later

- `placket released --file F --state DIR` says whether the bytes of a file are a part that crossed to the applier's key, and under which receipt ([the command line](../reference/cli.md#released)).
- `placket verify --delivery` checks a delivery's body against its receipt, beside the journal ([verify](../verify.md)).

That the action matches the release is the applier's doing. A receipt says what was released to the applier's key, and nothing about what the applier did with it ([what placket does not do](../concepts/limits.md)).

To publish an applier as a pattern of this repository, follow [`patterns/README.md`](../../patterns/README.md).
