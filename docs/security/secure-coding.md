# Secure coding

How placket's code handles keys, cryptography, untrusted bytes, files and the socket. Read this before a change that touches any of them. Each rule here lives under its public name in `docs/rules/`, linked below; this file explains them together and adds what a rule cannot say in one line. `docs/security/threat-model.md` says what placket defends against, and `SECURITY.md` how to report a problem.

## Cryptography comes from the standard library

- Signing is `crypto/ed25519`, hashing is `crypto/sha256`, and randomness is `crypto/rand`. No primitive is written by hand, and no third-party cryptography is added ([`crypto-from-stdlib`](../rules/core.md#crypto-from-stdlib)).
- The core draws no randomness and reads no clock: the door mints salts with `crypto/rand` and passes them in, with the time and the keys ([`core-no-world`](../rules/core.md#core-no-world), [`inputs-as-parameters`](../rules/core.md#inputs-as-parameters), [`door-owns-clock`](../rules/adapters.md#door-owns-clock)).
- `math/rand` is never used for anything a party could guess to its advantage: a salt, a nonce, a key, an identifier.
- The door signs and verifies a call with the core's own payload-signing functions, and defines no second construction ([`call-signing`](../rules/adapters.md#call-signing)).

## Comparing secrets

- A secret, or a value whose bytes an attacker could learn from the time a comparison takes, is compared with `crypto/subtle.ConstantTimeCompare`, never with `==` or `bytes.Equal` ([`crypto-from-stdlib`](../rules/core.md#crypto-from-stdlib)).
- A signature is checked only through the core's signing functions, `signing.Verify` and `signing.VerifyPayload` in `core/signing/`. They refuse a key that is not the canonical encoding of its point, or whose point is of small order, before they call `ed25519.Verify`, which accepts both (signing-1). No other code calls `ed25519.Verify` itself, and the boolean those functions return is the only thing compared ([`call-signing`](../rules/adapters.md#call-signing)).

## Keys in memory

Go cannot reliably wipe a secret from memory: the garbage collector may copy a value, and the compiler may drop a write it judges dead. So placket makes no promise of memory hygiene. What it does:

- A private key is held as a `[]byte` (an `ed25519.PrivateKey`), never as a `string`, which could not be overwritten at all.
- A key lives as short a time as the call that needs it, and is not kept in a structure that outlives the call when it can be read again from its file.
- A struct that holds a secret prints the same fixed text under every format verb ([`secrets-never-print`](../rules/core.md#secrets-never-print)).

## Keys and state on disk

- A key file is written atomically and never overwrites one ([`keys-on-disk`](../rules/adapters.md#keys-on-disk)): the seed goes to a temporary file of mode 0600 in the same directory, which is synced and then linked to the key's name, so a crash leaves either no key file or the whole key, and an existing name is refused (`key-exists`). The temporary name is removed and the directory synced.
- A key file is refused when it is loaded if any of mode 0077 is set or another user owns it (`key-mode`). A symbolic link is followed and the file it reaches is checked; anything but a regular file is refused before it is opened (`key-read`), and at most 4 KiB is read.
- The journal and state directories placket makes are created mode 0700, and every file the window writes in them mode 0600; a directory that already exists keeps its mode (`docs/security/threat-model.md`, L14). The window sets its socket to mode 0600, and the client checks the socket's owner before it sends.
- A write the window promises is synced to disk before it replies ([`journal-synced`](../rules/adapters.md#journal-synced)), and the receipts file is never rewritten ([`journal-is-evidence`](../rules/adapters.md#journal-is-evidence)).
- A path built from input is joined under a directory placket owns and never follows a name that leaves it.

## Untrusted bytes

- Every byte from outside the process is untrusted: a call on the socket, a file a party hands over, a journal being verified.
- Parsing is strict: a duplicate key, trailing data or any other bytes the canonical form refuses are refused, never repaired ([`parse-strict`](../rules/core.md#parse-strict)). The core judges the bytes as they were sent ([`door-passes-bytes`](../rules/adapters.md#door-passes-bytes)).
- The door checks a call in a fixed order, size first, and stops at the first failure, before the mediator sees anything ([`door-check-order`](../rules/adapters.md#door-check-order)).
- Data that passed verification has a type only verification produces; no API hands unverified data on as trusted ([`verified-by-type`](../rules/core.md#verified-by-type)).

## Size limits

- Every decoder the door runs has a size limit, set before it reads a byte ([`parse-strict`](../rules/core.md#parse-strict)). An `io.Reader` from outside is wrapped in `io.LimitReader`, or read with a bounded buffer, never with `io.ReadAll` alone.
- The requirement text bounds some sizes and not others; where it is silent, the door's cap is the bound (`docs/security/threat-model.md`, known limits).

## Fuzzing

- Every function that parses untrusted input has a fuzz test, seeded from the vectors, or for a door format from its own test cases ([`fuzz-parsers`](../rules/tests.md#fuzz-parsers)). In the core, every exported function that takes bytes or a string has one ([`core-no-panic`](../rules/core.md#core-no-panic)).
- `make ci` runs each fuzz test on its seed corpus (`fuzz-seeds`). A crash found by a longer run is fixed with its input added to the corpus.

## Goroutines and the socket

- One goroutine owns the mediator, and every call reaches it over a channel ([`one-mediator-goroutine`](../rules/adapters.md#one-mediator-goroutine)).
- Every goroutine has an owner, a way to be stopped and a way it ends ([`goroutines-owned`](../rules/adapters.md#goroutines-owned)). A connection has a deadline, so a client that stops reading cannot hold the window.

## Nothing secret in errors or logs

- Error text is the package and the code, plus an index, an actor or an offset where the requirement gives one; never a body, salt, seed, key, message or payload ([`error-text-no-content`](../rules/errors.md#error-text-no-content)).
- A refusal returns a code, not a decoder's error, which may quote its input ([`errors-as-codes`](../rules/errors.md#errors-as-codes)).
- Nothing a tool prints quotes a key. Placket hands a salt only to the party it was released to, or to an answerer of the open question that holds the part; a party may disclose its own salts. A body is printed only to the party it was released to.
