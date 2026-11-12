#!/bin/sh
# The planted-fault test of vectors-independent: a vector's expected bytes never come from the code
# the vector tests.
#
# For each group of the table "Groups and the packages they test" in vectors/README.md, each core
# package its row names, and each of that package's faults in the list below, it copies the
# repository without .git, plants the fault in the copy, runs the group's generator there, and checks
# that no byte of the group's vectors changed and that the conformance runner fails on a vector of
# the group. A group's generator is the
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

# The planted faults, one a line: the package, the file, the text and its replacement, separated by
# tabs. Each is a change of logic that compiles and that some vector of every group testing the
# package notices. A package may have more than one; the mediator has one in Give and one in Resolve.
faults='canonical	core/canonical/canonical.go	return a[i] < b[i]	return a[i] > b[i]
signing	core/signing/signing.go	b.WriteString("DSSEv1 ")	b.WriteString("DSSEv2 ")
setup	core/setup/setup.go	return until <= at	return until < at
composition	core/composition/composition.go	if sent == 0 || got == 0 {	if sent == 0 && got == 0 {
receipts	core/receipts/receipts.go	"application/vnd.in-toto+json"	"application/vnd.in-toto+jsom"
mediator	core/mediator/mediator.go	next := link{seq: last.seq + 1}	next := link{seq: last.seq + 2}
mediator	core/mediator/questions.go	digest := receipts.Digest(r.Salt, []byte(r.Answer))	digest := receipts.Digest([]byte(r.Answer), r.Salt)'

# faultsOf <package>: the package's lines of the list.
faultsOf() { printf '%s\n' "$faults" | awk -F'\t' -v p="$1" '$1 == p'; }

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

# plant <group> <package> <file> <old> <new> <label>: plant one fault and judge the group under it.
plant() {
	_g=$1 _p=$2 _file=$3 _old=$4 _new=$5 _l=$6
	copy "$tmp/fault"
	if ! awk -v old="$_old" -v new="$_new" '
		{ i = index($0, old); if (i) { $0 = substr($0, 1, i - 1) new substr($0, i + length(old)); n++ } print }
		END { if (n != 1) exit 3 }' "$root/$_file" >"$tmp/fault/$_file"; then
		bad "$_g, $_l: the fault's text is not in $_file exactly once"
		echo fail >>"$tmp/failed"
		return
	fi
	if ! generate "$tmp/fault" "$_g"; then
		bad "$_g, $_l: the generator failed with the fault planted"
		sed 's/^/    /' "$tmp/gen.out"
		echo fail >>"$tmp/failed"
		return
	fi
	if ! diff -r "$root/vectors/$_g" "$tmp/fault/vectors/$_g" >"$tmp/diff"; then
		bad "$_g, $_l: a vector byte changed with a fault in $_p"
		head -n 20 "$tmp/diff" | sed 's/^/    /'
		echo fail >>"$tmp/failed"
		return
	fi
	(cd "$tmp/fault/core/conformance" && go test -count=1 -run '^TestVectors$' .) >"$tmp/run" 2>&1 </dev/null
	if grep -q "^runner-no-silent-pass: fail: vectors/$_g/" "$tmp/run"; then
		say "ok   $_g, $_l: no vector byte changed, and the runner fails on $(grep -c "^runner-no-silent-pass: fail: vectors/$_g/" "$tmp/run") of the group's vectors"
		echo ok >>"$tmp/checked"
	else
		bad "$_g, $_l: the runner fails on no vector of the group with a fault in $_p"
		grep -m 5 'fail\|FAIL' "$tmp/run" | sed 's/^/    /'
		echo fail >>"$tmp/failed"
	fi
}

printf '%s\n' "$table" | while read -r g pkgs; do
	for p in $(printf '%s' "$pkgs" | tr ',' ' '); do
		n=$(faultsOf "$p" | wc -l | tr -d ' ')
		if [ "$n" -eq 0 ]; then
			bad "$g: no fault is written here for the package $p"
			echo fail >>"$tmp/failed"
			continue
		fi
		i=1
		while [ "$i" -le "$n" ]; do
			line=$(faultsOf "$p" | sed -n "${i}p")
			label=$p
			[ "$n" -gt 1 ] && label="$p, fault $i of $n"
			plant "$g" "$p" "$(printf '%s' "$line" | cut -f2)" "$(printf '%s' "$line" | cut -f3)" "$(printf '%s' "$line" | cut -f4)" "$label" </dev/null
			i=$((i + 1))
		done
	done
done

[ -f "$tmp/failed" ] && failed=1
[ -f "$tmp/checked" ] && checked=$(grep -c ok "$tmp/checked")
say "$checked faults planted"
[ "$checked" -gt 0 ] || failed=1
if [ "$failed" -ne 0 ]; then say "refused: a vector depends on the code it tests (AGENTS.md, vectors-independent)"; exit 1; fi
say ok
