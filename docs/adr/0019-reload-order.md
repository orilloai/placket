# 0019. Reload takes its data first, then the times and the key

Status: accepted. Date: 2026-11-17. Changes one signature of record 0008. Amended by record 0025.

## Context

Record 0008 gave `mediator.Reload` the signature `Reload(founding []byte, foundedAt string, key signing.Key, list [][]byte, open []Open, queued []Queued, at string)`. `inputs-as-parameters` asks that a function whose output is hashed or signed take its time, salt and key after the data. `Reload` signs the closings it writes, and its time and key came between the data.

## Decision

`func Reload(founding []byte, list [][]byte, open []Open, deliveries []Queued, foundedAt string, key signing.Key, at string) (*Mediator, []Outcome, error)`. The data come first, then the times and the key. What it checks, in what order, and what it writes do not change.

`receipts.Digest(salt, body)` keeps its order, for the reason record 0016 gives.

## Consequences

- One exported signature changes before the first tag, with a `CHANGELOG.md` line (`api-compat`). Its one caller, the door, changes with it.
