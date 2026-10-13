#!/usr/bin/env bash
#
# ui-snapshot.sh — render registered React panels to PNGs at 360 and 1280.
#
# Local evidence only: the output lives outside the repository and is never
# committed. Run this for a change that alters a panel's layout, then read both
# PNGs (360 and 1280) — the pull request states what was seen. Interactions
# (keyboard, paste, focus) stay hand-driven.
#
# Steps: build the frontend (fresh CSS), run the harness (one self-contained
# HTML per case), then screenshot every HTML in a docker Chrome twice.
#
# Overrides:
#   UI_SNAPSHOT_DIR  output directory   (default /tmp/opencode/ui-snap)
#   CHROME_DIR       unpacked Chrome dir (default the pinned linux build)
#   VERA_CHROME_IMAGE docker image       (default vera-chrome-go:latest)

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

out_dir="${UI_SNAPSHOT_DIR:-/tmp/opencode/ui-snap}"
chrome_dir="${CHROME_DIR:-/tmp/opencode/chrome-dl/chrome/linux-155.0.8059.39/chrome-linux64}"
image="${VERA_CHROME_IMAGE:-vera-chrome-go:latest}"
wrapper_dir="${UI_SNAPSHOT_WRAPPER_DIR:-/tmp/opencode/chrome-wrapper}"

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
  sed -n '3,16p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  exit 0
fi

echo "==> building the frontend (fresh CSS)"
npm --prefix "$repo_root/frontend" run build

echo "==> running the harness (UI_SNAPSHOT=1)"
mkdir -p "$out_dir"
(
  cd "$repo_root/frontend"
  UI_SNAPSHOT=1 UI_SNAPSHOT_DIR="$out_dir" npx vitest run src/ui-snapshot.harness.test.tsx
)

if [ ! -d "$chrome_dir" ]; then
  echo "error: Chrome not found at $chrome_dir" >&2
  echo "       set CHROME_DIR to an unpacked chrome-linux64 directory" >&2
  exit 1
fi

# Chrome runs unsandboxed in the container, so the wrapper adds --no-sandbox and
# resolves the mounted binary at /chrome.
mkdir -p "$wrapper_dir"
cat > "$wrapper_dir/chrome" <<'WRAPPER'
#!/bin/sh
exec /chrome/chrome --no-sandbox "$@"
WRAPPER
chmod +x "$wrapper_dir/chrome"

shopt -s nullglob
html_files=("$out_dir"/*.html)
if [ "${#html_files[@]}" -eq 0 ]; then
  echo "error: no HTML produced in $out_dir" >&2
  exit 1
fi

pngs=()
for html in "${html_files[@]}"; do
  base="$(basename "$html" .html)"
  for size in 360x900 1280x900; do
    width="${size%x*}"
    height="${size#*x}"
    png="/out/${base}-${width}.png"
    echo "==> screenshot $base at ${width}x${height}"
    docker run --rm \
      -v "$out_dir:/out" \
      -v "$chrome_dir:/chrome:ro" \
      -v "$wrapper_dir:/wrapper:ro" \
      -e PATH=/wrapper:/usr/bin:/bin \
      "$image" \
      chrome --headless=new --disable-gpu --hide-scrollbars \
        --screenshot="$png" \
        --window-size="${width},${height}" \
        "file:///out/$(basename "$html")"
    pngs+=("$out_dir/${base}-${width}.png")
  done
done

echo
echo "==> snapshots:"
for png in "${pngs[@]}"; do
  echo "    $png"
done
