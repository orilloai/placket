# Security policy

Placket signs receipts that parties rely on as evidence, so we take reports of weaknesses in it seriously. This file says how to report one, what is in scope, and what happens next.

## Reporting a vulnerability

Report privately. Never open a public issue, pull request or discussion about a vulnerability.

- **GitHub private vulnerability reporting** is the channel: the "Report a vulnerability" button under the repository's Security tab.
- **Email security@orillo.ai** if you cannot use GitHub.

Include what you can: the version or commit, what you ran, what happened, what you expected, and the smallest input that shows it. A report that a person has checked is worth more than a long one. A report written by a tool that nobody has verified is closed without action.

## Supported versions

| Version | Supported |
|---|---|
| `main` | yes |
| the latest release | yes |
| anything older | no |

## In scope

- Bypassing verification: a receipt, envelope, declaration, binding or call accepted when its signature or its key should refuse it.
- Ambiguity in the canonical form or a hash: two different values with the same canonical bytes or digest, or one value with two.
- Forging or reordering a chain of receipts so that verification passes.
- Bypassing the authentication of the local socket: a call that reaches the mediator without a valid signature from a member.
- Exposing key files: a key written, logged or printed where another user or process can read it.
- Denial of service through the parsers: input that makes a parser crash, hang or use memory far out of proportion to its size.
- A judge's request whose content is more than the part's kind, its body and the instruction, or that carries anything beside them but the config's model, the adapter's fixed words (the path `/v1/systemone`, and one question, `verdict`, of type `choice`, between two criteria, `pass`, "The content meets the instruction.", and `block`, "The content does not meet the instruction.") and the key as its credential; or any content reaching a host the window did not print at start (see "What a judge sends").
- The judges' key written to disk, the journal, `judged/` or any other file, or printed.

## What a judge sends

A `maybe` term may let a judge decide or advise where the terms would ask a person. In v0.1.0 the one judge provider is Jev, called with the user's own key. For each part a judge runs on, the window sends one HTTPS request to the path `/v1/systemone` on the host of the judge's config, carrying the part's kind, its body and the config's instruction, with the config's model, and the key as the request's credential. Beside them it carries the adapter's own fixed words, the same in every request: one question, `verdict`, of type `choice`, between two criteria, `pass`, "The content meets the instruction.", and `block`, "The content does not meet the instruction." Nothing else of the part, its message, its envelope or the Accord goes, and a raw body is never sent. With no key, the judge sends nothing and says none. At start the window prints which kinds may go to which host.

The judge key is the user's own credential. Placket reads it only from the environment variable `PLACKET_JEV_KEY`, once, when the window or the eval starts. It holds the key only in memory, and never writes it to disk, the journal, `judged/` or any other file. `docs/security/threat-model.md`, "The Jev data flow", says more.

## Out of scope

- A compromised host, or an attacker with root on it.
- The machine's own user set: anyone who runs as the window's user can already read its keys and journal.
- The known limits accepted for this version, listed in `docs/security/threat-model.md`, and its non-goals, including what a receiver does with content after receipt.

## What happens next

We follow the timelines of the Go project's own security policy.

- **Acknowledgement within 7 days.** If you have heard nothing after 7 days, follow up on the same channel, and check your spam folder for our reply.
- **Fixed or made public within 90 days** of our acknowledgement. We agree the date of disclosure with you, and publish an advisory with the fix.
- There is no separate deadline for triage: we tell you what we found as soon as we know.

## Coordinated disclosure and safe harbour

We ask you to keep the report private until the fix is out or the 90 days have passed, whichever is first.

We will not pursue or support legal action against research done in good faith under this policy: research that avoids harm to others, uses only accounts, keys and data that are yours or that you have permission to use, does not degrade a service for others, and reports what it finds to us privately and promptly. If in doubt, ask us first at security@orillo.ai.

## Credit

We credit every reporter by name in the advisory and in `CHANGELOG.md`, unless you ask us not to.
