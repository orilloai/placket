# The placket protocol, version 0.1

This document is the normative text of the placket protocol. It says what a mediator does at a handoff between parties that do not fully trust each other: how an Accord is founded, how a part is given, judged, held, asked about, answered, released and fetched, what every receipt states, and how a third party who holds only the mediator's public key checks the receipts. `README.md` beside it says how to read `spec/`; the schemas under `schema/` and the register, `frozen-formats.md`, hold the formats this text defines.

## How to read this document

The document has a section per operation, in the order a reader meets them. Each section opens with the operation's check table: one row per check, in the order the operation runs them, with what the check reads and the code it returns. The first check that fails decides the result, so the order of the rows is their precedence. Each code is defined once, in the row that returns it. A section's requirements follow its table, each under a heading `### <id> <title>`, with at least one scenario under a heading `#### Scenario: <title>`.

A requirement id is permanent. It is never reused and never renumbered, and an id that names no requirement in this version is listed in `reserved.txt`. Where a requirement cites another, it cites it by id.

These rules hold in every section.

- **Bytes are read in their canonical form.** Everything hashed, signed or judged is read from the canonical bytes of canonical-1. Where an operation receives bytes, the first check that reads them as JSON returns `not-canonical` when the canonical form refuses them, with one exception: a give's envelope is read by the door's signature row, so envelope bytes the canonical form refuses, a repeated `seq` included, are `envelope-signature` (crossing-6). `not-canonical` means nothing else anywhere: bytes the canonical form refuses.
- **The check that reads a member owns its defects.** A member that is missing, of the wrong JSON type, out of range, of the wrong form or not listed fails with the code of the check that reads it. An operation may open with one whole-shape check of its own; after it, a member belongs to the later check that reads it. No other rule decides a code for input off its format.
- **Every format is closed.** An object holds exactly the members its format lists. A member has a meaning or is absent: no member is `null`, and a member that would carry nothing is left out. A member kept for a later version is reserved with a fixed shape, read for shape only, and changes no result in this version.
- **One timestamp rule.** Every time in every format is written `YYYY-MM-DDTHH:MM:SSZ`: a date the Gregorian calendar has, a time of day from `00:00:00` to `23:59:59` in whole seconds, UTC, and no leap second (`timestamp.json`).
- **Lowercase hex.** Every digest, salt, seed, key and hex signature is written in lowercase hex and accepted only so. Base64, where a format uses it, is standard base64 with padding (RFC 4648, section 4), in the one encoding that decodes and encodes back to the same text.
- **Codes are closed lists.** Each operation returns only the codes its table lists, spelled as the table spells them. A code is a result a caller or a verifier sees; how a program reports its own misuse is not part of the protocol.
- **Nothing past the crossing.** The protocol judges what crosses a handoff. No requirement, receipt or code states anything about what a receiver does with a part after it was queued for it.

The external standards this text uses are named once, by version: JSON (RFC 8259), UTF-8 (RFC 3629), UTF-16 code units as Unicode 16.0 defines them, SHA-256 (FIPS 180-4), Ed25519 (RFC 8032), base64 (RFC 4648), timestamps (a profile of RFC 3339), DSSE 1.0 (Dead Simple Signing Envelope), the in-toto Statement v1, the A2A protocol 1.0 message format, and JSON Schema draft 2020-12 for the schemas.

## 1. Canonical form and hashes

The canonical form turns JSON bytes into the one byte string every party hashes and signs, or refuses them with a code and a byte offset. It reads the input once; the checks below are what can make it refuse. Row 1 wins wherever it sits in the input. Rows 2 to 5 are taken token by token in input order: the first token with any defect decides, and within one token the lower row comes first. A token is a string, a member name, a number, a literal, or a structural character.

| # | Check | Reads | Code, at offset |
|---|---|---|---|
| 1 | Unicode | Every byte, for well-formed UTF-8 (canonical-6); every quoted span found by canonical-11, for an escape of an unpaired surrogate | `invalid-string`, at the first byte of the ill-formed sequence, or at the backslash that starts the surrogate escape, whichever is first |
| 2 | Grammar | The JSON grammar of RFC 8259 with no byte order mark, the escapes it allows, and nesting to depth 1000 (canonical-7, canonical-8) | `invalid-json`, at the first byte no JSON text can continue with, or at the input's length when it ends too soon |
| 3 | Noncharacters | Each string and member name, after its escapes are read (canonical-9) | `invalid-string`, at the string's opening quote |
| 4 | Repeated names | Each object's member names, after their escapes are read (canonical-4) | `duplicate-key`, at the opening quote of the second name |
| 5 | Numbers | Each number's exact value (canonical-2, canonical-5) | `fractional-number` or `unsafe-integer`, at the number's first byte |

### canonical-1 Canonical bytes

The canonical form of a JSON value is UTF-8 with no whitespace between tokens. An object's members are written sorted ascending by the UTF-16 code units of their names, compared after escapes are read. An array keeps its order. A number is written as a plain decimal integer: an optional `-`, then digits with no leading zero, and zero is `0`. The literals are `true`, `false` and `null`. A string is written between quotes with exactly these escapes: `\"` for a quote, `\\` for a backslash, `\b`, `\t`, `\n`, `\f` and `\r` for U+0008, U+0009, U+000A, U+000C and U+000D, and `\u00` followed by two lowercase hex digits for every other code point from U+0000 to U+001F. Every other code point, U+007F and every character outside ASCII included, is written as its UTF-8 bytes. The canonical form of canonical bytes is the same bytes.

#### Scenario: members sorted and whitespace dropped

- Given the input `{ "b": 1, "a": [ true ] }`
- When it is put in canonical form
- Then the output is `{"a":[true],"b":1}`

#### Scenario: names sorted by UTF-16 code units

- Given an object with a member named U+FB01 and a member named U+1F600
- When it is put in canonical form
- Then the member named U+1F600 comes first, because its first UTF-16 code unit, 0xD83D, is below 0xFB01, although its code point is above

#### Scenario: only quotes, backslashes and controls are escaped

- Given the string value of a quote, a backslash, U+0001, U+0009, `é` and U+007F, each written as an escape in the input
- When it is put in canonical form
- Then the output string is `"\"\\\u0001\té` followed by the byte 0x7F and a closing quote

### canonical-2 The value set

The canonical form holds objects, arrays, strings, the three literals, and integers from −(2^53 − 1) to 2^53 − 1. A number with a value outside that set is refused: `fractional-number` for a value below 10^16 in magnitude that is not an integer, and `unsafe-integer` for any other value outside the set, as canonical-5 decides.

#### Scenario: a fraction is refused

- Given the input `[0.5]`
- When it is put in canonical form
- Then it is refused with `fractional-number` at offset 1, and no bytes are output

#### Scenario: the ends of the range are accepted

- Given the input `[9007199254740991,-9007199254740991]`
- When it is put in canonical form
- Then the output is the input

#### Scenario: one past the end is refused

- Given the input `9007199254740992`
- When it is put in canonical form
- Then it is refused with `unsafe-integer` at offset 0

### canonical-3 Typed hashes

The typed hash of a value is `sha256:` followed by the 64 lowercase hex digits of the SHA-256 of its canonical bytes. An Accord id, a card hash, a message hash, a receipt id and a receipt's `prev` are typed hashes (`hash.json`). A typed hash is computed only from bytes the canonical form accepts.

#### Scenario: the hash of an empty object

- Given the input `{ }`
- When its typed hash is computed
- Then it is `sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a`, the SHA-256 of the two bytes `{}`

### canonical-4 Refusals

When the canonical form refuses its input, it outputs no bytes and returns exactly one code and one byte offset, counted from the first byte of the input. The codes are `invalid-json`, `invalid-string`, `duplicate-key`, `fractional-number` and `unsafe-integer`, decided by the table above. A refusal carries nothing of the input. Two member names of one object that are equal after their escapes are read are a repeated name, `duplicate-key`; names that differ only in case are different names.

#### Scenario: a repeated name

- Given the input `{"a":1,"a":2}`
- When it is put in canonical form
- Then it is refused with `duplicate-key` at offset 7

#### Scenario: a name repeated only after its escape is read

- Given the input `{"a":1,"\u0061":2}`
- When it is put in canonical form
- Then it is refused with `duplicate-key` at offset 7

#### Scenario: names that differ in case

- Given the input `{"Vector":1,"vector":2}`
- When it is put in canonical form
- Then the output is the input

### canonical-5 Numbers by exact value

A number is judged by the exact value its digits and exponent write, never by a binary approximation. `1.0` is 1, `1e2` is 100, and `-0` and `0e999999999` are 0. A value of 10^16 or more in magnitude is `unsafe-integer`, whether or not it is an integer. Below that, a value that is not an integer is `fractional-number`, and an integer above 2^53 − 1 in magnitude is `unsafe-integer`. Any other value is that integer, written as canonical-1 says. The value is decided without expanding the exponent: reading the exponent's digits with saturation at 2^40 decides the same as the exact value.

#### Scenario: spellings of one value

- Given the input `[1.0,1e2,-0,0e999999999]`
- When it is put in canonical form
- Then the output is `[1,100,0,0]`

#### Scenario: magnitude before integrality

- Given the inputs `12345678901234567.5` and `9007199254740993.5`
- When each is put in canonical form
- Then the first is refused with `unsafe-integer` and the second with `fractional-number`, each at offset 0

#### Scenario: huge exponents

- Given the inputs `1e999999999`, `1e9223372036854775808` and `1e-999999999`
- When each is put in canonical form
- Then they are refused with `unsafe-integer`, `unsafe-integer` and `fractional-number`, each at offset 0, with no allocation that grows with the exponent

### canonical-6 Strings

Input must be well-formed UTF-8 everywhere. A byte that is not part of a well-formed UTF-8 sequence is `invalid-string` at that byte, inside a string or outside one, and also when it follows a backslash. A `\u` escape of a high surrogate must be followed at once by a `\u` escape of a low surrogate, and the pair is one code point. An escape of a low surrogate with no high one before it, or of a high surrogate with anything else after it, is `invalid-string` at the backslash that starts the unpaired escape. A backslash followed by a byte that starts no escape RFC 8259 allows, a character outside ASCII included, is `invalid-json` at that byte. A raw control character, U+0000 to U+001F, inside a string is `invalid-json` at that byte.

#### Scenario: invalid UTF-8 inside a string

- Given the input of the hex bytes `22 ff 22`
- When it is put in canonical form
- Then it is refused with `invalid-string` at offset 1

#### Scenario: after a backslash

- Given the input of the hex bytes `22 5c ff 22`, and separately the input `"\é"`
- When each is put in canonical form
- Then the first is refused with `invalid-string` at offset 2 and the second with `invalid-json` at offset 2

