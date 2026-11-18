#!/bin/sh
# The notifier: a person who answers questions hears of each one when it opens, not by asking again.
# placket watch waits on the questions of the labels it is given, and prints each new question once,
# as one JSON object with --json; this pattern runs one command for each, with its label and its id.
# Placket runs no command itself: the command is the pattern's, on the edge.
#
# Three invented parties share one Accord: a proposer, who proposes changes; a receiver, who applies
# them; and an approver, who answers each one. The labels config names the approver's mediator
# approve and the proposer's propose, so no command here needs a socket, a key or an Accord id.
#
# Run it as it is: `sh pattern.sh`. It uses the placket binary and jq on PATH, makes everything it
# needs in the current directory, keeps its labels config and its state there too (XDG_CONFIG_HOME and
# XDG_STATE_HOME), starts its own window on a socket under $TMPDIR, and stops it before it exits.
# See README.md.
set -eu

# Each new question runs this, with the question's label and id. CHANGE: your own command, such as
# osascript -e "display notification \"$2\" with title \"placket: $1\"" on macOS, or
# notify-send "placket: $1" "$2" on Linux.
notify() { echo "notify: a question on $1"; }

here=$(pwd)
export XDG_CONFIG_HOME="$here/config" XDG_STATE_HOME="$here/state"
# A unix socket's path is short on some systems: the socket lives in a directory of its own.
sock_dir=$(mktemp -d "${TMPDIR:-/tmp}/plk.XXXXXX")
sock=$sock_dir/s
window_pid=
watch_pid=
cleanup() {
	for pid in $watch_pid $window_pid; do
		kill "$pid" 2>/dev/null || :
		wait "$pid" 2>/dev/null || :
	done
	rm -rf "$sock_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

step() { echo; echo "== $*"; }
pause() { sleep 0.1 2>/dev/null || sleep 1; }
say() {
	who=$1
	shift
	echo "$who: $*"
}

step "1. keys, terms and the founding set"
for k in mediator operator principal proposer approver receiver; do
	placket key new --out "$k.key" >"$k.ref"
done
ref() { cat "$1.ref"; }
cat >proposer.terms <<'T'
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:receiver"],"select":{"kind":["proposal"]},"resolve":{"to":["role:approver"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:approver"],"select":{"kind":["answer","changed-version","comment"]}}]}
T
cat >receiver.terms <<'T'
{"terms":[
 {"stance":"want","dir":"get","with":["role:proposer"],"select":{"kind":["proposal"]}},
 {"stance":"want","dir":"get","with":["role:approver"],"select":{"kind":["changed-version"]}}]}
T
cat >approver.terms <<'T'
{"terms":[
 {"stance":"want","dir":"give","with":["role:proposer"],"select":{"kind":["answer","changed-version","comment"]}},
 {"stance":"want","dir":"give","with":["role:receiver"],"select":{"kind":["changed-version"]}}]}
T
cat >policy.json <<P
{"roles":["proposer","approver","receiver"],"principals":[
 {"principal":"$(ref principal)","roles":["proposer","approver","receiver"]}]}
P
for a in proposer approver receiver; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket bind --key principal.key --actor "$(ref "$a")" --role "$a" --mediator "$(ref mediator)" \
		--until 2099-01-01T00:00:00Z --policy policy.json >"$a.binding"
done
echo "{\"actor\":\"$(ref mediator)\",\"operator\":\"$(ref operator)\",\"runs\":\"process\"}" >mediator.json
placket card --default >card.json 2>/dev/null
placket founding --json --policy policy.json --mediator mediator.json --card card.json \
	--declaration proposer.declaration --declaration approver.declaration --declaration receiver.declaration \
	--binding proposer.binding --binding approver.binding --binding receiver.binding >founding.out 2>/dev/null
jq '.founding' founding.out >founding.json
accord=$(jq -r '.accord' founding.out)

step "2. the window, and the labels config"
placket window --key mediator.key --founding founding.json --socket "$sock" --journal journal 2>window.log &
window_pid=$!
n=0
until grep -q '^operator ' window.log 2>/dev/null; do
	n=$((n + 1))
	[ "$n" -lt 300 ] || { echo "the window did not start"; cat window.log; exit 1; }
	pause
done
mkdir -p config/placket
# CHANGE: your own mediators, each an Accord id, a key file, a founding set and a socket, labelled.
cat >config/placket/mediators.json <<C
{"mediators":[
 {"label":"approve","accord":"$accord","key":"$here/approver.key","founding":"$here/founding.json","socket":"$sock"},
 {"label":"propose","accord":"$accord","key":"$here/proposer.key","founding":"$here/founding.json","socket":"$sock"}]}
C
placket mediators --json | jq -r '.mediators[] | "mediators: \(.label)"'

step "3. the approver's watch, and two proposals"
# CHANGE: the labels to watch, and how many questions before it stops; with no --count it runs
# until it is stopped.
placket watch --label approve --count 2 --json 2>watch.err | while read -r line; do
	echo "$line" | jq -e 'has("id")' >/dev/null || continue
	echo "watch: a $(echo "$line" | jq -r '.kind') on $(echo "$line" | jq -r '.label')"
	notify "$(echo "$line" | jq -r '.label')" "$(echo "$line" | jq -r '.id')"
done >notified.out &
watch_pid=$!
for text in "Rename the column total to amount." "Drop the column notes."; do # CHANGE: your own proposals
	rc=0
	placket give --label propose --json --to "$(ref receiver)" --kind proposal --text "$text" >give.out 2>/dev/null || rc=$?
	say proposer "gives a proposal: $(jq -r '.outcome.word' give.out), exit $rc"
done
wait "$watch_pid"
watch_pid=
cat notified.out

step "4. the approver answers the first, from the line watch printed"
# watch prints, for each question, the placket answer, change and decline lines that answer it.
placket watch --label approve --count 1 --json >first.out 2>/dev/null
first=$(jq -r 'select(.id) | .commands.answer' first.out)
rc=0
$first --json >answer.out 2>/dev/null || rc=$?
say approver "accepts: answer $(jq -r '.outcome.word' answer.out), release $(jq -r '.release.word // "none"' answer.out), exit $rc"
