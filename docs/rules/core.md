# The core

The rules for code under `core/`: what it may reach, how its packages are shaped, what it exports, and the decision about what crosses. The core is pure: its outputs are a function of its inputs, and the world is the adapters' job. Its error rules are in `errors.md`; its tests' rules in `tests.md`. `docs/security/secure-coding.md` explains the security rules in more depth.

## `core-no-world`

No non-test file in `core/` reads the clock (`time.Now`, and `time.Since`, `Until`, `After`, `AfterFunc`, `Tick`, `NewTimer`, `NewTicker` and `Sleep`, which wait on it or measure against it), draws randomness (`crypto/rand`, `math/rand`), reads the environment (`os.Getenv`), opens a file (`os.Open`), touches the network (`net` and below), reaches the process, its files or its memory (`os`, `syscall`, `io/ioutil`, `runtime`, `unsafe`, and the packages below them), logs (`log` and below) or prints (`fmt.Print*`, `print`, `println`). The core is Go alone: no cgo (`import "C"`), and no assembly, C, C++ or other non-Go source file anywhere in `core/` outside `testdata/`, since those reach the world where no check of Go can see. The core takes the time, salts and keys as parameters. The test helpers under `core/internal/testing/` count as test files, since only tests import them: they may read the vectors from disk, and a file built into the core that imports them is refused.

Why: output that depends on when or where it ran cannot be pinned by a vector.

Checked by: `make ci`, step `core-no-world`.

## `core-no-panic`

No function in `core/` panics on input. The machine half: no non-test file in `core/` calls `panic`. The other half: every exported function that takes bytes or a string has a fuzz test with a committed seed corpus, and a new entry point adds or extends one. Seeds given with `f.Add` in the test are a committed seed corpus; a `testdata/fuzz/` directory is not needed.

Why: a mediator that crashes on a hostile message denies the service to every party at once.

Checked by: `make ci` checks the call, step `core-no-panic`; the reviewer, the fuzz tests.

## `mediator-no-logic`

The mediator package holds state and operations and calls the capability packages; it holds no capability's logic. A mediator function that reads no state is misplaced, unless it reads a founding set.

Why: logic in the mediator cannot be tested by a vector without a whole mediator.

Checked by: the reviewer.

## `trace-new-names`

Each name a change exports from `core/` has a reason on record, a requirement id or a decision, and its doc comment cites it.

Why: an exported name is a promise; it needs a reason someone can check.

Checked by: the reviewer.

## `specs-name-behaviour`

The requirement text names behaviour, never Go identifiers; the code names requirement ids in comments.

Why: the text outlives any one implementation.

Checked by: `make ci`, step `specs-name-behaviour`, reads the requirement text and refuses a line that names Go code; the reviewer checks that the code names requirement ids in its comments.

## `bytes-at-boundaries`

At every boundary a verifier or a vector passes through, values travel as bytes, not Go structs. A struct is fine only where neither a verifier nor a vector ever looks at the value's bytes.

Why: the canonical bytes are the one value both sides agree on.

Checked by: the reviewer.

## `inputs-as-parameters`

When a function's output gets hashed or signed, its time, salt and key arrive as arguments, placed after the data, and a test proves that altering any of them alters the result.

Why: this is how the core stays free of the world (`core-no-world`) and stays testable.

Checked by: the reviewer.

## `spec-words`

Names in the code are the requirement text's words, and a verb from the text is not renamed in Go; codes keep the text's spelling.

Why: one vocabulary across the text, the vectors and the code.

Checked by: the reviewer.

## `results-are-copies`

A result never shares memory with internal state; a test mutates what was returned and checks the original is unchanged.

Why: a caller must not be able to rewrite the mediator's state by accident.

Checked by: the reviewer.

## `secrets-never-print`

Unexported state is read through functions only, and any exported struct holding a secret prints the same fixed text whatever format verb is used. A test prints it every way and looks for the key, the body and the salt.

Why: a secret in a log is a leak nobody notices.

Checked by: the reviewer.

## `constants-once`

An exported constant that is part of a format (a type URI, a minimum length, a payload type) is defined in one place and imported everywhere else, never retyped.

Why: a retyped constant is a second format waiting to drift.

Checked by: `make ci`, step `constants-once`, for the string literals that hold `placket.tech/` or `vnd.placket`; the reviewer, for every other format constant, such as a minimum length or a word another package already exports.

## `parse-strict`

Untrusted bytes are parsed strictly. Duplicate keys, trailing data and any other bytes the canonical form refuses are refused, never repaired. The door sets a size limit for every decoder.

Why: two readers that accept different bytes as one value let a sender show each of them something different.

Checked by: the reviewer.

## `verified-by-type`

Data that has passed verification has a type only verification can produce. No API hands unverified data to a caller as trusted.

Why: a caller cannot then take bytes nobody checked for bytes that were.

Checked by: the reviewer.

## `crypto-from-stdlib`

Signing, hashing and randomness come from the Go standard library (`crypto/ed25519`, `crypto/sha256`, `crypto/rand`). No primitive is written by hand. A secret is compared only with `crypto/subtle`.

Why: hand-written cryptography fails in ways no test shows, and a plain comparison of a secret leaks it through its timing.

Checked by: the reviewer.

## `one-decision-module`

The decision about what crosses is reached through one entry point in the composition package. No other code compares a stance, matches a term or compares two terms' judges, and a recorded verdict is never computed again, not on reload and not in the export. The one exception is the conformance check, `core/conform`, which decides again only when the card's kernel hash is the one the verifier trusts: then the module that decides again is the module trusted for that hash, so the reason a reload decides nothing again, that the module may have changed, does not hold (`docs/adr/0009-who-decided.md`).

Why: the decision module is the seam that can be swapped; logic outside it would survive the swap.

Checked by: the reviewer.

## `release-in-resolve`

Content is released only by the operation that resolves a question, during the very call that carries the answer permitting it, with every receipt of that call built before any state changes. One function recognises a release from receipts, and everything else calls it.

Why: a release outside the answer is content crossing that nobody agreed to.

Checked by: the reviewer.

## `reserved-evaluate`

Reserved for evaluating a proposal before it is sent. When it is built, it shares one matching function with the decision and restates none of its rules.

Why: a second copy of the matching rules would let the preview and the real decision disagree.

Checked by: the reviewer.
