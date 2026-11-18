# The placket protocol, version 0.1

This document is the normative text of the placket protocol. It says what a mediator does at a handoff between parties that do not fully trust each other: how an Accord is founded from terms each party keeps sealed from the others and a view each signs, how a part is given, judged, held, asked about, answered, released and fetched, how an Accord is closed, what every receipt states and who it says decided, and how a third party who holds only the Accord's id, its founding set and its acceptances checks that the receipts are valid and that every decision was one the Accord let its decider make. `README.md` beside it says how to read `spec/`; the schemas under `schema/` and the register, `frozen-formats.md`, hold the formats this text defines.

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

The external standards this text uses are named once, by version: JSON (RFC 8259), JSON Pointer (RFC 6901), UTF-8 (RFC 3629), UTF-16 code units as Unicode 16.0 defines them, SHA-256 (FIPS 180-4), Ed25519 (RFC 8032), base64 (RFC 4648), timestamps (a profile of RFC 3339), DSSE 1.0 (Dead Simple Signing Envelope), the in-toto Statement v1, the A2A protocol 1.0 Message, of which the crossing message is a closed profile, and JSON Schema draft 2020-12 for the schemas.

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

An actor is named by its Ed25519 public key. A declaration, a binding, an envelope, an answer call and an acceptance each carry a signature by the actor that stands behind it, and a receipt carries the mediator's. Signing has no codes of its own: an operation that checks a signature returns its own code when any row below fails, and the table is the same for each of them.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Reference | The reference of the actor that must have signed: `ed25519:` and 64 lowercase hex (signing-1) | The calling operation's signature code |
| 2 | Key | The 32 bytes: a point of the curve that is not of small order (signing-1) | The calling operation's signature code |
| 3 | Signature form | The object's `sig`: present, a string of 128 lowercase hex | The calling operation's signature code |
| 4 | Equation | The signing bytes of signing-2, under the type URI the caller names, and the signature | The calling operation's signature code |

The calling operation's signature codes are `declaration-signature`, `binding-signature` and `acceptance-signature` in founding (setup-contract-8), `envelope-signature` at the door (crossing-6) and `answer-call` for an answer (section 6, maybe-11). A receipt's signature is checked by the verifier's `signature` row (receipt-6).

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

- Given a binding signed under `https://placket.tech/declaration/0.1` instead of `https://placket.tech/binding/0.1`
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

Placket signs objects of five kinds under five type URIs: `https://placket.tech/declaration/0.1`, `https://placket.tech/binding/0.1`, `https://placket.tech/envelope/0.1`, `https://placket.tech/answer/0.1`, the answer call's (maybe-11), and `https://placket.tech/acceptance/0.1`, a member's acceptance of its view (setup-contract-15). Each ends in the protocol number setup-contract-4 names. A signature over any other payload, such as a receipt's statement or a message of a door, is a DSSE signature over that payload under its own payload type, and its payload type is never one of the five, so no payload signature can be taken for a placket object's signature, or the reverse. A receipt is signed so, under the payload type `application/vnd.in-toto+json` (receipt-1).

#### Scenario: a placket URI as a payload type

- Given a payload signature whose payload type is `https://placket.tech/envelope/0.1`, and separately one whose payload type is `https://placket.tech/answer/0.1`
- When each is made or checked as a payload signature
- Then each is refused, and no envelope or answer call is signed or verified by that route

#### Scenario: an answer call signed and verified

- Given an answer call signed by its answerer under `https://placket.tech/answer/0.1`
- When it is verified under that type URI by its `from`, by another key, and under the envelope's type URI
- Then only the first verifies

#### Scenario: an acceptance signed and verified

- Given a member's acceptance of its view, signed by that member under `https://placket.tech/acceptance/0.1`
- When it is verified under that type URI by its `from`, by another key, and under the declaration's type URI
- Then only the first verifies

## 3. Founding an Accord

Founding reads a founding set, the openings its founder holds, the members' acceptances and the time it is founded at, and either founds an Accord or refuses with one code. The set holds each declaration as its seal, not in the clear (setup-contract-12); the openings are what open the seals (setup-contract-13), and the acceptances are each member's signed consent to its view of the merged Accord (setup-contract-14, setup-contract-15). The set itself names the mediator. Founding opens with the canonical form and a whole-shape check. After them, each seal entry is checked in actor-reference order (setup-contract-10), all its rows before the next entry's, then the rows that look across declarations, then the acceptances: the list whole, and then each member's in actor-reference order, all its rows before the next member's. The first failing row decides. A founder that holds no opening, as any party and any verifier, runs every row but 4a, 5 to 8 and 21 (setup-contract-16).

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Canonical form | The set's bytes (canonical-1) | `not-canonical` |
| 2 | Shape | The whole set but what row 4 reads: the top level; `policy`, `mediator` and `card` in full; `declarations` and `bindings` as non-empty arrays of objects; each seal entry's members, exactly `actor`, a string, and `seal`, a typed hash in the form `hash.json` reads (setup-contract-12); each binding in full, `until` by the timestamp rule included; the order of both lists (setup-contract-9, setup-contract-10) | `invalid-shape` |
| 3 | One binding per actor | The bindings' `actor` members | `binding-duplicate` |
| 4 | Actor | The seal entry's `actor`, as an actor reference (signing-1) | `actor-reference` |
| 4a | Opening | The opening the founder holds for the entry: one is held (`opening-missing`); its bytes are accepted by the canonical form and hold exactly `declaration` and `salt`, the salt 32 bytes in lowercase hex, the declaration an object of no member but `actor`, `terms` and `sig`, its `actor` present and a string (`invalid-shape`); the declaration's `actor` is the entry's, and the salted digest of its canonical bytes under the salt is the seal (setup-contract-13) | `opening-missing`, `seal-mismatch` |
| 5 | Declaration signature | The opened declaration's `sig`, and the declaration, under `https://placket.tech/declaration/0.1` by its `actor` (signing-2) | `declaration-signature` |
| 6 | Terms | The opened declaration's `terms` and each term's members, a `resolve` on a term that is not a `maybe` and the form of every `resolve` and of its `judges` included, but not whether a `maybe` term has one (setup-contract-1) | `term-invalid` |
| 7 | Maybe rule | Each term of stance `maybe`, for the presence of its `resolve` member | `maybe-without-resolve` |
| 8 | Judges | Each judge a term's `resolve.judges` names, against the names of the card's `judges` | `judge-unknown` |
| 9 | Binding present | A binding whose `actor` is the declaration's | `binding-missing` |
| 10 | Principal | That binding's `principal`, against the `principal` of each entry of the policy's `principals` | `binding-principal` |
| 11 | Binding signature | That binding's `sig`, under `https://placket.tech/binding/0.1` by its `principal` (signing-2) | `binding-signature` |
| 12 | Binding roles | Each role of that binding's `roles`, against the `roles` of its principal's entry | `binding-role` |
| 13 | Binding policy | That binding's `policy`, against the typed hash of the set's `policy` (canonical-3) | `binding-policy` |
| 14 | Binding mediator | That binding's `mediator`, against the set's `mediator.actor` | `binding-mediator` |
| 15 | Binding time | That binding's `until`, against the founding time: for the window, the time it holds the last acceptance; for a verifier, the first receipt's `at` (verify-1) | `binding-expired` |
| 16 | Declaration present | Each binding's `actor`, for a declaration by that actor | `declaration-missing` |
| 17 | Roles held | Each role of the policy's `roles`, for a declaring actor whose binding holds it | `role-missing` |
| 18 | Acceptance present | First, once, the acceptance list whole: bytes the canonical form refuses are `not-canonical`; a list of objects each of exactly `accord`, `view`, `from` and `sig`, `accord` and `view` typed hashes and `from` a string, in strictly ascending actor-reference order of `from`, each `from` a member, or it is `invalid-shape`. Then, for each member, an acceptance whose `from` is the member (setup-contract-15) | `acceptance-missing` |
| 19 | Acceptance signature | That acceptance's `sig`, and the acceptance, under `https://placket.tech/acceptance/0.1` by the member (signing-2) | `acceptance-signature` |
| 20 | Acceptance Accord | That acceptance's `accord`, against the Accord id (setup-contract-5) | `acceptance-accord` |
| 21 | Acceptance view | That acceptance's `view`, against the typed hash of the member's view built from the openings (setup-contract-14); run only by a founder that holds every opening | `acceptance-view` |

When every row passes, the Accord is founded: its id is the typed hash of the set (setup-contract-5). A founder that ran every row with every opening checked the terms; one that held some or none says that the terms were not checked (setup-contract-16).

### setup-contract-1 Declarations

A declaration is an actor's own terms, signed by that actor (`declaration.json`, `term.json`). Its `actor` is the actor's reference, its `terms` a non-empty list of terms, and its `sig` the actor's signature under `https://placket.tech/declaration/0.1`. A term has a `stance` (`need`, `want`, `maybe` or `no`), a `dir` (`give` or `get`), `with`, a non-empty list of distinct role selectors `role:<label>` naming roles the other side must hold, and `select`, an object whose one member `kind` is a non-empty list of distinct kinds. A label and a kind are lowercase letters and digits in words joined by single hyphens, at most 64 characters. A term of stance `maybe` has a `resolve` member: `to`, a non-empty list of distinct role selectors naming who answers, `timeout`, a whole number of seconds from 1 to 2^53 − 1, and optionally `judges`, the judges the term grants, in order: a non-empty list of objects each with exactly a judge's `name`, a label the card's `judges` names, and its `use`, `decide` or `advise`, no two of one name (composition-7). A judge the card does not name is `judge-unknown`. A term of any other stance has no `resolve`. A term holds no other member: a term with a member missing, of the wrong type, of the wrong form or not listed, such as `checks`, is `term-invalid`, and so is a `terms` member that is missing, empty or not a list. Every party of an Accord signs its own terms: founding refuses a declaration signed by any key but its actor's. A founding set holds a declaration only as its seal, and these rules read the declaration its opening holds (setup-contract-12, setup-contract-13).

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
- Then it is refused with `term-invalid`, since a term lists no member `checks`

#### Scenario: a judge the card does not name

- Given a set whose card's `judges` is empty and whose declaration has a maybe term whose `resolve.judges` names `jev` to decide, correctly signed
- When it is founded
- Then it is refused with `judge-unknown`

#### Scenario: a judge named twice

- Given a set whose declaration has a maybe term whose `resolve.judges` names `jev` twice, once to decide and once to advise
- When it is founded
- Then it is refused with `term-invalid`

#### Scenario: a declaration edited after signing

- Given a set whose declaration's terms were changed after it was signed, and then sealed
- When it is founded with its openings
- Then it is refused with `declaration-signature`

### setup-contract-2 Bindings and roles

An actor holds a role only by a binding: a principal's signed statement that the actor holds a list of roles under one mediator and one policy until a time (`binding.json`). A binding has the `actor`, the `principal` that signs it, `roles`, a non-empty list of distinct labels, `policy`, the typed hash of the canonical bytes of the founding set's `policy` object it was signed for (canonical-3), the `mediator`'s reference, `until`, a timestamp, and `sig`, the principal's signature under `https://placket.tech/binding/0.1`. Roles come from nowhere else: not from a declaration, a policy or a message. The policy says which roles each principal may grant; a binding gives an actor only roles its principal's entry lists. The principal that signs an actor's binding is that party's principal, and one principal may stand behind several parties of an Accord, or all of them. The policy does not list actors: founding checks that a binding's principal has an entry, may grant the roles the binding gives and signed for this policy, and never which actor a principal may bind. Who stands behind each actor is read from the founding set (commands-4, `verify --founding`). A binding's `until` must be later than the founding time; a binding is checked against the clock at founding only. One binding may name several roles, and one actor may then hold them all.

#### Scenario: two principals

- Given a set whose policy has two entries, one principal that may grant `proposer` and `applier` and another that may grant `decider`, and whose every binding is signed by its own party's principal
- When it is founded
- Then the Accord founds, each actor holds the roles its binding gives, and each actor's principal is the one that signed its binding

#### Scenario: one principal behind every party

- Given a set whose policy has one entry that may grant every role, and whose every binding that principal signs
- When it is founded
- Then the Accord founds

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

For each declaration, founding finds the binding whose `actor` is the declaration's, and checks it in this order: one exists (`binding-missing`); its `principal` is the `principal` of one of the policy's entries (`binding-principal`); its signature verifies under that principal (`binding-signature`); every role it gives is in the `roles` of its principal's entry (`binding-role`); its `policy` is the typed hash of the set's `policy` (`binding-policy`); its `mediator` is the set's `mediator.actor` (`binding-mediator`); its `until` is later than the founding time (`binding-expired`). So a binding counts only under the policy its principal signed for, and only for the roles that policy lets its principal grant. Before any declaration, two bindings for one actor are `binding-duplicate`. After every declaration, a binding for an actor with no declaration is `declaration-missing`.

#### Scenario: a binding by an unlisted principal

- Given a set whose binding is signed by a key the policy does not list, and names that key as its principal
- When it is founded
- Then it is refused with `binding-principal`

#### Scenario: a binding that gives a role its principal may not grant

- Given a set whose policy entry for a principal lists `proposer` alone, and whose binding by that principal gives an actor `decider`, correctly signed
- When it is founded
- Then it is refused with `binding-role`

#### Scenario: a binding signed for another policy

- Given a set whose binding names the typed hash of a policy other than the set's, correctly signed, and nothing else wrong
- When it is founded
- Then it is refused with `binding-policy`

#### Scenario: a binding for another mediator

- Given a set whose binding names a mediator other than the set's `mediator.actor`
- When it is founded
- Then it is refused with `binding-mediator`

#### Scenario: a binding with no declaration

- Given a set with two declarations and three bindings, the third for an actor with no declaration
- When it is founded
- Then it is refused with `declaration-missing`

### setup-contract-4 The card

The card says what can change a verdict under the Accord (`card.json`). Its `spec` is `0.1`, the version of this protocol and the one protocol number every placket URI carries: each type URI (signing-4), the receipt's predicate type (receipt-1) and the crossing extension's URI (crossing-1) end in it, and each schema's `$id` holds it as the segment after `schema/`, as in `https://placket.tech/schema/0.1/card.json`. This version knows one value, `0.1`, and a card whose `spec` is any other is `invalid-shape`. A later version changes every URI and the `spec` together. Its `kernel` names the decision module by a `version` and a `hash`, which the operator writes and founding reads for shape only. Its `judges` is the ordered list of the judges a term may grant, each entry a `name`, a label, and the `version` and `hash` that pin the judge's build, its configuration included; no two entries share a name, so one name with two versions or hashes is `invalid-shape`. Running a judge is not founding's: founding reads the list for shape. The card holds no other member. Its typed hash is the card hash, which every receipt carries (receipt-3), so every receipt names the module that judged it.

#### Scenario: a card that loads a check

- Given a set whose card has a member `checks` listing one check
- When it is founded
- Then it is refused with `invalid-shape`, since the card lists no member `checks`

#### Scenario: a judge the card names twice

- Given a set whose card's `judges` lists `jev` twice, under two hashes
- When it is founded
- Then it is refused with `invalid-shape`

#### Scenario: a member the card does not list

- Given a set whose card has a member `models`
- When it is founded
- Then it is refused with `invalid-shape`

#### Scenario: a card of another spec

- Given a set whose card's `spec` is `0.2`
- When it is founded
- Then it is refused with `invalid-shape`

### setup-contract-5 The Accord id

The Accord id is the typed hash of the founding set's canonical bytes; the card hash is the typed hash of the card's. The same set founds to the same id at any time, and any change to the set, a term or a role included, founds to another: a term changes its declaration's seal (setup-contract-12), so the id commits every term though the set holds none in the clear. The Accord is founded only once every member has accepted its view (setup-contract-15). Every role the policy's `roles` names must be held, through a binding, by an actor that has a declaration in the set, or the set is refused with `role-missing`. A role held but not in the policy's `roles` is allowed, when the binding's principal may grant it, and the policy's `roles` may be empty. A role in the policy's `roles` that no entry lists can never be held, so its set is refused with `role-missing`.

#### Scenario: the same set twice

- Given one founding set, founded at two different times
- When the two Accord ids are compared
- Then they are equal

#### Scenario: a role held that the policy does not require

- Given a set whose policy entry lets a principal grant `proposer` and `auditor`, whose binding by it gives an actor both, and whose policy's `roles` is `["proposer"]`
- When it is founded
- Then the Accord founds

#### Scenario: a policy role nobody holds

- Given a set whose policy names the role `auditor`, which no binding gives
- When it is founded
- Then it is refused with `role-missing`

### setup-contract-6 Membership

