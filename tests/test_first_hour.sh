#!/bin/sh
# The first hour of each of placket's three entry points, from a clean HOME, as a person would spend
# it (docs/adr/0012-the-mediator-api-and-the-command-line.md): a, approve before apply; b, an out-gate on a merge; c, handoffs
# between agents across two windows, whose journals one mediator key signs.
#
# Usage: sh tests/test_first_hour.sh [a|b|c]. With no argument, as make ci runs it, it runs the three
# side by side; with one, that entry alone.
#
# First the install, once: the host's archive, built from the tree as .goreleaser.yaml builds it
# (CGO_ENABLED=0, -trimpath, -ldflags "-s -w"), a tar of placket and the files its archive's files
# list names, read from .goreleaser.yaml. Then each entry, in a process group of its own that is
# killed whatever happens, with a HOME of its own, no XDG directory of the caller's, a TMPDIR of its
# own short enough for a socket's path, and no PLACKET_JEV_KEY, so no example reaches a judge's host:
# it unpacks the archive and puts its placket first on PATH; runs placket init; starts each window
# with the line init printed; gives through placket mcp, served as the claude mcp add line says, by
# the scripted client tests/firsthour/ in place of Claude Code; answers with the line placket watch
# prints; runs the applier from the tree's apply.sh, where the fetch line would put it, as the run
# line says; stops each window and runs placket verify on each journal with every member's opening.
# An entry passes when every step does, and verify prints its valid and conforms lines for each
# journal, within LIMIT seconds. GOCACHE and GOMODCACHE stay the caller's, outside the clean HOME.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
LIMIT=60

