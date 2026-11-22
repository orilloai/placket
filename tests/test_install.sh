#!/bin/sh
# install.sh installs the placket of a release for this machine, as docs/release.md says: it takes
# checksums.txt and its signature, then this platform's archive, checks the signature with cosign,
# the archive against its line, and the provenance with gh when gh is there and signed in, and says
# which it ran and which it skipped; with no cosign it installs nothing unless
# PLACKET_INSTALL_UNSIGNED=1 says to, and then says the origin was not checked; a check that runs and
# fails installs nothing; the archive and the binary in it are the version --version names, and the
# binary's own version is the archive's before it replaces the installed one; it refuses a platform
# the release has no build for, by name, before it downloads anything. The README's and
# docs/release.md's block that checks install.sh before it runs it stops at the first step that fails.
#
# The release is a directory built at run time from the tree, served as file:// through
# PLACKET_RELEASE_BASE: this host's archive holds the tree's placket, built as .goreleaser.yaml builds
# it, and the other three a stub. Each run has a clean HOME, and a PATH of only the utilities
# install.sh uses, linked into a directory of their own, so that no cosign, gh or placket of the host
# is seen. curl is a stand-in that logs each call and refuses any https URL, so no run reaches the
# network; cosign, gh and uname are stand-ins where a case puts them first. Every run is under dash
# where the host has it, else sh, with install.sh read from standard input as `curl | sh` runs it.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
out=$tmp/out
shell=$(command -v dash || command -v sh)

case $(uname -s) in Linux) os=linux ;; Darwin) os=darwin ;; *) os=$(uname -s) ;; esac
case $(uname -m) in x86_64 | amd64) arch=amd64 ;; aarch64 | arm64) arch=arm64 ;; *) arch=$(uname -m) ;; esac
host=${os}_$arch

