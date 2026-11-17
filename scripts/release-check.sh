#!/bin/sh
# release-check.sh: checks a directory of placket's release artifacts (docs/release.md).
#
#   release-check.sh --snapshot DIR       a snapshot built from this checkout's HEAD
#   release-check.sh --release vX.Y.Z DIR the release vX.Y.Z, as downloaded
#
# It prints one line per check and ends with "release-check: ok", or with
# "release-check: refused: <file>: <reason>" and exit 1. With --release, it needs cosign and a
# signed-in gh, and checks the signature and the provenance first: it opens no archive and runs no
# binary from DIR until both hold.
#
#   signature   (--release) cosign verifies checksums.txt.sigstore.json, the bundle that signs
#               checksums.txt, as signed by placket's release workflow on main
#   checksums   every line of checksums.txt names a file of DIR whose SHA-256 it is, and every archive
#               and SBOM in DIR has its line
#   provenance  (--release) gh attestation verify finds each archive's build provenance, signed by
#               exactly the identity the signature has, built on GitHub's own runners
#   archives    four archives, linux and darwin on amd64 and arm64, each holding exactly placket,
#               LICENSE and README.md
#   version     the binary for this machine's platform runs `placket version`, and its version is not
#               a +dirty build: with --release it is the tag, with --snapshot it ends with the first 12
#               characters of HEAD's commit
set -u

# The release workflow is planned and not in the repository yet (docs/release.md); a release it
# signs will carry this identity.
workflow=orilloai/placket/.github/workflows/release.yml
identity=https://github.com/$workflow@refs/heads/main
issuer=https://token.actions.githubusercontent.com

usage() {
	echo "release-check: usage: release-check.sh --snapshot DIR | --release vX.Y.Z DIR"
	exit 2
}
say() { echo "release-check: $1"; }
refuse() {
	[ -s "${tmp:-/nonexistent}/log" ] && sed 's/^/    | /' "$tmp/log"
	echo "release-check: refused: $1: $2"
	exit 1
}

case ${1:-} in
--snapshot)
	[ $# -eq 2 ] || usage
	mode=snapshot tag='' dir=$2
	;;
--release)
	[ $# -eq 3 ] || usage
	mode=release tag=$2 dir=$3
	expr "$tag" : 'v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$' >/dev/null || usage
	;;
*) usage ;;
esac
[ -d "$dir" ] || refuse "$dir" "not a directory"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM

sums=$dir/checksums.txt
[ -f "$sums" ] || refuse checksums.txt missing

# signature
if [ "$mode" = release ]; then
	command -v cosign >/dev/null || refuse cosign "not installed"
	command -v gh >/dev/null || refuse gh "not installed"
	cosign verify-blob --bundle "$dir/checksums.txt.sigstore.json" \
		--certificate-identity "$identity" --certificate-oidc-issuer "$issuer" \
		"$sums" >"$tmp/log" 2>&1 || refuse checksums.txt.sigstore.json signature
	: >"$tmp/log"
	say "signature: ok"
fi

# checksums
lines=0
while read -r want name; do
	name=${name#\*}
	[ -f "$dir/$name" ] || refuse "$name" missing
	got=$(shasum -a 256 "$dir/$name" | cut -d' ' -f1)
	[ "$got" = "$want" ] || refuse "$name" checksum
	lines=$((lines + 1))
done <"$sums"
for f in "$dir"/*.tar.gz "$dir"/*.sbom.json; do
	[ -e "$f" ] || continue
	name=$(basename "$f")
	awk -v f="$name" '$2 == f || $2 == "*" f { found = 1 } END { exit !found }' "$sums" || refuse "$name" "no checksum"
done
say "checksums: ok, $lines files"

# provenance
if [ "$mode" = release ]; then
	for f in "$dir"/*.tar.gz; do
		[ -e "$f" ] || continue
		gh attestation verify "$f" --repo orilloai/placket \
			--cert-identity "$identity" --cert-oidc-issuer "$issuer" \
			--source-ref refs/heads/main --deny-self-hosted-runners >"$tmp/log" 2>&1 ||
			refuse "$(basename "$f")" provenance
		: >"$tmp/log"
		say "provenance: ok, $(basename "$f")"
	done
fi

# archives
archives=0
for f in "$dir"/*.tar.gz; do
	[ -e "$f" ] || continue
	archives=$((archives + 1))
done
[ "$archives" -eq 4 ] || refuse "$dir" "$archives archives, not 4"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
	set -- "$dir"/*_"$p".tar.gz
	[ $# -eq 1 ] && [ -e "$1" ] || refuse "$dir" "no one archive for $p"
	held=$(tar -tzf "$1" 2>/dev/null | sed 's|^\./||' | sort | tr '\n' ' ')
	[ "$held" = "LICENSE README.md placket " ] || refuse "$(basename "$1")" contents
done
say "archives: ok, $archives"

# version
case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; *) os=$(uname -s) ;; esac
case $(uname -m) in x86_64 | amd64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; *) arch=$(uname -m) ;; esac
set -- "$dir"/*_"${os}_$arch".tar.gz
[ -e "$1" ] || refuse "$dir" "no archive for ${os}_$arch"
tar -xzf "$1" -C "$tmp" placket 2>"$tmp/log" || refuse "$(basename "$1")" "no placket"
"$tmp/placket" version >"$tmp/version" 2>"$tmp/log" || refuse placket "version does not run"
version=$(sed -n 's/^version //p' "$tmp/version" | head -n 1)
case $version in
*+dirty) refuse placket "dirty build" ;;
esac
if [ "$mode" = release ]; then
	[ "$version" = "$tag" ] || refuse placket "version $version is not $tag"
else
	head=$(git rev-parse HEAD 2>"$tmp/log" | cut -c1-12)
	[ -n "$head" ] || refuse placket "no HEAD to check the version against"
	case $version in
	*-"$head") ;;
	*) refuse placket "version $version is not a build of $head" ;;
	esac
fi
say "version: ok, $version"
say ok
