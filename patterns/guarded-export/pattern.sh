#!/bin/sh
# The guarded export: a receiver exports a value only when exactly that value was released to it.
#
# Three invented parties share one Accord: a proposer, who proposes price records; an approver, who
# accepts, changes or declines each one; and a receiver, whose export step publishes prices. The
# proposer normalizes each record before it crosses, the approver answers, the receiver fetches every
# delivery to files, and a guard that holds only the record it is about to export asks placket
# whether exactly that record was released to the receiver. Then the log shows every crossing and
# the journal is verified.
#
# Run it as it is: `sh pattern.sh`. It uses the placket binary and jq on PATH, makes everything it
# needs in the current directory, starts its own window on a socket under $TMPDIR, and stops it
# before it exits. See README.md.
set -eu

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
# say WHO TEXT...: a line of what a party did, as the pattern prints it.
say() {
	who=$1
	shift
	echo "$who: $*"
}

# normalize: the one spelling a record has, on both sides of the crossing. Placket compares bytes and
# normalizes nothing, so whatever rule fixes a value's spelling runs before the part crosses and again
# before the guard checks it. This one sorts the members, drops whitespace, and writes a price as a
# string with two decimals.
normalize() {
	jq -cS '.price |= (tostring | if test("^[0-9]+$") then . + ".00" elif test("^[0-9]+\\.[0-9]$") then . + "0" else . end)' # CHANGE: your own normalization, the same on both sides
}

step "1. keys, terms and the founding set"
for k in mediator operator principal proposer approver receiver; do
	placket key new --out "$k.key" >"$k.ref"
done
ref() { cat "$1.ref"; }
# The proposer's records may cross to the receiver only once the approver answers (a maybe term).
cat >proposer.terms <<'T'
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:receiver"],"select":{"kind":["proposal"]},"resolve":{"to":["role:approver"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:approver"],"select":{"kind":["answer","changed-version","comment"]}}]}
T
cat >approver.terms <<'T'
{"terms":[
 {"stance":"want","dir":"give","with":["role:proposer"],"select":{"kind":["answer","changed-version","comment"]}},
 {"stance":"want","dir":"give","with":["role:receiver"],"select":{"kind":["changed-version"]}}]}
T
cat >receiver.terms <<'T'
{"terms":[
 {"stance":"want","dir":"get","with":["role:proposer"],"select":{"kind":["proposal"]}},
 {"stance":"want","dir":"get","with":["role:approver"],"select":{"kind":["changed-version"]}}]}
T
for a in proposer approver receiver; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	placket bind --key principal.key --actor "$(ref "$a")" --role "$a" --mediator "$(ref mediator)" \
		--until 2099-01-01T00:00:00Z >"$a.binding" # CHANGE: how long each binding holds
done
echo "{\"roles\":[\"proposer\",\"approver\",\"receiver\"],\"principals\":[\"$(ref principal)\"]}" >policy.json
echo "{\"actor\":\"$(ref mediator)\",\"operator\":\"$(ref operator)\",\"runs\":\"process\"}" >mediator.json
placket card --default >card.json 2>/dev/null
placket founding --json --policy policy.json --mediator mediator.json --card card.json \
	--declaration proposer.declaration --declaration approver.declaration --declaration receiver.declaration \
	--binding proposer.binding --binding approver.binding --binding receiver.binding >founding.out 2>/dev/null
jq '.founding' founding.out >founding.json
accord=$(jq -r '.accord' founding.out)
say founding "three parties, roles $(jq -r '.roles | join(", ")' policy.json)"

step "2. the window"
placket window --key mediator.key --founding founding.json --socket "$sock" --journal journal 2>window.log &
window_pid=$!
n=0
until grep -q '^operator ' window.log 2>/dev/null; do
	n=$((n + 1))
	[ "$n" -lt 100 ] || { echo "the window did not start"; cat window.log; exit 1; }
	sleep 0.1
done
say window "started"

# as NAME: the flags of a socket command run by NAME.
as() { echo "--key $1.key --socket $sock --accord $accord"; }

# propose FILE: the proposer normalizes the record in FILE and gives it to the receiver. The terms
# make it a question for the approver, so it waits, unsent, for an answer.
propose() {
	normalize <"$1" >"$1.sent"
	rc=0
	placket give $(as proposer) --json --to "$(ref receiver)" --kind proposal --file "$1.sent" --keep >give.out 2>/dev/null || rc=$?
	say proposer "gives $(cat "$1.sent"): $(jq -r '.outcome.word' give.out), exit $rc"
}
# question: the approver's first open question; it prints the record the approver reads.
question() {
	placket questions $(as approver) --json >questions.out 2>/dev/null
	q=$(jq -r '.questions[0].id' questions.out)
	say approver "reads $(jq -r '.questions[0].message' questions.out)"
}
# answered WORD: what the approver's answer did, from its object.
answered() {
	say approver "$1: answer $(jq -r '.outcome.word' answer.out), release $(jq -r '.release.word // "none"' answer.out), exit $rc"
}

step "3. records proposed, and each one answered"
# CHANGE: your own records. These are invented.
echo '{ "sku": "TB-100", "price": 18.4, "currency": "EUR" }' >tb-100.json
echo '{ "sku": "TB-200", "price": 7.25, "currency": "EUR" }' >tb-200.json
echo '{ "sku": "TB-300", "price": 3, "currency": "EUR" }' >tb-300.json

propose tb-100.json
question
rc=0
placket answer $(as approver) --json --question "$q" >answer.out 2>/dev/null || rc=$?
answered accepts

