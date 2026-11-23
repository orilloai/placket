# 0009. Who decided: the answer call and judges

Status: accepted. Date: 2026-11-23.

## Context

A receipt that says what was decided, and not who decided it, leaves a reader unable to tell a decision the terms made from one a person made, or which person, without trusting the mediator's word. An answer the mediator alone records leaves the answerer nothing to show later: the answerer should sign its answer itself, under a salt of its own, so that every party the answer reaches can show the answerer answered this exact text.

Accords also need judges where the terms would ask a person: a model behind a network service that passes or blocks a part. A judge is slow, it fails, its output can be steered by the content it reads, and the content it reads leaves the machine. The core cannot run one (`core-no-world`), and the window's loop, which owns the mediator (`one-mediator-goroutine`), cannot wait on one without stalling every other party. The formats freeze at the first release, so who decided, the answer call and the judges' format had to be settled before it.

## Decision

### Every receipt names its decider

- Every predicate holds `by`, required, in exactly one form (receipt-11): `"terms"`; `{"judge":"<card name>"}`; `{"answer":"<typed hash>"}`, the typed hash of the answer call's canonical bytes, `sig` included; or `{"mediator":"<event>"}`, the event one of `deadline`, `undeliverable` and `close`. The list of events is closed.
- The verifier's statement row refuses a `by` that does not fit its receipt's kind, verdict and reason, by receipt-11's table; a judge's block must name, in `by`, the judge its reason names. The release row refuses a crossing receipt an answer decided that is no question's release, and a release whose `by` is not its closing's. An accept or a change that composition then blocks is still the answer's: its reason shows what the terms did.
- No receipt holds a probability, a score or a threshold.

### The answer call

- **An answer is one signed call,** a frozen format (`spec/schema/answer.json`, maybe-11): `{"accord","action","digest"?,"from","question","release"?,"sig"}`, signed under its own type URI, `https://placket.tech/answer/0.1` (signing-4). `digest` and `release` are present exactly for an accept and a change. The answerer mints the answer's 32-byte salt and signs the answer's salted digest, never its text; the text and the salt travel beside the call. The call has no time and no nonce: a question closes once, so a call sent again is `no-open-question`.
- **A cancel is the asker's,** signed by the asker. An answerer's cancel, and the asker's cancel of a question no member can answer, are `not-an-answerer`. That question ends by the mediator's finding or its deadline.
- **The answer's check table** (section 6) is five rows: member, answer call (`answer-call`), open question, answerer by action, answer body (`answer-body`). The call is checked right after membership, as the door authenticates before it reads, so maybe-7's rule that a non-answerer learns only that no question is open still holds. A salt of the wrong length, no salt, an empty version and a digest that does not match are `answer-body`.
- **Where the call goes.** The call's digest is on the resolution receipt and on the release receipt. The call travels inside every delivery its answer queues, as the delivery's optional `answer`, the released part included, and every scenario vector compares it (maybe-12). The journal keeps every call it accepted, in `answers.jsonl` (record 0011), with no text and no salt.
- **What the command line checks before it signs.** Before it signs an accept, a change or a decline, `placket` reads the question the window shows and checks that the ask's receipt names it, under this Accord, and that the body under the salt makes the receipt's digest; a part that does not is tool error `asked`, and nothing is signed or sent. What it does with a question the window does not show is record 0012's. It does not check the mediator's key: the window is local, and the Accord id pins the founding set. `placket questions` makes the same check and prints only the questions that pass it, ending in tool error `asked` when it left one out; what it prints of each is a raw part's kind and size, never the salt or a raw body, and its `--json` entries carry `receiptId` and `receipt`.
- `released` does not read a kept answer call: `answer --keep` and `change --keep` keep it in `kept.jsonl`, and `placket log --kept` reads it for its `answer_signed` column (record 0011).

### Judges, as a format of the terms

