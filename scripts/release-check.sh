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
#               and SBOM in DIR, and install.sh, has its line; with --snapshot, install.sh is the
#               checkout's, since GoReleaser leaves it there
#   provenance  (--release) gh attestation verify finds each archive's build provenance, signed by
#               exactly the identity the signature has, built on GitHub's own runners
#   archives    four archives, linux and darwin on amd64 and arm64, each holding exactly placket,
#               LICENSE, NOTICE and README.md
#   cask        (--snapshot) the Homebrew cask, DIR/homebrew/Casks/placket.rb: each URL names this
#               release's archive for the platform of its block, at its line of checksums.txt, one for
#               each of the four, and on macOS a step after install clears placket's quarantine mark
#   version     the binary for this machine's platform runs `placket version`, and its version is not
#               a +dirty build: with --release it is the tag, with --snapshot it ends with the first 12
#               characters of HEAD's commit
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
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
# A last line with no line feed after it is read all the same.
while read -r want name || [ -n "$want" ]; do
	name=${name#\*}
	file=$dir/$name
	[ "$mode" = snapshot ] && [ "$name" = install.sh ] && file=$root/install.sh
	[ -f "$file" ] || refuse "$name" missing
	got=$(shasum -a 256 "$file" | cut -d' ' -f1)
	[ "$got" = "$want" ] || refuse "$name" checksum
	lines=$((lines + 1))
done <"$sums"
for f in "$dir"/*.tar.gz "$dir"/*.sbom.json "$dir/install.sh"; do
	[ -e "$f" ] || [ "$f" = "$dir/install.sh" ] || continue
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
	[ "$held" = "LICENSE NOTICE README.md placket " ] || refuse "$(basename "$1")" contents
done
say "archives: ok, $archives"

# cask: read block by block. A url's name has the cask's version for #{version}; it must be the
# archive of its block's platform, with the sha256 before it as its line of checksums.txt.
if [ "$mode" = snapshot ]; then
	cask=homebrew/Casks/placket.rb
	[ -f "$dir/$cask" ] || refuse "$cask" missing
	why=$(awk -v sums="$sums" '
		BEGIN {
			while ((getline l < sums) > 0) {
				split(l, f, " ")
				n = f[2]
				sub(/^\*/, "", n)
				if (n ~ /\.tar\.gz$/) sum[n] = f[1]
			}
		}
		function fail(why) { print why; failed = 1; exit }
		{ line = $0; sub(/^[ \t]+/, "", line); sub(/[ \t]+$/, "", line) }
		line ~ /^version "[^"]*"$/ { v = line; sub(/^version "/, "", v); sub(/"$/, "", v) }
		line == "on_macos do" { os = "darwin"; if (post) mac = 1 }
		line == "on_linux do" { os = "linux" }
		line == "on_arm do" { arch = "arm64" }
		line == "on_intel do" { arch = "amd64" }
		line == "postflight_steps do" { post = 1 }
		line ~ /^sha256 "[0-9a-f]*"$/ { s = line; sub(/^sha256 "/, "", s); sub(/"$/, "", s) }
		line ~ /^url "/ {
			u = line; sub(/^url "/, "", u); sub(/"$/, "", u)
			gsub(/#\{version\}/, v, u)
			p = os "_" arch
			if (u !~ /^https:\/\/github\.com\/orilloai\/placket\/releases\/download\/v[^\/]*\/[^\/]*$/) fail("url of " p " is not of a placket release")
			name = u; sub(/.*\//, "", name)
			if (!(name in sum)) fail("url of " p " names " name ", no archive of this release")
			if (name !~ ("_" p "\\.tar\\.gz$")) fail("url of " p " names " name)
			if (s != sum[name]) fail("sha256 of " name " is not its line in checksums.txt")
			if (p in seen) fail("two urls for " p)
			seen[p] = 1; urls++; s = ""
		}
		line == "binary \"placket\"" { binary = 1 }
		mac && line == "run \"/usr/bin/xattr\", args: [\"-dr\", \"com.apple.quarantine\", \"{{staged_path}}/placket\"]" { cleared = 1 }
		END {
			if (failed) exit
			if (urls != 4) { print urls + 0 " archives, not 4"; exit }
			if (!binary) { print "no binary placket"; exit }
			if (!cleared) print "no step clears the quarantine mark of placket on macOS"
		}
	' "$dir/$cask")
	[ -z "$why" ] || refuse "$cask" "$why"
	say "cask: ok, 4 archives"
fi

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
