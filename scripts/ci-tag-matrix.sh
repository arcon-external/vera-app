#!/usr/bin/env bash
# The #495 tag matrix: the build-tag combinations Wails needs and the default
# suite never compiles.
#
# One source of truth. The PR `#495` step runs this inside the published
# GTK/WebKit image, and the ci-image publish job runs the same file against the
# candidate before `latest` is promoted. Selection and proof therefore cannot
# drift: a command added here is a command the image is validated with, and an
# image that cannot run this file never reaches PRs.
#
# `dev` and `production` select Wails' real Linux frontend, which cgo-links
# GTK/WebKit; only the default tags use Wails' no-cgo stub. Ubuntu 24.04 ships
# WebKit 4.1, so every root command adds `webkit2_41` and the consuming image
# supplies the packages. Without them the first command fails in pkg-config
# before any Go file is compiled.
#
# Root package covers default, packaged, dev and debug-packaged. The Wails-free
# auth package covers those plus dev+production, which pins the exclusive
# predicate (dev && !production versus !dev || production). Wails v2.16 itself
# declares App twice when both tags are set, so the dev+production root package
# cannot link; auth is where that combination is executable.
set -euo pipefail

# The tagged root package must compile, including its test files.
go vet -tags dev,webkit2_41 ./ ./backend/auth

# Default tags: the ordinary resolvers, then each tagged variant.
go test -count=1 \
  -run 'TestAPIBaseURL|TestProductionAPIURL|TestKeyring' ./ ./backend/auth
for tags in production dev production,debug; do
  go test -tags "$tags,webkit2_41" -count=1 \
    -run 'TestAPIBaseURL|TestProductionAPIURL|TestKeyring' ./ ./backend/auth
done

# dev+production together is only executable in the Wails-free auth package.
go test -tags dev,production -count=1 \
  -run 'TestKeyring' ./backend/auth