# A run of one entry, started by the test below in its own process group: --entry NAME TMP.
if [ "${1:-}" = --entry ]; then
	entry=$2 tmp=$3
	work=$tmp/$entry
	export HOME="$work/home"
	unset XDG_CONFIG_HOME XDG_STATE_HOME XDG_RUNTIME_DIR XDG_DATA_HOME
	mkdir -p "$HOME" "$work/install" "$work/run"
	cd "$work/run" || exit 1
	windows=
	cleanup() {
		for pid in $windows; do
			kill "$pid" 2>/dev/null || :
			wait "$pid" 2>/dev/null || :
		done
	}
	trap cleanup EXIT
	trap 'exit 1' INT TERM
	pause() { sleep 0.1 2>/dev/null || sleep 1; }
	die() {
		echo "FAIL $entry: $*"
		exit 1
	}
	say() { echo "$entry: $*"; }

	# Install: unpack the archive and put its placket first on PATH.
	tar -xzf "$tmp/archive.tar.gz" -C "$work/install" || die "the archive does not unpack"
	PATH=$work/install:$PATH
	export PATH
	[ "$(command -v placket)" = "$work/install/placket" ] || die "placket on PATH is not the archive's"
	say "installed placket from the archive"

	# printed PREFIX: the command init printed that starts with PREFIX, without its indent.
	printed() { sed -n "s/^  \($1.*\)$/\1/p" init.out; }
	# ref LABEL: the key reference init printed for the party LABEL.
	ref() { sed -n "s/^party $1 \([^ ]*\) .*$/\1/p" init.out; }
	# start: starts each window init printed a line for, and waits until each is founded.
	start() {
		i=0
		printed "placket window " >windows.txt
		while IFS= read -r line; do
			i=$((i + 1))
			eval "$line 2>window$i.log &"
			windows="$windows $!"
			n=0
			until grep -q '^operator ' "window$i.log" 2>/dev/null; do
				n=$((n + 1))
				[ "$n" -lt 300 ] || die "window $i did not start: $(cat "window$i.log")"
				pause
			done
			grep -q '^accord sha256:[0-9a-f]* founded$' "window$i.log" || die "window $i did not start founded: $(cat "window$i.log")"
		done <windows.txt
		say "started $i windows, each founded"
	}
	# mcp: runs the scripted client on standard input against the server the claude mcp add line
	# names, printing each tool's result.
	mcp() {
		server=$(printed "claude mcp add placket -- ")
		server=${server#claude mcp add placket -- }
		eval "\"$tmp/firsthour\" $server"
	}
	# answer: waits for the person's question with the watch line init printed, and runs the answer
	# line watch printed for it.
	answer() {
		eval "$(printed 'placket watch ') --count 1" >watch.out 2>watch.err || die "watch: $(cat watch.err)"
		line=$(grep '^placket answer ' watch.out) || die "watch printed no answer line"
		eval "$line" >answer.out 2>answer.err || die "answer: $(cat answer.err)"
		grep -q '^released ok sha256:[0-9a-f]*$' answer.err || die "the answer released nothing: $(cat answer.err)"
		say "the person accepts, with the line placket watch printed, and the part is released"
	}
	# applier PATTERN TARGET: puts the tree's apply.sh of PATTERN where the fetch line would, and
	# runs it as the run line says, TARGET in place of the line's last word.
	applier() {
		script=$(printed "curl " | sed -n 's/^curl .* -o \([^ ]*\) .*$/\1/p')
		[ -n "$script" ] || die "init printed no fetch line"
		mkdir -p "$(dirname "$script")"
		cp "$root/patterns/$1/apply.sh" "$script"
		line=$(printed "sh ")
		eval "${line% *} \"\$2\"" >apply.out 2>apply.err || die "the applier: $(cat apply.out apply.err)"
	}
	# verify: stops the windows, and verifies each Accord's journal with every member's opening.
	verify() {
		cleanup
		windows=
		n=$(jq '.accords | length' init.json)
		i=0
		while [ "$i" -lt "$n" ]; do
			a=$(jq -c ".accords[$i]" init.json)
			label=$(echo "$a" | jq -r .label)
			d=$(jq -r .dir init.json)
			opens=$(echo "$a" | jq -r '.parties[].actor | sub("^ed25519:"; "")' | sed "s|.*|--open $d/&.opening.json|")
			# shellcheck disable=SC2086 # the --open flags, split on purpose
			placket verify --accord "$(echo "$a" | jq -r .accord)" --founding "$d/$label/founding.json" \
				--acceptances "$d/$label/acceptances.json" --receipts "$d/$label/journal" $opens >verify.out 2>verify.err ||
				die "verify $label: $(cat verify.err)"
			grep -q '^valid: [0-9]* receipts signed by mediator ed25519:[0-9a-f]*$' verify.err || die "verify $label printed no valid line: $(cat verify.err)"
			grep -qx 'conforms: every decision is one this Accord let its decider make' verify.err ||
				die "verify $label printed no conforms line: $(cat verify.err)"
			say "verify $label: $(sed -n 's/^\(valid: [0-9]* receipts\) .*/\1/p' verify.err), conforms"
			sed -n 's/^valid: [0-9]* receipts signed by mediator //p' verify.err >>mediators
			i=$((i + 1))
		done
	}

	case $entry in
	a)
		placket init approve --json >init.json 2>init.out || die "init: $(cat init.out)"
		start
		text="Hello from the first hour."
		echo "{\"name\":\"give\",\"arguments\":{\"mediator\":\"approve-agent\",\"to\":\"$(ref approve-applier)\",\"kind\":\"proposal\",\"text\":\"$text\"}}" |
			mcp >give.out || die "the agent's give"
		q=$(jq -r '.question // empty' give.out)
		[ -n "$q" ] || die "the agent's give asked nothing: $(cat give.out)"
		say "the agent proposes, through placket mcp, and the terms ask"
		answer
		echo "{\"name\":\"result\",\"arguments\":{\"mediator\":\"approve-agent\",\"question\":\"$q\"}}" | mcp >result.out || die "the agent's result"
		[ "$(jq -r '.closing.word' result.out)" = accepted ] || die "the agent's result: $(cat result.out)"
		say "the agent's result, through placket mcp: $(jq -r '.closing.word' result.out)"
		applier local-apply "$work/run/notes.txt"
		[ "$(cat "$work/run/notes.txt" 2>/dev/null)" = "$text" ] || die "the file holds: $(cat notes.txt 2>&1)"
		say "the applier writes the accepted text: $(cat apply.out | sed "s|$work/run/||")"
		verify
		;;
	b)
		placket init merge --json >init.json 2>init.out || die "init: $(cat init.out)"
		start
		# The stand-in gh answers from its fixtures in place of GitHub, and keeps its state beside it.
		cp -R "$root/patterns/github-merge/stand-in" "$work/stand-in"
		PATH=$work/stand-in:$PATH
		body=$(jq -c '{repo: .base.repo.full_name, pr: .number, base: .base.ref, head: .head.sha}' "$work/stand-in/fixtures/repos/example/widgets/pulls/7.json")
		echo "{\"name\":\"give\",\"arguments\":{\"mediator\":\"merge-agent\",\"to\":\"$(ref merge-applier)\",\"kind\":\"merge\",\"data\":$body}}" |
			mcp >give.out || die "the agent's give"
		[ -n "$(jq -r '.question // empty' give.out)" ] || die "the agent's give asked nothing: $(cat give.out)"
		say "the agent proposes a merge, through placket mcp, and the terms ask"
		answer
		applier github-merge example/widgets
		[ "$(cat apply.out)" = "applied pr 7" ] || die "the applier: $(cat apply.out apply.err)"
		[ "$(jq -r .merged "$work/stand-in/state/pulls/example/widgets/7.json")" = true ] || die "pull request 7 is not merged"
		say "the applier merges: $(cat apply.out)"
		verify
		;;
	c)
		placket init handoff --json >init.json 2>init.out || die "init: $(cat init.out)"
		start
		{
			echo "{\"name\":\"give\",\"arguments\":{\"mediator\":\"handoff-plan-planner\",\"to\":\"$(ref handoff-plan-coder)\",\"kind\":\"plan\",\"text\":\"Add a test, then the code.\"}}"
			echo '{"name":"get","arguments":{"mediator":"handoff-plan-coder","wait":10}}'
			echo "{\"name\":\"give\",\"arguments\":{\"mediator\":\"handoff-review-coder\",\"to\":\"$(ref handoff-review-reviewer)\",\"kind\":\"work\",\"text\":\"The test, and the code.\"}}"
			echo '{"name":"get","arguments":{"mediator":"handoff-review-reviewer","wait":10}}'
		} | mcp >handoffs.out || die "the handoffs: $(cat handoffs.out)"
		[ "$(sed -n 2p handoffs.out | jq -r '.delivery.text')" = "Add a test, then the code." ] || die "the coder fetched: $(sed -n 2p handoffs.out)"
		[ "$(sed -n 4p handoffs.out | jq -r '.delivery.text')" = "The test, and the code." ] || die "the reviewer fetched: $(sed -n 4p handoffs.out)"
		for i in 2 4; do
			sed -n "${i}p" handoffs.out | jq -e '.delivery.receiptId | test("^sha256:[0-9a-f]{64}$")' >/dev/null || die "a fetch without its receipt"
		done
		say "the planner hands its plan to the coder, and the coder its work to the reviewer, each fetched with its receipt"
		verify
		# init handoff writes one mediator key for both Accords: both valid lines name it.
		[ "$(wc -l <mediators | tr -d ' ')" = 2 ] && [ "$(sort -u mediators | wc -l | tr -d ' ')" = 1 ] ||
			die "the two journals are not signed by one mediator key: $(cat mediators)"
		say "both journals are signed by one mediator key"
		;;
	esac
	exit 0
