# 0025. Each party's terms are sealed in the founding set, and each party signs its view before the Accord is founded

Status: accepted. Date: 2026-11-18. Amends records 0005, 0006, 0008, 0010, 0011, 0019, 0020, 0021, 0022 and 0024. Amended by record 0026.

## Context

Until this record the founding set carried every declaration in the clear, and every party held the set: to found, to verify and to name the Accord. So each party read every term of every other party, its `no` terms included, and a party's `no` terms are a map of what it guards. Nothing in the protocol needs that. A party needs to know what will cross for it, and a verifier needs to know that the receipts are the mediator's and that each one names the Accord its parties founded.

The Accord id is the typed hash of the set, and placket's formats freeze at their first release, so a change to what the set holds must come before it.

## Decision

### The seal

Each declaration in the set is replaced by its seal: `{"actor","seal"}`, where `seal` is `sha256:` followed by the 64 lowercase hex of the SHA-256 of a 32-byte salt and then the declaration's canonical bytes, its `sig` included. That is crossing-3's salted digest, written in the form `hash.json` reads. The actor mints the salt. `declarations` stays a list in strict actor-reference order (setup-contract-10). Policy, mediator, card and bindings stay in the clear: roles, principals and the module are what a verifier reads, and bindings name roles a counterpart must know.

The opening is a file holding exactly `{"declaration","salt"}`, the salt in lowercase hex, named `<hex of the actor reference>.opening.json` and written with mode 0600. It goes only to whoever runs the window, as a file. The Accord id is still the typed hash of the set, so every term stays committed by the id. The declaration keeps its own signature, so an opened seal proves the terms are the actor's.

Every declaration is sealed, in every Accord, the one-party case included, so the set has one shape.

The window's operator holds every opening and so reads every term. In version 0.1 the operator runs the mediator, which must read every term to decide (threat model, non-goals). The seal hides terms from every other party.

### The view and the acceptance

Before the Accord is founded, each member is shown its view (`view.json`, a hashed format) and signs it. A view is built from the viewer's own terms only:

- `crossings`: for each term of the viewer, each kind it selects and each member whose binding gives a role one of its `with` selectors names, one row: the counterpart's reference and its roles as its binding gives them, `dir` from the viewer, `kind`, and `outcome`. The outcome is composition's decision of that crossing with no judgment and not formed: `crosses` for a pass, `blocked` for a block, with no reason, and `asked` for an ask, with the agreed rule's `to` (role selectors, sorted), `timeout`, and `judges`, the judges both sides consented to (composition-7), when there are any. A kind only the other side names never appears. A `blocked` row never says whether the other side refused or said nothing, and maybe rules that differ show as `blocked`.
- `answers`: every crossing between two other members that composition asks about and whose agreed rule's `to` names a role of the viewer: `asker`, `receiver`, `kind` and `timeout`.
- `accord`, the Accord id; `viewer`; and `salt`, the viewer's own opening salt, so that the view's typed hash, which every verifier holds, cannot be matched against views guessed from the clear set.

Rows are sorted by counterpart in actor-reference order, then `dir`, then `kind`, both in byte order; answers by asker, receiver and kind. So the same openings give the same bytes.

The acceptance is a new signed object, `{"accord","view","from","sig"}`, `view` the typed hash of the viewer's view, signed by `from` under a fifth type URI, `https://placket.tech/acceptance/0.1` (signing-4). `acceptances.json` is the list of every member's acceptance, in strict `from` order, kept beside the founding set. There is no deadline and no decline: a member that does not consent does not accept, and a set never fully accepted leaves no record and no receipt.

### Founding

Section 3's table gains row 4a, after the actor row: an opening is held for the entry (`opening-missing`); it holds exactly `declaration` and `salt`, the salt 32 bytes, the declaration an object with no member but `actor`, `terms` and `sig` and its `actor` a string, and the canonical form accepts it (`invalid-shape`); the declaration's `actor` is the entry's, and its salted digest is the seal (`seal-mismatch`). Rows 5 to 8 then read the opened declaration. Rows 18 to 21 follow row 17: the acceptance list read whole (`not-canonical`, `invalid-shape`), then per member in actor-reference order, all of one member's rows before the next's: an acceptance from it (`acceptance-missing`), its signature (`acceptance-signature`), its `accord` (`acceptance-accord`), and, with the openings, its `view` against the typed hash of the member's recomputed view (`acceptance-view`). The founding time of row 15 is, for the window, the time the last acceptance is held, and for a verifier the first receipt's `at`, as before.

A founder without the openings, any party or verifier, runs every row but 4a, 5 to 8 and 21, and the result says the terms were not checked. A verifier given some openings, as in a dispute, runs rows 4a to 8 for the members they open, and row 21 only when every member is opened, since a view reads its counterparts' terms.

