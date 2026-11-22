# The Accord

An Accord is the agreement between the parties of one handoff. It says who the members are, which roles each holds, what each will let cross, which mediator runs it, and which decision module decides. A mediator decides every part under exactly one Accord, and every receipt names it.

## What founds it

An Accord is founded from a founding set and the members' acceptances ([section 3 of the protocol](../../spec/protocol.md#3-founding-an-accord)). The founding set holds:

- the **policy**: the roles that must be held, and each principal with the roles it may grant ([the principal](principal.md));
- the **mediator**: the key that signs every receipt, the operator who runs it, and how it runs, which in this version is one process;
- the **card**: the decision module, named by its kernel hash, and the judges the Accord may call on ([judges](../guides/judges.md));
- each party's **seal**: the salted digest of its signed declaration of [terms](terms.md);
- each party's **binding**: its principal's signed statement of the roles it holds.

The Accord's id is the typed hash of the founding set (setup-contract-5). Any change to the set, a term, a role, the operator or the card, founds another Accord with another id. The id is the one value a verifier is given out of band ([verify](../verify.md)).

## Sealed terms and a view each party signs

A party's terms stay sealed from the other parties. The founding set holds only each declaration's seal; the opening, the declaration with its salt, goes to whoever runs the window and to no other party (setup-contract-12, setup-contract-13).

Before the Accord is founded, the window shows each member its own view: what will cross to and from it, and which questions it will answer, built from its own terms (setup-contract-14). The member signs an acceptance of that view. Until every member has accepted, the Accord is proposed: the window serves each member its view and takes its acceptance, and its door refuses every other call with `not-founded`. Once the last member accepts, the window writes `acceptances.json` beside the founding set and the Accord is founded (setup-contract-15). There is no decline: a member that does not consent does not accept.

```sh
placket view --label approve-person
placket accept --label approve-person
```

## Founding one

- **For one person who holds every key**, `placket init` founds an example Accord in one command, and signs each member's acceptance with its own key. The [quickstarts](../quickstarts/approve.md) start there.
- **Between parties who each hold their own keys**, each party declares and seals its own terms, its principal binds it, and someone puts the parts together with `placket founding`. [The README's "Found an Accord"](../../README.md#found-an-accord) gives every command.

## Its end

An Accord is closed by `placket window close` on a stopped window's journal: every open question is closed, and the close receipt comes last ([section 13](../../spec/protocol.md#13-closing-an-accord)). A closed Accord never reopens. A successor Accord names the one it replaces, and the close, in its policy's `supersedes` (setup-contract-11), as [the rotation guide](../guides/rotation.md) shows.
