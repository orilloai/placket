# 0013. What a verifier checks and says

Status: accepted. Date: 2026-11-23.

## Context

A verifier that checks only signatures and chains can say that the receipts are the mediator's, not that each decision was one the Accord allowed. A member holds more than a stranger does: the view it signed, which states what the merged terms make of every crossing its own terms select. And a person reads every line of `verify`, `released` and `show` as a claim, so each line must mean exactly what was checked: no more, and in the same words a second implementation that follows the requirement text would reach. Two verifiers that follow the protocol must agree on every receipt: whether it verifies, and what its id is. The formats freeze at the first release, so these rules are settled before it.

## Decision

### Validity, then conformance

- `verify` checks validity and then conformance (verify-1 to verify-4). It needs the Accord id alone out of band: it takes the mediator's reference from the founding set whose typed hash is that id. It takes the kept answer calls (`--answers`), a subset (`--subset`) and one delivery (`--delivery`) when given.
- **Conformance rows** run per receipt after validity: the Accord, the parties, the decision decided again, the witness of an answer, and, for one delivery a caller gives, its body. With sealed terms (record 0006), the decision row runs for a receipt only when every member whose terms it reads is opened; without openings it is skipped, and the witness row checks a decline's signer as a member, as when the module is not trusted. A member's view adds rows 0a and 4a (below), and a delivery is read as one first (below).
- **A verifier never runs a judge again and never fetches anything.** A judge's outcome is taken as the receipt claims it, and the decision row checks that the Accord let that judge decide.
- **The decision row runs only under the kernel hash the verifier trusts,** its own; under another, the row is skipped and the witness and body rows still run.
- **Before a close,** verify claims nothing about completeness (record 0010). It can check a subset of the receipts, whose chains may start late, and one delivery's body and salt.

### The witness of an answer

- A receipt an answer decided is witnessed by a call given that passes row 4, unwitnessed with no call given, and contradicted by a call given that does not pass row 4. A contradicted receipt fails `witness`; it is never printed witnessed or unwitnessed. `verify` prints `<receipt id> contradicted: the call given for it does not fit`. Contradicted is a printed word and an outcome under the code `witness`, not a code of its own.
- A release reads its closing's call. So a release after a contradicted accept is contradicted too. An accept whose call fits stays witnessed when only its release fails, as when the call asks for no release: the failure is the release's.
- The unwitnessed line names the roles the answerer's binding gives it: the question's rule is not known when the terms are sealed, and the binding's roles always are.

### A member checks its own receipts against its view

- **What a view check reads.** `verify --view V` takes one member's view, as the member kept it, beside the founding set and its acceptances. It adds two rows to verify-1's conformance table.
  - **Row 0a, the view, before any receipt and before validity.** V is the view of its `viewer` that the viewer accepted: its bytes are accepted by the canonical form, it has the closed shape of `view.json`, its `accord` is the Accord id, its viewer is a member, and its typed hash is the `view` of the viewer's acceptance in `acceptances.json`. Anything else is `acceptance-view`, the code founding's row 21 returns for the same fact. A view founding could not check, for want of the openings, is checked here by its hash: the hash is what the member signed.
  - **Row 4a, each receipt, after rows 1 to 4.** A receipt of the viewer's is checked against V's rows, and one that does not match is `view-conformance`, at its index. The receipts of the viewer's are the receipts of every give it sends or receives, and the closings and releases of every question it asked or received, found by receipt-6's question rule. No other receipt is read: not a crossing between two other members, and not a closing the viewer signed as an answerer on a question it neither asked nor received.