The members of an Accord are the actors with a declaration in its founding set, each held as its seal entry (setup-contract-12). The mediator's key is a member only when it has a declaration there. Every operation on an Accord that has a caller, a give, a fetch and an answer, authenticates its caller by key and refuses a caller that is not a member with `not-a-member`, before it reads any other input. A refused operation changes nothing.

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

Founding runs the rows of the table in order and returns the first that fails: the canonical form, the shape, one binding per actor; then for each declaration in actor-reference order its actor, its opening, its signature, its terms, its maybe rules, its judges, and its binding's presence, principal, signature, roles, policy, mediator and time; then a binding with no declaration; then a policy role nobody holds; then the acceptance list whole, and for each member in actor-reference order its acceptance's presence, signature, Accord and view. A refused set founds nothing and yields no id.

#### Scenario: two defects in one declaration

- Given a set whose first declaration has a bad signature and whose binding has expired
- When it is founded
- Then it is refused with `declaration-signature`

#### Scenario: defects in two declarations

- Given a set whose first declaration's binding has expired and whose second declaration has a malformed term
- When it is founded
- Then it is refused with `binding-expired`, since the first declaration's rows all run first

#### Scenario: defects in two members' acceptances

- Given a set that founds but for its acceptances: the first member's, in actor-reference order, names a view other than its own, and the second member's is signed by another key
- When it is founded with every opening
- Then it is refused with `acceptance-view`, since the first member's rows all run first

#### Scenario: a policy role nobody holds and acceptances that are not canonical

- Given a set whose policy names a role no binding gives, and acceptance bytes the canonical form refuses
- When it is founded with every opening
- Then it is refused with `role-missing`, since the list is read after row 17

#### Scenario: a role its principal may not grant and another mediator

- Given a set whose first declaration's binding gives a role its principal's entry does not list and names another mediator, correctly signed
- When it is founded
- Then it is refused with `binding-role`

#### Scenario: a role its principal may not grant and another policy

- Given a set whose binding gives a role its principal's entry does not list and names the typed hash of another policy, correctly signed
- When it is founded
- Then it is refused with `binding-role`

### setup-contract-9 The shape of a set

A founding set is an object with exactly `policy`, `mediator`, `card`, `declarations`, a list of seal entries (setup-contract-12), and `bindings` (`founding.json`). `policy` has exactly `roles`, a list of distinct labels, and `principals`, a non-empty list of entries, and may have `supersedes` (setup-contract-11). Each entry is an object with exactly `principal`, an actor reference, and `roles`, a non-empty list of distinct labels: the roles that principal may grant. No two entries name the same principal; two entries may list the same role. A binding's `policy` is a typed hash. `mediator` is as setup-contract-7 says and `card` as setup-contract-4 says. `declarations` and `bindings` are non-empty lists. Bytes the canonical form refuses are `not-canonical`, before the shape is read, and no id is computed for them. Every defect of shape is `invalid-shape`, except in the members a later row reads: a seal entry's `actor` form belongs to row 4, and an opened declaration's `actor` form, its `sig` and its `terms` belong to rows 4a to 8.

#### Scenario: a repeated member name

- Given the bytes `{"policy":{},"policy":{}}` as a founding set
- When they are founded
- Then they are refused with `not-canonical`

#### Scenario: an actor of the wrong type

- Given a set whose seal entry's `actor` is the number 7, and separately one whose `actor` is the string `alice`
- When each is founded
- Then the first is refused with `invalid-shape`, since the order of the list reads `actor` as a string, and the second with `actor-reference`

#### Scenario: a policy entry or a binding of the wrong shape

- Given sets, each with nothing else wrong, with: an empty `principals`; an entry whose `roles` is empty; an entry whose `roles` repeats a label; an entry with a role that is not a label; an entry whose `principal` is not an actor reference; two entries for one principal; an entry with a member other than `principal` and `roles`; a binding with no `policy`; and a binding whose `policy` is not a typed hash
- When each is founded
- Then each is refused with `invalid-shape`

#### Scenario: a term with a missing member

- Given a set whose opened declaration has a term with no `dir`, correctly signed
- When it is founded with its openings
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

### setup-contract-11 The Accord a policy supersedes

A policy may say that its Accord supersedes an earlier one: `supersedes`, an object with exactly `accord`, the earlier Accord's id, and `close`, the id of that Accord's close receipt, both typed hashes. It is signed through each binding's `policy`, the typed hash of the whole policy, so every principal signs it. Founding reads it for shape only: a `supersedes` of another shape is `invalid-shape`. What it means for the earlier Accord is not this version's to say.

#### Scenario: a policy that supersedes an earlier Accord

- Given a set whose policy has `supersedes` naming an earlier Accord's id and its close receipt's id, every binding signed for that policy
- When it is founded
- Then the Accord founds and names both

#### Scenario: a close that is not a typed hash

- Given a set whose policy's `supersedes` has a `close` that is not a typed hash, the bindings signed for that policy
- When it is founded
- Then it is refused with `invalid-shape`

### setup-contract-12 The seal

A founding set holds no declaration in the clear. Each entry of its `declarations` is a seal entry: an object of exactly `actor`, the declaring actor's reference, and `seal`, `sha256:` followed by the 64 lowercase hex digits of the SHA-256 of a salt of exactly 32 bytes and then the declaration's canonical bytes, its `sig` included. That is crossing-3's salted digest, written in the form `hash.json` reads. The actor mints the salt. The policy, the mediator, the card and the bindings stay in the clear. The Accord id is the typed hash of the set, so it commits every term through its seal, and a declaration keeps its own signature, so an opened seal still proves the terms are its actor's. Every declaration of every Accord is sealed, an Accord of one party included.

#### Scenario: a sealed set

- Given the ask-and-log set, each declaration sealed, with every opening and every member's acceptance
- When it is founded
- Then the Accord founds, and its id is the typed hash of the sealed set

#### Scenario: a seal entry of the wrong shape

- Given a set whose seal entry also holds `terms`, and separately one whose seal is 64 hex digits with no `sha256:`
- When each is founded
- Then each is refused with `invalid-shape`

### setup-contract-13 The opening

The opening of a seal is an object of exactly `declaration`, the declaration as its actor signed it, and `salt`, its 32 bytes in lowercase hex (`opening.json`). Its actor keeps it in a file of its own, named for the 64 hex digits of its reference followed by `.opening.json` and readable by its owner alone, and hands it only to whoever runs the window, which needs every term to decide. Row 4a reads the opening the founder holds for an entry: none held is `opening-missing`; bytes the canonical form refuses, a member besides the two, a salt of another length or form, or a declaration that is not an object, holds a member but `actor`, `terms` and `sig`, or has an `actor` missing or not a string, are `invalid-shape`; a declaration whose `actor` is not the entry's, or whose salted digest under the salt is not the seal, is `seal-mismatch`. Rows 5 to 8 then read the opened declaration as setup-contract-1 says. An actor may hand over its own opening, as in a dispute, and anyone may then check it against the Accord id (verify-1).

#### Scenario: an opening that does not open its seal

- Given the ask-and-log set and its openings, where the proposer's opening holds its declaration with one byte changed, or another salt, or the applier's declaration
- When each is founded
- Then each is refused with `seal-mismatch`

#### Scenario: no opening for a member

- Given the ask-and-log set and every opening but the proposer's
- When it is founded by a founder that holds the openings
- Then it is refused with `opening-missing`

#### Scenario: an opening of the wrong shape

- Given openings with a member besides `declaration` and `salt`; with a salt of 31 bytes; whose declaration holds a member besides `actor`, `terms` and `sig`; whose declaration's `actor` is the number 7; and whose bytes repeat a member name
- When each is founded
- Then each is refused with `invalid-shape`

#### Scenario: a maybe without its rule inside an opening

- Given a sealed set whose proposer's opening opens its seal and holds a maybe term with no `resolve`
- When it is founded
- Then it is refused with `maybe-without-resolve`, since rows 5 to 8 read the opened declaration

### setup-contract-14 The view

Each member is shown its view of the merged Accord before it is founded (`view.json`): what will cross for it, built from its own terms only. A view has `accord`, the Accord id; `viewer`, the member's reference; `salt`, the member's own opening salt; `crossings`; and `answers`.

- **Crossings.** For each term of the viewer, each kind its `select` names, and each member whose binding gives a role one of the term's `with` selectors names, the view has one row, each crossing once: `counterpart`, that member's reference, and `roles`, its roles as its binding gives them; `dir`, the term's direction from the viewer; `kind`; and `outcome`, composition's decision of that crossing, given by a member with no judgment (section 5). A pass is `crosses`. A block is `blocked`, with no reason: a row never says whether the other side refused or said nothing, and maybe rules that differ (composition-5) are `blocked`. An ask is `asked`, with the agreed rule's `to`, its role selectors in sorted order, its `timeout`, and `judges`, the judges both sides consented to (composition-7), left out when there are none. A kind only the other side names has no row.
- **Answers.** Every crossing between two other members that composition asks about and whose agreed rule's `to` names a role of the viewer has one row under `answers`: its `asker`, `receiver`, `kind` and `timeout`, each crossing once.

Crossings are sorted by counterpart in actor-reference order, then by `dir` and by `kind` in byte order; answers by asker and receiver in actor-reference order, then by `kind`. So the same openings give the same bytes. The salt keeps the view's typed hash, which every verifier holds in the acceptances, from being matched against views guessed from the founding set. A view holds no byte of another party's declaration, salt or opening, and no row for a kind its viewer's terms do not select. It still shows something of the other side: an `asked` row shows the agreed rule, which may be the other side's alone; a `crosses` row shows that the other side has a matching term; and an answerer's `answers` show the asker, receiver, kind and timeout of every crossing it answers, before and whether or not it is ever asked. A view is built only by a founder that holds every opening, since its outcomes read the counterparts' terms. It is hashed, never signed by the mediator: the member's acceptance is what binds it.

#### Scenario: a kind only the other side names

- Given A, which refuses to give secrets and drafts to B and gives it notes, and B, which gets notes and drafts and has no term for secrets
- When the views are built for B and for A
- Then B's view has no row for secrets, and its draft row is `blocked`; A's has both rows `blocked` beside notes that cross

#### Scenario: blocked carries no reason

- Given the same B, once where A refuses drafts and once where A says nothing of them
- When B's views are built
- Then B's draft rows are the same bytes

#### Scenario: maybe rules that differ

- Given A and B, both `maybe` on memos with one rule, and both `maybe` on plans with timeouts 60 and 120, answered by C
- When the views of A, B and C are built
- Then memos are `asked` with `to` and `timeout`, plans are `blocked`, and C answers the memos only

#### Scenario: asked with the judges consented to

- Given both sides `maybe` on reports with one rule, both listing `jev` to decide, and on reviews with one rule, only A listing `jev`
- When A's view is built
- Then reports are `asked` with `to`, `timeout` and `jev` to decide, and reviews `asked` with no judges

#### Scenario: the same openings twice

- Given one set and its openings
- When the view of one member is built twice
- Then the two views are the same bytes with the same typed hash

#### Scenario: an answerer's view

- Given A and C, each asking the other about contracts under a rule whose `to` names `approver`, which D holds, D a side of no crossing of contracts
- When D's view and A's are built
- Then D's `answers` list the A to C and C to A contracts with their kind and timeout, and no term of A's or C's; and A's view shows its contracts `asked` with `to` naming `approver`

#### Scenario: a view needs every opening

- Given a two-party set and one member's opening alone
- When a view is built
- Then it is refused with `opening-missing`

### setup-contract-15 The acceptance and the founding record

A member consents to the Accord by signing an acceptance of its view (`acceptance.json`): an object of exactly `accord`, the Accord id; `view`, the typed hash of the member's view (setup-contract-14); `from`, the member's reference; and `sig`, its signature under `https://placket.tech/acceptance/0.1` (signing-4). The founding record is the founding set and `acceptances.json`, the list of every member's acceptance in strictly ascending actor-reference order of `from`, kept beside the set. The Accord is founded only when every member has accepted, so consent is given to the outcome while the terms behind it stay sealed. Rows 18 to 21 read the list: whole first, then each member's acceptance, all its rows before the next member's. The same set and acceptances found the same Accord. For the window, the founding time is when it holds the last acceptance. There is no deadline and no decline: a member that does not consent does not accept, and a set never fully accepted leaves no record and no receipt.

#### Scenario: an acceptance refused

- Given the ask-and-log set and every opening, with acceptances where one member's is missing; one is signed by another key; one's `sig` is not 128 lowercase hex; one names another Accord; and one names a view other than its member's
- When each is founded
- Then they are refused with `acceptance-missing`, `acceptance-signature`, `acceptance-signature`, `acceptance-accord` and `acceptance-view`

#### Scenario: an acceptance list of the wrong shape

- Given acceptance lists out of `from` order; with one member's acceptance twice; with an acceptance from a key that is no member; with an acceptance holding a member besides the four; and with a `view` that is not a typed hash
- When each is founded
- Then each is refused with `invalid-shape`

#### Scenario: acceptance bytes that are not canonical

- Given acceptance bytes that repeat a member name, and separately no acceptance list at all
- When each is founded
- Then each is refused with `not-canonical`

### setup-contract-16 Founding without the openings

A founder that holds no opening, as every party but the window's operator, and every verifier, runs every row but 4a, 5 to 8 and 21: it checks the set's clear parts, the bindings and the acceptances' signatures and Accord, and says that the terms were not checked. A founder that holds some openings, as a verifier given a dispute's openings, runs rows 4a to 8 for the members they open, and row 21 only when it holds every member's. The window's operator holds every opening and runs every row.

#### Scenario: no opening

- Given the sealed ask-and-log set and its acceptances, and no opening
- When it is founded
- Then the Accord founds to the same id, and the result says the terms were not checked

#### Scenario: an acceptance of another view, unopened

- Given the same set with an acceptance whose `view` is not its member's, signed by it, and no opening
- When it is founded
- Then the Accord founds, the terms not checked, since row 21 is not run

#### Scenario: the clear rows still run

- Given, with no opening, a set whose binding expires at the founding time, and separately an acceptance signed by another key
- When each is founded
- Then they are refused with `binding-expired` and `acceptance-signature`

## 4. Giving and getting a part at the door

The door is where a member gives a part to another member and fetches what was queued for it. A give carries two byte strings, the message and the envelope, from an authenticated caller at a time. The door authenticates before it reads the message: it checks that the caller is a member, then the envelope's signature under the caller's key, then the message's canonical bytes, then that the envelope names those bytes and a fresh sequence number, and only then the message's content. A give that fails any row is rejected: it writes nothing and changes nothing (crossing-7). A give that passes every row is decided by composition (section 5), and the mediator writes its receipt.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Member | The caller's key, against the Accord's members (setup-contract-6) | `not-a-member` |
| 1a | Open Accord | The Accord: not closed (close-3) | `closed` |
| 2 | Envelope signature | The envelope bytes: accepted by the canonical form, an object with exactly `seq`, `hash` and `sig`, its `sig` verifying under the caller's key and `https://placket.tech/envelope/0.1` (signing-2) | `envelope-signature` |
| 3 | Canonical message | The message bytes (canonical-1) | `not-canonical` |
| 4 | Envelope hash | The envelope's `hash`: the typed hash of the message's canonical bytes (crossing-4) | `envelope-hash` |
| 5 | Sequence | The envelope's `seq`: a whole number from 1 to 2^53 − 1, above the caller's highest accepted `seq` in this Accord (crossing-4) | `envelope-sequence` |
| 6 | Crossing message | The message's frame: an object; no member but `messageId`, `role`, `parts`, `extensions` and `metadata`; `messageId` a non-empty string; `role` `ROLE_USER`; `extensions` exactly the crossing extension's URI; `metadata` holding exactly the extension's object under it; and that object's `re`, `topic` and `gate`, and no member it does not list. `to`, `accord` and `parts`, missing included, belong to rows 7, 8 and 9 (crossing-1) | `extension-missing` |
| 7 | Receiver | The extension's `to`: present, an actor reference of a member (crossing-1) | `receiver-unknown` |
| 8 | Accord | The extension's `accord`: present, this Accord's id | `accord-mismatch` |
| 9 | One part | The message's `parts`: present, a list of exactly one element | `one-part` |
| 10 | One body | The part: an object with exactly one of the body members `text`, `raw`, `data` and `url`, a `text` that is a string, a `raw` in standard base64, a `data` that is not `null`, a `mediaType` of a media type's form when it has one, and no member but that body, `metadata` and `mediaType` (crossing-2) | `one-body` |
| 11 | Body form | A `url` body, whatever its value (crossing-2) | `url-unsupported` |
| 12 | Part metadata | The part's `metadata`: exactly the crossing extension's object, with a `kind` and a `salt` of exactly 32 bytes (crossing-2) | `part-metadata` |

