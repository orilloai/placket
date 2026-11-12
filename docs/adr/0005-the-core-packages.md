# 0005. The core's packages and every name they export

Status: accepted. Date: 2026-11-12.

## Context

The core gets its capability packages: the canonical form, signing, founding, composition, receipts and the mediator. Each is named for the protocol capability it implements (`package-per-capability`), and every name one exports is a promise a verifier, a door or a later milestone builds on (`trace-new-names`). A builder should not have to invent an entry point, and a reviewer should be able to check each one against a requirement (`change-is-one-capability`). This record lists them all, by package and signature, before the code.

Building the core implements the protocol text as it stands; it is not itself a protocol change. A correction to the text, if building shows one is needed, is its own change.

## Decision

### Dependencies

The packages depend one way, in this order: `canonical`, `signing`, `setup`, `composition`, `receipts`, `mediator`; `conformance` uses them all. None imports anything outside the standard library and the core.

One package is added beyond the capabilities: `core/internal/shape`, which holds the format rules several packages read values by: the timestamp rule and the arithmetic of a deadline, labels, kinds, gates and role selectors, typed hashes and lowercase hex, standard base64 in its one encoding, whole numbers in the safe range, and the reading of bytes into JSON values through the canonical form and the writing of them back. It decides no result: each caller turns a value that breaks a rule into the code of its own check. It is internal, so nothing outside the core can depend on it. Without it each package would carry its own copy of the timestamp rule, and two copies drift.

### Names

Names are the protocol text's words (`spec-words`). The canonical form's entry point is `Form`, since the text says a value is put in canonical form, and its hash is `TypedHash`, the text's typed hash. Each code is a constant spelled as the text spells it.

**`canonical`** (canonical-1 to canonical-11)

- `func Form(input []byte) ([]byte, error)`: the canonical bytes, or a refusal.
- `func TypedHash(input []byte) (string, error)`: `sha256:` and the hex SHA-256 of the canonical bytes.
- `func Less(a, b string) bool`: the order of UTF-16 code units, for member names and actor references.
- `type Code string`, with `InvalidJSON`, `InvalidString`, `DuplicateKey`, `FractionalNumber`, `UnsafeInteger`.
- `type Error struct { Code Code; Offset int }`, and its `Error() string`.
- `const HashPrefix = "sha256:"`, `const MaxDepth = 1000`.

**`signing`** (signing-1, signing-2, signing-4)

- `type Key struct` (unexported fields), `func NewKey(seed []byte) (Key, error)`, `func (Key) Reference() string`; `Format`, `GoString` and `String` print a fixed text (`secrets-never-print`).
- `func ParseReference(ref string) (ed25519.PublicKey, bool)`: the form of a reference.
- `func PAE(payloadType string, payload []byte) []byte`: DSSE 1.0's pre-authentication encoding.
- `func SigningBytes(object []byte, typeURI string) ([]byte, error)`, `func Sign(object []byte, typeURI string, key Key) (string, error)`, `func Verify(object []byte, typeURI string, signer string) bool`: objects under the declaration, binding and envelope type URIs only.
- `func SignPayload(payloadType string, payload []byte, key Key) ([]byte, error)`, `func VerifyPayload(payloadType string, payload, sig []byte, signer string) bool`: any payload type but signing-4's four.
- `const TypeDeclaration, TypeBinding, TypeEnvelope, TypeRegistration`, `const ReferencePrefix = "ed25519:"`.
- `type Code string`, with `CodeSeed`, `CodeTypeURI`, `CodeObject`, and `type Error struct { Code Code }`: a caller's misuse. Signing has no protocol code; the operation that checks a signature returns its own.

`Verify` refuses a key that cannot sign (signing-1) before it checks the equation: the bytes must be the canonical encoding of their point, which is not one of the eight points of small order. The equation itself is the standard library's `crypto/ed25519`, which is cofactorless, compares R as bytes and refuses an S at or above the group order.

