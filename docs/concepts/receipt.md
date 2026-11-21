# The receipt

A receipt is the mediator's signed statement of one decision about one part, under one Accord. The mediator writes one for every decision it makes: every part given, every question closed, every release, and the close. A third party checks them with the mediator's public key alone ([section 7 of the protocol](../../spec/protocol.md#7-receipts-and-their-chains)).

## What it holds

A receipt is a DSSE envelope, signed by the mediator's key, around an in-toto statement (receipt-1). Its id is the typed hash of that statement. It names:

- the part by its kind and its salted digest, never by its body or its salt: whoever holds both can show that a part is the one judged (crossing-3);
- the two parties, from and to, the Accord and its card;
- the verdict, `pass`, `block` or `escalate`, and a reason from a closed list: `ok`, `no-match`, `maybe`, `judge:<name>`, `declined`, `cancelled`, `timeout`, `undeliverable` and `closed` (receipt-4);
- who decided, in `by`: the terms, a judge of the card, an answer call by its typed hash, or the mediator, for a deadline, an undeliverable question or a close (receipt-11);
- a time, in whole seconds.

A crossing receipt records a give or a release; a resolution receipt, how a question closed; a close receipt, the end of the Accord.

## Chains

The receipts between each ordered pair of parties form a chain, by `seq` and `prev` (receipt-5). A receipt removed from the middle of a chain is found at the next receipt of that pair. One removed from the end is not found by verifying alone: a party that kept its own records finds it with `placket reconcile` (receipt-7), and a journal that ends in its close receipt is complete against that close (verify-4).

## What it claims

`placket show` decodes one receipt without checking it, and prints what it claims. Each claim line is followed by the scope line:

```
claim: the mediator judged this part under the Accord's terms and queued it for the receiver.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

A receipt shown is not a receipt checked: `show` says `not verified`. `placket verify` checks a whole journal ([verify](../verify.md)). A receipt does not show that the mediator is honest, or that its clock is true ([what placket does not do](limits.md)).

## Where receipts are kept

The window writes every receipt to its journal, `receipts.jsonl`, before it replies. Each delivery a party fetches carries the receipt that passed it, and the party's state directory keeps it, so each party holds its own record beside the operator's.

```sh
placket show --receipt receipt.json
```
