# The GitHub merge applier

An agent proposes a merge to Placket, a person answers, and only the applier, which alone may merge under the repository's ruleset, merges what was released, at exactly the head commit released. Anyone can follow a merge commit back to its receipt and verify it.

## The question

"My agent can open pull requests. How do I make sure it cannot merge one unless I said yes, and show afterwards, for any merge, that I did?"

## When to use it

Use it when an agent works on a GitHub repository and merging is the step that must wait for a person: the agent proposes a merge, a person with the deciding role accepts it, and `apply.sh` merges the pull request the release names, only if it is still at the head commit released.

`apply.sh` is the applier alone. It takes its label and its repository as arguments and sets up nothing, so a copy of that one file runs on your own Accord. `pattern.sh` founds an invented Accord, sets up the labels config, plays the agent and the person, runs `apply.sh` against a stand-in for GitHub, and then follows the auditor's path below.

It is the pattern only where the repository's ruleset lets nothing but the applier merge, and the window, the deciding role's key and the applier run in an account the agent cannot use (`custody.md`). Without that, an agent with a merge credential merges around it, and the applier's records prove nothing about the merges it did not make. It is not for publishing, sending or deploying: those are other appliers. It never closes, edits or pushes to a pull request, and its only comment is the delivery record.

## What it relies on

From `spec/protocol.md`:

- §3, Founding an Accord, setup-contract-2, Bindings and roles: the founding set's bindings name the keys of each role in the clear, so the applier finds the deciding role's keys there though the terms are sealed (setup-contract-12 to setup-contract-15).
- §4, Giving and getting a part at the door, crossing-3, Digests, salts and deliveries: a delivery holds the body and the salt its receipt's digest names.
- §5, Composition and the verdict, composition-5, Maybe: the agent's terms make every merge a question for the deciding role; they list no judge, so none may pass one.
- §6, Questions, the three answers and release: maybe-3 and maybe-10, an accept with release queues the part for the applier exactly as given; maybe-11 and maybe-12, the answer call, signed by the answerer, travels inside that delivery.
- §7, Receipts and their chains, receipt-11, Who decided: the release receipt names the answer call by its typed hash.
- §8, Verifying receipts, verify-1 to verify-3: the auditor's `verify`, with the delivery and its answer call, says who answered and that the body is the receipt's part.
- §11, What the command line prints for a script: `mediators`, `give`, `answer`, `get --keep --wait --json` and `verify --delivery --answers`.

## What it claims

The pattern claims nothing a receipt does not. The release receipt's claim line, as `show` prints it, and the scope line that follows every claim line:

