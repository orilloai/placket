# 0002. The core: no dependency, no world, its packages and its names

Status: accepted. Date: 2026-11-23.

## Context

Placket's core decides what crosses between parties and writes the receipts that prove it. Anyone holding the published vectors must be able to check that core, and get the same bytes on every run, on every machine. Three things break that promise. A dependency brings code nobody in the protocol agreed to, and a version bump can change a byte. An import of an adapter brings sockets, files and processes with it. And a call that reads the world, such as the clock, the environment, a random source, a file, the network or the terminal, makes the output depend on when and where the code ran, which no vector can pin. Each is easy to add in a single line, and hard to remove once other code relies on it.

Every name the core exports is a promise a verifier, a door or a later change builds on (`trace-new-names`). A builder should not have to invent an entry point, and a reviewer should be able to check each one against a requirement.

## Decision

### No dependency and no world

- The core is its own Go module, `github.com/orilloai/placket/core`, in `core/`. Its `go.mod` names the module and the Go version (`go 1.27`, `toolchain go1.27.4`) and has no `require`. A dependency in the core needs a new decision record first, saying which code path uses it and why the standard library cannot.
- Nothing under `core/`, test files included, imports `github.com/orilloai/placket/adapters`, `github.com/orilloai/placket/cmd` or `github.com/orilloai/placket/recipes`, or any package below them.
- No code built into the core reads the clock, draws randomness, reads the environment, opens a file, touches the network or prints. What the core needs from the world, such as the time, a salt or a key, it takes as a parameter from its caller, an adapter. Test files may use any of these.
- The core is Go alone: no cgo and no assembly, C or other non-Go source, which could reach the world out of sight of any check of Go. No file under `core/` is a symbolic link, and no import path in it is written with an escape, so what the checks read is what the Go tool builds.
- `make ci` enforces each of these as a named check: `core-imports`, `core-no-deps` and `core-no-world`. Their names, and what each matches, are in `docs/rules/`.

### The packages

The capability packages depend one way, in this order: `canonical`, `signing`, `setup`, `composition`, `founding`, `receipts`, `answer`, `mediator`, `conform`. Each imports only packages before it, and `conformance`, the runner (record 0003), uses them all. None imports anything outside the standard library and the core.

- **`core/founding`** is a second package for section 3 of the requirement text, because row 21 of founding's table recomputes a view, which needs composition, and composition imports `setup`. The rows that do not need composition stay in `setup` (record 0006).
- **`core/answer`** holds the answer call (record 0009). It is a package of its own because the mediator, the conformance check, the export and the command line all read a call, and none of them is where the call is defined.
- **`core/conform`** holds conformance to the Accord (record 0013). It is a package of its own because it calls the decision module, the verifier and the reader of calls, and is none of them.
- **`core/internal/shape`** holds the format rules several packages read values by: the timestamp rule and the arithmetic of a deadline, labels, kinds, gates and role selectors, typed hashes and lowercase hex, standard base64 in its one encoding, whole numbers in the safe range, and the reading of bytes into JSON values through the canonical form and the writing of them back. It decides no result: each caller turns a value that breaks a rule into the code of its own check. Without it each package would carry its own copy of the timestamp rule, and two copies drift.
- **`core/internal/crossing`** holds what crossing-3 asks of a delivery, written once: building it, checking a restored one against its receipt, decoding a part's body and salt, and reading a file as a delivery. The mediator and `conform` call it, so a restored delivery and an admitted one are hashed by the same code, and `mediator` keeps only state (`mediator-no-logic`).
- **`core/internal/testing/`** (package `ptesting`) holds the helpers the core's tests share, the vectors' keys and founding sets (`test-helpers-placed`), and **`core/internal/testing/seeds`** reads the vectors for fuzz seeds. `seeds` imports nothing of the core, so the tests of `signing` and `shape`, which `ptesting` imports, can use it too. Only tests import either, so they may read files, and `core-no-world` refuses a file built into the core that imports them.

The internal packages export no name outside the core module, and this record is their reason (`packages-by-adr`).

### Names

Names are the protocol text's words (`spec-words`). The canonical form's entry point is `Form`, since the text says a value is put in canonical form, and its hash is `TypedHash`, the text's typed hash. Each code is a constant spelled as the text spells it. Each exported name has a reason in a record, which lists it under its package with a clause of purpose; this record lists the names of the founding capabilities, and records 0003 and 0006 to 0013 list the rest.

**`canonical`** (canonical-1 to canonical-11)

- `Form`: the canonical bytes of a value, or a refusal.
- `TypedHash`: `sha256:` and the hex SHA-256 of the canonical bytes.
- `Less`: the order of UTF-16 code units, for member names and actor references.
- `HashPrefix` and `MaxDepth`: the typed hash's prefix and the deepest nesting accepted.
- `Code`, with `InvalidJSON`, `InvalidString`, `DuplicateKey`, `FractionalNumber` and `UnsafeInteger`: the five refusal codes.
- `Error` and `Error.Error`: a refusal, its code and offset, with nothing of the input.
- `Error.Code` and `Error.Offset`: the refusal's code, and the byte offset it was found at.