- **A maybe term's `resolve` may list judges**, in order, each `{"name","use"}` with use `decide` or `advise`, no two of one name. The card's `judges` is the ordered list of the judges a term may grant, each pinned by its `name`, `version` and `hash`, which covers its configuration; one name with two builds is `invalid-shape`. Founding's row 8 is `judge-unknown`, a judge a term names that the card does not. A term has no `checks`: a term with `checks` is `term-invalid`. The reason `judge:<name>`, on a crossing block only, names a judge's block.
- **Composition takes a judge's judgment as an input**: the judge's card name and its verdict (composition-7). Its row runs after the maybe rules and before the ask: when the judges both sides consented to list that judge with use `decide`, its pass is `pass ok` and its block `block judge:<name>`. Consent is decided in one place, composition: the judges hold only when every matching term of both sides is a maybe and all of them list the same judges, the same names in the same order with the same uses. A want beside a maybe, a maybe on one side alone, and two maybe terms whose judges differ leave the part to be asked, and composition-5 compares maybe rules without their judges, so two such terms are asked, not blocked. A part the mediator formed is never judged, and neither is a release.

### The core gains two reads, and decides nothing new

- **`mediator.Judges`** says which judges a give would reach. It runs every rejection row `Give` runs before composition, in `Give`'s order (membership, then `closed`, then the door's rows), and then `composition.Decide` with no judgment. It returns true only when that verdict is `escalate` and the agreed rule lists judges. `Judging` holds the part's kind, the form of its body, the body as `Question` reads it and the agreed rule's judges in order. It writes nothing. Consent is decided once, in composition: the mediator matches no term and compares no list (`one-decision-module`, `reserved-evaluate`). `Give` runs admission again when the judgment comes back, so a give whose state changed meanwhile is refused as it would be anyway.
- **`setup.Accord.JudgeKinds`** returns the sorted, distinct kinds of every term, of any member, whose `resolve` lists the judge. Only a `maybe` term has a `resolve`, so it compares no stance. It is what the window prints at start: a superset of what can reach the judge, since both sides must agree.

### The judge seam is in the adapters

- **`adapters/judge`** is provider-neutral. It holds a judge's config (`Config`, read by `ReadConfig`), the interface `Judge`, a request and an answer, the stack of a rule's judges (`Stack`, `Configured`), the runners `Run` and `Advise`, and the record of each run (`Record`, written by `Line`). `Budget` is 20 seconds; `MaxOutput` is 64 KiB. Its `*Error` carries one code per cause.
- **`adapters/judge/jev`** is the one adapter of this version, behind `judge.Judge`, on `net/http` alone. Swapping providers is one new adapter package: the runner, the core and every format stay as they are.
- **`adapters/judge/eval`** reads a file of labelled cases and runs them against a judge, and **`cmd/judge-eval`** is its thin binary, run by `make judge-eval`. A subcommand of `placket` would need a row in the protocol's command table, so the eval is a second binary, outside `make ci` and outside the release. The cases live in `evals/judge/`.

### A judge's config is a door format, hashed into the card

- A config is one canonical JSON object of exactly `name`, `provider` (`jev`), `host`, `model`, `threshold` and `instruction`. Its typed hash (canonical-3) is the card's hash for that judge, and `jev.Version` is its version, so changing any byte of the config, the instruction included, is a new card and a new Accord. `ReadConfig` checks, in this order: the canonical form (`config-canonical`, a fraction included, which the canonical form refuses), the members (`config-shape`), the provider (`config-provider`), the host (`config-host`: a DNS name with an optional port, never a scheme, a path, a user or an IP literal) and the threshold, a whole number from 0 to 100 (`config-threshold`). Its callers trim one trailing line feed first. The format is not frozen.
- `placket card --default --judge FILE...` lists the configs in flag order in the card's `judges`. `placket window --judge FILE...` compares them with the card at start, before anything is written to the journal: every judge the card lists needs exactly one config whose hash is the card's and whose version is `jev.Version`, and every config must be one the card lists. Otherwise the window prints `error founding judge` and starts nothing. A window that cannot take a give, `window close` and a window on a closed journal, skips the comparison and needs no `--judge`.