- **How a receipt is read against a view.** A give's receipt is read against the row for its counterpart, its direction from the viewer and its kind: `crosses` is a `pass` `ok` by the terms; `blocked` is a `block` by the terms, `no-match` or `maybe`, since a blocked row gives no reason; `asked` is an `escalate` `maybe` by the terms, or a decision by a judge the row lists with use `decide`, a `pass` `ok` or a `block` `judge:<name>`. A give with no row is a kind the viewer's terms do not select toward that counterpart, which composition-2 blocks: it matches only as a `block` `no-match` by the terms.
- A closing and its release are read against the asked row of their question: the row's `to` names the answerers, by the bindings in the clear, and its `timeout`, after the ask's `at`, is the deadline, as maybe-1 sets it. Each closing is then read as verify-1's row 3 reads it, the row standing for the agreed rule. A part the mediator formed from an answer, a closing's or a change release's subject of kind `answer` or `changed-version`, is read as composition-5 decides it, against the row of the formed crossing when the viewer is one of its sides: `crosses` is a `pass` `ok`; `blocked` a `block`, as for a give; no row only a `block` `no-match`; and `asked` a `block` `maybe`, since a formed part is never asked about. A release after an accept is a `pass` `ok`, since its question's row is `asked` and the same part decided again with no judgment is asked again (maybe-10). When the viewer is no side of a formed crossing, as the receiver of a question whose answer goes to the asker, the answer's verdict is not read, only that the closing fits the rule.
- **These rows read the view, not the terms:** they compare no stance, match no term and compare no two terms' judges, so they hold no logic of the decision module (`one-decision-module`). They run whatever the card's module, since the view's outcomes are the ones the member signed.
- **The order.** The view row comes after row 0 and before validity, as a founding fact. The per-receipt row comes after rows 1 to 4 of the same receipt, and the first failure of any row decides, by receipt in list order (`check-order`). Every receipt of the viewer's is still checked after a failure, so that each line can be printed.
- **The command line.** `placket verify --accord A --founding S --acceptances C --receipts J --view V` prints, after the decision lines, one line per receipt it checked, with the line of `receipts.jsonl` and the row it read, and then a closing line, that every receipt of the viewer's matches the view it accepted, or that one does not, at its line. With no receipt of the viewer's in the list, the view line says so: `view of <viewer>: no receipt of its own is in this list, so none was checked against the view it accepted`; the exit code is unchanged. A view that is not the one accepted is the command's error `founding` with `acceptance-view`, and prints nothing else, no validity code included.
- **One view per run.** `--view` takes one view per run. With `--open` of the viewer's own opening it is `usage`: an opening checks the viewer's seal and terms, and with every opening, every decision. With `--subset` it is `usage`: the view line speaks of every receipt of the viewer's, which a subset cannot hold for sure.
- A member checks the decisions it is a side of without any counterpart's opening, and a dispute needs the openings only for the rest. A view check of every member at once, and of the `answers` rows an answerer signed, are later work.

### A body and a delivery

- **A part's digest names its kind and its body's bytes,** not the form the body crossed in (crossing-3): a `text`, a `raw` and a `data` body with the same bytes have the same digest. A reader who needs the form takes it from the kind's own rule, the agreement of the parties on what a part of that kind holds, not from the receipt. Putting the form in the digest would change every digest, and a receipt would still not say what the bytes mean.
- **A part may hold `mediaType`** beside its body and `metadata`: lowercase `type/subtype` with no parameters, at most 127 characters a side, which no term reads and no digest covers, and which its delivery carries. A bad value, and any other member, is `one-body` (crossing-2).
- **A delivery is read as a delivery first.** Row 5 of verify-1 first reads the delivery as `delivery.json` shapes it: an object with `receiptId`, `receipt`, `messageId`, `kind` and `salt`, exactly one body, `text`, `raw` or `data`, and no member but those, `re`, `mediaType` and `answer`. A file that is not one fails `body` at index -1, and `verify` prints `error delivery`, exit 3, with no `body` line. A delivery an answer queued holds its answer call as `answer` (maybe-12), so a file that carries the call is still a delivery. The check is the one the mediator runs on a held delivery after a restart, in `core/internal/crossing`; no schema validator is added to the binary. The values inside the members are still row 5's: a salt or a kind that does not make the receipt's digest fails `body` at the receipt's index.
- **A delivery of no receipt.** A close is no receipt a delivery names. A delivery whose `receiptId` is empty or absent, or names no receipt of the list, fails `body` at index -1, and `verify` prints `error delivery`.
- **A delivery of a blocked part is not refused:** a delivery's bytes against a block's digest are still a fact a reader may want shown. The `body` line says the delivery's body bytes, under its salt, are the part its receipt names; it says neither the form nor that the part passed, and the receipt's own claim says whether the part passed.
- **The body line is printed whether or not the log conforms.** The `conforms` line keeps the first failure's code. The runner checks a vector's body only after its log conforms.

