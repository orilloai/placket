#!/bin/sh
# The planted-fault test of vectors-independent: a vector's expected bytes never come from the code
# the vector tests.
#
# For each group of the table "Groups and the packages they test" in vectors/README.md, and each
# core package its row names, it copies the repository without .git, plants that package's fault
# below in the copy, runs the group's generator there, and checks that no byte of the group's vectors
# changed and that the conformance runner fails on a vector of the group. A group's generator is the
# Go command in vectors/<group>/, run from vectors/ as `go run ./<group>`. Before any fault, the
# generators run on a clean copy must write exactly the vectors committed. It fails when the table
# lists no group, when a group has no generator, when a package of a row has no fault here, and when
# a fault's text is not found exactly once in its file, so a fault that no longer applies is never
# taken for one that held.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
checked=0
say() { echo "vectors-independent test: $*"; }
bad() { say "FAIL: $*"; failed=1; }

# fault <package>: set file, old and new to the file, the text and its replacement that plant one
# fault in the package: a change of logic that compiles and that some vector of every group testing
# the package notices.
fault() {
	case "$1" in
	canonical) file=core/canonical/canonical.go old='return a[i] < b[i]' new='return a[i] > b[i]' ;;
	signing) file=core/signing/signing.go old='b.WriteString("DSSEv1 ")' new='b.WriteString("DSSEv2 ")' ;;
	setup) file=core/setup/setup.go old='return until <= at' new='return until < at' ;;
	composition) file=core/composition/composition.go old='if sent == 0 || got == 0 {' new='if sent == 0 && got == 0 {' ;;
	receipts) file=core/receipts/receipts.go old='"application/vnd.in-toto+json"' new='"application/vnd.in-toto+jsom"' ;;
	mediator) file=core/mediator/mediator.go old='next := link{seq: last.seq + 1}' new='next := link{seq: last.seq + 2}' ;;
	*) return 1 ;;
	esac
}

# copy <dir>: the repository, without .git, in <dir>.
copy() {
	rm -rf "$1"
	mkdir -p "$1"
	for f in "$root"/* "$root"/.[!.]*; do
		[ -e "$f" ] || continue
		[ "${f##*/}" = .git ] && continue
		cp -R "$f" "$1/"
	done
}

# generate <tree> <group>: run the group's generator in the tree.
generate() { (cd "$1/vectors" && go run "./$2") >"$tmp/gen.out" 2>&1; }

table=$(awk -F'|' '/^## / { on = ($0 == "## Groups and the packages they test") }
	on && NF >= 4 && $2 ~ /^ *`[a-z0-9-]+` *$/ { g = $2; gsub(/[ `]/, "", g); p = $(NF - 1); gsub(/[ `]/, "", p); print g " " p }' \
	"$root/vectors/README.md" 2>/dev/null)
if [ -z "$table" ]; then
	say "FAIL: vectors/README.md lists no group"
	exit 1
fi

groups=$(printf '%s\n' "$table" | awk '{ print $1 }')
for g in $groups; do
	if ! grep -lqs '^package main$' "$root/vectors/$g"/*.go; then
		bad "$g: no generator; a group's generator is the Go command in vectors/$g/"
	fi
done
[ "$failed" -eq 0 ] || exit 1

# The generators reproduce the committed vectors.
copy "$tmp/clean"
for g in $groups; do
	if ! generate "$tmp/clean" "$g"; then
		bad "$g: the generator failed"
		sed 's/^/    /' "$tmp/gen.out"
	elif ! diff -r "$root/vectors/$g" "$tmp/clean/vectors/$g" >"$tmp/diff"; then
		bad "$g: the generator does not write the committed vectors"
		head -n 20 "$tmp/diff" | sed 's/^/    /'
	fi
done
[ "$failed" -eq 0 ] || exit 1

printf '%s\n' "$table" | while read -r g pkgs; do
	for p in $(printf '%s' "$pkgs" | tr ',' ' '); do
		if ! fault "$p"; then
			bad "$g: no fault is written here for the package $p"
			echo fail >>"$tmp/failed"
			continue
		fi
		copy "$tmp/fault"
		if ! awk -v old="$old" -v new="$new" '
			{ i = index($0, old); if (i) { $0 = substr($0, 1, i - 1) new substr($0, i + length(old)); n++ } print }
			END { if (n != 1) exit 3 }' "$root/$file" >"$tmp/fault/$file"; then
			bad "$g, $p: the fault's text is not in $file exactly once"
			echo fail >>"$tmp/failed"
			continue
		fi
		if ! generate "$tmp/fault" "$g"; then
			bad "$g, $p: the generator failed with the fault planted"
			sed 's/^/    /' "$tmp/gen.out"
			echo fail >>"$tmp/failed"
			continue
		fi
		if ! diff -r "$root/vectors/$g" "$tmp/fault/vectors/$g" >"$tmp/diff"; then
			bad "$g, $p: a vector byte changed with a fault in $p"
			head -n 20 "$tmp/diff" | sed 's/^/    /'
			echo fail >>"$tmp/failed"
			continue
		fi
		(cd "$tmp/fault/core/conformance" && go test -count=1 -run '^TestVectors$' .) >"$tmp/run" 2>&1
		if grep -q "^runner-no-silent-pass: fail: vectors/$g/" "$tmp/run"; then
			say "ok   $g, $p: no vector byte changed, and the runner fails on $(grep -c "^runner-no-silent-pass: fail: vectors/$g/" "$tmp/run") of the group's vectors"
			echo ok >>"$tmp/checked"
		else
			bad "$g, $p: the runner fails on no vector of the group with a fault in $p"
			grep -m 5 'fail\|FAIL' "$tmp/run" | sed 's/^/    /'
			echo fail >>"$tmp/failed"
		fi
	done
done

[ -f "$tmp/failed" ] && failed=1
[ -f "$tmp/checked" ] && checked=$(grep -c ok "$tmp/checked")
say "$checked faults planted"
[ "$checked" -gt 0 ] || failed=1
if [ "$failed" -ne 0 ]; then say "refused: a vector depends on the code it tests (AGENTS.md, vectors-independent)"; exit 1; fi
say ok
