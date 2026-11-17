#!/bin/sh
# scripts/release-check.sh --snapshot checks a directory of release artifacts (docs/release.md):
# every archive and SBOM in checksums.txt and matching it; four archives, each holding placket, LICENSE
# and README.md; and the binary for this platform naming a clean build of HEAD.
#
# The directory is built at run time: fake archives whose placket is a stub that prints the version a
# snapshot of HEAD has, fake SBOMs and their checksums.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
check=$root/scripts/release-check.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
out=$tmp/out

head=$(git -C "$root" rev-parse HEAD | cut -c1-12)

# build <dir> <version>: the artifacts of a snapshot whose binary prints <version>.
build() {
	rm -rf "$1" "$tmp/files"
	mkdir -p "$1" "$tmp/files"
	printf '#!/bin/sh\n[ "$1" = version ] || exit 3\nprintf "version %%s\\ngo go1.27.4\\n" "%s"\n' "$2" >"$tmp/files/placket"
	chmod 755 "$tmp/files/placket"
	echo license >"$tmp/files/LICENSE"
	echo readme >"$tmp/files/README.md"
	for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
		a=placket_0.0.0-SNAPSHOT-${head}_$p.tar.gz
		(cd "$tmp/files" && tar -czf "$1/$a" placket LICENSE README.md)
		echo "{\"spdxVersion\":\"SPDX-2.3\",\"name\":\"$a\"}" >"$1/$a.sbom.json"
	done
	(cd "$1" && shasum -a 256 ./*.tar.gz ./*.sbom.json | sed 's| \./| |') >"$tmp/checksums"
	mv "$tmp/checksums" "$1/checksums.txt"
}

# expect <name> <exit> <last line> <dir>: release-check --snapshot of <dir>, run at the root, ends
# with exactly <last line> and exits <exit>.
expect() {
	(cd "$root" && sh "$check" --snapshot "$4") >"$out" 2>&1
	got=$?
	if [ "$got" -ne "$2" ] || [ "$(tail -n 1 "$out")" != "$3" ]; then
		echo "FAIL $1: want exit $2 and last line '$3', got exit $got"
		sed 's/^/    | /' "$out"
		failed=1
		return 1
	fi
	echo "ok   $1"
}

dir=$tmp/dist
version=v0.0.0-20261201000000-$head
build "$dir" "$version"
if expect "a snapshot of HEAD passes" 0 "release-check: ok" "$dir"; then
	if [ "$(sed -n 1p "$out")" != "release-check: checksums: ok, 8 files" ] ||
		[ "$(sed -n 2p "$out")" != "release-check: archives: ok, 4" ] ||
		[ "$(sed -n 3p "$out")" != "release-check: version: ok, $version" ]; then
		echo "FAIL a passing check prints a line per check"
		sed 's/^/    | /' "$out"
		failed=1
	fi
fi

# One byte of one archive altered.
altered=placket_0.0.0-SNAPSHOT-${head}_darwin_arm64.tar.gz
printf 'x' | dd of="$dir/$altered" bs=1 seek=20 conv=notrunc 2>/dev/null
expect "an archive altered by one byte is refused" 1 "release-check: refused: $altered: checksum" "$dir"

# An SBOM with no line in checksums.txt.
build "$dir" "$version"
grep -v 'linux_arm64.tar.gz.sbom.json$' "$dir/checksums.txt" >"$tmp/checksums"
mv "$tmp/checksums" "$dir/checksums.txt"
expect "an SBOM with no checksum is refused" 1 \
	"release-check: refused: placket_0.0.0-SNAPSHOT-${head}_linux_arm64.tar.gz.sbom.json: no checksum" "$dir"

# Three archives in place of four.
build "$dir" "$version"
rm "$dir/placket_0.0.0-SNAPSHOT-${head}_linux_arm64.tar.gz"
expect "three archives are refused" 1 "release-check: refused: $dir: 3 archives, not 4" "$dir"

# A dirty build of HEAD, and a build of another commit.
build "$dir" "$version+dirty"
expect "a dirty build is refused" 1 "release-check: refused: placket: dirty build" "$dir"
build "$dir" v0.0.0-20261201000000-000000000000
expect "a build of another commit is refused" 1 \
	"release-check: refused: placket: version v0.0.0-20261201000000-000000000000 is not a build of $head" "$dir"

exit $failed
