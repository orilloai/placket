# 0022. An Accord is closed by a signed close receipt, and a verifier can say a closed log is complete

Status: accepted. Date: 2026-11-17. Amends records 0008, 0010, 0011 and 0021. Amended by records 0025, 0028 and 0030.

## Context

Until this record an Accord had no end. Its journal could always grow, so no one could say that a list of receipts was the whole record: a list cut short after any receipt still verifies (receipt-6, verify-4). The same gap left no way to retire a mediator's key. A key in use cannot be swapped inside one Accord, because every receipt and every chain is signed by it, and a new Accord could not say which earlier record it replaces. Placket's formats freeze at their first release, so a close added later would be a new protocol version.

## Decision

### A close is the mediator's own act, written by a window action

`placket window close --key F --founding S --journal DIR --accord ID [--socket P]` closes the Accord of a stopped window's journal. Whoever runs the window runs it. The operator and the mediator's key have no door identity, so the close is no door call and no op of the mediator API: a running window holds the journal's lock, and the command is then `journal-locked`. The command needs `--accord`, which must be the founding set's Accord id, since a close cannot be undone; a missing or wrong id is `usage`. It rebuilds the mediator along the restart path (record 0008) with one function, `mediator.Close`, that shares `Reload`'s parameters, checks and codes through one unexported rebuild.

A close writes, in one batch built before any state changes and in one journal write: first a timeout for every open question whose deadline is at or before the close's time, at its deadline, as a tick would (maybe-5); then a closing of each question still open, in the order the questions were opened, `block` with the new reason `closed` and the new decider `{"mediator":"close"}`, from the first answerer or the mediator; and last the close receipt. Opened order is not deadline order, since deadlines are set per rule, which is why the timeouts come first. The close never waits for a fetch, a deadline or a member. Without `--socket` the command exits 0 once the close is written; with it, the command then serves the closed Accord until interrupted. In both modes its `--json` object holds `accord`, `close` as an outcome's `{receiptId, receipt}`, and `closings`. Every reply that carries the close, and every line that keeps it, holds it whole: a close whose receipt, beside an empty list of questions, would make a reply longer than `local.MaxReply`, the longest line the journal and a state directory read, writes nothing and is tool error `journal`, `receipts.jsonl`, `close-size` (`local.CoreCloseSize`, with the line `cli.LineCloseSize`). The receipt grows with the deliveries not yet fetched, so the close can run again once members fetch what is queued for them.

### The close receipt is a format of its own

The close receipt is a DSSE envelope as a receipt's, with one signature by the mediator, and a statement of its own (`spec/schema/close.json`, close-2): predicate type `https://placket.tech/close/0.1`; one in-toto subject, `{"name":"accord","digest":{"sha256":H}}`, where H is the 64 hex of the Accord id, the SHA-256 of the founding set's canonical bytes, which is the in-toto meaning of a subject digest; and a predicate of exactly `accord`, `card`, `mediator`, `at`, `chains` and `unfetched`. `chains` holds, for each chain of the receipts before the close, its `from`, `to`, last `seq` and last receipt id, ordered by pair; `unfetched` holds each delivery not yet fetched, its receiver and the receipt that queued it, in the order queued. Both lists are present when empty, so an Accord with no receipts can be closed, and its close is its first receipt. The close has no `seq`, `prev`, `envelope`, `verdict`, `reason` or `by`: it is in no chain and belongs to no question. Its pins are its chain list.

A receipt statement and a close statement are told apart by their predicate type. `receipts.Decode` still reads a receipt's statement only and refuses a close; `receipts.DecodeClose` reads a close. A list that verifies yields a close as a `receipts.Verified` whose `Close` is set and whose `Statement` is the zero statement, so no reader of statements takes a close for a decision. `receipts.Releases` places a close as `RoleClose`.

### What a verifier checks

Section 8 reads a close through rows 1 to 5, its statement under row 4 and its `mediator` under row 5, and gains two rows. Row 8, `close`, over a whole list: the close's chains are exactly the last receipt of every chain before it, in pair order; each `unfetched` entry names a distinct receipt before it that passed to the entry's `to`, which is all a verifier can say, since it cannot see a fetch; and the `closed` closings come together right before the close, at its time, or end the list, as a torn close leaves it. A `closed` closing followed by any receipt but another or a close fails `close` at that receipt. Row 9, `after-close`: nothing follows a close. Over a subset, a verifier checks the close's signature, form and row 9, and of row 8 only that each chain the subset holds ends where the close says.

