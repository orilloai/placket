# Decision records

A record is a decision as it was made, with its context and its consequences. A later record may amend it: it then names the earlier record on its own `Status:` line, after "Amends", "Changes" or "Supersedes", and the earlier record's `Status:` line ends "Amended by" with the records that amend it. So to learn what holds now, read a record and then each record its "amended by" column names, in number order; where they differ, the later one holds.

Each record has one area: layout, the repository and its modules; core, the Go core's packages; adapters, the door, the journal and the command line; formats, the frozen formats and what they carry; verify, what a verifier checks and says; release, how placket is built and released; process, how a change is made and checked. Every record is accepted, and none is superseded whole. The columns "amends" and "amended by" hold only what the records' `Status:` lines name.

A new record takes the next free number, adds its row here, and adds its number to the `Status:` line of each record it amends.

| Record | Title | Area | Status | Amends | Amended by |
|---|---|---|---|---|---|
| [0001](0001-repository-layout.md) | The repository layout | layout | accepted |  | 0003, 0023, 0024, 0027 |
| [0002](0002-core-boundaries.md) | The core is a module of its own, with no dependency and no world | core | accepted |  | 0007, 0015 |
| [0003](0003-one-normative-source.md) | Placket's behaviour has one normative source | layout | accepted | 0001 |  |
| [0004](0004-the-conformance-runner.md) | The conformance runner is a package of the core, and reads the schema itself | core | accepted |  |  |
| [0005](0005-the-core-packages.md) | The core's packages and every name they export | core | accepted |  | 0012, 0016, 0020, 0021, 0022, 0023, 0025 |
| [0006](0006-the-mediator-package.md) | The mediator holds an Accord's state and runs its operations | core | accepted |  | 0020, 0021, 0022, 0023, 0025 |
| [0007](0007-one-checker.md) | The checks that read the tree are one Go program, proved by its own Go tests | process | accepted | 0002 | 0013, 0015, 0020 |
| [0008](0008-reload.md) | Reload rebuilds a mediator from its record and closes what was left open | core | accepted |  | 0019, 0020, 0021, 0022, 0025 |
| [0009](0009-the-root-module.md) | The root module holds the adapters and the binary | layout | accepted |  | 0016, 0021, 0023, 0024, 0027 |
| [0010](0010-the-door.md) | The door: calls on a local socket, the window that answers them, and the answer call | adapters | accepted |  | 0017, 0020, 0021, 0022, 0023, 0024, 0025 |
| [0011](0011-the-journal.md) | The journal, the state directory and the log | adapters | accepted |  | 0021, 0022, 0023, 0025 |
| [0012](0012-the-kernel-hash.md) | The kernel hash, the default card, and a card of another module | formats | accepted | 0005 | 0020, 0021, 0023 |
| [0013](0013-red-green-tests.md) | Tests are shown red against main and green at the head, however the branch was built | process | accepted | 0007 | 0015 |
| [0014](0014-a-principal-per-party.md) | A principal per party, scoped to the roles the policy gives it | formats | accepted |  | 0020 |
| [0015](0015-rules-in-topic-files.md) | The rules live in topic files, and AGENTS.md is a short entry file | process | accepted | 0002, 0007, 0013 |  |
| [0016](0016-verified-receipts.md) | Verified receipts have a type of their own | verify | accepted | 0005, 0009 |  |
| [0017](0017-the-doors-own-code.md) | The door's own failures have a code of their own, and a reply has a size limit | adapters | accepted | 0010 | 0021 |
| [0018](0018-releases-and-version.md) | Releases are signed without a key and rebuild to the same bytes, and `placket version` names a build | release | accepted |  |  |
| [0019](0019-reload-order.md) | Reload takes its data first, then the times and the key | core | accepted | 0008 | 0025 |
| [0020](0020-placket-tech-and-fixed-salts.md) | Every placket URI under placket.tech and one protocol number, salts of exactly 32 bytes, and A2A's meaning of role | formats | accepted | 0005, 0006, 0007, 0008, 0010, 0012, 0014 | 0021, 0025 |
| [0021](0021-who-decided-and-the-answer-call.md) | Every receipt names its decider, an answer is one signed call, judges are a format of the terms, and verify checks conformance to the Accord | formats | accepted | 0005, 0006, 0008, 0009, 0010, 0011, 0012, 0017, 0020 | 0022, 0024, 0025, 0026, 0028, 0029, 0031 |
| [0022](0022-the-close.md) | An Accord is closed by a signed close receipt, and a verifier can say a closed log is complete | formats | accepted | 0005, 0006, 0008, 0010, 0011, 0021 | 0025, 0028, 0030 |
| [0023](0023-judges.md) | Judges run in the window, off its loop, and the core takes their judgment as an input | adapters | accepted | 0001, 0005, 0006, 0009, 0010, 0011, 0012 | 0024 |
| [0024](0024-the-mediator-api.md) | The mediator API, the client, and placket mcp | adapters | accepted | 0001, 0009, 0010, 0021, 0023 | 0025, 0029 |
| [0025](0025-sealed-terms.md) | Each party's terms are sealed in the founding set, and each party signs its view before the Accord is founded | formats | accepted | 0005, 0006, 0008, 0010, 0011, 0019, 0020, 0021, 0022, 0024 | 0026 |
| [0026](0026-verify-against-a-view.md) | A member checks its own receipts against the view it signed | verify | accepted | 0021, 0025 | 0028 |
| [0027](0027-placket-init.md) | `placket init` founds an example Accord, and a test runs each first hour | adapters | accepted | 0001, 0009 |  |
| [0028](0028-what-verify-says.md) | What verify, released and show say is exactly what they checked | verify | accepted | 0021, 0022, 0026 | 0031 |
| [0029](0029-help-and-answers-the-window-does-not-show.md) | `--help` prints usage, and an answer the window does not show goes to the window | adapters | accepted | 0021, 0024 |  |
| [0030](0030-the-successor-a-reader-names.md) | The successor check takes the reader's id and the successor's acceptances, and founds at the close's time | verify | accepted | 0022 | 0031 |
| [0031](0031-what-a-body-an-empty-journal-and-the-claim-prove.md) | What a body check, an empty journal and verify's claim line prove | verify | accepted | 0021, 0028, 0030 |  |
| [0032](0032-a-receipts-payload-is-its-canonical-bytes.md) | A receipt's payload is its canonical bytes; its envelope is read loose | verify | accepted |  |  |