A fetch carries only the authenticated caller.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Member | The caller's key (setup-contract-6) | `not-a-member` |

A member's fetch returns the first delivery queued for it, once, or nothing when none is queued (crossing-3); once the Accord is closed, a fetch that finds nothing queued returns the close receipt (close-3). A fetch writes no receipt.

### crossing-1 The crossing message

A part crosses in an A2A protocol 1.0 message that carries the crossing extension, `https://placket.tech/crossing/0.1` (`message.json`). The frame is a closed profile of the A2A 1.0 Message: it keeps A2A's names and meanings, fixes which members a message has and the values some of them take, and allows nothing else. The mediator stands on the agent's side of A2A's roles, and each member on the client's. The message has exactly the members `messageId`, a non-empty string; `role`, `ROLE_USER` for a part a member gives, as an A2A client sends, and `ROLE_AGENT` for one the mediator forms from an answer (maybe-8, maybe-9), as an A2A agent answers; `parts`, the list of its one part; `extensions`, the list holding exactly the crossing extension's URI; and `metadata`, holding exactly one member, under that URI, the crossing extension's object. That object has `accord`, the Accord's id; `to`, the receiver's reference; and it may have `re`, the id of the receipt the message answers; `topic`, a non-empty string that groups crossings; and `gate`, reserved for a later version and read for shape only: lowercase letters and digits in words joined by single hyphens or underscores, at most 64 characters, such as `out_you`. The sender is not a member of the message: it is the caller, whose key signed the envelope (crossing-6). The extension has no `from`, so a give whose extension carries one is `extension-missing`, as for any member the frame does not list, whatever its value; no member of a message can name a sender other than the caller. The door accepts only `ROLE_USER` from a member: a give with `ROLE_AGENT` is `extension-missing`, as for any other defect of the frame. No check reads `topic` or `gate` but for their shape, no check reads `role` but for its value, and no receipt or delivery carries `topic` or `gate`.

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

#### Scenario: a give in the agent's role

- Given a member's correctly signed give whose `role` is `ROLE_AGENT`, and separately the same give with `ROLE_USER`
- When the door checks each
- Then the first is rejected with `extension-missing`, and the second is decided

#### Scenario: a reserved gate

- Given a member's correctly signed give whose extension has `gate` `out_you`, and separately one with `gate` `Out_You`
- When the door checks each
- Then the first is decided as if it had no `gate`, and the second is rejected with `extension-missing`

### crossing-2 The part

The one part has exactly one body, the crossing extension's metadata, and optionally `mediaType`, the body's media type (`part.json`). The body is `text`, a string; `raw`, bytes in standard base64; or `data`, any JSON value but `null`. A part with no body, with two, or with any member but its body and `metadata` is `one-body`, and so is a `raw` that is not standard base64 in its one encoding. A `url` body is not a body of this version: a part whose one body is `url` is `url-unsupported`, whatever the value. The part's `metadata` holds exactly one member, under the crossing extension's URI, with exactly `kind`, a kind as setup-contract-1 defines it, and `salt`, exactly 32 bytes in lowercase hex, 64 digits; any other is `part-metadata`. A `mediaType` is a type and a subtype joined by one slash, in lowercase letters, digits and the characters `!#$&^_.+-`, each from 1 to 127 characters and starting with a letter or a digit, with no parameters, such as `application/json`; any other value is `one-body`. No term reads it and no digest covers it; the part's delivery carries it. The body's bytes are the UTF-8 of `text`, the decoded bytes of `raw`, or the canonical bytes of `data`.

#### Scenario: two bodies

- Given a correctly signed give whose part has both `text` and `data`
- When the door checks it
- Then it is rejected with `one-body`

#### Scenario: a url body

- Given a correctly signed give whose part's one body is `url`
- When the door checks it
- Then it is rejected with `url-unsupported`

#### Scenario: a salt of 31 or 33 bytes

- Given a correctly signed give whose part's salt is 31 bytes, and separately one whose salt is 33 bytes
- When the door checks each
- Then each is rejected with `part-metadata`

#### Scenario: a part's media type

- Given a correctly signed give whose part has `mediaType` `application/json`, and separately ones whose `mediaType` is `Application/JSON`, `json`, a type with a parameter, or a number
- When the door checks each
- Then the first is decided and its delivery carries `application/json`, and the others are rejected with `one-body`

### crossing-3 Digests, salts and deliveries

A part is named in a receipt by its kind and its salted digest: the lowercase hex SHA-256 of the salt's bytes followed by the body's bytes. A salt is exactly 32 bytes, so a digest opens as one salt and one body only. The first holder of a part mints its salt, 32 random bytes: the giver of a part it gives, and the answerer of an answer or a version (maybe-8, maybe-9). The mediator mints no salt. No receipt carries a body or a salt. When a part passes, the mediator queues for its receiver a delivery (`delivery.json`): the receipt that passed it and its id, the part's `messageId`, `kind` and `salt`, the message's `re` when it has one, the part's `mediaType` when it had one, the answer call as `answer` when an answer queued it (maybe-12), and the body, as `text`, `raw` or `data` as it crossed. A delivery holds nothing else. Its format is frozen because a receiver checks the receipt and the digest with it and every scenario vector compares it member by member (conformance-vectors-5). A door hands a delivery out in a reply of its own format. Deliveries are queued per receiver in the order of their receipts, and each is handed out once, to its receiver only, by a fetch. The mediator keeps no body or salt of a part that was blocked, or of a delivery once fetched.

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

The envelope is the sender's signed claim to one submission (`envelope.json`): `seq`, its sequence number in this Accord, `hash`, the typed hash of the message's canonical bytes, and `sig`, the sender's signature under `https://placket.tech/envelope/0.1`. A message edited after its envelope was signed is `envelope-hash`. A `seq` at or below the highest the caller has had accepted in this Accord, or not a whole number from 1 to 2^53 − 1, is `envelope-sequence`, so a replayed envelope is refused. A `seq` is accepted when its give writes a receipt; a rejected give consumes none. A `seq` may skip numbers: it need only be above the last accepted.

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

The door runs the rows of its table in order and returns the first that fails. It authenticates first: membership, then whether the Accord is closed (close-3), then the envelope's signature under the caller's key, then the message's canonical bytes, then that the envelope's `hash` names those bytes and its `seq` is fresh, then the message's content. No member of the message is read before the message is known to be the one the caller signed for. The caller is the sender because the envelope verified under the caller's key: no member of the message names the sender, and a `from` in the extension is a member the frame does not list, `extension-missing` at row 6 (crossing-1). The message is not read at all before the envelope's signature has verified, so a stranger's give is `not-a-member` and a member's give under a bad envelope signature is `envelope-signature`, whatever the message holds. Everything decided is read from the message's canonical bytes, so a member name repeated anywhere in the message is `not-canonical`, at row 3, whichever copy is wrong. Row 2 reads the envelope's bytes, its members' names and its `sig`, so bytes the canonical form refuses, a member missing or not listed, and a bad signature are all `envelope-signature`; the value of its `hash` belongs to row 4 and of its `seq` to row 5.

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

Composition decides a crossing that the door, or an answer (section 6), hands it: a sender, a receiver, the part's kind, whether the part was given by a member or formed by the mediator from an answer, and a judge's judgment of it, or none (composition-7). It reads the two parties' terms and the roles their bindings give them, and returns a verdict and a reason, and with them who decided: the terms, or the judge (receipt-11). It refuses nothing: every crossing that reaches it gets a receipt. It runs no judge: a judgment is given to it, a judge's card name and its verdict, pass or block, settled before the crossing is decided. A term matches when it is the sender's of `dir` `give` or the receiver's of `dir` `get`, its `select.kind` holds the part's kind, and one of its `with` selectors names a role the other party holds (composition-1). The rows run in order, and the first that applies decides.

| # | Check | Reads | Verdict and reason |
|---|---|---|---|
| 1 | A refusal | Every matching term, for stance `no` (composition-3) | `block`, `no-match` |
| 2 | Both sides | The matching terms: at least one of the sender's and at least one of the receiver's (composition-2) | `block`, `no-match` |
| 3 | Formed part | A matching term of stance `maybe`, when the part was formed by the mediator (composition-5) | `block`, `maybe` |
| 4 | Maybe rules | Every matching term of stance `maybe`: the set of roles its `resolve.to` names, and its `timeout`, not its `judges` (composition-5) | `block`, `maybe`, when two differ |
| 5 | A judge | The judgment given, against the judges both sides consented to, with use `decide` (composition-7) | `pass`, `ok`, or `block`, `judge:<name>`, by the judge |
| 6 | Ask | A matching term of stance `maybe`, all rules agreeing (composition-5, maybe-1) | `escalate`, `maybe` |
| 7 | Pass | Everything else | `pass`, `ok` |

Every verdict but row 5's is the terms' decision.

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

When any matching term, of either party, has stance `no`, the crossing is `block` with reason `no-match`, before any maybe is read and before any judgment.

#### Scenario: no beside want

- Given a receiver with a matching `want` term and a matching `no` term for the kind
- When composition decides the crossing
- Then it is `block`, `no-match`

### composition-5 Maybe

When a matching term has stance `maybe` and no earlier row applied, the crossing is asked about, under the rule the maybe terms agree on. Every matching `maybe` term must name the same set of roles in `resolve.to`, in any order, and the same `timeout`; the judges they list are not compared here. Then, unless a judge decides it (composition-7), the verdict is `escalate` with reason `maybe` and the mediator holds the part as a question (maybe-1). When two of them differ, the crossing is `block` with reason `maybe` and no question is opened: no value a party chooses when it gives can pick which rule applies. A part the mediator formed from an answer that meets a `maybe` term is `block` with reason `maybe`, and no second question is opened: a formed part is never judged.

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

#### Scenario: maybe rules whose judges differ

- Given a sender's and a receiver's maybe terms that agree on their roles and timeout, one listing `jev` to decide and the other `jev` to advise, or only one of them listing any
- When composition decides the crossing with `jev`'s pass
- Then it is `escalate`, `maybe`, by the terms, and no judge is consulted

### composition-6 Need

A term of stance `need` composes exactly as one of stance `want`.

#### Scenario: need meets want

- Given a sender's `need` give term and a receiver's `want` get term that both match
- When composition decides the crossing
- Then it is `pass`, `ok`, as for two `want` terms

### composition-7 A judge decides where the terms would ask

A judge acts only where the terms would ask, and only where both sides consented to it. The judges consented to are those of the agreed rule: they are listed only when every matching term, of both parties, has stance `maybe`, and every one of them lists the same `judges`, the same names in the same order with the same uses. Otherwise no judge is consented to: a `want` or `need` term beside a `maybe` one, a `maybe` term on one side alone, and two `maybe` terms whose judges differ all leave the part to be asked. When the crossing's judgment names a judge consented to with use `decide`, its `pass` decides `pass`, `ok`, and its `block` decides `block`, `judge:` followed by the judge's card name, and the receipt names that judge as its decider (receipt-11). Any other judgment is none: a judge consented to only to advise, a judge not consented to, and a verdict other than pass or block. A part the mediator formed is never judged (composition-5), and neither is a release (maybe-10). Which judge's judgment is given, when several are listed, is settled before the crossing is decided: this text says only what a judgment decides.

#### Scenario: a judge both sides consented to

- Given a sender's and a receiver's maybe terms for a report that both list `jev` with use `decide`
- When composition decides a report with `jev`'s pass, and then with its block, and then with no judgment
- Then the first is `pass`, `ok`, by the judge `jev`; the second `block`, `judge:jev`, by `jev`; and the third `escalate`, `maybe`, by the terms

#### Scenario: a judge only one side consented to

- Given a sender's maybe term that lists `jev` to decide and a receiver's maybe term that lists no judge, and separately both listing `jev` to advise, and separately the sender's matching `want` term beside its maybe term
- When composition decides each with `jev`'s pass
- Then each is `escalate`, `maybe`, by the terms

## 6. Questions, the three answers and release

A give that composition decides `escalate` opens a question: the mediator writes the ask's receipt, holds the part, and queues nothing for the receiver (maybe-1). The question's id is the id of that receipt. A question closes exactly once: by an answer, an accept, a change or a decline from a member holding a role it names, or a cancel by its asker; by the mediator finding that nobody can be reached to answer it; by its deadline; or by the close of its Accord (close-1). An answer is one signed call, the answer call (maybe-11), which names the question, the action, and for an accept or a change the salted digest of the answer's text under the answerer's salt and whether it asks for release (maybe-10); for an accept or a change the answer's text and that salt are given beside the call. It runs these rows in order.

| # | Check | Reads | Code |
|---|---|---|---|
| 1 | Member | The caller's key (setup-contract-6) | `not-a-member` |
| 1a | Open Accord | The Accord: not closed (close-3) | `closed` |
| 2 | Answer call | The call: canonical bytes, its closed shape (maybe-11), its `accord` this Accord's id, its `from` the caller, and its `sig` under `from` and `https://placket.tech/answer/0.1` (signing-2) | `answer-call` |
| 3 | Open question | The call's `question`: a question that is open, at a time before its deadline (maybe-7) | `no-open-question` |
| 4 | Answerer | The call's `from`, by its action: for an accept, a change or a decline, among the question's answerers (maybe-6); for a cancel, the question's asker, of a question with at least one answerer. An asker who is also an answerer cancels as the asker | `not-an-answerer` |
| 5 | Answer body | For an accept or a change, a salt of exactly 32 bytes and a text, not empty for a change, whose salted digest (crossing-3) is the call's `digest`; for a decline or a cancel, no text and no salt | `answer-body` |

A rejected answer writes nothing and leaves the question as it was. An answer that passes the rows closes the question and writes its receipts, all of them before any state changes: the resolution receipt first, then the release receipt when one is written. The mediator's own finding that a question cannot be delivered has no caller and no call, and reads only row 3. Its deadline is reached by the passing of time, which reads nothing and refuses nothing (maybe-5). A question no member can answer is cancelled by nobody: it ends by the mediator's finding or by its deadline (maybe-4).

The answerers of a question are the members whose bindings give them a role its rule names in `resolve.to`, in actor-reference order. The list may be empty: founding checks that the policy's roles are held, not that a rule's are.

| Closing | Receipt from, to | Subject | Verdict, reason | Queued |
|---|---|---|---|---|
| Accept (maybe-3) | The answerer, to the asker | The answer, kind `answer` | As composition decides the answer | The answer, for the asker, on a pass |
| Change (maybe-9) | The answerer, to the asker | The version, kind `changed-version` | As composition decides the version | The version, for the asker, on a pass |
| Decline, cancel (maybe-4) | The first answerer, to the asker, whoever signed the call | The asked part | `block`, `declined` or `cancelled` | Nothing |
| Undeliverable (maybe-4) | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `undeliverable` | Nothing |
| Timeout (maybe-5) | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `timeout` | Nothing |
| Closed (close-1) | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `closed` | Nothing |

Every one of these is a resolution receipt that echoes the question's envelope (crossing-5). An accept, a change, a decline and a cancel name the answer call as their decider, and an undeliverable, a timeout and a close's closing the mediator (receipt-11). An accept or a change that passes, and whose call asks for release, then writes a release receipt (maybe-10).

### maybe-1 Asking

A give decided `escalate` writes a crossing receipt with verdict `escalate` and reason `maybe`, from the giver to the receiver, and holds a question under that receipt's id: the asker, the receiver, the part as it was given, its message's `messageId` and `re`, the envelope, the agreed rule, and the deadline. The deadline is the give's time plus the rule's `timeout` in seconds; a deadline past `9999-12-31T23:59:59Z` is that time. Nothing is queued for the receiver. The held part leaves the mediator only by a release (maybe-10). While the question is open, the part is also shown to the question's answerers, and to no one else, so that an answer is about this exact content: its body, in the form it was given, raw bytes included, its salt, and the ask's receipt, so that an answerer checks that the body under the salt is the part the receipt names before it signs an answer. Showing it queues nothing and writes nothing.

#### Scenario: a proposal is asked about

- Given the ask-and-log Accord, whose proposer gives `proposal` to the applier under a maybe term with a timeout of 3600
- When the proposer gives a proposal at `2026-11-12T09:00:00Z`
- Then the receipt is `escalate`, `maybe`, a question is open with deadline `2026-11-12T10:00:00Z`, and the applier's fetch returns nothing

#### Scenario: what an answerer is shown

- Given an open question of a text part, one of a raw part and one of a data part
- When an answerer is shown them, and a member who is not an answerer
- Then the answerer is shown, for each, the ask's receipt, the part's salt and its body, whose salted digest is the receipt's subject digest, and the other member is shown none

#### Scenario: a deadline past the last time

