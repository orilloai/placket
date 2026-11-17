# Releases

A placket release is a GitHub release of the tag `vX.Y.Z`. It holds:

- `placket` for Linux and macOS, on amd64 and arm64, one `.tar.gz` archive per platform, each holding the binary, `LICENSE` and `README.md`;
- one SBOM per archive, in SPDX JSON, `<archive>.sbom.json`;
- `checksums.txt`, the SHA-256 of every archive and SBOM;
- `checksums.txt.sigstore.json`, the cosign bundle that signs `checksums.txt`;
- build provenance for every archive and for `checksums.txt`, kept by GitHub.

Signing and provenance use the release workflow's own GitHub identity: there is no key. Anyone can check that an archive came from placket's release workflow on `main`, and rebuild the binary and its archive to the same bytes. The SBOMs carry their own creation time, so they are signed, through `checksums.txt`, but a rebuild does not reproduce them.

`placket version` says which release a binary is:

```
version v0.1.0
commit 0123456789abcdef0123456789abcdef01234567
commitTime 2026-12-01T09:30:00Z
go go1.27.4
module 0.1.0 sha256:...
```

The version is the one Go stamps from the git state of the build: the tag for a build at a tag, a pseudo-version for any other commit, and a version ending `+dirty` for a tree with changes not committed. A build with no git information, such as one from a source tarball, prints `(devel)`. `commit` and `commitTime` are printed only when the build carries them. `module` is the decision module, as `placket card --default` names it. With `--json`, it prints one object of the same members.

## For users

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
     --signer-workflow orilloai/placket/.github/workflows/release.yml \
     --source-ref refs/heads/main --deny-self-hosted-runners
   ```

4. The version: unpack the archive for your platform and run `./placket version`. Its first line is `version v0.1.0`.

### Rebuild

A rebuild needs a git clone at the tag, since Go reads the version from git; a source tarball has no git information and builds `(devel)`. It needs the Go that `go.mod`'s `toolchain` line names, which the `go` command fetches by itself unless `GOTOOLCHAIN` says otherwise. Build to a directory outside the clone, so the tree stays clean and Go stamps no `+dirty`:

```sh
git clone --branch v0.1.0 https://github.com/orilloai/placket.git
cd placket
OUT=$(mktemp -d)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$OUT/placket" ./cmd/placket
tar -xzOf ../placket-v0.1.0/placket_0.1.0_linux_amd64.tar.gz placket | cmp - "$OUT/placket"
```

`cmp` prints nothing when the two are the same bytes. Set `GOOS` and `GOARCH` for the archive to compare against: `linux` or `darwin`, `amd64` or `arm64`. The archive itself is rebuilt by the release tooling at the tag, `goreleaser release --snapshot --clean --skip=sign` with the version of GoReleaser the release workflow pins: every time inside an archive is the commit's. A snapshot's archives are named `placket_<version>-SNAPSHOT-<commit>_<os>_<arch>.tar.gz`, not `placket_<version>_<os>_<arch>.tar.gz`, so compare the hashes platform by platform, not the lines of the two `checksums.txt`:

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

A `v*` tag pushed by hand starts nothing, and since no `v*` tag can be moved or deleted, its version number is used up: a release PR for it is refused as "a tag at another commit".

A release can be withdrawn from GitHub, but its signature in the public transparency log and its provenance records stay.

On every pull request, the workflow also builds a snapshot twice without signing, checks that the two builds' archives are the same bytes, and runs `scripts/release-check.sh --snapshot` on it. It is not a required check.

### Versions

- Tags are `vMAJOR.MINOR.PATCH`, for the binary and the root module alike. The first release is `v0.1.0`.
- The protocol's version and each frozen format's version are their own, in the register `spec/frozen-formats.md` (`format-change-noted`); each release's notes name them.
- Go's version is set in two places that change together: the `toolchain` line of `go.mod`, which the release workflow builds with, and `GO_VERSION` in `.github/workflows/loom.yml`, which `make ci` runs with.
- GoReleaser, cosign and syft are pinned by version in the release workflow; a change of any of them may change the archives' bytes, so it lands in its own pull request.
