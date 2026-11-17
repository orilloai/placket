#!/bin/sh
# scripts/release-detect.sh says whether a push to main is a release (docs/release.md): a release when
# the push adds one version section to CHANGELOG.md, a resume when that version's tag is at the push
# and its release is missing or a draft, and otherwise not a release push, or a refusal with its
# reason.
#
# Each case builds a small git repository at run time and runs the script in it, checking the exact
# line it prints, its exit code and the notes it writes.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
detect=$root/scripts/release-detect.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
out=$tmp/out
repo=$tmp/repo

g() { git -C "$repo" -c user.name=placket -c user.email=placket@example.invalid -c commit.gpgsign=false -c tag.gpgsign=false "$@"; }

# fresh: a repository whose first commit holds a changelog with an Unreleased section only.
fresh() {
	rm -rf "$repo"
	mkdir "$repo"
	g init -q
	printf '# Changelog\n\n## [Unreleased]\n\n- Something new.\n' >"$repo/CHANGELOG.md"
	g add -A
	g commit -q -m first
}

# commit <message>: commits the tree as it is, and prints the commit.
commit() {
	g add -A
	g commit -q -m "$1"
	g rev-parse HEAD
}

# release_section <version> <date>: the changelog with the Unreleased section moved under <version>,
# above the sections already there.
release_section() {
	{
		printf '# Changelog\n\n## [Unreleased]\n\n## [%s] - %s\n\n### Added\n\n- Something new.\n- `placket version`.\n\n' "$1" "$2"
		sed -n '/^## \[[0-9]/,$p' "$repo/CHANGELOG.md"
	} >"$tmp/changelog"
	mv "$tmp/changelog" "$repo/CHANGELOG.md"
}

# expect <name> <exit> <line> <args>...: the script, run in the repository, prints exactly <line> and
# exits <exit>.
expect() {
	name=$1 code=$2 want=$3
	shift 3
	(cd "$repo" && sh "$detect" "$@") >"$out" 2>&1
	got=$?
	if [ "$got" -ne "$code" ] || [ "$(cat "$out")" != "$want" ]; then
		echo "FAIL $name: want exit $code and '$want', got exit $got"
		sed 's/^/    | /' "$out"
		failed=1
		return 1
	fi
	echo "ok   $name"
}

notes=$tmp/notes.md
printf '### Added\n\n- Something new.\n- `placket version`.\n' >"$tmp/want-notes"

# D1: a push that adds one section, with no tag yet, is a release, and its notes are the section's body.
fresh
before=$(g rev-parse HEAD)
release_section 0.1.0 2026-12-01
after=$(commit 'Release 0.1.0')
if expect "a push that adds one section is a release" 0 "release 0.1.0" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none --notes "$notes"; then
	if ! cmp -s "$notes" "$tmp/want-notes"; then
		echo "FAIL the notes are not the section's body"
		diff "$tmp/want-notes" "$notes" | sed 's/^/    | /'
		failed=1
	else
		echo "ok   the notes are the section's body"
	fi
fi
expect "a section dated the day before is a release" 0 "release 0.1.0" \
	--before "$before" --after "$after" --today 2026-12-02 --release-state none

# D2: the same push, its tag at it and its release a draft or missing, a week later, is a resume.
g tag -a v0.1.0 -m v0.1.0 "$after"
rm -f "$notes"
if expect "a tagged push whose release is a draft is a resume" 0 "resume 0.1.0" \
	--before "$before" --after "$after" --today 2026-12-08 --release-state draft --notes "$notes"; then
	if ! cmp -s "$notes" "$tmp/want-notes"; then
		echo "FAIL the resume's notes are not the section's body"
		failed=1
	fi
fi
expect "a tagged push with no release is a resume" 0 "resume 0.1.0" \
	--before "$before" --after "$after" --today 2026-12-08 --release-state none

# D3: published already, or no section added, is not a release push.
expect "a tagged push whose release is published is not a release push" 0 "not a release push" \
	--before "$before" --after "$after" --today 2026-12-08 --release-state published
echo more >"$repo/README.md"
next=$(commit 'Not a release')
expect "a push that adds no section is not a release push" 0 "not a release push" \
	--before "$after" --after "$next" --today 2026-12-08 --release-state none
expect "a push that adds no section is not a release push in a private repository" 0 "not a release push" \
	--before "$after" --after "$next" --today 2026-12-08 --release-state none --private