- Given a maybe term with `timeout` 9007199254740991
- When a part is asked about under it
- Then the question's deadline is `9999-12-31T23:59:59Z`

### maybe-3 An accepted answer

An accept forms the answer as maybe-8 says and decides it through composition from the answerer to the asker, as a part formed by the mediator. The resolution receipt's verdict and reason are composition's, its subject is the answer, and it echoes the question's envelope. A pass queues the answer for the asker. A block queues nothing, for anyone: the held part is dropped. The resolution receipt names the answer call as its decider, whatever composition decided: its reason shows what the terms did.

#### Scenario: an accept that passes

- Given an open question in the ask-and-log Accord
- When the decider accepts it with the text `accepted`
- Then one resolution receipt is written from the decider to the proposer, `pass`, `ok`, of kind `answer`, echoing the question's envelope, and the answer is queued for the proposer

#### Scenario: an accept the asker's terms do not take

- Given an open question whose asker has no term to get `answer` from the answerer's role
- When an answerer accepts it
- Then the resolution receipt is `block`, `no-match`, nothing is queued, and the question is closed

### maybe-4 Decline, cancel and undeliverable

A decline and a cancel each close the question with a resolution receipt from the question's first answerer, by actor-reference order, to the asker: `block` with reason `declined` or `cancelled`, naming the asked part by its kind and digest, echoing the question's envelope and naming the answer call as its decider. The receipt names the first answerer whichever answerer declined, and when the asker cancelled; only the call names who signed it. When the mediator finds that nobody can be reached to answer a question, it closes it the same way with reason `undeliverable`, naming itself, with the event `undeliverable`, as its decider; when the question has no answerer, the receipt is from the mediator's own reference. An undeliverable, a timeout and a closing a close wrote, with reason `closed` (close-1), are the only resolution receipts that can be from the mediator. Nothing is queued, and the held part is dropped.

#### Scenario: a decline

- Given an open question in the ask-and-log Accord
- When the decider declines it
- Then one receipt is written from the decider to the proposer, `block`, `declined`, of kind `proposal` with the asked part's digest, and nothing is queued for the applier

#### Scenario: a question nobody can answer

- Given a question whose rule names a role no member holds
- When the mediator closes it as undeliverable
- Then the receipt is from the mediator's reference to the asker, `block`, `undeliverable`, by the mediator

#### Scenario: the asker cancels

- Given an open question in the ask-and-log Accord
- When the proposer, its asker, cancels it with a call it signed
- Then one receipt is written from the decider, the first answerer, to the proposer, `block`, `cancelled`, naming the asked part and the call as its decider, and nothing is queued

### maybe-5 Timeout

As time passes the mediator closes, in the order they were opened, every open question whose deadline is at or before the current time, each with a resolution receipt from the first answerer, or the mediator when there is none, to the asker: `block` with reason `timeout`, naming the asked part, echoing the question's envelope, naming the mediator, with the event `deadline`, as its decider, and with `at` the deadline, not the time it was noticed. A passing of time that reaches no deadline writes nothing.

#### Scenario: one receipt at the deadline

- Given a question with deadline `2026-11-02T09:00:01Z`
- When time is passed to `2026-11-02T09:00:05Z`
- Then exactly one receipt is written, `block`, `timeout`, with `at` `2026-11-02T09:00:01Z`

#### Scenario: before the deadline

- Given a question with deadline `2026-11-02T09:00:01Z`
- When time is passed to `2026-11-02T09:00:00Z`
- Then nothing is written and the question stays open

### maybe-6 Only an answerer answers

An accept, a change or a decline from a member that is not among the question's answerers is rejected with `not-an-answerer`, writes nothing, and leaves the question open. The asker may answer its own question when its binding gives it a role the rule names. A cancel is the asker's alone: a cancel signed by any other member, an answerer included, is `not-an-answerer`, and so is the asker's cancel of a question that has no answerer, which ends by the mediator's finding or its deadline (maybe-4).

#### Scenario: the proposer answers its own proposal

- Given an open question in the ask-and-log Accord, where only the decider holds `decider`
- When the proposer accepts it
- Then it is rejected with `not-an-answerer`, nothing is written, and the question stays open

#### Scenario: an answerer cancels

- Given an open question in the ask-and-log Accord
- When the decider, its answerer, cancels it
- Then it is rejected with `not-an-answerer`, nothing is written, and the question stays open

#### Scenario: a cancel of a question nobody can answer

- Given an open question whose rule names a role no member holds
- When its asker cancels it
- Then it is rejected with `not-an-answerer`, nothing is written, and the question stays open

### maybe-7 A closed question

An answer, or a finding that a question cannot be delivered, for an id that names no open question, or at a time at or after the question's deadline, is rejected with `no-open-question` and writes nothing. A question closes in the operation that writes its resolution receipt, and its release is written in that same operation, never by a later one. Row 3 comes before row 4, so a member who holds no answering role learns only that no question is open for it. The answer call has no time and no nonce: a call sent again finds its question closed, and is `no-open-question`.

#### Scenario: a second answer

- Given a question the decider has accepted
- When the decider declines it
- Then it is rejected with `no-open-question` and nothing is written

#### Scenario: an answer at the deadline

- Given an open question with deadline `2026-11-12T10:00:00Z`
- When the decider accepts it at `2026-11-12T10:00:00Z`, before the passing of time has closed it
- Then it is rejected with `no-open-question`, and the next passing of time writes the timeout

### maybe-8 The answer's message

An accepted answer is a part the mediator forms: `messageId` `answer:` followed by the question's id, kind `answer`, `re` the question's id, a `text` body holding the answer's text, and the salt the answerer minted for it, 32 bytes, so its salted digest is the call's `digest`. Its message's `role` is `ROLE_AGENT`, since the mediator forms it. The answer's text may be any string.

#### Scenario: the delivered answer

- Given an accept of question `sha256:` followed by `ab` repeated 32 times, with text `accepted`, that passed
- When the asker fetches
- Then the delivery's `messageId` is `answer:sha256:` followed by `ab` repeated 32 times, its kind `answer`, its `re` the question's id, its `text` `accepted`, its `salt` the one the answerer minted, and its `answer` the answer call

### maybe-9 A changed answer

A change is an answerer's own version of the asked part, as a non-empty text. The mediator forms one part from it: `messageId` `change:` followed by the question's id, kind `changed-version`, `re` the question's id, a `text` body holding the version, and the one salt the answerer minted, 32 bytes, with `role` `ROLE_AGENT`. It decides that part from the answerer to the asker as it decides an accepted answer, and the resolution receipt's subject is the version, of kind `changed-version`: that is how a change is recorded. A pass queues the version for the asker. A change with an empty version is `answer-body`: it writes nothing and leaves the question open. The asked part is never released after a change.

#### Scenario: a change returned to the asker

- Given an open question in the ask-and-log Accord
- When the decider changes it with the text `Rename total to amount_due.`, without release
- Then one resolution receipt is written from the decider to the proposer, `pass`, `ok`, of kind `changed-version`, and the version is queued for the proposer with `re` the question's id

#### Scenario: an empty change

- Given an open question
- When an answerer changes it with an empty text
- Then it is rejected with `answer-body`, nothing is written and the question stays open

### maybe-10 Release

An answer's call may ask for release. When it does and its resolution receipt's verdict is `pass`, the mediator decides the part for the question's receiver in the same operation, and writes a release receipt that echoes the question's envelope and names the same answer call as its decider, after the resolution receipt and before any state changes. No judge is consulted at a release.

- After an accept, the held part is decided again, as given and with no judgment, from the asker to the receiver. When composition returns `block`, the release receipt is a crossing receipt from the asker to the receiver, `block` with composition's reason, naming the asked part, and nothing is queued for the receiver. On any other verdict the answer stands, and it is the answer that releases: the release receipt is a crossing receipt from the asker to the receiver, `pass`, `ok`, with the asked part's kind and digest unchanged, and the held part is queued for the receiver exactly as it was given, with the answer call inside its delivery (maybe-12). The mediator adds no verdict of its own. The terms are fixed, so a part a maybe term asked about is decided `escalate` again, and the release passes.
- After a change, the same version, under the same salt and so with the same digest, is decided from the answerer to the receiver as a part formed by the mediator. The release receipt is a resolution receipt from the answerer to the receiver, of kind `changed-version`, with composition's verdict and reason. A pass queues the version for the receiver. When the receiver is the answerer or the asker, a change releases nothing.
- A decline, a cancel, an undeliverable, a timeout, an answer whose resolution receipt is `block`, and an answer whose call does not ask for release write no release receipt and queue nothing for the receiver. The held part is dropped.

A release receipt consumes no sequence number of any sender. A question has at most one.

#### Scenario: an accept releases the asked part with its digest unchanged

- Given an open question in the ask-and-log Accord, asked with a proposal of digest D
- When the decider accepts it, asking for release
- Then two receipts are written in one operation: the resolution from the decider to the proposer, `pass`, `ok`, of kind `answer`; then a crossing receipt from the proposer to the applier, `pass`, `ok`, of kind `proposal` with digest D, echoing the question's envelope; both name the answer call as their decider; and the applier's fetch returns the proposal as it was given, with the call inside its delivery

#### Scenario: a change returned to the asker and released to the receiver under one digest

- Given an open question in the ask-and-log Accord
- When the decider changes it, asking for release
- Then two resolution receipts are written, from the decider to the proposer and from the decider to the applier, both `pass`, `ok`, of kind `changed-version` with one digest, both echoing the question's envelope; the version is queued for both; and the proposal is never queued for the applier

#### Scenario: an accept's release is decided again and passes

- Given an open question in the ask-and-log Accord, opened by the proposer's `maybe` term for a proposal to the applier
- When the decider accepts it, asking for release, and the proposal is decided again from the proposer to the applier
- Then that decision is `escalate` once more, which is not a block, so the release receipt is `pass`, `ok`, the proposal is queued for the applier, and the answer stays queued for the proposer

#### Scenario: a judge never acts at release

- Given an open question of a report whose two sides' maybe terms list `jev` to decide
- When the decider accepts it, asking for release
- Then the release is decided with no judgment, `pass`, `ok`, by the answer call, never by `jev`

#### Scenario: a release receipt echoes the question's envelope and counts as an echo for the asker only

- Given a change released to the applier, whose release receipt is addressed to the applier and echoes the proposer's envelope
- When the applier's kept envelopes are reconciled
- Then the release receipt counts as an echo of the proposer's envelope only, and of no envelope of the applier's

#### Scenario: a decline releases nothing

- Given an open question in the ask-and-log Accord
- When the decider declines it
- Then one receipt is written, no release receipt follows, and nothing is queued for the applier

### maybe-11 The answer call

An answer is one signed call, the answer call (`answer.json`): a closed object with exactly `accord`, the Accord's id; `question`, the question's id, the id of its ask's receipt; `action`, `accepted`, `changed`, `declined` or `cancelled`; for `accepted` and `changed` exactly, `digest`, the salted digest (crossing-3) of the answer's text, or the version, under the answerer's salt, and `release`, a boolean, whether it asks for release; `from`, the signer's actor reference; and `sig`, its signature under `https://placket.tech/answer/0.1`, by signing-2. It has no time and no nonce. The answerer mints the answer's salt, exactly 32 bytes, and signs the salted digest, never the text: the text and the salt are given beside the call, never inside it. Its typed hash (canonical-3), over its canonical bytes with `sig`, names it in the receipts its answer writes (receipt-11). The mediator checks the call, by section 6's rows, before it writes anything.

#### Scenario: a call signed by another key

- Given an open question and the decider's accept, whose call names the decider as `from` but was signed by the proposer's key
- When the decider sends it
- Then it is rejected with `answer-call`, nothing is written, and the question stays open

#### Scenario: a text the digest does not name

- Given the decider's accept, its call signed over the text `accepted` under its salt, given with the text `accepted.`, or with another salt
- When the decider sends it
- Then it is rejected with `answer-body`, nothing is written, and the question stays open; the same call with its own text and salt then closes the question

### maybe-12 The call travels with the answer

The receipts an answer writes name its call: the resolution receipt and the release receipt both have as their decider the call's typed hash (receipt-11). The call travels inside every delivery the answer queues, as the delivery's `answer`: the answer or the version queued for the asker, and the released part or version queued for the receiver. No receipt holds the call itself. A party that fetched one holds the call, and with the answer's text and salt it shows who answered and what; with the receipts alone, the call's hash shows which call it was.

#### Scenario: the call inside both deliveries

- Given an accept asking for release that passes
- When the proposer and the applier fetch
- Then each delivery holds the call as `answer`, and both receipts name the call's typed hash as their decider

## 7. Receipts and their chains

A receipt is the mediator's signed statement of one decision. Writing one refuses nothing: the mediator writes a receipt for every give the door passes and for every closing and release of a question, and one close receipt when the Accord is closed (close-2), and for nothing else. This table says which receipt each event writes; every receipt is chained in the pair of its `from` and `to` (receipt-5).

| Event | `kind` | `from`, `to` | Subject | `verdict`, `reason` | Echoed envelope | `by` |
|---|---|---|---|---|---|---|
| A give the door passes | `crossing` | The giver, the receiver | The given part | Composition's (section 5) | The give's | The terms, or the judge that decided |
| An accept or a change | `resolution` | The answerer, the asker | The answer or the version | Composition's | The question's | The answer call |
| A decline, a cancel | `resolution` | The first answerer, the asker | The asked part | `block`, `declined` or `cancelled` | The question's | The answer call |
| An undeliverable | `resolution` | The first answerer or the mediator, the asker | The asked part | `block`, `undeliverable` | The question's | The mediator, `undeliverable` |
| A timeout | `resolution` | The first answerer or the mediator, the asker | The asked part | `block`, `timeout` | The question's | The mediator, `deadline` |
| A close's closing | `resolution` | The first answerer or the mediator, the asker | The asked part | `block`, `closed` | The question's | The mediator, `close` |
| A release after an accept | `crossing` | The asker, the receiver | The asked part | `pass`, `ok`, or composition's block | The question's | The answer call |
| A release after a change | `resolution` | The answerer, the receiver | The version | Composition's | The question's | The answer call |

### receipt-1 The receipt

A receipt is a DSSE 1.0 envelope (`receipt.json`) with exactly `payloadType`, `application/vnd.in-toto+json`; `payload`, the standard base64 of the canonical bytes of the receipt's statement; and `signatures`, a list of exactly one entry with exactly `keyid`, the mediator's reference, and `sig`, the standard base64 of the mediator's Ed25519 signature over the DSSE pre-authentication encoding of the payload type and the payload's bytes (signing-2). The statement is an in-toto Statement v1 (`statement.json`): `_type` `https://in-toto.io/Statement/v1`, `subject`, `predicateType` `https://placket.tech/receipt/0.1`, and `predicate`. A receipt's id is the typed hash of the canonical bytes of its statement. The mediator writes the payload in canonical form, so putting it in canonical form again changes nothing. A close receipt is an envelope of this form whose statement is the close's own (close-2); its id is taken the same way.

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

The predicate (`predicate.json`) has exactly these members: `accord`, the Accord's id; `card`, the card hash; `mediator`, the mediator's reference; `kind`, `crossing` or `resolution`; `from` and `to`, the references of the receipt's two parties; `seq` and `prev`, its place in its chain (receipt-5); `at`, its time (receipt-10); `envelope`, the echoed envelope: `sender`, the reference of the key that signed it, and its `seq` and `hash` (crossing-5); `verdict`; `reason`; and `by`, who decided (receipt-11). In a crossing receipt the envelope's `sender` is the receipt's `from`: a give's receipt echoes the giver's own envelope, and a release after an accept the asker's, which is the question's. A member a receipt cannot fill is absent, never `null`: `prev` is absent exactly when `seq` is 1, and no other member is ever absent.

#### Scenario: the first receipt of a pair

- Given the first receipt the mediator writes from one party to another
- When its predicate is read
- Then its `seq` is 1 and it has no `prev`

### receipt-4 Verdicts and reasons

A crossing receipt's `verdict` is `pass`, `block` or `escalate`; a resolution receipt's is `pass` or `block`. The reasons are a closed list of nine: `ok`, `no-match` and `maybe`, on any receipt; `judge:<name>`, on a crossing `block` only, where `<name>` is a judge's card name (composition-7); and `declined`, `cancelled`, `timeout`, `undeliverable` and `closed`, on resolution receipts only. A `pass` has reason `ok`, an `escalate` has reason `maybe`, and a `block` has any other reason, `maybe` included: a crossing whose maybe rules differ is a block with reason `maybe` (composition-5). A reason that opens `check:` is no reason of this version.

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

#### Scenario: a judge's block

- Given a crossing predicate with `verdict` `block`, `reason` `judge:jev` and `by` the judge `jev`, and separately one with `reason` `check:forbidden`
- When each is validated against `predicate.json`
- Then the first is accepted and the second refused

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

