#!/bin/sh
# placket version names a build at a tag by that tag: Go stamps the tag as the main module's version
# when it builds a clean git tree at it, and version prints it as it is (docs/release.md).
#
# It copies the modules into a new git repository, commits and tags them, builds the binary outside
# that tree, so the tree stays clean, and reads the first line version prints.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

repo=$tmp/repo
mkdir "$repo"
cp -R "$root/go.mod" "$root/core" "$root/adapters" "$root/cmd" "$repo/"
g() { git -C "$repo" -c user.name=placket -c user.email=placket@example.invalid -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }
if ! { g init -q && g add -A && g commit -q -m 'a release' && g tag -a v0.0.1-test -m v0.0.1-test; } >"$tmp/out" 2>&1; then
	echo "FAIL the tagged copy could not be made"
	sed 's/^/    | /' "$tmp/out"
	exit 1
fi
if ! (cd "$repo" && go build -o "$tmp/placket" ./cmd/placket) >"$tmp/out" 2>&1; then
	echo "FAIL the tagged copy does not build"
	sed 's/^/    | /' "$tmp/out"
	exit 1
fi
if ! "$tmp/placket" version >"$tmp/out" 2>&1; then
	echo "FAIL version exits non-zero"
	sed 's/^/    | /' "$tmp/out"
	exit 1
fi
if [ "$(sed -n 1p "$tmp/out")" != "version v0.0.1-test" ]; then
	echo "FAIL a build at the tag v0.0.1-test does not print it"
	sed 's/^/    | /' "$tmp/out"
	exit 1
fi
echo "ok   a build at a tag prints the tag as its version"
