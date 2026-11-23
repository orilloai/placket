#!/bin/sh
# The files a release ships with, ready for it: NOTICE names both holders of placket's copyright, and
# CHANGELOG.md's newest version section, "## [X.Y.Z] - YYYY-MM-DD", is there and reads as release
# notes, as does Unreleased: neither names a work order or unit of the work that built them (no
# "ORI-", no "unit ").
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
failed=0
fail() {
	echo "FAIL $1"
	failed=1
}

if [ ! -f "$root/NOTICE" ]; then
	fail "NOTICE: missing"
else
	for holder in "Orillo Labs SL" "Jocelyn Lindenger"; do
		grep -Fq "$holder" "$root/NOTICE" || fail "NOTICE: does not name $holder"
	done
fi

newest=$(awk '/^## / { if (seen) exit; in_section = ($0 ~ /^## \[[0-9]+\.[0-9]+\.[0-9]+\] - [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/); seen = in_section; next } in_section' "$root/CHANGELOG.md")
[ -n "$newest" ] || fail "CHANGELOG.md: no version section, or an empty newest one"
unreleased=$(awk '/^## / { in_section = ($0 == "## Unreleased"); next } in_section' "$root/CHANGELOG.md")
printf '%s\n%s\n' "$unreleased" "$newest" | grep -n -e 'ORI-' -e 'unit ' | sed 's/^/FAIL CHANGELOG.md: release notes name a work order or a unit: /' | grep . && failed=1

[ "$failed" -eq 0 ] && echo "ok   NOTICE and CHANGELOG.md's newest version section are ready for a release"
[ "$failed" -eq 0 ]