**`signing`** (signing-1, signing-2, signing-4)

- `Key`, `NewKey` and `Key.Reference`: an actor's private key from its seed, and its actor reference.
- `Key.Format`, `Key.GoString` and `Key.String`: a key prints as a fixed text (`secrets-never-print`).
- `ParseReference`: the form of an actor reference.
- `PAE`: DSSE 1.0's pre-authentication encoding.
- `SigningBytes`, `Sign` and `Verify`: an object's signing bytes, signature and check, under one of placket's object type URIs.
- `SignPayload` and `VerifyPayload`: a payload signature under any payload type but placket's type URIs.
- `TypeDeclaration`, `TypeBinding` and `TypeEnvelope`: the type URIs of a declaration, a binding and an envelope.
- `ReferencePrefix`: the prefix of every actor reference.
- `Code`, with `CodeSeed`, `CodeTypeURI` and `CodeObject`, and `Error` and `Error.Error`: a caller's misuse. Signing has no protocol code; the operation that checks a signature returns its own.
- `Error.Code`: the misuse's code.

`Verify` refuses a key that cannot sign (signing-1) before it checks the equation: the bytes must be the canonical encoding of their point, which is not one of the eight points of small order. The equation itself is the standard library's `crypto/ed25519`, which is cofactorless, compares R as bytes and refuses an S at or above the group order.

**`setup`** (setup-contract-1 to setup-contract-10)

- `Found`: the rows of founding's table that need no composition, in order.
- `Accord`: a founded set, whose reads return copies and take a nil Accord as one with no members.
- `Accord.ID`, `Accord.Card`, `Accord.Mediator` and `Accord.Operator`: the Accord id, the card hash, the mediator's reference and the operator's.
- `Accord.Members`, `Accord.Member`, `Accord.Roles` and `Accord.Terms`: the members, membership, a member's roles and its terms.
- `Term` and `Rule`: a term as founding read it, and a maybe term's resolve.
- `Term.Stance`, `Term.Dir`, `Term.With`, `Term.Kinds` and `Term.Resolve`: a term's stance, its direction, the role labels of its selectors, the kinds its select names, and its resolve, nil but on a maybe term.
- `Rule.To` and `Rule.Timeout`: the role labels of who answers, and the timeout in seconds.
- `StanceNeed`, `StanceWant`, `StanceMaybe`, `StanceNo`, `DirGive` and `DirGet`: a term's stances and directions.
- `Code`, with the founding codes `NotCanonical`, `InvalidShape`, `BindingDuplicate`, `ActorReference`, `DeclarationSignature`, `TermInvalid`, `MaybeWithoutResolve`, `BindingMissing`, `BindingPrincipal`, `BindingSignature`, `BindingMediator`, `BindingExpired`, `DeclarationMissing` and `RoleMissing`: founding's refusals, spelled as its table spells them.
- `CodeAt`: a founding time that breaks the timestamp rule, which is not a founding code.
- `Error` and `Error.Error`: a refused founding, its code, with nothing of the set.
- `Error.Code`: the code of the first row that failed.

A declaration's terms are sealed, and founding reads them once, from the opening that opens the seal (record 0006): it validates them (row 6), so it reads them into `Term` there, and composition reads them from the Accord. A second reader of one format elsewhere would drift.

**`composition`** (composition-1 to composition-6)

- `Decide`: the one entry point of the decision module (`one-decision-module`).
- `Crossing` and `Decision`: what composition decides, and its verdict, reason and agreed rule.
- `Crossing.From`, `Crossing.To`, `Crossing.Kind` and `Crossing.Formed`: the sender, the receiver, the part's kind, and whether the mediator formed the part from an answer.
- `Decision.Verdict`, `Decision.Reason` and `Decision.Rule`: the verdict, the reason, and for an escalate or a judged pass or block the rule every matching maybe term agreed on.
- `Verdict`, with `Pass`, `Block` and `Escalate`: the verdicts.
- `ReasonOK`, `ReasonNoMatch` and `ReasonMaybe`: the reasons composition returns.

No interface stands for the module, and there is no second one: `core/conform` runs `Decide` again, and only under the kernel hash the verifier trusts (record 0013).

**`receipts`** (receipt-1 to receipt-10, crossing-5)