printf '\n- A fix.\n' >>"$repo/CHANGELOG.md"
edited=$(commit 'An edit under a released section')
expect "a push that edits a released section is not a release push" 0 "not a release push" \
	--before "$next" --after "$edited" --today 2026-12-08 --release-state none

# D4: the refusals.
fresh
before=$(g rev-parse HEAD)
release_section 0.1.0 2026-12-01
release_section 0.1.1 2026-12-01
after=$(commit 'Two releases')
expect "two new sections are refused" 1 "release: refused: 2 new version sections" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none

fresh
g tag -a v0.2.0 -m v0.2.0
before=$(g rev-parse HEAD)
release_section 0.1.0 2026-12-01
after=$(commit 'An older release')
expect "a version below the latest tag is refused" 1 "release: refused: 0.1.0 is not above the latest tag v0.2.0" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none
g tag -d v0.2.0 >/dev/null
g tag -a v0.1.0 -m v0.1.0 "$before"
expect "a version tagged at another commit is refused" 1 "release: refused: v0.1.0 is a tag at another commit" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none
g tag -d v0.1.0 >/dev/null
g tag -a v0.1.0-rc.1 -m rc "$before"
g tag -a v0.0.9 -m v0.0.9 "$before"
expect "a version above the latest tag is a release" 0 "release 0.1.0" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none
g tag -a v0.1.0 -m v0.1.0 "$after"
g tag -a v0.1.1 -m v0.1.1 "$before"
expect "a resume below the latest tag is refused" 1 "release: refused: 0.1.0 is not above the latest tag v0.1.1" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state draft
g tag -d v0.1.0 v0.1.1 >/dev/null
expect "a section dated two days after today is refused" 1 "release: refused: 0.1.0 is dated 2026-12-01, more than one day from 2026-11-29" \
	--before "$before" --after "$after" --today 2026-11-29 --release-state none
expect "a section dated two days before today is refused" 1 "release: refused: 0.1.0 is dated 2026-12-01, more than one day from 2026-12-03" \
	--before "$before" --after "$after" --today 2026-12-03 --release-state none
expect "a section dated across a month and a year is a release" 0 "release 0.1.0" \
	--before "$before" --after "$after" --today 2026-11-30 --release-state none
expect "a release in a private repository is refused" 1 "release: refused: repository is private" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none --private
g tag -a v0.1.0 -m v0.1.0 "$after"
expect "a resume in a private repository is refused" 1 "release: refused: repository is private" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state draft --private
g tag -d v0.1.0 >/dev/null

echo more >"$repo/README.md"
plain=$(commit 'No section')
g tag -a v0.3.0 -m v0.3.0 "$plain"
expect "a tag at a push whose changelog has no section for it is refused" 1 \
	"release: refused: v0.3.0 is a tag at this push, and CHANGELOG.md has no section for 0.3.0" \
	--before "$after" --after "$plain" --today 2026-12-01 --release-state none

# A push whose before is the zero commit reads as one from an empty tree.
fresh
release_section 0.1.0 2026-12-31
after=$(commit 'A release at the year end')
expect "a section dated a day before a new year is a release" 0 "release 0.1.0" \
	--before 0000000000000000000000000000000000000000 --after "$after" --today 2027-01-01 --release-state none

# Arguments that cannot be read are a usage error.
expect "a release state that is not one of the three is refused" 2 "release-detect: usage: --release-state none|draft|published" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state maybe
expect "a day that is not a date is refused" 2 "release-detect: usage: --today YYYY-MM-DD" \
	--before "$before" --after "$after" --today 12/01/2026 --release-state none

# A changelog that ends with link references: the notes stop at the first of them.
fresh
before=$(g rev-parse HEAD)
release_section 0.1.0 2026-12-01
printf '[Unreleased]: https://github.com/orilloai/placket/compare/v0.1.0...HEAD\n[0.1.0]: https://github.com/orilloai/placket/releases/tag/v0.1.0\n' >>"$repo/CHANGELOG.md"
after=$(commit 'Release 0.1.0, with links')
rm -f "$notes"
if expect "a changelog ending with link references is a release" 0 "release 0.1.0" \
	--before "$before" --after "$after" --today 2026-12-01 --release-state none --notes "$notes"; then
	if ! cmp -s "$notes" "$tmp/want-notes"; then
		echo "FAIL the notes take in the link references"
		diff "$tmp/want-notes" "$notes" | sed 's/^/    | /'
		failed=1
	else
		echo "ok   the notes stop at the link references"
	fi
fi

exit $failed
