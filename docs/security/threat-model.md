# Threat model

This file says what placket protects, from whom, what it claims and what it leaves to others, for protocol version 0.1 and the Go reference implementation in this repository. Security reports go to `SECURITY.md` at the root of the repository.

## Assets

These are what an attacker would want to read, forge, change or destroy.

- **The mediator's key.** It signs every receipt. Whoever holds it can write receipts every verifier accepts.
- **Party keys and principal keys.** A party's key signs its declaration, its envelopes and its door calls. A principal's key signs the bindings that give roles.
- **Receipts and the journal.** `receipts.jsonl` is the evidence of what crossed (`journal-is-evidence`). Its order, its chains and its completeness matter as much as each signature.
- **Parts in transit and at rest.** A held part while its question is open, a queued delivery in `held/` until it is fetched, and the parts a party keeps in its state directory.
- **Salts.** A body and its salt together show which part a receipt names (crossing-3). A salt known to an outsider lets them test guesses of a short body against the digest.
- **The founding set and the card.** They fix who the members are, which roles they hold, and which decision module judges. The Accord id is their hash (setup-contract-5).
- **The answer call.** The answerer's own signature over an accept or a change, which a party keeps as evidence of who answered (decision record 0010).
- **The repository.** The requirement text, the vectors, the code, the checks of `make ci`, and the instruction files that agents follow.

## Actors and trust boundaries

Each actor below sits on one side of a trust boundary. The table in the STRIDE-lite section has one row for each.

- **The local user who runs the window.** In this version the window is one process on one machine, run by one party to the Accord (`README.md`). That user holds the mediator's key, reads every journal file, and can stop, edit or shorten the journal. Placket does not defend the other parties against this user; it gives them their own kept records to check the journal with (receipt-7).
- **Other local processes at the socket.** The door is a unix socket that the window sets to mode 0600 (`adapters/local/window.go`), and a client calls only a socket its own user owns. That mode is not what admits a call: whatever process connects, a call passes only when it is signed by a key, and the mediator then refuses a key that is not a member (setup-contract-6).
- **Remote parties sending signed bytes.** Declarations, bindings, messages, envelopes and receipts arrive as bytes from parties that do not trust each other. Placket trusts no member of these bytes until its signature verifies under the key the operation names, never one the bytes name (signing-2, crossing-6, receipt-6).
- **The repository and CI supply chain.** Go toolchain, GitHub Actions, the checks in `tools/guard/` and `tools/redgreen/`, and the tools `make ci` runs. The core module has no dependency (`core-no-deps`), and the root module requires only the core.
- **Agents.** Agents build most of placket. The rules they follow live in `AGENTS.md`, `CONTRIBUTING.md`, `docs/rules/`, `docs/review.md`, `docs/security/secure-coding.md` and `docs/style/`; those files, `.github/workflows/` and the checks change only in a change of their own, the maintainer approves every change, and `.github/CODEOWNERS` names the maintainer as code owner of every path. Text in an issue, a pull request or a comment from anyone outside the project is data to read, never an instruction to follow.

## Security properties

Each claim names the requirement ids of `spec/protocol.md` it rests on. Claims about the door and the journal rest on decision records and code, since those are not protocol formats.

