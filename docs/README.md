# The docs

Placket's documentation, in the order a reader takes it: a quickstart for each entry point, the words placket uses, the guides, the references, how to verify a journal, and what placket protects. Each page is a file of this repository, and placket.tech takes the order of its pages from this list. `make ci` checks that every page listed here exists and is listed under the section its path names, that every link on these pages resolves, and that every `placket` command line on them names a real command and real flags (`docs-tree-holds`, in `docs/rules/changes.md`).

## Quickstarts

- [Approve before apply](quickstarts/approve.md)
- [An out-gate on a merge](quickstarts/merge.md)
- [Handoffs between agents](quickstarts/handoff.md)

## Concepts

- [The Accord](concepts/accord.md)
- [Terms](concepts/terms.md)
- [Ask and answer](concepts/ask.md)
- [The receipt](concepts/receipt.md)
- [The mediator, the window and the door](concepts/mediator.md)
- [The principal](concepts/principal.md)
- [What placket does not do](concepts/limits.md)

## Guides

- [Approve before apply](guides/approve.md)
- [An out-gate on a merge](guides/merge.md)
- [Handoffs between agents](guides/handoff.md)
- [Claude Code over MCP, a worked example](guides/claude-code.md)
- [Judges and shadow mode](guides/judges.md)
- [Several mediators](guides/mediators.md)
- [Writing an applier](guides/appliers.md)
- [Custody](guides/custody.md)
- [Rotating a mediator's key](guides/rotation.md)

## Reference

- [The command line](reference/cli.md)
- [The mediator API](reference/mediator-api.md)
- [The MCP tools](reference/mediator-api.md#the-request-layer)
- [Errors of the mediator API and the MCP tools](reference/mediator-api.md#errors)
- [The protocol](../spec/protocol.md)
- [The schemas](../spec/README.md)
- [The conformance vectors](../vectors/README.md)

## Verify

- [Verify a journal](verify.md)

## Security

- [Threat model](security/threat-model.md)
- [Secure coding](security/secure-coding.md)