### The core's names

- **`signing`**: `TypeAcceptance`, accepted by `Sign` and `Verify`, refused as a payload type.
- **`setup`**: `Found(set []byte, openings Openings, at string) (*Accord, error)` runs rows 1 to 17; its signature changes. `type Openings struct { ByActor map[string][]byte; Some bool }`: no openings when `ByActor` is nil and `Some` false, every member's when `ByActor` is set, some members' when `Some` is set. `Seal(declaration, salt []byte) (entry, opening []byte, err error)` builds a seal entry and its opening. `(*Accord).Opened(actor) bool`, `TermsChecked() bool` and `Salt(actor) []byte`. The codes `OpeningMissing`, `SealMismatch`, `AcceptanceMissing`, `AcceptanceSignature`, `AcceptanceAccord` and `AcceptanceView`.
- **`composition`**: `Sides(a *setup.Accord, member string) []Side` and `Answers(a *setup.Accord, member string) []Asked`, the view's rows, in `view.go`, outside the file the kernel hash reads, built on `Decide` and the one term matcher. `type Side struct { Counterpart, Dir, Kind string; Decision Decision }`, `type Asked struct { Asker, Receiver, Kind string; Timeout int64 }`.
- **`founding`**, a new package: `type Inputs struct { Set []byte; Openings setup.Openings; Acceptances []byte }`; `Found(in Inputs, at string) (*setup.Accord, error)`, rows 1 to 21; `View(a *setup.Accord, viewer string) ([]byte, error)`, the view's canonical bytes; `Accept(accord string, view []byte, key signing.Key) ([]byte, error)`, an acceptance; `Accepted(a *setup.Accord, acceptance []byte) (string, error)`, rows 19 to 21 for one acceptance, returning its `from`; `Acceptances(list [][]byte) ([]byte, error)`, the list in `from` order. A refusal is a `*setup.Error`. The package exists because row 21 needs composition, which imports `setup`.
- **`mediator`**: `Found(in founding.Inputs, key, at)`, `Reload(in founding.Inputs, …)` and `Close(in founding.Inputs, …)` take the inputs in place of the set's bytes.
- **`conform`**: `Evidence` gains `Openings setup.Openings` and `Acceptances []byte`. Without openings, row 3 is skipped and row 4 checks a decline's signer as a member, as when the module is not trusted; with some, row 3 runs for a receipt only when every member whose terms it reads is opened. `Report` gains `Terms`, whether every member was opened.

### The window and the door

`placket window --founding S --openings DIR` with an empty journal starts a proposed Accord: it runs rows 1 to 17 with every opening and serves only two new calls of the mediator API, `view`, which returns the caller's own view, and `accept`, which takes the caller's signed acceptance and checks it by rows 19 to 21; an acceptance that fails is refused with that row's founding code. Every other op is refused with the door code `not-founded`. Once every member has accepted, the window founds at that time, writes `acceptances.json` beside the founding set, journals the founding, prints its founding line and serves as before. Started with `--acceptances A`, or with an `acceptances.json` beside the founding set, it is founded at once, on an empty journal as on a founded one; a founded journal with neither is refused `acceptance-missing`. `placket mcp` lists neither call.

### The command line

- `placket seal --key K --declaration D [--out DIR]` prints the seal entry and writes the opening.
- `placket founding` takes `--seal E` entries in place of `--declaration`.
- `placket view` and `placket accept`, with the socket or label flags, show the view in the product's words, keep it as `view.json` in the member's state directory, and sign the acceptance, kept as `acceptance.json`.
- `placket verify --accord A --founding S --acceptances C --receipts J` founds without the openings and prints that the terms were not checked; `--open O`, once or more, checks each opening, recomputes views when every member is opened, and runs conformance on what the openings cover.
- `placket mcp` refuses a label `answering-key` when its accepted view has an `answers` row or an asked row whose `to` names one of its roles, and `founding` when it has no accepted view.

## Consequences

- Every Accord id, every vector and the demo journal change. No verdict, reason or receipt format changes: the same terms decide the same crossings.
- A view shows the agreed rule of an asked row, which may be the other side's alone; a `crosses` row shows the other side has a matching term; an answerer's `answers` rows show who asks whom about what, before any question is opened. Probing still shows terms: each probe is a signed receipt the other side reads, and an escalation shows a `maybe` covered that kind. The threat model says so.
- A view can be recomputed only with the openings of every member it reads, so one member's opening proves its seal and its terms, not its view, and a dispute over a view needs the counterparts' openings: each counterpart holds its own, and the operator holds them all. We accept that narrowing rather than add a partial check here. Checking a member's kept view against its own terms with its opening alone, and checking receipts against a signed view, are later work.
- The founding package is a second package for section 3. Its rows that do not need composition stay in `setup`.
