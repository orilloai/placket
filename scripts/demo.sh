#!/bin/sh
# The worked example, ask and log: a proposer asks, a decider answers from their own terminal, an
# applier fetches what was released, the window is killed and carries on, and a third party
# verifies the journal and exports the log. `make demo` runs it; tests/test_demo.sh runs it in
# make ci and checks what it prints.
#
# Usage: sh scripts/demo.sh [DIR]. Everything goes in DIR, a new directory by default: the keys,
# the terms and the founding set, the journal, the three state directories and the window's log.
# PLACKET names a built binary to use instead of building one.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-$(mktemp -d "${TMPDIR:-/tmp}/placket-demo.XXXXXX")}
mkdir -p "$out"
out=$(cd "$out" && pwd)
# A unix socket's path is short on some systems: the socket lives in a directory of its own.
sock_dir=$(mktemp -d /tmp/plk.XXXXXX)
sock=$sock_dir/s
window_pid=
cleanup() {
	[ -n "$window_pid" ] && kill "$window_pid" 2>/dev/null || :
	rm -rf "$sock_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

if [ -n "${PLACKET:-}" ]; then
	p=$PLACKET
else
	p=$out/placket
	(cd "$root" && go build -o "$p" ./cmd/placket)
fi
cd "$out"

step() { echo; echo "== $*"; }
# run NAME ARGS...: run the binary as one party, printing what it prints, and its exit code.
run() {
	who=$1
	shift
	set +e
	"$p" "$@" >"$out/stdout" 2>"$out/stderr"
	rc=$?
	set -e
	sed "s/^/$who: /" "$out/stderr"
	echo "$who: exit $rc"
	return 0
}
# socket NAME: the flags of a socket command run by NAME.
socket() { echo "--key $out/$1.key --socket $sock --accord $accord"; }

step "1. keys, declarations, bindings, the founding set"
for k in mediator operator principal proposer applier decider; do
	"$p" key new --out "$k.key" >"$k.ref"
done
ref() { cat "$out/$1.ref"; }
cat >proposer.terms <<T
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","changed-version","comment"]}}]}
T
cat >applier.terms <<T
{"terms":[
 {"stance":"want","dir":"get","with":["role:proposer"],"select":{"kind":["proposal"]}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["changed-version"]}}]}
T
cat >decider.terms <<T
{"terms":[
 {"stance":"want","dir":"give","with":["role:proposer"],"select":{"kind":["answer","changed-version","comment"]}},
 {"stance":"want","dir":"give","with":["role:applier"],"select":{"kind":["changed-version"]}}]}
T
for a in proposer applier decider; do
	"$p" declare --key "$a.key" --terms "$a.terms" >"$a.declaration"
	"$p" bind --key principal.key --actor "$(ref "$a")" --role "$a" --mediator "$(ref mediator)" --until 2099-01-01T00:00:00Z >"$a.binding"
done
echo "{\"roles\":[\"proposer\",\"applier\",\"decider\"],\"principals\":[\"$(ref principal)\"]}" >policy.json
echo "{\"actor\":\"$(ref mediator)\",\"operator\":\"$(ref operator)\",\"runs\":\"process\"}" >mediator.json
echo '{"spec":"0.1","kernel":{"version":"0.1.0","hash":"sha256:a8755bef6ff188c2dc6b74377a73c6a49d612fc44915675db9d2795bb5b99475"},"judges":[],"checks":[]}' >card.json
"$p" founding --policy policy.json --mediator mediator.json --card card.json \
	--declaration proposer.declaration --declaration applier.declaration --declaration decider.declaration \
	--binding proposer.binding --binding applier.binding --binding decider.binding >founding.json 2>founding.out
cat founding.out
accord=$(awk '$1 == "accord" { print $2 }' founding.out)

step "2. the window"
start_window() {
	rm -f window.ready
	"$p" window --key mediator.key --founding founding.json --socket "$sock" --journal journal 2>>window.log &
	window_pid=$!
	n=0
	until grep -q '^operator ' window.log 2>/dev/null && [ "$(grep -c '^operator ' window.log)" -ge "$1" ]; do
		n=$((n + 1))
		[ "$n" -lt 100 ] || { echo "the window did not start"; cat window.log; exit 1; }
		sleep 0.1
	done
}
start_window 1
sed 's/^/window: /' window.log