# The tree's placket, built as .goreleaser.yaml builds it.
mkdir "$tmp/files"
(cd "$root" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$tmp/files/placket" ./cmd/placket) ||
	{ echo "FAIL the tree's placket does not build"; exit 1; }
cp "$root/LICENSE" "$root/NOTICE" "$root/README.md" "$tmp/files/"
"$tmp/files/placket" version >"$tmp/version" || { echo "FAIL the tree's placket does not run version"; exit 1; }
version=$(sed -n 's/^version //p' "$tmp/version")
name=${version#v}
archive=placket_${name}_$host.tar.gz

# The release: four archives, checksums.txt and install.sh, and a bundle that the stand-in cosign
# takes for checksums.txt's signature: the SHA-256 of the file it signs.
rel=$tmp/release
mkdir "$rel" "$tmp/stub"
printf '#!/bin/sh\necho stub\n' >"$tmp/stub/placket"
chmod 755 "$tmp/stub/placket"
cp "$tmp/files/LICENSE" "$tmp/files/NOTICE" "$tmp/files/README.md" "$tmp/stub/"
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
	d=$tmp/stub
	[ "$p" = "$host" ] && d=$tmp/files
	(cd "$d" && tar -czf "$rel/placket_${name}_$p.tar.gz" LICENSE NOTICE README.md placket)
done
cp "$root/install.sh" "$rel/install.sh"
(cd "$rel" && sha256sum ./*.tar.gz install.sh 2>/dev/null || shasum -a 256 ./*.tar.gz install.sh) |
	sed 's| \./| |' >"$rel/checksums.txt"
digest() { { sha256sum "$1" 2>/dev/null || shasum -a 256 "$1"; } | cut -d' ' -f1; }
digest "$rel/checksums.txt" >"$rel/checksums.txt.sigstore.json"
cp -R "$rel" "$tmp/pristine"
# reset: the release as it was built.
reset() {
	rm -rf "$rel"
	cp -R "$tmp/pristine" "$rel"
}

# The utilities install.sh uses, and its stand-ins use, each linked from the host's PATH, and curl,
# which logs each call, refuses https and runs the host's curl for anything else.
tools=$tmp/tools
mkdir "$tools"
for u in uname expr mktemp rm sed grep cut tar gzip mkdir cp chmod mv cat basename sha256sum shasum; do
	c=$(command -v "$u") || continue
	ln -s "$c" "$tools/$u"
done
cat >"$tools/curl" <<EOF
#!/bin/sh
echo "curl \$*" >>"$tmp/curl.log"
for a; do case \$a in https://*) exit 22 ;; esac; done
exec "$(command -v curl)" "\$@"
EOF
chmod 755 "$tools/curl"

# The stand-ins of cosign and gh, in a directory of their own. Each logs its call, naming a file by
# its base name. cosign verifies a bundle that holds the SHA-256 of the file it names last; gh's
# `auth status` exits GH_AUTH_EXIT and its `attestation verify` GH_EXIT, 0 when unset.
signers=$tmp/signers
mkdir "$signers"
cat >"$signers/cosign" <<EOF
#!/bin/sh
log=cosign bundle= blob=
while [ \$# -gt 0 ]; do
	a=\$1
	[ "\$a" = --bundle ] && bundle=\$2
	if [ -f "\$a" ]; then blob=\$a a=\$(basename "\$a"); fi
	log="\$log \$a"
	shift
done
echo "\$log" >>"$tmp/calls"
sum=\$( { sha256sum "\$blob" 2>/dev/null || shasum -a 256 "\$blob"; } | cut -d' ' -f1)
[ "\$(cat "\$bundle")" = "\$sum" ]
EOF
cat >"$signers/gh" <<EOF
#!/bin/sh
log=gh
for a; do
	[ -f "\$a" ] && a=\$(basename "\$a")
	log="\$log \$a"
done
echo "\$log" >>"$tmp/calls"
case \$1 in
auth) exit "\${GH_AUTH_EXIT:-0}" ;;
attestation) exit "\${GH_EXIT:-0}" ;;
esac
exit 9
EOF
chmod 755 "$signers/cosign" "$signers/gh"
# gh alone, with no cosign beside it.
mkdir "$tmp/gh"
ln -s "$signers/gh" "$tmp/gh/gh"

# install <name> <path> [<args>...]: runs install.sh, read from standard input, with the PATH <path>
# and the arguments, in a clean HOME, or the last run's when keep is set. PLACKET_RELEASE_BASE is the
# release, or empty when nobase is set, or relbase when that is set; PLACKET_INSTALL_DIR is dest;
# PLACKET_INSTALL_UNSIGNED is unsigned; the stand-in gh's GH_AUTH_EXIT and GH_EXIT are gh_auth and
# gh_attest. Its output, standard output and standard error together, goes to $out, and its exit
# status to $rc.
n=0 keep= nobase= relbase= dest= unsigned= gh_auth=0 gh_attest=0
install() {
	current=$1 path=$2
	shift 2
	if [ -z "$keep" ]; then
		n=$((n + 1))
		home=$tmp/home$n
		mkdir "$home"
	fi
	base=file://$rel
	[ -z "$nobase" ] || base=
	[ -z "$relbase" ] || base=$relbase
	rm -f "$tmp/calls" "$tmp/curl.log"
	(cd "$home" && env HOME="$home" PATH="$path" PLACKET_RELEASE_BASE="$base" PLACKET_INSTALL_DIR="$dest" \
		PLACKET_INSTALL_UNSIGNED="$unsigned" GH_AUTH_EXIT="$gh_auth" GH_EXIT="$gh_attest" \
		"$shell" -s -- "$@" <"$root/install.sh") >"$out" 2>&1
	rc=$?
}

fail() {
	echo "FAIL $current: $*"
	sed 's/^/    | /' "$out"
	failed=1
}

# expect <exit> <file>: the run exited <exit> and printed exactly <file>.
expect() {
	if [ "$rc" -ne "$1" ]; then
		fail "want exit $1, got $rc"
		return 1
	fi
	if ! cmp -s "$out" "$2"; then
		fail "it printed other than this"
		diff "$2" "$out" | sed 's/^/    > /'
		return 1
	fi
	return 0
}

# refused <exit> <last line>: the run exited <exit>, its last line is exactly <last line>, and HOME
# holds no placket.
refused() {
	if [ "$rc" -ne "$1" ] || [ "$(tail -n 1 "$out")" != "$2" ]; then
		fail "want exit $1 and last line '$2', got exit $rc"
		return 1
	fi
	if [ -e "$home/.local/bin/placket" ]; then
		fail "it installed placket"
		return 1
	fi
	return 0
}

# calls <file>: the stand-ins of cosign and gh were called exactly as <file> says.
calls() {
	if ! cmp -s "${tmp}/calls" "$1"; then
		[ -e "$tmp/calls" ] || : >"$tmp/calls"
		fail "cosign and gh were not called as placket's release workflow on main signed"
		diff "$1" "$tmp/calls" | sed 's/^/    > /'
		return 1
	fi
	return 0
}

ok() { echo "ok   $current"; }

# plant: a clean HOME whose ~/.local/bin holds an earlier placket, which the next install, with keep
# set, runs in.
plant() {
	n=$((n + 1))
	home=$tmp/home$n
	mkdir -p "$home/.local/bin"
	printf 'old\n' >"$home/.local/bin/placket"
	keep=1
}

# untouched: ~/.local/bin holds the earlier placket as it was, and nothing beside it.
untouched() {
	if [ "$(cat "$home/.local/bin/placket")" != old ]; then
		fail "it replaced the installed placket"
		return 1
	elif [ "$(cd "$home/.local/bin" && ls -A)" != placket ]; then
		fail "it left a file beside the installed placket"
		ls -A "$home/.local/bin" | sed 's/^/    > /'
		return 1
	fi
	return 0
}

# With no cosign, and nothing that says to go on without it, it installs nothing, before any
# download, and names cosign and the one variable that goes on without the check.
plant
install "with no cosign, it installs nothing and names PLACKET_INSTALL_UNSIGNED" "$tools"
keep=
if [ "$rc" -ne 1 ] || [ "$(tail -n 1 "$out")" != "install: refused: cosign: not on PATH, so the signature on checksums.txt cannot be checked, and nothing is installed; install cosign, or set PLACKET_INSTALL_UNSIGNED=1 to install without that check" ]; then
	fail "want exit 1 and the refusal that names cosign and PLACKET_INSTALL_UNSIGNED=1, got exit $rc"
elif untouched; then
	if [ -s "$tmp/curl.log" ]; then
		fail "it downloaded"
	else
		ok
	fi
fi

# A PLACKET_INSTALL_UNSIGNED of another value than 1 does not go on.
unsigned=yes
install "PLACKET_INSTALL_UNSIGNED=yes is not PLACKET_INSTALL_UNSIGNED=1, and installs nothing" "$tools"
unsigned=
refused 1 "install: refused: cosign: not on PATH, so the signature on checksums.txt cannot be checked, and nothing is installed; install cosign, or set PLACKET_INSTALL_UNSIGNED=1 to install without that check" && ok

# With PLACKET_INSTALL_UNSIGNED=1, the host's archive, with no cosign and no gh, into ~/.local/bin,
# which is not on PATH: it says it skipped the signature and the provenance, and ends saying the
# binary's origin was not checked.
unsigned=1
install "with PLACKET_INSTALL_UNSIGNED=1 and no cosign, it installs and says the origin was not checked" "$tools"
unsigned=
bin=$home/.local/bin
{
	echo "install: placket for $host, from file://$rel"
	echo "install: signature: skipped, cosign is not on PATH and PLACKET_INSTALL_UNSIGNED=1 is set"
	echo "install: checksum: ok, $archive"
	echo "install: provenance: skipped, gh is not on PATH"
	echo "install: installed $bin/placket"
	echo "install: $bin is not on PATH; add it, as in: export PATH=\"$bin:\$PATH\""
	cat "$tmp/version"
	echo "install: origin: not checked, so nothing shows that $bin/placket came from placket's release workflow; install cosign and run install.sh again to check it"
} >"$tmp/want"
if expect 0 "$tmp/want"; then
	if ! cmp -s "$bin/placket" "$tmp/files/placket"; then
		fail "$bin/placket is not the tree's placket"
	elif [ "$("$bin/placket" version | sed -n 1p)" != "version $version" ]; then
		fail "$bin/placket version does not print the tree's version, $version"
	else
		ok
	fi
fi

# With PLACKET_INSTALL_UNSIGNED=1, no cosign and a signed-in gh, the provenance ties the archive to
# placket's release workflow, so it does not say the origin was not checked.
unsigned=1
install "with PLACKET_INSTALL_UNSIGNED=1 and a gh that verifies the provenance, the origin was checked" "$tmp/gh:$tools"
unsigned=
bin=$home/.local/bin
{
	echo "install: placket for $host, from file://$rel"
	echo "install: signature: skipped, cosign is not on PATH and PLACKET_INSTALL_UNSIGNED=1 is set"
	echo "install: checksum: ok, $archive"
	echo "install: provenance: ok, gh verified $archive as built by placket's release workflow on main"
	echo "install: installed $bin/placket"
	echo "install: $bin is not on PATH; add it, as in: export PATH=\"$bin:\$PATH\""
	cat "$tmp/version"
} >"$tmp/want"
expect 0 "$tmp/want" && ok

# A cosign that runs and does not verify refuses, PLACKET_INSTALL_UNSIGNED=1 or not.
digest "$rel/install.sh" >"$rel/checksums.txt.sigstore.json"
for u in '' 1; do
	unsigned=$u
	plant
	install "a signature that does not verify installs nothing, with PLACKET_INSTALL_UNSIGNED='$u'" "$signers:$tools"
	keep= unsigned=
	if [ "$rc" -ne 1 ] || [ "$(tail -n 1 "$out")" != "install: refused: checksums.txt.sigstore.json: cosign did not verify the signature" ]; then
		fail "want exit 1 and the refusal of the signature, got exit $rc"
	else
		untouched && ok
	fi
done
reset

# With cosign and a signed-in gh and an install directory on PATH: both checks run, as placket's
# release workflow on main signed.
mkdir "$tmp/dest"
dest=$tmp/dest
install "a release with cosign and gh checks the signature, then the provenance" "$signers:$tmp/dest:$tools"
dest=
{
	echo "install: placket for $host, from file://$rel"
	echo "install: signature: ok, cosign verified checksums.txt as signed by placket's release workflow on main"
	echo "install: checksum: ok, $archive"
	echo "install: provenance: ok, gh verified $archive as built by placket's release workflow on main"
	echo "install: installed $tmp/dest/placket"
	cat "$tmp/version"
} >"$tmp/want"
{
	echo "cosign verify-blob --bundle checksums.txt.sigstore.json" \
		"--certificate-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main" \
		"--certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt"
	echo "gh auth status"
	echo "gh attestation verify $archive --repo orilloai/placket" \
		"--cert-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main" \
		"--cert-oidc-issuer https://token.actions.githubusercontent.com" \
		"--source-ref refs/heads/main --deny-self-hosted-runners"
} >"$tmp/want-calls"
if expect 0 "$tmp/want" && calls "$tmp/want-calls"; then
	if ! cmp -s "$tmp/dest/placket" "$tmp/files/placket"; then
		fail "$tmp/dest/placket is not the tree's placket"
	elif [ -e "$home/.local/bin" ]; then
		fail "it wrote into ~/.local/bin with PLACKET_INSTALL_DIR set"
	else
		ok
	fi
fi

# A second install over an earlier one replaces it.
plant
install "an install over an earlier one replaces it" "$signers:$tools"
keep=
if [ "$rc" -ne 0 ] || ! cmp -s "$home/.local/bin/placket" "$tmp/files/placket"; then
	fail "want exit 0 and the tree's placket in ~/.local/bin, got exit $rc"
elif [ "$(cd "$home/.local/bin" && ls -A)" != placket ]; then
	fail "it left a file beside placket"
else
	ok
fi

# The version: a release of stand-ins, each archive named for <name> and its placket printing
# `version <printed>`, signed as the tree's release is. mkrel <dir> <name> <printed>.
mkrel() {
	mkdir "$1" "$1.files"
	printf '#!/bin/sh\necho "version %s"\necho "commit 0123456789abcdef0123456789abcdef01234567"\n' "$3" >"$1.files/placket"
	chmod 755 "$1.files/placket"
	for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
		(cd "$1.files" && tar -czf "$1/placket_${2}_$p.tar.gz" placket)
	done
	(cd "$1" && sha256sum ./*.tar.gz 2>/dev/null || shasum -a 256 ./*.tar.gz) | sed 's| \./| |' >"$1/checksums.txt"
	digest "$1/checksums.txt" >"$1/checksums.txt.sigstore.json"
}
mkrel "$tmp/r999" 9.9.9 v9.9.9
mkrel "$tmp/r010" 0.1.0 v0.1.0
mkrel "$tmp/r010x" 0.1.0 v0.2.0

# --version v0.1.0, with PLACKET_RELEASE_BASE a release of another version: the tree's, and 9.9.9.
for r in "$rel" "$tmp/r999"; do
	relbase=file://$r
	plant
	install "--version v0.1.0 from a PLACKET_RELEASE_BASE of ${r##*/}, which holds no v0.1.0, installs nothing" "$signers:$tools" --version v0.1.0
	keep= relbase=
	if [ "$rc" -ne 1 ] || [ "$(tail -n 1 "$out")" != "install: refused: checksums.txt: 0 lines for placket_0.1.0_$host.tar.gz, not 1" ]; then
		fail "want exit 1 and the refusal that names placket_0.1.0_$host.tar.gz, got exit $rc"
	elif untouched; then
		if grep -q '\.tar\.gz$' "$tmp/curl.log"; then
			fail "it downloaded an archive"
		else
			ok
		fi
	fi
done

# --version v0.1.0 from a release of v0.1.0 installs it, and says which version it is.
relbase=file://$tmp/r010
install "--version v0.1.0 from a PLACKET_RELEASE_BASE of v0.1.0 installs v0.1.0" "$signers:$tools" --version v0.1.0
relbase=
bin=$home/.local/bin
{
	echo "install: placket for $host, from file://$tmp/r010"
	echo "install: signature: ok, cosign verified checksums.txt as signed by placket's release workflow on main"
	echo "install: checksum: ok, placket_0.1.0_$host.tar.gz"
	echo "install: provenance: ok, gh verified placket_0.1.0_$host.tar.gz as built by placket's release workflow on main"
	echo "install: installed $bin/placket"
	echo "install: $bin is not on PATH; add it, as in: export PATH=\"$bin:\$PATH\""
	echo "version v0.1.0"
	echo "commit 0123456789abcdef0123456789abcdef01234567"
} >"$tmp/want"
if expect 0 "$tmp/want"; then
	if ! cmp -s "$bin/placket" "$tmp/r010.files/placket"; then
		fail "$bin/placket is not the v0.1.0 placket"
	else
		ok
	fi
fi

# An archive named for 0.1.0 whose placket says another version is refused before the rename,
# pinned or not, and the installed placket is left as it was.
for v in v0.1.0 ''; do
	set --
	[ -z "$v" ] || set -- --version "$v"
	relbase=file://$tmp/r010x
	plant
	install "an archive named for 0.1.0 whose placket says v0.2.0 installs nothing${v:+, with --version $v}" "$signers:$tools" "$@"
	keep= relbase=
	if [ "$rc" -ne 1 ] || [ "$(tail -n 1 "$out")" != "install: refused: placket_0.1.0_$host.tar.gz: its placket says it is version v0.2.0, not v0.1.0" ]; then
		fail "want exit 1 and the refusal of the version, got exit $rc"
	else
		untouched && ok
	fi
done
set --

# An empty --version is refused, not taken for the latest.
install "an empty --version is refused" "$signers:$tools" --version ''
if refused 2 "install: usage: install.sh [--version vX.Y.Z]"; then
	if [ -s "$tmp/curl.log" ]; then
		fail "it downloaded"
	else
		ok
	fi
fi

# U2: one byte of the host's archive changed: byte 20 with its lowest bit flipped, so it differs
# whatever the archive held there.
b=$(dd if="$rel/$archive" bs=1 skip=20 count=1 2>/dev/null | od -An -tu1 | tr -d ' ')
# shellcheck disable=SC2059 # the format is the octal escape of the flipped byte
printf "\\$(printf '%03o' $((b ^ 1)))" | dd of="$rel/$archive" bs=1 seek=20 conv=notrunc 2>/dev/null
install "an archive changed by one byte installs nothing" "$signers:$tools"
refused 1 "install: refused: $archive: its SHA-256 is not its line in checksums.txt" && ok
reset

# U2b: the bundle of another file, which the stand-in cosign does not verify. It stops before it
# downloads the archive.
digest "$rel/install.sh" >"$rel/checksums.txt.sigstore.json"
install "a signature that does not verify installs nothing" "$signers:$tools"
if refused 1 "install: refused: checksums.txt.sigstore.json: cosign did not verify the signature"; then
	if grep -q "$archive" "$tmp/curl.log"; then
		fail "it downloaded the archive before the signature held"
	else
		ok
	fi
fi
reset

# U2b: a gh that is not signed in skips the provenance, and says so.
gh_auth=1
install "a gh that is not signed in skips the provenance" "$signers:$tools"
gh_auth=0
if [ "$rc" -ne 0 ] || [ "$(sed -n 4p "$out")" != "install: provenance: skipped, gh is not signed in (gh auth status failed)" ]; then
	fail "want exit 0 and the provenance skipped as gh is not signed in, got exit $rc"
elif grep -q '^gh attestation' "$tmp/calls"; then
	fail "it ran gh attestation verify"
else
	ok
fi

# Provenance that gh does not verify.
gh_attest=1
install "provenance that does not verify installs nothing" "$signers:$tools"
gh_attest=0
refused 1 "install: refused: $archive: gh did not verify its provenance" && ok

# A checksums.txt with no line for this platform, which no signature covers, so the stand-in cosign
# would refuse it first.
grep -v "_$host.tar.gz\$" "$rel/checksums.txt" >"$tmp/sums"
mv "$tmp/sums" "$rel/checksums.txt"
unsigned=1
install "a checksums.txt with no line for this platform installs nothing" "$tools"
unsigned=
refused 1 "install: refused: checksums.txt: 0 lines for $host, not 1" && ok
reset

# U3: a platform the release has no build for, named, before any download.
mkdir "$tmp/windows" "$tmp/riscv"
printf '#!/bin/sh\ncase $1 in -s) echo windows ;; -m) echo x86_64 ;; esac\n' >"$tmp/windows/uname"
printf '#!/bin/sh\ncase $1 in -s) echo Linux ;; -m) echo riscv64 ;; esac\n' >"$tmp/riscv/uname"
chmod 755 "$tmp/windows/uname" "$tmp/riscv/uname"
for p in windows/x86_64:windows Linux/riscv64:riscv; do
	install "the platform ${p%%:*} is refused by name, and nothing is downloaded" "$tmp/${p#*:}:$tools"
	if refused 1 "install: refused: ${p%%:*}: no placket release for this platform; there is one for Linux and macOS, on amd64 and arm64"; then
		if [ -s "$tmp/curl.log" ]; then
			fail "it downloaded"
		else
			ok
		fi
	fi
done

# The release's URL: the latest, or the tag --version names, over https only. The stand-in curl
# refuses https, so each run stops at its first download, and the log shows its URL.
for v in latest v0.1.0; do
	if [ "$v" = latest ]; then
		set --
		url=https://github.com/orilloai/placket/releases/latest/download
	else
		set -- --version "$v"
		url=https://github.com/orilloai/placket/releases/download/$v
	fi
	nobase=1
	install "with no PLACKET_RELEASE_BASE, the $v release is taken from GitHub over https" "$signers:$tools" "$@"
	nobase=
	if refused 1 "install: refused: checksums.txt: could not download it from $url"; then
		case $(cat "$tmp/curl.log") in
		"curl -fsSL --proto =https --proto-redir =https -o "*"/checksums.txt $url/checksums.txt") ok ;;
		*)
			fail "its one download is not $url/checksums.txt, over https only"
			sed 's/^/    > /' "$tmp/curl.log"
			;;
		esac
	fi
