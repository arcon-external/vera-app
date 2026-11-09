#!/usr/bin/env bash
# Start the local API for desktop development.
#
# The desktop never owns the database or the server lifecycle: this script is
# the server half, run in its own terminal beside `wails dev`.
#
# It loads the server-owned, gitignored project .env (DSN, signing key,
# bootstrap admin) and then forces a loopback bind. VERA_API_ADDR is set after
# the file is sourced so a copied .env.example cannot accidentally expose the
# development API on every interface. Production is untouched: veraserver's
# compiled default stays :8080 for Cloud Run.
#
# --local-development makes the server refuse a remote, multi-host, or
# unparseable Postgres target before it opens the store, so a copied production
# DSN or PG* environment can never migrate or seed anything here.
set -euo pipefail

cd "$(dirname "$0")/.."

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

export VERA_API_ADDR=127.0.0.1:8081

exec go run ./cmd/veraserver --local-development
