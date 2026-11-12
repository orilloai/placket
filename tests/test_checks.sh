#!/bin/sh
# Self-tests for the checks of `make ci` that read the vectors' generators, the format constants, the
# product words, the claims, the binary's main, the root module and the tools' versions, and for the
# toolchain steps.
#
# Each case copies the repository without its tests/test_*.sh (so the copy's `make ci` does not run
# this file again), lays one fixture from tests/testdata/checks/<case>/ over the copy, runs $setup
# there when it is set, and then runs make. A case of the checks runs `make -k` on every check named in
# $checks: a planted fault must be refused by its own check, with exactly the lines the case names, and
# no other check may refuse. A case of a toolchain step runs that step alone and reads its output. The
# clean tree runs the whole of `make -k ci` and must pass with every check and step reporting "ok", the
# coverage table printed and the conformance runner's open entries listed.
# Fixtures stay under testdata/, which `make ci` never builds, vets, tests or reads as part of the tree.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
fixtures=$root/tests/testdata/checks
checks="vectors-independent constants-once product-words-once claims-stop-at-crossing main-is-thin root-module machine-rules-in-ci"
steps="vet lint vulnerabilities secrets race fuzz-seeds coverage-reported runner-no-silent-pass"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0
setup=

# run <case> <targets>: copy the repository, lay the case's fixture over it ("" for none), run $setup,
# which is cleared, then `make -k <targets>`. The output goes to $tmp/out and the exit status to $rc.
run() {
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
	shift
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

# refusal_lines [rules]: the output's refusal lines of the checks, other than those of the rules named.
refusal_lines() {
	for r in $checks; do
		case " ${1:-} " in *" $r "*) continue ;; esac
		awk -v p="$r: " -v ok="$r: ok" 'index($0, p) == 1 && $0 != ok && index($0, p "refused: ") != 1' "$tmp/out"
	done
}

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$tmp/out"
	failed=$((failed + 1))
}

# refused <case> <rule> <line>...: make fails; the rule's refusal lines are exactly the lines given, in
# that order, each without the "<rule>: " it starts with; it closes with its "refused:" line; and no
# other check refuses.
refused() {
	name=$1
	rule=$2
	shift 2
	cases=$((cases + 1))
	run "$name" $checks
	if [ "$rc" -eq 0 ]; then bad "make passed; $rule should refuse"; return; fi
	want=$(for l in "$@"; do printf '%s: %s\n' "$rule" "$l"; done)
	got=$(awk -v p="$rule: " -v ok="$rule: ok" 'index($0, p) == 1 && $0 != ok && index($0, p "refused: ") != 1' "$tmp/out")
	if [ "$got" != "$want" ]; then bad "$rule's lines differ; want:
$want"; return; fi
	if ! starts "$rule: refused: "; then bad "no closing '$rule: refused: ' line"; return; fi
	other=$(refusal_lines "$rule")
	if [ -n "$other" ]; then bad "refused by another check: $other"; return; fi
	echo "ok   $name: $rule refuses $#"
}

# passes <case>: every check passes and says so.
passes() {
	name=$1
	cases=$((cases + 1))
	run "$name" $checks
	if [ "$rc" -ne 0 ]; then bad "make failed (exit $rc)"; return; fi
	for r in $checks; do
		if ! has "$r: ok"; then bad "$r did not report ok"; return; fi
	done
	echo "ok   $name: passes"
}

# step <case> <step> <prefix>...: the step alone fails on the case, with a line starting with each
# prefix given and its own "<step>: <module>: refused" or "<step>: refused: " line.
step() {
	name=$1
	st=$2
	shift 2
	cases=$((cases + 1))
	run "$name" "$st"
	if [ "$rc" -eq 0 ]; then bad "$st passed; it should refuse"; return; fi
	for p in "$@"; do
		if ! starts "$p"; then bad "no line starting '$p'"; return; fi
	done
	if has "$st: ok"; then bad "$st also reported ok"; return; fi
	echo "ok   $name: $st refuses"
}

