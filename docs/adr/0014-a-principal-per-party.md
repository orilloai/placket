# 0014. A principal per party, scoped to the roles the policy gives it

Status: accepted. Date: 2026-11-17. Amends record 0005. Amended by record 0020.

## Context

An actor holds a role only by a binding: a principal's signed statement that the actor holds some roles under one mediator until a time (setup-contract-2). Until now a founding set's policy listed its principals as a flat list of actor references, and any listed principal could bind any actor to any role.

That fits one person running several actors, as the demo does: one principal signs every binding. It does not fit an Accord between sides with different owners. With a flat list, one side's principal could bind a key of its own as the other side's `decider`, and founding would accept it. An Accord between two owners needs each owner to vouch for its own side only, and needs to say which side may supply which role. One principal standing behind several parties, or all of them, must stay as short to set up as it is now.

## Decision

- **A policy entry per principal.** `policy.principals` is a non-empty list of entries, each with exactly `principal`, an actor reference, and `roles`, the labels that principal may grant: at least one, none repeated. No two entries name one principal. Two entries may list the same role, so two sides may each supply, say, a reviewer. `policy.roles` keeps its meaning, the roles that must be held; a role may be grantable without being required. A required role no entry lists can never be held, and its set ends in `role-missing`, with no earlier refusal of shape.
- **A principal is scoped to roles, not to actors.** A binding may give only roles its principal's entry lists; otherwise founding refuses it with `binding-role`. The policy does not list the actors a principal may bind. The actors of an Accord are its declarations; the policy is written before anyone knows every key a side will use, and a side rotates its own keys without the other side's say. What the other side needs to agree to is which roles a principal may fill, and that is what the entry says.
- **A binding commits to its policy.** A binding names `policy`, the typed hash (canonical-3) of the canonical bytes of the founding set's whole `policy` object, under the same type URI as before. The principal's signature covers it, so a binding counts only under the policy its principal signed for; one signed for another policy is refused with `binding-policy`. Without it, a binding signed for one Accord's policy could be reused in a set whose policy gives its principal more, or gives another principal the same roles. Hashing the whole policy, `policy.roles` included, means each principal agrees to every other principal's grant: adding a party or changing any grant means every principal signs its bindings again. That is the price of a binding that cannot be carried into a policy its principal never saw.
- **Where the checks run.** The two new rows run after the binding's signature, so a binding whose signature fails never reaches them, and before its mediator: principal listed (`binding-principal`, now read against the entries' `principal` members), signature, `binding-role`, `binding-policy`, mediator, time. The role comes before the policy because it is the narrower statement about what this principal may do.
- **Who stands behind each member is visible.** `setup` keeps, for each member, the principal that signed its binding. `verify --founding` prints one line per member, in actor-reference order, after the module's lines: `member <actor> roles <r1,r2> principal <principal>`, and its `--json` object holds the same as `members`. Receipts do not change: a receipt names actors, and the founding set says who stands behind them.
- **The command line.** `placket bind` requires `--policy FILE`. It reads the policy, puts it in canonical form and writes its typed hash into the binding; a file it cannot read, or that the canonical form refuses, is its `file` error with the detail `policy`. It does not check that the policy lets the principal grant the roles it writes: founding does, and one place judges it.
- **The names it adds**, in `core/setup`: the codes `BindingRole` (`binding-role`) and `BindingPolicy` (`binding-policy`), and `func (*Accord) Principal(actor string) string`, the reference of the principal that signed actor's binding, or empty for an actor that is not a member.

## What it does not do

- No signature by a principal over the founding set or a declaration. A principal is not a member unless its key also declares.
- No second binding per actor: one actor has one binding, and so one principal (`binding-duplicate` stays).
- No delegation, vouching, admitting joiners, revocation, `principal:<id>` selectors in a term, or reveal levels for principals. Nothing is reserved for them.
- No change to receipts, envelopes, deliveries, crossing, composition, questions, release, reconciliation or the log.

## Consequences

- Every founding set, binding and Accord id changes, and with them every scenario vector's receipts and ids; every vector group was written again. The register and the vectors' README carry a note.
- A single-owner Accord writes one policy entry that lists every role, as the demo and the command line's tests do. The guarded-export pattern uses two principals, one behind the proposer and the approver, one behind the receiver.
- The committed demo journal was founded again under the new formats. Its card still names the kernel hash typed by hand (0012), so `verify --founding` on it still names that module as another build's.
