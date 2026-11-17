#!/bin/sh
# scripts/release-check.sh checks a directory of release artifacts (docs/release.md): every archive
# and SBOM in checksums.txt and matching it; four archives, each holding placket, LICENSE and
# README.md; and the binary for this platform naming a clean build of HEAD with --snapshot, or the tag
# with --release. With --release it first checks the signature and the provenance, with cosign and gh,
# and runs no binary of the directory when either refuses.
#
# The directory is built at run time: fake archives whose placket is a stub that prints a version and
# leaves a mark that it ran, fake SBOMs and their checksums. cosign and gh are stubs too.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
check=$root/scripts/release-check.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
out=$tmp/out

head=$(git -C "$root" rev-parse HEAD | cut -c1-12)

ran=$tmp/ran

# build <dir> <version> [<name>]: the artifacts of a build whose binary prints <version> and marks
# that it ran; their archives are named placket_<name>_<platform>.tar.gz, a snapshot of HEAD by
# default.
build() {
	rm -rf "$1" "$tmp/files" "$ran"
	mkdir -p "$1" "$tmp/files"
	printf '#!/bin/sh\n: >"%s"\n[ "$1" = version ] || exit 3\nprintf "version %%s\\ngo go1.27.4\\n" "%s"\n' "$ran" "$2" >"$tmp/files/placket"
	chmod 755 "$tmp/files/placket"
	echo license >"$tmp/files/LICENSE"
	echo readme >"$tmp/files/README.md"
	for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
		a=placket_${3:-0.0.0-SNAPSHOT-$head}_$p.tar.gz
		(cd "$tmp/files" && tar -czf "$1/$a" placket LICENSE README.md)
		echo "{\"spdxVersion\":\"SPDX-2.3\",\"name\":\"$a\"}" >"$1/$a.sbom.json"
	done
	sums "$1"
	echo bundle >"$1/checksums.txt.sigstore.json"
}

