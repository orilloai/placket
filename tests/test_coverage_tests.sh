#!/bin/sh
# The coverage list's Go tests (runner-no-silent-pass): every Go test vectors/coverage.json names
# exists and passes when run alone, so a name in the list cannot stand for a test that never ran; and
# each scenario no vector operation can run names a Go test in its entry.
#
# A Go test is named in an entry's "tests" as the directory of its package from the repository root,
# a colon and its name, such as core/receipts:TestAForgedReceiptShowsAsARealOne
# (vectors/README.md). It is run from its module's root with `go test -count=1 -run '^<name>$' -v`,
# and passes only when the output holds its own "--- PASS" line: a test that is skipped, or a package
# that merely builds, does not pass.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
list=$root/vectors/coverage.json
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
say() { echo "coverage tests: $*"; }
bad() { say "FAIL: $*"; failed=1; }

command -v jq >/dev/null 2>&1 || { say "FAIL: jq: not found"; exit 1; }
[ -f "$list" ] || { say "FAIL: vectors/coverage.json: missing"; exit 1; }

# The scenarios no vector operation can run, each covered by a Go test: the requirement id and the
# scenario's title, separated by a tab.
required='receipt-8	a forged receipt shown
signing-4	a placket URI as a payload type
receipt-10	two mediators, one record'

printf '%s\n' "$required" | while IFS='	' read -r id title; do
	n=$(jq -r --arg i "$id" --arg s "$title" '.[$i].scenarios[$s].tests // [] | length' "$list" 2>/dev/null)
	if [ "${n:-0}" -gt 0 ]; then
		say "ok   $id: scenario \"$title\" names $n Go test(s)"
	else
		say "FAIL: $id: scenario \"$title\" names no Go test, and no vector operation can run it"
		echo fail >>"$tmp/failed"
	fi
done

jq -r '[.. | objects | select(has("tests")) | .tests | arrays | .[]] | unique | .[]' "$list" >"$tmp/tests" 2>/dev/null ||
	{ say "FAIL: vectors/coverage.json: not JSON"; exit 1; }
count=0
while read -r t; do
	dir=${t%%:*}
	name=${t#*:}
	count=$((count + 1))
	if [ "$dir" = "$t" ] || [ ! -d "$root/$dir" ]; then
		bad "$t: no package directory $dir"
		continue
	fi
	if ! grep -qE "^func $name\(" "$root/$dir"/*_test.go 2>/dev/null; then
		bad "$t: no func $name in $dir/*_test.go"
		continue
	fi
	mod=$dir
	while [ "$mod" != . ] && [ ! -f "$root/$mod/go.mod" ]; do mod=$(dirname "$mod"); done
	if [ ! -f "$root/$mod/go.mod" ]; then
		bad "$t: no module holds $dir"
		continue
	fi
	rel=./${dir#"$mod"}
	rel=$(printf '%s' "$rel" | sed 's|^\./*|./|')
	if (cd "$root/$mod" && go test -count=1 -run "^$name\$" "$rel" -v) >"$tmp/out" 2>&1 &&
		grep -qE -- "^\s*--- PASS: $name " "$tmp/out"; then
		say "ok   $t"
	else
		bad "$t: does not pass when run alone"
		tail -n 5 "$tmp/out" | sed 's/^/    /'
	fi
done <"$tmp/tests"

[ -f "$tmp/failed" ] && failed=1
say "$count Go tests named"
[ "$count" -gt 0 ] || failed=1
if [ "$failed" -ne 0 ]; then say "refused: a Go test the coverage list names must exist and pass (AGENTS.md, runner-no-silent-pass)"; exit 1; fi
say ok