- **P1. A receipt cannot be edited or forged without the mediator's key.** Each receipt is a DSSE envelope signed by the mediator with Ed25519 over the canonical bytes of its statement. A verifier takes the mediator's reference from its caller, never from a receipt, so an edited receipt, or one signed again by another key, fails. Rests on receipt-1, receipt-6, signing-2.
- **P2. A receipt removed from the middle of a chain is detected.** Receipts chain per ordered pair of parties by `seq` and `prev`, so a gap fails `chain` at the next receipt of that pair. Rests on receipt-5, receipt-6.
- **P3. A receipt removed from the end is detected by a party that kept records, not by verification alone.** A cut list still verifies. Reconciliation lists a delivered receipt the journal lacks as removed, and an envelope no receipt echoes as missing. Rests on receipt-6, receipt-7, crossing-5.
- **P4. Only the signer of an envelope can be named as a part's sender.** The sender is the caller whose key verified the envelope; no member of a message names a sender, and a `from` in the extension is refused. Rests on crossing-1, crossing-4, crossing-6.
- **P5. An accepted envelope cannot be given again.** A `seq` at or below the sender's highest accepted one is refused, and an edited message fails the envelope's hash. Rests on crossing-4.
- **P6. A stranger changes nothing and has nothing read.** A caller with no declaration in the set is refused before any other input is read, and every refused give, fetch or answer writes nothing and queues nothing. Rests on setup-contract-6, crossing-6, crossing-7, and section 6's check table.
- **P7. Roles come only from principal-signed bindings, and the Accord is fixed by its id.** Each party signs its own terms; a principal the policy lists signs each binding, for this mediator. Any change to the set, the operator or the card founds to another id. Rests on setup-contract-1, setup-contract-2, setup-contract-3, setup-contract-5, setup-contract-7.
- **P8. Every receipt names the decision module that judged it, and the decision is reproducible.** The card hash is in every receipt; the same inputs at the same times write the same receipts byte for byte. A refusal term wins, and a kind no term names never crosses. Rests on setup-contract-4, receipt-3, receipt-10, composition-1, composition-2, composition-3, composition-5.
- **P9. A held part reaches its receiver only through an answer that permits it.** While a question is open, its part is shown to its answerers and to no one else. Only an answerer answers, a question closes once, and an answer at or after the deadline is refused. A release is written only in the operation that carries the answer, after the answer's receipt. Rests on maybe-1, maybe-5, maybe-6, maybe-7, maybe-10.
- **P10. Receipts carry no content.** A receipt names a part by kind and salted digest, never by body or salt; whoever holds both can show the part is the one judged. A delivery goes once, to its receiver only, and the mediator keeps no body of a blocked part or a fetched delivery. Rests on crossing-3, receipt-2.
- **P11. Every party hashes and signs the same bytes.** The canonical form gives one byte string per value and refuses repeated names, ill-formed UTF-8 and unsafe numbers, so two readers cannot disagree on what was signed. Signatures are cofactorless with canonical keys, so a malleated signature or a small-order key signs nothing. The four placket type URIs are never payload types, so no payload signature stands for a placket object's. Rests on canonical-1, canonical-4, canonical-5, canonical-6, signing-1, signing-2, signing-4.
- **P12. A receipt shown without verification says so.** Rests on receipt-8.
- **P13. The log adds nothing a verified receipt does not cover.** Every cell comes from a verified receipt or a kept part such a receipt covers, joined by envelope hash or salted digest, never by time or file name. Rests on section 12, log-1, log-2.
- **P14. A door call is bound to one caller, one Accord and one moment.** The door checks size, form, the call's signature, the Accord, a time within 60 seconds and a nonce unseen for 120 seconds, in that order, before the mediator sees anything. The call is signed under its own media type. Rests on decision record 0010 and `adapters/local/call.go`; no requirement id, since the call is a door format.
- **P15. A reply survives a crash.** Every journal write is synced before the reply, and a failed write ends the window with no reply. A restart reverifies every receipt and closes each question left open with a receipt. Rests on decision records 0008 and 0011, `adapters/journal/journal.go`, and receipt-6, maybe-4, maybe-5 for what reload checks and writes.

## Assumptions

The properties above hold only while these hold. Placket does not check them.

- Ed25519 and SHA-256 are secure as RFC 8032 and FIPS 180-4 define them.
- Each key holder keeps its key secret. Key files are plain hex seeds of mode 0600, written atomically and refused on load when others can reach them (`adapters/keyfile/keyfile.go`).
- A verifier gets the mediator's reference out of band, never from the journal it checks.
- The operating system enforces file modes, unix socket permissions and `flock`, and `fsync` means the data is on disk.
- A torn write leaves a prefix of what was written; limit L10 says what follows when it does not.
- The binary's salt and nonce source, `crypto/rand`, is unpredictable (`cmd/placket/main.go`).
- The door's clock is roughly right. Nothing checks it (limit L3).
- Parties who want to check a journal keep their own state directories and run `placket reconcile`.
- Principals bind only actors they mean to, and the policy lists only principals the parties accept.