fi

case ${1:-} in
'') entries="a b c" ;;
a | b | c) entries=$1 ;;
*)
	echo "usage: sh tests/test_first_hour.sh [a|b|c]"
	exit 2
	;;
esac

command -v jq >/dev/null 2>&1 || { echo "FAIL jq is not on PATH; the first hour needs it"; exit 1; }
tmp=$(mktemp -d)
# A unix socket's path is short on some systems: each entry's TMPDIR lives under a short directory.
short=$(mktemp -d /tmp/plkh.XXXXXX)
groups=
cleanup() {
	for g in $groups; do kill -KILL -"$g" 2>/dev/null; done
	rm -rf "$tmp" "$short"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

# LEADER is a perl program that makes itself the leader of a new process group and then becomes the
# command its arguments name, so the command's process id, $!, is also its group's id.
LEADER='setpgrp(0, 0) or die "setpgrp: $!\n"; exec @ARGV or die "exec $ARGV[0]: $!\n"'

# The install's archive: placket, built as .goreleaser.yaml builds it, and the files of its archive.
files=$(awk '/^[a-z]/ { in_archives = ($0 == "archives:") } in_archives && $1 == "-" && $2 == "src:" { print $3 }' "$root/.goreleaser.yaml")
[ -n "$files" ] || { echo "FAIL .goreleaser.yaml's archive names no files"; exit 1; }
mkdir "$tmp/archive"
# The builds run at the lowest priority: make ci runs the shell tests side by side, and the
# patterns' runs, each within its own time limit, must not wait on a compile.
(cd "$root" && CGO_ENABLED=0 nice -n 19 go build -trimpath -ldflags "-s -w" -o "$tmp/archive/placket" ./cmd/placket) ||
	{ echo "FAIL this tree does not build"; exit 1; }
for f in $files; do
	cp "$root/$f" "$tmp/archive/" || { echo "FAIL $f, which the archive holds, is missing"; exit 1; }
done
# shellcheck disable=SC2086 # the archive's files, split on purpose
(cd "$tmp/archive" && tar -czf "$tmp/archive.tar.gz" placket $files) || { echo "FAIL the archive"; exit 1; }
(cd "$root" && nice -n 19 go build -o "$tmp/firsthour" ./tests/firsthour) || { echo "FAIL tests/firsthour does not build"; exit 1; }
GOCACHE=$(go env GOCACHE) GOMODCACHE=$(go env GOMODCACHE)
export GOCACHE GOMODCACHE

for e in $entries; do
	mkdir -p "$tmp/$e" "$short/$e"
	(
		# No example needs a judge's key, even where the caller's shell holds one.
		TMPDIR=$short/$e exec env -u PLACKET_JEV_KEY perl -e "$LEADER" sh "$0" --entry "$e" "$tmp"
	) >"$tmp/$e.out" 2>&1 </dev/null &
	g=$!
	groups="$groups $g"
	echo "$g" >"$tmp/$e.group"
	(exec perl -e "$LEADER" sh -c "sleep $LIMIT; kill -KILL -$g") >/dev/null 2>&1 &
	echo "$!" >"$tmp/$e.watchdog"
done
failed=0
for e in $entries; do
	g=$(cat "$tmp/$e.group")
	wait "$g"
	rc=$?
	w=$(cat "$tmp/$e.watchdog")
	kill -KILL -"$w" 2>/dev/null
	wait "$w" 2>/dev/null
	kill -KILL -"$g" 2>/dev/null
	if [ "$rc" -eq 0 ]; then
		sed 's/^/    /' "$tmp/$e.out"
		echo "ok   $e"
	else
		failed=1
		if [ "$rc" -eq 137 ]; then echo "FAIL $e: killed, or did not end within ${LIMIT}s"; else echo "FAIL $e: exit $rc"; fi
		sed 's/^/    | /' "$tmp/$e.out"
	fi
done
[ "$failed" -eq 0 ]
