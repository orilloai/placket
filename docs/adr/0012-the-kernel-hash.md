# 0012. The kernel hash, the default card, and a card of another module

Status: accepted. Date: 2026-11-14.

## Context

The card names the decision module that judges every receipt by a kernel `version` and `hash` (setup-contract-4). The operator writes them, and founding reads them for shape only. Until now nothing in placket said what the hash of placket's own module is: the demo and the tests typed one in by hand, the typed hash of a fixed phrase. A builder copied it from the demo, and a binary whose module had changed founded and ran an Accord whose card named the old module, so every receipt it signed named a module that did not judge it. Nothing said so.

## Decision

- **The kernel is `core/composition`'s source.** The decision module is the composition package (`one-decision-module`); its logic is `composition.go`. The kernel hash is the typed hash (canonical-3) of the JSON object `{"composition.go": <the file's bytes as one string>}`, taken from the bytes the binary was built from, through `go:embed`, so it cannot drift from the code and needs no step by hand. Any byte changed in that file, a comment included, gives another hash, and anyone holding the source can compute it again. Its other files (this record's `kernel.go`, the tests) are not in it: they decide nothing. A file the module adds later is added to the object, with a note here.
- **The kernel version** is a constant beside it, `0.1.0`. It is raised when the verdicts the module returns change; the hash changes with every edit whether or not it is raised.
- **The names it adds**, in `core/composition`: `const KernelVersion = "0.1.0"`; `func KernelHash() string`; `func Card() []byte`, this module's card in canonical bytes: `spec` 0.1, the kernel, no judges and no checks, a new slice at each call. In `core/setup`: `const Spec = "0.1"`, the card's `spec`, which founding already required and now reads from this one constant.
- **`placket card --default`** prints `composition.Card()` and a line feed on standard output, and on standard error `card <card hash>` and `module <version> <hash>`. `card` without `--default` is a usage error: no other card is built by the command line in this version.
- **A card of another module** is one whose kernel `version` or `hash` is not this build's. The reserved `judges` are not compared.
  - `founding` founds it, since the one who founds an Accord need not run its window, and prints after its card line `module <version> <hash> is not this build's decision module, <version> <hash>`.
  - `window` refuses it: it prints that line and tool error `founding kernel`, and starts nothing, so a window never signs a receipt whose card names a module it does not run. It reads the card after the key and the founding file and before the set is founded; a set whose card it cannot read is left to founding, which refuses it by its own code. A set that is malformed elsewhere and names another module is refused as `founding kernel`.
  - `verify --founding` prints that line after its module line: the verifier's build need not be the mediator's, so this is information, never a failure.
- **The core is unchanged in what it judges.** Founding still reads the kernel for shape only, the vectors keep the hash they were written with, and the requirement text is not changed: which module an operator runs is the door's to check, not the protocol's.

## Consequences

- After any change to `composition.go`, a window refuses its old founding set; the Accord is founded again with `placket card --default`, and gets a new id. A journal of the old Accord still verifies, and `verify --founding` names its module as another build's.
- The committed demo journal under `scripts/demo/` names the hand-typed hash; `verify --founding` on it prints the other-module line.
- A checkout that rewrites line endings (`core.autocrlf`) builds another hash from the same code.
- A full `placket init` is not built: declarations are signed by each party's key and bindings by the principal's, so one command cannot write every founding file. `policy.json` and `mediator.json` could each be printed by a short command; that is left to a follow-up.
