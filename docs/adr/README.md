# Decision records

A record is a decision, with its context and its consequences. The first fourteen state what held at placket v0.1.0, one subject each, and each reads on its own. Each record from 0015 on is a later decision, and names on its `Status:` line the records it changes.

Each record has one area: layout, the repository and its modules; core, the Go core's packages; process, how a change is made and checked; formats, the frozen formats and what they carry; adapters, the door, the journal and the command line; verify, what a verifier checks and says; release, how placket is built and released.

A new record takes the next free number, from 0015 on, and adds its row here. A record from 0015 on that changes an earlier one names it on its own `Status:` line, and the earlier record's `Status:` line names it back. To learn what holds, read a record and then each later record its `Status:` line names; where they differ, the later one holds.

| Record | Title | Area |
|---|---|---|
| [0001](0001-the-repository.md) | The repository: its layout, its modules and one normative source | layout |
| [0002](0002-the-core.md) | The core: no dependency, no world, its packages and its names | core |
| [0003](0003-the-conformance-runner.md) | The conformance runner | core |
| [0004](0004-how-a-change-is-checked.md) | How a change is checked: one checker, rules in topic files, red-green tests | process |
| [0005](0005-names-and-numbers.md) | Names and numbers: placket.tech, one protocol number, salts and roles | formats |
| [0006](0006-founding.md) | Founding: a principal per party, sealed terms and signed views | formats |
| [0007](0007-the-card-and-the-kernel-hash.md) | The card and the kernel hash | formats |
| [0008](0008-the-mediator-and-reload.md) | The mediator and reload | core |
| [0009](0009-who-decided.md) | Who decided: the answer call and judges | formats |
| [0010](0010-the-close-and-the-successor.md) | The close and the successor | formats |
| [0011](0011-the-door-and-the-journal.md) | The door, the journal and the state directory | adapters |
| [0012](0012-the-mediator-api-and-the-command-line.md) | The mediator API, placket mcp and the command line | adapters |
| [0013](0013-what-a-verifier-checks-and-says.md) | What a verifier checks and says | verify |
| [0014](0014-releases-and-versions.md) | Releases and versions | release |
| [0015](0015-no-payload-signature-under-a-placket-uri.md) | No payload signature under a placket URI | core |