- Given a predicate with a member `result`, and separately one with no `by`
- When each is validated against `predicate.json`
- Then each is refused

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

### receipt-11 Who decided

Every predicate names its decider in `by`, in exactly one of these forms, and holds no probability, score or threshold:

- `"terms"`: the terms decided a give's crossing receipt, a `pass` `ok`, a `block` `no-match` or `maybe`, or an `escalate` `maybe` (section 5).
- `{"judge":"<card name>"}`: a judge's judgment decided a give's crossing receipt, a `pass` `ok`, or a `block` `judge:<name>` with the same name (composition-7).
- `{"answer":"<typed hash>"}`: an answer call decided the resolution receipt of an accept, a change, a decline or a cancel, and its release receipt; the value is the typed hash of the call's canonical bytes, `sig` included (maybe-11, maybe-12). An accept or a change that composition then blocks is still the answer's: its reason shows what the terms did.
- `{"mediator":"deadline"}`: the resolution receipt of a timeout (maybe-5); `{"mediator":"undeliverable"}`: that of an undeliverable (maybe-4); and `{"mediator":"close"}`: that of a closing a close wrote, with reason `closed` (close-1). These three are the mediator's only values.

A `by` must fit its receipt's kind, verdict and reason, as this table says; the verifier's statement row refuses any other (receipt-6). It cannot tell a release from another crossing receipt, so it takes an answer's `by` on any crossing `pass` or `block` the table allows, and the release row refuses one that is not a question's release, or whose `by` is not its closing's.

| Kind | Verdict | Reason | `by` allowed |
|---|---|---|---|
| crossing | `pass` | `ok` | the terms, a judge, an answer |
| crossing | `block` | `no-match`, `maybe` | the terms, an answer |
| crossing | `block` | `judge:<name>` | the judge of that name |
| crossing | `escalate` | `maybe` | the terms |
| resolution | `pass` | `ok` | an answer |
| resolution | `block` | `no-match`, `maybe`, `declined`, `cancelled` | an answer |
| resolution | `block` | `timeout` | the mediator, `deadline` |
| resolution | `block` | `undeliverable` | the mediator, `undeliverable` |
| resolution | `block` | `closed` | the mediator, `close` |

#### Scenario: every event names its decider

- Given a give the terms pass, one a judge passes, one asked about, its accept with its release, a decline, an undeliverable and a timeout
- When their receipts are read
- Then each names its decider as the table says: the terms, the judge, the terms, the answer call twice, the call, and the mediator with `undeliverable` and `deadline`

#### Scenario: a by that does not fit

- Given a correctly signed timeout whose `by` is the terms, and a correctly signed `block` `judge:jev` whose `by` names the judge `other`
- When each is verified
- Then each fails `statement-type` at its index

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

A verifier needs only the mediator's reference and a list of receipts in the order the mediator wrote them, such as its journal; or the Accord's id, its founding set and its acceptances, from which it takes the mediator's reference (verify-1). It checks the reference once, then each receipt in list order, every row of one receipt before the next receipt, and stops at the first failure, which it reports with its code and the receipt's index, counted from 0. An empty list verifies. A list that verifies yields its statements, and its close when it holds one. A close receipt (close-2) goes through rows 1 to 5, its statement read against the close's form at row 4 and its `mediator` at row 5, and through rows 8 and 9; it is in no chain and belongs to no question, so rows 6 and 7 do not read it.

| # | Check | Reads | Code |
|---|---|---|---|
| 0 | Mediator | The reference the verifier is given, before any receipt (signing-1) | `reference` |
| 1 | Bytes | The receipt's bytes, and the bytes `payload` decodes to when it is a string of standard base64 in its one encoding (canonical-1) | `not-canonical` |
| 2 | Payload type | `payloadType`: present, exactly `application/vnd.in-toto+json` | `payload-type` |
| 3 | Signature | `payload` as standard base64 in its one encoding; `signatures`, exactly one entry with exactly `keyid`, the given mediator's reference, and `sig`, standard base64 of 64 bytes; the signature, over the pre-authentication encoding of the payload type and the payload's bytes, under the given mediator's key; and no member of the receipt but these three | `signature` |
| 4 | Statement | The statement: exactly `_type`, `subject`, `predicateType` and `predicate`, with the values receipt-1 and receipt-2 give; the predicate's members as receipt-3, receipt-4, receipt-9, receipt-10 and receipt-11 give, but `mediator`, `seq` and `prev`, its `by` fitting its kind, verdict and reason; and in a crossing receipt, the envelope's `sender` equal to its `from` (receipt-3) | `statement-type` |
| 5 | Mediator named | The predicate's `mediator`: the given mediator's reference | `mediator` |
| 6 | Chain | The predicate's `seq` and `prev`, against the receipt before it in the list from the same `from` to the same `to` (receipt-5) | `chain` |
| 7 | Release | The receipt's place among the receipts before it that echo the same envelope, and a release's `by` against its closing's (receipt-6) | `release` |
| 8 | Close | A close's `chains` and `unfetched` against the receipts before it, and the place of each closing with reason `closed` (close-4) | `close` |
| 9 | After the close | No receipt after a close (close-4) | `after-close` |

### receipt-6 The verifier

A verifier checks receipts with the mediator's key alone and performs no key lookup: the mediator's reference comes from the verifier's caller, or from the founding set whose typed hash is the Accord id its caller gives (verify-1), never from a receipt. A receipt edited after signing, or signed again by another key, fails `signature`. A receipt signed by the right key that names another mediator fails `mediator`. In each pair the first receipt in the list must have `seq` 1 and no `prev`, and each later one `seq` one above and `prev` the id of the one before; a receipt removed from the middle of a chain therefore fails `chain` at the receipt after it. A list cut short after any receipt, the journal's last receipts removed, still verifies (receipt-7). Row 7 reads questions from receipts, by this rule, which every reader of a journal applies and none restates:

1. A crossing receipt with verdict `escalate` is a question, and its echoed envelope, `sender`, `seq` and `hash`, is its key. It always opens a question of its own and belongs to no question before it, even one with the same key, as after a shortened journal is resumed.
2. A later receipt that is not a question and echoes the same envelope belongs to the latest question before it with that key. Since row 4 makes a question's `sender` its `from`, every receipt that belongs to a question names the question's `from` as `sender`.
3. A question's closing receipt is the first receipt that belongs to it. It must be a resolution receipt addressed to the question's `from`. When its reason is `declined`, `cancelled`, `timeout`, `undeliverable` or `closed`, its subject is the question's, name and digest. Otherwise its subject `name` is `answer` or `changed-version`, and when its verdict is `pass` it is the question's answer: an accept when the name is `answer`, a change when it is `changed-version`.
4. After an answer, the receipt right after the closing receipt in the list may belong to the question: it is the release receipt, and its `by` is its closing's. After an accept it is a crossing receipt from the question's `from` to its `to`, with the question's subject, verdict `pass` or `block`. After a change it is a resolution receipt from the answer's `from` to the question's `to`, with the answer's subject, and the question's `to` is neither its `from` nor the answer's `from`.
5. Every other receipt that belongs to a question fails `release`: one before the closing receipt that is not a resolution addressed to the asker, a closing of the wrong subject, a second release, a release that is not right after its answer, one after a closing that is not an answer, one of the wrong form, one whose `by` is not its closing's. So do a resolution receipt that belongs to no question and a crossing receipt an answer decided that belongs to none.

A verifier given a subset of the receipts, not the whole list, reads them by this rule, and rows 8 and 9 as close-4 says, with two differences: a chain may start at any `seq`, the first receipt of a pair with a `prev` of its form, and each chain that starts above 1 is named; and a receipt that would belong to a question not in the subset is outside it, never a failure of `release`.

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

#### Scenario: a release of another answer

- Given an ask, its accept and a release whose `by` names another answer call than the accept's
- When the three are verified
- Then they fail with `release` at index 2

#### Scenario: a subset that starts late

- Given the last two receipts of a chain of three
- When they are verified as a subset
- Then they verify, and the chain is named as starting at `seq` 2; verified as a whole list they fail with `chain` at index 0

### receipt-8 Reading without verifying

A receipt can be decoded and shown without being verified: its statement read from its payload, its words printed. Whatever does so says the receipt was not verified, because a forged receipt decodes and shows exactly as a real one does. Anything that presents a receipt as the mediator's decision verifies it first.

#### Scenario: a forged receipt shown

- Given a receipt whose signature is not the mediator's
- When it is decoded and shown without verifying
- Then it shows like any other receipt, and the showing says it was not verified

### verify-1 What a verifier is given

A verifier that checks conformance is given the Accord's id out of band, and nothing else: the founding set and its acceptances, the receipts, and optionally the openings of some or every member, answer calls, a subset of the receipts rather than the whole list, and one delivery. It founds the set, with the openings it holds, at the first receipt's time (setup-contract-16), and takes the mediator's reference from the set whose typed hash is the Accord id, its `mediator.actor`; a set of another typed hash, one that does not found, or one whose mediator is not the reference the verifier was also given, is `founding`, and nothing else is checked. It checks the receipts' validity, section 8's first table, under that reference, and then their conformance, the table below. It never runs a judge again, never fetches anything, and never takes the mediator's reference from a receipt.

Conformance runs only with a founding set whose typed hash is the Accord id, receipt by receipt in list order after validity passes, every row of one receipt before the next; the first failure decides, with its code and the receipt's index. Row 3 runs only when the card's kernel hash is one the verifier trusts to decide again, the module it runs itself; with any other, row 3 is skipped and rows 4 and 5 still run. Row 3 also reads terms, so for each receipt it runs only when the verifier holds the opening of every member whose terms the decision reads: the receipt's `from` and `to`, and for a question's closing or release the question's asker and receiver; otherwise it is skipped for that receipt, and row 4 reads a decline's signer as a member, as when the module is not trusted. So a verifier with no opening checks what was signed: the receipts' validity, the founding set's clear parts and its acceptances, each receipt's Accord and parties, and each answer call against the bindings; and it says that the terms were not checked. With some openings it checks the decisions whose sides they open, and with every opening it checks every decision. Row 5 reads only a delivery and its receipt, and runs with or without a founding set.

| # | Check | Reads | Code |
|---|---|---|---|
| 0 | Founding | The set: with its acceptances and the openings the verifier holds, it founds at the first receipt's `at` (setup-contract-16), its typed hash is the Accord id, and the receipts are verified under its `mediator.actor` | `founding` |
| 1 | Accord | Each receipt's `accord` and `card`, a close's included, against the set's | `accord` |
| 2 | Parties | Each receipt's `from` and `to`, members of the set, or a `from` that is the mediator where maybe-4, maybe-5 or close-1 let it be | `party` |
| 3 | Decision | The receipt decided again, by composition, with the judgment its `by` and verdict claim (composition-7): it gives the receipt's verdict and reason, and a judge it names is one consented to with use `decide`. An accept's or a change's `from` is an answerer of the question's rule, and its `at` before the deadline; a decline or a cancel is from the first answerer, before the deadline; a timeout's `at` is the deadline; an undeliverable, and a closing with reason `closed`, is from the first answerer, or the mediator when there is none, before the deadline; and a release follows maybe-10 | `decision` |
| 4 | Witness | For a receipt an answer decided, a call given whose typed hash is its `by`: the call reads (maybe-11), names the Accord id and the receipt's question, takes the action the receipt records, names the receipt's digest for an accept or a change, and is from the receipt's `from` for an accept or a change, an answerer for a decline, or a member when row 3 is skipped and the question's answerers are not known, the asker for a cancel. A release follows the closing only when the call asks for one; and when it asks for one, a release follows, but for a change whose question's receiver is its answerer or its asker, which releases nothing, and in a subset, which may leave the release out | `witness` |
| 5 | Body | A delivery: its receipt is one of the list, by its id, and the salted digest of its body under its salt is that receipt's subject digest, of its kind | `body` |

A close receipt goes through rows 0 and 1 and no other: it is no decision. When the close is the first receipt, row 0 founds the set at its `at`.

A call is matched only in its canonical bytes, as it was signed: a copy re-encoded with other whitespace or member order has the call's typed hash once made canonical, but it matches no receipt. A call given that matches no receipt is counted and fails nothing. A receipt an answer decided that no call given matches is unwitnessed, never a failure (verify-3). The first failure decides the code and index, and a verifier still reads every receipt's decider and witness, so that a log that does not conform is shown whole.

#### Scenario: a journal verified by its Accord id

- Given a journal of a note, an ask and an accept with release, and its founding set
- When it is verified with the Accord id and the set
- Then it is valid and conforms; and with the id one hex digit off it is `founding`

#### Scenario: a journal verified without the openings

- Given the same journal, its sealed founding set and its acceptances, the decider's call, and no opening
- When it is verified
- Then it is valid and conforms with its terms not checked, and the accept and its release are witnessed by the decider's call

### verify-2 Conformance

Every decision a receipt states must be one the Accord let its decider make, by the rows of verify-1's table: the receipt names the Accord, its parties are members, decided again it is what the terms, the judge it names or the answer it names make of it, and an answer it names is witnessed by a call that fits it, when one is given. A verifier reports the first receipt that does not conform, with its code and index.

#### Scenario: decisions the Accord did not allow

- Given, each alone, a `pass` `ok` of a kind the terms block, an accept from a member who holds no answering role, a timeout whose `at` is not the deadline, and a judge's pass where both sides grant the judge only to advise
- When each journal is checked for conformance
- Then each fails `decision` at the receipt's index

#### Scenario: decisions the sealed terms hide

- Given a `pass` `ok` of a kind the terms block, from the proposer to the applier
- When it is checked with no opening, with the proposer's alone, and with the proposer's and the applier's
- Then it conforms with its terms not checked, conforms with its terms not checked, and fails `decision` at index 0

#### Scenario: calls that do not fit

- Given, each alone, an accept whose matched call names the decider but was signed by another key, one whose call names another Accord, a release whose call asks for no release, and a passing accept whose call asks for release with no release receipt after it
- When each journal is checked
- Then each fails `witness` at the receipt's index; and a passing change asking for release whose question's receiver is its answerer, or its asker, conforms with no release receipt

#### Scenario: one body

- Given a delivery a receiver fetched, and the journal
- When its body is checked
- Then it is the part its receipt names; with one byte of its text changed it fails `body` at its receipt's index

### verify-3 Witnessed and unwitnessed

An answer's receipt is witnessed when a call given, whose typed hash is its `by`, passes row 4: the call shows that the key that signed it answered, and the founding set shows which principal bound that key to which roles. With no such call it is unwitnessed: only the mediator says that a key holding the answering role answered, or that the asker cancelled. An unwitnessed answer is never a failure.

#### Scenario: with and without the calls

- Given a journal with an accept and the journal's answer calls
- When it is checked with the calls, and then without them
- Then the accept and its release are witnessed by the decider's key, and then unwitnessed, and both conform

### verify-4 Completeness only against a close

A verifier checks the receipts it is given. Until an Accord has a close, no receipt says that a journal holds every receipt the mediator wrote: a list cut short after any receipt still verifies (receipt-6). So without a close, conformance claims nothing about completeness, and a subset claims less still. A whole list that ends in its close is complete against that close (close-4): the close pins the last receipt of every chain, and every chain links back to its first receipt by `prev`. That claims nothing about another list the mediator's key may have signed for the same Accord.

#### Scenario: no close receipt

- Given any valid journal, and separately a subset of it
- When it is checked
- Then completeness is not claimed: of the journal, because the Accord has no close receipt, and of the subset, because only a subset was given

## 9. Reconciling kept records

Reconciling compares a mediator's receipts, as they are presented, with what each party kept: the envelopes it signed and the receipts of the deliveries it fetched. It names what is in one record and not the other. Its inputs are the mediator's reference, the Accord id and the card hash it expects, the presented receipts in order, and for each party its kept envelopes and delivered receipts. A close among the presented receipts is checked against the Accord id and the card hash as every receipt is; it echoes no envelope and queued nothing, so no list names it. Parties are read in actor-reference order and each party's records in the order kept.

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

Every vector has `vector`, its path under `vectors/` without the `.json` extension, a group and a name each in lowercase words joined by hyphens; `spec`, `0.1`; `tests`, one line saying what the vector tests; and `operation`, what it runs: `canonical`, `sign`, `verify-signature`, `found`, `view`, `scenario`, `verify`, `reconcile` or `conform`.

#### Scenario: a vector's path

- Given the file `vectors/canonical/duplicate-key.json`
- When its header is read
- Then its `vector` is `canonical/duplicate-key` and its `spec` is `0.1`

### conformance-vectors-2 Inputs an implementation would choose

