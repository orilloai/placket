# 0030. The successor check takes the reader's id and the successor's acceptances, and founds at the close's time

Status: accepted. Date: 2026-11-22. Amends record 0022. Amended by record 0031.

## Context

Record 0022 lets a verifier check a closed journal against a successor Accord: `placket verify --superseded-by NEW` founded the successor's set and checked that its policy's `supersedes` names the old Accord and the journal's close. The complete line then said that the successor pins the close. Three things made that line claim more than the check read:

- The successor was whatever set the verifier was handed. Its id was computed from that file, so the check said only that the file and the journal agree. Whoever holds the old mediator key can sign a whole other journal with another close, and found a set of their own whose `supersedes` names that close. Handed both, the check passed.
- No acceptance was read. A founding set is only a proposal until every member has accepted its own view (setup-contract-15), so a set no member accepted pinned the close as well as one every member had.
- The successor was founded at the verifier's wall clock. A binding's `until` is checked against the founding time, so the same files gave a pinned line one day and a `founding` error the next, once a binding of the successor had run out.

The rotation guide had faults of the same kind: its successor's files sat beside the old Accord's, so the successor's window read the old `acceptances.json`; one inline command was refused as written; and the successor's window ran in an unlabelled block that its test never ran.

The check's flags and its printed line freeze with the first release, so they are settled before it.

## Decision

`verify --superseded-by FILE` takes two more flags, both required with it: `--successor-accord ID`, the successor's Accord id, and `--successor-acceptances FILE`, the successor's acceptances. The three are given together; one or two of them alone are the command's error `usage`, after the line `usage: placket verify: --superseded-by, --successor-accord and --successor-acceptances are given together`. `--superseded-by` still needs `--accord` and `--founding`.

The id is the reader's own: the id it accepted, or one it recorded when the successor was founded. The docs say so wherever they name the flag. The acceptances are never read from beside the successor's set: `verify` reads only the files it is given.

The check runs after validity, in this order:

1. The set's typed hash is the id given, else tool error `verify superseded`. Bytes the canonical form refuses have no id, so they fail here.
2. The list ends in a close, else tool error `verify superseded`, as before. This comes before founding because founding needs the close's time.
3. The set founds with the acceptances by founding's rows 1 to 21, at the `at` of the close, else tool error `founding` with the founding code, such as `acceptance-missing`, `acceptance-accord` or `binding-expired`. No opening is given, so row 21 does not run: the check is that every member accepted the successor, not what its terms say.
4. The successor's `supersedes` names this Accord and this close, else tool error `verify superseded`, as before.

The founding time is the close's `at`, the moment the successor takes over. A successor whose bindings outlast the close pins it on any day the check runs, and one whose bindings ended at or before the close never does. A founding set carries no time of its own, and an acceptance holds exactly `accord`, `view`, `from` and `sig`, so the close's `at` is the one time in the inputs that names the hand-over.

The printed line keeps its words, `complete: against close receipt <close>, which successor accord <id> pins; not claimed: that the mediator signed no other log for this Accord`, and `verify --json` keeps its members. With the reader's id and the acceptances checked, the line is true as written. The check has no conformance vector: it is a command's behaviour, tested by the command's Go tests and `tests/test_rotation.sh`.

The rotation guide is rewritten once. The successor lives in a directory of its own. The order is: close the old Accord; found the successor's set; run the successor's window until every member has accepted; check the old journal against the accepted successor; retire the old key. Every command is in a labelled `sh` block, and `tests/test_rotation.sh` runs every block in order, refuses an unlabelled fence, and refuses a command named with its flags in the prose that the guide does not run.

## Amendments

- **Record 0022, the close.** "What complete claims, and what it does not" now reads: `placket verify --superseded-by NEW --successor-accord ID --successor-acceptances FILE` checks that NEW's typed hash is ID, founds it with its acceptances at the close's `at`, and checks that its `supersedes` names this Accord and this log's close. A set that is not the one ID names, a log with no close, a successor that supersedes nothing, and one that names another Accord or another close are tool error `verify superseded`. A successor that does not found is tool error `founding` with its code.

## Consequences

- A verifier who checks a journal against the successor it accepted learns that the close it pins is the one every member of the successor accepted. A successor founded by whoever holds the old key, under an id the reader never accepted, pins nothing.
- The check gives one result for a fixed set of files on any day.
- A verifier needs one more file and one more value than before. A script that ran `--superseded-by` alone now gets `usage`.
- The founding time of a journal with no receipt still differs between `verify` and `conform.Check`; that is not this record's to settle.
- A close the members countersign, and outside witnessing of chain heads, are still later work (threat model L19).
