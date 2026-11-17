# Changelog

Every change to placket that a user can see is listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions will follow [Semantic Versioning](https://semver.org/) from the first tag.

A user-visible change adds its line under `Unreleased` in the same pull request. For an outside contribution, the reviewer or the project's own agents add the line; a contributor is never asked to.

This file covers the tool and its Go API. Changes to the protocol's frozen formats carry their own version notes, in the register (`spec/frozen-formats.md`) and the vectors' notes (`vectors/README.md`).

## Unreleased

### Added

- `LICENSE`: placket's code and its requirement text are under the Apache License 2.0.
- `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`, issue forms and a pull request template, for outside contributors and security reporters.
- `docs/security/threat-model.md`: what placket protects, its security properties, assumptions, non-goals and the limits accepted for this version.
- `docs/rules/`: every rule placket's code keeps, each under its public name, with `docs/style/` and `docs/review.md`.
- Two checks in `make ci`: `no-hidden-text` and `agents-md-short`.

### Changed

- A refusal from a check of `make ci` now ends by linking the rule's heading, `(docs/rules/<file>.md#<rule>)`, in place of `(AGENTS.md, <rule>)`.
- `AGENTS.md` is a short entry file that points to the rules.
