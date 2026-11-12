# core

Placket's core: the Go module `github.com/orilloai/placket/core`. It has no dependency, imports nothing from the rest of the repository, and never touches the world: no clock, randomness, environment, file, network or printing. What it needs from the world it takes as parameters.

It holds one package so far, `conformance`, the conformance runner, which reads the vectors under `vectors/` and runs each through the core (`docs/adr/0004-the-conformance-runner.md`). The capability packages land from M1. See `docs/adr/0002-core-boundaries.md` for why it is built this way, and `AGENTS.md` for the rules `make ci` checks here.