#### Scenario: an unpaired surrogate escape

- Given the inputs `"\ud800"`, `"\udc00"` and `"\ud800A"`
- When each is put in canonical form
- Then each is refused with `invalid-string` at offset 1

### canonical-7 Grammar and offsets

The input is one JSON text as RFC 8259 defines it, with whitespace (space, tab, line feed, carriage return) allowed between tokens and nothing after the value but whitespace. A byte order mark is not allowed. The offset of `invalid-json` is the first byte no JSON text can continue with: an input that could still become valid is not refused until the byte that rules it out, and an input that ends too soon, the empty input included, is refused at its length.

#### Scenario: the empty input and an input that ends too soon

- Given the empty input, and separately the input `[1,`
- When each is put in canonical form
- Then they are refused with `invalid-json` at offsets 0 and 3

#### Scenario: a byte order mark

- Given the input of the hex bytes `ef bb bf 7b 7d`
- When it is put in canonical form
- Then it is refused with `invalid-json` at offset 0

#### Scenario: trailing data and a token cut short

- Given the inputs `1,`, `[1],` and `[1,2,tru]`
- When each is put in canonical form
- Then they are refused with `invalid-json` at offsets 1, 3 and 8: in the last, `tru` could still begin `true` until the `]`

### canonical-8 Nesting

Arrays and objects nest to a depth of at most 1000. The bracket or brace that would open level 1001 is `invalid-json` at its offset, refused before the level is entered, so reading the input never needs more than 1000 levels.

#### Scenario: depth 1000 and 1001

- Given an input of 1000 `[` followed by 1000 `]`, and an input of 1001 `[` followed by 1001 `]`
- When each is put in canonical form
- Then the first is output unchanged and the second is refused with `invalid-json` at offset 1000

### canonical-9 Noncharacters

A string or member name holding a Unicode noncharacter, written as UTF-8 or as an escape, is `invalid-string` at its opening quote. The noncharacters are U+FDD0 to U+FDEF, and the last two code points of every plane, U+nFFFE and U+nFFFF for n from 0 to 16, U+10FFFF included. A grammar defect in the same string comes first.

#### Scenario: noncharacters and their neighbours

- Given the inputs `["\ufdd0"]`, `{"\uffff":1}` and `"\udbff\udfff"`, and separately `["\ufdcf","\ufdf0"]`
- When each is put in canonical form
- Then the first three are refused with `invalid-string` at offsets 1, 1 and 0, and the last is accepted

#### Scenario: a grammar defect in the same string

- Given the input `"\ufdd0\q"`
- When it is put in canonical form
- Then it is refused with `invalid-json` at offset 8

### canonical-10 Precedence among defects

When an input has several defects, the table decides which one is returned. A Unicode defect of row 1 wins wherever it sits. Otherwise the first token in input order that has a defect decides, and within that token a grammar defect comes before a noncharacter, a noncharacter before a repeated name, and a repeated name before a number outside the value set.

#### Scenario: invalid UTF-8 after a grammar error

- Given the input `[1,,"` followed by the byte 0xFF and `"]`
- When it is put in canonical form
- Then it is refused with `invalid-string` at offset 5, not with the `invalid-json` at offset 3

#### Scenario: a number before a repeated name

- Given the input `{"a":0.5,"a":1}`
- When it is put in canonical form
- Then it is refused with `fractional-number` at offset 5

### canonical-11 Quoted spans

Row 1 finds the strings of the input by scanning its quotes from the first byte, whether or not the grammar holds: a quote opens a span, a backslash in the span makes the byte after it part of the span, and the next quote not so taken closes it. A surrogate escape inside such a span is judged by canonical-6. A `\u` outside every span is not in a string, and is a grammar defect.

#### Scenario: a surrogate escape after the grammar broke

- Given the input `[1 2, "\ud800"]`
- When it is put in canonical form
- Then it is refused with `invalid-string` at offset 7, although the grammar broke at offset 3

#### Scenario: a surrogate escape outside any quotes

- Given the input `[\ud800]`
- When it is put in canonical form
- Then it is refused with `invalid-json` at offset 1

## 2. Actor references and signing

An actor is named by its Ed25519 public key. A declaration, a binding and an envelope each carry a signature by the actor that stands behind it, and a receipt carries the mediator's. Signing has no codes of its own: an operation that checks a signature returns its own code when any row below fails, and the table is the same for each of them.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Reference | The reference of the actor that must have signed: `ed25519:` and 64 lowercase hex (signing-1) | The calling operation's signature code |
| 2 | Key | The 32 bytes: a point of the curve that is not of small order (signing-1) | The calling operation's signature code |
| 3 | Signature form | The object's `sig`: present, a string of 128 lowercase hex | The calling operation's signature code |
| 4 | Equation | The signing bytes of signing-2, under the type URI the caller names, and the signature | The calling operation's signature code |

The calling operation's signature codes are `declaration-signature` and `binding-signature` in founding (setup-contract-8) and `envelope-signature` at the door (crossing-6). A receipt's signature is checked by the verifier's `signature` row (receipt-6).

### signing-1 Actor references and keys that sign nothing

An actor reference is `ed25519:` followed by the 64 lowercase hex digits of a 32-byte Ed25519 public key (`reference.json`). Any other string names no actor: another length, uppercase hex, another prefix or no prefix. A key whose 32 bytes do not decode to a point of the curve, are not the canonical encoding of the point they decode to, or decode to a point of order 1, 2, 4 or 8, signs nothing: every signature under it fails verification. A reference's form is checked before any key is used.

#### Scenario: reference forms

- Given the strings `ed25519:` followed by 64 lowercase hex digits, the same with the hex in uppercase, with 62 hex digits, and with the prefix `ecdsa:`
- When each is read as an actor reference
- Then only the first names an actor

#### Scenario: a small-order key

- Given a declaration whose `actor` is the reference of a point of order 8, with any signature
- When its signature is verified
- Then it fails, and founding returns `declaration-signature` for it

### signing-2 Signing bytes and verification

The signing bytes of an object under a type URI are the DSSE 1.0 pre-authentication encoding of the type URI and the canonical bytes of the object with its `sig` member removed: the ASCII text `DSSEv1`, a space, the type URI's length in bytes as a decimal number, a space, the type URI, a space, the canonical bytes' length as a decimal number, a space, and the canonical bytes. The signature is Ed25519 (RFC 8032, without context) over those bytes, written as 128 lowercase hex digits in `sig`. A `sig` member present when an object is signed is ignored. The type URI is the one the verifying operation names for the kind of object it reads, never one taken from the object. Verification is cofactorless: the signature `R ‖ S` verifies when S, read little-endian, is below the group order L, and R's 32 bytes equal the encoding of [S]B − [k]A, with A the key's point and k the SHA-512 of R, the key's 32 bytes and the signing bytes, as RFC 8032 defines it. R is compared as bytes, so a non-canonical R fails. A key's 32 bytes must be the canonical encoding of its point; a key whose bytes are any other encoding signs nothing (signing-1). A signature that only a cofactored check accepts fails, and so does S + L in place of S.

#### Scenario: the type URI comes from the operation

- Given a binding signed under `https://placket.dev/declaration/0.1` instead of `https://placket.dev/binding/0.1`
- When founding verifies it
- Then it fails with `binding-signature`

#### Scenario: signed by another key

- Given an envelope whose `sig` is a valid signature of its signing bytes by a key other than the caller's
- When the door verifies it under the caller's key
- Then it fails with `envelope-signature`

#### Scenario: a malleated signature

- Given a valid signature whose S is replaced by S + L
- When it is verified
- Then it fails

### signing-4 Type URIs

Placket signs objects of four kinds under four type URIs: `https://placket.dev/declaration/0.1`, `https://placket.dev/binding/0.1`, `https://placket.dev/envelope/0.1` and `https://placket.dev/registration/0.1`. The last names a registration, which this version reserves: no object of this version is signed or verified under it. A signature over any other payload, such as a receipt's statement or a message of a door, is a DSSE signature over that payload under its own payload type, and its payload type is never one of the four, so no payload signature can be taken for a placket object's signature, or the reverse. A receipt is signed so, under the payload type `application/vnd.in-toto+json` (receipt-1).

#### Scenario: a placket URI as a payload type

- Given a payload signature whose payload type is `https://placket.dev/envelope/0.1`
- When it is made or checked as a payload signature
- Then it is refused, and no envelope is signed or verified by that route

## 3. Founding an Accord

Founding reads a founding set and the time it is founded at, and either founds an Accord or refuses the set with one code. Its inputs are the set's bytes and the founding time; the set itself names the mediator. It opens with the canonical form and a whole-shape check. After them, each declaration is checked in actor-reference order (setup-contract-10), all its rows before the next declaration's, then the rows that look across declarations. The first failing row decides.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Canonical form | The set's bytes (canonical-1) | `not-canonical` |
| 2 | Shape | The whole set but what rows 4 to 8 read: the top level; `policy`, `mediator` and `card` in full; `declarations` and `bindings` as non-empty arrays of objects; each declaration's members, none but `actor`, `terms` and `sig`, and its `actor` present and a string; each binding in full, `until` by the timestamp rule included; the order of both lists (setup-contract-9, setup-contract-10) | `invalid-shape` |
| 3 | One binding per actor | The bindings' `actor` members | `binding-duplicate` |
| 4 | Actor | The declaration's `actor`, as an actor reference (signing-1) | `actor-reference` |
| 5 | Declaration signature | The declaration's `sig`, and the declaration, under `https://placket.dev/declaration/0.1` by its `actor` (signing-2) | `declaration-signature` |
| 6 | Terms | The declaration's `terms` and each term's members, a `resolve` on a term that is not a `maybe` and the form of every `resolve` included, but not whether a `maybe` term has one (setup-contract-1) | `term-invalid` |
| 7 | Maybe rule | Each term of stance `maybe`, for the presence of its `resolve` member | `maybe-without-resolve` |
| 8 | Checks | Each check a term names, against the card's `checks` | `check-unknown` |
| 9 | Binding present | A binding whose `actor` is the declaration's | `binding-missing` |
| 10 | Principal | That binding's `principal`, against the policy's `principals` | `binding-principal` |
| 11 | Binding signature | That binding's `sig`, under `https://placket.dev/binding/0.1` by its `principal` (signing-2) | `binding-signature` |
| 12 | Binding mediator | That binding's `mediator`, against the set's `mediator.actor` | `binding-mediator` |
| 13 | Binding time | That binding's `until`, against the founding time | `binding-expired` |
| 14 | Declaration present | Each binding's `actor`, for a declaration by that actor | `declaration-missing` |
| 15 | Roles held | Each role of the policy's `roles`, for a declaring actor whose binding holds it | `role-missing` |