done

# A --version that is not a release tag.
install "a --version that is not vX.Y.Z is refused" "$tools" --version 0.1
refused 2 "install: usage: install.sh [--version vX.Y.Z]" && ok

# A file:// release is downloaded over file:// alone: every curl call of the first install's kind
# carries --proto =file.
install "a file:// PLACKET_RELEASE_BASE limits each download to file://" "$signers:$tools"
if [ "$rc" -ne 0 ] || [ ! -s "$tmp/curl.log" ]; then
	fail "want exit 0 and a download, got exit $rc"
elif grep -v -e '--proto =file --proto-redir =file ' "$tmp/curl.log" >"$tmp/loose"; then
	fail "a download is not limited to file://"
	sed 's/^/    > /' "$tmp/loose"
else
	ok
fi

# Any other PLACKET_RELEASE_BASE is refused before any download.
for b in http://example.invalid/r ftp://example.invalid/r /tmp/release; do
	relbase=$b
	install "PLACKET_RELEASE_BASE=$b is refused, and nothing is downloaded" "$tools"
	relbase=
	if [ "$rc" -ne 1 ] || ! tail -n 1 "$out" | grep -q '^install: refused: PLACKET_RELEASE_BASE: '; then
		fail "want exit 1 and a last line starting 'install: refused: PLACKET_RELEASE_BASE: ', got exit $rc"
	elif [ -s "$tmp/curl.log" ]; then
		fail "it downloaded"
	elif [ -e "$home/.local/bin/placket" ]; then
		fail "it installed placket"
	else
		ok
	fi
