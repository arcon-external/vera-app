#! /bin/bash
# macOS universal build plus drag-to-Applications DMG.
# Produces build/bin/Vera-<version>-macos-universal.dmg.
#
# Requires: macOS (CGO/WebKit cannot cross-compile), Go, Node 22, the Wails
# CLI, and create-dmg (brew install create-dmg) for the DMG step. VERSION
# defaults to the current git tag (or commit when untagged).
set -euo pipefail
cd "$(dirname "$0")/.."

export VERSION="${VERSION:-$(git describe --tags --always)}"
# shellcheck disable=SC1091
source ./scripts/stamp-version.sh

echo -e "Building the macOS app for $VERSION..."
wails_build --clean --platform darwin/universal -ldflags "$LDFLAGS"

APP="$(ls -d build/bin/*.app | head -1)"
echo -e "Wrapping $APP in a DMG..."
if ! command -v create-dmg >/dev/null 2>&1; then
  echo "create-dmg not found (brew install create-dmg); leaving $APP unwrapped." >&2
  exit 1
fi
DMG="build/bin/Vera-$VERSION-macos-universal.dmg"
rm -f "$DMG"
create-dmg \
  --volname "Vera $VERSION" \
  --window-size 540 380 \
  --app-drop-link 380 180 \
  "$DMG" "$APP"
ls "$DMG"
echo -e "Done."