A vector gives every input an implementation could choose or read from the world: each key as a 32-byte Ed25519 seed in lowercase hex under a name in `keys`, every salt, every message id, and every time. An implementation run on a vector generates none of them, so two implementations produce the same bytes.

#### Scenario: an answer's salt

- Given a scenario vector with an accept
- When the runner runs it
- Then the answer's salt is the step's `salt`, the one the answerer minted, not one the implementation minted

### conformance-vectors-3 Scenario vectors

A `scenario` vector founds one mediator from `founding`, with every member's opening under `openings` and the acceptances under `acceptances`, at `foundedAt`, run by the key named `mediator`, and applies its `steps` in order. A step is `give`, `get`, `resolve`, `tick` or `close`, and each names the time it runs at in `at`, but `get`, which reads no time. A `close` step closes the Accord (close-1) from the mediator's state as the steps before it left it, and expects `receipts`, the outcomes of the closings it writes, in order, and `close`, the close receipt and its id; a `get` after the close that finds nothing queued expects that `close` (close-3). A `give` step may carry `judge`, `{"name","verdict"}`, the judgment the decision is given: a judge's card name and `pass` or `block` (composition-7). After the last step the runner compares `final`: `receipts`, every receipt the mediator wrote, in order, each with its id; `delivered`, for each party by name, the deliveries still queued for it, in order, a party with none left out; and `pending`, the ids of the questions still open, in the order they were opened. Every outcome a step expects names a receipt's `verdict` and `reason`, paired as receipt-4 says: a `give` step's is a crossing receipt's, never with a reason only a resolution receipt has; a `tick` step's are timeouts, `block` with reason `timeout`; and a `resolve` step's is its resolution receipt's, with its `release` composition's. Every expected receipt names its decider (receipt-11). The runner also verifies `final.receipts` under the mediator's reference (section 8), and a list that fails fails the vector. Steps after a rejected step still run.

#### Scenario: a give, then the end state

- Given a scenario vector whose one step is a give that passes
- When the runner runs it
- Then `final.receipts` holds that receipt, `final.delivered` holds its delivery under the receiver's name, and `final.pending` is empty

### conformance-vectors-4 Operations and groups

Each operation names what the runner calls and what it compares. `canonical` puts `inputText` or `inputHex` in canonical form and expects `canonicalHex` and `hash`, or `code` and `offset`. `sign` signs `input`, an object, as the key named `signer` under `typeUri`, and expects `signingBytesHex` and `sig`. `verify-signature` verifies the object given as `input`, `inputText` or `inputHex` under `typeUri` by `reference`, and expects `valid`. `found` founds the set given as `input`, `inputText` or `inputHex` at `foundedAt`, as the mediator the set names, with `openings`, every member's, or none when it has none, and the acceptance list as `acceptances`, `acceptancesText` or `acceptancesHex`, and expects `accord` and `card`, with `terms` `not-checked` when it holds no opening (setup-contract-16), or `code`. `view` runs founding's rows 1 to 17 over the set given as `found`'s is, at `foundedAt` with every member's opening under `openings`, and builds the view of the member `viewer` names (setup-contract-14), and expects `view` and `hash`, its typed hash, or `code`. `verify` verifies `receipts` under `reference`, and expects `verified`, the number of receipts, or `code`, with `index` for every code but `reference`. `reconcile` reconciles `receipts` with `kept` under `reference`, `accord` and `card`, `parties` giving each party's name its reference, and expects `removed`, `unrecorded` and `missing`, or `code` with the index, and for `kept` the party. `conform` checks `receipts` for conformance (verify-1) given `accord`, `founding` with `acceptances` or `reference`, `openings`, those the verifier holds, some or every member's, or none, `answers`, each a call as `call` or `callHex`, `subset`, `delivery`, and `kernel`, the kernel hash the verifier trusts to decide again, and expects `code` with `index`, a code of section 8's first table or of verify-1's, or `conforms` with the counts `witnessed`, `unwitnessed` and `unmatched`, `module`, whether the card's kernel was the trusted one, `terms` `not-checked` when a founding set was given and a seal was not opened, and `body` when a delivery was given. A `verify` or `conform` vector expects the codes of rows 8 and 9 of section 8, `close` and `after-close`, as it does every code of that table. Vectors are filed in groups by the section of this text they test: `canonical`, `signatures`, `founding`, `view`, `crossing`, `receipts`, `questions`, `conform` and `close`; `view` is filed under section 3.

#### Scenario: a founding refusal

- Given a `found` vector whose set has a binding by an unlisted principal
- When the runner runs it
- Then the implementation returns `binding-principal`, the vector's `expect.code`

#### Scenario: a founding without the openings

- Given a `found` vector of a sealed set with its acceptances and no `openings`
- When the runner runs it
- Then the implementation returns the Accord id and the card hash, and that the terms were not checked, `expect.terms`

#### Scenario: a view

- Given a `view` vector
- When the runner runs it
- Then the implementation's view is `expect.view`, byte for byte in its canonical form, and its typed hash `expect.hash`

### conformance-vectors-5 How results compare

The runner compares exactly. A rejection is compared by its code. An outcome is compared by `verdict`, `reason`, `receiptId` and the receipt, byte for byte in its canonical form. A delivery is compared member by member, its body in the form it crossed. A canonical output is compared as bytes, a code with its offset. No result is compared by a part of its value, and a result the vector does not state is not taken as a pass.

#### Scenario: a receipt that differs in one byte

- Given a give step whose expected receipt differs from the one the implementation wrote in one byte of its signature
- When the runner compares them
- Then the vector fails at that step

### conformance-vectors-6 The resolve step

A `resolve` step is an answer step or the mediator's finding. An answer step has `callHex`, the answer call's canonical bytes (maybe-11), and for `accepted` and `changed` also `answer`, the answer's text or the version, and `salt`, the salt the answerer minted, exactly 32 bytes in lowercase hex; the action, the question and whether it asks for release are the call's. `by` names the member that sends it; without it, the caller is the call's `from`. The mediator's finding that nobody can be reached has `question`, the id of the question's receipt, and `undeliverable`, `true`, and no `callHex`, `answer`, `salt` or `by`: it takes no caller. A step holds no `action`. `expect` is the outcome of the resolution receipt, or the rejection. After an accept or a change that outcome is composition's, and so is the release's, which `expect` holds as `release` only when the call asks for one; after a decline, a cancel or an undeliverable it is `block` with that action as its reason. The schema cannot read the call, so a runner reads it and fails, as a malformed vector, an answer step whose `expect` does not pair with its call so.

#### Scenario: an accepted answer with release

- Given a `resolve` step whose call is an accept asking for release, and an answer that passes
- When the runner runs it
- Then the implementation's resolution outcome equals `expect`, and its release outcome equals `expect.release`

#### Scenario: a change with release

- Given a `resolve` step whose call is a change asking for release, `answer` holding the version
- When the runner runs it
- Then the implementation forms the version as maybe-9 says, under the step's `salt`, its resolution outcome to the asker equals `expect`, and its release outcome to the receiver equals `expect.release`

#### Scenario: an answer step that does not pair with its call

- Given an answer step whose call is a decline and whose `expect` is `pass`, `ok`
- When the runner runs it
- Then the vector fails as malformed

### conformance-vectors-7 The tick step

A `tick` step passes time to its `at` and expects `receipts`, the outcomes of the receipts written, in order, which is empty when no deadline was reached.

#### Scenario: a tick past a deadline

- Given a question with deadline `2026-11-02T09:00:01Z` and a `tick` step at `2026-11-02T09:00:05Z`
- When the runner runs it
- Then `expect.receipts` holds exactly one outcome, `block`, `timeout`, whose receipt's `at` is the deadline

### conformance-vectors-8 Bytes under test

An input whose exact bytes are under test is given in one of three forms: as a JSON value, whose bytes are its canonical form; as text, whose bytes are its UTF-8; or as hex. The top-level input is `input`, `inputText` or `inputHex`; the acceptance list is `acceptances`, `acceptancesText` or `acceptancesHex`; an opening under `openings` is an object holding its `actor` and `opening`, `openingText` or `openingHex`; a give's message is `message`, `messageText` or `messageHex`, and its envelope `envelope`, `envelopeText` or `envelopeHex`; a receipt in a list is an object holding `receipt`, `receiptText` or `receiptHex`. Exactly one form of each is given. A `canonical` vector takes only `inputText` or `inputHex`, and an `offset` is counted in those bytes. Empty hex is the empty input.

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

## 11. What the command line prints for a script

Placket's command line prints prose for a person on standard error and JSON or a body on standard output. A script reads neither safely: an id is one word of a line, a question's id shares standard output with the receipts, and a code is the last word of a line. So every command also takes `--json`. With it, the command prints on standard output exactly one JSON object, on one line, after it has done its work, and nothing else. Its exit code and every line it prints on standard error stay what they are without `--json`, and without `--json` nothing it prints changes. This section says what the object holds; it adds no receipt, no code and no format a mediator or a verifier reads.

Every object has `exit`, the command's exit code. The words a person reads are the product words: `pass`, `block` and `ask` for the verdicts `pass`, `block` and `escalate`, `released` for a release that passed, and for a closing the words `accepted`, `changed`, `declined`, `cancelled`, `timeout`, `undeliverable`, `closed`, `blocked` and `open`. A reason and a code are spelled as this text spells them, and a receipt, a predicate, a declaration, a binding and a founding set are carried as JSON values, unchanged. A member the tables give with a condition is left out when the condition does not hold; a list is there even when it is empty. These are the objects a command's members are built of.

| Object | Members |
|---|---|
| Refusal | One of `rejection`, a code of this text the window returned; `founding`, a founding code the window refused an acceptance with (setup-contract-15); `door`, a code of the window's door; or `error`, a code of the command's own, with `detail`, the list of what the error line carries after its code, each as the line prints it, such as `["chain","receipts.jsonl","line 3"]`, when it carries any |
| Outcome | `word`, the product word of the receipt's verdict, `released` for a release that passed; `reason`; `receiptId`; and `receipt`, the receipt as the window returned it |
| Closing | `word`, the closing's word; and, for a question that closed, `receiptId` and `receipt`, those of the receipt that closed it |
| Delivery | `receiptId`, `receipt`, `messageId`, `kind`, `salt`, `re` when the part has one, `mediaType` when it had one, and `answer`, the answer call, when an answer queued it, as the delivery carries them; and its body as `text`, `raw` or `data`, as the delivery carries it |
| Comment | `crossed`, `true` only when the comment's give passed; and the comment give's `outcome`, or the members of its refusal |

A command that is refused, or fails, has the members of its refusal beside what it had done before. The commands' members, besides `exit`:

| Command | Members |
|---|---|
| `key new`, `key ref` | `actor`, the key's actor reference |
| `declare` | `declaration`, the signed declaration |
| `seal` | `seal`, the seal entry; `opening`, the path of the file the opening was written to, never the opening |
| `bind` | `binding`, the signed binding |
| `founding` | `founding`, the founding set; `accord`, its Accord id; `card`, its card hash |
| `window` | Printed when the window stops. Once it started: `accord`, `card`, `operator`, and `started`, `founded`, `reloaded`, `closed` or `proposed`, and `founded` once every member of a proposed Accord accepted; on a closed Accord, `close`, the close receipt's id |
| `view` | `view`, the member's own view; `hash`, its typed hash; `lines`, the lines it prints for a person |
| `accept` | `acceptance`, the signed acceptance the window took; `founded`, whether the Accord is founded |
| `window close` | Printed when it stops: `accord`; `close`, `{"receiptId","receipt"}`, the close receipt; and `closings`, the list of the outcomes of the closings it wrote, in order. With `--socket`, also the members `window` holds |
| `give` | `outcome`; `question`, the ask's receipt id, when the outcome is an ask; with `--wait`, the members of `result` besides |
| `result`, and `give --wait` | `question`; `closing`; `release`, an outcome, when the answer had a release; after an accepted or changed answer that passed, `fetched`, `true` when the answer or the version was found, `answer`, its delivery, when it was, and `deliveries`, the list of the other deliveries it fetched, when it fetched any. A question still open, and a wait that was interrupted, have the closing `{"word":"open"}` |
| `answer`, `cancel` | `question`; `outcome`; `release`, when the answer had one |
| `change` | As `answer`, and `comment` when a comment was given |
| `decline` | `question`; `outcome`; `comment`, once the window took the decline |
| `get` | `delivery`, when one was fetched; with `--out`, the delivery without its body, and `path`, the file the body was written to. With `--all`, in place of `delivery` and `path`, `deliveries`, the list of every delivery it fetched, in the order it fetched them, each as `get` without `--all` holds it; with `--out`, each without its body and with `path`, the file its body was written to, when it was written. `close`, `{"receiptId","receipt"}`, when the window handed out the Accord's close receipt (close-3) |
| `questions` | `questions`, the list of the open questions this key may answer, each the object the command prints for it without `--json`, with `receiptId` and `receipt`, those of the ask; never the part's salt or a raw body |
| `mediators` | `mediators`, the list of the labels config's entries, each the object the command prints for it without `--json`: `accord`, `founding`, `key`, `label` when it has one, `socket` and `state` |
| `watch` | Before its own object, which holds `exit` and any refusal, one object per question it shows, each on a line of its own as the question opens: `label`, `id`, `kind`, `asker`, `deadline`, `form`, the part as `questions` shows it, and `commands`, the `answer`, `change` and `decline` lines that answer it |
| `show` | `verified`, `false`; `receiptId`; `word`; `reason`; `by`, the decider as the predicate holds it; `kind`, the part's kind; `at`; `from`; `to`; `module`, the module line; `claim`, the receipt's claim line; `notClaimed`, the scope line; and `shown`, the list of the lines the command prints on standard output without `--json` |
| `verify` | `verified`, the number of receipts; `mediator`; `accord`; `card`; with `--founding`, `operator`, `runs`, `module`, an object of the kernel's `version` and `hash`, `judges`, the card's judges as it lists them, `members`, a list with one object per member in actor-reference order, each its `actor`, its `roles` as its binding gives them, and the `principal` that signed its binding, `custody`, its line, and `supersedes`, `{"accord","close"}`, when the policy has one; `counts`, the number of receipts by product word, `pass`, `block` and `ask`; `questionsClosed`; `questionsOpen`; `releases`; `blockedReleases`; `starts`, a list of `{"from","to","seq"}`, each chain a subset starts late; `decisions`, a list of `{"receiptId","line"}`, each decision line; with `--founding`, `terms`, the terms line, `seals`, the list of seal lines, and `views`, the list of view lines; `unmatched`, the number of calls matching no receipt; `valid`, `conforms` and `complete`, each its line, `complete` against the close when the list ends in one, and with `--superseded-by` the successor that pins it; `body`, its line, with `--delivery`; `predicates`, the list of the receipts' predicates in journal order; `claim`, the claim line; `notClaimed`, the scope line |
| `log` | `log`, the object the command prints without `--json`, which carries the claim and the scope line. `--json` with `--format csv` is the command's error `format` |
| `reconcile` | `removed`, `unrecorded` and `missing`, as the command prints them without `--json` |
| `released` | On a match, `receiptId` and `receipt`, those of the receipt the file, or the kept part, matched; `kind`, the part's kind; `verified`, `false`; `claim`, the claim line, the value's with `--at`; `notClaimed`, the scope line. With no match, `exit` alone. With `--every`, in place of `receiptId`, `receipt` and `kind`, `matches`, the list of every match in the order the key fetched them, each `receiptId`, `kind`, `at`, the receipt's time, `from`, the receipt's sender, and `re` when the part has one; on a match `verified`, `claim` and `notClaimed` beside it |
| `version` | `version`, the binary's version as its build stamped it, or `(devel)` when the build stamped none; `commit` and `commitTime`, the commit the binary was built from and that commit's time, when the build carries them; `go`, the Go release that built it, as Go spells it, such as `go1.27.4`; `module`, an object of the decision module's `version` and `hash`, the module `card --default` names |

### commands-1 One object on standard output

With `--json`, a command prints exactly one JSON object and a line feed on standard output, and nothing else there, whatever its exit code, but `watch`, which runs until it is stopped and prints one object per question before its own, as its row says; `mcp`, whose standard output carries the MCP protocol, takes no `--json`; standard error and the exit code are as without `--json`. The object has `exit`, and the members the table gives its command, of the objects the table above defines. A refusal's members name its code under the member of its kind. A command's flags that cannot be read, or that name no command, are the error `usage`, in an object, whenever `--json` is among them. Whatever combination of its flags a command runs with, the object holds every result the run produced: each outcome, closing and release the window returned to it, each delivery it fetched and each file it wrote. No flag leaves a result out or puts one in the place of another, and a list of results is there even when it is empty. On a terminal the object is escaped as the command's other JSON is, and stays the same JSON.

