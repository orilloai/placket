#!/bin/sh
# The coverage list's Go tests (runner-no-silent-pass): every Go test vectors/coverage.json names
# exists and passes when run alone, so a name in the list cannot stand for a test that never ran; and
# each scenario listed below, every one no vector operation can run among them, names a Go test in
# its entry.
#
# A Go test is named in an entry's "tests" as the directory of its package from the repository root,
# a colon and its name, such as core/receipts:TestAForgedReceiptShowsAsARealOne
# (vectors/README.md). The named tests of each module run in one go test from its root, given each
# package and a pattern of exactly the names listed, `go test -count=1 -json -run '^(<name>|...)$'`,
# so no test the list does not name runs beside them. A test passes only when the run reports a pass
# of its own, in its own package: a test that is skipped, or a package that merely builds, does not.
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

# The scenarios each covered by a Go test: those no vector operation can run, such as a change with
# an empty version, which the vector format refuses, and those whose Go test also proves what the
# runner cannot see, such as an empty list of answerers that is not nil. The requirement id and the
# scenario's title, separated by a tab.
required='receipt-8	a forged receipt shown
signing-4	a placket URI as a payload type
receipt-10	two mediators, one record
maybe-9	an empty change
maybe-1	a deadline past the last time
maybe-4	a question nobody can answer
maybe-6	the proposer answers its own proposal
commands-1	a give that passes
commands-1	a refused command
commands-1	every result under every flag
commands-2	an ask left open
commands-2	waits that end with an accept and a change
commands-2	a decline and its comment
commands-3	a founding
commands-3	an empty queue and a delivery
commands-3	every delivery at once
commands-3	every delivery at once into a directory
commands-3	a window stops
commands-4	a shown receipt and a verified journal
commands-4	the log and reconciliation
log-1	the log without every crossing is unchanged
log-2	every crossing in journal order
log-2	content by digest and by envelope, never by time
commands-5	a file that crossed, and one byte changed
commands-5	a released proposal
commands-5	a block, and a part to another key
commands-6	a value in a data body, and one byte changed
commands-6	a fraction in a text body
commands-6	a match limited to kinds
commands-6	a block, a part to another key, and a body never released
commands-7	every condition alone and combined
commands-7	every match in fetch order
commands-7	a kept line changed, and nothing kept'

printf '%s\n' "$required" | while IFS='	' read -r id title; do
	n=$(jq -r --arg i "$id" --arg s "$title" '.[$i].scenarios[$s].tests // [] | length' "$list" 2>/dev/null)
	if [ "${n:-0}" -gt 0 ]; then
		say "ok   $id: scenario \"$title\" names $n Go test(s)"
	else
		say "FAIL: $id: scenario \"$title\" names no Go test"
		echo fail >>"$tmp/failed"
	fi
done

jq -r '[.. | objects | select(has("tests")) | .tests | arrays | .[]] | unique | .[]' "$list" >"$tmp/tests" 2>/dev/null ||
	{ say "FAIL: vectors/coverage.json: not JSON"; exit 1; }
count=0
: >"$tmp/named"
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
	printf '%s\t%s\t%s\t%s\n' "$mod" "$rel" "$name" "$t" >>"$tmp/named"
done <"$tmp/tests"

# One go test per module, over the packages of its named tests, with the pattern of their names. Its
# events are read by package, the module's path and the package's directory within it, and by test.
cut -f1 "$tmp/named" | sort -u >"$tmp/modules"
i=0
while read -r mod; do
	i=$((i + 1))
	path=$(cd "$root/$mod" && go list -m) || { bad "$mod: go list -m failed"; continue; }
	pkgs=$(awk -F'\t' -v m="$mod" '$1 == m { print $2 }' "$tmp/named" | sort -u)
	names=$(awk -F'\t' -v m="$mod" '$1 == m { print $3 }' "$tmp/named" | sort -u | paste -sd'|' -)
	# shellcheck disable=SC2086 # each package directory is a word of its own, with no space in it
	(cd "$root/$mod" && go test -count=1 -json -run "^($names)\$" $pkgs) >"$tmp/$i.json" 2>"$tmp/$i.err" </dev/null
	awk -F'\t' -v m="$mod" '$1 == m' "$tmp/named" | while IFS='	' read -r _ rel name t; do
		sub=${rel#./}
		pkg=$path${sub:+/$sub}
		if jq -R -e --arg p "$pkg" --arg n "$name" 'fromjson? | select(.Package == $p and .Test == $n and .Action == "pass")' \
			"$tmp/$i.json" >/dev/null 2>&1; then
			say "ok   $t"
		else
			say "FAIL: $t: does not pass"
			echo fail >>"$tmp/failed"
			# The test's own output, or else its package's, such as why it did not build.
			{ jq -R -j --arg p "$pkg" --arg n "$name" 'fromjson? | select(.Action == "output" or .Action == "build-output")
				| select(.Test == $n or (.Test == null and ((.Package // .ImportPath // "") | split(" ")[0] | sub("_test$"; "")) == $p))
				| select(.Package == $p or .Test == null) | .Output' "$tmp/$i.json" 2>/dev/null
				cat "$tmp/$i.err"; } | tail -n 5 | sed 's/^/    /'
		fi
	done
done <"$tmp/modules"

[ -f "$tmp/failed" ] && failed=1
say "$count Go tests named"
[ "$count" -gt 0 ] || failed=1
if [ "$failed" -ne 0 ]; then say "refused: a Go test the coverage list names must exist and pass (docs/rules/tests.md#runner-no-silent-pass)"; exit 1; fi
say ok
