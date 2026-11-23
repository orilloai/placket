#!/bin/sh
# The default card names the binary's own decision module (docs/adr/0007-the-card-and-the-kernel-hash.md): a
# build whose core/composition/composition.go differs by one comment prints another card, names the
# first build's card as another module's when it founds under it, and refuses to run a window of an
# Accord founded under it.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0

fail() {
	echo "FAIL $1"
	[ -f "$tmp/out" ] && sed 's/^/    | /' "$tmp/out"
	failed=1
}

# The build of this tree, and of a copy whose kernel has one more line.
(cd "$root" && go build -o "$tmp/this" ./cmd/placket) || { echo "FAIL this tree does not build"; exit 1; }
mkdir "$tmp/copy"
cp -R "$root/go.mod" "$root/core" "$root/adapters" "$root/api" "$root/cmd" "$tmp/copy/"
printf '\n// Another kernel.\n' >>"$tmp/copy/core/composition/composition.go"
(cd "$tmp/copy" && go build -o "$tmp/other" ./cmd/placket) || { echo "FAIL the changed copy does not build"; exit 1; }

"$tmp/this" card --default >"$tmp/this.card" 2>"$tmp/this.err" || fail "card --default of this build"
"$tmp/other" card --default >"$tmp/other.card" 2>"$tmp/other.err" || fail "card --default of the other build"
this_module=$(awk '$1 == "module" { print $2, $3 }' "$tmp/this.err")
other_module=$(awk '$1 == "module" { print $2, $3 }' "$tmp/other.err")
if cmp -s "$tmp/this.card" "$tmp/other.card" || [ "$this_module" = "$other_module" ] || [ -z "$this_module" ]; then
	cat "$tmp/this.card" "$tmp/this.err" "$tmp/other.card" "$tmp/other.err" >"$tmp/out"
	fail "two kernels print one card"
else
	echo "ok   a build of another kernel prints another card"
fi
case $this_module in
"0.2.0 sha256:"*) ;;
*) fail "this build's module line is '$this_module'" ;;
esac

# One Accord of one party, founded by this build under its own card.
cd "$tmp"
for k in mediator operator principal party; do
	./this key new --out "$k.key" >"$k.ref" || fail "key new $k"
done
echo '{"terms":[{"stance":"want","dir":"give","with":["role:peer"],"select":{"kind":["note"]}},{"stance":"want","dir":"get","with":["role:peer"],"select":{"kind":["note"]}}]}' >party.terms
./this declare --key party.key --terms party.terms >party.declaration || fail "declare"
mkdir openings
./this seal --key party.key --declaration party.declaration --out openings >party.seal 2>/dev/null || fail "seal"
echo "{\"roles\":[\"peer\"],\"principals\":[{\"principal\":\"$(cat principal.ref)\",\"roles\":[\"peer\"]}]}" >policy.json
./this bind --key principal.key --actor "$(cat party.ref)" --role peer --mediator "$(cat mediator.ref)" --until 2099-01-01T00:00:00Z \
	--policy policy.json >party.binding || fail "bind"
echo "{\"actor\":\"$(cat mediator.ref)\",\"operator\":\"$(cat operator.ref)\",\"runs\":\"process\"}" >mediator.json
found() {
	"$1" founding --policy policy.json --mediator mediator.json --card "$2" --seal party.seal --binding party.binding >"$3" 2>"$tmp/out"
}

if found ./this this.card this.set && [ "$(wc -l <"$tmp/out")" -eq 2 ]; then
	echo "ok   this build founds under its own card and names no other module"
else
	fail "this build founding under its own card"
fi

want="module $this_module is not this build's decision module, $other_module"
if found ./other this.card other.set && [ "$(sed -n 3p "$tmp/out")" = "$want" ]; then
	echo "ok   the other build founds under this build's card and names it another module's"
else
	fail "the other build founding under this build's card, want line 3: $want"
fi

./other window --key mediator.key --founding this.set --openings openings --socket "$tmp/s" --journal "$tmp/j" >"$tmp/out.stdout" 2>"$tmp/out"
rc=$?
if [ "$rc" -eq 3 ] && [ "$(cat "$tmp/out")" = "$want
error founding kernel" ] && [ ! -e "$tmp/j" ]; then
	echo "ok   the other build refuses a window of an Accord under this build's card"
else
	echo "exit $rc" >>"$tmp/out"
	fail "the other build's window under this build's card"
fi
[ "$failed" -eq 0 ]
