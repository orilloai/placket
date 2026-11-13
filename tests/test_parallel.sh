#!/bin/sh
# The side-by-side test of two steps of make ci: scripts runs the repository's shell tests at once,
# and tests runs each Go module's tests at once, and each still reports every result in its old order.
#
# Each case makes a minimal copy: the Makefile and the fixture tests/testdata/parallel/<case>/. A
# fixture's file is kept as <name>.in and named at setup, so that no run of this repository takes the
# planted test for one of its own. The fixtures' tests meet: each leaves a mark and waits, up to 30
# seconds, for the other's, so they pass only when they run at the same time.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
fixtures=$root/tests/testdata/parallel
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0
out=$tmp/out

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$out"
	failed=$((failed + 1))
}

# copy <dir>: the Makefile and the fixture's files at <dir> in $tmp/repo, each <name>.in named <name>.
copy() {
	rm -rf "$tmp/repo"
	mkdir -p "$tmp/repo/$1"
	cp "$root/Makefile" "$tmp/repo/"
	cp -R "$fixtures/$2/." "$tmp/repo/$1/"
	find "$tmp/repo" -name '*.in' | while read -r f; do mv "$f" "${f%.in}"; done
}

# shown <line>...: the output holds each line given, in that order, with lines in between allowed.
shown() {
	printf '%s\n' "$@" >"$tmp/want"
	awk 'NR == FNR { w[++n] = $0; next } i < n && $0 == w[i + 1] { i++ } END { exit i != n }' "$tmp/want" "$out"
}

# scripts: three shell tests, two of which meet and one of which fails alone. The step runs them at
# once, prints each one's output whole under its name in the tests' order, refuses, and names both
# that failed.
name="scripts runs every test at once"
cases=$((cases + 1))
copy tests scripts
(cd "$tmp/repo" && MAKEFLAGS= MAKELEVEL= make -k --no-print-directory scripts) >"$out" 2>&1
rc=$?
if [ "$rc" -eq 0 ]; then bad "make passed; two tests fail"
elif ! shown '== tests/test_a.sh' 'a met b' '== tests/test_b.sh' 'b met a' 'b fails' '== tests/test_c.sh' 'c fails' \
	'scripts: failed: tests/test_b.sh tests/test_c.sh'; then
	bad "the tests did not all run at once, in order, with both failures named"
elif grep -q 'did not run at the same time' "$out"; then bad "a test waited alone"
elif ! grep -q '^time: scripts: [0-9][0-9]*s$' "$out"; then bad "no time line for scripts"
else echo "ok   $name: each test's output in order, and both failures named"; fi

# tests: two Go modules whose tests meet. The shared run runs them at once, and race passes, with
# the modules' lines in their order.
name="tests runs every module at once"
cases=$((cases + 1))
copy . modules
mkdir "$tmp/meet"
(cd "$tmp/repo" && PLACKET_MEET=$tmp/meet MAKEFLAGS= MAKELEVEL= make -k --no-print-directory race) >"$out" 2>&1
rc=$?
if [ "$rc" -ne 0 ]; then bad "make failed; the modules' tests should meet"
elif ! shown 'tests: ./core: passed' 'tests: ./other: passed' 'tests: ok' 'race: ok'; then bad "the modules' lines are not in order"
elif ! grep -q '^time: tests: [0-9][0-9]*s$' "$out"; then bad "no time line for tests"
else echo "ok   $name: both modules passed, in order"; fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