# vectors-independent: a generator under vectors/<group>/ imports no core package its group tests,
# as vectors/README.md's table gives them, and never the runner; its test files are generators too.
refused independent-imports vectors-independent \
	'vectors/canonical/gen/main.go:5: "github.com/orilloai/placket/core/canonical"'
refused independent-block vectors-independent \
	'vectors/questions/gen/gen.go:8: m "github.com/orilloai/placket/core/mediator"' \
	'vectors/questions/gen/gen.go:9: "github.com/orilloai/placket/core/conformance"' \
	'vectors/questions/gen/gen.go:11: "github.com/orilloai/placket/core/receipts/internal/chain"' \
	'vectors/questions/gen/gen_test.go:3: import "github.com/orilloai/placket/core/mediator"'
# A generator outside every group's directory, or in a group the table does not list, is refused whole.
refused independent-placement vectors-independent \
	"vectors/elsewhere/gen.go: the group elsewhere has no row in vectors/README.md's table of the packages each group tests" \
	"vectors/gen.go: a generator outside every group's directory; generators go under vectors/<group>/"
# A symbolic link under vectors/ is refused: the check does not follow it.
setup='ln -s ../core vectors/core'
refused "" vectors-independent 'vectors/core: a symbolic link'
# A lower package, an import named only in a comment, and code under testdata/ pass.
passes independent-allowed

# constants-once: each placket.dev/ or vnd.placket literal is in one non-test file of core/ and
# adapters/, named at its first line in each file that holds it; a second use in the same file, a
# test file and a comment pass.
refused constants-twice constants-once \
	'adapters/local/local.go:5: "application/vnd.placket.call+json" is also in core/b/b.go' \
	'core/a/a.go:5: "https://placket.dev/crossing/0.1" is also in core/b/b.go' \
	'core/b/b.go:5: "https://placket.dev/crossing/0.1" is also in core/a/a.go' \
	'core/b/b.go:8: "application/vnd.placket.call+json" is also in adapters/local/local.go'

# product-words-once: the core spells neither "ask" nor "released" as a whole literal, in either quote
# form; "asks", a test file, a comment and the command line's own words file pass.
refused words-in-core product-words-once \
	'core/w/w.go:5: released = "released"' \
	'core/w/w.go:7: ask      = `ask`'

# claims-stop-at-crossing: no literal under adapters/ or cmd/ holds a word about use after receipt, in
# any case or common ending, a raw string over two lines included; "retainer", an identifier, a
# comment, a test file and the core pass.
refused claims claims-stop-at-crossing \
	'adapters/cli/words.go:6: "the receiver will not retain the part"' \
	'adapters/cli/words.go:8: "a line\nTraining on it is not prevented"' \
	'cmd/placket/main.go:3: "SHARED with consent"'

# main-is-thin: 40 lines is refused at the 40th; 39 pass; with no main.go the check passes (clean tree).
refused main-40 main-is-thin 'cmd/placket/main.go:40: main.go has 40 lines'
passes main-39

# root-module: a root go.mod requires the core and replaces it with ./core, and is the root module; no
# go.work or go.work.sum is committed, one under testdata/ aside. With no root go.mod the check passes.
refused root-no-replace root-module 'go.mod: no replace of github.com/orilloai/placket/core with ./core'
refused root-other root-module \
	'go.mod:1: the module is github.com/orilloai/placket/adapters, not github.com/orilloai/placket' \
	'go.mod:8: github.com/orilloai/placket/core is replaced with ../placket-core, not ./core'
setup='printf "go 1.27\n\nuse ./core\n" >go.work && : >go.work.sum'
refused "" root-module 'go.work: a workspace file; none is committed' 'go.work.sum: a workspace file; none is committed'
setup='mkdir -p tests/testdata/w && printf "go 1.27\n" >tests/testdata/w/go.work'
passes root-ok

