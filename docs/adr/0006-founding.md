# 0006. Founding: a principal per party, sealed terms and signed views

Status: accepted. Date: 2026-11-23.

## Context

An actor holds a role only by a binding: a principal's signed statement that the actor holds some roles under one mediator until a time (setup-contract-2). One person running several actors needs one principal behind every binding. An Accord between sides with different owners needs each owner to vouch for its own side only, and to say which side may supply which role: with a flat list of principals, one side's principal could bind a key of its own as the other side's `decider`, and founding would accept it.

The founding set is what every party holds: to found, to verify and to name the Accord. If it carried every declaration in the clear, each party would read every term of every other party, its `no` terms included, and a party's `no` terms are a map of what it guards. Nothing in the protocol needs that. A party needs to know what will cross for it, and a verifier needs to know that the receipts are the mediator's and that each one names the Accord its parties founded. And a party should consent, by its own signature, to what the merged terms make of its own crossings before the Accord is founded.

## Decision

### A principal per party

- **A policy entry per principal.** `policy.principals` is a non-empty list of entries, each with exactly `principal`, an actor reference, and `roles`, the labels that principal may grant: at least one, none repeated. No two entries name one principal. Two entries may list the same role, so two sides may each supply, say, a reviewer. `policy.roles` are the roles that must be held; a role may be grantable without being required. A required role no entry lists can never be held, and its set ends in `role-missing`, with no earlier refusal of shape.
- **A principal is scoped to roles, not to actors.** A binding may give only roles its principal's entry lists; otherwise founding refuses it with `binding-role`. The policy does not list the actors a principal may bind. The actors of an Accord are its declarations; the policy is written before anyone knows every key a side will use, and a side rotates its own keys without the other side's say. What the other side needs to agree to is which roles a principal may fill, and that is what the entry says.
- **A binding commits to its policy.** A binding names `policy`, the typed hash (canonical-3) of the canonical bytes of the founding set's whole `policy` object. The principal's signature covers it, so a binding counts only under the policy its principal signed for; one signed for another policy is refused with `binding-policy`. Without it, a binding signed for one Accord's policy could be reused in a set whose policy gives its principal more, or gives another principal the same roles. Hashing the whole policy, `policy.roles` included, means each principal agrees to every other principal's grant: adding a party or changing any grant means every principal signs its bindings again. That is the price of a binding that cannot be carried into a policy its principal never saw.
- **The order of a binding's checks:** principal listed (`binding-principal`, read against the entries' `principal` members), signature, `binding-role`, `binding-policy`, mediator, time. The two policy rows run after the binding's signature, so a binding whose signature fails never reaches them; the role comes before the policy because it is the narrower statement about what this principal may do.
- **Who stands behind each member is visible.** `setup` keeps, for each member, the principal that signed its binding. `verify --founding` prints one line per member, in actor-reference order, after the module's lines: `member <actor> roles <r1,r2> principal <principal>`, and its `--json` object holds the same as `members`. Receipts name actors, and the founding set says who stands behind them.
- **`placket bind` requires `--policy FILE`.** It reads the policy, puts it in canonical form and writes its typed hash into the binding; a file it cannot read, or that the canonical form refuses, is its `file` error with the detail `policy`. It does not check that the policy lets the principal grant the roles it writes: founding does, and one place judges it.
- **What it does not do.** No signature by a principal over the founding set or a declaration: a principal is not a member unless its key also declares. No second binding per actor: one actor has one binding, and so one principal (`binding-duplicate`). No delegation, vouching, admitting joiners, revocation, `principal:<id>` selectors in a term, or reveal levels for principals; nothing is reserved for them.

### Sealed terms

