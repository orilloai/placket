#!/bin/sh
# Self-tests for the protocol's checks in `make ci`: specs-name-behaviour and schemas-hold.
#
# Each case copies the repository without its tests/test_*.sh, plants one fault in the copy with a
# shell command run there, runs `make -k specs-name-behaviour schemas-hold` in it and reads the output.
# A planted fault must make the run fail, and the refusal lines of the two checks must be exactly the
# lines the case names: no line missing and no other. A refusal line is a line starting with a rule's
# name that is not its "ok", a schema's "ok, <counts>" line, or its closing "refused:" line. The clean
# tree must pass, with both checks reporting "ok" and every schema printed with its counts.
# The faults are planted relative to the tree as it is, so they hold as the text grows.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
cases=0
P=spec/protocol.md

# Facts of the tree as it is, which the expected lines are written from.
first=$(grep -n '^### ' "$root/$P" | head -n 1)
first_line=${first%%:*}
first_heading=${first#*:}
first_id=$(printf '%s\n' "$first_heading" | awk '{ print $2 }')
first_title=${first_heading#"### $first_id "}
text_lines=$(awk 'END { print NR }' "$root/$P")
ids_lines=$(awk 'END { print NR }' "$root/spec/ids.txt")
reserved_id=$(head -n 1 "$root/spec/reserved.txt" 2>/dev/null)
next=$((text_lines + 1))

# run <setup>: copy the repository, run <setup> in the copy, then the two checks. Output in $tmp/out,
# exit status in $rc.
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
		(cd "$tmp/repo" && eval "$1") >"$tmp/out" 2>&1 || { echo "setup failed: $1" >>"$tmp/out"; rc=2; return; }
	fi
	(cd "$tmp/repo" && MAKEFLAGS= MAKELEVEL= make -k --no-print-directory specs-name-behaviour schemas-hold) >"$tmp/out" 2>&1
	rc=$?
}

# refusal_lines: the output's refusal lines of the two checks, in order.
refusal_lines() {
	awk '
		index($0, "specs-name-behaviour: ") != 1 && index($0, "schemas-hold: ") != 1 { next }
		$0 == "specs-name-behaviour: ok" || $0 == "schemas-hold: ok" { next }
		/^schemas-hold: spec\/schema\/[^ ]*[.]json: ok, [0-9]+ accepted, [0-9]+ refused$/ { next }
		/^(specs-name-behaviour|schemas-hold): refused: / { next }
		{ print }
	' "$tmp/out"
}

bad() {
	echo "FAIL $name: $1"
	sed 's/^/    | /' "$tmp/out"
	failed=$((failed + 1))
}

# refused <case> <setup> <line>...: with <setup> planted, the checks fail and their refusal lines are
# exactly the lines given, in any order.
refused() {
	name=$1
	setup=$2
	shift 2
	cases=$((cases + 1))
	run "$setup"
	if [ "$rc" -eq 2 ] && grep -q '^setup failed: ' "$tmp/out"; then bad "the fault could not be planted"; return; fi
	if [ "$rc" -eq 0 ]; then bad "the checks passed; they should refuse"; return; fi
	: >"$tmp/want"
	for l in "$@"; do printf '%s\n' "$l" >>"$tmp/want"; done
	refusal_lines | sort >"$tmp/got"
	sort "$tmp/want" -o "$tmp/want"
	if ! cmp -s "$tmp/want" "$tmp/got"; then
		bad "the refusal lines differ from those expected"
		diff "$tmp/want" "$tmp/got" | sed 's/^/    > /'
		return
	fi
	echo "ok   $name"
}

# passes: the clean tree passes, both checks report ok, and every schema is printed with its counts.
passes() {
	name="clean tree"
	cases=$((cases + 1))
	run ""
	if [ "$rc" -ne 0 ]; then bad "the checks failed (exit $rc)"; return; fi
	for l in "specs-name-behaviour: ok" "schemas-hold: ok"; do
		grep -qxF "$l" "$tmp/out" || { bad "no line '$l'"; return; }
	done
	for s in "$root"/spec/schema/*.json; do
		n=${s#"$root/"}
		grep -q "^schemas-hold: $n: ok, [0-9][0-9]* accepted, [1-9][0-9]* refused\$" "$tmp/out" || { bad "no counts line for $n"; return; }
	done
	other=$(refusal_lines)
	if [ -n "$other" ]; then bad "refusal lines on the clean tree: $other"; return; fi
	echo "ok   $name"
}

S=specs-name-behaviour
H=schemas-hold
D=https://json-schema.org/draft/2020-12/schema
U=https://placket.dev/schema/0.1

# specs-name-behaviour: the requirement headings and the two id lists.
refused "heading without its id" \
	"awk -v n=$first_line 'NR == n { sub(/^### [^ ]+ /, \"### \") } { print }' $P >x && mv x $P" \
	"$S: $P:$first_line: a level-3 heading that is not \"### <id> <title>\": ### $first_title" \
	"$S: spec/ids.txt:1: $first_id: no requirement heading in $P"
refused "an id twice" \
	"printf '%s\n\n#### Scenario: again\n' '$first_heading' >>$P" \
	"$S: $P:$next: $first_id: a second heading for this id (the first is at line $first_line)"
refused "a requirement with no scenario" \
	"awk -v n=$first_line 'NR > n && /^(#|##|###) / { past = 1 } !(NR > n && !past && /^#### /) { print }' $P >x && mv x $P" \
	"$S: $P:$first_line: $first_id: no scenario (a \"#### \" heading under it)"
refused "a listed id with no requirement" \
	"echo canonical-99 >>spec/ids.txt" \
	"$S: spec/ids.txt:$((ids_lines + 1)): canonical-99: no requirement heading in $P"
refused "a requirement not listed" \
	"grep -vxF '$first_id' spec/ids.txt >x && mv x spec/ids.txt" \
	"$S: $P:$first_line: $first_id: not listed in spec/ids.txt"
refused "an id listed twice" \
	"echo '$first_id' >>spec/ids.txt" \
	"$S: spec/ids.txt:$((ids_lines + 1)): $first_id: listed twice"
refused "an id both listed and reserved" \
	"echo '$first_id' >>spec/reserved.txt" \
	"$S: spec/ids.txt:1: $first_id: also in spec/reserved.txt" \
	"$S: $P:$first_line: $first_id: a reserved id (spec/reserved.txt)"
refused "a reserved id used" \
	"printf '### %s A reserved requirement\n\n#### Scenario: used\n' '$reserved_id' >>$P" \
	"$S: $P:$next: $reserved_id: a reserved id (spec/reserved.txt)"
refused "a line naming a Go path" \
	"echo 'The question is held in core/mediator until it closes.' >>$P" \
	"$S: $P:$next: names Go code: The question is held in core/mediator until it closes."
refused "a line naming a package-qualified name" \
	"echo 'A release is found by receipts.Releases alone.' >>$P" \
	"$S: $P:$next: names Go code: A release is found by receipts.Releases alone."
refused "a line naming a function" \
	"echo 'It is written as func Resolve.' >>$P" \
	"$S: $P:$next: names Go code: It is written as func Resolve."

# schemas-hold: every schema, its examples and the register.
refused "a refused example moved into accepted" \
	"mv spec/schema/testdata/predicate/refused/block-with-reason-ok.json spec/schema/testdata/predicate/accepted/" \
	"$H: spec/schema/testdata/predicate/accepted/block-with-reason-ok.json: refused by spec/schema/predicate.json"
refused "an accepted example moved into refused" \
	"mv spec/schema/testdata/predicate/accepted/crossing-block-maybe.json spec/schema/testdata/predicate/refused/" \
	"$H: spec/schema/testdata/predicate/refused/crossing-block-maybe.json: accepted by spec/schema/predicate.json"
refused "a schema no register row names" \
	"printf '{\"\$schema\":\"$D\",\"\$id\":\"$U/extra.json\",\"type\":\"string\"}\n' >spec/schema/extra.json" \
	"$H: spec/schema/extra.json: no row of spec/frozen-formats.md names it"
refused "a schema that is not JSON" \
	"printf '{\"type\": \"string\",\n' >spec/schema/broken.json" \
	"$H: spec/schema/broken.json: not JSON"
refused "a schema of another draft" \
	"printf '{\"\$schema\":\"http://json-schema.org/draft-07/schema#\",\"\$id\":\"$U/old.json\",\"type\":\"string\"}\n' >spec/schema/old.json" \
	"$H: spec/schema/old.json: not a JSON Schema draft 2020-12 schema: its \$schema is not $D"
refused "JSON that names no draft" \
	"printf '[\"not\", \"a\", \"schema\"]\n' >spec/schema/list.json" \
	"$H: spec/schema/list.json: not a JSON Schema draft 2020-12 schema: its \$schema is not $D"
refused "a schema whose id is not its file" \
	"printf '{\"\$schema\":\"$D\",\"\$id\":\"$U/other.json\",\"type\":\"string\"}\n' >spec/schema/named.json" \
	"$H: spec/schema/named.json: its \$id is not $U/named.json"
refused "a schema that would fetch" \
	"printf '{\"\$schema\":\"$D\",\"\$id\":\"$U/remote.json\",\"\$ref\":\"https://example.com/x.json\"}\n' >spec/schema/remote.json" \
	"$H: spec/schema/remote.json: names a schema outside $U/: https://example.com/x.json"
refused "a schema that does not compile" \
	"printf '{\"\$schema\":\"$D\",\"\$id\":\"$U/typo.json\",\"type\":5}\n' >spec/schema/typo.json &&
	 printf '| Typo | none | \`spec/schema/typo.json\` | 0.1, unreleased | nothing | nothing |\n' >>spec/frozen-formats.md &&
	 mkdir -p spec/schema/testdata/typo/accepted spec/schema/testdata/typo/refused &&
	 echo '\"a\"' >spec/schema/testdata/typo/accepted/a.json && echo 1 >spec/schema/testdata/typo/refused/one.json" \
	"$H: spec/schema/typo.json: does not compile as JSON Schema draft 2020-12 with nothing fetched"
refused "a schema with no refused example" \
	"rm -r spec/schema/testdata/hash/refused" \
	"$H: spec/schema/hash.json: no refused example in spec/schema/testdata/hash/refused/"
refused "examples of no schema" \
	"mkdir -p spec/schema/testdata/ghost/accepted && echo '{}' >spec/schema/testdata/ghost/accepted/a.json" \
	"$H: spec/schema/testdata/ghost/: examples of no schema; there is no spec/schema/ghost.json"
refused "an example that is not JSON" \
	"printf '{\"seq\":' >spec/schema/testdata/envelope/refused/cut-short.json" \
	"$H: spec/schema/testdata/envelope/refused/cut-short.json: not JSON"
passes

echo "$cases cases, $failed failed"
[ "$failed" -eq 0 ]