propose tb-200.json
question
echo '{"sku":"TB-200","price":"7.50","currency":"EUR"}' | normalize >tb-200.version
rc=0
placket change $(as approver) --json --question "$q" --file tb-200.version --keep >answer.out 2>/dev/null || rc=$?
answered "changes it to $(cat tb-200.version)"

propose tb-300.json
question
rc=0
placket decline $(as approver) --json --question "$q" --comment "Not this quarter." >answer.out 2>/dev/null || rc=$?
answered declines

step "4. the receiver fetches every delivery to files, and keeps each one"
mkdir -p inbox
placket get $(as receiver) --all --out inbox --json --keep >get.out 2>/dev/null
jq -r '.deliveries[] | "receiver: fetched a \(.kind) into " + (.path | sub("/[^/]*$"; "/"))' get.out

# The receiver's state directory, where get --keep kept every part it fetched.
receiver_state=receiver.key.state/${accord#sha256:}

# Where the exported value sits inside a part. Here the record is the whole part. A part that wraps the
# value in a body, such as {"summary":...,"payload":...}, names the value's JSON Pointer, such as
# /payload: then released reads the value at that path in every part the receiver kept.
at= # CHANGE: the path of the value inside the part; empty for the whole part

# guard FILE: the export step's guard. It holds only the record it is about to export, in FILE: no
# delivery, no file of inbox, no receipt id. It normalizes the record as the proposer did and asks
# released whether exactly those bytes are a whole part, or the value at $at inside one, of a
# proposal or a changed version that crossed to the receiver and that the receiver kept. Only then
# does the export run.
guard() {
	normalize <"$1" >"$1.check"
	value=$1.check
	if [ -n "$at" ]; then
		# A value inside a body has no newline after it, and released compares its bytes exactly.
		printf %s "$(cat "$value")" >"$value.at"
		set -- --at "$at" --value-file "$value.at"
	else
		set -- --file "$value"
	fi
	rc=0
	placket released --json --state "$receiver_state" --kind proposal --kind changed-version \
		"$@" >released.out 2>/dev/null || rc=$?
	if [ "$rc" -eq 0 ]; then
		cat "$value" >>export.jsonl # CHANGE: your export step, run only on a match
		say guard "exports $(cat "$value"): released as a $(jq -r '.kind' released.out)"
	else
		say guard "refuses $(cat "$value"): released exit $rc"
	fi
}

step "5. the guard, before each export"
: >export.jsonl
# CHANGE: the records your export step is about to publish, one file each.
echo '{"currency":"EUR","price":18.40,"sku":"TB-100"}' >current-1.json
echo '{"sku":"TB-200","price":"7.50","currency":"EUR"}' >current-2.json
echo '{"sku":"TB-200","price":7.25,"currency":"EUR"}' >current-3.json
echo '{"sku":"TB-300","price":3,"currency":"EUR"}' >current-4.json
echo '{"sku":"TB-100","price":"18.41","currency":"EUR"}' >current-5.json
echo '{"sku":"TB-900","price":"1.00","currency":"EUR"}' >current-6.json
say guard "the accepted record, spelled another way"
guard current-1.json
say guard "the approver's changed version"
guard current-2.json
say guard "the record as proposed, before the approver changed it"
guard current-3.json
say guard "the declined record, which was blocked"
guard current-4.json
say guard "the accepted record with one byte changed"
guard current-5.json
say guard "a record never proposed"
guard current-6.json
say export "$(wc -l <export.jsonl | tr -d ' ') records exported"

step "6. a record replaced later still matches its own receipt"
echo '{ "sku": "TB-100", "price": 19.1, "currency": "EUR" }' >tb-100b.json
propose tb-100b.json
question
rc=0
placket answer $(as approver) --json --question "$q" >answer.out 2>/dev/null || rc=$?
answered accepts
placket get $(as receiver) --all --out inbox --json --keep >get.out 2>/dev/null
say receiver "fetched $(jq '.deliveries | length' get.out) more"
say guard "the old record of TB-100, since replaced"
guard current-1.json
# released --every lists every match in fetch order; "only the latest" is a rule the caller adds.
placket released --json --state "$receiver_state" --kind proposal --kind changed-version \
	--at /sku --value '"TB-100"' --every >every.out 2>/dev/null
say guard "released --every for TB-100: $(jq '.matches | length' every.out) matches, in fetch order"
# CHANGE: your own rule for "only the latest". This one: the record the guard holds must be the last
# release of its sku in fetch order.
if [ "$(jq -r '.receiptId' released.out)" = "$(jq -r '.matches[-1].receiptId' every.out)" ]; then
	say guard "the latest release of TB-100"
else
	say guard "not the latest release of TB-100: a caller that needs only the latest refuses it"
fi

step "7. the log of every crossing, and the journal verified"
kept="--kept proposer.key.state/${accord#sha256:} --kept approver.key.state/${accord#sha256:} --kept $receiver_state"
placket log --json --all --mediator "$(ref mediator)" --receipts journal $kept >log.out 2>/dev/null
jq -r '.log.rows[] | "log: \(.kind) \(.word) \(.reason)" + (if .answer != "" then " answer \(.answer)" else "" end) + (if .released_at != "" then ", released" else "" end)' log.out
jq -r '"log: \(.log.claim)", "log: \(.log.notClaimed)"' log.out
rc=0
placket verify --json --mediator "$(ref mediator)" --receipts journal --founding founding.json >verify.out 2>/dev/null || rc=$?
jq -r '"verify: \(.verified) receipts verified, exit \(.exit)", "verify: pass \(.counts.pass), block \(.counts.block), ask \(.counts.ask), releases \(.releases)"' verify.out

kill "$window_pid"
wait "$window_pid" 2>/dev/null || :
window_pid=
say window "stopped"