- **The seal.** Each declaration in the set is replaced by its seal, `{"actor","seal"}`, where `seal` is `sha256:` followed by the 64 lowercase hex of the SHA-256 of a 32-byte salt and then the declaration's canonical bytes, its `sig` included: crossing-3's salted digest, written in the form `hash.json` reads (setup-contract-12). The actor mints the salt. `declarations` stays a list in strict actor-reference order (setup-contract-10). Policy, mediator, card and bindings stay in the clear: roles, principals and the module are what a verifier reads, and bindings name roles a counterpart must know. Every declaration is sealed, in every Accord, the one-party case included, so the set has one shape.
- **The opening** is a file holding exactly `{"declaration","salt"}`, the salt in lowercase hex, named `<hex of the actor reference>.opening.json` and written with mode 0600 (setup-contract-13). It goes only to whoever runs the window, as a file, and the window reads it as it reads a key file, with a size cap. The Accord id is still the typed hash of the set, so every term stays committed by the id. The declaration keeps its own signature, so an opened seal proves the terms are the actor's.
- **Who reads the terms.** The window's operator holds every opening and so reads every term: in this version the operator runs the mediator, which must read every term to decide. The seal hides terms from every other party.

### The view and the acceptance

- **The view.** Before the Accord is founded, each member is shown its view (`view.json`, a hashed format; setup-contract-14) and signs it. A view is built from the viewer's own terms only:
  - `crossings`: for each term of the viewer, each kind it selects and each member whose binding gives a role one of its `with` selectors names, one row: the counterpart's reference and its roles as its binding gives them, `dir` from the viewer, `kind`, and `outcome`. The outcome is composition's decision of that crossing with no judgment and not formed: `crosses` for a pass, `blocked` for a block, with no reason, and `asked` for an ask, with the agreed rule's `to` (role selectors, sorted), `timeout`, and `judges`, the judges both sides consented to (composition-7), when there are any. A kind only the other side names never appears. A `blocked` row never says whether the other side refused or said nothing, and maybe rules that differ show as `blocked`.
  - `answers`: every crossing between two other members that composition asks about and whose agreed rule's `to` names a role of the viewer: `asker`, `receiver`, `kind` and `timeout`.
  - `accord`, the Accord id; `viewer`; and `salt`, the viewer's own opening salt, so that the view's typed hash, which every verifier holds, cannot be matched against views guessed from the clear set.

  Rows are sorted by counterpart in actor-reference order, then `dir`, then `kind`, both in byte order; answers by asker, receiver and kind. So the same openings give the same bytes.
- **The acceptance** is a signed object, `{"accord","view","from","sig"}`, `view` the typed hash of the viewer's view, signed by `from` under its own type URI, `https://placket.tech/acceptance/0.1` (signing-4, setup-contract-15). `acceptances.json` is the list of every member's acceptance, in strict `from` order, kept beside the founding set. There is no deadline and no decline: a member that does not consent does not accept, and a set never fully accepted leaves no record and no receipt.

### Founding

- Section 3's table has row 4a, after the actor row: an opening is held for the entry (`opening-missing`); it holds exactly `declaration` and `salt`, the salt 32 bytes, the declaration an object with no member but `actor`, `terms` and `sig` and its `actor` a string, and the canonical form accepts it (`invalid-shape`); the declaration's `actor` is the entry's, and its salted digest is the seal (`seal-mismatch`). Rows 5 to 8 then read the opened declaration. Rows 18 to 21 follow row 17: the acceptance list read whole (`not-canonical`, `invalid-shape`), then per member in actor-reference order, all of one member's rows before the next's: an acceptance from it (`acceptance-missing`), its signature (`acceptance-signature`), its `accord` (`acceptance-accord`), and, with the openings, its `view` against the typed hash of the member's recomputed view (`acceptance-view`).
- **The founding time** of row 15 is, for the window, the time the last acceptance is held, and for a verifier the first receipt's `at`. With no receipt, no binding's `until` is checked (setup-contract-16; record 0013).
- **A founder without the openings,** any party or verifier, runs every row but 4a, 5 to 8 and 21, and the result says the terms were not checked. A verifier given some openings, as in a dispute, runs rows 4a to 8 for the members they open, and row 21 only when every member is opened, since a view reads its counterparts' terms.
- One member's opening proves its seal and its terms, not its view. A member checks its own receipts against the view it accepted, with no counterpart's opening (record 0013).