When every row passes, the Accord is founded: its id is the typed hash of the set (setup-contract-5).

### setup-contract-1 Declarations

A declaration is an actor's own terms, signed by that actor (`declaration.json`, `term.json`). Its `actor` is the actor's reference, its `terms` a non-empty list of terms, and its `sig` the actor's signature under `https://placket.dev/declaration/0.1`. A term has a `stance` (`need`, `want`, `maybe` or `no`), a `dir` (`give` or `get`), `with`, a non-empty list of distinct role selectors `role:<label>` naming roles the other side must hold, and `select`, an object whose one member `kind` is a non-empty list of distinct kinds. A label and a kind are lowercase letters and digits in words joined by single hyphens, at most 64 characters. A term of stance `maybe` has a `resolve` member: `to`, a non-empty list of distinct role selectors naming who answers, and `timeout`, a whole number of seconds from 1 to 2^53 − 1. A term of any other stance has none. A term may name checks in `checks`, a non-empty list of objects each with a check's `name` and a `corpus` label; this version loads no check, so every named check is `check-unknown`. A term with a member missing, of the wrong type, of the wrong form or not listed is `term-invalid`, and so is a `terms` member that is missing, empty or not a list. Every party of an Accord signs its own terms: founding refuses a declaration signed by any key but its actor's.

#### Scenario: a selector that is not a role

- Given a set whose one declaration has a term with `with` `["decider"]`, correctly signed
- When it is founded
- Then it is refused with `term-invalid`

#### Scenario: a maybe without its rule

- Given a set whose declaration has a term of stance `maybe` with no `resolve`, and nothing else wrong
- When it is founded
- Then it is refused with `maybe-without-resolve`

#### Scenario: a term that names a check

- Given a set whose declaration has a term with `checks` `[{"name":"forbidden","corpus":"tests"}]`
- When it is founded
- Then it is refused with `check-unknown`, since the card loads no check

#### Scenario: a declaration edited after signing

- Given a set whose declaration's terms were changed after it was signed
- When it is founded
- Then it is refused with `declaration-signature`

### setup-contract-2 Bindings and roles

An actor holds a role only by a binding: a principal's signed statement that the actor holds a list of roles under one mediator until a time (`binding.json`). A binding has the `actor`, the `principal` that signs it, `roles`, a non-empty list of distinct labels, the `mediator`'s reference, `until`, a timestamp, and `sig`, the principal's signature under `https://placket.dev/binding/0.1`. Roles come from nowhere else: not from a declaration, a policy or a message. A binding's `until` must be later than the founding time; a binding is checked against the clock at founding only. One binding may name several roles, and one actor may then hold them all.

#### Scenario: one binding, two roles

- Given a set whose policy names three roles and whose one binding gives an actor two of them, the third held by another bound actor
- When it is founded
- Then the Accord founds, and the actor holds both roles

#### Scenario: a binding that expires at the founding time

- Given a set whose binding's `until` equals the founding time
- When it is founded
- Then it is refused with `binding-expired`

#### Scenario: a time the timestamp rule refuses in until

- Given three sets, each with nothing else wrong, whose binding's `until` is `2026-12-31T23:59:60Z`, `2026-12-31T24:00:00Z` and `2027-02-29T00:00:00Z`
- When each is founded
- Then each is refused with `invalid-shape`, since the timestamp rule allows no second 60, no hour 24 and no day the calendar lacks

### setup-contract-3 Binding checks

For each declaration, founding finds the binding whose `actor` is the declaration's, and checks it in this order: one exists (`binding-missing`); its `principal` is one of the policy's `principals` (`binding-principal`); its signature verifies under that principal (`binding-signature`); its `mediator` is the set's `mediator.actor` (`binding-mediator`); its `until` is later than the founding time (`binding-expired`). Before any declaration, two bindings for one actor are `binding-duplicate`. After every declaration, a binding for an actor with no declaration is `declaration-missing`.

#### Scenario: a binding by an unlisted principal

- Given a set whose binding is signed by a key the policy does not list, and names that key as its principal
- When it is founded
- Then it is refused with `binding-principal`

#### Scenario: a binding for another mediator

- Given a set whose binding names a mediator other than the set's `mediator.actor`
- When it is founded
- Then it is refused with `binding-mediator`

#### Scenario: a binding with no declaration

- Given a set with two declarations and three bindings, the third for an actor with no declaration
- When it is founded
- Then it is refused with `declaration-missing`

### setup-contract-4 The card

The card says what can change a verdict under the Accord (`card.json`). Its `spec` is `0.1`, the version of this protocol. Its `kernel` names the decision module by a `version` and a `hash`, which the operator writes and founding reads for shape only. Its `judges` is a reserved list, read for shape only: each entry a `name`, a `version` and a `hash`. Its `checks` lists the checks the module loads, each with a `name`, a `version` and a `hash`; in this version it is empty. The card holds no other member. Its typed hash is the card hash, which every receipt carries (receipt-3), so every receipt names the module that judged it.

#### Scenario: a card that loads a check

- Given a set whose card's `checks` lists one check
- When it is founded
- Then it is refused with `invalid-shape`

#### Scenario: a member the card does not list

- Given a set whose card has a member `models`
- When it is founded
- Then it is refused with `invalid-shape`

### setup-contract-5 The Accord id

The Accord id is the typed hash of the founding set's canonical bytes; the card hash is the typed hash of the card's. The same set founds to the same id at any time, and any change to the set, a term or a role included, founds to another. Every role the policy's `roles` names must be held, through a binding, by an actor that has a declaration in the set, or the set is refused with `role-missing`. A role held but not in the policy is allowed, and the policy's `roles` may be empty.

#### Scenario: the same set twice

- Given one founding set, founded at two different times
- When the two Accord ids are compared
- Then they are equal

#### Scenario: a policy role nobody holds

- Given a set whose policy names the role `auditor`, which no binding gives
- When it is founded
- Then it is refused with `role-missing`

### setup-contract-6 Membership

The members of an Accord are the actors with a declaration in its founding set. The mediator's key is a member only when it has a declaration there. Every operation on an Accord that has a caller, a give, a fetch and an answer, authenticates its caller by key and refuses a caller that is not a member with `not-a-member`, before it reads any other input. A refused operation changes nothing.

#### Scenario: a stranger

- Given an Accord, and a caller whose key has no declaration in its set
- When the caller gives a part, or fetches one
- Then the operation is rejected with `not-a-member`, whatever the rest of its input holds

#### Scenario: the mediator's own key

- Given an Accord whose set has no declaration by the mediator's key
- When the mediator's key calls an operation
- Then it is rejected with `not-a-member`

### setup-contract-7 The mediator and its operator

The set's `mediator` names who runs the Accord: `actor`, the mediator's reference, which signs every receipt; `operator`, the reference of the party that runs the mediator; and `runs`, the class of place it runs in, which in this version is `process`. All three are hashed into the Accord id, so every party founds with them. A mediator founds only a set whose `mediator.actor` is its own key; asked to found any other, it refuses, and that refusal is not a founding code, since no set is wrong for it.

#### Scenario: a class other than process

- Given a set whose `mediator.runs` is `container`
- When it is founded
- Then it is refused with `invalid-shape`

#### Scenario: the operator changes the id

- Given two sets that differ only in `mediator.operator`
- When both are founded
- Then their Accord ids differ

### setup-contract-8 The refusal order

Founding runs the rows of the table in order and returns the first that fails: the canonical form, the shape, one binding per actor; then for each declaration in actor-reference order its actor, its signature, its terms, its maybe rules, its checks, and its binding's presence, principal, signature, mediator and time; then a binding with no declaration; then a policy role nobody holds. A refused set founds nothing and yields no id.

#### Scenario: two defects in one declaration

- Given a set whose first declaration has a bad signature and whose binding has expired
- When it is founded
- Then it is refused with `declaration-signature`

#### Scenario: defects in two declarations

- Given a set whose first declaration's binding has expired and whose second declaration has a malformed term
- When it is founded
- Then it is refused with `binding-expired`, since the first declaration's rows all run first

### setup-contract-9 The shape of a set

A founding set is an object with exactly `policy`, `mediator`, `card`, `declarations` and `bindings` (`founding.json`). `policy` has exactly `roles`, a list of distinct labels, and `principals`, a non-empty list of distinct actor references. `mediator` is as setup-contract-7 says and `card` as setup-contract-4 says. `declarations` and `bindings` are non-empty lists. Bytes the canonical form refuses are `not-canonical`, before the shape is read, and no id is computed for them. Every defect of shape is `invalid-shape`, except in the members a later row reads: a declaration's `actor` form, its `sig` and its `terms` belong to rows 4 to 8.

#### Scenario: a repeated member name

- Given the bytes `{"policy":{},"policy":{}}` as a founding set
- When they are founded
- Then they are refused with `not-canonical`

#### Scenario: an actor of the wrong type

- Given a set whose declaration's `actor` is the number 7, and separately one whose `actor` is the string `alice`
- When each is founded
- Then the first is refused with `invalid-shape`, since the order of the list reads `actor` as a string, and the second with `actor-reference`

#### Scenario: a term with a missing member

- Given a set whose declaration has a term with no `dir`, correctly signed
- When it is founded
- Then it is refused with `term-invalid`, not `invalid-shape`

### setup-contract-10 The order of the lists

Actor-reference order is ascending order of references by their UTF-16 code units; for references of the form signing-1 allows, that is byte order. `declarations` are in strictly ascending order of `actor`, so two declarations by one actor are `invalid-shape`. `bindings` are in ascending order of `actor`, two equal neighbours allowed, so that two bindings for one actor reach row 3 as `binding-duplicate`. A set is never reordered: a list out of order is `invalid-shape`.

#### Scenario: declarations out of order

- Given a set whose two declarations are in descending order of `actor`
- When it is founded
- Then it is refused with `invalid-shape`

#### Scenario: two bindings for one actor

- Given a set with two bindings for one actor, next to each other
- When it is founded
- Then it is refused with `binding-duplicate`

## 4. Giving and getting a part at the door

