#!/usr/bin/env bash
#
# build-po-extract-image.sh — build the pinned text-PDF extractor image and
# print the engine version it carries.
#
# The Dockerfile pins both base images by digest, pins poppler-utils to an exact
# Debian package version, and records the installed Poppler version at build
# time. This script is the one-liner the deployment notes use; it changes nothing
# unless the image tag already exists (Docker replaces it).
#
# Override the tag with VERA_PO_EXTRACT_IMAGE.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image="${VERA_PO_EXTRACT_IMAGE:-vera-po-extract:dev}"

docker build -f "$repo_root/docker/po-extract/Dockerfile" -t "$image" "$repo_root"
docker run --rm "$image" -version
