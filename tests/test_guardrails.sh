#!/bin/sh
# Self-tests for the core's guardrail checks in `make ci`.
#
# Each case copies the repository without its tests/test_*.sh (so the copy's `make ci` does not run
# this file again), lays one fixture from tests/testdata/guardrails/<case>/ over the copy, runs
# `make ci` there and reads its output. A planted fault must be refused by its own check, with a
# line "<rule>: <file>:<line>: ..."; a clean tree must pass with every check reporting "ok".
# Fixtures stay under testdata/, which `make ci` never builds, vets or tests.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
fixtures=$root/tests/testdata/guardrails
rules="core-no-deps core-imports core-no-world core-no-panic"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0

# ci <case>: run `make ci` on a copy of the repository with the case's fixture laid over it
# ("" for none). The output goes to $tmp/out and the exit status to $rc.
ci() {
	rm -rf "$tmp/repo"
	mkdir "$tmp/repo"
	for f in "$root"/* "$root"/.[!.]*; do
		[ -e "$f" ] || continue
		[ "${f##*/}" = .git ] && continue
		cp -R "$f" "$tmp/repo/"
	done
	rm -f "$tmp/repo"/tests/test_*.sh
	if [ -n "$1" ]; then
		[ -d "$fixtures/$1" ] || { echo "no fixture $1" >"$tmp/out"; rc=2; return; }
		cp -R "$fixtures/$1/." "$tmp/repo/"
	fi
	(cd "$tmp/repo" && MAKEFLAGS= MAKELEVEL= make --no-print-directory ci) >"$tmp/out" 2>&1
	rc=$?
}

# has <prefix>: some line of the output starts with prefix.
has() {
	awk -v p="$1" 'index($0, p) == 1 { found = 1 } END { exit !found }' "$tmp/out"
}

# refusals [rule]: the output's refusal lines, other than those of rule.
refusals() {
	for r in $rules; do
		[ "$r" = "${1:-}" ] && continue
		awk -v p="$r: " -v ok="$r: ok" 'index($0, p) == 1 && $0 != ok' "$tmp/out"
	done
}

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$tmp/out"
	failed=$((failed + 1))
}

# refused <case> <rule> <file>:<line>: make ci fails, and only rule refuses, naming file and line.
refused() {
	name=$1
	cases=$((cases + 1))
	ci "$1"
	if [ "$rc" -eq 0 ]; then bad "make ci passed; $2 should refuse $3"; return; fi
	if ! has "$2: $3: "; then bad "no line starting '$2: $3: '"; return; fi
	other=$(refusals "$2")
	if [ -n "$other" ]; then bad "refused by another check: $other"; return; fi
	echo "ok   $name: $2 refuses $3"
}

# passes <case>: make ci passes, and every check runs and reports ok.
passes() {
	name=${1:-clean tree}
	cases=$((cases + 1))
	ci "$1"
	if [ "$rc" -ne 0 ]; then bad "make ci failed (exit $rc)"; return; fi
	for r in $rules; do
		if ! has "$r: ok"; then bad "$r did not run as its own step"; return; fi
	done
	other=$(refusals)
	if [ -n "$other" ]; then bad "a check refused: $other"; return; fi
	echo "ok   $name: passes"
}

# core-imports: nothing under core/, test files included, imports adapters, cmd or recipes.
refused imports-adapters core-imports core/bad/bad.go:5
refused imports-cmd core-imports core/bad/bad.go:4
refused imports-recipes core-imports core/bad/bad.go:4
refused imports-in-test core-imports core/bad/bad_test.go:6

# core-no-deps: core/go.mod requires nothing, in either form.
refused deps-require core-no-deps core/go.mod:7
refused deps-require-block core-no-deps core/go.mod:7

# core-no-world: each refused name, one case each, in a non-test file.
refused world-time-now core-no-world core/world/world.go:7
refused world-crypto-rand core-no-world core/world/world.go:4
refused world-math-rand core-no-world core/world/world.go:4
refused world-math-rand-v2 core-no-world core/world/world.go:4
refused world-os-getenv core-no-world core/world/world.go:7
refused world-os-open core-no-world core/world/world.go:7
refused world-net core-no-world core/world/world.go:4
refused world-net-http core-no-world core/world/world.go:4
refused world-fmt-print core-no-world core/world/world.go:7
refused world-fmt-printf core-no-world core/world/world.go:7
refused world-fmt-println core-no-world core/world/world.go:7

# core-no-panic: a call in code; a "panic(" inside a string literal (the known false positive);
# a call after a string that holds "//", which is not a comment.
refused panic-call core-no-panic core/p/p.go:7
refused panic-in-string core-no-panic core/p/p.go:5
refused panic-after-slashes core-no-panic core/p/p.go:5

# The same names in a _test.go file, or only in // comments, pass.
passes world-in-test
passes comments-only

# The repository as it stands passes. Its empty core/ module and every module under testdata/
# (one of which does not compile) are neither built, vetted nor tested.
passes ""
if [ "$rc" -eq 0 ]; then
	has "== ./core: no Go packages; skipped" || bad "the package-less core/ module was not skipped"
	if awk '/^== .*testdata/ { found = 1 } END { exit !found }' "$tmp/out"; then
		bad "make ci went into a testdata/ module"
	fi
fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
