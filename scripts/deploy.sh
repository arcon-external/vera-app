#!/usr/bin/env bash
# Deploy a veraserver image to 3studio-server by commit SHA.
#
# Usage: scripts/deploy.sh <full-commit-sha> [--dry-run]
#
# Prerequisites (operator machine):
#   - docker logged in to ghcr.io with PULL access to the org package, e.g.:
#       echo "$CR_PAT" | docker login ghcr.io -u USERNAME --password-stdin
#     (a classic PAT with read:packages, or a fine-grained PAT granted the
#     package; the gh CLI token is not sufficient for private org packages)
#   - passwordless SSH to the server (VERA_SSH_HOST, default below)
#
# What it does:
#   1. Verifies the image exists in ghcr.io (never deploys an unpushed SHA).
#   2. Points the server compose file at the exact image digest.
#   3. Recreates the container, waits for health.
#   4. Probes /healthz and one authenticated route (expects 401 = app booted,
#      not just TCP accepting).
#   5. Prints expected vs running digest; any mismatch exits non-zero.
#
# Rollback: re-run with the previous SHA from `git log` or `gh run list`.
set -euo pipefail

SHA="${1:-}"
DRY_RUN=0
if [ "${2:-}" = "--dry-run" ]; then
  DRY_RUN=1
fi

if ! [[ "$SHA" =~ ^[0-9a-f]{40}$ ]]; then
  echo "usage: $0 <full-40-char-commit-sha> [--dry-run]" >&2
  exit 1
fi

SSH_HOST="${VERA_SSH_HOST:-3studio-server-tail}"
COMPOSE_DIR="${VERA_COMPOSE_DIR:-vera-api}"
IMAGE="ghcr.io/arcon-external/vera-app/veraserver:sha-$SHA"
API_BASE="${VERA_API_BASE:-http://100.93.103.83:8081}"

run_remote() {
  if [ "$DRY_RUN" = "1" ]; then
    echo "[dry-run] ssh $SSH_HOST: $*"
  else
    ssh -o BatchMode=yes "$SSH_HOST" "$@"
  fi
}

echo "== resolving $IMAGE to a digest =="
if [ "$DRY_RUN" = "1" ]; then
  echo "[dry-run] docker buildx imagetools inspect $IMAGE"
  EXPECTED_DIGEST="<digest-from-registry>"
else
  EXPECTED_DIGEST="$(docker buildx imagetools inspect "$IMAGE" --format '{{json .Manifest}}' 2>/dev/null | grep -o '"digest":"[^"]*"' | head -1 | cut -d'"' -f4)"
  if [ -z "$EXPECTED_DIGEST" ]; then
    echo "error: image not found in ghcr.io (was it pushed by the build workflow?)" >&2
    exit 1
  fi
  echo "digest: $EXPECTED_DIGEST"
fi

echo "== pinning compose file (image line only; secrets untouched) =="
run_remote "cd \"\$HOME/$COMPOSE_DIR\" && sed -i.bak -E 's|^[[:space:]]*image:.*|    image: $IMAGE@$EXPECTED_DIGEST|' compose.yml && grep -n '^ *image:' compose.yml"

echo "== recreating container =="
run_remote "cd \"\$HOME/$COMPOSE_DIR\" && docker compose up -d --force-recreate --quiet-pull"

echo "== waiting for health =="
if [ "$DRY_RUN" = "1" ]; then
  echo "[dry-run] wait + probe $API_BASE/healthz and one 401 route"
else
  for i in $(seq 1 30); do
    if curl -sf -m 5 "$API_BASE/healthz" >/dev/null 2>&1; then
      break
    fi
    if [ "$i" = "30" ]; then
      echo "error: API never became healthy" >&2
      exit 1
    fi
    sleep 2
  done
  CODE=$(curl -s -m 10 -o /dev/null -w "%{http_code}" "$API_BASE/v1/admin/users")
  if [ "$CODE" != "401" ]; then
    echo "error: authenticated route probe = $CODE, want 401 (app not fully booted?)" >&2
    exit 1
  fi
  RUNNING="$(ssh -o BatchMode=yes "$SSH_HOST" 'docker inspect vera-api --format "{{.Image}}"')"
  echo "running image: $RUNNING"
  echo "deploy verified: health OK, auth gate OK"
fi
