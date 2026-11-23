# 0007. The card and the kernel hash

Status: accepted. Date: 2026-11-23.

## Context

The card names the decision module that judges every receipt by a kernel `version` and `hash` (setup-contract-4). The operator writes them, and founding reads them for shape only. Unless placket says what the hash of its own module is, a builder copies one from somewhere, and a binary whose module has changed founds and runs an Accord whose card names the old module: every receipt it signs names a module that did not judge it, and nothing says so.

## Decision

- **The kernel is `core/composition`'s source.** The decision module is the composition package (`one-decision-module`); its logic is `composition.go`. The kernel hash is the typed hash (canonical-3) of the JSON object `{"composition.go": <the file's bytes as one string>}`, taken from the bytes the binary was built from, through `go:embed`, so it cannot drift from the code and needs no step by hand. Any byte changed in that file, a comment included, gives another hash, and anyone holding the source can compute it again. The package's other files (`kernel.go`, `view.go`, the tests) are not in it: they decide nothing. A file the module adds later is added to the object, with a note here.
- **The kernel version** is a constant beside it, `0.2.0`. It is raised when the verdicts the module returns change; the hash changes with every edit whether or not it is raised.
- **The card.** `composition.Card()` is this module's card: `spec` 0.1, the kernel, and no judges. A card has no `checks`. Its `judges` are the ordered list of the judges a term may grant, each pinned by its name, version and hash (record 0009).
- **`placket card --default`** prints the card and a line feed on standard output, and on standard error `card <card hash>` and `module <version> <hash>`. With `--judge FILE...` it lists those judges' configs in the card's `judges`, in flag order. `card` without `--default` is a usage error: no other card is built by the command line.
- **A card of another module** is one whose kernel `version` or `hash` is not this build's. The card's judges are not compared for this.
  - `founding` founds it, since the one who founds an Accord need not run its window, and prints after its card line `module <version> <hash> is not this build's decision module, <version> <hash>`.
  - `window` refuses it: it prints that line and tool error `founding kernel`, and starts nothing, so a window never signs a receipt whose card names a module it does not run. It reads the card after the key and the founding file and before the set is founded; a set whose card it cannot read is left to founding, which refuses it by its own code. A set that is malformed elsewhere and names another module is refused as `founding kernel`.
  - `verify --founding` prints that line after its module line: the verifier's build need not be the mediator's, so this is information, never a failure. `verify` then skips the decision row of conformance, which runs only under the kernel hash it trusts (record 0013).
- **Which module an operator runs is the door's to check, not the protocol's.** Founding reads the kernel for shape only, the vectors keep the hash they were written with, and the requirement text says nothing of this build's hash.
- **The demo journal.** The committed demo journal under `scripts/demo/` is founded under `placket card --default`, so its card names the module this build runs and `verify --founding` on it prints no other-module line; `tests/test_demo.sh` asserts that. A change that alters the kernel hash or a frozen format writes the demo journal again, and quotes the README's first receipt and its `show` output again, in the same pull request; otherwise `make ci` fails.

### Its names

**`composition`**

- `KernelVersion`: this module's kernel version.
- `KernelHash`: this module's kernel hash, from the source the build compiled.
- `Card`: this module's card in canonical bytes, a new slice at each call.

**`setup`**

- `Spec`: the card's `spec`, the version of the protocol, which founding reads from this one constant.

## Consequences

- After any change to `composition.go`, a window refuses its old founding set; the Accord is founded again with `placket card --default`, and gets a new id. A journal of the old Accord still verifies, and `verify --founding` names its module as another build's.
- A checkout that rewrites line endings (`core.autocrlf`) builds another hash from the same code.
