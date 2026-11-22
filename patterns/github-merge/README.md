# The GitHub merge applier

An agent's merge on GitHub happens only through an applier, `apply.sh`, after Placket released it and a person in the deciding role accepted it, and exactly at the head commit released. An auditor follows any merge commit back to its receipt and runs `placket verify` on it.

## The question

"My agent opens pull requests. How do I make sure none merges unless I said yes to that exact commit, and how does anyone check later that I did?"

## When to use it

Use it when an agent proposes merges on GitHub and a person must accept each one before it happens. The agent gives a merge to the applier through Placket, the terms ask the deciding role, and only the applier, which alone may merge under the repository's ruleset (`custody.md`), merges what was released.

It rests on two things, and Placket holds only one of them. The receipt says the mediator released a part naming the head commit, and the answer call in the delivery says which key accepted it. That nothing merged any other way is the claim of the ruleset that lets the applier alone merge, which `custody.md` sets out and Placket neither sets nor checks.

It is not the pattern for a merge a judge may decide: a merge cannot be undone, so the terms here list no judge, and a person answers every one. Nor is it a gate on a status check: the `placket` status it sets is for the auditor, and custody never rests on it. It is one forge's applier; it does not publish, send or push.

## What it relies on

From `spec/protocol.md`:

- §3, Founding an Accord, setup-contract-2, and setup-contract-12 to setup-contract-15: each party seals its terms, the bindings name each key's roles in the clear, and the Accord is founded once every member accepts its own view. The applier reads the deciding role's keys from the bindings, since the terms that name the role are sealed.
- §4, Giving and getting a part at the door, crossing-3: a delivery holds its receipt id, its body and its salt.
- §6, Questions, the three answers and release, maybe-3, maybe-10, maybe-11 and maybe-12: the decider's accept releases the part to the applier, and the answer call travels inside the delivery, as its `answer`, naming its signer and its action.
- §7, Receipts and their chains, receipt-2, receipt-3 and receipt-11: a receipt's subject names the part's kind, and its predicate whether it crossed, to whom and who decided; a release after an accept is a crossing receipt that an answer call decided.
- §8, Verifying receipts, verify-1 to verify-3: `verify` checks the receipts, the delivery's body against its receipt, and which key witnessed an answer, without the openings.
- §11, What the command line prints for a script, commands-3 and commands-8: `get --json` prints the delivery the applier handles, and `verify` prints the lines the auditor reads.

## What it claims

The pattern claims nothing a receipt does not. The auditor holds no opening, so `verify` checks the signatures, the founding set's clear parts and the answer calls, not the sealed terms. The auditor leans on the claim line it prints for that, and on the scope line that follows every claim line:

```
claim: each receipt and close is signed by this mediator.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

### The auditor's path

From a merge commit `COMMIT` of `OWNER/NAME`, as `pattern.sh` runs it in its last step. The operator hands the auditor the Accord's id, its founding set, its `acceptances.json` and the journal, or a subset of it that holds the receipt.

1. Find the pull request the commit merged: `gh api repos/OWNER/NAME/commits/COMMIT/pulls`, the one whose `merge_commit_sha` is `COMMIT` and whose `merged_at` is set.
2. List every status named `placket` on the commit, every page of them, newest first: `gh api --paginate repos/OWNER/NAME/commits/COMMIT/statuses | jq -s 'add | map(select(.context == "placket"))'`. Anyone who may write statuses can set one named `placket`, so try each in turn, newest first, until one passes steps 3 to 6. If none does, the merge has no record.
3. Take the comment's ID only from a `target_url` of the form `https://github.com/OWNER/NAME/pull/N#issuecomment-ID`, where `N` is that pull request's number and `ID` is all digits. Save the delivery: `gh api repos/OWNER/NAME/issues/comments/ID | jq -r '.body | ltrimstr("    ")' >comment.json`. Its `receiptId` must be the status's description, and its body must name that pull request: `repo` exactly the base repository's `full_name`, `pr` its number, `base` its base branch and `head` its head commit.
4. Copy the journal's receipts into an empty directory, without its answer calls: `D=$(mktemp -d) && cp J/receipts.jsonl "$D"/`. `verify` reads a journal's `answers.jsonl` beside the calls it is given, so a copy without it leaves the comment's call the only call `verify` reads. Save that call, `jq -c .answer comment.json >answer.json`, and run `placket verify --json --accord A --founding F --acceptances C --receipts "$D" [--subset] --delivery comment.json --answers answer.json >verify.json`, then `rm -rf "$D"`. It must exit 0. It prints `valid`, and `terms: not checked: the declarations are sealed; with every member's opening, --open checks them`, since the auditor holds no opening.
5. The receipt must be a release of a merge to the applier that the deciding role accepted. Pass the status only when all of these hold:
   - `verify` prints `<receipt id> witnessed: signed by key K, which principal P bound to role R` for the status's own receipt id. The comment's call is the only call it read, so that call is the one the receipt names as its decider, and `K` signed it;
   - `K` is a key the founding set's bindings give the deciding role, and `R` names that role;
   - the comment's call is from `K` and its `action` is `accepted`;
   - the receipt's statement, `jq '.receipt.payload | @base64d | fromjson' comment.json`, has the subject `name` `merge`, and its `predicate` is one of the `predicates` in `verify.json`, with `kind` `crossing`, `verdict` `pass`, `reason` `ok`, `by` an object with `answer`, and `to` the applier's key reference. That is a release after an accept: it passed, an answer call decided it, and it let the merge cross to the applier. A decline, an accept's own closing, a blocked release, a release of a note and a release to another member each fail it. Step 6's body line says that this statement is the one the receipt id names.
6. `verify` prints `body: the part of receipt <receipt id> is the one it names`.

Its conclusion is "the mediator released a part naming this SHA, and key K of the deciding role accepted it". That nothing merged without it is the ruleset's claim, not Placket's.

### What it does not claim

- **The answer call's signature.** The applier checks that the delivery holds an answer call, that its `from` is a key the founding set binds to the deciding role, and that it is an accept. `placket get` has refused a delivery whose call is not the one its receipt names. The call's signature rests on the mediator (P16 in `docs/security/threat-model.md`); checking it from a delivery alone, without the journal, is [issue #51](https://github.com/orilloai/placket/issues/51). The auditor's `verify` checks the comment's own call against the journal's receipts, with none of the journal's calls beside it.
- **The base, between the read and the merge.** GitHub's merge call fixes the head commit, not the base branch. A pull request retargeted after the applier's read merges into its new base if the ruleset there lets the applier merge; the applier reads it again, sees the other base and stops, with no record (`custody.md`, step 2, keeps it out of the other branches).
- **What is public.** The comment publishes the delivery: the body, the salt, the receipt and the answer call, and with them the parties' key references. Anyone with write access to the repository can edit or delete that comment. The operator's journal and the applier's `kept.jsonl` keep the delivery, and the comment can be posted again from it.

## What it does

`sh pattern.sh` runs it on invented pull requests, against the stand-in `gh` in `stand-in/`, and prints `expected.txt`:

1. Makes the keys and founds two Accords, each sealed: in the first, the agent's merges and notes to the applier are asked of role `decider`; in the second, the terms pass a merge with no one asked.
2. Writes the labels config, `config/placket/mediators.json`, with the applier's key on each Accord.
3. The agent proposes sixteen merges and the decider accepts each one. The applier merges pull request 7 at its head and records it, and pull request 14, whose merge call merged although its answer was lost. It refuses the rest, each with its reason: a head a push moved after the read; the repository, the number, the base and the head each differing from the pull request; a body of the wrong shape, data that is a string, a text, raw bytes and a note; a pull request merged already, and one closed; a merge the ruleset refuses; and a merge call that failed and merged nothing.
4. Shows what the applied release left on GitHub: the merge at the body's head, the comment before the status, and the status on the merge commit, naming the receipt and linking the comment, with neither the body nor the salt in it.
5. Pipes pull request 7's kept delivery to the applier again, which refuses it as `applied`, then altered copies of it: without its answer, its answer a change, its answer from the agent's key, each `unwitnessed`; and a base no branch can have, and two deliveries on one input, each a `body`.
6. A release the second Accord's terms passed, with no answer call, is refused `unwitnessed`.
7. GitHub fails partway, and the applier stops for a person each time: the repository no longer reads, a rate limit on the merge call, a comment GitHub does not take after the merge, a status GitHub does not take after the comment, a pull request retargeted after the read, a pull request that cannot be read again after its merge, and a merge call answered as made that merged nothing. Each stop line names what failed. Then a person sends the applier TERM while GitHub reads pull request 22, and it stops with `apply: interrupted at pr 22`. Then a person does what "When it stops partway" says for the first three, and pipes pull request 22 again.
8. A body that spells the repository otherwise than GitHub does is refused `repo`.
9. Thirty-two statuses named `placket` are set on pull request 7's merge commit after the applier's own. The auditor tries each, newest first, refuses them, and reaches the applier's on the second page, with the lines `verify` prints.
10. Three more statuses name a receipt of the journal, each linking a comment that holds pull request 7's merge body and salt: the applied release, its answer the decider's accept of pull request 14; a decline of the same merge, its answer the accept of pull request 7; and a note of the same body, released to the applier, as the applier fetched it. The auditor refuses each at step 5, and refuses the applier's own status when it names the decider's key as the applier's.

## Making it yours

Copy `apply.sh` alone, and change its lines marked `# CHANGE`: the deciding role, as your terms name it; the merge method, `merge`, `squash` or `rebase`; and `forever=yes`, to wait for releases until interrupted. Run it as `sh apply.sh LABEL OWNER/NAME`, with `OWNER/NAME` spelled as GitHub spells it, in the account `custody.md` says, after setting up what it lists. `applied.jsonl` is made beside it.