### The runner: decide off the loop, then write the receipt

- When a give would be asked and its agreed rule lists a `decide` judge, the loop marks the caller busy, starts `judge.Run` on a goroutine it owns, and goes on serving. `Run` asks the `decide` judges in order within one budget for the whole stack, and settles on one judgment: the first pass or block, or none. Any error, an answer below the config's threshold, and a judge still running at the budget are none. The goroutine hands the judgment back on a channel; the loop calls `Give` with it and with the time the door read when the give arrived (`door-owns-clock`), journals the receipt, writes the run's records, replies and clears the busy mark. A `Give` that then rejects drops the records. A raw body is never sent: each listed judge's record is none, `raw-body`, and the give is asked.
- `advise` judges run after the receipt is written, for every give whose agreed rule lists one, whatever decided it, but never on a raw body. Each runs within its own budget on an owned goroutine, its records come back on the same channel and are appended, and the window prints `advise <name> <pass|block|none> <receipt id>`. An advise judge decides nothing and delays nothing.
- Stopping the window cancels every judge run; `Run` returns once every goroutine has ended, and nothing more is written. A judged give cut so gets no receipt.

### The judge's output is content, kept apart

Each judged receipt's records go in `judged/<hex of the receipt id>.jsonl` in the journal directory, mode 0600, one canonical line per judge run: `{"abstained"?,"error"?,"judge","outcome","output"?,"outputBase64"?,"score"?,"use"}`. Output that is not valid UTF-8 is written as standard base64 in `outputBase64`. The decide lines are written after the receipt and before the reply; the advise lines are appended later. A failed append is a failed journal write (`journal-synced`). Nothing of a judge goes in a receipt beyond its name in `by` and `judge:<name>`, and no reader of receipts reads `judged/`: `journal.Read`, reload, `verify`, `log`, `export` and `reconcile` never open it, and removing it changes nothing they print.

### The key, and what leaves the machine

The Jev key is the user's own credential. It is read once, from `PLACKET_JEV_KEY`, when the window or the eval starts, held only in memory, and written nowhere. With no key, the adapter sends nothing and the judge says none. A request carries the part's kind, its body (a text's text, a data body's canonical JSON) and the config's instruction, with the config's model, the key as its credential and the adapter's fixed words, the same in every request: the path `/v1/systemone`, and one question, `verdict`, of type `choice`, between two criteria, `pass`, "The content meets the instruction.", and `block`, "The content does not meet the instruction."; and nothing else of the part, the message, the envelope or the Accord. It goes over HTTPS, with verified TLS, to the config's host only: the transport uses no proxy from the environment and follows no redirect, and a reply is read up to `MaxOutput`, strictly. At start the window prints, for each card judge, which kinds may go to which host and model, or that it sends nothing.

### Three rules, and their reasons

The rules' texts are in `docs/rules/`; their reasons are these.

- **`answer-call-attached`**: the call travels inside every delivery its answer queues, and the journal keeps each accepted call, with no body and no salt, so every party an answer reaches, and the operator's journal, can show who answered, and a verifier can say an answer is witnessed.
- **`one-decision-module`**: a recorded verdict is never computed again, with one exception: `core/conform` decides again, and only when the card's kernel hash is the one the verifier trusts. The reason a verdict is not computed twice, that the module may change between processes, does not apply there: the module that decides again is the one trusted for the card's kernel hash.
- **`door-owns-clock`**: the door reads the clock once per call, at whole seconds; a salt is minted with `crypto/rand` by the party that gives or answers, and the door mints none. An answerer that minted its own salt can show what it signed.

### Its names

**`answer`** (maybe-11, section 6)

