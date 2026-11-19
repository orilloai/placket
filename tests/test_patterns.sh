#!/bin/sh
# The patterns' test: every patterns/<job>/pattern.sh runs as it is and prints its expected.txt.
#
# First the shape patterns/README.md states: the README is there; every directory under patterns/ is a
# job, with its README.md, pattern.sh and expected.txt; the job's README has the four sections in
# order; its pattern.sh marks the lines a caller changes; and the README's list names every job once,
# with the Placket version it was last checked against, and no job that is not there.
#
# Then each pattern runs, one at a time, on the binary this tree builds and nothing else of the tree:
# from a fresh copy of its own directory, with the binary's directory first on PATH and a fresh TMPDIR
# of its own, short enough for a socket's path, and with no PLACKET_JEV_KEY. It must exit 0 within LIMIT seconds, print exactly its
# expected.txt (standard output and standard error together) and leave no process running. It runs in a
# process group of its own, which is killed whatever happens, so a pattern that fails, hangs or is
# interrupted leaves no window or socket behind. With no patterns it passes, and builds nothing.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
patterns=$root/patterns
LIMIT=60
tmp=$(mktemp -d)
# A unix socket's path is short on some systems: each pattern's TMPDIR lives under a short directory.
short=$(mktemp -d /tmp/plkp.XXXXXX)
group=
watchdog=
cleanup() {
	[ -n "$group" ] && kill -KILL -"$group" 2>/dev/null
	[ -n "$watchdog" ] && kill -KILL -"$watchdog" 2>/dev/null
	rm -rf "$tmp" "$short"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
failed=0
out=$tmp/out

fail() {
	echo "FAIL $1"
	failed=1
}

# LEADER is a perl program that makes itself the leader of a new process group and then becomes the
# command its arguments name: run as "exec perl -e "$LEADER" CMD..." in a subshell started with &, the
# command's process id, $!, is also its group's id.
LEADER='setpgrp(0, 0) or die "setpgrp: $!\n"; exec @ARGV or die "exec $ARGV[0]: $!\n"'

# The shape.
if [ ! -f "$patterns/README.md" ]; then
	echo "FAIL patterns/README.md: missing; it says what a pattern is and lists every one"
	exit 1
fi
jobs=
for d in "$patterns"/*; do
	[ -d "$d" ] || continue
	job=${d##*/}
	case $job in
	*[!a-z0-9-]* | -*)
		fail "patterns/$job/: a job's name is lower-case letters, digits and hyphens"
		continue
		;;
	esac
	jobs="$jobs $job"
	for f in README.md pattern.sh expected.txt; do
		[ -f "$d/$f" ] || fail "patterns/$job/$f: missing"
	done
	if [ -f "$d/README.md" ]; then
		got=$(grep '^## ' "$d/README.md" | head -n 4)
		want='## The question
## When to use it
## What it relies on
## What it claims'
		[ "$got" = "$want" ] || fail "patterns/$job/README.md: its first sections are not, in order: $(echo "$want" | sed 's/^## //' | paste -sd, - | sed 's/,/, /g')"
	fi
	if [ -f "$d/pattern.sh" ] && ! grep -q '# CHANGE' "$d/pattern.sh"; then
		fail "patterns/$job/pattern.sh: no line is marked '# CHANGE' for a caller to change"
	fi
done
# The list: in the section "## The patterns", up to the next heading, one line per job,
# "- `<job>`: <the job it does>. Checked against placket <version>."
awk '/^## / { in_list = ($0 == "## The patterns"); next } in_list' "$patterns/README.md" >"$tmp/list"
listed=$(sed -n 's/^- `\([^`]*\)`: .*$/\1/p' "$tmp/list")
for job in $(printf "%s\n" "$listed" | sort -u); do
	[ -d "$patterns/$job" ] || fail "patterns/README.md: lists $job, which is not a directory under patterns/"
	[ "$(printf '%s\n' "$listed" | grep -Fcx "$job")" -eq 1 ] || fail "patterns/README.md: lists $job more than once"
	grep -q '^- `'"$job"'`: .*[^ ]\. Checked against placket [^ ][^ ]*\.$' "$tmp/list" ||
		fail "patterns/README.md: the line of $job does not end 'Checked against placket <version>.'"
done
for job in $jobs; do
	printf '%s\n' "$listed" | grep -Fqx "$job" || fail "patterns/README.md: does not list $job"
done
if [ "$failed" -ne 0 ]; then
	exit 1
fi
echo "ok   every pattern has the shape patterns/README.md states"

if [ -z "$jobs" ]; then
	echo "ok   no patterns yet"
	exit 0
fi

command -v jq >/dev/null 2>&1 || { echo "FAIL jq is not on PATH; a pattern needs it"; exit 1; }
mkdir "$tmp/bin"
(cd "$root" && go build -o "$tmp/bin/placket" ./cmd/placket) || { echo "FAIL this tree does not build"; exit 1; }

for job in $jobs; do
	run=$tmp/run/$job
	mkdir -p "$run" "$short/$job"
	cp -R "$patterns/$job/." "$run/"
	(
		cd "$run" || exit 1
		# No pattern reaches a judge's host, even where the caller's shell holds a key.
		PATH=$tmp/bin:$PATH TMPDIR=$short/$job exec env -u PLACKET_JEV_KEY perl -e "$LEADER" sh pattern.sh
	) >"$out" 2>&1 </dev/null &
	group=$!
	(exec perl -e "$LEADER" sh -c "sleep $LIMIT; kill -KILL -$group") >/dev/null 2>&1 &
	watchdog=$!
	wait "$group"
	rc=$?
	kill -KILL -"$watchdog" 2>/dev/null
	wait "$watchdog" 2>/dev/null
	watchdog=
	left=0
	if kill -0 -"$group" 2>/dev/null; then
		left=1
		kill -KILL -"$group" 2>/dev/null
	fi
	group=
	if [ "$rc" -ne 0 ]; then
		if [ "$rc" -eq 137 ]; then fail "$job: killed, or did not end within ${LIMIT}s"; else fail "$job: exit $rc"; fi
		sed 's/^/    | /' "$out"
	elif ! cmp -s "$run/expected.txt" "$out"; then
		fail "$job: printed what expected.txt does not say"
		diff -u "$run/expected.txt" "$out" | sed 's/^/    | /'
	elif [ "$left" -ne 0 ]; then
		fail "$job: left a process running; a pattern stops its own window"
	else
		echo "ok   $job"
	fi
	rm -rf "$short/$job"
done
[ "$failed" -eq 0 ]
