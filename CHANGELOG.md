# Changelog

Every change to placket that a user can see is listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions will follow [Semantic Versioning](https://semver.org/) from the first tag.

A user-visible change adds its line under `Unreleased` in the same pull request. For an outside contribution, the reviewer or the project's own agents add the line; a contributor is never asked to.

This file covers the tool and its Go API. Changes to the protocol's frozen formats carry their own version notes, in the register (`spec/frozen-formats.md`) and the vectors' notes (`vectors/README.md`).

## Unreleased

### Added

- `placket version` prints which build a binary is: its version as Go stamped it, the commit and its time, the Go release and the decision module; `--json` prints the same as one object.
- `docs/adr/0016` to `0019`: verified receipts, the door's own code, releases and `placket version`, and `Reload`'s order.
- The door replies `door-internal` under its `door` member when it fails itself (its salt source, encoding a reply, or an error the mediator does not type), never the mediator's `internal`.
- `key-mode`: a key file that another user can read or write (any of mode 0077), or that another user owns, is refused when it is loaded. A key file that is not a regular file is `key-read`.
- Two checks in `make ci`: `recipes-core-only`, and action pins in `machine-rules-in-ci`.
- `LICENSE`: placket's code and its requirement text are under the Apache License 2.0.
- `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue forms and a pull request template, for outside contributors and security reporters.
- `docs/security/threat-model.md`: what placket protects, its security properties, assumptions, non-goals and the limits accepted for this version.
- `docs/rules/`: every rule placket's code keeps, each under its public name, with `docs/style/` and `docs/review.md`.
- Two checks in `make ci`: `no-hidden-text` and `agents-md-short`.

### Changed

- `placket key` writes a key file atomically: to a temporary file, synced, then linked into place, so a crash never leaves half a key. It still never overwrites an existing file.
- `bind` requires `--policy FILE`, and an Accord's policy gives each principal the roles it may grant (decision record 0014).
- Text and flags that are not valid UTF-8 are refused, never repaired: `give`, `answer` and `change --text` give tool error `text`, `decline` and `change --comment` give `comment`, and `give --to`, `--kind`, `--re` and `--topic` and `bind --actor`, `--role` and `--mediator` give `usage`.
- `--accord` must be an Accord id, `sha256:` and 64 lowercase hex characters, before any path is built from it; otherwise `usage`.
- `give --as raw --text` is `usage`: `--text` gives a text part only.
- `verify --founding` on a journal with no receipts checks the founding set and that its mediator is `--mediator`, with no Accord to compare, where it gave `error founding` before.
- The client refuses a reply longer than `MaxReply`, four times `MaxCall`, with its `reply` error, and the window sends none: past `MaxReply` the questions reply shows a text or data part by its size alone, and leaves out the newest questions that do not fit even so. A journal, state file or key file placket reads must be a regular file, and each line of a journal or state file is read with a cap of `MaxReply`.
- Every door format, handed file and reply is read strictly: a repeated member, ill-formed UTF-8 or a member name in another case is refused.
- `receipts.Verify` returns `[]receipts.Verified`, `receipts.Releases` takes it, and `receipts.Build` returns the receipt and its `Verified` statement in place of its id (decision record 0016).
- `mediator.Reload` takes its data first, then the times and the key: `Reload(founding, list, open, deliveries, foundedAt, key, at)` (decision record 0019).
- `reconcile` refuses a kept actor that is not an actor reference, before anything else, and never prints it.
- A state error's text is `state: <code>`; the path is no longer in it.
- A refusal from a check of `make ci` now ends by linking the rule's heading, `(docs/rules/<file>.md#<rule>)`, in place of `(AGENTS.md, <rule>)`.
- `AGENTS.md` is a short entry file that points to the rules.