done

# The file installed is written first under a name mktemp makes in the install directory, so the
# rename stays on one file system and no name comes from the process id: a stand-in mktemp, first on
# PATH, logs each call, and a file already named .placket.install.<the install's pid> is left as it
# was. The shell that plants it then runs install.sh in its own process, by exec.
mkdir "$tmp/mk" "$tmp/mkdest"
cat >"$tmp/mk/mktemp" <<EOF
#!/bin/sh
echo "mktemp \$*" >>"$tmp/mktemp.log"
exec "$(command -v mktemp)" "\$@"
EOF
chmod 755 "$tmp/mk/mktemp"
current="the file installed is first written under a name mktemp makes in the install directory"
(cd "$tmp" && env HOME="$tmp/mkdest" PATH="$tmp/mk:$signers:$tools" PLACKET_RELEASE_BASE="file://$rel" PLACKET_INSTALL_DIR="$tmp/mkdest" \
	"$shell" -c 'echo planted >"$PLACKET_INSTALL_DIR/.placket.install.$$" && exec "$0" -s' "$shell" <"$root/install.sh") >"$out" 2>&1
rc=$?
planted=$(cd "$tmp/mkdest" && ls -A | grep '^\.placket\.install\.')
if [ "$rc" -ne 0 ] || ! cmp -s "$tmp/mkdest/placket" "$tmp/files/placket"; then
	fail "want exit 0 and the tree's placket in the install directory, got exit $rc"
