# 0021. Every receipt names its decider, an answer is one signed call, judges are a format of the terms, and verify checks conformance to the Accord

Status: accepted. Date: 2026-11-17. Amends records 0005, 0006, 0008, 0010, 0011 and 0012.

## Context

Placket 0.1 is not released, and its formats freeze at the release. Four gaps in them would each cost a new protocol version if closed after it:

- **A receipt said what was decided, not who decided it.** A receipt of a give, of an answer and of a deadline looked alike in this respect. A reader could not tell a decision the terms made from one a person made, and could not tell which person, without trusting the mediator's word.
- **An answer was a door call beside a delivery.** The answerer signed its door call, which held the answer's text, and the door attached that call beside the deliveries whose body was the answer. The call was not a frozen format, no receipt named it, the released part carried none, and the journal dropped it once a delivery was fetched. The mediator, not the answerer, minted the answer's salt, so the answerer signed text it could not later show under a salt of its own.
- **The terms could not grant a judge.** The card and the term held reserved slots for checks, and a reason `check:<name>` was read for shape only. Later work runs judges, models among them, where the terms would ask a person. Without a format for which judge a term grants, and for a judge's decision in a receipt, that work would change the formats.
- **Verifying checked signatures and chains, never the terms.** A verifier could say that the receipts were the mediator's, not that each decision was one the Accord allowed.

## Decision

### Who decided

Every predicate holds `by`, required, in exactly one form: `"terms"`; `{"judge":"<card name>"}`; `{"answer":"<typed hash>"}`, the typed hash of the answer call's canonical bytes, `sig` included; or `{"mediator":"<event>"}`, the event one of `deadline` and `undeliverable`. The verifier's statement row refuses a `by` that does not fit its receipt's kind, verdict and reason, by a table in receipt-11; a judge's block must name, in `by`, the judge its reason names. The release row refuses a crossing receipt an answer decided that is no question's release, and a release whose `by` is not its closing's. An accept or a change that composition then blocks is still the answer's: its reason shows what the terms did. No receipt holds a probability, a score or a threshold. The mediator's values are an enumerated list, so a close, when it comes, adds its own value as a format change of its own.

### The answer call

An answer is one signed call, a frozen format (`spec/schema/answer.json`, maybe-11): `{"accord","action","digest"?,"from","question","release"?,"sig"}`, signed under a fourth object type URI, `https://placket.tech/answer/0.1` (signing-4). `digest` and `release` are present exactly for an accept and a change. The answerer mints the answer's 32-byte salt and signs the answer's salted digest, never its text; the text and the salt travel beside the call. The call has no time and no nonce: a question closes once, so a call sent again is `no-open-question`. A cancel is the asker's, signed by the asker; an answerer's cancel, and the asker's cancel of a question no member can answer, are `not-an-answerer`. That question ends by the mediator's finding or its deadline, as before.

The answer's check table is five rows: member, answer call (`answer-call`, new), open question, answerer by action, answer body (`answer-body`, new). The call is checked right after membership, as the door authenticates before it reads, so maybe-7's rule that a non-answerer learns only that no question is open still holds. The three misuse codes of the old answer, `action`, `salt` and `answer`, are retired: an unknown action cannot be signed into a call that reads, and a salt of the wrong length, no salt, an empty version or a digest mismatch are `answer-body`. Salts of a give, of a reload and of the export keep the codes record 0020 gave them.

The call's digest is on the resolution receipt and on the release receipt. The call travels inside every delivery its answer queues, as the delivery's optional `answer`, the released part included, and every scenario vector compares it (maybe-12). The journal keeps every call it accepted, in `answers.jsonl`, one canonical call a line, appended and synced before the receipts of its closing; it holds no text and no salt. A line no receipt names, as after a crash between the two writes, is read as evidence by no one: verify counts it, and a closing whose line is missing reads as unwitnessed. A start removes a cut last line of `answers.jsonl`, as it does for the journal's other appended files, so that the next call appended is a whole line.

### What an answerer is shown, and what it checks

The door's `questions` reply carries, for each open question an answerer may answer, the ask's receipt, the part's salt and its body in the form it was given, a raw body included, as standard base64. This keeps the salt rule of `docs/security/secure-coding.md`: the reply goes to an answerer of the open question that holds the part. The reply holds as many whole entries as fit under `MaxReply`, oldest first, and says `"more":true` when it left some out; there is no cursor, so a later question shows once earlier ones close. Before it signs an accept, a change or a decline, `placket` reads the question the window shows and checks that the ask's receipt names it, under this Accord, and that the body under the salt makes the receipt's digest; a question the window does not show, and a part that does not, is the new tool error `asked`, and nothing is signed or sent. It does not check the mediator's key: the window is local, and the Accord id pins the founding set. `placket questions` makes the same check and prints only the questions that pass it, ending in tool error `asked` when it left one out; what it prints of each is unchanged, a raw part still its kind and size; its `--json` entries add `receiptId` and `receipt`, never the salt or a raw body.