The door is where a member gives a part to another member and fetches what was queued for it. A give carries two byte strings, the message and the envelope, from an authenticated caller at a time. The door authenticates before it reads the message: it checks that the caller is a member, then the envelope's signature under the caller's key, then the message's canonical bytes, then that the envelope names those bytes and a fresh sequence number, and only then the message's content. A give that fails any row is rejected: it writes nothing and changes nothing (crossing-7). A give that passes every row is decided by composition (section 5), and the mediator writes its receipt.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Member | The caller's key, against the Accord's members (setup-contract-6) | `not-a-member` |
| 2 | Envelope signature | The envelope bytes: accepted by the canonical form, an object with exactly `seq`, `hash` and `sig`, its `sig` verifying under the caller's key and `https://placket.dev/envelope/0.1` (signing-2) | `envelope-signature` |
| 3 | Canonical message | The message bytes (canonical-1) | `not-canonical` |
| 4 | Envelope hash | The envelope's `hash`: the typed hash of the message's canonical bytes (crossing-4) | `envelope-hash` |
| 5 | Sequence | The envelope's `seq`: a whole number from 1 to 2^53 − 1, above the caller's highest accepted `seq` in this Accord (crossing-4) | `envelope-sequence` |
| 6 | Crossing message | The message's frame: an object; no member but `messageId`, `role`, `parts`, `extensions` and `metadata`; `messageId` a non-empty string; `role` `ROLE_AGENT` or `ROLE_USER`; `extensions` exactly the crossing extension's URI; `metadata` holding exactly the extension's object under it; and that object's `re`, `topic` and `gate`, and no member it does not list. `to`, `accord` and `parts`, missing included, belong to rows 7, 8 and 9 (crossing-1) | `extension-missing` |
| 7 | Receiver | The extension's `to`: present, an actor reference of a member (crossing-1) | `receiver-unknown` |
| 8 | Accord | The extension's `accord`: present, this Accord's id | `accord-mismatch` |
| 9 | One part | The message's `parts`: present, a list of exactly one element | `one-part` |
| 10 | One body | The part: an object with exactly one of the body members `text`, `raw`, `data` and `url`, a `text` that is a string, a `raw` in standard base64, a `data` that is not `null`, and no member but that body and `metadata` (crossing-2) | `one-body` |
| 11 | Body form | A `url` body, whatever its value (crossing-2) | `url-unsupported` |
| 12 | Part metadata | The part's `metadata`: exactly the crossing extension's object, with a `kind` and a `salt` of at least 16 bytes (crossing-2) | `part-metadata` |

A fetch carries only the authenticated caller.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Member | The caller's key (setup-contract-6) | `not-a-member` |

A member's fetch returns the first delivery queued for it, once, or nothing when none is queued (crossing-3). A fetch writes no receipt.

### crossing-1 The crossing message

A part crosses in an A2A protocol 1.0 message that carries the crossing extension, `https://placket.dev/crossing/0.1` (`message.json`). The message has exactly the members `messageId`, a non-empty string; `role`, `ROLE_AGENT` for a part a member gives and `ROLE_USER` for one the mediator forms from an answer (maybe-8, maybe-9); `parts`, the list of its one part; `extensions`, the list holding exactly the crossing extension's URI; and `metadata`, holding exactly one member, under that URI, the crossing extension's object. That object has `accord`, the Accord's id; `to`, the receiver's reference; and it may have `re`, the id of the receipt the message answers; `topic`, a non-empty string that groups crossings; and `gate`, reserved for a later version and read for shape only: lowercase letters and digits in words joined by single hyphens or underscores, at most 64 characters, such as `out_you`. The sender is not a member of the message: it is the caller, whose key signed the envelope (crossing-6). The extension has no `from`, so a give whose extension carries one is `extension-missing`, as for any member the frame does not list, whatever its value; no member of a message can name a sender other than the caller. The door accepts either `role` from a member. No check reads `topic`, `gate` or `role` but for their shape, and no receipt or delivery carries `topic` or `gate`.

#### Scenario: a message from another Accord

- Given a member's correctly signed give whose message names another Accord's id
- When the door checks it
- Then it is rejected with `accord-mismatch`

#### Scenario: an unknown receiver

- Given a member's correctly signed give whose `to` is the reference of a key with no declaration
- When the door checks it
- Then it is rejected with `receiver-unknown`

#### Scenario: a member the frame does not list

- Given a member's correctly signed give whose message also has `contextId`, and separately one whose extension object has `from`, naming the caller or another member
- When the door checks each
- Then each is rejected with `extension-missing`

#### Scenario: a reserved gate

- Given a member's correctly signed give whose extension has `gate` `out_you`, and separately one with `gate` `Out_You`
- When the door checks each
- Then the first is decided as if it had no `gate`, and the second is rejected with `extension-missing`

### crossing-2 The part

The one part has exactly one body and the crossing extension's metadata (`part.json`). The body is `text`, a string; `raw`, bytes in standard base64; or `data`, any JSON value but `null`. A part with no body, with two, or with any member but its body and `metadata` is `one-body`, and so is a `raw` that is not standard base64 in its one encoding. A `url` body is not a body of this version: a part whose one body is `url` is `url-unsupported`, whatever the value. The part's `metadata` holds exactly one member, under the crossing extension's URI, with exactly `kind`, a kind as setup-contract-1 defines it, and `salt`, at least 16 bytes in lowercase hex; any other is `part-metadata`. The body's bytes are the UTF-8 of `text`, the decoded bytes of `raw`, or the canonical bytes of `data`.

#### Scenario: two bodies

- Given a correctly signed give whose part has both `text` and `data`
- When the door checks it
- Then it is rejected with `one-body`

#### Scenario: a url body

- Given a correctly signed give whose part's one body is `url`
- When the door checks it
- Then it is rejected with `url-unsupported`

#### Scenario: a short salt

- Given a correctly signed give whose part's salt is 15 bytes
- When the door checks it
- Then it is rejected with `part-metadata`

### crossing-3 Digests, salts and deliveries

A part is named in a receipt by its kind and its salted digest: the lowercase hex SHA-256 of the salt's bytes followed by the body's bytes. The first holder of a part mints its salt, at least 16 random bytes; the mediator mints the salt of every part it forms. No receipt carries a body or a salt. When a part passes, the mediator queues for its receiver a delivery (`delivery.json`): the receipt that passed it and its id, the part's `messageId`, `kind` and `salt`, the message's `re` when it has one, and the body, as `text`, `raw` or `data` as it crossed. A delivery holds nothing else. Its format is frozen because a receiver checks the receipt and the digest with it and every scenario vector compares it member by member (conformance-vectors-5). A door hands a delivery out in a reply of its own format, and where the delivery's body is an answer or a changed version the door may attach beside it the call by which the answerer signed that answer. That call is the door's format, not a member of the delivery: no receipt covers it, and no check of this protocol reads it. Deliveries are queued per receiver in the order of their receipts, and each is handed out once, to its receiver only, by a fetch. The mediator keeps no body or salt of a part that was blocked, or of a delivery once fetched.

#### Scenario: whoever holds the body and salt can show the part

- Given a delivery, and the salted digest its receipt names
- When the SHA-256 of the delivery's salt bytes followed by its body bytes is computed
- Then it equals the digest

#### Scenario: a delivery is fetched once

- Given a part that passed to its receiver
- When the receiver fetches twice
- Then the first fetch returns the delivery and the second returns nothing

#### Scenario: no delivery for a block

- Given a give that the terms block
- When its receiver fetches
- Then nothing is returned

### crossing-4 The envelope

The envelope is the sender's signed claim to one submission (`envelope.json`): `seq`, its sequence number in this Accord, `hash`, the typed hash of the message's canonical bytes, and `sig`, the sender's signature under `https://placket.dev/envelope/0.1`. A message edited after its envelope was signed is `envelope-hash`. A `seq` at or below the highest the caller has had accepted in this Accord, or not a whole number from 1 to 2^53 − 1, is `envelope-sequence`, so a replayed envelope is refused. A `seq` is accepted when its give writes a receipt; a rejected give consumes none. A `seq` may skip numbers: it need only be above the last accepted.

#### Scenario: an edited message

- Given a member's envelope signed over one message, given with that message changed by one byte
- When the door checks it
- Then it is rejected with `envelope-hash`

#### Scenario: a repeated sequence number

- Given a member whose give at `seq` 1 was accepted
- When it gives again at `seq` 1
- Then it is rejected with `envelope-sequence`

#### Scenario: a skipped sequence number

- Given a member whose give at `seq` 1 was accepted
- When it gives at `seq` 3
- Then the give is decided, and its receipt echoes `seq` 3

### crossing-6 The door's order

The door runs the rows of its table in order and returns the first that fails. It authenticates first: membership, then the envelope's signature under the caller's key, then the message's canonical bytes, then that the envelope's `hash` names those bytes and its `seq` is fresh, then the message's content. No member of the message is read before the message is known to be the one the caller signed for. The caller is the sender because the envelope verified under the caller's key: no member of the message names the sender, and a `from` in the extension is a member the frame does not list, `extension-missing` at row 6 (crossing-1). The message is not read at all before the envelope's signature has verified, so a stranger's give is `not-a-member` and a member's give under a bad envelope signature is `envelope-signature`, whatever the message holds. Everything decided is read from the message's canonical bytes, so a member name repeated anywhere in the message is `not-canonical`, at row 3, whichever copy is wrong. Row 2 reads the envelope's bytes, its members' names and its `sig`, so bytes the canonical form refuses, a member missing or not listed, and a bad signature are all `envelope-signature`; the value of its `hash` belongs to row 4 and of its `seq` to row 5.

#### Scenario: a repeated member behind a bad signature

- Given a member's give whose message repeats a member name and whose envelope's signature does not verify
- When the door checks it
- Then it is rejected with `envelope-signature`

#### Scenario: a repeated member under a good signature

- Given a member's give whose envelope verifies and whose message has the member `topic` twice, the second copy holding the correct value
- When the door checks it
- Then it is rejected with `not-canonical`

#### Scenario: a repeated seq in the envelope

- Given a member's give whose envelope bytes have the member `seq` twice, with a correct message
- When the door checks it
- Then it is rejected with `envelope-signature`, not `not-canonical`, since row 2 reads the envelope's bytes

#### Scenario: two content defects

- Given a member's correctly signed give whose message names another Accord and has two parts
- When the door checks it
- Then it is rejected with `accord-mismatch`

#### Scenario: a message that is not the one signed for

- Given a member's correctly signed envelope over one message, given with another message that names another Accord
- When the door checks it
- Then it is rejected with `envelope-hash`, and the other message's content is not read

### crossing-7 A rejection changes nothing

A rejected give writes no receipt, consumes no sequence number, queues nothing and holds no body. The next give is checked as if the rejected one had not been made. A rejected fetch hands out nothing.

#### Scenario: six rejections, then a pass

- Given a member that gives six times at `seq` 1, each with a different defect of rows 6 to 12
- When it then gives a good message at `seq` 1
- Then the six are rejected, and the seventh is decided and its receipt echoes `seq` 1 and is the first of its pair

#### Scenario: a stranger's bytes are not read

- Given a caller with no declaration whose message bytes are not JSON
- When it gives
- Then it is rejected with `not-a-member`, not `not-canonical`

## 5. Composition and the verdict

