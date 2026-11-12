# 0006. The mediator holds an Accord's state and runs its operations

Status: accepted. Date: 2026-11-12.

## Context

The capability packages decide: the canonical form, signatures, founding, composition and receipts are each a function of their inputs. Something must hold what those decisions leave behind, between one operation and the next, for one founded Accord: the receipts written, the chain of each pair, each sender's highest accepted sequence number, what is queued for each receiver, and the parts held as questions. It must run the door's check table in order, call composition for the verdict, have receipts build each receipt, and change its state only after every receipt of an operation is built. It is the one role package of the core (`packages-by-adr`), and it must hold no capability's logic (`mediator-no-logic`).

The door's check table covers crossing-1 to crossing-4 and crossing-6: the crossing message, the part, the envelope and their order. These read the mediator's state as they go (its members, its Accord id, each sender's highest seq), and no capability package of the core is named for them.

## Decision

- **`core/mediator` holds one Accord's state.** A `Mediator` holds the founded Accord, the mediator's key, every receipt written in order, the last link of each ordered pair's chain, each sender's highest accepted `seq`, a queue of deliveries per receiver, and the open questions in the order they were opened. One goroutine owns it; its methods are not safe for concurrent use.
- **Its names**, by signature:
  - `func Found(set []byte, key signing.Key, at string) (*Mediator, error)`: founds the set by `setup.Found`, and refuses with `ErrOtherMediator` a set whose `mediator.actor` is not the key's reference (setup-contract-7), a sentinel because no set is wrong for it (`sentinels-uncoded`). A founding refusal is `setup`'s `*Error`, returned as it is.
  - `func (m *Mediator) Give(caller string, message, envelope []byte, at string) (Outcome, error)`: the door's twelve rows in order, then `composition.Decide`, then `receipts.Build`, then the state. `type Outcome struct { Verdict, Reason, ReceiptID string; Receipt []byte }`.
  - `func (m *Mediator) Get(caller string) ([]byte, error)`: the first delivery queued for the caller, as the canonical bytes of `delivery.json`, or nil when none is queued.
  - `func (m *Mediator) Receipts() [][]byte` and `func (m *Mediator) Pending() []string`: the reads a scenario's end state needs, as copies (`results-are-copies`). `func (m *Mediator) Accord() string` and `func (m *Mediator) Card() string`.
  - `type Rejection struct { Code RejectionCode }` with the door's twelve codes, what the caller did wrong; `type Error struct { Code ErrorCode }` with `CodeAt`, a time off the timestamp rule, and `CodeInternal`, a receipt the mediator could not write, what the caller's program did wrong or what failed inside (`rejection-vs-error`). `const Extension`, `RoleAgent`, `RoleUser`, `MinSaltBytes`.
- **The door's rows are one operation of the mediator.** They run in the order of section 4's table and stop at the first that fails; nothing of the message is read before the envelope's signature verified under the caller's key. They are written as one function that reads the mediator's state, not as a package of their own: each row reads a member or the state, and decides nothing composition or receipts decide. A rejected give returns before any state is touched (crossing-7).
- **A receipt before state.** `Give` builds the receipt, and the delivery or the question it leads to, and only then appends the receipt, advances the pair's chain and the sender's `seq`, and queues or holds the part. No test can stop the core between the two, since it does no I/O; the reviewer reads it in the code.
- **Asking** (maybe-1). A give decided `escalate` writes its crossing receipt and holds a question under that receipt's id: the asker, the receiver, the part as its delivery would carry it, the echoed envelope, the subject, the agreed rule, and the deadline, the give's time plus the rule's timeout, at most `9999-12-31T23:59:59Z`. Nothing is queued. Answering, the deadline's passing, release and reload are the next milestone's: they add `Resolve`, `Undeliverable`, `Tick`, `Reload` and the reads of a question to this package, and keep the rule that every receipt of an operation is built before the state changes.

## Consequences

- Two mediators founded from one set, with one key, at one time, and given the same operations at the same times, write the same receipts byte for byte (receipt-10). Nothing here reads a clock or mints a salt.
- A vector can test a give end to end through one mediator, and a door adds only what touches the world: the socket, the clock, the journal.
- The door's rows live beside the state they read. A reviewer checks that none of them compares a stance, matches a term or builds a receipt.
