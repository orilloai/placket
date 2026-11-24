#!/bin/sh
# The shadow judge: a judge reads every part the terms ask a person about, and decides nothing, so a
# team can compare what it would have said with what the person answered before it trusts it.
#
# Three invented parties share one Accord: a proposer, who proposes changes; a receiver, who applies
# them; and an approver, who answers each one. The card lists one judge, review, whose config is this
# pattern's judge.json, and both sides' terms list it to advise. The window runs it on each part it
# asks about, after the receipt is written; the part still waits for the approver. As it is, the
# pattern clears PLACKET_JEV_KEY before it starts the window, so the judge sends nothing and says
# none, even when your shell holds a key: these invented parts never reach the judge's host.
#
# Run it as it is: `sh pattern.sh`. It uses the placket binary and jq on PATH, reads judge.json from
# its own directory, makes everything else it needs in the current directory, starts its own window
# on a socket under $TMPDIR, and stops it before it exits. See README.md.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
judge=$here/judge.json # CHANGE: your judge's config; its hash goes into the card

# A unix socket's path is short on some systems: the socket lives in a directory of its own.
sock_dir=$(mktemp -d "${TMPDIR:-/tmp}/plk.XXXXXX")
sock=$sock_dir/s
window_pid=
cleanup() {
	if [ -n "$window_pid" ]; then
		kill "$window_pid" 2>/dev/null || :
		wait "$window_pid" 2>/dev/null || :
	fi
	rm -rf "$sock_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

step() { echo; echo "== $*"; }
# pause: a tenth of a second where sleep takes a fraction, which POSIX does not promise, and a
# second where it does not.
pause() { sleep 0.1 2>/dev/null || sleep 1; }
say() {
	who=$1
	shift
	echo "$who: $*"
}
# wait_for PATTERN: wait until a line of window.log matches PATTERN.
wait_for() {
	n=0
	until grep -q "$1" window.log 2>/dev/null; do
		n=$((n + 1))
		[ "$n" -lt 300 ] || { echo "the window printed no line matching $1"; cat window.log; exit 1; }
		pause
	done
}

step "1. keys, terms, the judge and the founding set"
for k in mediator operator principal proposer approver receiver; do
	placket key new --out "$k.key" >"$k.ref"
done
ref() { cat "$1.ref"; }
name=$(jq -r '.name' "$judge")
# Both sides ask the approver about every proposal, and both list the judge to advise: it reads the
# part, and decides nothing. A judge listed to decide would pass or block the part where both sides
# list it so; a team moves it there only once its advice has matched the approver's answers.
use=advise # CHANGE: advise while you compare; decide once you trust it
cat >proposer.terms <<T
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:receiver"],"select":{"kind":["proposal"]},"resolve":{"to":["role:approver"],"timeout":3600,"judges":[{"name":"$name","use":"$use"}]}},
 {"stance":"want","dir":"get","with":["role:approver"],"select":{"kind":["answer","changed-version","comment"]}}]}
T
cat >receiver.terms <<T
{"terms":[
 {"stance":"maybe","dir":"get","with":["role:proposer"],"select":{"kind":["proposal"]},"resolve":{"to":["role:approver"],"timeout":3600,"judges":[{"name":"$name","use":"$use"}]}},
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
# Each party seals its own declaration; its opening goes to whoever runs the window, and to no other
# party.
mkdir -p openings
for a in proposer approver receiver; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket seal --key "$a.key" --declaration "$a.declaration" --out openings >"$a.seal" 2>/dev/null
	placket bind --key principal.key --actor "$(ref "$a")" --role "$a" --mediator "$(ref mediator)" \
		--until 2099-01-01T00:00:00Z --policy policy.json >"$a.binding"
done
echo "{\"actor\":\"$(ref mediator)\",\"operator\":\"$(ref operator)\",\"runs\":\"process\"}" >mediator.json
# The card lists the judge by its name, its adapter's version and the hash of its config.
placket card --default --judge "$judge" >card.json 2>/dev/null
say card "lists judge $(jq -r '.judges[0].name' card.json), version $(jq -r '.judges[0].version' card.json)"
placket founding --json --policy policy.json --mediator mediator.json --card card.json \
	--seal proposer.seal --seal approver.seal --seal receiver.seal \
	--binding proposer.binding --binding approver.binding --binding receiver.binding >founding.out 2>/dev/null
jq '.founding' founding.out >founding.json
accord=$(jq -r '.accord' founding.out)

step "2. the window, and where the judge's requests may go"
# The window runs only the judge configs its card lists, and reads the key from PLACKET_JEV_KEY.
unset PLACKET_JEV_KEY # CHANGE: delete this line to send your own parts to the judge with your key
placket window --key mediator.key --founding founding.json --openings openings --socket "$sock" --journal journal \
	--judge "$judge" 2>window.log &
window_pid=$!
wait_for '^operator '
# The Accord is proposed: each party views its own view and accepts it, and the last acceptance
# founds it.
for a in proposer approver receiver; do
	placket view --key "$a.key" --socket "$sock" --accord "$accord" >/dev/null 2>&1
	placket accept --key "$a.key" --socket "$sock" --accord "$accord" >/dev/null 2>&1
done
grep '^judge ' window.log | sed 's/^/window: /'

as() { echo "--key $1.key --socket $sock --accord $accord"; }

step "3. a proposal, asked of the approver"
# CHANGE: your own proposal. This one is invented.
rc=0
placket give $(as proposer) --json --to "$(ref receiver)" --kind proposal --text "Rename the column total to amount." >give.out 2>/dev/null || rc=$?
say proposer "gives a proposal: $(jq -r '.outcome.word' give.out), exit $rc"
question=$(jq -r '.outcome.receiptId' give.out)
# The window runs the judge after the receipt is written, and prints what it said, never its output.
wait_for '^advise '
sed -n 's/^\(advise [^ ]* [^ ]*\) .*/window: \1, of the proposal'"'"'s receipt/p' window.log
# What the judge said is kept beside the receipt, in the journal's judged/ directory.
say judged "$(jq -c '{judge, use, outcome, error}' "journal/judged/${question#sha256:}.jsonl")"

step "4. the approver answers, as the judge's advice is compared with it"
placket questions $(as approver) --json >questions.out 2>/dev/null
say approver "reads $(jq -r '.questions[0].message' questions.out)"
rc=0
placket answer $(as approver) --json --question "$question" >answer.out 2>/dev/null || rc=$?
say approver "accepts: answer $(jq -r '.outcome.word' answer.out), release $(jq -r '.release.word // "none"' answer.out), exit $rc"
placket get $(as receiver) --json >get.out 2>/dev/null
say receiver "fetches a $(jq -r '.delivery.kind' get.out)"
