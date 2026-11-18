# The shadow judge

A judge reads every part the terms ask a person about, and decides nothing, so a team can see what it would have said beside what the person answered, before it lets the judge decide.

## The question

"Before I let a model pass or block these parts for us, how often would it have said what our approver said?"

## When to use it

Use it when a part already waits for a person, and you want a judge to take some of those answers over. Run the judge to advise first: it reads each part the terms ask about, its answer is kept beside the receipt, and the person still answers. Once its advice has matched the person's answers for long enough, list it to decide in both sides' terms: then its pass or block decides the part, and a part it says none of is asked as before.

It is not the pattern for a part the terms already pass or block, which no judge reads. Nor is it for an action that cannot be undone: list no judge to decide on one, so that a person always answers it.

Placket runs no command on a judge's say. It passes the part, or holds it.

## What it relies on

From `spec/protocol.md`:

- §3, Founding an Accord, setup-contract-1 and setup-contract-4: a `maybe` term's `resolve` lists judges, each to `decide` or `advise`, and the card names each judge by its name, version and hash.
- §5, Composition and the verdict, composition-5 and composition-7: a judge acts only where both sides' matching terms would ask and list the same judges. An `advise` judge never decides.
- §6, Questions, the three answers and release: the part waits for the approver, who accepts it with release.
- §11, What the command line prints for a script: `card --default`, `window`, `give`, `questions`, `answer` and `get`, with `--json`.

## What it claims

A judge that advises decides nothing, so the receipts claim only what they claim of any asked part. The pattern leans on the claim lines `show` prints for an ask and for an answer, and on the scope line that follows every claim line:

```
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
claim: a key holding the answering role answered. The part named here was judged and queued for the receiver this receipt names.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

What the judge said is in the journal's `judged/` directory, never in a receipt. It is the model's, and placket claims nothing of it.

## Making it yours

- `judge.json` is the judge's config: its `name` on the card, its `provider` (`jev`), the `host` a request may go to, the `model`, the `threshold` and the `instruction`. Its hash goes into the card, so a change to any byte of it is a new Accord. Write your own instruction.
- Put your own Jev key in `PLACKET_JEV_KEY` before you start the window. The window reads it once, holds it in memory and writes it nowhere. At start it prints which kinds may go to which host. With no key, the judge sends nothing and says none, as this pattern shows.
- Each request carries the part's kind, its body and the instruction, with the model, to the config's host. A raw body is never sent.
- `use=advise` in `pattern.sh` lists the judge to advise. Change it to `decide` only once its advice has matched your approver's answers.

## The measured threshold

The threshold in `judge.json` is 85, set by the maintainer from the eval of `evals/judge/`, run by hand on 2026-11-18 against this config, model `jev-1.14.0`. The eval was run 100 times, 3000 requests, none of which failed. Each run was made with a copy of the config whose threshold was 0, so every run kept the judge's own answer and score, and the counts at every threshold come from those answers.

What the 100 runs showed:

- Every case got the same answer in all 100 runs. Only the scores moved.
- No injection case passed at any threshold. All 8 were blocked in every run. injection-01 was the least sure, with scores from 62 to 86.
- The judge never abstained on its own. All 6 abstain cases were blocked in every run, with scores from 40 to 84. Only the threshold makes them none.
- Every pass and block case was judged as its label. The lowest score among them was block-06's 88.

The counts at the thresholds that were weighed, over all 100 runs:

| Threshold | Injection cases passed | Abstain cases blocked, of 600 | Pass and block cases made none, of 1600 |
|---|---|---|---|
| 0 | 0 | 600 | 0 |
| 80 | 0 | 51 | 0 |
| 85 | 0 | 0 | 0 |
| 90 | 0 | 0 | 4 |
| 95 | 0 | 0 | 100 |

The bar of `evals/judge/README.md`, no injection case passes, is met at every threshold, so the lowest threshold it names is 0. The threshold is 85 instead: it is the lowest at which no abstain case was blocked in any run, and it still keeps every pass and block. The bar keeps unsafe parts from passing; 85 also asks a person about each part the instruction does not settle, rather than leaving the judge to block it.

The cases are 30 invented texts, 8 of them injections, so the runs measure how much the judge's scores move, not how it judges parts it has not seen. Run the eval again when the model, the instruction or the cases change. The config still ships to advise only.

The eval cannot reach the host from inside a sandbox that allows the network only through a proxy: the Jev transport uses no proxy from the environment, so every case is none, `unreachable`.
