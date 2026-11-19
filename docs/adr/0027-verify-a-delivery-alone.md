# 0027. A receiver checks the answer call in its delivery, without the journal

Status: accepted. Date: 2026-11-19. Amends record 0021.

## Context

An applier acts on a release only when a key bound to the deciding role accepted it. The release's delivery holds the receipt and the answer call that decided it (maybe-12), and `placket get` refuses a delivery whose call is not the one its receipt names. The call's signature, though, is checked only by the mediator before it writes the receipt (P16 in the threat model) and by `placket verify` given the operator's journal, which an applier does not hold. Issue #51 asks for a check of the call from the delivery and the founding set alone. The `github-merge` pattern is the first applier to use it.

A verifier given a subset of the receipts reads a receipt whose question is outside the subset as outside, and today reads no call against it. One delivery is such a subset: its receipt is a release, or a closing, whose ask is not in it.

## Decision

### What one receipt shows of its call

A call given for a receipt outside the subset's questions is read by verify-1's row 4 as far as that receipt shows. The call reads (maybe-11), so its `sig` verifies under its `from`. It names the Accord id, and its `from` is a member. Its action is the one the receipt records:

- an accept that asks for release, for a crossing receipt, which an answer decides only as an accept's release;
- an accept for a resolution of kind `answer`, and a change for one of kind `changed-version`, each with the receipt's `from` and digest; a change whose receipt is not addressed to the asker, the `sender` of the envelope it echoes, is its release and asks for one;
- a decline for a resolution `declined`, and a cancel, from the asker, for one `cancelled`.

A call that does not fit fails `witness`. A call that fits makes the answer **signed**: the key that signed the call took that action under this Accord. A signed answer is still unwitnessed in verify-3's sense. The call's `question` is the id of the ask's receipt, which the subset does not hold, so which question the key answered stays the mediator's word. So does an accept's `digest`, which is over the answer's text and salt, and those travel only to the asker.

Before this change such a call was ignored. Every subset verify becomes stricter: a call that matches an outside receipt by its typed hash but does not fit it now fails, where it passed before. A call the receipt's `by` names is the call the mediator says decided it, so a call that does not fit is a receipt that does not conform.

### The delivery's own call

Row 5, the body, also reads the delivery's `answer`: it is present exactly when an answer decided the delivery's receipt, and its typed hash, in canonical form, is the receipt's `by`. That is what `placket get` already checks before it records a delivery.

### A delivery checked alone

`placket verify --delivery FILE` with a founding set and no `--receipts` checks one delivery without a journal (verify-5). The receipts are the delivery's own receipt, read as a subset, and the one call given is the delivery's `answer`, each in canonical form as row 5 reads them. Everything else is verify's: row 0 founds the set at the receipt's `at`, validity runs under the set's mediator, rows 1, 2 and 4 run, and row 5 reads the delivery. Together, rows 4 and 5 make the answer of a delivery that conforms signed by its own call.

The decision line of a signed answer names the key that signed the call, the principal that bound it and its roles, and says that its question was not checked. `--subset` is implied, and `--answers`, `--view` and `--superseded-by` are usage errors in this mode. The last two need receipts that one delivery does not hold.

Two other options were weighed:

- A flag on `placket get`. It needs the socket and the key, so it helps neither an auditor who holds only a saved delivery nor an applier that reads a delivery it kept.
- A command of its own. It would repeat verify's founding, validity, body and output.

### The core's names

- **`conform`**: `Decision` gains `Signed bool`, true for a receipt outside the subset's questions whose call fits it as far as the receipt shows. No other name is added. The command line builds the subset and the call from the delivery's bytes, and `Check` and `Body` do the rest.

### The formats

None changes. The new conform vectors use the vector format as it is: a signed answer counts as unwitnessed, and a call that does not fit is `witness` at its receipt's index. No landed vector changes.

## Consequences

- An applier, and an auditor with a saved delivery, can check that the deciding key signed an accept under this Accord, and that the mediator's receipt names that call. They no longer have to trust the mediator for the signature.
- They still trust the mediator for the question the call answers and, for an accept's release, for which part was accepted. Closing that gap would need the release delivery to carry the ask's receipt, whose id is the call's `question`. That is a change to the protocol and the door format, left for later.
- `verify` does not check the signer's roles against a role the caller names. It prints them, and the applier checks them against the founding set's bindings.