Composition decides a crossing that the door, or an answer (section 6), hands it: a sender, a receiver, the part's kind, and whether the part was given by a member or formed by the mediator from an answer. It reads the two parties' terms and the roles their bindings give them, and returns a verdict and a reason. It refuses nothing: every crossing that reaches it gets a receipt. A term matches when it is the sender's of `dir` `give` or the receiver's of `dir` `get`, its `select.kind` holds the part's kind, and one of its `with` selectors names a role the other party holds (composition-1). The rows run in order, and the first that applies decides.

| # | Check | Reads | Verdict and reason |
|---|---|---|---|
| 1 | A refusal | Every matching term, for stance `no` (composition-3) | `block`, `no-match` |
| 2 | Both sides | The matching terms: at least one of the sender's and at least one of the receiver's (composition-2) | `block`, `no-match` |
| 3 | Checks | Every check a matching term names, each run once (composition-4) | `block`, `check:<name>` of the first that fails |
| 4 | Formed part | A matching term of stance `maybe`, when the part was formed by the mediator (composition-5) | `block`, `maybe` |
| 5 | Maybe rules | Every matching term of stance `maybe`: the set of roles its `resolve.to` names, and its `timeout` (composition-5) | `block`, `maybe`, when two differ |
| 6 | Ask | A matching term of stance `maybe`, all rules agreeing (composition-5, maybe-1) | `escalate`, `maybe` |
| 7 | Pass | Everything else | `pass`, `ok` |

### composition-1 Matching terms

A term of the sender matches a crossing when its `dir` is `give`, its `select.kind` holds the part's kind, and one of its `with` selectors `role:<label>` names a role the receiver's binding gives it. A term of the receiver matches when its `dir` is `get`, its `select.kind` holds the kind, and one of its `with` selectors names a role the sender's binding gives it. Roles are read from bindings only (setup-contract-2). A party that sends to itself is matched on both sides by its own terms.

#### Scenario: the applier's get term

- Given the ask-and-log Accord, where the applier wants to get `proposal` with `role:proposer`, and the proposer's binding gives it `proposer`
- When composition reads a `proposal` from the proposer to the applier
- Then the applier's term matches

#### Scenario: a role the binding does not give

- Given a receiver whose get term names `role:auditor`, and a sender whose binding does not give it `auditor`
- When composition reads the crossing
- Then that term does not match

### composition-2 No match

A crossing with no matching term of the sender, or no matching term of the receiver, is `block` with reason `no-match`. A kind no term selects therefore never crosses.

#### Scenario: no get term

- Given a sender with a matching give term and a receiver with no get term for the kind
- When composition decides the crossing
- Then it is `block`, `no-match`

#### Scenario: a kind no term names

- Given the ask-and-log Accord
- When the proposer gives a part of kind `tests` to the applier
- Then it is `block`, `no-match`

### composition-3 A refusal wins

When any matching term, of either party, has stance `no`, the crossing is `block` with reason `no-match`, before any check runs and before any maybe is read.

#### Scenario: no beside want

- Given a receiver with a matching `want` term and a matching `no` term for the kind
- When composition decides the crossing
- Then it is `block`, `no-match`

### composition-4 Checks

When both parties have a matching term and no matching term refuses, every check any matching term names runs, each distinct check and corpus once, sender's terms first, in term order. The first that fails decides: `block` with reason `check:` followed by the check's name, and no question is opened. In this version a card loads no check, so a founded Accord has no term that names one, and this row never applies.

#### Scenario: no check runs in this version

- Given any Accord this version founds
- When composition decides any crossing
- Then no check runs, and no receipt has a reason that starts `check:`

### composition-5 Maybe

When a matching term has stance `maybe` and no earlier row applied, the crossing is asked about, under the rule the maybe terms agree on. Every matching `maybe` term must name the same set of roles in `resolve.to`, in any order, and the same `timeout`; then the verdict is `escalate` with reason `maybe` and the mediator holds the part as a question (maybe-1). When two of them differ, the crossing is `block` with reason `maybe` and no question is opened: no value a party chooses when it gives can pick which rule applies. A part the mediator formed from an answer that meets a `maybe` term is `block` with reason `maybe`, and no second question is opened.

#### Scenario: maybe rules that agree

- Given a sender's maybe term with `resolve.to` `["role:decider","role:lead"]` and a receiver's with `["role:lead","role:decider"]`, both with `timeout` 60
- When composition decides the crossing
- Then it is `escalate`, `maybe`

#### Scenario: maybe rules that differ

- Given the same terms with timeouts 60 and 120
- When composition decides the crossing
- Then it is `block`, `maybe`

#### Scenario: an answer that meets a maybe term

- Given an asker with a `maybe` term to get `answer` from the answerer's role
- When an accepted answer is decided from the answerer to the asker
- Then it is `block`, `maybe`, and no question is opened

### composition-6 Need

A term of stance `need` composes exactly as one of stance `want`.

#### Scenario: need meets want

- Given a sender's `need` give term and a receiver's `want` get term that both match
- When composition decides the crossing
- Then it is `pass`, `ok`, as for two `want` terms

## 6. Questions, the three answers and release

A give that composition decides `escalate` opens a question: the mediator writes the ask's receipt, holds the part, and queues nothing for the receiver (maybe-1). The question's id is the id of that receipt. A question closes exactly once: by an answer, an accept, a change, a decline or a cancel, from a member holding a role it names; by the mediator finding that nobody can be reached to answer it; or by its deadline. An answer is given with the question's id, the action, and for an accept or a change the answer's text and the salt the mediator minted for it, and whether the answer asks for release (maybe-10). It runs these rows in order.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Member | The caller's key (setup-contract-6) | `not-a-member` |
| 2 | Open question | The question's id: a question that is open, at a time before its deadline (maybe-7) | `no-open-question` |
| 3 | Answerer | The caller, among the question's answerers (maybe-6) | `not-an-answerer` |

A rejected answer writes nothing and leaves the question as it was. The answer's own inputs are its caller's to get right, and are checked after these rows with no protocol code: an action that is not one of the four, an accept or a change without a salt of at least 16 bytes, or a change with an empty text, is refused as the caller's misuse, writes nothing and leaves the question open. An answer that passes the rows closes the question and writes its receipts, all of them before any state changes: the resolution receipt first, then the release receipt when one is written. The mediator's own finding that a question cannot be delivered reads only row 2. Its deadline is reached by the passing of time, which reads nothing and refuses nothing (maybe-5).

The answerers of a question are the members whose bindings give them a role its rule names in `resolve.to`, in actor-reference order. The list may be empty: founding checks that the policy's roles are held, not that a rule's are.

| Closing | Receipt from, to | Subject | Verdict, reason | Queued |
|---|---|---|---|---|
| Accept (maybe-3) | The answerer, to the asker | The answer, kind `answer` | As composition decides the answer | The answer, for the asker, on a pass |
| Change (maybe-9) | The answerer, to the asker | The version, kind `changed-version` | As composition decides the version | The version, for the asker, on a pass |
| Decline, cancel (maybe-4) | The first answerer, to the asker | The asked part | `block`, `declined` or `cancelled` | Nothing |
| Undeliverable (maybe-4) | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `undeliverable` | Nothing |
| Timeout (maybe-5) | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `timeout` | Nothing |

Every one of these is a resolution receipt that echoes the question's envelope (crossing-5). An accept or a change that passes, and asks for release, then writes a release receipt (maybe-10).

### maybe-1 Asking

A give decided `escalate` writes a crossing receipt with verdict `escalate` and reason `maybe`, from the giver to the receiver, and holds a question under that receipt's id: the asker, the receiver, the part as it was given, its message's `messageId` and `re`, the envelope, the agreed rule, and the deadline. The deadline is the give's time plus the rule's `timeout` in seconds; a deadline past `9999-12-31T23:59:59Z` is that time. Nothing is queued for the receiver. The held part leaves the mediator only by a release (maybe-10).

#### Scenario: a proposal is asked about

- Given the ask-and-log Accord, whose proposer gives `proposal` to the applier under a maybe term with a timeout of 3600
- When the proposer gives a proposal at `2026-11-12T09:00:00Z`
- Then the receipt is `escalate`, `maybe`, a question is open with deadline `2026-11-12T10:00:00Z`, and the applier's fetch returns nothing

#### Scenario: a deadline past the last time

- Given a maybe term with `timeout` 9007199254740991
- When a part is asked about under it
- Then the question's deadline is `9999-12-31T23:59:59Z`

### maybe-3 An accepted answer

An accept forms the answer as maybe-8 says and decides it through composition from the answerer to the asker, as a part formed by the mediator. The resolution receipt's verdict and reason are composition's, its subject is the answer, and it echoes the question's envelope. A pass queues the answer for the asker. A block queues nothing, for anyone: the held part is dropped. Every check a term names runs on the answer as on any part.

#### Scenario: an accept that passes

- Given an open question in the ask-and-log Accord
- When the decider accepts it with the text `accepted`
- Then one resolution receipt is written from the decider to the proposer, `pass`, `ok`, of kind `answer`, echoing the question's envelope, and the answer is queued for the proposer

#### Scenario: an accept the asker's terms do not take

- Given an open question whose asker has no term to get `answer` from the answerer's role
- When an answerer accepts it
- Then the resolution receipt is `block`, `no-match`, nothing is queued, and the question is closed

### maybe-4 Decline, cancel and undeliverable

A decline and a cancel each close the question with a resolution receipt from the question's first answerer, by actor-reference order, to the asker: `block` with reason `declined` or `cancelled`, naming the asked part by its kind and digest and echoing the question's envelope. The receipt names the first answerer whichever answerer closed it. When the mediator finds that nobody can be reached to answer a question, it closes it the same way with reason `undeliverable`; when the question has no answerer, the receipt is from the mediator's own reference. An undeliverable and a timeout are the only resolution receipts that can be from the mediator. Nothing is queued, and the held part is dropped.

#### Scenario: a decline

- Given an open question in the ask-and-log Accord
- When the decider declines it
- Then one receipt is written from the decider to the proposer, `block`, `declined`, of kind `proposal` with the asked part's digest, and nothing is queued for the applier

#### Scenario: a question nobody can answer

- Given a question whose rule names a role no member holds
- When the mediator closes it as undeliverable
- Then the receipt is from the mediator's reference to the asker, `block`, `undeliverable`

### maybe-5 Timeout

As time passes the mediator closes, in the order they were opened, every open question whose deadline is at or before the current time, each with a resolution receipt from the first answerer, or the mediator when there is none, to the asker: `block` with reason `timeout`, naming the asked part, echoing the question's envelope, and with `at` the deadline, not the time it was noticed. A passing of time that reaches no deadline writes nothing.

#### Scenario: one receipt at the deadline

- Given a question with deadline `2026-11-02T09:00:01Z`
- When time is passed to `2026-11-02T09:00:05Z`
- Then exactly one receipt is written, `block`, `timeout`, with `at` `2026-11-02T09:00:01Z`

