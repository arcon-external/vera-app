#!/bin/bash
# Stamp the release version into every artifact that carries one:
#   1. Go binary  via ldflags  (-X vera-app/backend.AppVersion=<tag>)
#   2. Installer  via wails.json info.productVersion
# All three derive from the single VERSION argument (a git tag like v0.2.0),
# so a missed step fails loudly here instead of shipping version-less or
# mismatched artifacts. Usage: VERSION=v0.2.0 scripts/stamp-version.sh
set -euo pipefail

if [ -z "${VERSION:-}" ]; then
  echo "usage: VERSION=vX.Y.Z $0" >&2
  exit 1
fi
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "VERSION $VERSION is not vMAJOR.MINOR.PATCH" >&2
  exit 1
fi

export LDFLAGS="-X vera-app/backend.AppVersion=$VERSION"

# The PDF engine a build carries, when it is not the default one.
#
# Empty means gpdf, the native engine, which needs no browser. Setting
# VERA_RENDER_ENGINE=chromedp selects the browser engine instead -- the one to
# reach for when the HTML/CSS template has changed and the native layout has not
# caught up. Both are the same document; only the renderer differs.
#
# This is a build input rather than a code change so a release can be turned
# around without touching the tree: set the repository variable and re-run the
# tag build.
export BUILD_TAGS=""
if [ "${VERA_RENDER_ENGINE:-}" = "chromedp" ]; then
  export BUILD_TAGS="chromedp"
fi

# wails_build wraps `wails build` so the engine tag is applied only when one was
# selected.
#
# A function rather than an array because an empty -tags argument is not the same
# as no -tags argument, and macOS runners ship bash 3.2, where expanding an empty
# array under `set -u` is an error. Every build script goes through here, so the
# tag cannot be applied in one place and forgotten in another.
wails_build() {
  if [ -n "$BUILD_TAGS" ]; then
    echo "building with engine tag: $BUILD_TAGS"
    wails build "$@" -tags "$BUILD_TAGS"
  else
    wails build "$@"
  fi
}

# wails.json info.productVersion must match the tag exactly, modulo the
# leading v that Wails convention omits ("0.1.0" satisfies tag "v0.1.0").
CURRENT=$(python3 -c "import json; print(json.load(open('wails.json'))['info']['productVersion'])")
if [ "$CURRENT" != "$VERSION" ] && [ "v$CURRENT" != "$VERSION" ]; then
  echo "wails.json info.productVersion is $CURRENT, want $VERSION (or ${VERSION#v})." >&2
  echo "Update it to the tag and re-run." >&2
  exit 1
fi

echo "version stamp OK: Go ldflags and wails.json info agree on $VERSION"
echo "build with: wails_build -ldflags \"\$LDFLAGS\" --platform <target>"
if [ -n "$BUILD_TAGS" ]; then
  echo "engine: chromedp (VERA_RENDER_ENGINE=chromedp)"
else
  echo "engine: gpdf (the default; set VERA_RENDER_ENGINE=chromedp for the browser)"
fi
