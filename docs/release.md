# Releases

A placket release is a GitHub release of the tag `vX.Y.Z`. It holds:

- `placket` for Linux and macOS, on amd64 and arm64, one `.tar.gz` archive per platform, each holding the binary, `LICENSE`, `NOTICE` and `README.md`;
- one SBOM per archive, in SPDX JSON, `<archive>.sbom.json`;
- `install.sh`, the one-line install (below);
- `checksums.txt`, the SHA-256 of every archive and SBOM, and of `install.sh`;
- `checksums.txt.sigstore.json`, the cosign bundle that signs `checksums.txt`;
- `placket.rb`, the Homebrew cask. It is written after `checksums.txt` is signed, so it has no line there: what ties it to the signed release is that each of its SHA-256s is an archive's line in `checksums.txt`;
- build provenance for every archive and for `checksums.txt`, kept by GitHub.

Signing and provenance use the release workflow's own GitHub identity: there is no key. Anyone can check that an archive came from placket's release workflow on `main`, and rebuild the binary and its archive to the same bytes. The SBOMs carry their own creation time, so they are signed, through `checksums.txt`, but a rebuild does not reproduce them.

`placket version` says which release a binary is:

```
version v0.1.0
commit 0123456789abcdef0123456789abcdef01234567
commitTime 2026-12-01T09:30:00Z
go go1.27.4
module 0.2.0 sha256:...
```

The version is the one Go stamps from the git state of the build: the tag for a build at a tag, a pseudo-version for any other commit, and a version ending `+dirty` for a tree with changes not committed. A build with no git information, such as one from a source tarball, prints `(devel)`. `commit` and `commitTime` are printed only when the build carries them. `module` is the decision module, as `placket card --default` names it. With `--json`, it prints one object of the same members.

## For users

### Install in one line

```sh
curl -fsSL https://github.com/orilloai/placket/releases/latest/download/install.sh | sh
```

`install.sh` is a POSIX shell script. It finds this machine's platform among the four the release builds, and refuses any other by name. It downloads `checksums.txt` and its bundle first, then the one archive whose line in `checksums.txt` ends `_<os>_<arch>.tar.gz`, and checks the archive's SHA-256 against that line. Two checks run only with a tool already installed, since the script installs neither:

- with `cosign` on `PATH`, it verifies the bundle on `checksums.txt` before it reads a line of it, with the identity and issuer below;
- with `gh` on `PATH` and signed in, as `gh auth status` says, it verifies the archive's provenance.

It prints each check it ran, and each it skipped and why. A check that runs and fails, the checksum included, ends it with `install: refused: <file>: <reason>`, and it writes nothing. Then it installs `placket` in `$PLACKET_INSTALL_DIR`, else `~/.local/bin`, never with `sudo`: it writes a file `mktemp` makes in that directory and renames it over any `placket` there, so the rename stays on one file system. It says so when that directory is not on `PATH`, and runs `placket version`.

`curl | sh` runs the script before anything has checked it. To check it first, take it from the release with `checksums.txt` and its bundle, verify them, then run it:

```sh
base=https://github.com/orilloai/placket/releases/latest/download
curl -fsSLO "$base/install.sh" -O "$base/checksums.txt" -O "$base/checksums.txt.sigstore.json"
cosign verify-blob --bundle checksums.txt.sigstore.json \
  --certificate-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
grep ' install.sh$' checksums.txt | shasum -a 256 -c
sh install.sh
```

`--version vX.Y.Z` installs that release in place of the latest: `sh install.sh --version v0.1.0`, or `| sh -s -- --version v0.1.0`. `PLACKET_RELEASE_BASE`, when set, is the URL of a directory holding a release's files, which replaces either: an `https://` URL or a `file://` directory, and every download is held to that scheme; any other is refused before anything is downloaded. `tests/test_install.sh` points it at a `file://` directory. The whole script is one function, called in braces on its last line, so a download cut short anywhere, within that line too, runs nothing.

### Homebrew

On macOS or Linux:

```sh
brew install --cask orilloai/tap/placket
```

The cask installs from the release's archives at their SHA-256s in `checksums.txt`. On macOS it clears the quarantine mark of the installed `placket`, so its first run asks nothing. The tap takes the cask from each published release once a day, so it may lag a release by up to a day.

### Download

With the GitHub command line, into a new directory:

```sh
gh release download v0.1.0 --repo orilloai/placket --dir placket-v0.1.0
```