Conformance reads the close's closings in row 3: a `closed` closing conforms when it is before its question's deadline and from the first answerer, or the mediator when there is none. That its time is the close's is row 8's. The close itself goes through rows 0 and 1 only; it is no decision. When it is the first receipt, row 0 founds the set at its time, so a binding whose `until` passed before such a close fails `founding`; this is a known limit, not worked around.

### What complete claims, and what it does not

A whole list that ends in its close is complete against that close: `placket verify` prints `complete: against close receipt <id>; not claimed: that the mediator signed no other log for this Accord`. The close pins each chain's last receipt, and each chain links back to its first by `prev`, so a close id fixes every receipt of the log. A subset is never complete, and a list with no close is not.

The not-claimed clause is the limit. Whoever holds the mediator's key can sign single receipts that verify on their own, or a whole other log for the same Accord with another close. A successor Accord's policy `supersedes`, which names the earlier Accord and its close (setup-contract-11), refuses the second: `placket verify --superseded-by NEW` founds the successor's set and checks that its `supersedes` names this Accord and this log's close, and the complete line then names the successor. A successor that supersedes nothing, or names another Accord or another close, or a log with no close, is tool error `verify superseded`. This is how a key is rotated: close the Accord, and found a new one, under the new key, whose `supersedes` pins the close (`docs/guides/rotation.md`). Rotation within one Accord, a countersigned close and outside witnessing of chain heads are later work.

### A closed Accord

A closed Accord never reopens. A member's give and answer are rejected with the new code `closed`, checked right after `not-a-member` in both tables, so a stranger learns nothing. No question is open, the passing of time writes nothing, and `result` still reads every closed question. A fetch still hands out what was queued; once a member's queue is empty, every `get` returns the close receipt, which `placket get` prints and keeps as `close.json` in the member's state directory, once, refusing one that differs. A member that never fetches has the close only from the journal, through whoever runs the window. A restarted window on a closed journal comes up closed, writes nothing, and names the close on its start line; `window close` on it is tool error `closed`. The close appends nothing to `answers.jsonl` and signs no answer call: the questions it closes are decided by the mediator. The calls already in the file are pinned through the receipts whose `by` names them.

### A close cut short

`AddReceipts` is one multi-line append, so a crash can cut a close after any of its lines. A record that holds `closed` closings with no close after them was cut inside a close. `Reload` refuses it with `close`, and the window says to run `placket window close`. `Close` completes it at the time of the closings already written, not at its own: it closes each question still open as `closed` at that time and writes the close receipt at it, so the completed close is the one the whole write would have made.

## Amendments

- **Record 0008, reload.** A record that ends in its close reloads as a closed mediator and writes nothing. A record cut inside a close is refused with `close`. `Close` takes `Reload`'s record and checks it in `Reload`'s order, with its codes, and a closed one is `closed`.
- **Record 0010, the door.** The `get` reply gains a member `close`, the close receipt, given when the caller's queue is empty and the Accord is closed. The `questions` reply of a closed Accord carries the close beside its empty list, so that `placket answer`, `change` and `decline`, which read the question before they sign, report the rejection `closed` without signing anything. These are door formats, not frozen.
- **Record 0011, the journal.** The journal's file layout does not change: on a closed Accord the close receipt is the last line of `receipts.jsonl`, and `Record.Closed` says so. The state directory gains `close.json`.
- **Record 0021, who decided.** The mediator's values of `by` become exactly `deadline`, `undeliverable` and `close`; the reason `closed`, on a resolution `block` only, goes only with `{"mediator":"close"}`, and a closing with reason `closed` names the question's subject (receipt-6 rule 3). Conformance row 2 lets the mediator be the `from` of a `closed` closing, and row 3 reads it as above.

## Consequences

- A closed journal is complete against its close, and a successor that pins the close makes a log signed with the old key after the close detectable, though not a single receipt shown alone.
- Every reader of a list of receipts handles one more kind of entry: `verify`, `log`, `reconcile`, the window's start and the export. The log has no row for the close and names it as `close`.
- The conformance vectors gain a group, `close`, and a scenario step, `close`; the register gains the close receipt and notes on the receipt, the reasons and codes, and the vector format.
- The close's size is bounded by the implementation, not by the protocol: the bound is `MaxReply`, as the call's is `MaxCall`, and it adds no frozen code. A close past it writes nothing, so in that case, which one member can cause by giving parts to a receiver that never fetches, the close waits on fetches, and an Accord whose chains alone pass it cannot be closed (threat model L22). An open Accord refuses no give or answer for its size.
- A close with a pending delivery leaves it fetchable; the close lists it as it stood. A member who never fetches never learns of the close from the door.
