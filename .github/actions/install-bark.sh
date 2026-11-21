#!/usr/bin/env bash
# Installs a Bark release binary for the bark-review and bark-pr-description
# actions and puts it on the PATH.
#
# BARK_VERSION picks the release:
#   - empty: the release the action itself comes from, read from the
#     CHANGELOG.md next to it, so ionut-t/bark/...@v2.35.0 installs Bark
#     v2.35.0 and upgrading the action's ref upgrades the binary with it;
#   - "latest": the newest release;
#   - a tag such as v2.35.0.
#
# The binary is checked against the release's checksums.txt and, for
# releases built since provenance attestations were added, against
# GitHub's signed build provenance: it must have been built by the
# release workflow on main of ionut-t/bark. Checksums alone come from the
# same release as the binary, so they don't catch a replaced release.
#
# Needs GH_TOKEN, RUNNER_OS, RUNNER_ARCH, RUNNER_TEMP, GITHUB_PATH and
# GITHUB_ACTION_PATH (set by the runner).
set -euo pipefail

REPO=ionut-t/bark
RELEASE_WORKFLOW=.github/workflows/release-please.yaml

case "$RUNNER_OS-$RUNNER_ARCH" in
  Linux-X64)     ASSET=bark-linux-amd64;       BIN=bark ;;
  Linux-ARM64)   ASSET=bark-linux-arm64;       BIN=bark ;;
  macOS-X64)     ASSET=bark-darwin-amd64;      BIN=bark ;;
  macOS-ARM64)   ASSET=bark-darwin-arm64;      BIN=bark ;;
  Windows-X64)   ASSET=bark-windows-amd64.exe; BIN=bark.exe ;;
  Windows-ARM64) ASSET=bark-windows-arm64.exe; BIN=bark.exe ;;
  *) echo "Error: unsupported runner $RUNNER_OS-$RUNNER_ARCH" >&2; exit 1 ;;
esac

case "${BARK_VERSION:-}" in
  "")
    # The action's files are the whole repository at the action's ref;
    # Release Please puts the newest release first in its CHANGELOG.md.
    CHANGELOG="$GITHUB_ACTION_PATH/../../../CHANGELOG.md"
    VERSION=$(awk -F'[][]' '/^## \[/ { print $2; exit }' "$CHANGELOG" 2>/dev/null || true)
    if [ -z "$VERSION" ]; then
      echo "Error: can't tell which Bark release this action comes from (no version in $CHANGELOG); set bark-version." >&2
      exit 1
    fi
    TAG="v$VERSION"
    ;;
  latest)
    TAG=$(gh release view --repo "$REPO" --json tagName --jq .tagName)
    ;;
  *)
    TAG="$BARK_VERSION"
    ;;
esac
echo "Installing Bark $TAG ($ASSET)"

DEST="$RUNNER_TEMP/bark-bin"
mkdir -p "$DEST"
gh release download "$TAG" --repo "$REPO" --pattern "$ASSET" --pattern checksums.txt --dir "$DEST" --clobber

cd "$DEST"
if command -v sha256sum >/dev/null; then
  awk -v f="$ASSET" '$2 == f' checksums.txt | sha256sum -c -
else
  awk -v f="$ASSET" '$2 == f' checksums.txt | shasum -a 256 -c -
fi

# Releases from before the release workflow attested its builds have no
# provenance to check. Whether a release should have it is read from the
# release workflow at the release's tag, not from the release's assets,
# which whoever replaced a binary could change too. Read into a variable,
# not piped to grep -q: grep exiting early can kill gh with SIGPIPE, and
# under pipefail that would skip the check. A failed read stops the install.
WORKFLOW_AT_TAG=$(gh api -H "Accept: application/vnd.github.raw" "repos/$REPO/contents/$RELEASE_WORKFLOW?ref=$TAG")
if [[ "$WORKFLOW_AT_TAG" == *attest-build-provenance* ]]; then
  gh attestation verify "$ASSET" --repo "$REPO" \
    --signer-workflow "$REPO/$RELEASE_WORKFLOW" \
    --source-ref refs/heads/main
else
  echo "::notice::Bark $TAG predates build provenance; checked its checksum only."
fi

mv "$ASSET" "$BIN"
chmod +x "$BIN"
echo "$DEST" >> "$GITHUB_PATH"
