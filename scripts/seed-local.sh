#!/usr/bin/env bash
# Seed the confirmed local database with the fictional #500 fixture set.
#
# This wrapper only loads the server-owned, gitignored project .env and runs the
# standalone veraseed command. It does not choose a database: --target is
# required, must name the resolved database exactly, and is checked before the
# store migrates or writes. Set VERA_POSTGRES_DSN and VERA_POSTGRES_SCHEMA in
# .env (or the environment) exactly as scripts/dev-api.sh does.
#
#   scripts/seed-local.sh --target vera_local
#   scripts/seed-local.sh --target vera_local --json
#
# See docs/local-seed.md. Production is never a valid target.
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

exec go run ./cmd/veraseed "$@"
