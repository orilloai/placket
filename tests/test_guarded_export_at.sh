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
cleanup() {
	# A run still going when the test exits ends on its own: its supervisor, below, sees that its parent
	# has gone and kills its group.
	rm -rf "$tmp" "$short"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
out=$tmp/out

# SUPERVISOR is a perl program, run as the shell's own child: SHELL LIMIT REPORT COMMAND... It makes
# itself the leader of a new process group and runs COMMAND in that group. It ends when COMMAND ends,
# when LIMIT seconds pass, or when its parent is no longer SHELL, which has then exited. It writes to
# REPORT one line: COMMAND's exit status (128 plus the signal, if a signal ended it), late, or
# orphaned, then a space and the number of other processes still in the group. Then it kills the
# whole group, itself with it. Only the leader signals its group, and only while it is alive: its id
# cannot be another's until the shell reaps it, so no signal reaches a group id that has been freed.
# The shell sends no group signal at all.
SUPERVISOR='
use POSIX qw(WNOHANG);
my ($shell, $limit, $report, @command) = @ARGV;
setpgrp(0, 0) or die "setpgrp: $!\n";
my $child = fork();
defined $child or die "fork: $!\n";
if ($child == 0) { exec @command or die "exec $command[0]: $!\n" }
my $end = time() + $limit;
my $how;
until (defined $how) {
	if (waitpid($child, WNOHANG) == $child) { $how = $? & 127 ? 128 + ($? & 127) : $? >> 8 }
	elsif (getppid() != $shell) { $how = "orphaned" }
	elsif (time() >= $end) { $how = "late" }
	else { select(undef, undef, undef, 0.1) }
}
my $left = "unknown";
my $ps = open(my $list, "-|", "ps", "-A", "-o", "pid=", "-o", "pgid=");
if ($ps) {
	my $n = 0;
	while (<$list>) { my ($pid, $group) = split; $n++ if $group == $$ && $pid != $$ && $pid != $ps }
	$left = $n if close $list;
}
if (open(my $out, ">", $report)) { print $out "$how $left\n"; close $out }
kill "-KILL", $$;
'

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
	PATH=$tmp/bin:$PATH TMPDIR=$short exec env -u PLACKET_JEV_KEY perl -e "$SUPERVISOR" "$$" "$LIMIT" "$tmp/report" sh pattern.sh
) >"$out" 2>&1 </dev/null &
# The supervisor's status is always KILL's, and the shell's note of it is noise: the report says how
# the run ended.
wait "$!" 2>/dev/null
how= left=
read -r how left 2>/dev/null <"$tmp/report"

grep -E '^(guard|export): ' "$pattern/expected.txt" >"$tmp/want"
grep -E '^(guard|export): ' "$out" >"$tmp/got"
if [ "$how" != 0 ]; then
	case $how in
	late) echo "FAIL guarded-export at /payload: did not end within ${LIMIT}s" ;;
	'') echo "FAIL guarded-export at /payload: ended before its supervisor could report" ;;
	*) echo "FAIL guarded-export at /payload: exit $how" ;;
	esac
	sed 's/^/    | /' "$out"
	exit 1
fi
if [ "$left" != 0 ]; then
	echo "FAIL guarded-export at /payload: left processes running: $left"
	exit 1
fi
if ! cmp -s "$tmp/want" "$tmp/got"; then
	echo "FAIL guarded-export at /payload: its guard decides otherwise than for the whole part"
	diff -u "$tmp/want" "$tmp/got" | sed 's/^/    | /'
	exit 1
fi
echo "ok   guarded-export at /payload: the guard decides each of $(wc -l <"$tmp/want" | tr -d ' ') lines as for the whole part"