In `pattern.sh`, remove the line that puts the stand-in first on `PATH`, and change the repository, the labels config and the auditor's inputs.

The agent gives each merge as data of kind `merge`, with exactly `repo`, as `owner/name`, `pr`, the pull request's number, `base`, the base branch's name, and `head`, the head commit's SHA:

```
placket give --label LABEL --to APPLIER --kind merge --as data --file merge.json
```

`base` is any name git takes for a branch: a name with letters outside ASCII passes, and one git refuses, such as `main..next`, is refused as `body`.

## When it stops partway

The applier stops, exit 1, with one line on standard error, whenever going on could refuse a release for a fault that is not the release's own, or could leave a merge without its record. The releases queued after it stay queued for the next run. To pipe a release again, take its delivery from the applier's own `kept.jsonl`, in its state directory (`placket mediators --json` gives it), and only from there, since placket checks none of what `-` reads:

```
jq -c 'select(.receiptId == "ID") | {delivery: .}' kept.jsonl | sh apply.sh LABEL OWNER/NAME -
```

- **The read of the pull request fails:** GitHub does not answer, refuses the credential, limits the rate, or answers 404 while the repository itself does not read, as when the App's installation no longer covers it. Nothing was merged and the receipt id was not kept. Once GitHub answers, pipe the release again. An App's installation token lasts an hour: mint a new one (`custody.md`, step 4).
- **The merge call is refused for the credential, a permission or a rate limit** (401, 403, 404 or 429). Nothing was merged, and the receipt id is in `applied.jsonl`. Once GitHub takes the calls again, take the id's line out of `applied.jsonl` and pipe the release again.
- **The merge call got no answer, or a server's error, and the pull request then reads as unmerged.** That release is refused `merge`, and is not tried again.
- **The pull request merged into another base than the one released.** No comment or status is posted. See to it by hand: the merge did not go as released.
- **The pull request cannot be read again after the merge call, or reads as unmerged after GitHub answered the merge as made.** See by hand whether it merged, and where. If it merged at the body's head into the body's base, post its record as below.
- **The merge happened and its comment was not posted** (`neither its comment nor its status was posted`). Post both by hand from the kept delivery, the comment first:

  ```
  jq -c 'select(.receiptId == "ID") | {body: ("    " + tojson)}' kept.jsonl | gh api -X POST repos/OWNER/NAME/issues/N/comments --input -
  ```

  then the status on the merge commit, its `target_url` the comment's `html_url`:

  ```
  jq -nc --arg url URL '{state: "success", context: "placket", description: "ID", target_url: $url}' | gh api -X POST repos/OWNER/NAME/statuses/COMMIT --input -
  ```

- **The merge happened and its comment was posted, but its status was not** (`its comment was posted, but its status was not`). The line gives the comment's `html_url`. Post the status alone, as above, with that URL; a second comment is not needed.
- **An INT or a TERM.** The applier stops once the command under way ends, exit 1, with `apply: interrupted at pr N` while it handles a release, or `apply: interrupted` between releases. For pull request N, look at `applied.jsonl` and at GitHub. If the receipt id is not in `applied.jsonl`, nothing was merged: pipe the release again. If it is there and the pull request is unmerged, take the id's line out and pipe it again. If it merged, post whatever of its comment and status is missing, as above. With no pull request named, the last line of `kept.jsonl` may be a release `placket get` fetched as the signal came and the applier never handled: if the applier printed no `applied` or `refused` line for it, pipe it again.

- **A crash.** The receipt id is synced to `applied.jsonl` before the merge call. A crash after that and before the merge leaves the release refused as `applied` on a second try: if the pull request is unmerged, take the id's line out of `applied.jsonl` and pipe it again. A crash after the merge and before the comment leaves a merge with no record or status: post both by hand, as above.
