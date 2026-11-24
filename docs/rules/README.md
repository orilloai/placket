# The rules

Every rule of placket has a short public name, and this is the one place they all appear together. Each rule lives once, under its name, as a heading of one topic file below, so `core.md#core-no-world` is a stable link. Under each heading: the rule, a `Why:` line and a `Checked by:` line.

A check in `make ci` starts its message with the rule's name, and its last line links the rule's heading, so `core-no-world: core/x/x.go:12: ...` points straight at it. A reviewer's finding cites the name too. Rules for parts not built yet are stated now, and each applies from the first change that builds its part. A rule a machine can check is checked by `make ci`, or, for `red-green-tests` and `outside-commits-signed-off`, by a job of its own beside it; the rest are the reviewer's.

A new rule gets its name here, in the change that adds it: a heading in its topic file and a line in this index. `make ci` (`agents-md-short`) refuses a heading missing from the index, a name with no heading or two, and a link to a heading that does not exist.

## Modules and boundaries

- [`core-imports`](modules.md#core-imports)
- [`core-no-deps`](modules.md#core-no-deps)
- [`root-module`](modules.md#root-module)
- [`recipes-core-only`](modules.md#recipes-core-only)
- [`packages-by-adr`](modules.md#packages-by-adr)
- [`package-per-capability`](modules.md#package-per-capability)

## The core

- [`core-no-world`](core.md#core-no-world)
- [`core-no-panic`](core.md#core-no-panic)
- [`mediator-no-logic`](core.md#mediator-no-logic)
- [`trace-new-names`](core.md#trace-new-names)
- [`specs-name-behaviour`](core.md#specs-name-behaviour)
- [`bytes-at-boundaries`](core.md#bytes-at-boundaries)
- [`inputs-as-parameters`](core.md#inputs-as-parameters)
- [`spec-words`](core.md#spec-words)
- [`results-are-copies`](core.md#results-are-copies)
- [`secrets-never-print`](core.md#secrets-never-print)
- [`constants-once`](core.md#constants-once)
- [`parse-strict`](core.md#parse-strict)
- [`verified-by-type`](core.md#verified-by-type)
- [`crypto-from-stdlib`](core.md#crypto-from-stdlib)
- [`one-decision-module`](core.md#one-decision-module)
- [`release-in-resolve`](core.md#release-in-resolve)
- [`reserved-evaluate`](core.md#reserved-evaluate)

## Errors

- [`one-error-type`](errors.md#one-error-type)
- [`errors-as-codes`](errors.md#errors-as-codes)
- [`rejection-vs-error`](errors.md#rejection-vs-error)
- [`error-text-no-content`](errors.md#error-text-no-content)
- [`sentinels-uncoded`](errors.md#sentinels-uncoded)
- [`check-order`](errors.md#check-order)

## The door, the journal and the command line

- [`adapters-no-recheck`](adapters.md#adapters-no-recheck)
- [`adapters-core-types`](adapters.md#adapters-core-types)
- [`main-is-thin`](adapters.md#main-is-thin)
- [`call-signing`](adapters.md#call-signing)
- [`one-mediator-goroutine`](adapters.md#one-mediator-goroutine)
- [`journal-synced`](adapters.md#journal-synced)
- [`door-owns-clock`](adapters.md#door-owns-clock)
- [`door-check-order`](adapters.md#door-check-order)
- [`door-passes-bytes`](adapters.md#door-passes-bytes)
- [`state-lock`](adapters.md#state-lock)
- [`journal-is-evidence`](adapters.md#journal-is-evidence)
- [`files-like-siblings`](adapters.md#files-like-siblings)
- [`restart-reproducible`](adapters.md#restart-reproducible)
- [`log-from-receipts`](adapters.md#log-from-receipts)
- [`product-words-once`](adapters.md#product-words-once)
- [`claims-stop-at-crossing`](adapters.md#claims-stop-at-crossing)
- [`answer-call-attached`](adapters.md#answer-call-attached)
- [`wait-and-result-shared`](adapters.md#wait-and-result-shared)
- [`keys-on-disk`](adapters.md#keys-on-disk)
- [`goroutines-owned`](adapters.md#goroutines-owned)

## Tests and vectors

- [`tests-exact`](tests.md#tests-exact)
- [`refusals-tested`](tests.md#refusals-tested)
- [`output-modes-tested`](tests.md#output-modes-tested)
- [`raw-bytes-in-hex`](tests.md#raw-bytes-in-hex)
- [`test-helpers-placed`](tests.md#test-helpers-placed)
- [`vectors-independent`](tests.md#vectors-independent)
- [`runner-no-silent-pass`](tests.md#runner-no-silent-pass)
- [`coverage-reported`](tests.md#coverage-reported)
- [`schemas-hold`](tests.md#schemas-hold)
- [`fuzz-parsers`](tests.md#fuzz-parsers)
- [`red-green-tests`](tests.md#red-green-tests)

## How changes are made

- [`change-is-one-capability`](changes.md#change-is-one-capability)
- [`deltas-stop-for-intent`](changes.md#deltas-stop-for-intent)
- [`decisions-recorded-first`](changes.md#decisions-recorded-first)
- [`format-change-noted`](changes.md#format-change-noted)
- [`api-compat`](changes.md#api-compat)
- [`machine-rules-in-ci`](changes.md#machine-rules-in-ci)
- [`ci-stays-fast`](changes.md#ci-stays-fast)
- [`no-hidden-text`](changes.md#no-hidden-text)
- [`outside-commits-signed-off`](changes.md#outside-commits-signed-off)
- [`docs-tree-holds`](changes.md#docs-tree-holds)
- [`agents-md-short`](changes.md#agents-md-short)

## What the checks print and miss

`checks.md` says what `make ci` prints, and what the checks that read the tree can and cannot see.

## Code principles

- The core is pure: its outputs are a function of its inputs. The world is the adapters' job.
- Bytes are the truth at every boundary a verifier crosses. Decode for convenience in the adapter.
- Errors are codes, never quotes of the input.
- No panic on input, anywhere in the core.
- One definition of every constant, word and check.
- Build what the requirement says, at the altitude it says. No speculative interfaces: v1 adds no interface for the decision module and no second module.
