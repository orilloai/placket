# Modules and boundaries

What may import what, which modules exist, and which packages the core holds. Dependencies point one way: `cmd` to `adapters` to `core`, and `recipes` to `core`. `docs/adr/0001-repository-layout.md`, `docs/adr/0002-core-boundaries.md` and `docs/adr/0009-the-root-module.md` record why.

## `core-imports`

Nothing under `core/`, test files included, imports `github.com/orilloai/placket/adapters`, `/cmd` or `/recipes`, or anything below them.

Why: the core is what a vector can test with no process around it; one import upward brings the world in with it.

Checked by: `make ci`, step `core-imports`; the reviewer reads the imports too.

## `core-no-deps`

`core/go.mod` has no `require`. A dependency in the core needs a decision record first, naming the code path that uses it.

Why: every byte the core writes must come from code the protocol stands behind, at a version that cannot drift.

Checked by: `make ci`, step `core-no-deps`.

## `root-module`

`adapters/` and `cmd/` live in one root module, `github.com/orilloai/placket`, which requires the core module and points that requirement at `./core` with a `replace` until the core is first tagged. Nobody commits a `go.work` file.

Why: one module for everything that touches the world, and a build that works the same from a fresh clone.

Checked by: `make ci`, step `root-module`. The root module comes with its own decision record.

## `recipes-core-only`

A recipe uses only the core's exported names, never an adapter.

Why: a recipe is one job built on the protocol, and must work against any door.

Checked by: `make ci`, step `recipes-core-only`, which refuses a file under `recipes/` that imports anything under `adapters/` or `cmd/`.

## `packages-by-adr`

A package beyond the planned map, a second role package, or a capability split across packages, needs a decision record before the code.

Why: the package map is what the reviewers and the vectors are organised by.

Checked by: the reviewer.

## `package-per-capability`

Each protocol capability has one package of the same name in `core/`, and the package comment names the requirement ids it implements.

Why: a reader goes from a requirement to its code, and back, without a search.

Checked by: the reviewer.
