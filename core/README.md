# core

Placket's core: the Go module `github.com/orilloai/placket/core`. It has no dependency, imports nothing from the rest of the repository, and never touches the world: no clock, randomness, environment, file, network or printing. What it needs from the world it takes as parameters.

It holds no package yet; the first lands in M1. See `docs/adr/0002-core-boundaries.md` for why it is built this way, and `AGENTS.md` for the rules `make ci` checks here.