### Judges, as a format

A maybe term's `resolve` may list judges, in order, each `{"name","use"}` with use `decide` or `advise`, no two of one name. The card's `judges` is the ordered list of the judges a term may grant, each pinned by its `name`, `version` and `hash`, which covers its configuration; one name with two builds is `invalid-shape`. Founding's row 8 becomes `judge-unknown`, a judge a term names that the card does not. The check slots go: the term's and the card's `checks`, the reason `check:<name>`, composition's check row and founding's `check-unknown`. A term with `checks` is `term-invalid`, a member a term does not list. The reason `judge:<name>`, on a crossing block only, takes the place of `check:<name>`; composition-4, the checks, is a reserved id.

Composition takes a judge's judgment as an input: the judge's card name and its verdict. A new row runs after the maybe rules and before the ask: when the judges both sides consented to list that judge with use `decide`, its pass is `pass ok` and its block `block judge:<name>`. Consent is decided in one place, composition: the judges hold only when every matching term of both sides is a maybe and all of them list the same judges, the same names in the same order with the same uses. A want beside a maybe, a maybe on one side alone, and two maybe terms whose judges differ leave the part to be asked, and composition-5 compares maybe rules without their judges, so two such terms are asked, not blocked. A part the mediator formed is never judged, and neither is a release. Nothing in this change runs a judge: the window hands the decision no judgment, and only the conformance check builds one, from a receipt, to decide again. Running judges, ordering them and keeping what advise judges say are later work, whose header is not a protocol change.

`composition.KernelVersion` becomes `0.2.0`, since the verdicts change, with the judge row in and the check row out.

### Conformance to the Accord

`verify` checks validity and then conformance (verify-1 to verify-4). It needs the Accord id alone out of band: it takes the mediator's reference from the founding set whose typed hash is that id. Conformance rows run per receipt after validity: the Accord, the parties, the decision decided again, the witness of an answer, and, for one delivery a caller gives, its body. A verifier never runs a judge again and never fetches anything: a judge's outcome is taken as the receipt claims it, and the decision row checks that the Accord let that judge decide. The decision row runs only when the card's kernel hash is the one the verifier trusts to decide again, its own; under another, the row is skipped and the witness and body rows still run. Before an Accord has a close, verify claims nothing about completeness. It can also check a subset of the receipts, whose chains may start late, and one delivery's body and salt.

Two packages hold the new work (`packages-by-adr`):

- **`core/answer`**, the answer call: its type URI, an alias of `signing.TypeAnswer`, written once (`constants-once`); its four actions, which move here from `mediator`; `Sign`, `Read` and `ID`; and its one code, `answer-call`. It is a package of its own because the mediator, the conformance check, the export and the command line all read a call, and none of them is where the call is defined.
- **`core/conform`**, conformance: `Check` over the evidence a verifier is given, and `Body`, the one place the body row runs. It is a package of its own because it calls the decision module, the verifier and the reader of calls, and is none of them.

`receipts.VerifySubset` verifies a subset, and a receipt whose question is not in the subset has the new place `RoleOutside`.

### Smaller formats

- A part may carry `mediaType`, lowercase `type/subtype` with no parameters, at most 127 characters a side, which no term reads and no digest covers, and which its delivery carries. A bad value is `one-body`, the row that reads the part's members.
- A policy may carry `supersedes`, `{"accord","close"}`, two typed hashes: the earlier Accord's id and its close receipt's id, signed through each binding's policy hash. Founding reads it for shape only.

### Three rules rewritten

Each of these rules changes in this change, which a rules change would otherwise carry alone, because the formats they govern change here:

- **`answer-call-attached`** (`docs/rules/adapters.md`): the call travels inside every delivery its answer queues; the journal keeps each accepted call in `answers.jsonl`, with no body and no salt; the call is never in a receipt. It said the call was attached beside a delivery and gone from the journal once fetched.
- **`one-decision-module`** (`docs/rules/core.md`): a recorded verdict is still never computed again, with one exception: `core/conform` decides again, and only when the card's kernel hash is the one the verifier trusts. Record 0008's reason, that the module may change between processes, then does not apply: the module that decides again is the one trusted for the card's kernel hash.
- **`door-owns-clock`** (`docs/rules/adapters.md`): the door still reads the clock once per call, at whole seconds; a salt is minted with `crypto/rand` by the party that gives or answers, and the door mints none. `docs/security/secure-coding.md`'s line that the door mints salts says so too.

## Exported names

**Removed or re-signed**, since none can keep its old signature and none can be deprecated first: Go has no overloads, and an unsigned answer must not be accepted alongside a signed one.