### An Accord with no receipt is founded at no time

With no receipt there is no time to found at, so no binding's `until` is checked. The rule is the core's: `conform` founds such a set at `0001-01-01T00:00:00Z`, and `placket verify` founds at the time `conform.FoundedAt` gives, so the command and the core cannot differ. The result of a verification must not depend on when it runs. A journal, or a subset, whose first receipt is after a binding's `until` still fails `founding`, a close that is the only receipt included (threat model L20).

### What verify says

- **Two claim lines.** `verify` prints `claim: each receipt is this mediator's signed decision about one part under this Accord's terms.` only when its `conforms` line is the full one, that every decision is one the Accord let its decider make: a founding set, this build's module, every opening, and no failure. Otherwise it prints `claim: each receipt and close is signed by this mediator.`: with `--mediator` alone, a card of another module, sealed terms, or a log or a body that does not conform. `verify --json`'s `claim` holds the line printed. The full line keeps its words whether or not the list ends in a close: the `complete` line is what speaks of a close.
- **Its counts.** `verify` prints its counts before its `valid`, `conforms` and `complete` lines. The member of `verify --json` that holds the number of receipts is `receiptCount`, beside `questionsClosed` and `blockedReleases`, and its judges are `judges`; `verify --json` has no `verified` member. In the commands' objects, `verified` is only ever the boolean of `show` and `released`. The vector format's `verify` expectation keeps its count `verified`: it is a frozen format, read by other implementations, and the name stops at the command line.
- **One party twice in `reconcile`.** A second kept record for a party already given fails `kept` at its index in the list of kept records, naming no party, before row 1: a refusal of section 9's row 0, beside the check that each record names its party by an actor reference. `reconcile` prints `error verify kept --kept N actor`. A party's records are one record, and two copies of it are a caller's mistake that a merge would hide. The vector format keys kept records by party, so this is a Go test, not a vector.

### A receipt's payload is its canonical bytes

- **The payload is strict.** A receipt verifies only when its payload is exactly the canonical bytes of its statement. A payload the canonical form accepts but would write otherwise, with a space after `{`, its members in another order or a number such as `seq` spelled `1.0`, fails section 8's row 1 with `not-canonical`, at that receipt's index, even when the mediator signed exactly those bytes. A close receipt follows the same rule, at the same row.
- **The envelope is loose.** A receipt's own bytes are read in any spelling the canonical form accepts: its members in any order, with any whitespace. Its `payload` string is read as written, and the id comes from the payload alone, so a re-encoded envelope verifies under the same id. A delivery carries its receipt as a JSON value and is re-encoded when it is read, so the envelope's bytes do not survive a delivery, while the payload string does. receipt-1 says so, and a vector pins it.
- **One code.** `not-canonical` keeps its row and has two meanings: bytes the canonical form refuses, and a receipt's payload that is not its own canonical form. No code is added.
- **Readers that do not verify follow the rule.** `receipts.Decode` and `receipts.DecodeClose`, which `placket show` uses, refuse such a payload with `not-canonical` (receipt-8). No path yields an id for a payload `verify` would refuse.
- The payload's base64 is taken only in its one encoding (row 3). The answer call requires its own canonical bytes and fails as `answer-call` (section 6, maybe-11).

### Its names

**`conform`** (verify-1 to verify-4)

