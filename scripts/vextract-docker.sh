#!/usr/bin/env bash
#
# vextract-docker.sh — run the text-PDF extractor in its restricted container.
#
# The server runs a command with the PDF on stdin and the extraction contract
# on stdout. Point VERA_EXTRACTOR_CMD at this wrapper during development and
# the same stdin/stdout contract reaches the image:
#
#   VERA_EXTRACTOR_CMD="$PWD/scripts/vextract-docker.sh" go run ./cmd/veraserver
#
# There is no in-process fallback: the server image ships no extractor, and an
# empty VERA_EXTRACTOR_CMD disables extraction (the start endpoint answers 503).
# A same-UID child in the server container could read the parent's /proc
# environment, so this wrapper (or an equivalent isolated worker with no
# database or signing credentials) is the supported path.
#
# The container has no network, no writable root filesystem, no credentials and
# hard memory/PID/CPU caps. The PDF is the only input and the JSON result is the
# only output. Build the image first:
#
#   docker build -f docker/po-extract/Dockerfile -t vera-po-extract:dev .
#
# Overrides: VERA_PO_EXTRACT_IMAGE, VERA_PO_EXTRACT_MEMORY, VERA_PO_EXTRACT_CPUS.
set -euo pipefail

image="${VERA_PO_EXTRACT_IMAGE:-vera-po-extract:dev}"
memory="${VERA_PO_EXTRACT_MEMORY:-512m}"
cpus="${VERA_PO_EXTRACT_CPUS:-1}"

exec docker run --rm -i \
  --network none \
  --read-only \
  --tmpfs /tmp:rw,size=64m,noexec,nosuid \
  --memory "$memory" \
  --memory-swap "$memory" \
  --pids-limit 64 \
  --cpus "$cpus" \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  -e HOME=/tmp \
  -e "VERA_EXTRACT_MAX_PAGES=${VERA_EXTRACT_MAX_PAGES:-40}" \
  "$image" "$@"
