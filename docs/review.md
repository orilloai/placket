# How a change is reviewed

How a reviewer judges a change to placket, and what a finding looks like. The rules a change keeps are in `docs/rules/`, each under its public name. The maintainer, as code owner, approves every change; a review comes before that approval.

## Who reviews, and against what

- A reviewer did not write the change. A review pass runs to its stop rule without pausing to check in.
- The reviewer reads the diff against the rules in `docs/rules/`, `CONTRIBUTING.md` and the decision records in `docs/adr/` the change touches.
- `make ci` passing is the floor, not the verdict. The reviewer reads what no machine checks: the imports of every file changed in `core/`, where logic sits, whether each test compares exactly, and whether each new exported name traces to a requirement or a decision.
- A change touches no check, workflow or rule unless it is that kind of change (`CONTRIBUTING.md`, the kinds of change). A change to `make ci`'s checks outside a rules change is a finding.

## The tests

- The reviewer reads `red-green-tests`'s report. They judge every trailer's reason, each test red only by not building (red (build)), each changed test green on base, and each changed helper.
- The reviewer checks that no test was weakened, skipped or deleted to pass.
- A reviewer of a code change runs mutation testing: they break lines on purpose and report which broken lines the tests did not notice.

## A finding

Every finding names three things:

- the rule's public name, such as `core-no-world`, linked to its heading where it helps;
- the file and line;
- the input or scenario that fails, so the writer can reproduce it.

`core-no-world: core/x/x.go:12: reads time.Now; a vector run twice gets two receipts` is a finding. "This looks fragile" is not.

A finding a linter or a check of `make ci` would raise is not raised by hand: fix the check instead, in a rules change.

## Severities

Each finding carries one of three severities.

- **blocking:** a broken rule; a correctness, security or compatibility problem; or a missing or weakened test. The change does not merge until it is fixed.
- **should-fix:** fixed before merge, unless the writer says why not, in the pull request. The reviewer may accept the reason.
- **nit:** a matter of taste or polish. It never blocks, and the writer may ignore it.

A security problem that is not yet public is reported as `SECURITY.md` says, never as a finding on a public pull request.

## When to stop

When findings reach diminishing returns, the review stops. What is left goes to a follow-up, named in the pull request.

## The changelog

A user-visible change has a line in `CHANGELOG.md`. When the writer is an outside contributor and the line is missing, the reviewer, or the project's own agents, add it. An outside contributor is never asked to.

## A change nobody answers for

Every contribution needs a person who answers for it and can explain it (`CONTRIBUTING.md`). When a change has none, or its author cannot explain it, every reviewer, the project's agents included, sends the same reply, once:

> Thank you for this. Before we review it further, we need someone who answers for the change: please check it yourself, make sure you can explain each part of it, and reply to the review questions. If nothing changes, we will close this pull request. You are welcome to open it again, or a new one, at any time.

If nothing changes, the pull request is closed. The same reply, with "issue" for "pull request", goes on an issue written by a tool and checked by nobody. A security report of that kind is closed without action, as `SECURITY.md` says.