- `Check`: validity, then conformance, over the evidence a verifier is given.
- `Evidence`: what a verifier is given: the Accord id, the mediator's reference, the founding set, its acceptances, the openings, the receipts, the answer calls, whether they are a subset, the trusted kernel hash and a view.
- `Evidence.Accord`, `Evidence.Mediator`, `Evidence.Founding`, `Evidence.Acceptances`, `Evidence.Openings`, `Evidence.Receipts`, `Evidence.Answers`, `Evidence.Subset`, `Evidence.Kernel` and `Evidence.View`: each of those, in that order; the mediator's reference is used only when no founding set is given.
- `Report`: what `Check` found: the verified subset, each receipt's decision, whether the card's module is trusted, the close, the operator, the members, the answer calls that match no receipt, whether the terms were checked, and the view rows read.
- `Report.Mediator`, `Report.Subset`, `Report.Decisions`, `Report.Module`, `Report.Close`, `Report.Operator`, `Report.OperatorPrincipalOf`, `Report.Members`, `Report.Unmatched`, `Report.Terms`, `Report.Viewer` and `Report.Viewed`: each of those, with the mediator the receipts were verified under, how many members the operator is principal of, and the viewer of the view given.
- `Decision`: one receipt's decider, its witness, and whether its rows were checked.
- `Decision.Index`, `Decision.By`, `Decision.Witness` and `Decision.Checked`: the receipt's index in the list, its decider, the witness of an answer's receipt, and whether its rows were checked.
- `Witness`, with `Witnessed`, `Unwitnessed` and `Contradicted`: what a receipt an answer decided says of its answer.
- `Viewed`: one receipt of the viewer's read against its view, with the row it read.
- `Viewed.Index`, `Viewed.Counterpart`, `Viewed.Dir`, `Viewed.Kind`, `Viewed.Outcome` and `Viewed.Matches`: the receipt's index, the row's counterpart, direction from the viewer, kind and outcome, empty when the view has no row for it, and whether it matches.
- `Body`: row 5, and the only place it runs.
- `FoundedAt`: the time a founding set is founded at for a list.
- `Code`, with `CodeFounding`, `CodeAccord`, `CodeParty`, `CodeDecision`, `CodeWitness`, `CodeBody`, `CodeAcceptanceView` and `CodeViewConformance`: the conformance codes, in the order of the table.
- `Error` and `Error.Error`: a receipt that does not conform, its code and index, with nothing of the evidence.
- `Error.Code` and `Error.Index`: the code of the first row that failed, and the receipt's index, -1 when no receipt is at fault.

**`receipts`**

- `VerifySubset` and `Subset`: a subset checked as `Verify` checks a whole list, whose chains may start late, and what it yields.
- `Subset.Verified`, `Subset.Places` and `Subset.Starts`: the verified statements, each receipt's place among the subset's questions, and the chains that start late.
- `Start`: a chain of a subset that starts after its first receipt.
- `Start.From`, `Start.To` and `Start.Seq`: the pair's from and to, and the seq of the pair's first receipt the subset holds, above 1.
- `RoleOutside`: a receipt whose question is not in the subset.

## Consequences

- A third party given only an Accord id, its founding set and a journal can say whether each decision it can read was one the Accord allowed, and which answers are witnessed by their answerer's own signature.
- The view check is as good as the view: a view states the outcome with no judgment, so a judge's decision is checked only as one the row lists to decide, never by what the judge said. A member that is only the receiver of a question checks that its closing fits the agreed rule, and not the answer's verdict toward the asker, which its view does not hold. A person checking two members runs verify twice.
- A `witnessed` line means a call given passed row 4, and nothing less; a `contradicted` line means one was given and did not. A view line never claims a receipt it did not read.
- A second implementation hashes a body's bytes alone, allows `mediaType` on a part, reads a delivery's shape before its body, founds a set with no receipt without checking `until`, and learns each from the text.
- One receipt id has one signed payload. A verifier in any language checks the signature over the payload's bytes and hashes those same bytes for the id, with no canonical form step between them that could disagree. A tool that rewrites a receipt's envelope, as a delivery does, keeps it verifiable; a tool that rewrites the decoded statement and re-signs it with the mediator's key makes a receipt that no longer verifies. Holding the envelope's bytes to canonical form too would refuse every receipt a delivery carried, and buys nothing, since the signature and the id read only the payload.
- `placket verify` on a journal with no receipt gives the same result whatever the clock says.
