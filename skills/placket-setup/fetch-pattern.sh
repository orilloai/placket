#!/bin/sh
# fetch-pattern.sh: copies one of placket's patterns from the repository, at the release a placket
# binary is, so the pattern matches the binary that runs it (skills/placket-setup/SKILL.md).
#
#   fetch-pattern.sh <version> <pattern> <dir>   copies patterns/<pattern>/ into <dir>
#   fetch-pattern.sh --list <version>            prints the patterns patterns/README.md lists
#
# <version> is what `placket version` prints on its first line after "version". A release tag,
# vX.Y.Z, is taken at that tag. Any other version, a build with no tag, is taken at main, which can
# change at any time, and it says so. <dir> must not exist, or be an empty directory. It prints the
# commit it took. PLACKET_REPO, when set, is the repository to take it from, any URL or path git
# clones.
set -u

repo=${PLACKET_REPO:-https://github.com/orilloai/placket.git}

usage() {
	echo "fetch-pattern: usage: fetch-pattern.sh <version> <pattern> <dir> | --list <version>" >&2
	exit 2
}
say() { echo "fetch-pattern: $1"; }
refuse() {
	[ -s "${tmp:-/nonexistent}/log" ] && sed 's/^/    | /' "$tmp/log" >&2
	echo "fetch-pattern: refused: $1: $2" >&2
	exit 1
}

if [ "${1:-}" = --list ]; then
	[ $# -eq 2 ] || usage
	list=1 version=$2 pattern= dir=
else
	[ $# -eq 3 ] || usage
	list= version=$1 pattern=$2 dir=$3
	# A pattern's name is lower-case letters, digits and hyphens (patterns/README.md).
	expr "$pattern" : '[a-z0-9][a-z0-9-]*$' >/dev/null || usage
	if [ -e "$dir" ] || [ -L "$dir" ]; then
		[ -d "$dir" ] && [ ! -L "$dir" ] && [ -z "$(ls -A "$dir")" ] || refuse "$dir" "not an empty directory"
	fi
fi
[ -n "$version" ] || usage
command -v git >/dev/null || refuse git "not installed; fetch-pattern.sh fetches with it"

if expr "$version" : 'v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$' >/dev/null; then
	ref=$version
else
	ref=main
	say "$version is no release tag, so this takes main, which can change at any time"
fi

tmp=$(mktemp -d) || refuse mktemp "no temporary directory"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

# Only the ref's last commit, and of its files only those read below.
git clone --quiet --depth 1 --filter=blob:none --no-checkout --branch "$ref" -- "$repo" "$tmp/repo" >"$tmp/log" 2>&1 ||
	refuse "$ref" "could not fetch the repository at that ref"
: >"$tmp/log"
commit=$(git -C "$tmp/repo" rev-parse HEAD)

if [ -n "$list" ]; then
	git -C "$tmp/repo" show "HEAD:patterns/README.md" >"$tmp/readme" 2>"$tmp/log" || refuse patterns/README.md "not at $ref"
	awk '/^## / { on = ($0 == "## The patterns") } on && /^- `/' "$tmp/readme"
	exit 0
fi

[ -n "$(git -C "$tmp/repo" ls-tree -d HEAD "patterns/$pattern")" ] || refuse "$pattern" "no pattern of that name at $ref"
git -C "$tmp/repo" checkout --quiet HEAD -- "patterns/$pattern" >"$tmp/log" 2>&1 || refuse "$pattern" "could not check it out at $ref"
: >"$tmp/log"
mkdir -p "$dir" 2>"$tmp/log" || refuse "$dir" "could not make the directory"
cp -R "$tmp/repo/patterns/$pattern/." "$dir/" 2>"$tmp/log" || refuse "$dir" "could not copy the pattern into it"
say "patterns/$pattern/ at $ref, commit $commit, into $dir"
