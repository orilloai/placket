# 0026. A member checks its own receipts against the view it signed

Status: accepted. Date: 2026-11-19. Amends records 0021 and 0025.

## Context

Since record 0025 each member signs its own view of the Accord, and the terms behind it stay sealed. A verifier with no opening checks what every party signed: that the receipts are the mediator's, that each names the Accord, its card and its members, and that every member accepted its view. It cannot say whether a decision was the one the terms allowed, since that reads the sealed terms. Opening a declaration is for a dispute.

A member holds more than that verifier: the view it signed. Its view states, for every crossing its own terms select, what the merged terms make of it, and for an asked crossing the agreed rule. So a member can check, with no other party's terms, that the mediator kept to what it consented to, for every give it sends or receives and every question it asked or received. Record 0025 left that check for later work.

## Decision

### What a view check reads

`verify --view V` takes one member's view, as the member kept it, beside the founding set and its acceptances, the no-terms verify of record 0025. It adds two rows to verify-1's conformance table.

- **The view, before any receipt.** V is the view of its `viewer` that the viewer accepted: its bytes are accepted by the canonical form, it has the closed shape of `view.json`, its `accord` is the Accord id, its viewer is a member, and its typed hash is the `view` of the viewer's acceptance in `acceptances.json`. Anything else is `acceptance-view`, the code founding's row 21 returns for the same fact. A view founding could not check, for want of the openings, is checked here by its hash: the hash is what the member signed.
- **Each receipt, after rows 1 to 4.** A receipt of the viewer's is checked against V's rows, and one that does not match is `view-conformance`, at its index. The receipts of the viewer's are the receipts of every give it sends or receives, and the closings and releases of every question it asked or received, found by receipt-6's question rule. No other receipt is read: not a crossing between two other members, and not a closing the viewer signed as an answerer on a question it neither asked nor received.

A give's receipt is read against the row for its counterpart, its direction from the viewer and its kind: `crosses` is a `pass` `ok` by the terms; `blocked` is a `block` by the terms, `no-match` or `maybe`, since a blocked row gives no reason; `asked` is an `escalate` `maybe` by the terms, or a decision by a judge the row lists with use `decide`, a `pass` `ok` or a `block` `judge:<name>`. A give with no row is a kind the viewer's terms do not select toward that counterpart, which composition-2 blocks: it matches only as a `block` `no-match` by the terms.

A closing and its release are read against the asked row of their question: the row's `to` names the answerers, by the bindings in the clear, and its `timeout`, after the ask's `at`, is the deadline, as maybe-1 sets it. Each closing is then read as verify-1's row 3 reads it, the row standing for the agreed rule. A part the mediator formed from an answer, a closing's or a change release's subject of kind `answer` or `changed-version`, is read as composition-5 decides it, against the row of the formed crossing when the viewer is one of its sides: `crosses` is a `pass` `ok`; `blocked` and no row a `block`, as for a give; and `asked` a `block` `maybe`, since a formed part is never asked about. A release after an accept is a `pass` `ok`, since its question's row is `asked` and the same part decided again with no judgment is asked again (maybe-10). When the viewer is no side of a formed crossing, as the receiver of a question whose answer goes to the asker, the answer's verdict is not read, only that the closing fits the rule.

These rows read the view, not the terms: they compare no stance, match no term and compare no two terms' judges, so they hold no logic of the decision module (`one-decision-module`). They run whatever the card's module, since the view's outcomes are the ones the member signed.

### The order

The view row comes after row 0 and before validity, as a founding fact. The per-receipt row comes after rows 1 to 4 of the same receipt, and the first failure of any row decides, by receipt in list order (`check-order`). Every receipt of the viewer's is still checked after a failure, so that each line can be printed.

### The command line

`placket verify --accord A --founding S --acceptances C --receipts J --view V` prints, after the decision lines, one line per receipt it checked, with the line of `receipts.jsonl` and the row it read, and then a closing line, that every receipt of the viewer's matches the view it accepted, or that one does not, at its line. A view that is not the one accepted is the command's error `founding` with `acceptance-view`, and prints nothing else. `--view` takes one view per run, and with `--open` of the viewer's own opening it is `usage`: an opening checks the viewer's seal and terms, and with every opening, every decision.

### The core's names

- **`conform`**: `Evidence` gains `View []byte`. `Report` gains `Viewer string` and `Viewed []Viewed`. `type Viewed struct { Index int; Counterpart, Dir, Kind, Outcome string; Matches bool }` is one receipt checked against the view, with the row it read, its `Outcome` empty when the view has no row. The codes `CodeAcceptanceView`, spelled as `setup.AcceptanceView`, and `CodeViewConformance`.

### The formats

The code `view-conformance` joins "Reasons and codes", and `acceptance-view` becomes a code a verifier also returns. A `conform` vector gains an optional input `view`, the view as a JSON value, which needs a founding set, and the expectation `viewed`, the number of receipts checked against it, when the vector gives one. A new group, `conform-view`, holds those vectors, filed under section 8.

## Consequences

- A member checks the decisions it is a side of without any counterpart's opening, and a dispute needs the openings only for the rest.
- The check is as good as the view: a view states the outcome with no judgment, so a judge's decision is checked only as one the row lists to decide, never by what the judge said.
- A member that is only the receiver of a question checks that its closing fits the agreed rule, and not the answer's verdict toward the asker, which its view does not hold.
- A person checking two members runs verify twice. A view check of every member at once, and of the `answers` rows an answerer signed, are left for later work.
