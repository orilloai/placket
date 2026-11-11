#!/bin/sh
# Self-tests for the core's guardrail checks in `make ci`.
#
# Each case copies the repository without its tests/test_*.sh (so the copy's `make ci` does not run
# this file again), lays one fixture from tests/testdata/guardrails/<case>/ over the copy, runs
# `make -k ci` there and reads its output. A planted fault must be refused by its own check, with a
# line "<rule>: <file>:<line>: ..."; a clean tree must pass with every check reporting "ok". The -k
# keeps make going after a refusal, so every other check runs too and its silence means something.
# A case that needs a symbolic link sets $setup, a command run in the copy after the fixture is laid;
# the links are made here rather than stored in git.
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
setup=

# ci <case>: run `make ci` on a copy of the repository with the case's fixture laid over it
# ("" for none), then $setup, which is cleared. The output goes to $tmp/out and the exit status to $rc.
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
	if [ -n "$setup" ]; then
		s=$setup
		setup=
		(cd "$tmp/repo" && eval "$s") >"$tmp/out" 2>&1 || { echo "setup failed: $s" >>"$tmp/out"; rc=2; return; }
	fi
	(cd "$tmp/repo" && MAKEFLAGS= MAKELEVEL= make -k --no-print-directory ci) >"$tmp/out" 2>&1
	rc=$?
}

# has <prefix>: some line of the output starts with prefix.
has() {
	awk -v p="$1" 'index($0, p) == 1 { found = 1 } END { exit !found }' "$tmp/out"
}

# refusals [rules]: the output's refusal lines, other than those of the rules named (space-separated).
refusals() {
	for r in $rules; do
		case " ${1:-} " in *" $r "*) continue ;; esac
		awk -v p="$r: " -v ok="$r: ok" 'index($0, p) == 1 && $0 != ok' "$tmp/out"
	done
}

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$tmp/out"
	failed=$((failed + 1))
}

# refused <case> <rules> <file>[:<line>]...: make ci fails, and only the rules named (one, or several
# space-separated) refuse, each naming every file and line given. A whole file is named without a line.
refused() {
	name=$1
	rule=$2
	shift 2
	cases=$((cases + 1))
	ci "$name"
	if [ "$rc" -eq 0 ]; then bad "make ci passed; $rule should refuse $*"; return; fi
	for r in $rule; do
		for at in "$@"; do
			if ! has "$r: $at: "; then bad "no line starting '$r: $at: '"; return; fi
		done
	done
	other=$(refusals "$rule")
	if [ -n "$other" ]; then bad "refused by another check: $other"; return; fi
	echo "ok   $name: $rule refuses $*"
}

# fails <case> <line>: make ci fails with that exact line in its output, and no guardrail check refuses.
fails() {
	name=$1
	cases=$((cases + 1))
	ci "$1"
	if [ "$rc" -eq 0 ]; then bad "make ci passed; it should fail with '$2'"; return; fi
	if ! awk -v l="$2" '$0 == l { found = 1 } END { exit !found }' "$tmp/out"; then bad "no line '$2'"; return; fi
	other=$(refusals)
	if [ -n "$other" ]; then bad "a check refused: $other"; return; fi
	echo "ok   $name: fails with '$2'"
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

# core-no-world: the packages that reach the process, files, logs and memory, under any name, and
# the other ways to read the clock or print.
refused world-imports core-no-world core/world/world.go:5 core/world/world.go:6 core/world/world.go:7 \
	core/world/world.go:8 core/world/world.go:9 core/world/world.go:10 core/world/world.go:11 \
	core/world/world.go:12 core/world/world.go:13
refused world-calls core-no-world core/world/world.go:8 core/world/world.go:9 core/world/world.go:10 \
	core/world/world.go:11 core/world/world.go:12 core/world/world.go:13 core/world/world.go:14 \
	core/world/world.go:15 core/world/world.go:16 core/world/world.go:17

# core-no-panic: a call in code; a "panic(" inside a string literal (the known false positive);
# a call after a string that holds "//", which is not a comment.
refused panic-call core-no-panic core/p/p.go:7
refused panic-in-string core-no-panic core/p/p.go:5
refused panic-after-slashes core-no-panic core/p/p.go:5

# core-no-world: cgo, and source files Go builds that are not Go (assembly, C, C++, headers, objects),
# outside testdata/; a selector split after its dot, by a line break or a comment.
refused world-cgo core-no-world core/c/c.go:5 core/c/d.go:4
refused world-non-go core-no-world core/a/a_amd64.s core/a/b_arm64.S core/a/x.c core/a/x.h core/a/y.cc \
	core/a/z.syso
refused world-split-selector core-no-world core/w/w.go:12 core/w/w.go:16 core/w/w.go:22

# An import path written with an escape cannot be read by a grep, so both checks that read import
# paths refuse it, whatever it spells.
refused world-escaped-import "core-imports core-no-world" core/e/e.go:4 core/e/f.go:4
refused imports-escaped "core-imports core-no-world" core/bad/bad.go:5

# A symbolic link under core/, to a file, a directory or core/go.mod itself, is refused by every
# check: the Go tool follows a link, and a check that skipped it would let anything in.
setup='mkdir -p core/s && ln -s ../../elsewhere/s.go core/s/s.go'
refused symlink-file "$rules" core/s/s.go
setup='ln -s ../elsewhere core/d'
refused symlink-dir "$rules" core/d
setup='rm core/go.mod && ln -s ../elsewhere/go.mod core/go.mod'
refused symlink-gomod "$rules" core/go.mod

# A /* */ comment is not code, even when a line inside it starts with //; a raw string is not a
# comment, even when a line inside it starts with //. Code after either is checked.
refused block-comment-import core-imports core/bad/bad.go:6
refused block-comment-world core-no-world core/w/w.go:6 core/w/w.go:15
refused block-comment-panic core-no-panic core/p/p.go:7
refused raw-string-slashes core-no-world core/w/w.go:8

# The same names in a _test.go file, or only in // or /* */ comments, pass.
passes world-in-test
passes comments-only
passes block-comments-only

# What core-no-world allows in a non-test file: fmt.Fprint* and fmt.Sprint*, a call of a field named
# print, "C" and escapes in a string that is not an import path, and assembly under testdata/.
passes world-allowed

# A module whose packages cannot be listed fails make ci; it is never taken for an empty one.
fails gomod-broken "== ./core: go list failed"

# A module with no Go packages is skipped: it is neither built, vetted nor tested.
passes empty-module
if [ "$rc" -eq 0 ]; then
	has "== ./empty: no Go packages; skipped" || bad "the package-less module was not skipped"
	if awk '$0 == "== ./empty" { found = 1 } END { exit !found }' "$tmp/out"; then
		bad "make ci went into the package-less module"
	fi
fi

# The repository as it stands passes. No module under testdata/ (one of which does not compile) is
# built, vetted or tested.
passes ""
if [ "$rc" -eq 0 ]; then
	if awk '/^== .*testdata/ { found = 1 } END { exit !found }' "$tmp/out"; then
		bad "make ci went into a testdata/ module"
	fi
fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
