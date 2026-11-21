# Ask and answer

When the terms ask, the mediator holds the part as a question, and a person, or a key, in the role the terms name answers it. Nothing reaches the receiver until an answer releases it ([section 6 of the protocol](../../spec/protocol.md#6-questions-the-three-answers-and-release)).

## The ask

A give the terms ask about gets a receipt of verdict `escalate`, which placket prints as `ask`, and the receipt's id is the question's id (maybe-1). The give exits 4 and prints the id. The question's deadline is the give's time plus the terms' `timeout`. While it is open, its part is shown to its answerers, and to no one else, so that an answer is about exactly that content.

```sh
placket questions --label approve-person
placket watch --label approve-person
```

`questions` lists the open questions the key may answer; `watch` prints each new one once, as it opens, with the lines that answer it.

## The three answers

- **Accept.** `placket answer` accepts, asking for release unless `--no-release`. The part is released to the receiver exactly as it was given (maybe-3, maybe-10).
- **Change.** `placket change` answers with a version of the answerer's own. The version, of kind `changed-version`, goes back to the asker and is released to the receiver in place of the part; the part itself is never released (maybe-9). With `--comment`, the asker also gets a comment.
- **Decline.** `placket decline` refuses it, with a comment for the asker. Nothing is released (maybe-4).

Only the question's answerers answer it, before its deadline, and a question closes once (maybe-6, maybe-7). The asker may cancel its own question with `placket cancel`. A question nobody answers closes as a `timeout` at its deadline, and one nobody can be reached to answer as `undeliverable` (maybe-4, maybe-5). A close of the Accord closes every question still open as `closed`.

The asker learns how its question closed with `placket result`, or by giving with `--wait`.

```sh
placket result --label approve-agent --question ID --wait 60
```

## The answer call

An answerer signs its answer itself: the answer call names the question, the action and the salted digest of the answer's text, never the text (maybe-11). The call travels inside every delivery its answer releases, and the journal keeps it in `answers.jsonl` (maybe-12). So an applier reads who accepted a part from the delivery it fetched, and a verifier shows each answer witnessed by the key that signed it ([verify](../verify.md)).

A judge never answers a question, and no judge runs at a release ([judges](../guides/judges.md)).