# machine-rules-in-ci: every go run or go install of a tool names an exact version. A tool of the
# Makefile set to @latest is refused where it runs, its variable read; in a script, no version, a
# partial one and a branch are refused, and a local path, an exact version inside parentheses, a
# pseudo-version, a comment and another command's "run" pass.
setup='sed "s|staticcheck@v[0-9.]*|staticcheck@latest|" Makefile >Makefile.new && mv Makefile.new Makefile'
lint_line=$(grep -n '(STATICCHECK) [.]/[.][.][.])$' "$root/Makefile" | cut -d: -f1)
refused "" machine-rules-in-ci "Makefile:$lint_line: honnef.co/go/tools/cmd/staticcheck@latest: not an exact version"
refused tool-unpinned machine-rules-in-ci \
	'scripts/fetch.sh:3: golang.org/x/tools/cmd/stringer: not an exact version' \
	'scripts/fetch.sh:4: golang.org/x/tools/cmd/stringer@v0.30: not an exact version' \
	'scripts/fetch.sh:7: golang.org/x/vuln/cmd/govulncheck@master: not an exact version'

# The toolchain steps each refuse their own fault: vet a format verb, lint an unused function, race a
# data race, fuzz-seeds a committed seed that fails, secrets a token written into the core. A fixture's
# test file is kept as <name>_test.go.in and named at setup, so that no test run of this repository
# takes the planted test for one of its own.
step vet-printf vet 'vet: ./core: refused' 'v/v.go:7:'
step lint-unused lint 'lint: ./core: refused' 'l/l.go:4:6: func unused is unused (U1000)'
setup='mv core/r/r_test.go.in core/r/r_test.go'
step race race 'race: ./core: refused' 'WARNING: DATA RACE'
setup='mv core/f/f_test.go.in core/f/f_test.go'
step fuzz-seed fuzz-seeds 'fuzz-seeds: ./core: refused' '    --- FAIL: FuzzHalf/seven'
# The token is put together here, so that this file holds no secret for the scan to find.
a=Zx9Qm2Lr7Tb4 b=Wk8Np3Vc6Hy1 c=Jd5Fs0Ga2Ue
setup="mkdir -p core/s && printf 'package s\n\n// A token left in by mistake.\nconst token = \"%s\"\n' 'gh''p_$a$b$c' >core/s/s.go"
step "" secrets 'secrets: core/s/s.go:4: ' 'secrets: refused: '

# coverage-reported prints a package with no test at 0% and passes: a number never fails it.
name=coverage-untested
cases=$((cases + 1))
run coverage-untested coverage-reported
if [ "$rc" -ne 0 ]; then bad "coverage-reported failed (exit $rc)"
elif ! awk '/^github[.]com\/orilloai\/placket\/core\/z\/z[.]go:[0-9]+:[ \t]+One[ \t]+0[.]0%$/ { found = 1 } END { exit !found }' "$tmp/out"; then
	bad "no 0.0% line for core/z"
elif ! has "coverage-reported: ok"; then bad "no 'coverage-reported: ok'"
else echo "ok   $name: coverage-reported prints 0% and passes"; fi

# The repository as it stands passes the whole of make ci: every check and every step runs by name and
# reports ok, the earlier checks too, each module's coverage table is printed, and the runner lists the
# coverage list's open entries.
name="clean tree"
cases=$((cases + 1))
run "" ci
if [ "$rc" -ne 0 ]; then bad "make ci failed (exit $rc)"
else
	missing=
	for r in core-no-deps core-imports core-no-world core-no-panic specs-name-behaviour schemas-hold $checks $steps; do
		has "$r: ok" || missing="$missing $r"
	done
	if [ -n "$missing" ]; then bad "no ok line from:$missing"
	elif [ -n "$(refusal_lines)" ]; then bad "a check refused: $(refusal_lines)"
	elif ! has "coverage-reported: ./core:" || ! starts "total:"; then bad "no coverage table for ./core"
	elif ! starts "runner-no-silent-pass: open: "; then bad "the runner listed no open entry"
	else echo "ok   $name: passes"; fi
fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
