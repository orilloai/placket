#!/bin/sh
# The wiring test of the checker in tools/guard/: that make runs each of its rules as a target of its
# own, that each target runs the rule it is named for and no other, and that every step prints its
# time. The rules themselves are proved by the checker's Go tests.
#
# In one copy of the repository's tracked files, without its tests/test_*.sh, it plants one fault for each rule at
# once, lays the fixture tests/testdata/wiring/ for those a file can hold, and runs
# `make -k guard <every rule> modules` once. Each rule must print a refusal naming its own fault and
# nothing else, then its closing "refused:" line and its time; modules must still list the core (the
# planted require and the root module's require and replace keep go list working); and the checker's
# build and modules must print their times. Then, in the same copy, a make ci started with the marker
# already set must stop before any step.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0
repo=$tmp/repo
out=$tmp/out

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$out"
	failed=$((failed + 1))
}

mkdir "$repo"
# The copy holds the files git tracks, as a clean checkout does, so a file only the writer's checkout
# holds, such as a binary built there, plays no part. Outside a work tree, it holds every file.
if [ "$(git -C "$root" rev-parse --show-toplevel 2>/dev/null)" = "$(cd "$root" && pwd -P)" ]; then
	git -C "$root" -c core.quotePath=false ls-files --cached | (
		cd "$root" && while IFS= read -r f; do
			if [ -e "$f" ] || [ -L "$f" ]; then printf '%s\n' "$f"; fi
		done | tar -cf - -T -
	) | tar -C "$repo" -xf -
else
	for f in "$root"/* "$root"/.[!.]*; do
		[ -e "$f" ] || continue
		[ "${f##*/}" = .git ] && continue
		cp -R "$f" "$repo/"
	done
fi
rm -f "$repo"/tests/test_*.sh
cp -R "$root/tests/testdata/wiring/." "$repo/"