Or take every asset of the release's page into one directory.

### Check the signature and the provenance

`scripts/release-check.sh` holds the checks, and the steps below are its commands. It needs `cosign` and `gh`, signed in, and runs on the downloaded directory:

```sh
scripts/release-check.sh --release v0.1.0 placket-v0.1.0
```

It ends `release-check: ok`, or `release-check: refused: <file>: <reason>`. It checks the signature and the provenance before it opens an archive or runs the binary in it. By hand, in the directory, in this order:

1. The signature on `checksums.txt`:

   ```sh
   cosign verify-blob --bundle checksums.txt.sigstore.json \
     --certificate-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com \
     checksums.txt
   ```

2. The checksums: `shasum -a 256 -c checksums.txt`.
3. The provenance of each archive:

   ```sh
   gh attestation verify placket_0.1.0_linux_amd64.tar.gz --repo orilloai/placket \
     --cert-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main \
     --cert-oidc-issuer https://token.actions.githubusercontent.com \
     --source-ref refs/heads/main --deny-self-hosted-runners
   ```

4. The version: unpack the archive for your platform and run `./placket version`. Its first line is `version v0.1.0`.

`install.sh` is checked with the rest: `release-check.sh` checks its line in `checksums.txt`, as `shasum -a 256 -c checksums.txt` does.

### Rebuild

A rebuild needs a git clone at the tag, since Go reads the version from git; a source tarball has no git information and builds `(devel)`. It needs exactly the Go that `go.mod`'s `toolchain` line names. That line is only a minimum: a newer Go installed locally would build with itself and give other bytes. So the build sets `GOTOOLCHAIN` to that line's version, and the `go` command fetches that Go when it is not the one installed. Build to a directory outside the clone, so the tree stays clean and Go stamps no `+dirty`:

```sh
git clone --branch v0.1.0 https://github.com/orilloai/placket.git
cd placket
OUT=$(mktemp -d)
TOOLCHAIN=$(sed -n 's/^toolchain //p' go.mod)
GOTOOLCHAIN=$TOOLCHAIN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$OUT/placket" ./cmd/placket
tar -xzOf ../placket-v0.1.0/placket_0.1.0_linux_amd64.tar.gz placket | cmp - "$OUT/placket"
```

`cmp` prints nothing when the two are the same bytes. Set `GOOS` and `GOARCH` for the archive to compare against: `linux` or `darwin`, `amd64` or `arm64`. The archive itself is rebuilt by the release tooling at the tag, `GOTOOLCHAIN=$TOOLCHAIN goreleaser release --snapshot --clean --skip=sign`, with the version of GoReleaser the release workflow pins: every time inside an archive is the commit's. A snapshot's archives are named `placket_<version>-SNAPSHOT-<commit>_<os>_<arch>.tar.gz`, not `placket_<version>_<os>_<arch>.tar.gz`, so compare the hashes platform by platform, not the lines of the two `checksums.txt`:

```sh
for p in linux_amd64 linux_arm64 darwin_amd64 darwin_arm64; do
  [ "$(grep "_$p.tar.gz\$" dist/checksums.txt | cut -d' ' -f1)" = \
    "$(grep "_$p.tar.gz\$" ../placket-v0.1.0/checksums.txt | cut -d' ' -f1)" ] && echo "$p: same" || echo "$p: differs"
done
```

## For the maintainer

### Cutting a release

A release PR does two things only:

- it moves `CHANGELOG.md`'s `Unreleased` section under the new version, `## [X.Y.Z] - YYYY-MM-DD`, dated the day it merges;
- it marks the formats in `spec/frozen-formats.md` released at that version.

Approving it is the decision to release, the register's change included. Once it merges, the release workflow, `.github/workflows/release.yml`, does the rest in one run on the push to `main`:

1. `scripts/release-detect.sh` reads the push. It prints `release X.Y.Z` when the push added exactly one version section and `vX.Y.Z` is not a tag; `resume X.Y.Z` when `vX.Y.Z` is a tag at the push and its release is missing or a draft; `not a release push` otherwise, and the run ends there. It refuses two new sections, a version not above the latest tag, a version tagged at another commit, a section dated more than a day from the run, and any release while the repository is private.
2. For a release, `make ci` runs at the commit, and the workflow pushes the annotated tag `vX.Y.Z` at it.
3. GoReleaser builds the archives, the SBOMs and `checksums.txt`, signs it, and uploads them to a draft release whose notes are the version's section.
4. GitHub's build provenance is added for every archive and for `checksums.txt`.
5. The draft is published.

