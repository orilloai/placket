#!/bin/sh
# install.sh installs the placket of a release for this machine, as docs/release.md says: it takes
# checksums.txt and its signature, then this platform's archive, checks the archive against its line,
# checks the signature with cosign and the provenance with gh when each is there to run, and says which
# it ran and which it skipped; a check that runs and fails installs nothing; it refuses a platform the
# release has no build for, by name, before it downloads anything.
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

# install <name> <path> [<args>...]: runs install.sh, read from standard input, with the PATH <path>
# and the arguments, in a clean HOME, or the last run's when keep is set. PLACKET_RELEASE_BASE is the
# release, or empty when nobase is set, or relbase when that is set; PLACKET_INSTALL_DIR is dest; the stand-in gh's GH_AUTH_EXIT
# and GH_EXIT are gh_auth and gh_attest. Its output, standard output and standard error together,
# goes to $out, and its exit status to $rc.
n=0 keep= nobase= relbase= dest= gh_auth=0 gh_attest=0
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
		GH_AUTH_EXIT="$gh_auth" GH_EXIT="$gh_attest" "$shell" -s -- "$@" <"$root/install.sh") >"$out" 2>&1
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

# U1: the host's archive, with no cosign and no gh, into ~/.local/bin, which is not on PATH.
install "a release installs into ~/.local/bin, saying it skipped the signature and the provenance" "$tools"
bin=$home/.local/bin
{
	echo "install: placket for $host, from file://$rel"
	echo "install: signature: skipped, cosign is not on PATH"
	echo "install: checksum: ok, $archive"
	echo "install: provenance: skipped, gh is not on PATH"
	echo "install: installed $bin/placket"
	echo "install: $bin is not on PATH; add it, as in: export PATH=\"$bin:\$PATH\""
	cat "$tmp/version"
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

# U1, with cosign and a signed-in gh, a pinned version and an install directory on PATH: both checks
# run, as placket's release workflow on main signed, and PLACKET_RELEASE_BASE replaces the tag's URL.
mkdir "$tmp/dest"
dest=$tmp/dest
install "a release with cosign and gh checks the signature, then the provenance" "$signers:$tmp/dest:$tools" --version v0.1.0
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
n=$((n + 1))
home=$tmp/home$n
mkdir -p "$home/.local/bin"
printf 'old\n' >"$home/.local/bin/placket"
keep=1
install "an install over an earlier one replaces it" "$tools"
keep=
if [ "$rc" -ne 0 ] || ! cmp -s "$home/.local/bin/placket" "$tmp/files/placket"; then
	fail "want exit 0 and the tree's placket in ~/.local/bin, got exit $rc"
elif [ "$(cd "$home/.local/bin" && ls -A)" != placket ]; then
	fail "it left a file beside placket"
else
	ok
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

# A checksums.txt with no line for this platform.
grep -v "_$host.tar.gz\$" "$rel/checksums.txt" >"$tmp/sums"
mv "$tmp/sums" "$rel/checksums.txt"
install "a checksums.txt with no line for this platform installs nothing" "$tools"
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
	install "with no PLACKET_RELEASE_BASE, the $v release is taken from GitHub over https" "$tools" "$@"
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
install "a file:// PLACKET_RELEASE_BASE limits each download to file://" "$tools"
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
(cd "$tmp" && env HOME="$tmp/mkdest" PATH="$tmp/mk:$tools" PLACKET_RELEASE_BASE="file://$rel" PLACKET_INSTALL_DIR="$tmp/mkdest" \
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
	(cd "$tmp" && env HOME="$tmp/cutdest" PATH="$tools" PLACKET_RELEASE_BASE="file://$rel" \
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

exit $failed