#### Scenario: a give that passes

- Given a member that gives another a part its terms let cross
- When it gives with `--json`
- Then standard output is one object of `exit` 0 and the outcome, whose `word` is `pass`, `reason` `ok`, and whose `receiptId` and `receipt` are the receipt's the journal holds
- And standard error is the line it prints without `--json`

#### Scenario: a refused command

- Given a stranger's give, a give with no flags, and a command that does not exist
- When each runs with `--json`
- Then the stranger's object is `{"exit":2,"rejection":"not-a-member"}`, and the others' `{"error":"usage","exit":3}`

#### Scenario: every result under every flag

- Given every command, and each combination of its flags that lets one run produce more than one result
- When it runs with that combination and `--json`
- Then the object holds every receipt the run added to the journal, every delivery it fetched and the path of every file it wrote

### commands-2 Outcomes, questions and answers

A give, an answer, a change, a decline, a cancel and a result say in their objects what they produced: each outcome with its product word, its reason and its receipt, the question's id under `question`, and a closing under its word. An ask's object holds the question's id under its own member, so a script never reads it out of the receipts. A comment's object says whether it crossed.

#### Scenario: an ask left open

- Given a give that the terms make an ask
- When it gives with `--json` and no wait
- Then the object's `exit` is 4, its outcome's `word` is `ask` and `reason` `maybe`, and its `question` is the outcome's `receiptId`
- And `result` for that question prints `{"closing":{"word":"open"},"exit":4,"question":<the id>}`

#### Scenario: waits that end with an accept and a change

- Given a give with `--wait` and `--json`, and the answerer's `answer` with `--json`
- Then the answer's object holds the outcome `pass` and the release `released`, and the wait's object holds the closing `accepted` with the answer's receipt, the release, `fetched` `true`, and the answer's delivery, with `exit` 0
- And for a `change`, the wait's closing is `changed`, its answer the version's delivery, and its `exit` 1

#### Scenario: a decline and its comment

- Given an open question
- When its answerer declines it with a comment and `--json`
- Then the object holds the outcome `block` with reason `declined`, and the comment `{"crossed":true,"outcome":...}` with the outcome `pass`

### commands-3 What a command makes or fetches

The commands that make a key, a declaration, a seal, a binding or a founding set put what they made in the object, as they print it without `--json`, and the ids they print on standard error beside it; a seal's object names the file its opening was written to, and never holds the opening. A member's view and its acceptance go in the objects of the commands that show and sign them, and the acceptance's object says whether the Accord is founded. A binding is made for one policy: the command reads the policy's file and writes the typed hash of its canonical bytes into the binding's `policy`; whether the policy lets the principal grant the binding's roles is founding's to judge. A fetch puts the delivery in the object; an empty queue leaves it out. A fetch of every delivery waiting puts every delivery it fetched in one list, in the order it fetched them, each as a fetch of one puts it, and with the path of its file when it writes each body to a file; on an empty queue the list is empty. A window prints its object when it stops.

#### Scenario: a founding

- Given the parts of a founding set
- When `founding` runs with `--json`
- Then the object holds the set it prints without `--json`, and the Accord id and the card hash it prints on standard error

#### Scenario: a binding for a policy

- Given a policy file whose members are in any order
- When `bind` runs with it and `--json`
- Then the object's binding names the typed hash of the policy's canonical bytes as its `policy`, and the binding founds in a set with that policy
- And a policy file the canonical form refuses is the command's error `file` with the detail `policy`, and no binding is printed

#### Scenario: an empty queue and a delivery

- Given a member with nothing queued
- When it fetches with `--json`
- Then the object is `{"exit":0}`
- And after a part crosses to it, the object holds the delivery with its text, and with `--out` the delivery without its text and the path of the file

#### Scenario: every delivery at once

- Given a member with nothing queued
- When it fetches every delivery with `--json`
- Then the object is `{"deliveries":[],"exit":0}`
- And after one part crosses to it, and then three, the object holds a list of one delivery, then of three in the order they crossed, each the delivery a fetch of one holds, with its text

#### Scenario: every delivery at once into a directory

- Given a member with nothing queued
- When it fetches every delivery into a directory with `--json`
- Then the object is `{"deliveries":[],"exit":0}`
- And after three parts cross to it, the object holds a list of three in the order they crossed, each the delivery without its text and with the path of the file that holds its body

#### Scenario: a seal and its opening

- Given a party's signed declaration
- When `seal` runs with `--json`
- Then the object holds the seal entry, whose seal is the salted digest of the declaration's canonical bytes under the opening's salt, and the path of the opening, a file of mode 0600 holding exactly the declaration and the salt, and nothing it prints holds the salt
- And a second seal into the same directory replaces nothing, and a declaration another key signed is the command's error `seal`

#### Scenario: a view and its acceptance

- Given a proposed Accord of two members
- When a member runs `view` and then `accept`
- Then `view` prints the member's own view and keeps it, and `accept` prints the acceptance of that view's typed hash and says the Accord is not founded yet; a give then is the door's `not-founded`
- And once the other member accepts, its object holds `founded` `true`, and a give passes

#### Scenario: a window stops

- Given a window run with `--json`
- When it stops
- Then the object holds the Accord id, the card hash, the operator and `founded`, and after a restart `reloaded`

### commands-4 Claims and records

Every command that prints a claim line carries it in its object under `claim`, with the scope line that follows every claim line under `notClaimed`, so a claim never travels without it: `show` the receipt's own claim, `verify` the claim of a verified journal, and `log` both inside the log object. `verify` and `reconcile` carry what they print on standard output in their objects, unchanged. `verify` with a founding set names who stands behind each member: its roles and the principal that signed its binding, on standard error and in its object.

#### Scenario: a shown receipt and a verified journal

- Given a journal of one receipt that passed
- When `show` and `verify --founding` run with `--json`
- Then `show`'s object holds the pass's claim line and the scope line, and `shown` the lines it prints without `--json`
- And `verify`'s object holds the count, the counts by product word, the operator and the module, every predicate it prints without `--json`, the claim line and the scope line

#### Scenario: who stands behind each member

- Given a journal and its founding set, whose members are bound by two principals
- When `verify --founding` runs with `--json`
- Then its object's `members` lists each member's actor, roles and principal, in actor-reference order
- And without `--json`, standard error carries one line per member in that order, naming its actor, its roles and its principal

#### Scenario: the log and reconciliation

- Given the same journal
- When `log` and `reconcile` run with `--json`
- Then `log`'s object holds the log it prints without `--json`, `reconcile`'s the three lists it prints, and `log` with `--format csv` is `{"error":"format","exit":3}`

### commands-5 A file checked against what crossed

`released --file F --state DIR` says whether the bytes of the file F are a part that crossed to the key whose state directory is DIR. It reads the directory and writes nothing. The directory keeps, for each delivery the key fetched, the delivery's receipt and the salt the part came with; a salt that is not 32 bytes matches nothing. The file matches a delivery when the delivery's receipt passed, is addressed to the directory's key, and names as its digest the salted digest of the file's bytes under that salt (crossing-3). A file matches only as the fetch wrote the body: a text's UTF-8, a raw body's decoded bytes, a data body's canonical bytes. The command compares bytes; which fields count, and how values are normalised before the check, are the caller's.

On a match the command prints the receipt's id on standard output and exits 0. On standard error it prints `not verified`, the claim line `claim: these bytes, under the salt they were fetched with, are the part this receipt let cross to this key.` and the scope line. The receipt is read as the directory holds it, not verified; verifying the journal shows it is the mediator's (receipt-6). With no match it prints `no release matches these bytes` on standard error, nothing on standard output, and exits 1. A receipt that blocked, and a receipt addressed to another key, never match, whatever the directory holds. A file that cannot be read is the command's error `file`. The command says only that these bytes are what crossed under the receipt, never what any party did with them after.

#### Scenario: a file that crossed, and one byte changed

- Given a text, a raw body and a data body given to a receiver, and each fetched into a file
- When the receiver checks each file
- Then each matches the receipt it was fetched under, exit 0, and with `--json` the object holds that receipt, its kind, `verified` `false`, the claim line and the scope line
- And the same bytes with any one byte changed, a byte added, or the data body as it was spelled before its canonical form, match nothing: exit 1, and with `--json` `{"exit":1}`

#### Scenario: a released proposal

- Given a proposal asked about, accepted with release and fetched by its receiver
- When the receiver checks the file
- Then it matches the release receipt, from the asker to the receiver, and the asker's own directory matches nothing for it

#### Scenario: a block, and a part to another key

- Given a receiver's directory holding the receipt of a part blocked for it, with the part's salt, and another key's directory holding the receipt and the salt of a part that crossed to the receiver
- When each checks the part's bytes
- Then neither matches

### commands-6 A value checked inside what crossed

`released --file F --at P --value-file V --state DIR` says whether the bytes of the file V are the value at the path P inside a part that crossed to the key whose state directory is DIR, where F holds that part's body as the fetch wrote it. `--at` and `--value-file` go together; either without the other is the error `usage`. The value matches a delivery when F matches it as commands-5 says, and the value at P in F's bytes, read as below, is exactly V's bytes. So a blocked part, a part addressed to another key, and a value inside a body that never crossed to this key never match. Placket normalises nothing: which value counts, and how it is spelled, rounded or chosen, is decided before the part crosses, so that the part the receipt names holds the value as it will be checked, and the caller writes V in those same bytes.

With `--kind K`, given once or more, with or without `--at`, a delivery matches only when its receipt's kind is one of those given, compared as written. Deliveries are tried in the order the key fetched them, and the first that matches is the result, which names its kind.

A path is a JSON Pointer: empty, for the whole body, or a `/` before each reference token, in which `~1` stands for `/` and `~0` for `~`. A path that is not one, such as `total`, or one with a `~` before anything but `0` or `1`, is the command's error `path`.

A body's bytes hold values only when they are one JSON text: well-formed UTF-8 (canonical-6) with no byte order mark, in the grammar of RFC 8259, nested at most 1000 deep (canonical-8), with no escape of an unpaired surrogate (canonical-6), and with no object holding two members whose names are equal once their escapes are read (canonical-4). Numbers are not judged by their value there: `12.50`, `1e3` and `-0` are numbers as written. A body that is not such a text, a text that is not JSON among them, holds no value at any path.

- **What a token selects.** In an object, a reference token selects the member whose name equals the token once the name's escapes are read, case included. In an array, it selects the element at the index the token writes: a decimal integer from 0, with no sign and no leading zero, below the array's length, so `-` and `01` select nothing. A token selects nothing in a string, a number or a literal, and a path with a token that selects nothing holds no value.
- **The bytes of a value.** The value's bytes are the body's bytes from the value's first byte to its last, exactly as the body spells them: a string with its quotes and its escapes as written, a number as written, an object or an array with whatever whitespace it holds inside. Whitespace before and after the value is not part of it.
- **A data body.** Its bytes are canonical (crossing-2), so the bytes of a value inside it are that value's canonical bytes (canonical-1), such as `"Ann"` or `1250`, and a caller can write them from the value alone. A data body holds no fraction (canonical-2).
- **A text or raw body.** A body that crosses as a text, or as raw bytes, that is a JSON text is read by the same rule and keeps every number as written, a fraction included: in the text `{"total": 12.50}` the value at `/total` is the five bytes `12.50`, and `12.5` does not match.

On a match the command prints the receipt's id on standard output and exits 0. On standard error it prints `not verified`, the claim line `claim: these bytes are the value at this path inside the part this receipt let cross to this key.` and the scope line. With no match it prints `no release holds this value at this path` on standard error, nothing on standard output, and exits 1. A file F or V that cannot be read is the command's error `file`. The command says only that these bytes are a value inside the part that crossed under the receipt, never what any party did with the value after.

#### Scenario: a value in a data body, and one byte changed

- Given a data body given to a receiver and fetched into a file
- When the receiver checks a string, a number, an object and the whole body at their paths, each written as its canonical bytes
- Then each matches the receipt it was fetched under, exit 0, and with `--json` the object holds that receipt, its kind, `verified` `false`, the value's claim line and the scope line
- And the same bytes with any one byte changed or a line feed added, a string without its quotes, and an object spelled with a space match nothing: exit 1, and with `--json` `{"exit":1}`

#### Scenario: a fraction in a text body

- Given a JSON text holding the number `12.50` and the array `[ 1, 2 ]`, which a data body refuses, given as a text and as raw bytes and fetched
- When the receiver checks `/total` and `/items`
- Then `12.50` and `[ 1, 2 ]` match, and `12.5`, `"12.50"` and `[1,2]` match nothing
- And a text that is not a JSON text matches as a file but holds no value at any path

#### Scenario: a match limited to kinds

- Given one data body given to a receiver twice, as a comment and as an answer, and fetched
- When the receiver checks the file and a value inside it with `--kind`
- Then each kind matches its own receipt and the result names that kind, and a kind neither part has matches nothing

#### Scenario: a block, a part to another key, and a body never released

- Given a receiver's directory holding the receipt and the salt of a body blocked for it, and another key's directory holding the receipt and the salt of the same body that crossed to the receiver
- When each checks a value inside that body, and the receiver checks the same value at the same path inside other bytes
- Then none matches

### commands-7 Everything a key kept, checked against what crossed

`released --state DIR`, without `--file`, says whether a part that crossed to the key whose state directory is DIR is among the parts the directory kept, and which. It reads the directory and writes nothing. It tries every part the key fetched and kept, in the order the key fetched them; a part the key gave, and a version it gave as an answer, are not tried. Each kept part is checked as commands-5 checks a file, its body as the fetch wrote it: it matches when the delivery's receipt the directory keeps, of the receipt id the part names, passed, is addressed to the directory's key, names the part's kind, and names as its digest the salted digest of the part's body under the salt the part was kept with (crossing-3). So a kept part whose body, salt or kind was changed after it was kept, or whose salt is not 32 bytes, never matches, and a directory that kept nothing matches nothing. With `--file`, the command is as commands-5 and commands-6 say.

Conditions narrow a match, and a part matches only when every condition given holds:

- **`--kind K`**, given once or more: the receipt's kind is one of those given, as commands-6 says.
- **`--at P`, with `--value-file V` or `--value S`**: the value at the path P in the part's body is exactly V's bytes, or the UTF-8 of S, read as commands-6 says. `--at` goes with exactly one of the two; `--at` alone, either of them without `--at`, or both of them, is the error `usage`. `--value` is taken with `--file` too.
- **`--re ID`**: the part's `re`, as its delivery carried it, is ID. A receipt does not state a part's `re`, so this condition reads the kept delivery, not the receipt. `--re` reads kept parts only: with `--file` it is the error `usage`.
- **`--from ACTOR`**: the receipt's `from` is ACTOR, compared as written. A released proposal is from its proposer. `--from` is taken with `--file` too.

Any release that matches is a match, not only the latest one. Placket keeps no notion of a current or latest release: two answers on one question, or one body released twice, are two matches, and which of them counts is the caller's rule. Without `--every` the result is the first match in fetch order. With `--every` the result is every match, in fetch order, and a caller applies its own rule to the list; `--every` reads kept parts only, and with `--file` it is the error `usage`.

On a match the command prints on standard output the receipt id of the first match, or with `--every` of each match, one a line, and exits 0. On standard error it prints `not verified`, the claim line `claim: for each receipt shown, a part this key kept, under the salt it was fetched with, is the part that receipt let cross to this key.`, or with `--at` `claim: for each receipt shown, these bytes are the value at this path inside a part this key kept, the part that receipt let cross to this key.`, and the scope line. With no match it prints `no part this key kept matches`, or with `--at` `no part this key kept holds this value at this path`, on standard error, nothing on standard output, and exits 1; with `--every` and `--json` the object then holds `matches`, empty. A value file that cannot be read is the command's error `file`, and a path that is not a JSON Pointer the error `path`. The command says only that a kept part is what crossed under each receipt shown, never what any party did with it after.

#### Scenario: every condition alone and combined

- Given a receiver that fetched and kept, in order, a proposal released after an accept, from its proposer with no `re`; a changed version of a second question, from the decider with `re` that question, holding the same value at `/total`; and a changed version of a third question, from the decider with `re` the third, holding another value there
- When the receiver asks with no condition, and with each of `--kind`, `--at` and `--value`, `--re` and `--from`, alone and together
- Then each answer is the first part in fetch order for which every condition given holds, and a set of conditions no single part meets matches nothing, though each holds for some part

#### Scenario: every match in fetch order

- Given the same directory
- When the receiver asks with `--every`, and with `--every` and a condition
- Then standard output holds the receipt id of each match, one a line, in fetch order, the two parts that hold the same value both among them, and with `--json` `matches` holds each match's receipt id, kind, time, sender and `re` when it has one
- And a condition that no part meets prints `no part this key kept matches`, exit 1, and with `--json` `{"exit":1,"matches":[]}`

