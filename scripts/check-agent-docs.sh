#!/bin/bash
# Check that every docs/agents/*.md pointer named in AGENTS.md resolves.
#
# AGENTS.md is the always-loaded file; the detail lives under docs/agents/. A
# pointer that names a moved or renamed file is worse than no pointer: an agent
# follows it, finds nothing, and reconstructs the rule from memory. So this
# check reads every docs/agents path AGENTS.md names and fails on a missing one,
# naming the pointer and the file that named it.
#
# Usage: scripts/check-agent-docs.sh
#
# Exit status:
#   0  every pointer resolves, and AGENTS.md names at least one
#   1  a pointer does not resolve, or AGENTS.md names no pointer at all
#   2  AGENTS.md is missing, or an unknown argument was given
set -euo pipefail

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
agents="$root/AGENTS.md"

case "${1:-}" in
  "") ;;
  -h | --help)
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
    ;;
  *)
    echo "usage: $0" >&2
    exit 2
    ;;
esac

if [ ! -f "$agents" ]; then
  echo "missing $agents" >&2
  exit 2
fi

pointers="$(grep -oE 'docs/agents/[A-Za-z0-9._-]+\.md' "$agents" | sort -u || true)"

if [ -z "$pointers" ]; then
  echo "AGENTS.md names no docs/agents/*.md pointer" >&2
  exit 1
fi

count=0
missing=0
while IFS= read -r pointer; do
  [ -n "$pointer" ] || continue
  count=$((count + 1))
  if [ ! -f "$root/$pointer" ]; then
    echo "missing pointer: $pointer (named in AGENTS.md)" >&2
    missing=$((missing + 1))
  fi
done <<<"$pointers"

if [ "$missing" -ne 0 ]; then
  exit 1
fi

echo "agent docs OK: $count pointer(s) resolve"
exit 0
