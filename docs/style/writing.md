# Writing style

How placket's prose is written: the requirement text, these docs, comments, commit messages, pull requests and every message the tool prints.

## The house voice

- Plain words. Prefer the short common word: "use", not "utilize"; "start", not "initiate".
- Short sentences, one idea each, in the active voice: "The door refuses the call", not "The call is refused by the door".
- The product's words, as the requirement text spells them: Accord, part, receipt, window, door, journal, pass, block, ask, release. A word the text defines means only that; never a second name for the same thing (`spec-words`).
- Say what is true now. A limit is stated as a limit; a plan is marked as a plan.
- No marketing: no "seamless", "powerful", "blazing", no exclamation marks, no promises the code does not keep. Placket makes no claim about what a receiver does with content after receipt (`claims-stop-at-crossing`).
- Sentence-case headings. Lists for things that are parallel; prose for reasons.
- Code, paths, commands, flags and rule names in backquotes: `make ci`, `core/`, `core-no-world`.
- Times are absolute dates, such as 2026-11-17, never "last week".

## Where it applies

- **The requirement text** names behaviour, never Go identifiers (`specs-name-behaviour`).
- **Comments** say why, not what the next line does.
- **Commit messages** say what changed and why, in placket's own words: a summary line under about 72 characters, a blank line, then the reason.
- **Messages the tool prints** start with what happened and, for a refusal, the code; they never quote a body, a key or a salt (`error-text-no-content`). The words for outcomes and claims are defined once (`product-words-once`).

## The tie-breaker

Where this file is silent, the [Google developer documentation style guide](https://developers.google.com/style) decides.