- `Call` and `Read`: what an answer call says, and its reader, which checks its bytes, shape and signature.
- `Call.Accord`, `Call.Question`, `Call.Action`, `Call.Digest`, `Call.Release` and `Call.From`: the Accord's id, the question's id, the action, for an accept or a change the salted digest of the answer and whether it asks for release, and the signer's reference.
- `Sign`: a call's canonical bytes, signed by the key.
- `ID`: a call's typed hash, the value of a receipt's `by.answer`.
- `TypeURI`: the call's type URI, `signing.TypeAnswer` written once (`constants-once`).
- `Action`, with `Accepted`, `Changed`, `Declined` and `Cancelled`: the four actions, spelled as the vector format spells them.
- `Code`, with `CodeCall`, and `Error` and `Error.Error`: a call that does not read, `answer-call`, with nothing of the call.
- `Error.Code`: the code of a call that does not read.

**`signing`**

- `TypeAnswer`: the answer call's type URI.

**`composition`**

- `Judgment`: a judge's verdict on a crossing, by its card name, or none.
- `Judgment.Name` and `Judgment.Verdict`: the judge's card name, and its verdict, `Pass` or `Block`.
- `Crossing.Judge`: the judgment a judge gave the crossing, or nil.
- `Decision.Judge`: the name of the judge that decided, empty when the terms decided.
- `ReasonJudge`: the reason of a judge's block, `judge:<name>`.

**`setup`**

- `Judge` and `Use`, with `Decide` and `Advise`: a term's grant of a judge, and the two uses.
- `Judge.Name` and `Judge.Use`: the judge's card name, and the use the term grants it.
- `Rule.Judges`: the judges a maybe term's resolve lists, in order.
- `CardJudge` and `Accord.CardJudges`: an entry of the card's judges, and the card's judges in order.
- `CardJudge.Name`, `CardJudge.Version` and `CardJudge.Hash`: a judge's name, and the version and hash that pin its build.
- `Accord.JudgeKinds`: the kinds whose terms list a judge.
- `JudgeUnknown`: the founding code of a judge a term names that the card does not.

**`receipts`**

- `By` and `Decider`, with `ByTerms`, `ByJudge`, `ByAnswer` and `ByMediator`: a predicate's decider and its four forms.
- `By.Decider`, `By.Judge`, `By.Answer` and `By.Event`: the decider, and exactly the one field it names: the judge, the answer call's typed hash, or the mediator's event; none for the terms.
- `Predicate.By`: the predicate's decider.
- `Event`, with `EventDeadline` and `EventUndeliverable`: what made the mediator decide.

**`mediator`**

- `Mediator.Judges` and `Judging`: which judges a give would reach, and what they would be shown.
- `Judging.Kind`, `Judging.Form`, `Judging.Body` and `Judging.Judges`: the part's kind, the form of its body, the body as `Question` reads it, and the agreed rule's judges in its order.
- `Question.Salt` and `Question.Receipt`: the asked part's salt, and the ask's receipt, whose digest is the part's under that salt, so an answerer checks what it answers.
- `Judging.Format`, `Judging.GoString` and `Judging.String`: a `Judging` prints as a fixed text.

## Consequences

- A third party given an Accord id, its founding set and a journal can say which answers are witnessed by their answerer's own signature (record 0013).
- A party that fetched an answer, or a part an answer released, holds the answerer's call; the mediator's journal holds every accepted call; neither holds an answer's text or salt beyond what was delivered.
- A judged give takes up to the budget to answer, and its receipt's `at` is when it arrived, so its receipt can follow one with a later `at`.
- A judge's pass is a model's. Text inside the part can steer it. The terms bound where a judge may act, and the eval measures how often an injection passes; neither proves it cannot.
- Model output may quote the body, and lies in clear in `judged/`, which may be removed at any time.
- What a provider keeps of a request is the provider's terms, not a claim of placket's.
- A second provider is a new adapter package, a new provider value in `ReadConfig`, and the window's version check against that adapter's version.
