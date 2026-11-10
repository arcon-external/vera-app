#!/bin/bash
# Verify that frontend/package.json.md5 matches frontend/package.json.
#
# Wails keeps this stamp beside the package manifest to decide whether a build
# needs `npm install`, and `wails build` rewrites it as a side effect. That is
# how it drifts: a manifest edit lands, the stamp is refreshed locally by a
# build, and the refreshed file is never committed -- so the next checkout
# compares a fresh manifest against a stale hash.
#
# usage: scripts/check-wails-md5.sh [--fix]
#   (no argument)  verify; exit 1 and print both hashes when stale
#   --fix          rewrite the stamp from the manifest, then verify
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="$root/frontend/package.json"
stamp="$root/frontend/package.json.md5"

fix=0
case "${1:-}" in
  "") ;;
  --fix) fix=1 ;;
  -h | --help)
    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
    ;;
  *)
    echo "usage: $0 [--fix]" >&2
    exit 2
    ;;
esac

if [ ! -f "$manifest" ]; then
  echo "missing $manifest" >&2
  exit 1
fi

md5_of() {
  if command -v md5sum >/dev/null 2>&1; then
    md5sum -- "$1" | awk '{print $1}'
  elif command -v md5 >/dev/null 2>&1; then
    md5 -q "$1"
  else
    echo "need md5sum (Linux) or md5 (macOS) to hash $1" >&2
    return 1
  fi
}

want="$(md5_of "$manifest")"
have=""
if [ -f "$stamp" ]; then
  have="$(tr -d '[:space:]' <"$stamp")"
fi

if [ "$have" = "$want" ]; then
  echo "frontend/package.json.md5 matches frontend/package.json ($want)"
  exit 0
fi

if [ "$fix" -eq 1 ]; then
  printf '%s\n' "$want" >"$stamp"
  echo "refreshed frontend/package.json.md5 ($want)"
  exit 0
fi

echo "frontend/package.json.md5 is stale." >&2
echo "  manifest: $want" >&2
echo "  stamp:    ${have:-<missing>}" >&2
echo "run: scripts/check-wails-md5.sh --fix" >&2
exit 1
