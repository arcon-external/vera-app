#! /bin/bash
# Windows installer build. Produces the version-stamped NSIS installer at
# build/bin/vera-app-<version>-windows-amd64-installer.exe.
#
# Requires: Go, Node 22, the Wails CLI, and NSIS (makensis) for the -nsis
# step. VERSION defaults to the current git tag (or commit when untagged,
# which stamps a dev build that never satisfies an update check).
set -euo pipefail
cd "$(dirname "$0")/.."

export VERSION="${VERSION:-$(git describe --tags --always)}"
# shellcheck disable=SC1091
source ./scripts/stamp-version.sh

echo -e "Building the Windows installer for $VERSION..."
wails build --clean --platform windows/amd64 -nsis -ldflags "$LDFLAGS"

echo -e "Renaming to the release asset convention..."
shopt -s nullglob
moved=0
for f in build/bin/*-installer.exe; do
  if [[ "$f" != *"windows-amd64-installer.exe" ]]; then
    mv "$f" "build/bin/vera-app-$VERSION-windows-amd64-installer.exe"
    moved=1
  fi
done
shopt -u nullglob
[ "$moved" = 1 ] || ls build/bin/*windows-amd64-installer.exe >/dev/null
ls build/bin/*windows-amd64-installer.exe
echo -e "Done."