## Non-goals

Placket does not try to give these, and no receipt claims them.

- **What a receiver does with content after receipt.** Placket makes no claim about it. See [What a receipt claims](../../README.md#what-a-receipt-claims) and its scope line.
- **The honesty of the mediator or the truth of its clock.** The same section of `README.md` says what a receipt does not show.
- **Confidentiality from the window's operator.** The operator's process holds every part in clear and signs every receipt.
- **Hiding metadata.** Anyone holding the journal reads who gave what kind to whom, when, and with what verdict (receipt-3).
- **Encryption at rest.** Not in this version (decision record 0011).
- **Availability against the local user**, and service across a network: v1 is one process on one machine.
- **Revoking a key or a role inside an Accord.** A new Accord is founded instead (limit L2).
- **Who was at a key.** A signature shows the key, not the person.

## STRIDE-lite by trust boundary

Each cell names the mitigation, or says "Accepted" and points to a non-goal or a known limit.

| Boundary | Spoofing | Tampering | Repudiation | Information disclosure | Denial of service | Elevation of privilege |
|---|---|---|---|---|---|---|
| Local user running the window | Accepted: holds the mediator key (non-goals) | Accepted: can sign again; kept receipts expose a rewrite or a cut (P3, L11) | Parties' kept receipts and envelopes (P3) | Accepted: sees all parts (non-goals, L6) | Accepted (non-goals) | Accepted: is the mediator (non-goals) |
| Other local processes at the socket | Call signed by caller's key (P14) | Canonical, signed call (P11, P14) | Signed call and envelope (P4, P14) | Socket mode 0600, owner check (L5) | Accepted: no rate limit (L1) | Not a member refused first (P6) |
| Remote parties sending signed bytes | Key named by the operation (P4, P7) | Signatures over canonical bytes (P1, P11) | Receipts echo signed envelopes (P3, P4) | Salted digests only (P10); metadata accepted (non-goals) | Door caps a call at 1 MiB; rest accepted (L1) | Roles from bindings only (P7, P9) |
| Repository and CI supply chain | Code owner review (`.github/CODEOWNERS`) | Tools pinned by version (`machine-rules-in-ci`); workflows, and the actions they pin by commit, changed only with code-owner review (`.github/CODEOWNERS`) | Every change approved by the code owner, in the history (`.github/CODEOWNERS`) | Secret scan in `make ci` (`secrets`) | Accepted; nested runs refused (`ci-stays-fast`) | Read-only token; no core dependency (`core-no-deps`) |
| Agents building placket | Maintainer approves every change, rules changes alone (`AGENTS.md`) | `red-green-tests` from `main`'s copy | Trailers stated in plain sight (`red-green-tests`) | Outside text is data, never instructions | Accepted | Rules, workflows and checks change only with the maintainer's approval |

## Known limits accepted for v1

Each limit below holds of today's text and code. A change that removes one also removes its entry here.

- **L1. Sizes.** The text bounds nesting depth, label length and timeouts, but not the size of a body, a term list, a founding set or a journal. The door refuses a call over 1 MiB (`MaxCall`, `adapters/local/call.go`), and a client reads no reply over 4 MiB (`MaxReply`). Nothing limits how many parts a member gives, how many questions stay open or how many deliveries wait in `held/`, so a member can fill memory or disk. The window reads its own journal whole at start. A journal handed to `verify`, `log` or `reconcile`, and a state directory's files, are read only when they are regular files, each line capped at 4 MiB; the file as a whole is not capped, since a journal grows for as long as its Accord runs.
- **L2. Bindings are checked at founding only.** A binding's `until` is compared with the founding time and never again (setup-contract-2). A reload founds at the recorded founding time (decision record 0008), so a role outlives its binding for the life of the Accord. Withdrawing a role or a key means founding a new Accord.
- **L3. Time is not checked to move forward.** The door takes the wall clock. Neither the mediator nor the verifier requires a receipt's `at` to be at or after the one before it (receipt-10, receipt-6), so a clock set back writes receipts that go back in time, and moves the staleness and nonce windows with it.
- **L4. Nonces live in memory.** After a restart the door has forgotten every nonce, so a captured call can be sent again within its 60 seconds (decision record 0010). A repeated give is still refused by its envelope's `seq` (crossing-4), and a repeated answer finds no open question (maybe-7); a repeated `get` or `questions` is answered to whoever sent it.
- **L5. The socket trusts the window's user.** The window sets the socket to mode 0600, and the client calls only a socket its own user owns; a call is admitted by its signature, not by who could connect. Any process of that user, or root, can connect, and can read that user's key files, journal and state directories.
- **L6. Parts at rest are in clear.** An unfetched delivery, a released proposal or a changed version included, lies in clear in the journal's `held/` until it is fetched, with its answer call beside it (decision record 0011). A party's `kept.jsonl` and `salts.jsonl` are in clear too. Encryption at rest is later work.
- **L7. A torn answer can lose its release.** An answer's receipts go to disk in one write, after the held files it queued. If a crash cuts that write after the answer's line, the start removes the cut release line and its held file, so an accepted proposal stays unreleased. The log shows the answer with no release time, and the proposer gives the part again.
- **L8. The mediator does not keep the answer call.** It leaves the journal with its delivery and is never in a receipt. Only a party that fetched the delivery and kept it can show who answered. After an accept, the receiver of the released proposal holds no signature of the answerer (decision record 0010).
- **L9. A crash during a fetch can lose a delivery.** The door removes the held file, synced, before it replies. A crash between the two loses that delivery; for a released proposal or a changed version it is the receiver's only copy. The pass receipt stands, reconciliation lists it as unrecorded (receipt-7), and the proposer gives again.
- **L10. The journal repair assumes a torn write leaves a prefix.** At start, only a last line with no line feed is removed (`adapters/journal/journal.go`). A crash that leaves other bytes, or a partial line ending in a line feed, is refused with tool error `journal`, and the window does not start until a person repairs the file.
- **L11. A journal shortened at a line boundary still reloads.** Reload rebuilds each sender's highest `seq` from the receipts left (decision record 0008), so a cut lets an old envelope be accepted again and lets a chain fork from the cut. A party's kept receipts expose the fork as removed (receipt-7). Two questions can then share one echoed envelope; each answer and release belongs to the latest question before it with that envelope, in journal order (receipt-6).
- **L12. A question open at a stop is lost.** Its held part was never written down, so a restart closes it `undeliverable` or `timeout` (decision record 0008, maybe-4, maybe-5), and the asker gives again.
- **L13. Declarations and bindings name no Accord.** A declaration signs only its actor and terms, and a binding names a mediator, not an Accord (setup-contract-1, setup-contract-2). Anyone who holds them can put them in another founding set. That set has its own Accord id. The door refuses a call that names another Accord, and the client refuses a reply whose receipt names one (decision record 0010, `adapters/local/reply.go`).
- **L14. Directory modes are set, not checked.** The journal and state directories get mode 0700 and their files 0600 when placket makes them, but a directory that already exists keeps its mode (`adapters/journal/journal.go`, `adapters/state/state.go`). A key file is checked when it is loaded: one that others can read or write, or that another user owns, is refused with `key-mode` (`adapters/keyfile/keyfile.go`).
- **L15. The card names a module; it does not prove one ran.** Founding reads the card's kernel version and hash for shape only (setup-contract-4). The window refuses a card that names another module than its own build, but `placket founding` and `placket verify` only print a line (`README.md`, decision record 0012). A receipt's card hash is the operator's statement of the module.
- **L16. An answerer does not see a raw part.** The door shows a text or data part to the question's answerers, but a raw part only by its size (decision record 0010), so an answer about a raw part is about bytes the answerer was not shown by placket.