# ask KIND TEXT TOPIC: the proposer gives a proposal and waits in the background; prints nothing.
ask() {
	set +e
	"$p" give $(socket proposer) --to "$(ref applier)" --kind proposal --topic "$2" --text "$1" --keep --wait >"$out/wait.out" 2>"$out/wait.err" &
	wait_pid=$!
	set -e
}
# waited: the proposer's wait ends; prints what it printed and its exit code.
waited() {
	set +e
	wait "$wait_pid"
	rc=$?
	set -e
	sed 's/^/proposer: /' "$out/wait.err"
	sed 's/^/proposer: /' "$out/wait.out" | grep -v '^proposer: {' || :
	echo "proposer: exit $rc"
}
# question: the id of the first question the decider may answer, once there is one.
question() {
	n=0
	while :; do
		"$p" questions $(socket decider) >"$out/questions" 2>/dev/null || :
		q=$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "$out/questions" | head -n 1)
		[ -z "$q" ] || break
		n=$((n + 1))
		[ "$n" -lt 100 ] || { echo "no question came"; exit 1; }
		sleep 0.1
	done
	sed -n 's/.*"message":"\([^"]*\)".*/decider: questions: \1/p' "$out/questions" | head -n 1
}

step "3 and 4. a proposal, asked and accepted; the applier fetches it"
ask "Rename the column total to amount." "billing"
question
run decider answer $(socket decider) --question "$q"
waited
run applier get $(socket applier) --keep
sed 's/^/applier: /' stdout

step "5. a proposal changed, with a comment; its version stays unfetched by the applier until step 9"
ask "Drop the column notes." "billing"
question
run decider change $(socket decider) --question "$q" --text "Keep the column notes, but hide it." --comment "Someone still reads it." --keep
waited

step "6. a proposal declined, with a comment"
ask "Delete the table invoices." "billing"
question
run decider decline $(socket decider) --question "$q" --comment "Invoices are kept for seven years."
waited

step "7. a proposal left open; the proposer cannot answer it"
run proposer give $(socket proposer) --to "$(ref applier)" --kind proposal --topic billing --text "Add an index on customer." --keep
open=$(tail -n 1 stdout)
run proposer answer $(socket proposer) --question "$open"

step "8. a part of a kind no term names"
run proposer give $(socket proposer) --to "$(ref applier)" --kind sketch --text "A sketch."

step "9. the window is killed with a question open and a released version unfetched, and started again"
kill -9 "$window_pid"
wait "$window_pid" 2>/dev/null || :
window_pid=
: >window.log
start_window 1
sed 's/^/window: /' window.log
run proposer result $(socket proposer) --question "$open"
run applier get $(socket applier) --keep
sed 's/^/applier: /' stdout
for a in proposer applier decider; do
	while :; do
		run "$a" get $(socket "$a") --keep
		grep -q '^no delivery$' stderr && break
		sed "s/^/$a: /" stdout
	done
done

step "10. verify, reconcile and the log; then one byte of a receipt changed"
run verifier verify --mediator "$(ref mediator)" --receipts journal --founding founding.json
card=$(awk '$1 == "card" { print $2 }' founding.out)
run verifier reconcile --mediator "$(ref mediator)" --accord "$accord" --card "$card" --receipts journal \
	--kept "proposer.key.state/${accord#sha256:}" --kept "applier.key.state/${accord#sha256:}" --kept "decider.key.state/${accord#sha256:}"
run verifier log --mediator "$(ref mediator)" --receipts journal --format csv \
	--kept "proposer.key.state/${accord#sha256:}" --kept "applier.key.state/${accord#sha256:}" --kept "decider.key.state/${accord#sha256:}"
sed 's/^/verifier: /' stdout
mkdir -p changed
awk 'NR == 1 { i = index($0, "\"sig\":\"") + 7; c = substr($0, i, 1); d = (c == "A") ? "B" : "A"; $0 = substr($0, 1, i - 1) d substr($0, i + 1) } { print }' journal/receipts.jsonl >changed/receipts.jsonl
run verifier verify --mediator "$(ref mediator)" --receipts changed
run verifier log --mediator "$(ref mediator)" --receipts changed --format csv
echo
echo "demo: done; everything is in $out"