elif ! grep -q "^mktemp $tmp/mkdest/[^ /]*XXXXXX\$" "$tmp/mktemp.log"; then
	fail "mktemp made no file in the install directory"
	sed 's/^/    > /' "$tmp/mktemp.log"
elif [ "$(echo "$planted" | wc -l | tr -d ' ')" != 1 ] || [ "$(cat "$tmp/mkdest/$planted")" != planted ]; then
	fail "the file named for the install's process id was not left as it was"
	ls -A "$tmp/mkdest" | sed 's/^/    > /'
elif [ "$(cd "$tmp/mkdest" && ls -A | grep -vx placket | grep -vx "$planted")" != "" ]; then
	fail "it left a file beside placket"
	ls -A "$tmp/mkdest" | sed 's/^/    > /'
else
	ok
fi

# install.sh cut short runs nothing: curl is never called and nothing is installed, as when a
# `curl | sh` download stops part way. It is cut after each of its lines but the last, and after each
# byte from the line that closes main() on, but the one that drops only the final newline, since a cut inside the call that ends the file is where a
# prefix could still run as a command of its own. A cut inside main()'s body leaves it unclosed, which
# sh refuses before it runs a line.
current="install.sh cut after any line but its last, or any byte of its last lines, runs nothing"
size=$(wc -c <"$root/install.sh" | tr -d ' ')
close=$(grep -n '^}$' "$root/install.sh" | tail -n 1 | cut -d: -f1)
from=$(head -n "$((close - 1))" "$root/install.sh" | wc -c | tr -d ' ')
mkdir "$tmp/cutdest"
ran=
run_cut() { # run_cut LABEL: pipes $tmp/cut.sh, a cut of install.sh, to sh
	rm -f "$tmp/curl.log"
	(cd "$tmp" && env HOME="$tmp/cutdest" PATH="$signers:$tools" PLACKET_RELEASE_BASE="file://$rel" \
		PLACKET_INSTALL_DIR="$tmp/cutdest" "$shell" -s) <"$tmp/cut.sh" >/dev/null 2>&1
	if [ -s "$tmp/curl.log" ] || [ -n "$(ls -A "$tmp/cutdest")" ]; then
		ran="$ran $1"
		rm -rf "$tmp/cutdest" && mkdir "$tmp/cutdest"
	fi
}
lines=$(wc -l <"$root/install.sh" | tr -d ' ')
i=1
while [ "$i" -lt "$lines" ]; do
	head -n "$i" "$root/install.sh" >"$tmp/cut.sh" && run_cut "line-$i"
	i=$((i + 1))
