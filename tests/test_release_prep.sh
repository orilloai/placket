#!/bin/sh
# The files the first release ships with, ready for it: NOTICE names both holders of placket's
# copyright, and CHANGELOG.md's Unreleased section reads as release notes, naming no work order or
# unit of the work that built them (no "ORI-", no "unit ").
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

unreleased=$(awk '/^## / { in_section = ($0 == "## Unreleased"); next } in_section' "$root/CHANGELOG.md")
[ -n "$unreleased" ] || fail "CHANGELOG.md: no Unreleased section, or an empty one"
printf '%s\n' "$unreleased" | grep -n -e 'ORI-' -e 'unit ' | sed 's/^/FAIL CHANGELOG.md: Unreleased names a work order or a unit: /' | grep . && failed=1

[ "$failed" -eq 0 ] && echo "ok   NOTICE and CHANGELOG.md's Unreleased are ready for a release"
[ "$failed" -eq 0 ]
