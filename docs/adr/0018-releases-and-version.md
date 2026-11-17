# 0018. Releases are signed without a key and rebuild to the same bytes, and `placket version` names a build

Status: accepted. Date: 2026-11-17. Records a decision built before its record; `docs/release.md` holds the steps.

## Context

Placket has no release yet, so anyone who wants the binary builds it from a checkout. A first release is what outside users get, so it has to be provable from the start: anyone should be able to check a binary's signature and provenance, and rebuild it to the same bytes. A user holding a binary also needs to say which build it is.

The release pieces and `placket version` landed before this record. It writes down what was decided and why, as far as the reasons were stated at the time. A reason nobody stated is marked "not recorded".

## Decision

- **What a release holds.** A GitHub release of the tag `vX.Y.Z`: one `.tar.gz` archive per platform, Linux and macOS on amd64 and arm64, each holding the binary, `LICENSE` and `README.md`; an SPDX SBOM per archive; `checksums.txt`; its cosign bundle; and GitHub's build provenance for every archive and for `checksums.txt`. Why these platforms: not recorded.
- **Signed without a key.** cosign signs `checksums.txt` with the release workflow's own GitHub identity, and the provenance uses the same identity. So there is no key to keep, and anyone can check that an archive came from placket's release workflow on `main`. The checks pin the exact certificate identity and issuer, with the source ref `refs/heads/main` and GitHub's own runners only. The bundle uses cosign's new bundle format, read by cosign 2.4 and later.
- **Reproducible.** The archives rebuild to the same bytes from a git clone at the tag: every time inside an archive is the commit's, and the build sets `GOTOOLCHAIN` to the version of `go.mod`'s `toolchain` line, because that line is only a minimum and a newer local Go builds other bytes. The SBOMs carry their own creation time, so they are signed through `checksums.txt` but are not reproduced.
- **A release is a merged release PR.** A release PR moves `CHANGELOG.md`'s `Unreleased` section under the new version and marks the register's formats released; approving it is the decision to release. `scripts/release-detect.sh` reads each push to `main` and says whether it is a release. It refuses a release while the repository is private, and does so only for a push that would release, so the workflow does not fail on every push before placket is public. A tag pushed by hand starts nothing.
- **The release workflow is a plan.** `.github/workflows/release.yml` is not in the repository yet. It comes in a workflow change of its own, the maintainer's.
- **`placket version`.** A command that prints, on standard output since the version is its result, `version`, then `commit` and `commitTime` when the build carries them, `go`, and `module`, the decision module as `placket card --default` names it. `--json` prints one object of the same members. The version is the one Go stamps from the git state: the tag at a tag, a pseudo-version at another commit, `+dirty` for a tree with changes not committed, and `(devel)` for a build with no git information. It is read from the build info, never set with `-X`; why: not recorded. The command adds a row to the requirement text's command table, and no format, receipt or vector changes.

## Consequences

- A user can check a release with `scripts/release-check.sh`, or by hand with the steps in `docs/release.md`, and rebuild it.
- Changing the pinned GoReleaser, cosign or syft may change the archives' bytes, so each lands in its own pull request.
- A version number whose release push was refused is released by taking its section back under `Unreleased` and adding it again, or by releasing the next version.
