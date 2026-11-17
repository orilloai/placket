# 0014. The rules live in topic files, and AGENTS.md is a short entry file

Status: accepted. Date: 2026-11-17.

## Context

`AGENTS.md` held everything a builder or a reviewer needed: the layout, every rule with its reason and its check, the code principles, how a change is made and reviewed, what `make ci` prints and what the checks miss. It had grown to about 30 KiB and 164 lines, and every agent loads it on every task, whatever the task touches.

That shape has three costs. Some harnesses stop reading instruction files at 32 KiB and drop the rest without a word. An agent pays for every line it reads, and follows long lists of instructions less well than short ones. And the file spoke only to the project's own agents: an outside contributor found no licence, no security policy and no code of conduct.

## Decision

- **`AGENTS.md` is the entry file**, at most 150 lines and 10 KiB. It holds the commands, the map, what is true of every change, a table of what to read before changing each part, the definition of done, and what needs a person. It holds no rule's text.
- **Every rule lives once, in `docs/rules/`**, as a heading that is its public name, in one of seven topic files (`modules.md`, `core.md`, `errors.md`, `adapters.md`, `tests.md`, `changes.md`, and `checks.md` for what the checks print and miss). Under each heading: the rule, a `Why:` line and a `Checked by:` line. `docs/rules/README.md` is the index, the one place every name appears together. The rules moved without rewording.
- **A refusal links its rule.** The last line of a check's refusal ends `(docs/rules/<file>.md#<rule>)`, in place of `(AGENTS.md, <rule>)`, so the reference opens the rule itself. The checker's rule table names each rule's topic file.
- **Two checks hold this in place**, both rules of the checker (record 0007): `agents-md-short` keeps the entry file within its limits, keeps the index and the headings in step, one heading per name, and resolves every link to a rule's heading; `no-hidden-text` refuses a bidi control or an invisible character in any tracked text file outside `testdata/`, and a tracked binary there, since instruction files and code are read by agents and people who cannot see them.
- **Nine rules are added**, each under its own name, for what no linter catches: `parse-strict`, `verified-by-type`, `crypto-from-stdlib`, `keys-on-disk`, `goroutines-owned`, `api-compat`, `fuzz-parsers`, `no-hidden-text` and `agents-md-short`. Existing code is not changed for them in this change.
- **The files an open project is expected to have** come with it: `LICENSE` (Apache-2.0, for the code and the requirement text), `CODE_OF_CONDUCT.md`, `SECURITY.md`, `CHANGELOG.md`, issue forms and a pull request template; and `docs/style/`, `docs/review.md` and `docs/security/`.
- **No nested instruction files and no vendor-specific ones.** Harnesses load a nested `AGENTS.md` at different times, or not at all, so a pointer in the entry file is the one that works everywhere.

This supersedes two sentences of record 0007: a rule's text now goes in its topic file of `docs/rules/`, not in `AGENTS.md`, and what the checks still miss is in `docs/rules/checks.md`. It supersedes the pointer in record 0002 the same way: the names of the core's checks, and what each matches, are in `docs/rules/`. Record 0007 is not edited. Record 0002's pointer is edited to name `docs/rules/`, since it says where the names are now. Records 0001, 0003 to 0006 and 0013 lose the words that named internal build stages and an old commit trailer, replaced by plain descriptions; no decision in them changes.

## Consequences

- An agent on a small task reads about 5 KiB, then only the topic file its change touches.
- A new rule gets its name in `docs/rules/`, a heading and an index line, in the change that adds it; `agents-md-short` refuses one without the other.
- Anything that printed or matched `(AGENTS.md, <rule>)` now reads `(docs/rules/<file>.md#<rule>)`. The workflow's comments still say `AGENTS.md`; they change with the next workflow change.
- `red-green-tests` runs `main`'s copy of `tools/redgreen/`, so its refusal keeps the old reference until this change has merged.