#### Scenario: before the deadline

- Given a question with deadline `2026-11-02T09:00:01Z`
- When time is passed to `2026-11-02T09:00:00Z`
- Then nothing is written and the question stays open

### maybe-6 Only an answerer answers

An answer from a member that is not among the question's answerers is rejected with `not-an-answerer`, writes nothing, and leaves the question open. The asker may answer its own question when its binding gives it a role the rule names.

#### Scenario: the proposer answers its own proposal

- Given an open question in the ask-and-log Accord, where only the decider holds `decider`
- When the proposer accepts it
- Then it is rejected with `not-an-answerer`, nothing is written, and the question stays open

### maybe-7 A closed question

An answer, or a finding that a question cannot be delivered, for an id that names no open question, or at a time at or after the question's deadline, is rejected with `no-open-question` and writes nothing. A question closes in the operation that writes its resolution receipt, and its release is written in that same operation, never by a later one. Row 2 comes before row 3, so a member who holds no answering role learns only that no question is open for it.

#### Scenario: a second answer

- Given a question the decider has accepted
- When the decider declines it
- Then it is rejected with `no-open-question` and nothing is written

#### Scenario: an answer at the deadline

- Given an open question with deadline `2026-11-12T10:00:00Z`
- When the decider accepts it at `2026-11-12T10:00:00Z`, before the passing of time has closed it
- Then it is rejected with `no-open-question`, and the next passing of time writes the timeout

### maybe-8 The answer's message

An accepted answer is a part the mediator forms: `messageId` `answer:` followed by the question's id, kind `answer`, `re` the question's id, a `text` body holding the answer's text, and the salt the mediator minted for it, at least 16 bytes. Its message's `role` is `ROLE_USER`. The answer's text may be any string.

#### Scenario: the delivered answer

- Given an accept of question `sha256:` followed by `ab` repeated 32 times, with text `accepted`, that passed
- When the asker fetches
- Then the delivery's `messageId` is `answer:sha256:` followed by `ab` repeated 32 times, its kind `answer`, its `re` the question's id, its `text` `accepted`, and its `salt` the one minted

### maybe-9 A changed answer

A change is an answerer's own version of the asked part, as a non-empty text. The mediator forms one part from it: `messageId` `change:` followed by the question's id, kind `changed-version`, `re` the question's id, a `text` body holding the version, and one salt the mediator minted, at least 16 bytes, with `role` `ROLE_USER`. It decides that part from the answerer to the asker as it decides an accepted answer, and the resolution receipt's subject is the version, of kind `changed-version`: that is how a change is recorded. A pass queues the version for the asker. A change with an empty version is the caller's misuse: it is refused with no protocol code, writes nothing and leaves the question open. The asked part is never released after a change.

#### Scenario: a change returned to the asker

- Given an open question in the ask-and-log Accord
- When the decider changes it with the text `Rename total to amount_due.`, without release
- Then one resolution receipt is written from the decider to the proposer, `pass`, `ok`, of kind `changed-version`, and the version is queued for the proposer with `re` the question's id

#### Scenario: an empty change

- Given an open question
- When an answerer changes it with an empty text
- Then nothing is written and the question stays open

### maybe-10 Release

An answer may ask for release. When it does and its resolution receipt's verdict is `pass`, the mediator judges the part for the question's receiver in the same operation, and writes a release receipt that echoes the question's envelope, after the resolution receipt and before any state changes.

- After an accept, the held part is decided again, as given, from the asker to the receiver, with the checks of that moment. When composition returns `block`, the release receipt is a crossing receipt from the asker to the receiver, `block` with composition's reason, naming the asked part, and nothing is queued for the receiver. On any other verdict the answer stands, and it is the answer that releases: the release receipt is a crossing receipt from the asker to the receiver, `pass`, `ok`, with the asked part's kind and digest unchanged, and the held part is queued for the receiver exactly as it was given. The mediator adds no verdict of its own. In this version the terms are fixed and no check is loaded, so this decision returns `escalate` again and the release passes.
- After a change, the same version, under the same salt and so with the same digest, is decided from the answerer to the receiver as a part formed by the mediator. The release receipt is a resolution receipt from the answerer to the receiver, of kind `changed-version`, with composition's verdict and reason. A pass queues the version for the receiver. When the receiver is the answerer or the asker, a change releases nothing.
- A decline, a cancel, an undeliverable, a timeout, an answer whose resolution receipt is `block`, and an answer that does not ask for release write no release receipt and queue nothing for the receiver. The held part is dropped.

A release receipt consumes no sequence number of any sender. A question has at most one.

#### Scenario: an accept releases the asked part with its digest unchanged

- Given an open question in the ask-and-log Accord, asked with a proposal of digest D
- When the decider accepts it, asking for release
- Then two receipts are written in one operation: the resolution from the decider to the proposer, `pass`, `ok`, of kind `answer`; then a crossing receipt from the proposer to the applier, `pass`, `ok`, of kind `proposal` with digest D, echoing the question's envelope; and the applier's fetch returns the proposal as it was given

#### Scenario: a change returned to the asker and released to the receiver under one digest

- Given an open question in the ask-and-log Accord
- When the decider changes it, asking for release
- Then two resolution receipts are written, from the decider to the proposer and from the decider to the applier, both `pass`, `ok`, of kind `changed-version` with one digest, both echoing the question's envelope; the version is queued for both; and the proposal is never queued for the applier

#### Scenario: a release the module blocks queues nothing

- Given an accepted answer that passed, and a decision module of a later version, one that loads a check, that returns `block` for the held part at the release
- When the release is decided
- Then the release receipt is a crossing receipt from the asker to the receiver, `block` with the module's reason, and nothing is queued for the receiver; the answer stays queued for the asker

#### Scenario: a release receipt echoes the question's envelope and counts as an echo for the asker only

- Given a change released to the applier, whose release receipt is addressed to the applier and echoes the proposer's envelope
- When the applier's kept envelopes are reconciled
- Then the release receipt counts as an echo of the proposer's envelope only, and of no envelope of the applier's

#### Scenario: a decline releases nothing

- Given an open question in the ask-and-log Accord
- When the decider declines it
- Then one receipt is written, no release receipt follows, and nothing is queued for the applier

## 7. Receipts and their chains

A receipt is the mediator's signed statement of one decision. Writing one refuses nothing: the mediator writes a receipt for every give the door passes and for every closing and release of a question, and for nothing else. This table says which receipt each event writes; every receipt is chained in the pair of its `from` and `to` (receipt-5).

| Event | `kind` | `from`, `to` | Subject | `verdict`, `reason` | Echoed envelope |
|---|---|---|---|---|---|
| A give the door passes | `crossing` | The giver, the receiver | The given part | Composition's (section 5) | The give's |
| An accept or a change | `resolution` | The answerer, the asker | The answer or the version | Composition's | The question's |
| A decline, a cancel | `resolution` | The first answerer, the asker | The asked part | `block`, `declined` or `cancelled` | The question's |
| An undeliverable, a timeout | `resolution` | The first answerer or the mediator, the asker | The asked part | `block`, `undeliverable` or `timeout` | The question's |
| A release after an accept | `crossing` | The asker, the receiver | The asked part | `pass`, `ok`, or composition's block | The question's |
| A release after a change | `resolution` | The answerer, the receiver | The version | Composition's | The question's |

### receipt-1 The receipt

A receipt is a DSSE 1.0 envelope (`receipt.json`) with exactly `payloadType`, `application/vnd.in-toto+json`; `payload`, the standard base64 of the canonical bytes of the receipt's statement; and `signatures`, a list of exactly one entry with exactly `keyid`, the mediator's reference, and `sig`, the standard base64 of the mediator's Ed25519 signature over the DSSE pre-authentication encoding of the payload type and the payload's bytes (signing-2). The statement is an in-toto Statement v1 (`statement.json`): `_type` `https://in-toto.io/Statement/v1`, `subject`, `predicateType` `https://placket.dev/receipt/0.1`, and `predicate`. A receipt's id is the typed hash of the canonical bytes of its statement. The mediator writes the payload in canonical form, so putting it in canonical form again changes nothing.

#### Scenario: the anatomy of a receipt

- Given any receipt a mediator wrote
- When its payload is decoded from base64
- Then the bytes are the canonical form of a statement whose `_type` and `predicateType` are those above, the signature verifies under the mediator's key over the pre-authentication encoding of `application/vnd.in-toto+json` and those bytes, and the receipt's id is their typed hash

### receipt-2 The subject

The statement's `subject` is a list of exactly one entry, the part the receipt is about: `name`, the part's kind, and `digest`, an object with exactly `salted-sha256`, the part's salted digest of crossing-3 in lowercase hex, with no prefix. A block names the part that was attempted, and a closing that queues nothing names the asked part.

#### Scenario: a block names the attempted part

- Given a give of kind `proposal` that composition blocks
- When its receipt is read
- Then its subject's `name` is `proposal` and its digest is the salted digest of the given part

### receipt-3 What a predicate holds

The predicate (`predicate.json`) has exactly these members: `accord`, the Accord's id; `card`, the card hash; `mediator`, the mediator's reference; `kind`, `crossing` or `resolution`; `from` and `to`, the references of the receipt's two parties; `seq` and `prev`, its place in its chain (receipt-5); `at`, its time (receipt-10); `envelope`, the echoed envelope: `sender`, the reference of the key that signed it, and its `seq` and `hash` (crossing-5); `verdict`; and `reason`. In a crossing receipt the envelope's `sender` is the receipt's `from`: a give's receipt echoes the giver's own envelope, and a release after an accept the asker's, which is the question's. A member a receipt cannot fill is absent, never `null`: `prev` is absent exactly when `seq` is 1, and no other member is ever absent.

#### Scenario: the first receipt of a pair

- Given the first receipt the mediator writes from one party to another
- When its predicate is read
- Then its `seq` is 1 and it has no `prev`

### receipt-4 Verdicts and reasons

A crossing receipt's `verdict` is `pass`, `block` or `escalate`; a resolution receipt's is `pass` or `block`. The reasons are a closed list of eight: `ok`, `no-match`, `maybe` and `check:<name>`, on any receipt; and `declined`, `cancelled`, `timeout` and `undeliverable`, on resolution receipts only. A `pass` has reason `ok`, an `escalate` has reason `maybe`, and a `block` has any other reason, `maybe` included: a crossing whose maybe rules differ is a block with reason `maybe` (composition-5). `check:<name>` is reserved for a check the card loads: no mediator of this version writes it, and a verifier reads it for shape only.

#### Scenario: a block with reason maybe

- Given a crossing predicate with `verdict` `block` and `reason` `maybe`
- When it is validated against `predicate.json`
- Then it is accepted