# sums <dir>: writes the checksums of the archives and SBOMs of <dir>.
sums() {
	(cd "$1" && shasum -a 256 ./*.tar.gz ./*.sbom.json | sed 's| \./| |') >"$tmp/checksums"
	mv "$tmp/checksums" "$1/checksums.txt"
}

# expect <name> <exit> <last line> <dir>: release-check --snapshot of <dir>, run at the root, ends
# with exactly <last line> and exits <exit>.
expect() {
	(cd "$root" && sh "$check" --snapshot "$4") >"$out" 2>&1
	judge "$@"
}

# expect_release <name> <exit> <last line> <dir>: the same, for release-check --release v0.1.0 of <dir>,
# with the stubs of cosign and gh first on the path.
expect_release() {
	(cd "$root" && PATH=$tmp/bin:$PATH sh "$check" --release v0.1.0 "$4") >"$out" 2>&1
	judge "$@"
}

judge() {
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

# Three archives in place of four, and a file checksums.txt names that is not there.
build "$dir" "$version"
rm "$dir/placket_0.0.0-SNAPSHOT-${head}_linux_arm64.tar.gz"
expect "a file checksums.txt names and the directory lacks is refused" 1 \
	"release-check: refused: placket_0.0.0-SNAPSHOT-${head}_linux_arm64.tar.gz: missing" "$dir"
build "$dir" "$version"
rm "$dir/placket_0.0.0-SNAPSHOT-${head}_linux_arm64.tar.gz"
grep -v 'linux_arm64.tar.gz$' "$dir/checksums.txt" >"$tmp/checksums"
mv "$tmp/checksums" "$dir/checksums.txt"
expect "three archives are refused" 1 "release-check: refused: $dir: 3 archives, not 4" "$dir"

# A dirty build of HEAD, and a build of another commit.
build "$dir" "$version+dirty"
expect "a dirty build is refused" 1 "release-check: refused: placket: dirty build" "$dir"
build "$dir" v0.0.0-20261201000000-000000000000
expect "a build of another commit is refused" 1 \
	"release-check: refused: placket: version v0.0.0-20261201000000-000000000000 is not a build of $head" "$dir"

# An archive that does not hold README.md.
build "$dir" "$version"
short=placket_0.0.0-SNAPSHOT-${head}_linux_arm64.tar.gz
(cd "$tmp/files" && tar -czf "$dir/$short" placket LICENSE)
sums "$dir"
expect "an archive without README.md is refused" 1 "release-check: refused: $short: contents" "$dir"

# --release. The stubs of cosign and gh write their arguments to a log, and exit with the status in
# COSIGN_EXIT or GH_EXIT, 0 when unset.
mkdir "$tmp/bin"
printf '#!/bin/sh\necho "cosign $*" >>"%s"\nexit "${COSIGN_EXIT:-0}"\n' "$tmp/calls" >"$tmp/bin/cosign"
printf '#!/bin/sh\necho "gh $*" >>"%s"\nexit "${GH_EXIT:-0}"\n' "$tmp/calls" >"$tmp/bin/gh"
chmod 755 "$tmp/bin/cosign" "$tmp/bin/gh"

# never_ran <name>: the binary of the directory did not run.
never_ran() {
	if [ -e "$ran" ]; then
		echo "FAIL $1: the binary of the directory ran"
		failed=1
	else
		echo "ok   $1: the binary of the directory did not run"
	fi
}

build "$dir" v0.1.0 0.1.0
rm -f "$tmp/calls"
if expect_release "a release whose signature, provenance and version hold passes" 0 "release-check: ok" "$dir"; then
	{
		echo "release-check: signature: ok"
		echo "release-check: checksums: ok, 8 files"
		for p in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
			echo "release-check: provenance: ok, placket_0.1.0_$p.tar.gz"
		done
		echo "release-check: archives: ok, 4"
		echo "release-check: version: ok, v0.1.0"
		echo "release-check: ok"
	} >"$tmp/want"
	{
		echo "cosign verify-blob --bundle $dir/checksums.txt.sigstore.json" \
			"--certificate-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main" \
			"--certificate-oidc-issuer https://token.actions.githubusercontent.com $dir/checksums.txt"
		for p in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do
			echo "gh attestation verify $dir/placket_0.1.0_$p.tar.gz --repo orilloai/placket" \
				"--signer-workflow orilloai/placket/.github/workflows/release.yml" \
				"--source-ref refs/heads/main --deny-self-hosted-runners"
		done
	} >"$tmp/want-calls"
	if ! cmp -s "$out" "$tmp/want"; then
		echo "FAIL a passing release check prints a line per check, the signature first"
		diff "$tmp/want" "$out" | sed 's/^/    | /'
		failed=1
	elif ! cmp -s "$tmp/calls" "$tmp/want-calls"; then
		echo "FAIL a release check calls cosign and gh as placket's release workflow on main signed"
		diff "$tmp/want-calls" "$tmp/calls" | sed 's/^/    | /'
		failed=1
	else
		echo "ok   a release check verifies the signature, then the provenance, as signed on main"
	fi
fi

build "$dir" v0.1.0 0.1.0
COSIGN_EXIT=1 expect_release "a release whose signature does not verify is refused" 1 \
	"release-check: refused: checksums.txt.sigstore.json: signature" "$dir"
never_ran "a signature that does not verify"

build "$dir" v0.1.0 0.1.0
printf 'x' | dd of="$dir/placket_0.1.0_linux_amd64.tar.gz" bs=1 seek=20 conv=notrunc 2>/dev/null
COSIGN_EXIT=1 expect_release "the signature is checked before the checksums" 1 \
	"release-check: refused: checksums.txt.sigstore.json: signature" "$dir"

build "$dir" v0.1.0 0.1.0
GH_EXIT=1 expect_release "a release whose provenance does not verify is refused" 1 \
	"release-check: refused: placket_0.1.0_darwin_amd64.tar.gz: provenance" "$dir"
never_ran "provenance that does not verify"

build "$dir" v0.2.0 0.1.0
expect_release "a release whose binary names another version is refused" 1 \
	"release-check: refused: placket: version v0.2.0 is not v0.1.0" "$dir"

exit $failed
