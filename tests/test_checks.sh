#!/bin/sh
# Self-tests for the steps of `make ci` that the checker in tools/guard/ does not own: the module
# loop (modules), the toolchain steps (vet, lint, race, fuzz-seeds, secrets, coverage-reported) and
# the conformance runner's list of open entries. The checks of the tree are proved by the checker's
# own Go tests, and their wiring into make by tests/test_wiring.sh.
#
# Each case makes a minimal copy: the Makefile and a core module of the case's own, the fixture
# tests/testdata/checks/<case>/ laid over a bare core/go.mod, and nothing of the real core/ or
# tools/guard/. It then runs $setup there when it is set, runs make -k on the steps the case names, and
# reads the output. Only the runner's case copies the real core/ and vectors/, which it runs.
# A fixture's test file is kept as <name>_test.go.in and named at setup, so that no test run of this
# repository takes the planted test for one of its own. Fixtures stay under testdata/, which make ci
# never builds, vets, tests or reads as part of the tree.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
fixtures=$root/tests/testdata/checks
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0
setup=

# copy <case>: a minimal copy in $tmp/repo: the Makefile, a bare core module and the case's fixture
# ("" for none).
copy() {
	rm -rf "$tmp/repo"
	mkdir -p "$tmp/repo/core"
	cp "$root/Makefile" "$tmp/repo/"
	printf 'module github.com/orilloai/placket/core\n\ngo 1.27\n' >"$tmp/repo/core/go.mod"
	if [ -n "$1" ]; then
		[ -d "$fixtures/$1" ] || { echo "no fixture $1" >"$tmp/out"; return 1; }
		cp -R "$fixtures/$1/." "$tmp/repo/"
	fi
}

# run <case> <targets>: make the case's copy, run $setup in it, which is cleared, then
# `make -k <targets>`. The output goes to $tmp/out and the exit status to $rc.
run() {
	copy "$1" || { rc=2; return; }
	shift
	make_in "$@"
}

# make_in <targets>: run $setup in the copy, which is cleared, then `make -k <targets>` there.
make_in() {
	if [ -n "$setup" ]; then
		s=$setup
		setup=
		(cd "$tmp/repo" && eval "$s") >"$tmp/out" 2>&1 || { echo "setup failed: $s" >>"$tmp/out"; rc=2; return; }
	fi
	(cd "$tmp/repo" && MAKEFLAGS= MAKELEVEL= make -k --no-print-directory "$@") >"$tmp/out" 2>&1
	rc=$?
}

# has <line>: the output holds exactly that line. starts <prefix>: some line starts with prefix.
has() {
	awk -v l="$1" '$0 == l { found = 1 } END { exit !found }' "$tmp/out"
}
starts() {
	awk -v p="$1" 'index($0, p) == 1 { found = 1 } END { exit !found }' "$tmp/out"
}
# timed <step>: the step printed its time.
timed() {
	awk -v p="time: $1: " 'index($0, p) == 1 && /s$/ { found = 1 } END { exit !found }' "$tmp/out"
}

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$tmp/out"
	failed=$((failed + 1))
}

# refuses <step> <prefix>...: the step refused, with a line starting with each prefix given, and
# printed its time.
refuses() {
	st=$1
	shift
	if [ "$rc" -eq 0 ]; then bad "make passed; $st should refuse"; return 1; fi
	for p in "$@"; do
		if ! starts "$p"; then bad "no line starting '$p'"; return 1; fi
	done
	if has "$st: ok"; then bad "$st also reported ok"; return 1; fi
	if ! timed "$st"; then bad "no time line for $st"; return 1; fi
}

# step <case> <step> <prefix>...: the step alone fails on the case, with a line starting with each
# prefix given, and does not report ok.
step() {
	fixture=$1
	st=$2
	name=${1:-$st}
	shift 2
	cases=$((cases + 1))
	run "$fixture" "$st"
	refuses "$st" "$@" || return
	echo "ok   $name: $st refuses"
}

# The toolchain steps each refuse their own fault: vet a format verb, lint an unused function, secrets
# a token written into the core.
step vet-printf vet 'vet: ./core: refused' 'v/v.go:7:'
step lint-unused lint 'lint: ./core: refused' 'l/l.go:4:6: func unused is unused (U1000)'
# The token is put together here, so that this file holds no secret for the scan to find.
a=Zx9Qm2Lr7Tb4 b=Wk8Np3Vc6Hy1 c=Jd5Fs0Ga2Ue
setup="mkdir -p core/s && printf 'package s\n\n// A token left in by mistake.\nconst token = \"%s\"\n' 'gh''p_$a$b$c' >core/s/s.go"
step "" secrets 'secrets: core/s/s.go:4: ' 'secrets: refused: '

# race, fuzz-seeds and coverage-reported read one shared test run. race refuses any failure of it: a
# data race, a plain failing test, a failing seed. fuzz-seeds also refuses a failing seed and names
# its test, and passes over a failing test that is not a fuzz test.
name=race
cases=$((cases + 1))
setup='mv core/r/r_test.go.in core/r/r_test.go'
run race race fuzz-seeds
if refuses race 'race: ./core: refused' 'WARNING: DATA RACE' && timed tests; then echo "ok   $name: race refuses a data race"; fi