### The command line

- `placket seal --key K --declaration D [--out DIR]` prints the seal entry and writes the opening.
- `placket founding` takes `--seal E` entries for the declarations.
- `placket view` and `placket accept`, with the socket or label flags, show the view in the product's words, keep it as `view.json` in the member's state directory, and sign the acceptance, kept as `acceptance.json`.
- `placket verify --accord A --founding S --acceptances C --receipts J` founds without the openings and prints that the terms were not checked; `--open O`, once or more, checks each opening, recomputes views when every member is opened, and runs conformance on what the openings cover.

### Its names

**`founding`** (section 3, setup-contract-12 to setup-contract-16)

- `Inputs`: what founding reads: the sealed set, the openings and the acceptances.
- `Inputs.Set`, `Inputs.Openings` and `Inputs.Acceptances`: the sealed set's bytes, the openings its founder holds, and `acceptances.json`'s bytes.
- `Found`: every row of founding's table, 1 to 21.
- `View`: a member's view, in canonical bytes.
- `Crosses`, `Asked` and `Blocked`: the outcomes of a view's crossing.
- `Accept`: a member's signed acceptance of its view.
- `Accepted`: rows 19 to 21 for one acceptance, as the window takes it, returning its `from`.
- `Acceptances`: `acceptances.json`, the list in `from` order.

A refusal is a `*setup.Error`.

**`setup`**

- `Seal`: a seal entry and its opening, from a signed declaration and a salt.
- `SaltBytes`: the length of a seal's salt.
- `Openings`: the openings a founder holds: none, every member's, or some members'.
- `Openings.ByActor` and `Openings.Some`: each opening's bytes under its actor's reference, and whether they are some members' only.
- `Accord.Opened`, `Accord.TermsChecked` and `Accord.Salt`: whether a member's seal was opened, whether every member's was, and a member's opening salt.
- `Accord.Principal`: the principal that signed a member's binding.
- `BindingRole`, `BindingPolicy`, `OpeningMissing`, `SealMismatch`, `AcceptanceMissing`, `AcceptanceSignature`, `AcceptanceAccord` and `AcceptanceView`: the founding codes of a principal's grant, its policy, an opening and an acceptance.
- `CodeViewer`: a view asked of an actor that is not a member, or of an Accord whose seals were not all opened; not a founding code.

**`signing`**

- `TypeAcceptance`: the acceptance's type URI, accepted by `Sign` and `Verify` and refused as a payload type.

**`composition`**

- `Sides` and `Side`: the crossings a member is a side of, by its own terms only, each with its decision.
- `Side.Counterpart`, `Side.Dir`, `Side.Kind` and `Side.Decision`: the counterpart, the direction from the member, the kind, and `Decide`'s decision of the crossing.
- `Answers` and `Asked`: the crossings between two other members that a member answers.
- `Asked.Asker`, `Asked.Receiver`, `Asked.Kind` and `Asked.Timeout`: the asker, the receiver, the kind and the agreed rule's timeout.

`Sides` and `Answers` are the view's rows, in `view.go`, outside the file the kernel hash reads (record 0007), built on `Decide` and the one term matcher.

## Consequences

- A single-owner Accord writes one policy entry that lists every role. An Accord between owners writes one entry per side.
- A view shows the agreed rule of an asked row, which may be the other side's alone; a `crosses` row shows the other side has a matching term; an answerer's `answers` rows show who asks whom about what, before any question is opened. Probing still shows terms: each probe is a signed receipt the other side reads, and an escalation shows a `maybe` covered that kind. The threat model says so.
- A view can be recomputed only with the openings of every member it reads, so a dispute over a view needs the counterparts' openings: each counterpart holds its own, and the operator holds them all.