# The faults a fixture cannot hold, planted relative to the tree as it is.
cd "$repo" || exit 1
set -e
# core-no-deps: the core requires a module.
printf '\nrequire example.com/dep v1.0.0\n' >>core/go.mod
# root-module: a root module whose replace names the core by another path than ./core. Its name and
# its require are kept, and the path still leads to the core, so go list still works once the
# adapters import packages of the root module.
printf 'module github.com/orilloai/placket\n\ngo 1.27\n\nrequire github.com/orilloai/placket/core v0.0.0\n\nreplace github.com/orilloai/placket/core => ./core/../core\n' >go.mod
# vectors-independent: a generator of the first group of the vectors' table imports a package the
# group tests, in a file no build reads.
row=$(awk -F '|' '/^## / { on = ($0 == "## Groups and the packages they test") }
	on && NF >= 4 && $2 ~ /^ *`[a-z0-9-]+` *$/ { g = $2; p = $(NF - 1); gsub(/[ `]/, "", g); gsub(/[ `]/, "", p); sub(/,.*/, "", p); if (p != "") { print g, p; exit } }' vectors/README.md)
group=${row% *}
pkg=${row#* }
printf '//go:build ignore\n\npackage main\n\nimport _ "github.com/orilloai/placket/core/%s"\n' "$pkg" >"vectors/$group/wiring.go"
# specs-name-behaviour: a line of the requirement text names Go code.
echo 'The question is held in core/mediator until it closes.' >>spec/protocol.md
# schemas-hold: the first refused example of the first schema moved among its accepted ones.
example=$(ls spec/schema/testdata/*/refused/*.json | head -n 1)
schema=${example#spec/schema/testdata/}
schema=${schema%%/*}
mv "$example" "spec/schema/testdata/$schema/accepted/"
moved=spec/schema/testdata/$schema/accepted/${example##*/}
# agents-md-short: the entry file over its 150 lines.
i=0; while [ "$i" -lt 151 ]; do echo x; i=$((i + 1)); done >>AGENTS.md
# no-hidden-text: a bidi control character in a Markdown file.
printf 'An \342\200\256 override\n' >docs/hidden.md
# machine-rules-in-ci: secrets left out of ci's prerequisites; the fixture's workflow also uses an
# action by its tag.
awk '/^ci:/ && !/=/ { on = 1 } on { line = $0; gsub(/ secrets( |$)/, " ", line); print line; if ($0 !~ /\\$/) on = 0; next } { print }' Makefile >Makefile.new
mv Makefile.new Makefile
set +e

# <rule> <file>...: the files each rule's refusal lines must name.
expect='core-no-deps core/go.mod
core-imports core/wiring/up.go
core-no-world core/wiring/clock.go
core-no-panic core/wiring/panic.go
specs-name-behaviour spec/protocol.md
schemas-hold '"$moved"'
vectors-independent vectors/'"$group"'/wiring.go
constants-once adapters/wiring/uri.go core/wiring/uri.go
product-words-once core/wiring/words.go
claims-stop-at-crossing adapters/wiring/say.go
main-is-thin cmd/placket/main.go
recipes-core-only recipes/wiring/up.go
root-module go.mod
machine-rules-in-ci Makefile .github/workflows/wiring.yml
ci-stays-fast scripts/clear.sh
no-hidden-text docs/hidden.md
agents-md-short AGENTS.md'
rules=$(printf '%s\n' "$expect" | awk '{ print $1 }')

name="one fault per rule"
cases=$((cases + 1))
MAKEFLAGS= MAKELEVEL= make -k --no-print-directory guard $rules modules >"$out" 2>&1
rc=$?
fail=
if [ "$rc" -eq 0 ]; then fail="make passed; every rule should refuse"; fi
printf '%s\n' "$expect" >"$tmp/expect"
while read -r rule files; do
	[ -z "$fail" ] || break
	# The rule's refusal lines: its lines but its ok, its closing line and schemas-hold's counts.
	lines=$(awk -v r="$rule: " 'index($0, r) == 1 && $0 != r "ok" && index($0, r "refused: ") != 1 && !/^schemas-hold: spec\/schema\/[^ ]*[.]json: ok, /' "$out")
	if [ -z "$lines" ]; then fail="$rule printed no refusal"; break; fi
	for f in $files; do
		printf '%s\n' "$lines" | awk -v p="$rule: $f" 'index($0, p ":") == 1 { found = 1 } END { exit !found }' || { fail="$rule did not name $f"; break; }
	done
	[ -z "$fail" ] || break
	other=$(printf '%s\n' "$lines" | awk -v r="$rule: " -v fs="$files" '
		BEGIN { n = split(fs, f, " ") }
		{ for (i = 1; i <= n; i++) if (index($0, r f[i] ":") == 1) next; print }')
	if [ -n "$other" ]; then fail="$rule refused something it was not planted: $other"; break; fi
	# Its closing line, and on the next line its time.
	if ! awk -v r="$rule" '
		prev { if ($0 ~ "^time: " r ": [0-9]+s$") found = 1; prev = 0 }
		index($0, r ": refused: ") == 1 && $0 ~ "[(]docs/rules/[a-z]+[.]md#" { prev = 1 }
		END { exit !found }' "$out"; then fail="$rule's closing line is not followed by its time"; fi
done <"$tmp/expect"
if [ -z "$fail" ]; then
	if ! awk '$0 == "== ./core" { found = 1 } END { exit !found }' "$out"; then fail="modules did not list ./core"
	elif ! grep -q '^time: guard: [0-9][0-9]*s$' "$out"; then fail="the checker's build printed no time"
	elif ! grep -q '^time: modules: [0-9][0-9]*s$' "$out"; then fail="modules printed no time"
	fi
fi
if [ -n "$fail" ]; then bad "$fail"; else echo "ok   $name: each rule refuses its own fault, and only it"; fi

# ci-stays-fast: with the marker set, a make ci stops before any step, whatever -k says.
name="make ci inside make ci"
cases=$((cases + 1))
PLACKET_IN_CI=1 MAKEFLAGS= MAKELEVEL= make -k --no-print-directory ci >"$out" 2>&1
rc=$?
if [ "$rc" -eq 0 ]; then bad "make ci passed"
elif ! grep -qxF 'ci-stays-fast: refused: make ci inside make ci' "$out"; then bad "no line 'ci-stays-fast: refused: make ci inside make ci'"
elif grep -q '^time: \|^guard: ' "$out"; then bad "a step ran"
else echo "ok   $name: refused before any step"; fi

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
