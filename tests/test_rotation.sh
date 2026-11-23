#!/bin/sh
# The rotation guide's test: it founds a fresh Accord as the worked example does, runs its window
# for one give and stops it, and then runs every sh block of docs/guides/rotation.md, in order, in
# one shell, with the binary this tree builds as placket: the close, the successor's founding set,
# the successor's window until every member has accepted, the check of the old journal against the
# accepted successor, and the old key's removal. It checks that the guide holds no unlabelled fence
# and no inline command it does not run, that the old journal ends in its close, that the
# successor's acceptances are written in its own directory and the old Accord's are untouched, and
# that verify --superseded-by prints the pinned complete line. Then it checks that verify refuses
# a successor under another id, the successor flags given apart, and the old Accord's acceptances
# for the successor.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
sock_dir=$(mktemp -d /tmp/plk.XXXXXX)
window_pid=
cleanup() {
	[ -n "$window_pid" ] && kill "$window_pid" 2>/dev/null
	rm -rf "$tmp" "$sock_dir"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
guide=$root/docs/guides/rotation.md

fail() {
	echo "FAIL $1"
	[ -f "$tmp/out" ] && sed 's/^/    | /' "$tmp/out"
	exit 1
}

mkdir "$tmp/bin" "$tmp/w"
(cd "$root" && go build -o "$tmp/bin/placket" ./cmd/placket) || fail "this tree does not build"
PATH=$tmp/bin:$PATH
export PATH
cd "$tmp/w" || exit 1

# A fresh Accord, as README.md founds the worked example's.
for k in mediator operator principal proposer applier decider; do
	placket key new --out "$k.key" >"$k.ref" || fail "key new $k"
done
cat >proposer.terms <<T
{"terms":[
 {"stance":"maybe","dir":"give","with":["role:applier"],"select":{"kind":["proposal"]},"resolve":{"to":["role:decider"],"timeout":3600}},
 {"stance":"want","dir":"get","with":["role:decider"],"select":{"kind":["answer","changed-version","comment"]}}]}
T
cat >applier.terms <<T
{"terms":[{"stance":"want","dir":"get","with":["role:proposer"],"select":{"kind":["proposal"]}}]}
T
cat >decider.terms <<T
{"terms":[{"stance":"want","dir":"give","with":["role:proposer"],"select":{"kind":["answer","changed-version","comment"]}}]}
T
echo "{\"roles\":[\"proposer\",\"applier\",\"decider\"],\"principals\":[{\"principal\":\"$(cat principal.ref)\",\"roles\":[\"proposer\",\"applier\",\"decider\"]}]}" >policy.json
mkdir openings
for a in proposer applier decider; do
	placket declare --key "$a.key" --terms "$a.terms" >"$a.declaration" || fail "declare $a"
	placket seal --key "$a.key" --declaration "$a.declaration" --out openings >"$a.seal" 2>/dev/null || fail "seal $a"
	placket bind --key principal.key --actor "$(cat "$a.ref")" --role "$a" --mediator "$(cat mediator.ref)" --until 2099-01-01T00:00:00Z \
		--policy policy.json >"$a.binding" || fail "bind $a"
done
echo "{\"actor\":\"$(cat mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator.json
placket card --default >card.json 2>/dev/null
placket founding --policy policy.json --mediator mediator.json --card card.json \
	--seal proposer.seal --seal applier.seal --seal decider.seal \
	--binding proposer.binding --binding applier.binding --binding decider.binding >founding.json 2>founding.out || fail "founding"

# The window, one ask left open, and a stop.
sock=$sock_dir/s
placket window --key mediator.key --founding founding.json --openings openings --socket "$sock" --journal journal 2>window.log &
window_pid=$!
i=0
until [ -S "$sock" ]; do
	i=$((i + 1))
	[ "$i" -lt 200 ] || fail "the window did not start"
	sleep 0.05 2>/dev/null || sleep 1
done
# The Accord is proposed until every party accepts its view.
proposed=$(sed -n 's/^accord //p' founding.out)
for a in proposer applier decider; do
	placket view --key "$a.key" --socket "$sock" --accord "$proposed" >/dev/null 2>&1 || fail "view $a"
	placket accept --key "$a.key" --socket "$sock" --accord "$proposed" >/dev/null 2>&1 || fail "accept $a"
done
id=$(sed -n 's/^accord \(sha256:[0-9a-f]*\) founded$/\1/p' window.log)
[ "$id" = "$proposed" ] || fail "the window did not found the Accord"
placket give --key proposer.key --socket "$sock" --accord "$id" --to "$(cat applier.ref)" --kind proposal --text "Rename total." >/dev/null 2>&1
[ $? -eq 4 ] || fail "the ask did not stay open"
kill "$window_pid"
wait "$window_pid" 2>/dev/null
window_pid=

# No fence of the guide is unlabelled, so no command of it goes unrun.
awk '/^```/ { if (!on && $0 == "```") { print FILENAME ":" FNR; bad = 1 } on = !on } END { exit bad }' "$guide" >"$tmp/out" ||
	fail "the guide has an unlabelled fence"
# Every sh block of the guide, in order, in one shell.
awk '/^```sh$/ { on = 1; next } /^```$/ { on = 0; next } on' "$guide" >guide.sh
[ -s guide.sh ] || fail "the guide has no sh block"
# A command the prose names with its flags is one the guide runs, so none is one placket refuses.
awk '/^```/ { on = !on; next } !on' "$guide" | grep -o '`placket [^`]*--[^`]*`' | tr -d '`' >inline.txt
while IFS= read -r c; do
	grep -qF -- "$c" guide.sh || { echo "$c" >"$tmp/out"; fail "the guide's prose names a command it does not run"; }
done <inline.txt
# The close's guard is the operator's own record of the founding, never the journal it closes.
grep -q 'journal/accord' guide.sh && fail "the guide reads the close's --accord from the journal it closes"
old=$(cksum <acceptances.json)
# A window the guide leaves running, on a failure, is stopped with the guide's shell.
{ echo 'trap '"'"'kill $(jobs -p) 2>/dev/null || :'"'"' EXIT'; cat guide.sh; } >run.sh
if ! sh -eu run.sh >"$tmp/out" 2>&1; then
	fail "the guide's commands did not run to their end"
fi

accord=$id
close=$(sed -n 's/^close //p' close.out)
last=$(tail -n 1 journal/receipts.jsonl | jq -r .payload | base64 -d | jq -r .predicateType)
[ "$last" = "https://placket.tech/close/0.1" ] || fail "the old journal does not end in its close"
grep -q "^closed block closed sha256:[0-9a-f]*$" close.out || fail "the open question was not closed as closed"
[ -f successor/acceptances.json ] || fail "the successor's window wrote no acceptances.json in the successor's directory"
[ "$(cksum <acceptances.json)" = "$old" ] || fail "the old Accord's acceptances.json changed"
[ ! -e mediator.key ] || fail "the old key was not removed"
new=sha256:$(tr -d '\n' <successor/founding.json | openssl dgst -sha256 -binary | od -An -v -tx1 | tr -d ' \n')
[ "$new" = "$(sed -n 's/^accord //p' successor/founding.out)" ] || fail "the successor's id is not its set's typed hash"
want="complete: against close receipt $close, which successor accord $new pins; not claimed: that the mediator signed no other log for this Accord"
grep -qxF "$want" "$tmp/out" || fail "verify --superseded-by printed no pinned complete line: $want"
echo "ok   the rotation guide closes, founds a successor, runs its window until every member accepts, and verify --superseded-by prints the pinned complete line"

# refused CODE WANT ARGS...: verify with ARGS exits CODE and prints exactly WANT on standard error,
# and nothing on standard output.
refused() {
	code=$1 want=$2
	shift 2
	placket verify --receipts journal --accord "$accord" --founding founding.json "$@" >"$tmp/stdout" 2>"$tmp/out"
	got=$?
	[ "$got" -eq "$code" ] || fail "verify $* exited $got, not $code"
	[ ! -s "$tmp/stdout" ] || fail "verify $* printed on standard output"
	[ "$(cat "$tmp/out")" = "$want" ] || fail "verify $* did not print: $want"
}
refused 1 "error verify superseded" --superseded-by successor/founding.json --successor-accord "$accord" \
	--successor-acceptances successor/acceptances.json
echo "ok   verify refuses a successor under an id that is not its set's, exit 1"
usage="usage: placket verify: --superseded-by, --successor-accord and --successor-acceptances are given together
error usage"
refused 3 "$usage" --superseded-by successor/founding.json --successor-accord "$new"
refused 3 "$usage" --superseded-by successor/founding.json --successor-acceptances successor/acceptances.json
refused 3 "$usage" --successor-accord "$new" --successor-acceptances successor/acceptances.json
echo "ok   verify takes --superseded-by, --successor-accord and --successor-acceptances only together"
refused 3 "error founding acceptance-accord" --superseded-by successor/founding.json --successor-accord "$new" \
	--successor-acceptances acceptances.json
echo "ok   verify refuses the old Accord's acceptances for the successor"