name=test-fails
cases=$((cases + 1))
setup='mv core/t/t_test.go.in core/t/t_test.go'
run test-fails race fuzz-seeds
if refuses race 'race: ./core: refused' '--- FAIL: TestSum'; then
	if ! has 'fuzz-seeds: ./core: 0 fuzz tests run on their seed corpora' || ! has 'fuzz-seeds: ok'; then bad "fuzz-seeds refused a test that is not a fuzz test"
	else echo "ok   $name: race refuses a failing test, and fuzz-seeds passes over it"; fi
fi

name=fuzz-seed
cases=$((cases + 1))
setup='mv core/f/f_test.go.in core/f/f_test.go'
run fuzz-seed race fuzz-seeds
if refuses race 'race: ./core: refused' && refuses fuzz-seeds 'fuzz-seeds: ./core: refused' '    --- FAIL: FuzzHalf/seven'; then
	echo "ok   $name: race and fuzz-seeds refuse a failing seed, naming its test"
fi

# coverage-reported prints a package with no test at 0% and passes: a number never fails it.
name=coverage-untested
cases=$((cases + 1))
run coverage-untested coverage-reported
if [ "$rc" -ne 0 ]; then bad "coverage-reported failed (exit $rc)"
elif ! awk '/^github[.]com\/orilloai\/placket\/core\/z\/z[.]go:[0-9]+:[ \t]+One[ \t]+0[.]0%$/ { found = 1 } END { exit !found }' "$tmp/out"; then
	bad "no 0.0% line for core/z"
elif ! has "coverage-reported: ok"; then bad "no 'coverage-reported: ok'"
else echo "ok   $name: coverage-reported prints 0% and passes"; fi

# On a clean module, the shared run passes: race and fuzz-seeds report ok, coverage-reported prints
# ./core's table and its total, and each step, the shared run among them, prints its time.
name=clean
cases=$((cases + 1))
setup='mv core/c/c_test.go.in core/c/c_test.go'
run clean race fuzz-seeds coverage-reported
if [ "$rc" -ne 0 ]; then bad "the steps failed (exit $rc)"
elif ! has "race: ok" || ! has "fuzz-seeds: ok" || ! has "coverage-reported: ok"; then bad "a step did not report ok"
elif ! has "coverage-reported: ./core:" || ! starts "total:"; then bad "no coverage table for ./core"
elif ! timed tests || ! timed race || ! timed fuzz-seeds || ! timed coverage-reported; then bad "a step printed no time"
else echo "ok   $name: the shared run passes, with ./core's coverage table"; fi

# modules: a module whose packages cannot be listed fails; it is never taken for an empty one.
name=gomod-broken
cases=$((cases + 1))
run gomod-broken modules
if [ "$rc" -eq 0 ]; then bad "modules passed"
elif ! has "== ./core: go list failed"; then bad "no line '== ./core: go list failed'"
elif ! timed modules; then bad "no time line for modules"
else echo "ok   $name: modules fails on a module it cannot list"; fi

# A module whose files go list does not see on this host (all behind a build constraint for another
# system) still has its format checked before it is skipped as one with no packages.
name=unlisted-unformatted
cases=$((cases + 1))
run unlisted-unformatted modules
if [ "$rc" -eq 0 ]; then bad "modules passed"
elif ! has "gofmt: ./core: not formatted: ./u/u.go"; then bad "no line 'gofmt: ./core: not formatted: ./u/u.go'"
else echo "ok   $name: modules checks the format of files no build on this host reads"; fi

# A module with no Go packages is skipped: no toolchain step enters it.
name=empty-module
cases=$((cases + 1))
run empty-module modules vet race
if [ "$rc" -ne 0 ]; then bad "the steps failed (exit $rc)"
elif ! has "== ./empty: no Go packages; skipped"; then bad "the package-less module was not skipped"
elif has "== ./empty" || starts "vet: ./empty" || starts "tests: ./empty" || starts "race: ./empty"; then bad "a step went into the package-less module"
else echo "ok   $name: no step enters a module with no packages"; fi

# No step enters a module under a testdata/ directory, though the one here does not compile.
name=in-testdata
cases=$((cases + 1))
setup='mv core/c/c_test.go.in core/c/c_test.go'
copy clean && cp -R "$fixtures/in-testdata/." "$tmp/repo/" && make_in modules vet race coverage-reported
if [ "$rc" -ne 0 ]; then bad "the steps failed (exit $rc)"
elif awk '/testdata/ && /^(== |gofmt: |vet: |tests: |race: |coverage-reported: )/ { found = 1 } END { exit !found }' "$tmp/out"; then
	bad "a step went into a testdata/ module"
else echo "ok   $name: no step enters a module under testdata/"; fi

# runner-no-silent-pass prints one "open:" line for each open entry of the coverage list, a scenario's
# included, and none when none is open.
name=runner-open
cases=$((cases + 1))
rm -rf "$tmp/repo"
mkdir "$tmp/repo"
cp -R "$root/Makefile" "$root/core" "$root/vectors" "$root/spec" "$tmp/repo/"
want=$(jq '[.[] | ., (.scenarios // {} | .[]) | select(type == "object" and (has("open") or has("rest")))] | length' "$root/vectors/coverage.json")
make_in runner-no-silent-pass
got=$(grep -c '^runner-no-silent-pass: open: ' "$tmp/out")
if [ "$rc" -ne 0 ]; then bad "runner-no-silent-pass failed (exit $rc)"
elif [ "$got" != "$want" ]; then bad "$got open lines for $want open entries"
elif ! timed runner-no-silent-pass; then bad "no time line for runner-no-silent-pass"
else echo "ok   $name: one open line for each of the $want open entries"; fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