#### Scenario: a kept line changed, and nothing kept

- Given a receiver's kept part whose body has one byte changed or a byte added, or whose salt, kind or receipt id is changed, the receipt id to another part's or to one the directory does not hold
- When the receiver asks
- Then the changed part never matches, the parts beside it still do, and a directory that kept nothing, or that holds only parts its key gave and versions it gave as answers, matches nothing: exit 1

### commands-8 What verify prints

`verify` prints, on standard error, in order: the line of the mediator, the Accord and the card; with a founding set, the operator, the module, one line per judge of the card in card order, one line per member, the custody line, which says of how many members the operator is the principal, and what the policy supersedes; then, for each member whose opening `--open` gave, that its seal opens, and, with every member's opening, that each member's view matches its acceptance, or, without, that an opened member's view was not checked; and the terms line, that the terms were checked or that they were not, the declarations being sealed (setup-contract-16); the counts of verdicts, questions and releases; with a subset, each chain it starts late; with a founding set, one line per receipt the terms did not decide, in list order: an answer witnessed, by the key that signed its call and the principal that bound it to its roles, or unwitnessed, by the roles of the answerer, the `from` of the question's closing, or, for a cancel, the asker's; a judge of the card; or the mediator, by its event; with a subset, how many decisions were not checked because their question is not in it; how many decisions the terms made; how many calls given match no receipt, when any; then the `valid` line, the receipts' count and the mediator; the `conforms` line: that every decision is one the Accord let its decider make; with a seal not opened, that every receipt names the Accord, its card and its members and every call given fits, and that decisions were checked against the terms only where the seals they read are opened; that conformance was not checked for want of a founding set or because the card's module is not the one the verifier runs, or that the log does not conform, with the code and the line of `receipts.jsonl`; the `complete` line: that completeness is not claimed, for want of a close or because only a subset was given; or that the list is complete against its close receipt, named by its id, and, with the founding set of a successor Accord, that the successor's `supersedes` pins that close, naming the successor; and in each case what it does not claim, that the mediator signed no other list for this Accord; with a delivery, the `body` line; and the claim and scope lines. A log that does not conform, and a body that is not its receipt's part, exit 1 with every line; a validity failure prints its code and line and none of these, and so does a delivery given that is not one, or whose receipt is none of the receipts, a tool error. With `--json` the object holds each of these lines under its member.

#### Scenario: the lines of a verified journal

- Given the worked journal and its founding set
- When `verify` runs with the Accord id, with and without `--json`
- Then standard error holds the valid, conforms, complete, custody and decision lines, and the object holds the same lines under `valid`, `conforms`, `complete`, `custody` and `decisions`

#### Scenario: a sealed set verified, without and with its openings

- Given the worked journal, its founding set and its acceptances
- When `verify` runs with the Accord id and no opening
- Then it exits 0 and prints the valid line, the line that the terms were not checked, and the `conforms` line that decisions were checked against the terms only where the seals they read are opened, and no seal or view line
- And with one member's opening it prints that the member's seal opens and its view was not checked; with every opening, that each seal opens and each view matches its acceptance, that the terms were checked, and that every decision is one the Accord let its decider make; with an opening of another salt it is the command's error `founding` with `seal-mismatch`, and with acceptances of which one signs another member's view, `acceptance-view`

#### Scenario: a log that does not conform

- Given a journal holding a receipt the mediator's key signed whose decision the terms do not allow
- When `verify` runs with the Accord id
- Then it exits 1, prints the valid line and that the log does not conform at that receipt's line, and still prints every other line: each decision line, the count of the terms' decisions, and the complete, claim and scope lines

## 12. The log of a journal

`placket log` reads a journal, verifies it under the mediator's key, and prints its log: rows of what crossed, as one JSON object or as CSV. The log is a door format, not a frozen one: a mediator never reads it and a verifier never needs it, so this section adds no receipt, no code and no format of the protocol. Every cell of every row comes from a receipt that verified, or from a kept part such a receipt covers; nothing comes from a file name, the clock, or a line no receipt covers. A journal that does not verify, or whose receipts name more than one Accord or card, prints no row. A verdict is never decided again: the answer and the release are read from the receipts by receipt-6's rule, the one every reader applies.

A kept part is a line of a party's kept record, given with `--kept`. It is used only when a verified receipt covers it, and it is joined to that receipt by what the receipt signed, never by a time, a kind or a sender alone:

- **A part the actor gave** is joined by its envelope: the typed hash of the kept message's canonical bytes is the receipt's `envelope.hash`, and the part's kind is the receipt's subject `name`. Its body, kind, topic and re are covered.
- **A part the actor fetched, or a version it gave as an answer**, is joined by its salted digest: the digest of crossing-3 under the kept salt, which must be 32 bytes, is the receipt's subject digest, and its kind is the subject `name`. Its body alone is covered; a topic or a re it carries is not used.

A kept part that no receipt covers is left out, and the log says so on standard error, naming the receipt it claims.

The JSON object is `{"accord","claim","close"?,"mediator","notClaimed","rows"}`: the Accord, the claim line and the scope line that follows every claim line, the id of the close receipt when the journal holds one, the mediator, and the rows, each an object of its columns, `receipts` a list. CSV is a header and one record per row, quoted per RFC 4180, `receipts` the ids joined by spaces, and the claim and the scope line are printed on standard error; a cell that begins with `=`, `+`, `-`, `@`, a tab or a carriage return is written after a `'`. With `--words` the columns `text` and `changed_text` follow the others; with `--no-words` the `comment` column is empty.

These are the columns of a question's row, in order.

| Column | What it holds |
|---|---|
| `proposal` | The question's id, the id of its ask's receipt |
| `topic` | The asker's topic, from the asker's kept message |
| `kind` | The asked part's kind |
| `asked_at` | The ask's time |
| `answered_at` | The closing's time; empty while the question is open |
| `answer` | The closing's word, `accepted`, `changed`, `declined`, `cancelled`, `timeout`, `undeliverable`, `closed` or `blocked`, read from the closing receipt; `open` while the question is open |
| `released_at` | The release receipt's time, when the release passed |
| `comment` | The text of a comment that passed from the answerer to the asker with re the question, from the giver's kept message |
| `answer_signed` | `yes` when an answer call whose typed hash is the closing receipt's `by` reads (maybe-11), from the journal's answer calls or a kept part, and fits the closing: its Accord and question, the action the closing records, and its signer, the closing's `from` for an accept or a change and the asker for a cancel, with the closing's subject digest for an accept or a change; a decline's signer is not compared, since any answerer may decline while the closing's `from` is the first answerer |
| `answerer` | The closing receipt's `from` |
| `receipts` | The ids of every receipt the row rests on: the ask's, the closing's, the release's and the comment's |
| `text` | With `--words`: the asked part's text |
| `changed_text` | With `--words`: the changed version's text, from a kept part the closing receipt's digest covers |

With `--all`, the log also has a row for every crossing that belongs to no question, and every row has these columns after `receipts` and before the columns of `--words`. A question's row fills them from its ask's receipt.

| Column | What it holds |
|---|---|
| `at` | The receipt's time |
| `from`, `to` | The receipt's `from` and `to` |
| `word` | The product word of the receipt's verdict: `pass`, `block`, or `ask` for a question's row |
| `reason` | The receipt's reason |
| `re` | The re of the part, from a kept message the actor gave |

### log-1 One row per question

Without `--all`, the log has one row per question, in the order the questions were asked, with the columns of a question's row and no other. Passes and blocks that belong to no question have no row.

#### Scenario: the log of the worked example

- Given the worked example's journal and its three parties' kept records
- When `log` runs with and without the kept records, in JSON and in CSV
- Then it prints four rows, accepted, changed, declined and undeliverable, with the same answers either way, and the topic and the comment only from kept parts

#### Scenario: the log without every crossing is unchanged

- Given a journal that holds passes and blocks with no question, besides its questions
- When `log` runs without `--all`
- Then it prints, byte for byte, the rows of the questions of `log --all` with the columns of `--all` taken out, and no other row

### log-2 A row for every crossing

With `--all`, the log has one row for every receipt that opens a question and every crossing receipt that belongs to no question, a pass or a block, in journal order: a question's row stands where its ask's receipt stands. A closing and a release are not rows of their own; they stay in their question's row. A crossing's row holds `kind`, `at`, `from`, `to`, `word`, `reason` and `receipts`, the receipt's id, from its receipt; `topic`, `re` and `text` from a kept part that receipt covers, by the join above; and leaves the question's columns empty.

#### Scenario: every crossing in journal order

- Given a journal of questions, comments, a pass with no question and a block
- When `log --all` runs with the parties' kept records, in JSON and in CSV
- Then it prints a row for each ask, pass and block, in the order of the journal, and no row for a closing or a release
- And a block's row holds `block` and its reason, and the giver's topic and text from its kept message

#### Scenario: content by digest and by envelope, never by time

- Given two parts of one kind from one sender, given in the same second, and fetched by their receiver
- When `log --all --words` runs with the receiver's kept record alone
- Then each row's text is the text of the part its receipt's digest covers, and its topic is empty, since a fetched part does not cover its topic
- And with the giver's kept record alone, each row's topic and text are those of the message its receipt's envelope hash covers

## 13. Closing an Accord

An Accord is closed once, by whoever runs its mediator, and never reopens. The close is the mediator's own act: no member calls it, and no door identity is given to the operator or to the mediator's key. It closes every question still open, then writes the close receipt, the last receipt of the Accord, which pins the last receipt of every chain and lists every delivery not yet fetched. It never waits for a fetch, a deadline or a member. A verifier given a whole list that ends in its close can then say the list is complete against that close; a new Accord whose policy `supersedes` names the close (setup-contract-11) pins it, which is how a mediator's key is retired.

| Closing written by a close | Receipt from, to | Subject | Verdict, reason | `at` | `by` |
|---|---|---|---|---|---|
| A question whose deadline is at or before the close's time | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `timeout` | The deadline | The mediator, `deadline` |
| Every other open question | The first answerer, or the mediator when there is none, to the asker | The asked part | `block`, `closed` | The close's | The mediator, `close` |

### close-1 What a close does, and in what order

A close writes, in one operation and in this order: first a timeout for every open question whose deadline is at or before the close's `at`, at its deadline, as maybe-5 writes them, in the order the questions were opened; then, for each question still open, in the order the questions were opened, a closing from its first answerer, or the mediator when it has none, to its asker, `block` with reason `closed`, naming the asked part, echoing the question's envelope and naming the mediator, with the event `close`, as its decider; and last the close receipt (close-2), at the same `at`. Opened order is not deadline order: deadlines are set per rule. A close of an Accord that holds closings with reason `closed` and no close receipt after them, as a write cut short leaves it, completes that close: it takes the `at` of the closings already written, not its own, closes each question still open as `closed` at that `at`, and then writes the close receipt at it. Only a close completes it; a restart of the mediator refuses such a record.

#### Scenario: three questions, one past its deadline

- Given three open questions, opened in the order Q1, Q2 and Q3, where only Q2's deadline is at or before the time T
- When the Accord is closed at T
- Then four receipts are written: Q2's timeout at its deadline, a `closed` closing of Q1 at T, one of Q3 at T, and the close receipt last

#### Scenario: a close cut short

- Given a record whose last write was cut after its `closed` closings and before the close receipt, and another cut between two `closed` closings of one close
- When each is closed at a later time
- Then the first gains the close receipt alone, at the closings' `at`, and the second the closing still to write and then the close receipt, both at the `at` of the closing already written

### close-2 The close receipt

A close receipt is a DSSE 1.0 envelope as receipt-1 gives it, payload type `application/vnd.in-toto+json`, with one signature by the mediator. Its statement (`close.json`) has exactly `_type`, `https://in-toto.io/Statement/v1`; `subject`, a list of exactly one entry, `name` `accord` and `digest` an object with exactly `sha256`, the 64 lowercase hex digits of the Accord id, the SHA-256 of the founding set's canonical bytes; `predicateType`, `https://placket.tech/close/0.1`; and `predicate`, with exactly `accord`, the Accord id; `card`, the card hash; `mediator`, the mediator's reference; `at`, its time; `chains`, a list with one entry for each chain of the receipts before it, `from`, `to`, `seq` and `receipt`, that chain's last receipt's `seq` and id, ordered by `from` and then by `to` in actor-reference order; and `unfetched`, a list with one entry for each delivery not yet fetched at the close, `to`, its receiver, and `receipt`, the id of the receipt that queued it, in the order queued. Both lists are there when empty. A close has no `seq`, `prev`, `kind`, `envelope`, `verdict`, `reason` or `by`: it is in no chain and belongs to no question. Its id is the typed hash of its statement's canonical bytes.

#### Scenario: the anatomy of a close

- Given any close receipt
- When its payload is decoded
- Then its `predicateType` is `https://placket.tech/close/0.1`, its one subject is the Accord by the hex of its id, and the statement validates against `close.json` and is refused by `statement.json`

#### Scenario: every chain and every delivery not fetched

- Given a journal with three chains and a delivery never fetched
- When the Accord is closed
- Then the close is written at once, its `chains` names each chain's last receipt in pair order, and its `unfetched` names the delivery's receiver and the receipt that queued it

### close-3 A closed Accord

A closed Accord never reopens, and nothing extends it. A member's give and a member's answer are rejected with `closed`, right after membership, so a stranger is `not-a-member` still and learns nothing; nothing is written. No question is open, so an answerer is shown none (maybe-1), and the passing of time writes nothing. A fetch still hands out each delivery queued for the member, once, in order; once its queue is empty, every fetch returns the close receipt. A mediator restarted on a closed record comes up closed and writes nothing. Closing it again is refused.

#### Scenario: a give and an answer after the close

- Given a closed Accord
- When a member gives and answers, and a stranger does
- Then the member's give and answer are `closed`, the stranger's `not-a-member`, and nothing is written

#### Scenario: fetches after the close

- Given a closed Accord with one delivery queued for the applier
- When the applier fetches three times
- Then the first fetch returns the delivery, and the second and third each return the same close receipt

#### Scenario: a restart on a closed record

- Given a closed record
- When its mediator starts again on it
- Then it comes up closed, writes nothing, and refuses a give with `closed`

### close-4 Verifying against a close

Section 8 reads a close in its rows 1 to 5, and in two rows of its own. Row 8, `close`, over a whole list: the close's `chains` are exactly one entry for each chain of the receipts before it, at that chain's last `seq` and receipt id, in pair order, and no other; each entry of its `unfetched` names a distinct receipt before it that queued a delivery to the entry's `to`, one with verdict `pass` addressed to it, since a verifier cannot see a fetch; the closings with reason `closed` come together right before the close, each at the close's `at`, or end the list, as a close cut short leaves it. A close whose lists or `at` do not hold fails `close` at its index; a closing with reason `closed` followed by any receipt but another such closing or a close fails `close` at that receipt's index. Row 9, `after-close`: a receipt after a close fails at its index. Over a subset, a verifier checks a close's signature, its form and row 9, and of row 8 only that each chain the subset holds ends at the `seq` and receipt the close lists for it. An Accord with no receipts can be closed: its close is the first receipt, with both lists empty.

A whole list that verifies and ends in its close is complete against that close; a subset is never complete, and a list with no close is not. Given also the founding set of a successor Accord, a verifier checks that the successor's policy `supersedes` names this Accord and this list's close; a successor with no `supersedes`, or one that names another Accord or another close, or a list with no close, fails the check. Completeness never claims that the mediator's key signed no other list for the Accord.

#### Scenario: a receipt after the close

- Given a list whose close is followed by one more receipt the mediator signed
- When it is verified
- Then it fails `after-close` at that receipt's index

#### Scenario: a close that does not match its list

- Given, each alone, a close whose `chains` names a `seq` that is not its chain's last, leaves out a chain or names one the list does not hold; whose `unfetched` names a receipt that queued nothing to its `to`, or one receipt twice; whose `at` is not its closings'; and a `closed` closing followed by a give
- When each is verified
- Then each fails `close`, at the close's index, or at the give's

#### Scenario: complete against the close

- Given a whole list that ends in its close, the same list's last two receipts, and a list with no close
- When each is verified
- Then the first is complete against its close, the second is a subset and claims no completeness, and the third claims none

#### Scenario: a successor that pins the close

- Given a closed list and the founding set of a new Accord whose `supersedes` names the old Accord and the list's close
- When the list is verified with the successor's set
- Then it is complete against its close, which the successor pins; and with a `supersedes` that names another close or another Accord, with a successor that supersedes nothing, or with a list that has no close, the check fails