A run that failed is re-run from the Actions tab. It resumes at step 3: the tag is there and the release is a draft or missing, so it replaces the draft and carries on. The archives are reproducible, so signing again changes nothing they hold.

A push that release-detect refuses publishes nothing, and merging a fix to its section's date later does not help: that push adds no new section, so it is not a release push. To release the version after a refusal, for a date more than a day from the merge or a repository that was private, take it out and put it back: a pull request that moves the section's entries back under `Unreleased`, then a new release PR dated the day it merges. Or release the next version instead.

A `v*` tag pushed by hand starts nothing, and since no `v*` tag can be moved or deleted, its version number is used up: a release PR for it is refused as "a tag at another commit".

A release can be withdrawn from GitHub, but its signature in the public transparency log and its provenance records stay.

On every pull request, the workflow also builds a snapshot twice without signing, checks that the two builds' archives are the same bytes, and runs `scripts/release-check.sh --snapshot` on it, which also checks the checkout's `install.sh` against its line in `checksums.txt`, and the cask GoReleaser wrote, `dist/homebrew/Casks/placket.rb`: each URL names this release's archive for the platform of its block, each SHA-256 is that archive's line in `checksums.txt`, and its macOS step clears the quarantine mark of `placket`. It is not a required check.

### The Homebrew tap

The tap is the repository `orilloai/homebrew-tap`, which holds `Casks/placket.rb`. GoReleaser writes the cask and pushes it nowhere: the release only attaches it. Placket's own workflows hold no token for the tap. A workflow in the tap, on a schedule and by hand, takes the cask from placket's latest published release, so it only ever names published assets; checks the signature on that release's `checksums.txt` and that each of the cask's SHA-256s is an archive's line there; and commits it with the tap's own token. So the cask may lag a release by up to a day; running the workflow by hand after a release brings it at once.

The tap's workflow, `.github/workflows/update.yml`:

```yaml
name: update
on:
  schedule:
    - cron: "23 5 * * *"
  workflow_dispatch:
permissions:
  contents: write
jobs:
  update:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
      - uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2
        with:
          cosign-release: v3.1.4
      - name: Take the cask from placket's latest published release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          set -eu
          tag=$(gh release view --repo orilloai/placket --json tagName --jq .tagName)
          d=$RUNNER_TEMP/release
          gh release download "$tag" --repo orilloai/placket --dir "$d" \
            --pattern placket.rb --pattern checksums.txt --pattern checksums.txt.sigstore.json
          cosign verify-blob --bundle "$d/checksums.txt.sigstore.json" \
            --certificate-identity https://github.com/orilloai/placket/.github/workflows/release.yml@refs/heads/main \
            --certificate-oidc-issuer https://token.actions.githubusercontent.com \
            "$d/checksums.txt"
          # Each SHA-256 of the cask is an archive's line in checksums.txt, one for each of the four.
          n=0
          for s in $(sed -n 's/^ *sha256 "\([0-9a-f]\{64\}\)"$/\1/p' "$d/placket.rb"); do
            grep -q "^$s  placket_.*\.tar\.gz\$" "$d/checksums.txt" || { echo "::error::sha256 $s is no archive's line in checksums.txt"; exit 1; }
            n=$((n + 1))
          done
          [ "$n" -eq 4 ] || { echo "::error::the cask has $n SHA-256s, not 4"; exit 1; }
          mkdir -p Casks
          if cmp -s "$d/placket.rb" Casks/placket.rb; then echo "the cask is already $tag's"; exit 0; fi
          cp "$d/placket.rb" Casks/placket.rb
          git config user.name 'github-actions[bot]'
          git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
          git add Casks/placket.rb
          git commit -m "placket $tag"
          git push
```

### Versions

- Tags are `vMAJOR.MINOR.PATCH`, for the binary and the root module alike. The first release is `v0.1.0`.
- The protocol's version and each frozen format's version are their own, in the register `spec/frozen-formats.md` (`format-change-noted`); each release's notes name them.
- Go's version is set in two places that change together: the `toolchain` line of `go.mod`, which the release workflow builds with, and `GO_VERSION` in `.github/workflows/loom.yml`, which `make ci` runs with.
- GoReleaser, cosign and syft are pinned by version in the release workflow; a change of any of them may change the archives' bytes, so it lands in its own pull request.