- `Build`: a receipt, signed, and its verified statement. Its time, the subject's digest and the key are its inputs (`inputs-as-parameters`); a statement section 8's row 4 would refuse is not written.
- `Digest`: the salted digest of crossing-3.
- `Verify`: section 8, every row of one receipt before the next.
- `Verified`, `Verified.Statement`: a statement only verification, or this package's own signing, can make, and a copy of it (`verified-by-type`).
- `Releases`, `Place`, `Role`, `RoleGive`, `RoleQuestion`, `RoleClosing` and `RoleRelease`: receipt-6's rule for questions, and each receipt's place under it.
- `Place.Role` and `Place.Question`: a receipt's role, and the index of its question's receipt for every role but a give, a receipt outside the subset and a close.
- `Reconcile`, `Report`, `Kept`, `Item` and `Missed`: section 9, a party's kept records and what is in one record and not the other.
- `Kept.Actor`, `Kept.Envelopes` and `Kept.Delivered`: the party, the envelopes it signed, and the receipts of the deliveries it fetched, in the order kept.
- `Report.Removed`, `Report.Unrecorded` and `Report.Missing`: reconciliation's three lists, parties in actor-reference order.
- `Item.Actor` and `Item.ReceiptID`: a party, and a receipt in its list by id.
- `Missed.Actor` and `Missed.Seq`: a party, and the seq of a kept envelope no presented receipt echoes.
- `Decode`, `Show` and `NotVerified`: reading a receipt without verifying it, and the first line `Show` writes.
- `Subject`, `Envelope`, `Predicate` and `Statement`: what a receipt names, echoes, states and is.
- `Subject.Name` and `Subject.Digest`: the part's kind and its salted digest.
- `Envelope.Sender`, `Envelope.Seq` and `Envelope.Hash`: the echoed envelope's signer, its seq and its hash.
- `Predicate.Accord`, `Predicate.Card`, `Predicate.Mediator`, `Predicate.Kind`, `Predicate.From`, `Predicate.To`, `Predicate.Seq`, `Predicate.Prev`, `Predicate.At`, `Predicate.Envelope`, `Predicate.Verdict` and `Predicate.Reason`: the members of receipt-3's predicate, with `Prev` empty exactly when `Seq` is 1.
- `Statement.ID`, `Statement.Subject` and `Statement.Predicate`: the receipt's id, the typed hash of its statement's canonical bytes, and what the statement names and states.
- `PayloadType`, `StatementType` and `PredicateType`: a receipt's fixed values.
- `KindCrossing` and `KindResolution`: the kinds of receipt.
- `ReasonDeclined`, `ReasonCancelled`, `ReasonTimeout` and `ReasonUndeliverable`: a resolution receipt's reasons.
- `NameAnswer` and `NameChanged`: the subject names of an answer and a changed version.
- `Code`, with `CodeReference`, `CodeNotCanonical`, `CodePayloadType`, `CodeSignature`, `CodeStatementType`, `CodeMediator`, `CodeChain`, `CodeRelease`, `CodePresented`, `CodeKept` and `CodeStatement`: the verifier's and reconciliation's codes, and a misuse of `Build`.
- `Error` and `Error.Error`: a failed verification or reconciliation, its code, index and party.
- `Error.Code`, `Error.Index` and `Error.Actor`: the code, the index of the receipt it failed at, -1 for `reference` and a misuse of `Build`, and for `kept` the party.

`Releases` is the one place a question's receipts and its release are recognised (`release-in-resolve`); `Verify` and `Reconcile` call it. `Verify` runs rows 1 to 6 to the first receipt that fails them, then the question rule over the receipts before it, so a release failure found there comes before the later failure, and the order of section 8 holds. `Verify`, `Releases` and `Build` take or return `Verified`, so a `Statement` from `Decode` cannot be handed to `Releases`.

`Decode` and `Show` verify no signature, mediator, chain or question, but hold every predicate member to its form, as section 8's rows 1 and 4 read it, then the predicate's `mediator` as an actor reference, then its `seq` and `prev` as receipt-3 and receipt-10 give them. A payload must be its statement's canonical bytes there too (record 0013). A receipt refused there is refused at index 0 with the code the verifier gives the row that refused it, so no unchecked text reaches the lines `Show` writes; `Show`'s first line is `NotVerified`.

`Digest(salt, body)` keeps the order of crossing-3's formula, which hashes the salt before the body: an exception to `inputs-as-parameters`, which puts the salt after the data, since changing it would make every caller read against the requirement text.

## Consequences

- Every function in the core is a function of its inputs. The same inputs give the same receipts on two machines, and a vector can state them byte for byte.
- An adapter does the work of the world: it reads the clock once per call, mints nonces, reads key files and prints. That puts more code in the adapters, and it is the right place for it.
- A verifier can build against the core alone and needs nothing else from this repository.
- Each exported name traces to a requirement id or to a record, and its doc comment names it. The core grows by adding names to a record first.
- `shape` and `crossing` are second places to look for a rule. Their package comments say they decide nothing, and a reviewer holds them to that.