#### Scenario: a block with reason ok

- Given a crossing predicate with `verdict` `block` and `reason` `ok`
- When it is validated against `predicate.json`
- Then it is refused

#### Scenario: a resolution reason on a crossing receipt

- Given a crossing predicate with `verdict` `block` and `reason` `declined`
- When it is validated
- Then it is refused

### receipt-5 Chains

Receipts are chained per ordered pair of parties, the pair of a receipt's `from` and `to`. The first receipt of a pair has `seq` 1 and no `prev`; each later one has `seq` one above the previous receipt of its pair and `prev` that receipt's id. The chain of a pair runs from one party to the other only: the receipts from the second to the first are another chain. A resolution receipt chains in the pair of its own `from` and `to`, so an answer is in the answerer's chain to the asker, and a release after a change in the answerer's chain to the receiver.

#### Scenario: two receipts in one pair

- Given two gives from the proposer to the applier
- When their receipts are read
- Then the second has `seq` 2 and `prev` the first's id

#### Scenario: the answer's own chain

- Given a give from the proposer to the applier that was asked about, and its accept, the first answer from the decider to the proposer
- When the accept's receipt is read
- Then its `seq` is 1 and it has no `prev`, since it is the first receipt from the decider to the proposer

### receipt-9 A closed predicate

A predicate holds no member but those receipt-3 lists, and each member has the form its schema gives. A predicate with any other member, of any name, is not a receipt predicate.

#### Scenario: an unlisted member

- Given a predicate with a member `result`
- When it is validated against `predicate.json`
- Then it is refused

### receipt-10 Sequence numbers and times

A receipt's `seq`, like an envelope's, is a whole number from 1 to 2^53 − 1. Its `at` is a timestamp, the time of the event the receipt records, given to the mediator by its caller: the time of the give or the answer, and for a timeout the question's deadline. A mediator reads no clock of its own; the same inputs at the same times write the same receipts, byte for byte.

#### Scenario: a timeout's time

- Given a question with deadline `2026-11-02T09:00:01Z`, closed when time is passed to `2026-11-02T09:00:05Z`
- When its receipt is read
- Then its `at` is `2026-11-02T09:00:01Z`

#### Scenario: two mediators, one record

- Given two mediators founded from one set with one key, given the same operations at the same times
- When their receipts are compared
- Then they are identical byte for byte

### crossing-5 Every envelope is echoed

Every receipt carries an envelope in its predicate's `envelope`, as its signer's reference, its `seq` and its `hash`: a give's receipt the give's own, and every receipt that closes or releases a question the question's, whether it is addressed to the asker or to the question's receiver. A release after an accept is therefore a second crossing receipt from the asker with the question's envelope. A receipt counts as an echo of an envelope for the actor that signed that envelope only, the `sender` it names: for a give's receipt its `from`, and for a receipt that closes or releases a question the question's asker, never the receiver it is addressed to. A sender that keeps its envelopes therefore finds every submission that has no receipt (receipt-7).

#### Scenario: every give is echoed

- Given a sender's three accepted gives
- When the receipts are read
- Then each carries its give's `seq` and `hash` in `envelope`

#### Scenario: a resolution echoes the question's envelope

- Given a question asked with envelope `seq` 4, and its decline
- When the decline's receipt is read
- Then its `envelope` is the question's: the asker as `sender`, `seq` 4, and the question's message hash

## 8. Verifying receipts

A verifier needs only the mediator's reference and a list of receipts in the order the mediator wrote them, such as its journal. It checks the reference once, then each receipt in list order, every row of one receipt before the next receipt, and stops at the first failure, which it reports with its code and the receipt's index, counted from 0. An empty list verifies. A list that verifies yields its statements.

| # | Check | Reads | Code |
|---|---|---|---|
| 0 | Mediator | The reference the verifier is given, before any receipt (signing-1) | `reference` |
| 1 | Bytes | The receipt's bytes, and the bytes `payload` decodes to when it is a string of standard base64 in its one encoding (canonical-1) | `not-canonical` |
| 2 | Payload type | `payloadType`: present, exactly `application/vnd.in-toto+json` | `payload-type` |
| 3 | Signature | `payload` as standard base64 in its one encoding; `signatures`, exactly one entry with exactly `keyid`, the given mediator's reference, and `sig`, standard base64 of 64 bytes; the signature, over the pre-authentication encoding of the payload type and the payload's bytes, under the given mediator's key; and no member of the receipt but these three | `signature` |
| 4 | Statement | The statement: exactly `_type`, `subject`, `predicateType` and `predicate`, with the values receipt-1 and receipt-2 give; the predicate's members as receipt-3, receipt-4, receipt-9 and receipt-10 give, but `mediator`, `seq` and `prev`; and in a crossing receipt, the envelope's `sender` equal to its `from` (receipt-3) | `statement-type` |
| 5 | Mediator named | The predicate's `mediator`: the given mediator's reference | `mediator` |
| 6 | Chain | The predicate's `seq` and `prev`, against the receipt before it in the list from the same `from` to the same `to` (receipt-5) | `chain` |
| 7 | Release | The receipt's place among the receipts before it that echo the same envelope (receipt-6) | `release` |

### receipt-6 The verifier

A verifier checks receipts with the mediator's key alone and performs no key lookup: the mediator's reference comes from the verifier's caller, never from a receipt. A receipt edited after signing, or signed again by another key, fails `signature`. A receipt signed by the right key that names another mediator fails `mediator`. In each pair the first receipt in the list must have `seq` 1 and no `prev`, and each later one `seq` one above and `prev` the id of the one before; a receipt removed from the middle of a chain therefore fails `chain` at the receipt after it. A list cut short after any receipt, the journal's last receipts removed, still verifies (receipt-7). Row 7 reads questions from receipts, by this rule, which every reader of a journal applies and none restates:

1. A crossing receipt with verdict `escalate` is a question, and its echoed envelope, `sender`, `seq` and `hash`, is its key. It always opens a question of its own and belongs to no question before it, even one with the same key, as after a shortened journal is resumed.
2. A later receipt that is not a question and echoes the same envelope belongs to the latest question before it with that key. Since row 4 makes a question's `sender` its `from`, every receipt that belongs to a question names the question's `from` as `sender`.
3. A question's closing receipt is the first receipt that belongs to it. It must be a resolution receipt addressed to the question's `from`. When its reason is `declined`, `cancelled`, `timeout` or `undeliverable`, its subject is the question's, name and digest. Otherwise its subject `name` is `answer` or `changed-version`, and when its verdict is `pass` it is the question's answer: an accept when the name is `answer`, a change when it is `changed-version`.
4. After an answer, the receipt right after the closing receipt in the list may belong to the question: it is the release receipt. After an accept it is a crossing receipt from the question's `from` to its `to`, with the question's subject, verdict `pass` or `block`. After a change it is a resolution receipt from the answer's `from` to the question's `to`, with the answer's subject, and the question's `to` is neither its `from` nor the answer's `from`.
5. Every other receipt that belongs to a question fails `release`: one before the closing receipt that is not a resolution addressed to the asker, a closing of the wrong subject, a second release, a release that is not right after its answer, one after a closing that is not an answer, one of the wrong form. So does a resolution receipt that belongs to no question.

A release receipt with verdict `pass` released its part; one with verdict `block` is a blocked release, which queued nothing. A crossing receipt whose envelope's `sender` is not its `from` fails `statement-type` at row 4, so a give's receipt cannot echo another member's envelope, and no envelope counts as echoed for its signer (crossing-5, receipt-7) by a receipt of someone else's give.

#### Scenario: an edited receipt

- Given a journal of three receipts whose second has its payload replaced by the base64 of its statement with the verdict changed, still canonical
- When it is verified
- Then it fails with `signature` at index 1

#### Scenario: a receipt that names another mediator

- Given a receipt signed by the given mediator's key whose predicate names another mediator
- When it is verified
- Then it fails with `mediator` at index 0

#### Scenario: a removed middle receipt

- Given three receipts of one pair, the second removed
- When the remaining two are verified
- Then they fail with `chain` at index 1

#### Scenario: a release with no answer before it

- Given an ask's receipt followed by a release receipt that echoes its envelope, with no resolution between them
- When the two are verified
- Then they fail with `release` at index 1

#### Scenario: a give's receipt that echoes another member's envelope

- Given a correctly signed crossing receipt whose `from` is the proposer and whose envelope's `sender` is the decider
- When it is verified
- Then it fails with `statement-type` at index 0

#### Scenario: a second ask with an earlier question's envelope

- Given an ask's receipt, a second ask's receipt in the same chain that echoes the same envelope, and then a decline addressed to the asker that echoes it
- When the three are verified
- Then they verify: the second ask opens a question of its own and belongs to none, and the decline closes the second question, leaving the first with no closing

#### Scenario: the first failure of the first failing receipt

- Given a list whose receipt at index 2 has a bad `payloadType` and whose receipt at index 3 is not JSON
- When it is verified
- Then it fails with `payload-type` at index 2

#### Scenario: a mediator that is not a reference

- Given the mediator `mediator`, and any list
- When it is verified
- Then it fails with `reference`, and no receipt is read

### receipt-8 Reading without verifying

A receipt can be decoded and shown without being verified: its statement read from its payload, its words printed. Whatever does so says the receipt was not verified, because a forged receipt decodes and shows exactly as a real one does. Anything that presents a receipt as the mediator's decision verifies it first.

#### Scenario: a forged receipt shown

- Given a receipt whose signature is not the mediator's
- When it is decoded and shown without verifying
- Then it shows like any other receipt, and the showing says it was not verified

## 9. Reconciling kept records

Reconciling compares a mediator's receipts, as they are presented, with what each party kept: the envelopes it signed and the receipts of the deliveries it fetched. It names what is in one record and not the other. Its inputs are the mediator's reference, the Accord id and the card hash it expects, the presented receipts in order, and for each party its kept envelopes and delivered receipts. Parties are read in actor-reference order and each party's records in the order kept.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Presented | The mediator's reference and the presented receipts, by every row of section 8 | That row's code, with the index but for `reference` |
| 2 | Presented Accord | Each presented receipt's `accord` and `card`, against the expected (receipt-7) | `presented`, with the index |
| 3 | Kept | Each kept delivered receipt: rows 1 to 5 of section 8, the expected Accord and card, verdict `pass`, `to` the party that kept it (receipt-7) | `kept`, with the party and the index in its list |

### receipt-7 Reconciliation

