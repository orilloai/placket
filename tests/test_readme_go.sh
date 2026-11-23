#!/bin/sh
# The README's Go program runs: built in a module of its own against the core in this tree, and fed a
# receipts vector's reference and receipts, it prints exactly the number the vector expects to
# verify, and exits 0. It refuses a changed signature by naming the receipt, and an input the
# canonical form refuses, such as a repeated name or a second value. The README's Go section gives
# the go get line of a tagged core and says what the core's packages promise.
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

# The section from its heading to the next one of its level.
awk '/^## / { on = ($0 == "## Use the core from Go") } on' "$root/README.md" >"$tmp/section.md"
if [ ! -s "$tmp/section.md" ]; then
	echo "FAIL README.md has no section '## Use the core from Go'"
	exit 1
fi
for want in 'go get github.com/orilloai/placket/core@v0.1.0' '0.x, may change' 'internal/'; do
	if grep -qF "$want" "$tmp/section.md"; then
		echo "ok   the Go section says: $want"
	else
		fail "the Go section does not say: $want"
	fi
done

# The section's one go block, as a main package.
mkdir "$tmp/example"
awk '/^```go$/ { n++; on = 1; next } /^```$/ { on = 0 } on && n == 1' "$tmp/section.md" >"$tmp/example/main.go"
if [ "$(grep -c '^```go$' "$tmp/section.md")" -ne 1 ] || [ ! -s "$tmp/example/main.go" ]; then
	echo "FAIL the Go section does not hold exactly one go block"
	exit 1
fi
printf 'module example.com/verify\n\ngo 1.27\n\nrequire github.com/orilloai/placket/core v0.1.0\n\nreplace github.com/orilloai/placket/core => %s/core\n' "$root" >"$tmp/example/go.mod"
if ! (cd "$tmp/example" && GOWORK=off GOFLAGS=-mod=mod GOPROXY=off go build -o "$tmp/verify" .) >"$tmp/out" 2>&1; then
	echo "FAIL the README's Go program does not build against the core in this tree"
	sed 's/^/    | /' "$tmp/out"
	exit 1
fi

vector=$root/vectors/receipts/a-chain-verifies.json
want="verified: $(jq -r '.expect.verified' "$vector")"
"$tmp/verify" <"$vector" >"$tmp/out" 2>&1
rc=$?
if [ "$rc" -eq 0 ] && [ "$(cat "$tmp/out")" = "$want" ] && [ "$want" = "verified: 3" ]; then
	echo "ok   the README's Go program verifies the vector's receipts and prints '$want'"
else
	echo "exit $rc" >>"$tmp/out"
	fail "the README's Go program on $vector, want exactly '$want' and exit 0"
fi

# refused NAME FILE WANT: the program, fed FILE, prints exactly WANT on standard error, nothing on
# standard output, and exits 1.
refused() {
	"$tmp/verify" <"$2" >"$tmp/stdout" 2>"$tmp/stderr"
	rc=$?
	if [ "$rc" -eq 1 ] && [ ! -s "$tmp/stdout" ] && [ "$(cat "$tmp/stderr")" = "$3" ]; then
		echo "ok   the README's Go program refuses $1: $3"
	else
		{ cat "$tmp/stdout" "$tmp/stderr"; echo "exit $rc"; } >"$tmp/out"
		fail "the README's Go program on $1, want exactly '$3' on standard error and exit 1"
	fi
}

# A receipt changed by one character of its signature does not verify, and the first receipt that
# fails is named.
jq '.receipts[1].receipt.signatures[0].sig |= (if startswith("A") then "B" else "A" end + .[1:])' "$vector" >"$tmp/broken.json"
refused 'a receipt whose signature was changed' "$tmp/broken.json" 'not verified: receipts: signature at index 1'

# The vector on one line, and its length: a name repeated after its last member, or bytes after it,
# is refused at that offset.
jq -c . "$vector" | tr -d '\n' >"$tmp/line.json"
end=$(wc -c <"$tmp/line.json" | tr -d ' ')
sed 's/}$/,"receipts":[]}/' "$tmp/line.json" >"$tmp/repeated.json"
refused 'a repeated member name' "$tmp/repeated.json" "error input: canonical: duplicate-key at offset $end"
{ cat "$tmp/line.json"; printf '{}'; } >"$tmp/trailing.json"
refused 'a second JSON value after the object' "$tmp/trailing.json" "error input: canonical: invalid-json at offset $end"

# A name that differs only in case is another member, which the program does not read.
sed 's/}$/,"Receipts":[]}/' "$tmp/line.json" >"$tmp/case.json"
"$tmp/verify" <"$tmp/case.json" >"$tmp/out" 2>&1
rc=$?
if [ "$rc" -eq 0 ] && [ "$(cat "$tmp/out")" = "$want" ]; then
	echo "ok   the README's Go program reads members by their exact names"
else
	echo "exit $rc" >>"$tmp/out"
	fail "the README's Go program with a member 'Receipts' added, want exactly '$want' and exit 0"
fi

exit $failed