**`setup`** (setup-contract-1 to setup-contract-10)

- `func Found(set []byte, at string) (*Accord, error)`: the rows of founding's table, in order.
- `type Accord struct` with `ID() string`, `Card() string`, `Mediator() string`, `Members() []string`, `Member(actor string) bool`, `Roles(actor string) []string`, `Terms(actor string) []Term`. Every read returns a copy.
- `type Term struct { Stance, Dir string; With, Kinds []string; Checks []Check; Resolve *Rule }`, `type Check struct { Name, Corpus string }`, `type Rule struct { To []string; Timeout int64 }`, and the stance and direction constants.
- `type Code string`, with the fifteen founding codes, and `CodeAt` for a founding time that breaks the timestamp rule, which is not a founding code; `type Error struct { Code Code }`.

Founding validates a declaration's terms (row 6), so it reads them into `Term` there and composition reads them from the Accord. Reading them a second time elsewhere would be a second reader of one format.

**`composition`** (composition-1 to composition-6)

- `func Decide(a *setup.Accord, c Crossing) Decision`: the one entry point of the decision module (`one-decision-module`).
- `type Crossing struct { From, To, Kind string; Formed bool }`, `type Decision struct { Verdict Verdict; Reason string; Rule setup.Rule }`.
- `type Verdict string`, with `Pass`, `Block`, `Escalate`; `ReasonOK`, `ReasonNoMatch`, `ReasonMaybe`.

No interface stands for the module, and there is no second one.

**`receipts`** (receipt-1 to receipt-10, crossing-5)

- `func Build(subject Subject, predicate Predicate, key signing.Key) ([]byte, string, error)`: the receipt and its id. Its time, the subject's digest and the key are its inputs (`inputs-as-parameters`); a statement row 4 would refuse is not written.
- `func Digest(salt, body []byte) string`: the salted digest of crossing-3.
- `func Verify(mediator string, list [][]byte) ([]Statement, error)`: section 8, every row of one receipt before the next.
- `func Releases(statements []Statement) ([]Place, error)`: receipt-6's rule for questions, the one place a question's receipts and its release are recognised (`release-in-resolve`). `Verify` and `Reconcile` call it.
- `func Reconcile(mediator, accord, card string, presented [][]byte, kept []Kept) (Report, error)`: section 9.
- `func Decode(receipt []byte) (Statement, error)` and `func Show(receipt []byte) ([]string, error)`: reading without verifying (receipt-8); `Show`'s first line is `NotVerified`.
- `type Subject`, `Envelope`, `Predicate`, `Statement`, `Place`, `Role` (with `RoleGive`, `RoleQuestion`, `RoleClosing`, `RoleRelease`), `Kept`, `Item`, `Missed`, `Report`.
- `type Code string`, with the verifier's and reconciliation's codes and `CodeStatement`, a misuse of `Build`; `type Error struct { Code Code; Index int; Actor string }`.
- `const PayloadType`, `StatementType`, `PredicateType`, `KindCrossing`, `KindResolution`, `ReasonDeclined`, `ReasonCancelled`, `ReasonTimeout`, `ReasonUndeliverable`, `NameAnswer`, `NameChanged`.

`Verify` runs rows 1 to 6 to the first receipt that fails them, then `Releases` over the receipts before it. Row 7 of a receipt reads only the receipts up to it, so a release failure found there comes before the later failure, and the order of section 8 holds.

**`mediator`**: record 0006.

**`conformance`** (record 0004) gains no exported name but a field: `Open.Rest`, what stays open of a coverage entry covered in part. `Default()` now returns a handler for each of the seven operations.

## Consequences

- Each exported name traces to a requirement id or to this record, and a doc comment names it.
- The core grows by adding names here first. M2's answers, deadlines and reload add to `mediator` (record 0006) and call `receipts.Releases`; they need no new capability package.
- `shape` is a second place to look for a format rule. Its package comment says it decides nothing, and a reviewer holds it to that.
