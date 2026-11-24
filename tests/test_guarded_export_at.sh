#!/bin/sh
# patterns/guarded-export with its value inside a body: the guard finds the value at `at` in each part
# and decides each record as it does when the record is the whole part.
#
# A copy of the pattern is changed so that the proposer gives, and the approver changes to, each
# record wrapped as {"summary":"a price","payload":<record>}, and at is /payload. The test first checks
# that each change took, so an edit that matched nothing cannot pass. The copy then runs as
# tests/test_patterns.sh runs a pattern: on the binary this tree builds, first on PATH, in a process
# group of its own that is killed whatever happens, with a TMPDIR of its own short enough for a
# socket's path and no PLACKET_JEV_KEY. It must exit 0 within LIMIT seconds, leave no process
# running, and print exactly the guard: and export: lines of the pattern's expected.txt. Its other
# lines show the wrapped body, and differ by design. The pattern itself is left as it is.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
pattern=$root/patterns/guarded-export
# The time limit of tests/test_patterns.sh.
LIMIT=240
command -v jq >/dev/null 2>&1 || { echo "FAIL jq is not on PATH; the pattern needs it"; exit 1; }
tmp=$(mktemp -d)
# A unix socket's path is short on some systems: the pattern's TMPDIR lives under a short directory.
short=$(mktemp -d /tmp/plka.XXXXXX)
group=
watchdog=
cleanup() {
	[ -n "$group" ] && kill -KILL -"$group" 2>/dev/null
	[ -n "$watchdog" ] && kill -KILL -"$watchdog" 2>/dev/null
	rm -rf "$tmp" "$short"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
out=$tmp/out

# LEADER is a perl program that makes itself the leader of a new process group and then becomes the
# command its arguments name, so the command's process id, $!, is also its group's id.
LEADER='setpgrp(0, 0) or die "setpgrp: $!\n"; exec @ARGV or die "exec $ARGV[0]: $!\n"'

# The copy, with each record wrapped in a body. The lines added are read from files, so that the
# edit itself quotes nothing.
run=$tmp/run
mkdir "$run"
cp -R "$pattern/." "$run/"
cat >"$tmp/wrap-proposal" <<'EOF'
	printf '{"summary":"a price","payload":%s}\n' "$(cat "$1.sent")" >"$1.wrapped"
EOF
cat >"$tmp/wrap-version" <<'EOF'
printf '{"summary":"a price","payload":%s}\n' "$(cat tb-200.version)" >tb-200.wrapped
EOF
awk -v proposal="$tmp/wrap-proposal" -v version="$tmp/wrap-version" '
	# swap <from> <to>: $0 with its first <from> replaced by <to>, compared as text.
	function swap(from, to,   i) {
		i = index($0, from)
		if (i) $0 = substr($0, 1, i - 1) to substr($0, i + length(from))
	}
	function insert(file,   l) { while ((getline l <file) > 0) print l; close(file) }
	index($0, "at= ") == 1 { $0 = "at=/payload " substr($0, 5) }
	{ swap("--file \"$1.sent\"", "--file \"$1.wrapped\""); swap("--file tb-200.version", "--file tb-200.wrapped") }
	{ print }
	$0 == "\tnormalize <\"$1\" >\"$1.sent\"" { insert(proposal) }
	index($0, "| normalize >tb-200.version") { insert(version) }
' "$pattern/pattern.sh" >"$run/pattern.sh"
# held <text>: the copy holds <text>, as a whole line of it or within one.
held() {
	grep -q -F -e "$1" "$run/pattern.sh" || { echo "FAIL the copy of patterns/guarded-export does not hold $1; the edit matched nothing"; exit 1; }
}
held 'at=/payload'
held '--file "$1.wrapped"'
held '--file tb-200.wrapped'
held '>"$1.wrapped"'
held '>tb-200.wrapped'

mkdir "$tmp/bin"
# The build runs at the lowest priority: make ci runs the shell tests side by side.
(cd "$root" && nice -n 19 go build -o "$tmp/bin/placket" ./cmd/placket) || { echo "FAIL this tree does not build"; exit 1; }

(
	cd "$run" || exit 1
	# No pattern reaches a judge's host, even where the caller's shell holds a key.
	PATH=$tmp/bin:$PATH TMPDIR=$short exec env -u PLACKET_JEV_KEY perl -e "$LEADER" sh pattern.sh
) >"$out" 2>&1 </dev/null &
group=$!
(exec perl -e "$LEADER" sh -c "sleep $LIMIT; kill -KILL -$group") >/dev/null 2>&1 &
watchdog=$!
wait "$group"
rc=$?
# The watchdog's id is cleared as its group is killed, before the reap that frees it for another.
kill -KILL -"$watchdog" 2>/dev/null
w=$watchdog watchdog=
wait "$w" 2>/dev/null
left=0
if kill -0 -"$group" 2>/dev/null; then
	left=1
	kill -KILL -"$group" 2>/dev/null
fi
group=

grep -E '^(guard|export): ' "$pattern/expected.txt" >"$tmp/want"
grep -E '^(guard|export): ' "$out" >"$tmp/got"
if [ "$rc" -ne 0 ]; then
	if [ "$rc" -eq 137 ]; then echo "FAIL guarded-export at /payload: killed, or did not end within ${LIMIT}s"; else echo "FAIL guarded-export at /payload: exit $rc"; fi
	sed 's/^/    | /' "$out"
	exit 1
fi
if [ "$left" -ne 0 ]; then
	echo "FAIL guarded-export at /payload: left a process running"
	exit 1
fi
if ! cmp -s "$tmp/want" "$tmp/got"; then
	echo "FAIL guarded-export at /payload: its guard decides otherwise than for the whole part"
	diff -u "$tmp/want" "$tmp/got" | sed 's/^/    | /'
	exit 1
fi
echo "ok   guarded-export at /payload: the guard decides each of $(wc -l <"$tmp/want" | tr -d ' ') lines as for the whole part"