Reconciling verifies first and reports only when every row passes. It then reports three lists. **Removed**: each receipt a party kept for a delivery it fetched whose id is not among the presented receipts. **Unrecorded**: each presented receipt with verdict `pass` addressed to a party that presented kept records, whose id is not among that party's delivered receipts. **Missing**: each envelope a party kept that no presented receipt echoes for that party (crossing-5), matched on both `seq` and `hash`, each number once, in ascending order. A release receipt counts as an echo for the question's asker only, never for the receiver it is addressed to, and a resolution receipt for its question's asker. A journal shortened after its last receipt still verifies; only reconciliation against a party's kept records shows the receipts that were cut.

#### Scenario: a removed last receipt

- Given a journal whose last receipt, a pass to the applier, was removed, and the applier's kept receipt of that delivery
- When the journal is reconciled with the applier's records
- Then it verifies, and the applier's kept receipt is listed as removed

#### Scenario: an unfetched delivery

- Given a journal holding a pass to the applier that the applier has not fetched
- When it is reconciled with the applier's records
- Then that receipt is listed as unrecorded for the applier

#### Scenario: a submission with no receipt

- Given a proposer that kept envelopes `seq` 1 and `seq` 2, and a journal that echoes only `seq` 1
- When they are reconciled
- Then `seq` 2 is listed as missing for the proposer, once

#### Scenario: a kept receipt from another Accord

- Given an applier whose kept receipt is a pass to it from another Accord under the same mediator
- When it is reconciled
- Then it fails with `kept`, naming the applier and the receipt's index

#### Scenario: an echo matches both seq and hash

- Given a kept envelope whose `seq` is echoed by a receipt with another `hash`
- When it is reconciled
- Then the envelope is listed as missing

## 10. Conformance vectors

A conformance vector is a file that states inputs and the exact results an implementation of this protocol must produce from them (`vector.json`). Vectors are what an implementation is checked against: a vector's codes, offsets and verdicts are derived by hand from this text, and its bytes, hashes and signatures are computed by a generator that does not use the code under test. A runner reads each vector, runs it against the implementation, and compares every expected result. It fails a vector in this order, at the first row that applies, and a vector it does not fail passes.

| # | Check | Reads | Result |
|---|---|---|---|
| 1 | Header | The file: canonical bytes with no repeated name; `vector`, `spec` and `tests` present and strings (conformance-vectors-11) | The vector fails before its group runs |
| 2 | Schema | The whole vector, against `vector.json`; no member the schema does not list, at any depth (conformance-vectors-9, conformance-vectors-10) | The vector fails |
| 3 | Inputs | Each byte input given exactly once, in one of its forms (conformance-vectors-8) | The vector fails |
| 4 | Operation | The operation, which the runner must be able to run in full (conformance-vectors-4) | The vector fails; no part of a vector is skipped |
| 5 | Results | Each expected result, in order, against what the implementation returned (conformance-vectors-5) | The vector fails at the first that differs |

### conformance-vectors-1 The header

Every vector has `vector`, its path under `vectors/` without the `.json` extension, a group and a name each in lowercase words joined by hyphens; `spec`, `0.1`; `tests`, one line saying what the vector tests; and `operation`, what it runs: `canonical`, `sign`, `verify-signature`, `found`, `scenario`, `verify` or `reconcile`.

#### Scenario: a vector's path

- Given the file `vectors/canonical/duplicate-key.json`
- When its header is read
- Then its `vector` is `canonical/duplicate-key` and its `spec` is `0.1`

### conformance-vectors-2 Inputs an implementation would choose

A vector gives every input an implementation could choose or read from the world: each key as a 32-byte Ed25519 seed in lowercase hex under a name in `keys`, every salt, every message id, and every time. An implementation run on a vector generates none of them, so two implementations produce the same bytes.

#### Scenario: an answer's salt

- Given a scenario vector with an accept
- When the runner runs it
- Then the answer's salt is the step's `salt`, not one the implementation minted

### conformance-vectors-3 Scenario vectors

A `scenario` vector founds one mediator from `founding` at `foundedAt`, run by the key named `mediator`, and applies its `steps` in order. A step is `give`, `get`, `resolve` or `tick`, and each names the time it runs at in `at`, but `get`, which reads no time. After the last step the runner compares `final`: `receipts`, every receipt the mediator wrote, in order, each with its id; `delivered`, for each party by name, the deliveries still queued for it, in order, a party with none left out; and `pending`, the ids of the questions still open, in the order they were opened. Every outcome a step expects names a receipt's `verdict` and `reason`, paired as receipt-4 says: a `give` step's is a crossing receipt's, never with a reason only a resolution receipt has; a `tick` step's are timeouts, `block` with reason `timeout`; and a `resolve` step's is its resolution receipt's, with its `release` composition's. The runner also verifies `final.receipts` under the mediator's reference (section 8), and a list that fails fails the vector. Steps after a rejected step still run.

#### Scenario: a give, then the end state

- Given a scenario vector whose one step is a give that passes
- When the runner runs it
- Then `final.receipts` holds that receipt, `final.delivered` holds its delivery under the receiver's name, and `final.pending` is empty

### conformance-vectors-4 Operations and groups

Each operation names what the runner calls and what it compares. `canonical` puts `inputText` or `inputHex` in canonical form and expects `canonicalHex` and `hash`, or `code` and `offset`. `sign` signs `input`, an object, as the key named `signer` under `typeUri`, and expects `signingBytesHex` and `sig`. `verify-signature` verifies the object given as `input`, `inputText` or `inputHex` under `typeUri` by `reference`, and expects `valid`. `found` founds the set given as `input`, `inputText` or `inputHex` at `foundedAt`, as the mediator the set names, and expects `accord` and `card`, or `code`. `verify` verifies `receipts` under `reference`, and expects `verified`, the number of receipts, or `code`, with `index` for every code but `reference`. `reconcile` reconciles `receipts` with `kept` under `reference`, `accord` and `card`, `parties` giving each party's name its reference, and expects `removed`, `unrecorded` and `missing`, or `code` with the index, and for `kept` the party. Vectors are filed in groups by the section of this text they test: `canonical`, `signatures`, `founding`, `crossing`, `receipts` and `questions`.

#### Scenario: a founding refusal

- Given a `found` vector whose set has a binding by an unlisted principal
- When the runner runs it
- Then the implementation returns `binding-principal`, the vector's `expect.code`

### conformance-vectors-5 How results compare

The runner compares exactly. A rejection is compared by its code. An outcome is compared by `verdict`, `reason`, `receiptId` and the receipt, byte for byte in its canonical form. A delivery is compared member by member, its body in the form it crossed. A canonical output is compared as bytes, a code with its offset. No result is compared by a part of its value, and a result the vector does not state is not taken as a pass.

#### Scenario: a receipt that differs in one byte

- Given a give step whose expected receipt differs from the one the implementation wrote in one byte of its signature
- When the runner compares them
- Then the vector fails at that step

### conformance-vectors-6 The resolve step

A `resolve` step has `question`, the id of the question's receipt, and `action`: `accepted`, `changed`, `declined`, `cancelled`, or `undeliverable`, the mediator's finding that nobody can be reached, which takes no caller. For `accepted` and `changed` it has `content.answer`, the answer's text or the version, which for `changed` is not empty, and `salt`, the salt the mediator mints. It may have `release`, `true`, for `accepted` and `changed` only; its `expect` then holds `release`, the release receipt's outcome, when one was written, and holds none when the step has no `release`. `by` names the member that answers; without it, the runner answers as the question's first answerer, by actor-reference order. `expect` is the outcome of the resolution receipt, or the rejection. After `accepted` or `changed` that outcome is composition's, and so is the release's; after `declined`, `cancelled` or `undeliverable` it is `block` with that action as its reason.

#### Scenario: an accepted answer with release

- Given a `resolve` step with `action` `accepted`, `release` `true`, and an answer that passes
- When the runner runs it
- Then the implementation's resolution outcome equals `expect`, and its release outcome equals `expect.release`

#### Scenario: a change with release

- Given a `resolve` step with `action` `changed`, `content.answer` holding the version, and `release` `true`
- When the runner runs it
- Then the implementation forms the version as maybe-9 says, under the step's `salt`, its resolution outcome to the asker equals `expect`, and its release outcome to the receiver equals `expect.release`

### conformance-vectors-7 The tick step

A `tick` step passes time to its `at` and expects `receipts`, the outcomes of the receipts written, in order, which is empty when no deadline was reached.

#### Scenario: a tick past a deadline

- Given a question with deadline `2026-11-02T09:00:01Z` and a `tick` step at `2026-11-02T09:00:05Z`
- When the runner runs it
- Then `expect.receipts` holds exactly one outcome, `block`, `timeout`, whose receipt's `at` is the deadline

### conformance-vectors-8 Bytes under test

An input whose exact bytes are under test is given in one of three forms: as a JSON value, whose bytes are its canonical form; as text, whose bytes are its UTF-8; or as hex. The top-level input is `input`, `inputText` or `inputHex`; a give's message is `message`, `messageText` or `messageHex`, and its envelope `envelope`, `envelopeText` or `envelopeHex`; a receipt in a list is an object holding `receipt`, `receiptText` or `receiptHex`. Exactly one form of each is given. A `canonical` vector takes only `inputText` or `inputHex`, and an `offset` is counted in those bytes. Empty hex is the empty input.

#### Scenario: an input only hex can hold

- Given a `canonical` vector whose input is the bytes `22 ff 22`
- When it is written
- Then the input is `inputHex` `22ff22`, since no JSON string holds the byte 0xFF

### conformance-vectors-9 A closed format

Every object of a vector is closed: a member `vector.json` does not list, at any depth, fails the vector, a member that differs from a listed one only in case included. The runner takes the list of top-level members from the schema itself, not from a list of its own.

#### Scenario: a case variant

- Given a vector with a top-level member `Vector` beside `vector`
- When the runner reads it
- Then the vector fails

### conformance-vectors-10 Every vector against the schema

Every vector under `vectors/` is validated against `vector.json` before it runs, and so are the schema's own examples: each accepted example must validate and each refused one must be refused. A run that finds no vector fails, with one exception, before the first vector is written: when the coverage list published with the vectors (`vectors/README.md`) names no vector, the run checks the schema against its examples and passes, with nothing else to run.

#### Scenario: a refused example that validates

- Given a refused example of `vector.json` that the schema accepts
- When the examples are checked
- Then the check fails, naming the example

#### Scenario: before the first vector

- Given a `vectors/` directory with no vector in it, and a coverage list that names none
- When the runner runs
- Then it checks `vector.json` against its examples and passes; had the list named a vector, the run would fail

### conformance-vectors-11 The header first

The runner reads a vector's header before anything else: a file whose bytes the canonical form refuses, a repeated top-level name included, or whose `vector`, `spec` or `tests` is missing or not a string, fails before its group runs, and so does a vector whose `vector` is not its own path.

#### Scenario: a repeated top-level name

- Given a vector file with `spec` twice
- When the runner reads it
- Then the vector fails before its group runs
