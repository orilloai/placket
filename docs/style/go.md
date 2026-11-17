# Go style

How placket's Go is written. Most of it is decided elsewhere: this file names the guides that decide, says what the tools own, and writes down only what placket adds. The rules that matter for the protocol are in `docs/rules/`, each under its public name.

## The tie-breakers

When two ways of writing something are both fine, these decide, in this order of authority:

1. The Google Go Style Guide: its [guide](https://google.github.io/styleguide/go/guide), then its [decisions](https://google.github.io/styleguide/go/decisions), then its [best practices](https://google.github.io/styleguide/go/best-practices).
2. [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).

A rule in `docs/rules/` wins over both. A reviewer cites the guide's section when a finding rests on it.

## What the tools own

Nobody writes these down again, and a reviewer does not raise them by hand:

- **Formatting** is `gofmt`'s. `make ci` (`modules`) refuses a Go file `gofmt` would rewrite.
- **Vet** is `go vet`'s (`make vet`).
- **Lint** is staticcheck's, at the version the `Makefile` pins (`make lint`).
- **Vulnerabilities** are govulncheck's (`make vulnerabilities`); **committed secrets** are gitleaks's (`make secrets`).

A `//nolint` or `//lint:ignore` directive carries its reason on the same line, in words a reviewer can judge: `//lint:ignore U1000 kept for the vectors' generator`. One with no reason is a finding.

## What placket adds

### Errors

- A package has one error type, `*Error`, whose `Code` is a constant of a stable string type (`one-error-type`).
- A refusal returns a code, never a decoder's error wrapped up. The one wrap allowed is another placket package's `*Error`, found with `errors.As` (`errors-as-codes`).
- A sentinel error is only for a failure that carries no code, and is matched with `errors.Is` (`sentinels-uncoded`).
- Error text never quotes input: no body, salt, key, message or payload (`error-text-no-content`).
- Outside the core, wrap with `%w` only when the caller is meant to match the wrapped error; otherwise format it with `%v`, so it does not become part of the API.

### Doc comments

- Every exported name has a doc comment that starts with its name, as the Go guides say.
- In `core/`, the doc comment names the requirement ids, or the decision record, the name rests on (`trace-new-names`). A package comment names the requirement ids the package implements (`package-per-capability`).
- A comment says why, or what the requirement text calls a thing; it does not restate the code.

### Deprecation

- A name about to go is marked with a paragraph of its own in its doc comment, starting `Deprecated:`, that says what to use instead. Tools and editors read that form.
- A deprecated name stays for at least one tagged release before it is removed, and the removal has a decision record (`api-compat`).
- Before the first tag, an exported name may still change, with a decision record and a `CHANGELOG.md` line.

### Names

- Initialisms keep one case throughout: `ID`, `URI`, `JSON`, `HTTP`, `CLI`; `id` and `uri` when unexported at the start of a name. Write `AccordID`, never `AccordId`.
- Names come from the requirement text, and a verb from the text is not renamed in Go (`spec-words`).
- A package name is the capability it implements, in one lower-case word (`package-per-capability`).

### Packages and visibility

- New code starts unexported, or under an `internal/` directory; a name is exported when a caller outside needs it and its reason is on record (`trace-new-names`, `api-compat`).
- No interface is added before a second implementation needs it. v1 adds none for the decision module.

### Tests

- Tests compare exactly (`tests-exact`), spell raw bytes in hex (`raw-bytes-in-hex`) and share helpers only through `core/internal/testing/` (`test-helpers-placed`).
- Table tests name each case; a failure message says what was wanted and what came back.
