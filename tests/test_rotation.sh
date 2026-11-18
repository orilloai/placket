#!/bin/sh
# The rotation guide's test: it founds a fresh Accord as the worked example does, runs its window
# for one give and stops it, and then runs every sh block of docs/guides/rotation.md, in order, in
# one shell, with the binary this tree builds as placket. It checks that the old journal ends in its
# close, that the successor's founding set supersedes it, and that verify --superseded-by prints the
# pinned complete line.
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

# Every sh block of the guide, in order, in one shell.
awk '/^```sh$/ { on = 1; next } /^```$/ { on = 0; next } on' "$guide" >guide.sh
[ -s guide.sh ] || fail "the guide has no sh block"
# The close's guard is the operator's own record of the founding, never the journal it closes.
grep -q 'journal/accord' guide.sh && fail "the guide reads the close's --accord from the journal it closes"
if ! sh -eu guide.sh >"$tmp/out" 2>&1; then
	fail "the guide's commands did not run to their end"
fi

close=$(sed -n 's/^close //p' close.out)
last=$(tail -n 1 journal/receipts.jsonl | jq -r .payload | base64 -d | jq -r .predicateType)
[ "$last" = "https://placket.tech/close/0.1" ] || fail "the old journal does not end in its close"
grep -q "^closed block closed sha256:[0-9a-f]*$" close.out || fail "the open question was not closed as closed"
new=sha256:$(tr -d '\n' <founding2.json | openssl dgst -sha256 -binary | od -An -v -tx1 | tr -d ' \n')
want="complete: against close receipt $close, which successor accord $new pins; not claimed: that the mediator signed no other log for this Accord"
grep -qxF "$want" "$tmp/out" || fail "verify --superseded-by printed no pinned complete line: $want"
echo "ok   the rotation guide closes, founds a successor, and verify --superseded-by prints the pinned complete line"
