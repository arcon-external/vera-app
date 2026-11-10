#!/bin/bash
# Generate the Wails bindings and prove the committed copy matches the Go API.
#
# `wails generate module` rewrites frontend/wailsjs on every run, and the output
# is not stable in the ways that matter for a diff:
#
#   * every otherwise-blank line gets a tab (552 lines in models.ts at v2.17.1),
#     so a regeneration that changed nothing looks like a rewrite;
#   * every file it writes is marked executable, while the committed copy is
#     plain text.
#
# That churn buries the one line that matters: a bound method the generator did
# not emit, which is a method the frontend cannot call. This script regenerates,
# normalizes the churn away, runs the bindings guard in bindings_test.go, and
# fails when anything is still different. Commit the result; the diff is then
# the real change and nothing else.
#
# There is no formatter in this repository, and running one over the generated
# TypeScript would itself be the large diff this script exists to avoid. The
# normalizer is therefore deliberately small: strip trailing whitespace, use LF,
# end with one newline, and make the files non-executable.
#
# Usage:
#   scripts/gen-bindings.sh [--root DIR] [--check]
#
#   --root DIR   repository root to operate on. Defaults to VERA_REPO_ROOT,
#                then to the parent of this script's directory.
#   --check      dry run: do the same work, then restore frontend/wailsjs, so
#                the tree is exactly as it was. Still exits non-zero when the
#                committed bindings would change. This is the CI/test form.
#
# Needs: wails (matching go.mod), go, python3, git.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="${VERA_REPO_ROOT:-}"
check=false

usage() {
  cat <<'EOF'
usage: gen-bindings.sh [--root DIR] [--check]

  --root DIR   repository root to operate on. Defaults to VERA_REPO_ROOT,
               then to the parent of this script's directory.
  --check      dry run: do the same work, then restore frontend/wailsjs, so
               the tree is exactly as it was. Still exits non-zero when the
               committed bindings would change. This is the CI/test form.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --root) root="${2:-}"; shift 2 ;;
    --root=*) root="${1#--root=}"; shift ;;
    --check) check=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "gen-bindings.sh: unknown argument $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ -z "$root" ]; then
  root="$(cd "$script_dir/.." && pwd)"
fi
requested_root="$root"
if ! root="$(cd "$requested_root" 2>/dev/null && pwd)"; then
  echo "gen-bindings.sh: root does not exist: $requested_root" >&2
  exit 2
fi

fail() {
  echo "gen-bindings: $*" >&2
  exit 1
}

[ -f "$root/wails.json" ] || fail "$root has no wails.json; --root must point at the repository root"
[ -f "$root/bindings_test.go" ] || fail "$root has no bindings_test.go"
[ -d "$root/frontend/wailsjs" ] || fail "$root has no frontend/wailsjs; run wails generate module first"
command -v wails >/dev/null 2>&1 || fail "wails is not on PATH; install the version go.mod pins"
command -v go >/dev/null 2>&1 || fail "go is not on PATH"
command -v python3 >/dev/null 2>&1 || fail "python3 is not on PATH (the normalizer uses it)"
git -C "$root" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "$root is not a git work tree; the diff check needs one"

# wails generate module compiles the root package, and main.go embeds
# frontend/dist. A fresh checkout or a clean worktree has no dist, and the
# generator fails before writing anything. A placeholder is enough for the
# embed; it is removed again on exit.
created_dist=""
if [ ! -d "$root/frontend/dist" ]; then
  mkdir -p "$root/frontend/dist"
  printf '<!doctype html>\n' > "$root/frontend/dist/index.html"
  created_dist="$root/frontend/dist"
elif [ -z "$(ls -A "$root/frontend/dist" 2>/dev/null)" ]; then
  printf '<!doctype html>\n' > "$root/frontend/dist/index.html"
  created_dist="$root/frontend/dist/index.html"
fi

# --check is a dry run on net: snapshot the generated tree, and put it back
# whatever happens. The diff check still runs against the regenerated tree
# before this restore.
snapshot=""
if [ "$check" = true ]; then
  snapshot="$(mktemp -d "${TMPDIR:-/tmp}/gen-bindings.XXXXXX")"
  cp -a "$root/frontend/wailsjs/." "$snapshot/"
fi

cleanup() {
  if [ -n "$snapshot" ]; then
    rm -rf "$root/frontend/wailsjs"
    mkdir -p "$root/frontend/wailsjs"
    cp -a "$snapshot/." "$root/frontend/wailsjs/"
    rm -rf "$snapshot"
  fi
  if [ -n "$created_dist" ]; then
    if [ -d "$created_dist" ]; then
      rm -rf "$created_dist"
    else
      rm -f "$created_dist"
    fi
  fi
}
trap cleanup EXIT

# The normalizer. Wails writes tabs on blank lines, CRLF on Windows, and a
# trailing newline it does not always keep; none of those carry meaning.
# Indentation is left alone on purpose: models.ts is tab-indented and the
# committed copy is tab-indented, so reindenting would create the diff this
# script is meant to prevent.
normalize_bindings() {
  python3 - "$1" <<'PY'
import os
import sys

root = sys.argv[1]
for dirpath, dirnames, filenames in os.walk(root):
    dirnames.sort()
    for name in sorted(filenames):
        path = os.path.join(dirpath, name)
        if os.path.islink(path) or not os.path.isfile(path):
            continue
        with open(path, "r", encoding="utf-8", newline="") as handle:
            text = handle.read()
        lines = text.replace("\r\n", "\n").replace("\r", "\n").split("\n")
        if lines and lines[-1] == "":
            lines.pop()
        normalized = "\n".join(line.rstrip(" \t") for line in lines)
        if normalized:
            normalized += "\n"
        with open(path, "w", encoding="utf-8", newline="\n") as handle:
            handle.write(normalized)
        os.chmod(path, 0o644)
PY
}

echo "gen-bindings: generating frontend/wailsjs"
if ! ( cd "$root" && wails generate module ); then
  fail "wails generate module failed"
fi

echo "gen-bindings: normalizing generator whitespace and file modes"
normalize_bindings "$root/frontend/wailsjs"

# The guard is the same one the Go suite runs. `-count=1` because the point is
# to judge the files just written, not a cached result.
echo "gen-bindings: running the bindings guard"
if ! ( cd "$root" && go test . -run '^TestGenerated' -count=1 ); then
  fail "the bindings guard failed; the generated files do not match the Go API"
fi

# git status rather than git diff: a newly bound type produces new files, and
# an untracked file is a binding that is not committed either.
status="$(git -C "$root" status --porcelain=v1 --untracked-files=all -- frontend/wailsjs)"
if [ -n "$status" ]; then
  echo "gen-bindings: frontend/wailsjs still differs after normalization:" >&2
  printf '%s\n' "$status" >&2
  echo >&2
  git -C "$root" --no-pager diff -- frontend/wailsjs >&2 || true
  fail "commit the regenerated bindings, or revert them if the change is not wanted"
fi

echo "gen-bindings: OK: the committed bindings match the Go API"