```
claim: a key holding the answering role answered. The part named here was judged and queued for the receiver this receipt names.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

And the line `verify` prints for a journal:

```
claim: each receipt is this mediator's signed decision about one part under this Accord's terms.
```

So the claim is: the mediator released a part naming this commit. That nothing merged without such a release is the ruleset's claim, not Placket's: Placket says nothing about what the applier did with the part, or about merges that did not go through it (`custody.md`).

### The auditor's path

`pattern.sh`'s last step runs it as written here, against the stand-in.

1. Start from a merge commit. Its commit status of context `placket` names a receipt id as its description and links a comment on the pull request.
2. The comment's body is the delivery as the applier fetched it, indented four spaces as a code block so that nothing in it renders. Save it whole, as `comment.json`, with `gh api repos/OWNER/NAME/issues/comments/ID | jq -r .body >comment.json`, and save its answer call as it is, with `jq -c .answer comment.json >answer.json`: `verify` matches a call only in the bytes it was signed in.
3. From the Accord's operator, get the Accord id, its founding set, `acceptances.json` and the journal, or a subset of it holding that receipt. The operator hands them over; the applier holds none of them.
4. Run:

   ```
   placket verify --accord A --founding founding.json --acceptances acceptances.json \
     --receipts journal [--subset] --delivery comment.json --answers answer.json
   ```

   It prints `valid`, the line that the terms were not checked, since they are sealed, the decision line of the release, witnessed by the key that signed the answer call and the role its principal bound it to, and the `body` line, that the delivery's body is the part the receipt names.
5. The body names the merge you started from. Find the pull request the merge commit belongs to, the merged one whose `merge_commit_sha` is that commit, as `gh api repos/OWNER/NAME/commits/COMMIT/pulls` lists it. The body's `repo`, `pr`, `base` and `head` must be its repository (`base.repo.full_name`), its number, its base branch (`base.ref`) and its head commit (`head.sha`). A status can be set by anyone who may write statuses, so a receipt that verifies but whose body names another pull request, another base or another head says nothing about this merge.

Publishing the delivery publishes the parties' key references, in the receipt and the answer call, and the body. The comment is public to whoever reads the repository, and anyone with write access to it, which is likely to include the agent's own credential, can edit or delete it. An edited comment fails `verify`, but a deleted one is gone: the records that survive are the operator's journal and the applier's `kept.jsonl`, which hold the receipt and the delivery. The status never holds the body or the salt, only the receipt id and the link.

### What the applier checks, and what it does not

For each delivery it prints `applied pr <n>` or `refused pr <n> <reason>`, `-` for `<n>` when the body names no number, and never tries a refused release again. `apply.sh` lists its checks in order. Two rest on others:

- **The answer call's signature.** The applier checks that the delivery holds an answer call, that its `from` is a key the founding set binds to the deciding role, and that it is an accept. `placket get` has refused a delivery whose call is not the one its receipt names. The call's signature is the mediator's check (P16 in `docs/security/threat-model.md`); checking it from a delivery alone, without the journal, is [issue #51](https://github.com/orilloai/placket/issues/51).
- **The head and the base.** The applier merges with `sha` set to the body's `head`, so GitHub refuses the merge, `head-moved`, if the head moved since the applier read it. GitHub's merge call takes no base, so the applier reads the pull request again after the merge, and stops, posting no comment or status, unless it merged into the body's `base`. Only the ruleset keeps a merge out of a branch no one released (`custody.md`, step 2).
- **What it posts.** The delivery's message id is the giver's own, so the applier takes only a plain one, of letters, digits and `.`, `_`, `:` and `-`, and a delivery that fits in one GitHub comment; it posts the delivery as a code block, so that nothing in it renders as a link, an image or a mention.

## When it stops partway

Each recovery below pipes the applier its own kept delivery, a line of `kept.jsonl` in its state directory, which `placket mediators` prints, with `jq -c 'select(.receiptId == "ID") | {delivery: .}' kept.jsonl | sh apply.sh LABEL OWNER/NAME -`. Feed `-` nothing else: placket checks none of what arrives on standard input, and a delivery copied from a pull request comment, or from anywhere but the applier's own `kept.jsonl`, is not one `placket get` checked against its receipt.

- **After `placket get`, before the receipt id is kept.** A crash there, or a read of the pull request GitHub did not answer, printed as `refused pr N read`, leaves the release fetched and not applied. Pipe it again, as above, once GitHub answers.
- **After the receipt id is kept, before the merge.** `apply.sh` adds the id to `applied.jsonl`, synced, before the merge call, so a crash after a merge never merges twice. A crash before the merge leaves the release refused as `applied` on any later run. To merge it, check the pull request is unmerged, remove that id's line from `applied.jsonl`, and pipe it again.
- **A merge call that failed.** When GitHub answers the merge call with an error other than `409`, or not at all, the applier reads the pull request again: merged at the body's head into its base, it posts the comment and the status and prints `applied pr N`; not merged, it prints `refused pr N merge`. When it cannot read it again, it stops with a line saying so: look at the pull request by hand, and post the record as below if it merged.
- **Merged into another base.** When the pull request read again after the merge has a base other than the body's, the applier stops with a line naming the branch it merged into, and posts no comment or status. The release is spent, and the merge is in a branch no one released: a person decides whether to revert it there.
- **After the merge, before the comment or the status.** The merge has no record or status, and `apply.sh` stops with a line saying so. Post both by hand from the kept delivery: the comment, with `jq -c 'select(.receiptId == "ID") | {body: ("    " + tojson)}' kept.jsonl | gh api -X POST repos/OWNER/NAME/issues/N/comments --input -`; then the status on the merge commit, `{"state":"success","context":"placket","description":"ID","target_url":"<the comment's html_url>"}`, posted to `repos/OWNER/NAME/statuses/<merge commit>`.

## Making it yours

- `custody.md` first: without its ruleset, the applier is one more way to merge, not the only one.
- In `apply.sh`, the lines marked `# CHANGE`: `role`, the role whose accept releases a merge, `decider` as shipped; `method`, `merge`, `squash` or `rebase`; and `forever`, `yes` to wait for releases until interrupted, where as shipped it stops once the queue is empty.
- Run it as `sh apply.sh LABEL OWNER/NAME`, in the window's account or another the agent cannot use (`custody.md`, steps 3 and 4), with your labels config naming the applier's key on your Accord, and `gh` signed in as the applier's identity. It keeps `applied.jsonl` beside itself.
- Your Accord's terms: the agent proposes kind `merge` to the applier under a `maybe` term answered by your deciding role, and no judge may pass it. The body is data with exactly `repo` (`owner/name`), `pr` (a number), `base` (the base branch's name) and `head` (the head commit's hash).
- In `pattern.sh`, the `# CHANGE` lines are the stand-in's `PATH` line, which a copy used for real removes so that `gh` is the real one, the repository, the labels config and the auditor's inputs.
