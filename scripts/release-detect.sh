#!/bin/sh
# release-detect.sh: is this push to main a release? (docs/release.md)
#
#   release-detect.sh --before SHA --after SHA --today YYYY-MM-DD --release-state none|draft|published
#                     [--private] [--notes FILE]
#
# It reads the git history and tags of the repository it runs in, and makes no network call: the
# release workflow passes in today's date and the state of the release for any tag at the push. It
# prints exactly one line:
#
#   release X.Y.Z        the push added exactly one section "## [X.Y.Z] - YYYY-MM-DD" to CHANGELOG.md,
#                        and vX.Y.Z is not a tag
#   resume X.Y.Z         vX.Y.Z is a tag at the push, and its release is missing or a draft: an earlier
#                        run stopped partway
#   not a release push   no section was added, or the version is tagged here and published; exit 0
#   release: refused: <reason>, exit 1
#
# For a release or a resume, --notes FILE gets the body of the version's section. "The latest tag" is
# the greatest vX.Y.Z tag other than one at the push. A malformed argument prints
# "release-detect: usage: ..." and exits 2.
set -u

usage() {
	echo "release-detect: usage: $1"
	exit 2
}
refuse() {
	echo "release: refused: $1"
	exit 1
}

before='' after='' today='' state='' private=false notes=''
while [ $# -gt 0 ]; do
	case $1 in
	--before | --after | --today | --release-state | --notes)
		[ $# -ge 2 ] || usage "$1 needs a value"
		case $1 in
		--before) before=$2 ;;
		--after) after=$2 ;;
		--today) today=$2 ;;
		--release-state) state=$2 ;;
		--notes) notes=$2 ;;
		esac
		shift 2
		;;
	--private)
		private=true
		shift
		;;
	*) usage "release-detect.sh --before SHA --after SHA --today YYYY-MM-DD --release-state none|draft|published [--private] [--notes FILE]" ;;
	esac
done
case $today in
[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;;
*) usage "--today YYYY-MM-DD" ;;
esac
case $state in
none | draft | published) ;;
*) usage "--release-state none|draft|published" ;;
esac
zero=0000000000000000000000000000000000000000
git rev-parse -q --verify "$after^{commit}" >/dev/null || usage "--after: not a commit"
[ "$before" = "$zero" ] || git rev-parse -q --verify "$before^{commit}" >/dev/null || usage "--before: not a commit"

# sections <commit>: "X.Y.Z YYYY-MM-DD" for each version section of CHANGELOG.md at the commit.
sections() {
	[ "$1" = "$zero" ] && return 0
	git show "$1:CHANGELOG.md" 2>/dev/null |
		sed -n 's/^## \[\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)\] - \([0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]\)[[:space:]]*$/\1 \2/p'
}

# versions: the X.Y.Z of each vX.Y.Z tag read on standard input, greatest last.
versions() {
	sed -n 's/^v\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)$/\1/p' | sort -t. -k1,1n -k2,2n -k3,3n
}

# above <a> <b>: whether version a is greater than version b.
above() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		split(a, x, "."); split(b, y, ".")
		for (i = 1; i <= 3; i++) { if (x[i] + 0 > y[i] + 0) exit 0; if (x[i] + 0 < y[i] + 0) exit 1 }
		exit 1 }'
}

# days <a> <b>: the number of days from date a to date b, without the sign.
days() {
	awk -v a="$1" -v b="$2" '
		function day(s,   y, m, d) {
			y = substr(s, 1, 4) + 0; m = substr(s, 6, 2) + 0; d = substr(s, 9, 2) + 0
			if (m <= 2) { y--; m += 12 }
			return 365 * y + int(y / 4) - int(y / 100) + int(y / 400) + int((153 * (m - 3) + 2) / 5) + d
		}
		BEGIN { n = day(b) - day(a); print (n < 0 ? -n : n) }'
}

# notes <version>: writes the body of the version's section at the push to the notes file, its blank
# lines at either end left out.
write_notes() {
	[ -n "$notes" ] || return 0
	git show "$after:CHANGELOG.md" | awk -v h="## [$1] - " '
		in_section && (/^## / || /^\[[^]]*\]: /) { exit }
		in_section { line[++n] = $0 }
		index($0, h) == 1 { in_section = 1 }
		END {
			s = 1; while (s <= n && line[s] ~ /^[[:space:]]*$/) s++
			e = n; while (e >= s && line[e] ~ /^[[:space:]]*$/) e--
			for (i = s; i <= e; i++) print line[i]
		}' >"$notes" || usage "--notes: cannot write $notes"
}

# not_above_latest <version> <tag at the push>: refuses a version not greater than the latest tag.
not_above_latest() {
	latest=$(git tag -l 'v*' | grep -v -x -F "${2:-none}" | versions | tail -n 1)
	if [ -n "$latest" ] && ! above "$1" "$latest"; then
		refuse "$1 is not above the latest tag v$latest"
	fi
}

here=$(git tag --points-at "$after" 'v*' | versions | tail -n 1)
if [ -n "$here" ]; then
	if [ "$state" = published ]; then
		echo "not a release push"
		exit 0
	fi
	sections "$after" | awk -v v="$here" '$1 == v { found = 1 } END { exit !found }' ||
		refuse "v$here is a tag at this push, and CHANGELOG.md has no section for $here"
	not_above_latest "$here" "v$here"
	[ "$private" = false ] || refuse "repository is private"
	write_notes "$here"
	echo "resume $here"
	exit 0
fi

new=$(OLD=$(sections "$before") && export OLD && sections "$after" | awk '
	BEGIN { n = split(ENVIRON["OLD"], l, "\n"); for (i = 1; i <= n; i++) { split(l[i], x, " "); old[x[1]] = 1 } }
	!($1 in old)')
count=$(printf '%s' "$new" | grep -c .)
if [ "$count" -eq 0 ]; then
	echo "not a release push"
	exit 0
fi
[ "$count" -eq 1 ] || refuse "$count new version sections"
version=${new% *}
dated=${new#* }
git rev-parse -q --verify "refs/tags/v$version" >/dev/null && refuse "v$version is a tag at another commit"
not_above_latest "$version" ""
[ "$(days "$dated" "$today")" -le 1 ] || refuse "$version is dated $dated, more than one day from $today"
[ "$private" = false ] || refuse "repository is private"
write_notes "$version"
echo "release $version"
