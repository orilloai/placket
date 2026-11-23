# 0008. Reload rebuilds a mediator from its record and closes what was left open

Status: accepted. Date: 2026-11-12. Amended by records 0019, 0020, 0021, 0022 and 0025.

## Context

A mediator lives in one process. When the process stops, a door that keeps a journal still holds what the mediator wrote: the founding set and the time it was founded at, every receipt in order, the deadline and answering roles of each open question, and each delivery queued but not yet fetched. The next process must carry on from that record so that every chain continues, no sender can give the same envelope twice, nothing queued is lost and nothing fetched is handed out again (`restart-reproducible`, `journal-is-evidence`).

Two things cannot come back. A held question's body and salt are never written down, so a restarted mediator could neither put the question to an answerer nor release its part. And a verdict a receipt recorded must not be reached a second time: the decision module may change between the two processes, and a record decided twice could say two things (`one-decision-module`).

Which files a journal uses, and how it lays them out, is for the door that keeps it, which comes later. The core takes the record as values.

## Decision

- **One function.** `mediator.Reload(founding []byte, foundedAt string, key signing.Key, list [][]byte, open []Open, queued []Queued, at string) (*Mediator, []Outcome, error)` returns a mediator rebuilt from the record, and the receipts it wrote while doing so, or no mediator and the first fault it found. `type Open struct { Question, Deadline string; Roles []string }` is an open question as the door recorded it when it was asked: its id, its deadline and the role labels that may answer it. `type Queued struct { To string; Delivery []byte }` is a delivery not yet fetched: the member it is for and its canonical bytes, as the mediator queued them. It takes no registration and no other input: the forbidden check is not in this version.
- **What it checks, in order.** The first fault decides.
  1. The reload time `at`, by the timestamp rule: `*Error` `at`.
  2. The founding set, founded at `foundedAt` as the mediator whose key is `key`, exactly as `Found` does it: founding's own `*Error`, or `ErrOtherMediator` for a key that is not the set's mediator.
  3. The receipts, by `receipts.Verify` under the key's reference: every row of section 8, the questions and releases of receipt-6 included. A failure is `*Error` `journal`, with the failing receipt's index, wrapping the verifier's own `*Error` (`errors.As` finds it).
  4. Each receipt's Accord id and card hash, against the founded set's: `*Error` `journal`, at that receipt's index. A receipt can verify under the mediator's key and still belong to another Accord the same mediator runs.
  5. Each entry of `open`, in order: its deadline by the timestamp rule, `*Error` `at`; then its question, which must be one the receipts hold open, an ask with no closing, and named by no earlier entry, `*Error` `question`. Both carry the entry's index. Then every question the receipts hold open must have an entry: one without is `*Error` `question` with no index.
  6. Each entry of `queued`, in order: its bytes in canonical form, an object with exactly the members of a delivery; its receipt id naming a receipt of the list whose verdict is `pass` and whose `to` is the entry's `To`, with the delivery's `receipt` member that receipt's bytes; its kind the receipt's subject name, and its salt and body hashing to the subject's digest (crossing-3); and no earlier entry naming that receipt. A failure is `*Error` `held` with the entry's index.
- **What it rebuilds.** The receipts and their ids, as given. Each pair's chain, from the last receipt of the pair. For each sender, the largest envelope `seq` any receipt echoes for it: only a give the door passed writes a receipt, and every receipt echoes an envelope its sender signed, so an envelope a sender already used is still refused with `envelope-sequence` (crossing-4). The queues, each receiver's deliveries in the order of the receipts that queued them, whatever order `queued` gave them in, so that a restarted mediator hands them out as the stopped one would have.
- **What it writes.** Each open question is closed, oldest first, before the mediator is returned: `block`, `timeout`, at the question's deadline when the deadline is not after the reload time (maybe-5), and otherwise `block`, `undeliverable`, at the reload time (maybe-4). Its `from` is the first member, by actor-reference order, whose binding gives it one of the entry's roles, or the mediator's reference when no member does. Each echoes the question's envelope and is addressed to the asker, as every closing is. The asker gives the part again if it still wants it to cross. The mediator's state is assembled from the record first; then every closing receipt is built, in one batch, before the state takes any of them, and a failure to build one is `*Error` `internal`, with no mediator.
- **It decides nothing again.** Reload never calls the decision module. The deadline and the roles come from `open`, not from the terms, so a closing names what was recorded when the question was asked even when the terms would now say something else. Which receipt is a question, an answer or a release is read with `receipts.Releases` over the verified statements, the one reader of questions (`release-in-resolve`), and the closings it writes are the two the protocol already has, so a verifier meets nothing it does not already read.
- **Not in the protocol text.** Reload's inputs are an implementation's record, not a format another party reads, and its closings are maybe-4's and maybe-5's. It is proved by the core's Go tests, not by vectors.
- **After a reload**, `Result` reads a closed question's closing and release from the receipts with `receipts.Releases`, as it does for a question closed in the same process, and `Queued` finds each restored delivery by the id of the receipt that queued it.

## Consequences

- A door that journals the founding set, each receipt, each ask's deadline and roles, and each delivery until it is fetched can stop at any moment between two calls and carry on with the same chains and the same sequence numbers.
- A question open at a stop is lost to its asker, by design, and the record says so with a closing receipt.
- A record edited by hand, cut in the middle, or mixed with another Accord's is refused with the index of the first receipt or entry at fault; a record cut after its last receipt reloads, as section 8 allows, and only reconciliation against the parties' own records shows what was cut.
- Two reloads of one record at one time return byte-identical mediators.
