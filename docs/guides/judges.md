# Judges and shadow mode

A judge is a model that reads a part the terms would ask a person about, and passes it, blocks it, or says none. It acts only where the terms would ask, and only where both sides' terms list it. A judge that advises decides nothing: it is shadow mode, a way to see what a judge would have said beside what the person answered, before it decides anything. In v0.1.0 the one judge provider is Jev, called with your own key.

This guide follows [`patterns/shadow-judge/`](../../patterns/shadow-judge/), which runs every step as it is, with no key.

## Where a judge may act

A `maybe` term lists the judges it grants, in order, each to `decide` or to `advise` ([terms](../concepts/terms.md)):

```json
{"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal"]},
 "resolve":{"to":["role:decider"],"timeout":3600,"judges":[{"name":"review","use":"advise"}]}}
```

A judge runs for a part only when both sides' matching terms would ask about it and list the same judges, in the same order (composition-7). A part the terms pass or block reaches no judge, and neither does a release, an answer, or a closed Accord's give. A judge never answers a question.

## Decide

The window asks the `decide` judges in order, within 20 seconds for them all. The first pass or block decides the part, and the receipt names the judge in `by`: a pass is `pass` `ok`, a block is `block` `judge:<name>`. A judge that errs, has no key, scores below its threshold, or is still running at 20 seconds says none, and when every judge says none the part is asked of a person as before. A pass or a block is final.

Never list a judge to `decide` on an action that cannot be undone, such as a merge: a person answers every one.

## Advise: shadow mode

An `advise` judge runs after the receipt is written, whatever decided the part, and its say goes in the journal's `judged/` directory, never in a receipt. The window prints one line per advice:

```
advise review pass sha256:...
```

The person still answers every question. Compare the judge's advice with the person's answers for as long as it takes; once they match well enough, list the judge to `decide` in both sides' terms. That is a change of terms, so it founds a new Accord.

## Set one up

1. Write the judge's config, `judge.json`: its `name`, `provider` (`jev`), `host`, `model`, `threshold`, a whole number from 0 to 100, and `instruction`. Its hash goes into the card, so a change to any byte of it is a new Accord ([`docs/adr/0009-who-decided.md`](../adr/0009-who-decided.md)).
2. List the judge on the card, and found the Accord with that card and the terms that list it:

   ```sh
   placket card --default --judge judge.json >card.json
   ```

3. Start the window with the same config, and your key in `PLACKET_JEV_KEY`. The window runs only the configs its card lists, and refuses to start, `error founding judge`, on any other.

   ```sh
   placket window --key mediator.key --founding founding.json --openings openings --judge judge.json
   ```

At start the window prints, for each judge of the card, which kinds may go to which host and model, or that it sends nothing because no term lists it or there is no key. With no key, a judge sends nothing and says none.

## What leaves the machine

A request carries the part's kind, its body and the config's instruction, with the model and your key, over HTTPS, to the config's host and nowhere else. A raw body is never sent. The key is read once, held in memory and written nowhere. What the provider keeps of a request is the provider's terms. A judge's pass is a model's, and text inside the part can steer it: [the threat model](../security/threat-model.md) says what a judge can and cannot be trusted with.

`evals/judge/` holds a basic eval of labelled cases, injections among them, that you run by hand with your own key, `make judge-eval JUDGE=judge.json`, before you let a judge decide.