done
i=$from
while [ "$i" -lt "$((size - 1))" ]; do
	head -c "$i" "$root/install.sh" >"$tmp/cut.sh" && run_cut "byte-$i"
	i=$((i + 1))
done
: >"$out"
if [ "$lines" -lt 2 ] || [ -z "$close" ] || [ -n "$(tail -c 1 "$root/install.sh")" ]; then
	fail "install.sh has $lines lines, its last closing brace is at line ${close:-none}, or it does not end in a newline"
elif [ -n "$ran" ]; then
	fail "cut after$ran, it downloaded or installed"
else
	ok
fi

# The block of README.md and docs/release.md that checks install.sh before it runs it stops at the
# first step that fails, in a directory of its own: it is run as printed, with stand-ins of curl,
# cosign and shasum first on PATH. curl copies each release file a URL names from a directory of
# fixtures, cosign exits COSIGN_EXIT, and shasum checks as the host's does; the install.sh it takes
# only notes that it ran. It runs with cosign failing, with no cosign, and with a checksums.txt whose
# line for install.sh does not match; then with every step passing.
# block <file>: the one code block of <file> that holds both cosign verify-blob and the line
# `sh install.sh`, with its fence's indent taken off each line.
block() {
	awk '
		!fence && match($0, /^ *```/) { ind = RLENGTH - 3; fence = 1; b = ""; next }
		fence && /^ *```$/ { fence = 0; if (index(b, "cosign verify-blob") && index("\n" b, "\nsh install.sh\n")) { n++; got = b }; next }
		fence { b = b substr($0, ind + 1) "\n" }
		END { if (n != 1) exit 1; printf "%s", got }
	' "$1"
}
bk=$tmp/block
mkdir "$bk" "$bk/files" "$bk/tools" "$bk/cosign"
printf 'echo "install.sh ran in $(pwd)" >>"%s"\n' "$bk/ran" >"$bk/files/install.sh"
echo bundle >"$bk/files/checksums.txt.sigstore.json"
cat >"$bk/tools/curl" <<EOF
#!/bin/sh
for a; do
	case \$a in https://*) cp "$bk/files/\${a##*/}" "./\${a##*/}" || exit 22 ;; esac
done
EOF
printf '#!/bin/sh\nexit "${COSIGN_EXIT:-0}"\n' >"$bk/cosign/cosign"
if command -v shasum >/dev/null; then
	printf '#!/bin/sh\nexec "%s" "$@"\n' "$(command -v shasum)" >"$bk/tools/shasum"
else
	printf '#!/bin/sh\n[ "$1 $2" = "-a 256" ] && shift 2\nexec "%s" "$@"\n' "$(command -v sha256sum)" >"$bk/tools/shasum"
fi
chmod 755 "$bk/tools/curl" "$bk/tools/shasum" "$bk/cosign/cosign"
for u in sh mkdir mktemp grep cat cp pwd; do
	c=$(command -v "$u") || continue
	[ -e "$bk/tools/$u" ] || ln -s "$c" "$bk/tools/$u"
done
good="$(digest "$bk/files/install.sh")  install.sh"
bad="$(printf '%064d' 0)  install.sh"
for f in README.md docs/release.md; do
	if ! block "$root/$f" >"$bk/block.sh"; then
		current="$f has one block that checks install.sh before it runs it"
		: >"$out"
		fail "it has none, or more than one"
		continue
	fi
	for c in "cosign-fails:1:$good" "no-cosign:0:$good" "checksum-differs:0:$bad" "all-pass:0:$good"; do
		what=${c%%:*} rest=${c#*:}
		cexit=${rest%%:*} sums=${rest#*:}
		path=$bk/cosign:$bk/tools
		[ "$what" != no-cosign ] || path=$bk/tools
		printf '%s\n' "$sums" >"$bk/files/checksums.txt"
		rm -rf "${bk:?}/ran" "${bk:?}/cwd" "${bk:?}/tmp"
		mkdir "$bk/cwd" "$bk/tmp"
		(cd "$bk/cwd" && env PATH="$path" TMPDIR="$bk/tmp" HOME="$bk/cwd" COSIGN_EXIT="$cexit" "$shell" "$bk/block.sh") >"$out" 2>&1
		rc=$?
		current="$f: the block that checks install.sh first, $what"
		if [ "$what" = all-pass ] && { [ "$rc" -ne 0 ] || [ ! -s "$bk/ran" ]; }; then
			fail "want exit 0 and install.sh run, got exit $rc"
		elif [ "$what" != all-pass ] && { [ "$rc" -eq 0 ] || [ -e "$bk/ran" ]; }; then
			fail "want a non-zero exit and install.sh not run, got exit $rc"
			[ ! -e "$bk/ran" ] || sed 's/^/    > /' "$bk/ran"
		elif [ -n "$(ls -A "$bk/cwd")" ]; then
			fail "it wrote into the directory it was run in, not one of its own"
		else
			ok
		fi
	done
done

exit $failed