- `local.AnswerCall`, `local.VerifyAnswerCall` and `local.SignedAnswer`: the door's call format they read is gone; readers use `answer.Read`.
- `mediator.Resolution`'s fields `Action`, `Answer` and `Release`: the action and the release come from the signed call. A `Resolution` is now `{Call, Text, Salt}`. Its field `Salt` keeps its name and type, but its meaning changes: it was the salt the mediator's caller minted for the answer, and it is now the salt the answerer minted and signed the digest under.
- `mediator.Give`, which takes the judgment, and `mediator.Resolve`, which takes the caller, the resolution and the time, the question being the call's.
- `local.ResolveArgs`, now the signed call, the text and the salt.
- `journal.HeldFile`, `journal.ParseHeld`, `journal.PutHeld` and `journal.TakeHeld`: a held file has no answer call beside its delivery.
- `export.Build` and `export.BuildAll`, which take the journal's answer calls.

**Deprecated, then removed before v0.1.0**, as record 0020 did with `MinSaltBytes`, so no release carries them: `mediator.Action` and its four constants, aliases of `answer.Action` and its constants; `mediator.CodeSalt`, `CodeAnswer` and `CodeAction`, which nothing returns; `setup.CheckUnknown`, which founding never returns; `setup.Check` and `setup.Term.Checks`, never built and always nil; `local.Window.Rand`, which the window never reads; `local.Reply.AnswerCall` and `local.Reply.Salt`, never set; and `local.Call.Action`, never set, the action being the call's.

**Added**: in `core/signing`, `TypeAnswer`; in `core/answer` and `core/conform`, the packages above; in `core/composition`, `Judgment`, `Crossing.Judge`, `Decision.Judge` and `ReasonJudge`; in `core/setup`, `Use`, `Decide`, `Advise`, `Judge`, `CardJudge`, `Rule.Judges`, `JudgeUnknown`, `Accord.CardJudges`, `Accord.Operator` and `Accord.Supersedes`; in `core/receipts`, `Decider`, `ByTerms`, `ByJudge`, `ByAnswer`, `ByMediator`, `Event`, `EventDeadline`, `EventUndeliverable`, `By`, `Predicate.By`, `Start`, `Subset`, `VerifySubset` and `RoleOutside`; in `core/mediator`, `AnswerCall`, `AnswerBody`, `Question.Salt` and `Question.Receipt`; in `adapters/local`, `Asked.Receipt`, `Asked.Salt`, `Asked.Raw`, `Reply.More`, and `Call.AnswerCall` and `Call.Salt`, the resolve call's new arguments as the door read them; in `adapters/journal`, `FileAnswers`, `Journal.AddAnswer` and `ReadAnswers`.

**Command-line flags** added or changed: `--no-release` on `answer` and `change`, which signs a call that asks for no release; `--keep` on `answer`, which keeps the call the key signed and the salt it minted, as `change --keep` now does beside the version; and `verify`'s `--accord`, `--answers`, `--subset` and `--delivery`. `cancel` is now the asker's command. `verify` prints its counts before the new `valid`, `conforms` and `complete` lines, in place of `verified <n> receipts`, and its `--json` object's `checks` becomes `judges`.

## What it amends

- **0005.** The core's packages gain `answer` and `conform`. `setup.Term.Checks` and `setup.Check` were deprecated, then removed before v0.1.0, and founding's row 8 is `judge-unknown`. `signing` signs four object kinds.
- **0006.** `Resolve` takes a signed call; a `Resolution` holds the call, the text and the salt; the misuse codes `action`, `salt` and `answer` are retired for the rejections `answer-call` and `answer-body`. `Give` takes a judgment.
- **0008.** A reload's closings name the mediator as their decider. The one-decision-module exception above is the conformance check's, not a reload's: a reload still decides nothing again.
- **0010.** The door's `resolve` call carries `{"answer","salt"?,"text"?}`, the signed call and, for an accept or a change, the answer's text and salt; its reply carries no salt; the door mints no salt; a delivery's reply carries no answer call beside it; the `questions` reply's entries carry `receipt`, `salt` and the body in its form, and the reply may say `more`.
- **0011.** The journal gains `answers.jsonl`; a held file is `{"delivery","to"}`, the answer call inside the delivery.
- **0012.** The card's `judges` is the ordered list of the judges a term may grant, with unique names, and the card has no `checks`. The kernel version is `0.2.0`.

## Consequences

- Every receipt, Accord id and vector changes once: every predicate gains `by`, and every card loses `checks`. The demo journal is written again with its `answers.jsonl`, and the README quotes its first receipt again.
- A third party given only an Accord id, its founding set and a journal can say whether each decision was one the Accord allowed, and which answers are witnessed by their answerer's own signature. It cannot say that the journal is complete; a close, later, can.
- A party that fetched an answer, or a part an answer released, holds the answerer's call; the mediator's journal holds every accepted call; neither holds an answer's text or salt beyond what was delivered.
- Judges can be added to an Accord's terms and card now, and run later, with no format change.
- The `questions` reply grows by each part's salt, receipt and body, and lists fewer questions at once when parts are large.
