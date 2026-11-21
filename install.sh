#!/bin/sh
# install.sh: installs the placket binary of a signed release for this machine (docs/release.md).
#
#   curl -fsSL https://github.com/orilloai/placket/releases/latest/download/install.sh | sh
#   sh install.sh [--version vX.Y.Z]
#
# It takes checksums.txt and its signature from the release first, then the one archive for this
# machine, and checks the archive's SHA-256 against its line in checksums.txt. It checks the signature
# with cosign, and the archive's provenance with gh, when each is there to run: it installs neither,
# and says which checks it ran and which it skipped, and why. Any check that runs and fails stops it,
# and it writes nothing. Then it puts placket in $PLACKET_INSTALL_DIR, else ~/.local/bin, without
# sudo, and runs `placket version`.
#
# The release is the latest, or the tag --version names. PLACKET_RELEASE_BASE, when set, is the URL
# of a directory that holds the release's files in place of either, such as a file:// directory.
set -u

repo=orilloai/placket
identity=https://github.com/$repo/.github/workflows/release.yml@refs/heads/main
issuer=https://token.actions.githubusercontent.com

say() { echo "install: $1"; }
refuse() {
	[ -s "${tmp:-/nonexistent}/log" ] && sed 's/^/    | /' "$tmp/log" >&2
	echo "install: refused: $1: $2" >&2
	exit 1
}
usage() {
	echo "install: usage: install.sh [--version vX.Y.Z]" >&2
	exit 2
}

tag=
while [ $# -gt 0 ]; do
	case $1 in
	--version)
		[ $# -ge 2 ] || usage
		tag=$2
		shift 2
		;;
	*) usage ;;
	esac
done
if [ -n "$tag" ]; then
	expr "$tag" : 'v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$' >/dev/null || usage
fi

# The platform: one of the four the release builds.
kernel=$(uname -s)
machine=$(uname -m)
case $kernel in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) os= ;;
esac
case $machine in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) arch= ;;
esac
[ -n "$os" ] && [ -n "$arch" ] || refuse "$kernel/$machine" "no placket release for this platform; there is one for Linux and macOS, on amd64 and arm64"

if [ -n "${PLACKET_RELEASE_BASE:-}" ]; then
	base=${PLACKET_RELEASE_BASE%/}
	proto=
elif [ -n "$tag" ]; then
	base=https://github.com/$repo/releases/download/$tag
	proto='=https'
else
	base=https://github.com/$repo/releases/latest/download
	proto='=https'
fi

command -v curl >/dev/null || refuse curl "not installed; install.sh downloads with it"
if command -v sha256sum >/dev/null; then
	sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null; then
	sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	refuse sha256sum "not installed, nor shasum; install.sh checks the archive with one of them"
fi

tmp=$(mktemp -d) || refuse mktemp "no temporary directory"
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

# fetch NAME: the release's file NAME, into the temporary directory.
fetch() {
	if [ -n "$proto" ]; then
		curl -fsSL --proto "$proto" --proto-redir "$proto" -o "$tmp/$1" "$base/$1" 2>"$tmp/log"
	else
		curl -fsSL -o "$tmp/$1" "$base/$1" 2>"$tmp/log"
	fi || refuse "$1" "could not download it from $base"
	: >"$tmp/log"
}

say "placket for ${os}_$arch, from $base"
fetch checksums.txt
fetch checksums.txt.sigstore.json

# The signature on checksums.txt, before anything trusts it.
if command -v cosign >/dev/null; then
	cosign verify-blob --bundle "$tmp/checksums.txt.sigstore.json" \
		--certificate-identity "$identity" --certificate-oidc-issuer "$issuer" \
		"$tmp/checksums.txt" >"$tmp/log" 2>&1 || refuse checksums.txt.sigstore.json "cosign did not verify the signature"
	: >"$tmp/log"
	say "signature: ok, cosign verified checksums.txt as signed by placket's release workflow on main"
else
	say "signature: skipped, cosign is not on PATH"
fi

# The archive: the one line of checksums.txt for this platform.
lines=$(grep -c "_${os}_$arch\.tar\.gz\$" "$tmp/checksums.txt")
[ "$lines" = 1 ] || refuse checksums.txt "$lines lines for ${os}_$arch, not 1"
line=$(grep "_${os}_$arch\.tar\.gz\$" "$tmp/checksums.txt")
want=${line%% *}
archive=${line#* }
archive=${archive# }
archive=${archive#\*}
expr "$want" : '[0-9a-f]\{64\}$' >/dev/null || refuse checksums.txt "no SHA-256 on the line for ${os}_$arch"
expr "$archive" : "placket_[A-Za-z0-9.+-]*_${os}_$arch\\.tar\\.gz\$" >/dev/null ||
	refuse checksums.txt "the line for ${os}_$arch names no placket archive"
fetch "$archive"
[ "$(sha256 "$tmp/$archive")" = "$want" ] || refuse "$archive" "its SHA-256 is not its line in checksums.txt"
say "checksum: ok, $archive"

# The archive's provenance.
if ! command -v gh >/dev/null; then
	say "provenance: skipped, gh is not on PATH"
elif ! gh auth status >/dev/null 2>&1; then
	say "provenance: skipped, gh is not signed in (gh auth status failed)"
else
	gh attestation verify "$tmp/$archive" --repo "$repo" \
		--cert-identity "$identity" --cert-oidc-issuer "$issuer" \
		--source-ref refs/heads/main --deny-self-hosted-runners >"$tmp/log" 2>&1 ||
		refuse "$archive" "gh did not verify its provenance"
	: >"$tmp/log"
	say "provenance: ok, gh verified $archive as built by placket's release workflow on main"
fi

mkdir "$tmp/x"
tar -xzf "$tmp/$archive" -C "$tmp/x" placket 2>"$tmp/log" || refuse "$archive" "holds no placket"
[ -f "$tmp/x/placket" ] && [ ! -L "$tmp/x/placket" ] || refuse "$archive" "its placket is not a file"
: >"$tmp/log"

# Install: into the directory, then renamed over any placket there, so no half-written file is left.
if [ -n "${PLACKET_INSTALL_DIR:-}" ]; then
	dir=$PLACKET_INSTALL_DIR
else
	[ -n "${HOME:-}" ] || refuse HOME "not set; set PLACKET_INSTALL_DIR to the directory to install into"
	dir=$HOME/.local/bin
fi
mkdir -p "$dir" 2>"$tmp/log" || refuse "$dir" "could not make the directory"
cp "$tmp/x/placket" "$dir/.placket.install.$$" 2>"$tmp/log" || {
	rm -f "$dir/.placket.install.$$"
	refuse "$dir" "could not write placket there"
}
chmod 755 "$dir/.placket.install.$$" && mv -f "$dir/.placket.install.$$" "$dir/placket" 2>"$tmp/log" || {
	rm -f "$dir/.placket.install.$$"
	refuse "$dir" "could not write placket there"
}
say "installed $dir/placket"
case :${PATH:-}: in
*:"$dir":*) ;;
*) say "$dir is not on PATH; add it, as in: export PATH=\"$dir:\$PATH\"" ;;
esac
"$dir/placket" version
