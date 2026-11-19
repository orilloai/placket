#!/bin/sh
# The local applier: a file is written only with what a person in the deciding role accepted, or
# changed it to, and never with a proposal they declined.
#
# Three invented parties share one Accord: an agent, which proposes the text of a file; a person in
# the deciding role, who answers each proposal; and the applier, apply.sh, which writes the file. This
# script founds the Accord with `placket init approve`, starts its window, plays the agent and the
# person, runs apply.sh after each answer, and verifies the journal.
#
# Run it as it is: `sh pattern.sh`. It uses the placket binary and jq on PATH. It makes everything
# it needs in the current directory, keeps its labels config and its state beside this file
# (XDG_CONFIG_HOME and XDG_STATE_HOME), starts its window on a socket under $TMPDIR, and stops it
# before it exits. See README.md.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
export XDG_CONFIG_HOME="$here/config" XDG_STATE_HOME="$here/state"
# A unix socket's path is short on some systems: the window's default socket lives under $TMPDIR.
sock_dir=$(mktemp -d "${TMPDIR:-/tmp}/plk.XXXXXX")
export XDG_RUNTIME_DIR="$sock_dir"
window=
cleanup() {
	if [ -n "$window" ]; then
		kill "$window" 2>/dev/null || :
		wait "$window" 2>/dev/null || :
	fi
	rm -rf "$sock_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

step() { echo; echo "== $*"; }
pause() { sleep 0.1 2>/dev/null || sleep 1; }

step "1. placket init founds the Accord, and its window starts"
placket init approve --dir "$PWD/accord" --json >init.json 2>/dev/null # CHANGE: your own label, with --label
jq -r '.accords[0].parties[] | "init: \(.label) holds role \(.role)"' init.json
applier=$(jq -r '.accords[0].parties[] | select(.label == "approve-applier") | .actor' init.json)
eval "$(jq -r '.accords[0].window' init.json) 2>window.log &"
window=$!
i=0
until grep -q '^operator ' window.log 2>/dev/null; do
	i=$((i + 1))
	[ "$i" -lt 300 ] || { echo "the window did not start"; cat window.log; exit 1; }
	pause
done
sed -n 's/^accord sha256:[0-9a-f]* \(founded\)$/window: \1/p' window.log

# ask TEXT: the agent proposes TEXT as the file's text; the person's question is in $q.
ask() {
	placket give --label approve-agent --to "$applier" --kind proposal --text "$1" >/dev/null 2>&1 || :
	i=0
	until q=$(placket questions --label approve-person --json 2>/dev/null | jq -er '.questions[0].id'); do
		i=$((i + 1))
		[ "$i" -lt 300 ] || { echo "no question came"; exit 1; }
		pause
	done
	echo "agent: proposes \"$1\""
}
# show FILE: what FILE holds, or that there is none.
show() { if [ -f "$1" ]; then echo "$1: $(cat "$1")"; else echo "$1: no file"; fi; }

step "2. the person accepts a proposal, and the applier writes it"
ask "Hello, world."
placket answer --label approve-person --question "$q" >/dev/null 2>&1
echo "person: accepts"
sh "$here/apply.sh" approve-applier accepted.txt
show accepted.txt

step "3. the person changes a proposal, and the applier writes the changed text"
ask "Goodbye, world."
placket change --label approve-person --question "$q" --text "Goodbye for now, world." >/dev/null 2>&1 || :
echo "person: changes it to \"Goodbye for now, world.\""
sh "$here/apply.sh" approve-applier changed.txt
show changed.txt

step "4. the person declines a proposal, and nothing is released to the applier"
ask "Delete everything."
placket decline --label approve-person --question "$q" --comment "Not this." >/dev/null 2>&1 || :
echo "person: declines"
sh "$here/apply.sh" approve-applier declined.txt
show declined.txt

step "5. the journal verifies, with every opening"
kill "$window"
wait "$window" 2>/dev/null || :
window=
accord=$(jq -r '.accords[0].accord' init.json)
opens=$(jq -r '.accords[0].parties[] | .actor | sub("^ed25519:"; "")' init.json | sed "s|.*|--open $PWD/accord/&.opening.json|")
# shellcheck disable=SC2086 # the --open flags, split on purpose
placket verify --accord "$accord" --founding accord/approve/founding.json --acceptances accord/approve/acceptances.json \
	--receipts accord/approve/journal $opens 2>&1 >/dev/null | sed -n 's/^\(valid: [0-9]* receipts signed by mediator\) .*/verify: \1 M/p; s/^\(conforms: .*\)/verify: \1/p'
