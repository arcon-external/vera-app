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

# wails.json info.productVersion must match the tag exactly, modulo the
# leading v that Wails convention omits ("0.1.0" satisfies tag "v0.1.0").
CURRENT=$(python3 -c "import json; print(json.load(open('wails.json'))['info']['productVersion'])")
if [ "$CURRENT" != "$VERSION" ] && [ "v$CURRENT" != "$VERSION" ]; then
  echo "wails.json info.productVersion is $CURRENT, want $VERSION (or ${VERSION#v})." >&2
  echo "Update it to the tag and re-run." >&2
  exit 1
fi

echo "version stamp OK: Go ldflags and wails.json info agree on $VERSION"
echo "build with: wails build -ldflags \"\$LDFLAGS\" --platform <target>"
